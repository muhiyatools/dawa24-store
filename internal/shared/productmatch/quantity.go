package productmatch

// The figures and the letters that separate one product from its neighbours.
//
// A brand family is written as one name plus one distinguishing mark, and the
// mark is almost never a word. It is a figure — 20 tablets against 200, a 25 g
// tube against 50 — or it is a single letter: بتنوفيت ان and بتنوفيت سي are
// Betnovate N and Betnovate C, two different medicines, and the whole
// difference between them is one letter that coreTokens discards for being too
// short to be a word.
//
// Both were invisible to the scorer. Figures reached it as an unordered bag in
// which a pack count and a bottle size and half a combination dose were the
// same kind of thing, compared by asking whether the shorter bag was contained
// in the longer one — which is true of "16" inside "16 و 12.5", so اتاكاند 16
// and اتاكاند دي 16/12.5 agreed on their numbers and differed on nothing else
// the engine could see. Letters reached it not at all.
//
// This file reads a name once and says what each figure was counting.

import (
	"sort"
	"strconv"
	"strings"

	"github.com/muhiya/dawa24-store/internal/shared/sheet"
)

// countUnit is a number and the thing it counts: 30 tablets, 5 suppositories,
// 10 sachets.
//
// The class is the canonical dosage-form key rather than the word, so "30 قرص",
// "30 اقراص" and "30 tabs" are one measurement written three ways.
type countUnit struct {
	class string
	value float64
}

// quantities is what one name's figures turned out to be.
type quantities struct {
	// counts are the figures that named what they counted.
	counts []countUnit
	// residual are the figures that named nothing — "20*10", "1+1", a wipe
	// count whose noun this engine does not know. They are compared only
	// against another name's residual figures, because that is the only
	// comparison they support.
	residual []float64
}

// readQuantities walks a name once and classifies every figure in it.
//
// A figure is a count when the word beside it names something countable, and a
// residual otherwise. It is never a count when the word beside it is a dose
// unit: "500 مجم" is a strength and is compared as one, by parseStrength, on a
// scale where 1 g and 1000 mg are the same number.
//
// Nothing is guessed. The old signature bagged every figure in the name
// together and compared the bags by containment, which made a 30-tablet pack
// and half a combination dose interchangeable evidence.
func readQuantities(text string) quantities {
	words := strings.Fields(sheet.NormalizeDigits(sheet.NormalizeName(text)))
	if len(words) == 0 {
		return quantities{}
	}

	var q quantities
	addCount := func(value float64, class string) {
		for i := range q.counts {
			if q.counts[i].class == class && q.counts[i].value == value {
				return
			}
		}
		q.counts = append(q.counts, countUnit{class: class, value: value})
	}
	addResidual := func(value float64) {
		for _, v := range q.residual {
			if v == value {
				return
			}
		}
		q.residual = append(q.residual, value)
	}

	place := func(value float64, next string) {
		if value <= 0 || value > 100000 {
			return
		}
		switch class := countClassOf(next); class {
		case classDose:
			// A strength. Compared elsewhere, on its own scale.
		case "":
			addResidual(value)
		default:
			addCount(value, class)
		}
	}

	for i, w := range words {
		// "30قرص" — the figure and the word glued, which is how most Egyptian
		// price lists are typed.
		if head, tail, ok := splitLeadingNumber(w); ok {
			place(head, tail)
			continue
		}
		if !isNumericLiteral(w) {
			continue
		}
		v, err := strconv.ParseFloat(strings.Trim(w, "."), 64)
		if err != nil {
			continue
		}
		place(v, lookahead(words, i))
	}

	sort.SliceStable(q.counts, func(i, j int) bool {
		if q.counts[i].class != q.counts[j].class {
			return q.counts[i].class < q.counts[j].class
		}
		return q.counts[i].value < q.counts[j].value
	})
	sort.Float64s(q.residual)
	return q
}

// lookahead finds the word that says what a bare figure was counting.
//
// Usually the very next one. Where the next token is itself a figure, the two
// are halves of a ratio — "10/20mg", "20/10 مجم", "45 - 60 ml" — and both
// halves are governed by the unit at the end of it. Reading only the immediate
// neighbour left the leading half unexplained, so a supplier writing 10/20 and
// a catalogue writing 20/10 were recorded as carrying different stray figures
// and contradicted each other over a combination they agree about.
//
// Bounded at four, which is longer than any real ratio and short enough that a
// name listing figures for unrelated reasons cannot chain them together.
func lookahead(words []string, i int) string {
	for n := 0; n < 4 && i+1 < len(words); n++ {
		i++
		next := words[i]
		if !hasDigit(next) {
			return next
		}
		if _, tail, ok := splitLeadingNumber(next); ok && tail != "" {
			return tail
		}
	}
	return ""
}

// classDose marks a figure the strength comparison already owns.
const classDose = "\x00dose"

// countClassOf resolves the word after a figure onto the thing it counts.
//
// It reads the same form vocabulary the rest of the engine uses, so a count of
// "اكياس" and a count of "قرص" are recognised as counting different things —
// which is what makes "دوليبران 1جم 8 اكياس" and "دوليبران 1جم 8 اقراص"
// distinguishable at all. A form word with no canonical group keeps its own
// spelling as its class: this engine does not group wipes, and two products
// differing only in whether the packet holds 25 of them or 50 are still two
// products.
func countClassOf(word string) string {
	if word == "" || hasDigit(word) {
		return ""
	}
	if isDoseUnitWord(word) {
		return classDose
	}
	if key := formKeyOf(word); key != "" {
		return key
	}
	if _, ok := formWords[word]; ok {
		return "form:" + word
	}
	if _, ok := unitWords[word]; ok {
		// A sales unit rather than a pharmaceutical form: "12 علبة". Grouped
		// under one class so counts of packaging compare with each other and
		// not with counts of tablets.
		return "pack"
	}
	return ""
}

// isDoseUnitWord reports whether a unit word states a strength rather than a
// quantity of packaging.
func isDoseUnitWord(word string) bool {
	_, ok := doseUnits[word]
	return ok
}

// splitLeadingNumber separates "30قرص" into 30 and "قرص".
func splitLeadingNumber(w string) (float64, string, bool) {
	cut := 0
	for cut < len(w) && (w[cut] >= '0' && w[cut] <= '9' || w[cut] == '.') {
		cut++
	}
	if cut == 0 || cut == len(w) {
		return 0, "", false
	}
	v, err := strconv.ParseFloat(strings.Trim(w[:cut], "."), 64)
	if err != nil || v <= 0 {
		return 0, "", false
	}
	return v, w[cut:], true
}

// identityMarks is the set of single letters a name carries as identity.
//
// Egyptian pharmacy names distinguish whole product lines by one letter:
// بتنوفيت ان (Betnovate N, with neomycin) against بتنوفيت سي (Betnovate C, with
// clioquinol) against plain بتنوفيت; بلانكا اس against بلانكا; مازيمال سي ار,
// which is the modified-release line. Every one of those is a different
// medicine at a different price, and every one of them was invisible: coreTokens
// drops a word shorter than two runes outright, and rarity weighting reduces
// what is left to noise.
//
// Both alphabets fold onto one key, because the same product is written "N" by
// one supplier and "ان" by the next.
//
// A mark counts only where the name also carries a real word, and only after
// one. A name that is nothing but short tokens has no brand for them to be
// modifying, and a leading article is not an identity.
func identityMarks(text string) map[string]struct{} {
	words := strings.Fields(sheet.NormalizeName(text))
	if len(words) < 2 {
		return nil
	}
	var out map[string]struct{}
	seenWord := false
	for i := 0; i < len(words); i++ {
		w := words[i]
		if hasDigit(w) {
			continue
		}
		if len([]rune(w)) >= minLinkRunes {
			if !noiseWords[w] && !isMeasureWord(w) {
				seenWord = true
			}
			continue
		}
		if key, ok := letterNames[w]; !ok || key == "" {
			continue
		}
		// A RUN of letters is an abbreviation, not an identity.
		//
		// "f.c.tabs" is film-coated tablets, "i.v." is intravenous, "u.s.p" is
		// the pharmacopoeia, and the Arabic "اس بي اف" is SPF. Normalisation
		// turns every one of those into consecutive single-letter tokens, and
		// reading them as line extensions raised a false conflict on a fifth of
		// every labelled row measured: the English file writes "abilia 15mg 30
		// f.c.tabs" where the catalogue writes "ابيليا 15مجم 30 قرص", and that
		// is one product which looked like Abilia F C against plain Abilia.
		//
		// The exception is the release codes, which are genuinely written as
		// two letters and genuinely name another product: "200mg c.r." is not
		// "200mg".
		run := letterRun(words, i)
		if len(run) > 1 && !releaseCodes[strings.Join(run, "")] {
			i += len(run) - 1
			continue
		}
		if !seenWord {
			continue // nothing yet for this letter to be an extension of
		}
		if out == nil {
			out = make(map[string]struct{}, 2)
		}
		for _, r := range run {
			if k := letterNames[r]; k != "" {
				out[k] = struct{}{}
			}
		}
		i += len(run) - 1
	}
	return out
}

// letterRun collects the maximal run of consecutive single-letter tokens
// starting at i.
func letterRun(words []string, i int) []string {
	out := make([]string, 0, 3)
	for ; i < len(words); i++ {
		w := words[i]
		if hasDigit(w) || len([]rune(w)) >= minLinkRunes {
			break
		}
		if _, named := letterNames[w]; !named {
			if _, filler := abbreviationLetters[w]; !filler {
				break
			}
		}
		out = append(out, w)
	}
	return out
}

// abbreviationLetters are short tokens that belong to an abbreviation run
// without being identity letters themselves.
//
// They exist so "i.v.", "u.s.p" and "e.c." are recognised as runs and suppress
// the letters beside them. Without them "u s p" would be read as a run of one
// ("s"), and Albumin USP would carry an identity letter it does not have.
var abbreviationLetters = map[string]struct{}{
	"a": {}, "e": {}, "i": {}, "j": {}, "l": {}, "o": {}, "u": {}, "v": {},
	"w": {}, "y": {}, "اي": {}, "ايه": {}, "او": {}, "يو": {}, "في": {},
	"ال": {}, "ام": {},
}

// releaseCodes are the two-letter runs that DO name a different product: the
// modified-release lines, written "c.r." or "سي ار" as often as "cr".
var releaseCodes = map[string]bool{
	"sr": true, "cr": true, "xr": true, "mr": true, "er": true,
	"xl": true, "la": true, "dr": true, "od": true,
}

// letterNames folds the ways one letter is written onto a single key.
//
// Only the letters that actually name a product line are listed, and the
// ambiguous ones are deliberately absent: "ال" is the Arabic article and "في" a
// preposition, and reading either as an identity mark would find one in half
// the catalogue. A letter left out of this map is simply not compared, which is
// the treatment every letter received before this file existed — the failure
// mode is a missed distinction, never an invented one.
var letterNames = map[string]string{
	"b": "b", "بي": "b",
	"c": "c", "سي": "c",
	"d": "d", "دي": "d",
	"f": "f", "اف": "f",
	"g": "g", "جي": "g",
	"h": "h", "اتش": "h", "اش": "h",
	"k": "k", "كيه": "k", "كي": "k",
	"m": "m", "ام": "m",
	"n": "n", "ان": "n", "ن": "n",
	"p": "p",
	"q": "q", "كيو": "q",
	"r": "r", "ار": "r",
	"s": "s", "اس": "s", "س": "s",
	"t": "t", "تي": "t",
	"x": "x", "اكس": "x",
	"z": "z", "زد": "z",
}

// topicalSubForm separates the semi-solid topicals the form key groups together.
//
// formKeyOf maps كريم, مرهم and جل onto one "topical" key, and that grouping is
// right for the veto: a supplier writes كريم loosely and refusing a match over
// it costs more than it saves. It is wrong for CHOOSING between two catalogue
// entries of the same brand, which is exactly what a family like كليروفات كريم
// and كليروفات مرهم requires — those are two products, stocked and priced
// separately, and the row says which one it wants.
//
// So the distinction lives here rather than in vetoableForm: it never refuses a
// match on its own, and it decides between candidates that are otherwise equal.
func topicalSubForm(text string) string {
	for _, w := range strings.Fields(sheet.NormalizeName(text)) {
		for _, run := range letterRuns(w) {
			if sub, ok := topicalWords[run]; ok {
				return sub
			}
		}
	}
	return ""
}

var topicalWords = map[string]string{
	"كريم": "cream", "cream": "cream", "crm": "cream",
	"مرهم": "ointment", "ointment": "ointment", "oint": "ointment",
	"جل": "gel", "جيل": "gel", "gel": "gel",
	"لوسيون": "lotion", "لوشن": "lotion", "lotion": "lotion",
	"معجون": "paste", "paste": "paste",
	"رغوة": "foam", "فوم": "foam", "foam": "foam",
	"بلسم": "balm", "balm": "balm",
	"زيت": "oil", "oil": "oil",
	"سيروم": "serum", "serum": "serum",
	"بودرة": "powder", "بودره": "powder", "باودر": "powder", "powder": "powder",
}
