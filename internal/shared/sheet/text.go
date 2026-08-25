// Package sheet decodes real-world spreadsheets and reduces their cells to
// comparable values.
//
// It exists because every importer in this codebase was solving the same three
// problems badly and separately: what format is this file really, what does
// this cell say once the invisible damage is removed, and is this text the same
// text as that text once an Egyptian data-entry clerk has typed both.
//
// Nothing here knows about products, prices or vendors. That belongs to the
// module that calls it.
package sheet

import (
	"strings"
	"unicode"
)

// arabicLetterFolds maps the letter variants Egyptian data entry treats as
// interchangeable onto one canonical form.
//
// This is the same folding catalog.NormalizeKey applies and the same the
// database applies in platform.normalize_arabic. All three must agree: a name
// matched in the import engine has to match in SQL too, or a row the preview
// showed as an update is written as an insert.
var arabicLetterFolds = map[rune]rune{
	'أ': 'ا', // أ -> ا
	'إ': 'ا', // إ -> ا
	'آ': 'ا', // آ -> ا
	'ٱ': 'ا', // ٱ -> ا
	'ى': 'ي', // ى -> ي
	'ئ': 'ي', // ئ -> ي
	'ؤ': 'و', // ؤ -> و
	'ة': 'ه', // ة -> ه
	'ڤ': 'ف', // ڤ -> ف
	'چ': 'ج', // چ -> ج
	'گ': 'ج', // گ -> ج
	'پ': 'ب', // پ -> ب
}

// isMark reports whether r is a diacritic, a tatweel, or an invisible control
// that carries no meaning for matching.
func isMark(r rune) bool {
	switch {
	case r >= 'ً' && r <= 'ْ': // harakat
		return true
	case r == 'ـ': // tatweel ـ
		return true
	case r == 'ٰ': // superscript alef
		return true
	case r >= 'ۖ' && r <= 'ۭ': // quranic marks
		return true
	case r == '​', r == '‌', r == '‍', r == '‎', r == '‏':
		return true // zero-width and bidi controls
	case r == '\uFEFF':
		return true // a BOM that survived as a character
	}
	return false
}

// foldDigit converts an Arabic-Indic (٠-٩) or Extended Arabic-Indic (۰-۹) digit
// to ASCII. Excel writes these verbatim when the sheet was typed on an Arabic
// keyboard, and every numeric parse in Go rejects them.
func foldDigit(r rune) (rune, bool) {
	switch {
	case r >= '٠' && r <= '٩':
		return '0' + (r - '٠'), true
	case r >= '۰' && r <= '۹':
		return '0' + (r - '۰'), true
	}
	return r, false
}

// NormalizeDigits converts Arabic-Indic digits to ASCII and leaves everything
// else alone. Used on values that must stay human-readable, such as a
// concentration printed on the box.
func NormalizeDigits(s string) string {
	if !strings.ContainsFunc(s, func(r rune) bool { _, ok := foldDigit(r); return ok }) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if d, ok := foldDigit(r); ok {
			b.WriteRune(d)
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// CleanCell trims a raw cell into a single-line display value.
//
// It preserves the letters — this is what gets stored and shown to a pharmacist
// — and removes only the invisible damage: non-breaking spaces, the newlines a
// wrapped cell carries, bidi controls, and the whitespace runs left by column
// padding.
func CleanCell(s string) string {
	if s == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(s))
	lastSpace := false
	for _, r := range s {
		switch {
		case r == ' ', r == ' ', r == '\t', r == '\r', r == '\n', r == '\v', r == '\f':
			r = ' '
		case r == '​', r == '‌', r == '‍', r == '‎', r == '‏', r == '\uFEFF':
			continue // invisible: drop rather than turn into a space
		case unicode.IsSpace(r):
			r = ' '
		}
		if r == ' ' {
			if lastSpace {
				continue
			}
			lastSpace = true
		} else {
			lastSpace = false
		}
		b.WriteRune(r)
	}
	return strings.TrimSpace(b.String())
}

// NormalizeKey reduces a header label to its comparable core: folded Arabic
// letters, ASCII digits, lowercase Latin, nothing else. Spaces and punctuation
// are removed, so "Item No.", "item_no" and "ITEM  NO" collapse to one key.
//
// Dropping spaces is what makes a synonym table tractable: a supplier writing
// "سعر الجمهور", "سعرالجمهور" or "سعر  الجمهور" produces one key, not three.
func NormalizeKey(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if isMark(r) {
			continue
		}
		if d, ok := foldDigit(r); ok {
			b.WriteRune(d)
			continue
		}
		if f, ok := arabicLetterFolds[r]; ok {
			r = f
		}
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r + 32)
		case r >= 'ء' && r <= 'ي':
			b.WriteRune(r)
		}
	}
	return b.String()
}

// NormalizeName folds a product name for comparison: the same folding as
// NormalizeKey, but single spaces survive, so two genuinely different products
// whose names differ only at a word boundary stay distinct.
func NormalizeName(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	lastSpace := true // leading spaces are dropped
	for _, r := range s {
		if isMark(r) {
			continue
		}
		if d, ok := foldDigit(r); ok {
			b.WriteRune(d)
			lastSpace = false
			continue
		}
		if f, ok := arabicLetterFolds[r]; ok {
			r = f
		}
		switch {
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r + 32)
			lastSpace = false
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r >= 'ء' && r <= 'ي':
			b.WriteRune(r)
			lastSpace = false
		default:
			if !lastSpace {
				b.WriteRune(' ')
				lastSpace = true
			}
		}
	}
	return strings.TrimSpace(b.String())
}

// Script describes which writing systems a string is made of. Column profiling
// uses it to tell a name column from a code column without reading the header.
type Script struct {
	Arabic int
	Latin  int
	Digit  int
	Other  int
	Total  int
}

// Profile counts the runes of s by script.
func Profile(s string) Script {
	var sc Script
	for _, r := range s {
		switch {
		case r >= '؀' && r <= 'ۿ', r >= 'ﭐ' && r <= 'ﻼ':
			sc.Arabic++
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z':
			sc.Latin++
		case unicode.IsDigit(r):
			sc.Digit++
		case unicode.IsSpace(r):
			continue // spaces say nothing about the script
		default:
			sc.Other++
		}
		sc.Total++
	}
	return sc
}

// Letters is how many of the counted runes were alphabetic in either script.
func (s Script) Letters() int { return s.Arabic + s.Latin }

// Words splits a normalised string into its tokens.
func Words(s string) []string { return strings.Fields(s) }

// Trigrams returns the overlapping three-rune windows of s, which is how two
// misspelled names are compared without a full edit distance over a catalogue
// of thirty thousand.
func Trigrams(s string) []string {
	r := []rune(s)
	if len(r) < 3 {
		if len(r) > 0 {
			return []string{string(r)}
		}
		return nil
	}
	out := make([]string, 0, len(r)-2)
	for i := 0; i <= len(r)-3; i++ {
		out = append(out, string(r[i:i+3]))
	}
	return out
}

// JaccardSets is the overlap of two token or trigram sets, from 0 to 1.
//
// The denominator is the union rather than the larger set: a two-word query
// fully contained in a ten-word catalogue name should not score 1.0, because
// "بانادول" is contained in a dozen different Panadol products.
func JaccardSets(a, b []string) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	setB := make(map[string]struct{}, len(b))
	for _, t := range b {
		setB[t] = struct{}{}
	}
	seen := make(map[string]struct{}, len(a))
	inter := 0
	for _, t := range a {
		if _, dup := seen[t]; dup {
			continue
		}
		seen[t] = struct{}{}
		if _, ok := setB[t]; ok {
			inter++
		}
	}
	union := len(setB) + len(seen) - inter
	if union <= 0 {
		return 0
	}
	return float64(inter) / float64(union)
}

// ContainmentSets is the share of a's distinct members that appear in b. Unlike
// Jaccard it does not punish b for carrying extra words, which is what a
// supplier's terse "ابل لايت 30ق" needs when matched against a catalogue name
// that spells the pack out in full.
func ContainmentSets(a, b []string) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	setB := make(map[string]struct{}, len(b))
	for _, t := range b {
		setB[t] = struct{}{}
	}
	seen := make(map[string]struct{}, len(a))
	inter := 0
	for _, t := range a {
		if _, dup := seen[t]; dup {
			continue
		}
		seen[t] = struct{}{}
		if _, ok := setB[t]; ok {
			inter++
		}
	}
	if len(seen) == 0 {
		return 0
	}
	return float64(inter) / float64(len(seen))
}
