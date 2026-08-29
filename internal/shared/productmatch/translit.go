package productmatch

import "strings"

// Comparing an Arabic name with a Latin one.
//
// Every product in the catalogue carries both names — 19,996 of 19,996 — and
// every supplier and pharmacy file is written in one of them. The scorer could
// not cross between: Arabic query tokens were compared against the catalogue's
// Arabic tokens and Latin against Latin, and the trigram fallback is built per
// token, so it shares nothing across scripts either. "بوباى صن سكرين" against
// "bobai sunscreen" scored exactly zero, and the two rows are the same product.
//
// The AI prompt is told about transliteration at length. The deterministic
// engine that runs first, on every row, was not told at all — so the cases the
// model was being paid to resolve were mostly cases arithmetic could have
// resolved for nothing.
//
// The bridge is a skeleton both scripts fold into: consonants only, one letter
// per sound, doubles collapsed. It is deliberately lossy. Egyptians write the
// same brand with and without long vowels ("بنادول" / "بانادول"), with ى or ي,
// with ه or ة, and transliterate the same Latin consonant several ways; a
// skeleton that survives all of that is worth more than one that is faithful.
//
// It is scored as a third, discounted channel — never above a shared whole word
// — because two different products can skeletonise alike and a shared word
// cannot happen by accident.

// arabicSkeleton maps each Arabic letter onto its Latin consonant sound.
//
// Vowel carriers (ا و ي) map to nothing: Arabic omits short vowels and Latin
// spells them, so the only way the two can agree is if neither is compared.
// That is also what makes "بنادول" and "بانادول" the same skeleton, which is
// the commonest spelling variation in real files.
var arabicSkeleton = map[rune]string{
	'ا': "", 'أ': "", 'إ': "", 'آ': "", 'ء': "", 'ؤ': "", 'ئ': "", 'ى': "", 'ي': "", 'و': "",
	'ب': "b", 'پ': "b",
	'ت': "t", 'ط': "t", 'ث': "s",
	'ج': "g", 'چ': "g",
	'ح': "h", 'ه': "h", 'ة': "h", 'خ': "k",
	'د': "d", 'ض': "d", 'ذ': "z",
	'ر': "r",
	'ز': "z", 'ظ': "z",
	'س': "s", 'ص': "s", 'ش': "s",
	'ع': "", 'غ': "g",
	'ف': "f", 'ڤ': "f",
	'ق': "k", 'ك': "k",
	'ل': "l", 'م': "m", 'ن': "n",
}

// latinSkeleton maps a Latin letter onto the same alphabet.
//
// The vowels drop for the same reason they do above. c, q and x are folded onto
// the sounds Arabic actually has, and the digraph rules in skeletonOf handle
// the pairs — ph, sh, ch, th — that a letter-by-letter fold would get wrong.
var latinSkeleton = map[rune]string{
	'a': "", 'e': "", 'i': "", 'o': "", 'u': "", 'y': "", 'h': "h", 'w': "",
	'b': "b", 'c': "k", 'd': "d", 'f': "f", 'g': "g", 'j': "g", 'k': "k",
	'l': "l", 'm': "m", 'n': "n", 'p': "b", 'q': "k", 'r': "r", 's': "s",
	't': "t", 'v': "f", 'x': "ks", 'z': "z",
}

// minSkeletonRunes is the shortest skeleton worth comparing.
//
// Two consonants match by coincidence far too often — "دار" and "دور" both
// reduce to "dr" — and a two-letter agreement is not evidence of anything.
const minSkeletonRunes = 3

// Skeleton reduces a name to the consonant sounds both scripts share.
//
// Exported because the recall index keys on it too: a query is retrieved by its
// skeleton as well as by its words, which is what puts a product back on the
// shortlist when the pharmacy wrote the brand in the other alphabet.
func Skeleton(s string) string {
	if s == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(s) / 2)

	runes := []rune(strings.ToLower(s))
	last := byte(0)
	for i := 0; i < len(runes); i++ {
		r := runes[i]

		// Latin digraphs first: read letter by letter, "ph" becomes "bh" and
		// "sh" becomes "sh", neither of which is the sound Arabic writes.
		if i+1 < len(runes) {
			if fold, ok := latinDigraph(r, runes[i+1]); ok {
				last = writeSkeleton(&b, fold, last)
				i++
				continue
			}
		}

		fold, known := arabicSkeleton[r]
		if !known {
			fold, known = latinSkeleton[r]
		}
		if !known {
			// Digits and punctuation carry no sound. Figures are compared
			// separately as a number signature, and dropping them here is what
			// lets "بروفين600" and "brufen 600" reduce alike.
			continue
		}
		last = writeSkeleton(&b, fold, last)
	}
	return b.String()
}

// latinDigraph folds the two-letter spellings of single Arabic sounds.
func latinDigraph(a, b rune) (string, bool) {
	switch {
	case a == 'p' && b == 'h':
		return "f", true
	case a == 's' && b == 'h':
		return "s", true
	case a == 'c' && b == 'h':
		return "s", true
	case a == 't' && b == 'h':
		return "s", true
	case a == 'g' && b == 'h':
		return "g", true
	case a == 'k' && b == 'h':
		return "k", true
	case a == 'c' && b == 'k':
		return "k", true
	}
	return "", false
}

// writeSkeleton appends a fold, collapsing a repeat of the previous sound.
//
// Doubling is spelling, not sound: "أوجمنتين" and "augmentin" agree, and so do
// "ammox" and "amox", only once the repeat is gone.
func writeSkeleton(b *strings.Builder, fold string, last byte) byte {
	if fold == "" {
		return last
	}
	for i := 0; i < len(fold); i++ {
		c := fold[i]
		if c == last {
			continue
		}
		b.WriteByte(c)
		last = c
	}
	return last
}

// skeletonOf reduces a set of already-tokenised words to one skeleton.
//
// Joined without separators on purpose: a supplier who writes "صن سكرين" and a
// catalogue that writes "sunscreen" must agree, and they only can if word
// boundaries are not part of the comparison. That is the same reason the
// spaceless name key exists.
func skeletonOf(tokens []string) string {
	if len(tokens) == 0 {
		return ""
	}
	sk := Skeleton(strings.Join(tokens, ""))
	if len([]rune(sk)) < minSkeletonRunes {
		return ""
	}
	return sk
}

// skeletonSimilarity compares two skeletons on their character trigrams.
//
// Trigrams rather than equality, because a skeleton is long enough that one
// extra consonant — a supplier writing the manufacturer into the name — would
// otherwise lose the whole match.
func skeletonSimilarity(a, b string) float64 {
	if a == "" || b == "" {
		return 0
	}
	if a == b {
		return 1
	}
	return jaccardSorted(sortedStringTrigrams(a), sortedStringTrigrams(b))
}

// sortedStringTrigrams builds the sorted, de-duplicated trigram set of one
// string, in the shape jaccardSorted expects.
func sortedStringTrigrams(s string) []string {
	// A skeleton is one word by construction, so the existing helper — which
	// joins tokens and trigrams the result — does exactly the right thing.
	return sortedTrigrams([]string{s})
}
