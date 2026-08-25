package productmatch

import (
	"math"
	"strings"
	"unicode"

	"github.com/muhiya/dawa24-store/internal/shared/arabic"
)

// Text primitives shared by every importer.
//
// These moved here from internal/modules/compare, which had grown its own copy
// of normalisation and string similarity alongside the ones in this package.
// Two implementations of "are these the same product name" is one too many:
// they drift, and the drift is invisible until two features disagree about the
// same spreadsheet. The algorithms below are compare's, carried across
// unchanged so that its behaviour is identical before and after the move.
//
// They are deliberately simpler than Index's scorer. Index compares a row
// against thirty thousand catalogue products and can afford precomputation;
// these answer a one-off "how alike are these two strings" and are used for
// header matching and small-set disambiguation.

// NormalizeText reduces a product name to comparable form: Arabic normalisation,
// lower case, punctuation stripped, whitespace collapsed.
func NormalizeText(s string) string {
	s = arabic.Normalize(s)
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsSpace(r) {
			b.WriteRune(r)
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// FirstMeaningfulWord returns the first token of at least three runes, which in
// a pharmaceutical name is nearly always the brand or ingredient. Short leading
// tokens are packaging noise ("2 مل", "ك").
func FirstMeaningfulWord(s string) string {
	normalized := NormalizeText(s)
	words := strings.Fields(normalized)
	for _, w := range words {
		if len([]rune(w)) >= 3 {
			return w
		}
	}
	if len(words) > 0 {
		return words[0]
	}
	return ""
}

// TextSimilarity scores two already-normalised strings between 0 and 1.
//
// It blends three signals because no single one survives Egyptian pharmacy
// spelling: containment (an abbreviation of the catalogue name), character
// bigrams (a typo), and token overlap (words reordered). The floors of 0.60 and
// 0.50 encode that a shared leading token is strong evidence on its own.
func TextSimilarity(s1, s2 string) float64 {
	if s1 == "" || s2 == "" {
		return 0.0
	}
	if s1 == s2 {
		return 1.0
	}

	if strings.Contains(s1, s2) || strings.Contains(s2, s1) {
		lenMin := min(len(s1), len(s2))
		lenMax := max(len(s1), len(s2))
		ratio := float64(lenMin) / float64(lenMax)
		return 0.70 + (0.30 * ratio)
	}

	bigrams1 := bigramsOf(s1)
	bigrams2 := bigramsOf(s2)

	bigramScore := 0.0
	if len(bigrams1) > 0 && len(bigrams2) > 0 {
		intersection := 0
		for bg := range bigrams1 {
			if bigrams2[bg] {
				intersection++
			}
		}
		bigramScore = (2.0 * float64(intersection)) / float64(len(bigrams1)+len(bigrams2))
	}

	w1 := strings.Fields(s1)
	w2 := strings.Fields(s2)
	if len(w1) > 0 && len(w2) > 0 {
		commonWords := 0
		w2Map := make(map[string]bool, len(w2))
		for _, w := range w2 {
			w2Map[w] = true
		}
		for _, w := range w1 {
			if len([]rune(w)) >= 3 && w2Map[w] {
				commonWords++
			}
		}
		if commonWords > 0 {
			overlapScore := float64(commonWords) / float64(max(len(w1), len(w2)))
			if w1[0] == w2[0] {
				return math.Max(0.60+(0.35*overlapScore), bigramScore)
			}
			return math.Max(0.50+(0.40*overlapScore), bigramScore)
		}
	}

	return bigramScore
}

// bigramsOf returns the set of adjacent rune pairs in s.
func bigramsOf(s string) map[string]bool {
	runes := []rune(s)
	out := make(map[string]bool)
	for i := 0; i < len(runes)-1; i++ {
		out[string(runes[i:i+2])] = true
	}
	return out
}
