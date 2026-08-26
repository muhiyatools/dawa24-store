package productmatch

import (
	"sort"
	"strconv"
	"strings"

	"github.com/muhiya/dawa24-store/internal/shared/sheet"
)

// Reducing a product name to the words that identify it.
//
// Everything here answers one question: of the words a supplier wrote, which
// ones say *which product this is*? The pharmaceutical furniture does not — the
// form, the unit, the pack count, the strength, the marketing noise a
// distributor appends. Nor do the figures glued to any of them, which Arabic
// price lists write without a space and which therefore arrived looking like
// ordinary vocabulary.
//
// What is dropped here is not lost. The strength is parsed as a number and
// compared as one, the form is collapsed to a key and compared as one, and
// every remaining figure is kept in the row's number signature. Only their
// pretence to be identifying words is discarded.

// coreTokens reduces a product name to the words that identify it.
//
// The pharmaceutical furniture is dropped — the form, the unit, the pack count,
// the marketing noise a distributor appends ("سعر جديد", "س ج", "**") — leaving
// the brand and the molecule. Without this, "بانادول اكسترا 24 قرص" and
// "بانادول اكسترا 24 قرص سعر جديد" score as different products, and every
// product ending in "أقراص" scores as similar to every other.
func coreTokens(name string) []string {
	if name == "" {
		return nil
	}
	words := strings.Fields(sheet.NormalizeName(name))
	out := make([]string, 0, len(words))
	for _, w := range words {
		if noiseWords[w] {
			continue
		}
		if _, ok := formWords[w]; ok {
			continue
		}
		if _, ok := unitWords[w]; ok {
			continue
		}
		// A bare number is a pack count or a dose already captured separately.
		if _, err := strconv.ParseFloat(w, 64); err == nil {
			continue
		}
		// So is a number glued to its unit. Arabic price lists write "20مجم"
		// and "20قرص" without a space, and normalisation does not split them,
		// so they arrived here looking like ordinary words. They are not: they
		// are the strength and the pack count, both already compared as
		// structured fields — and being common they carried more weight than
		// the brand beside them. That is how "اربييركس 20مجم 20قرص" scored 98%
		// against "ابيكوبريد 20مجم 20قرص": two of the three words agreed, and
		// the one that did not was the only one that identified the medicine.
		if hasDigit(w) {
			out = appendLetterRuns(out, w)
			continue
		}
		if len([]rune(w)) < 2 {
			continue
		}
		out = append(out, w)
	}
	return out
}

func hasDigit(w string) bool {
	for _, r := range w {
		if r >= '0' && r <= '9' {
			return true
		}
	}
	return false
}

// appendLetterRuns splits a token that mixes digits and letters, keeping the
// letters and discarding the figures.
//
// Arabic price lists glue them in every direction: "20مجم", "ليبانتيل300",
// "اقراص3شريط", "بروفين600مجم". Left whole, each of those is a token no other
// spelling of the same product shares, so two entries for one medicine score as
// strangers — and, worse, the common ones like "20مجم" are shared by thousands
// of products and outvoted the brand beside them.
//
// The figures are not lost: the strength is parsed as a number and compared as
// one, and every other figure is in the row's number signature. What is dropped
// here is only their pretence to be words.
//
// A run of one letter is dropped with the rest, which costs the "b" of "b12" —
// acceptable, because the 12 survives in the number signature and the products
// that case distinguishes are separated by it anyway.
func appendLetterRuns(out []string, w string) []string {
	runes := []rune(w)
	start := -1
	flush := func(end int) {
		if start < 0 {
			return
		}
		run := string(runes[start:end])
		start = -1
		if len([]rune(run)) < 2 || noiseWords[run] || isMeasureWord(run) {
			return
		}
		out = append(out, run)
	}
	for i, r := range runes {
		if r >= '0' && r <= '9' || r == '.' || r == ',' {
			flush(i)
			continue
		}
		if start < 0 {
			start = i
		}
	}
	flush(len(runes))
	return out
}

// isMeasureWord reports whether a word is a unit or a pharmaceutical form —
// the two things a number in a product name is ever counting.
func isMeasureWord(w string) bool {
	if _, ok := unitWords[w]; ok {
		return true
	}
	_, ok := formWords[w]
	return ok
}

// noiseWords are the words Egyptian distributors append to a product name that
// say nothing about which product it is.
var noiseWords = map[string]bool{
	"سعر": true, "جديد": true, "س": true, "ج": true, "سج": true, "ض": true,
	"عرض": true, "خصم": true, "مجانا": true, "توفير": true, "بديل": true,
	"مستورد": true, "باكو": true, "جدبد": true, "الجديد": true,
	"new": true, "price": true, "offer": true, "free": true, "imported": true,
}

// formKeyOf collapses the many ways a form is written onto one token, so
// "أقراص", "قرص", "tab" and "tablets" compare equal.
func formKeyOf(text string) string {
	clean := sheet.NormalizeName(text)
	for _, rule := range formGroups {
		for _, w := range rule.words {
			if strings.Contains(clean, w) {
				return rule.key
			}
		}
	}
	return ""
}

var formGroups = []struct {
	key   string
	words []string
}{
	{"tablet", []string{"اقراص", "قرص", "شريط", "tab", "tablet"}},
	{"capsule", []string{"كبسول", "كبسوله", "cap", "capsule"}},
	{"liquid", []string{"شراب", "معلق", "محلول", "syr", "syrup", "susp", "solution"}},
	{"injectable", []string{"حقن", "امبول", "فيال", "امبوله", "amp", "vial", "inj"}},
	{"topical", []string{"كريم", "مرهم", "جل", "جيل", "لوسيون", "دهان", "cream", "oint", "gel", "lotion"}},
	{"drops", []string{"نقط", "قطره", "قطرة", "drop"}},
	{"suppository", []string{"لبوس", "تحاميل", "supp"}},
	{"spray", []string{"بخاخ", "اسبراي", "سبراي", "spray"}},
	{"sachet", []string{"فوار", "كيس", "اكياس", "ساشيه", "sachet", "eff"}},
	{"wash", []string{"غسول", "شامبو", "صابون", "wash", "shampoo", "soap"}},
}

// doseUnits normalises the strength units onto a common base: milligrams for
// mass, millilitres for volume, and their own scale for the rest.
var doseUnits = map[string]struct {
	base  string
	scale float64
}{
	"mg": {"mg", 1}, "ملجم": {"mg", 1}, "مجم": {"mg", 1}, "مليجرام": {"mg", 1},
	// The abbreviations real supplier files actually use. "مج" and "مغ" are
	// everyday milligrams; "محم" is the ج/ح keyboard slip for "مجم" and turns
	// up in live pharmacy order files often enough to be worth reading.
	"مج": {"mg", 1}, "مغ": {"mg", 1}, "محم": {"mg", 1}, "ملجرام": {"mg", 1},
	"mcg": {"mg", 0.001}, "مكجم": {"mg", 0.001},
	"g": {"mg", 1000}, "gm": {"mg", 1000}, "جم": {"mg", 1000}, "جرام": {"mg", 1000},
	"ml": {"ml", 1}, "مل": {"ml", 1}, "مللي": {"ml", 1}, "ملي": {"ml", 1},
	"l": {"ml", 1000}, "لتر": {"ml", 1000},
	"iu": {"iu", 1}, "وحده": {"iu", 1}, "وحدة": {"iu", 1},
	"%": {"%", 1}, "spf": {"spf", 1},
}

// parseStrength reads the first dose written in a text.
func parseStrength(text string) strength {
	m := strengthPattern.FindString(sheet.NormalizeDigits(text))
	if m == "" {
		return strength{}
	}
	m = strings.ToLower(strings.TrimSpace(m))

	// Split the number from the unit at the first non-numeric rune.
	cut := 0
	for i, r := range m {
		if (r >= '0' && r <= '9') || r == '.' || r == '/' {
			cut = i + 1
			continue
		}
		if r == ' ' && cut == i {
			cut = i + 1
			continue
		}
		break
	}
	numPart := strings.TrimSpace(m[:cut])
	unitPart := strings.TrimSpace(m[cut:])
	// "5/10mg" is a combination product; the first figure identifies it well
	// enough and the second would only add noise.
	if head, _, ok := strings.Cut(numPart, "/"); ok {
		numPart = head
	}
	value, err := strconv.ParseFloat(numPart, 64)
	if err != nil || value <= 0 {
		return strength{}
	}
	u, ok := doseUnits[unitPart]
	if !ok {
		return strength{}
	}
	return strength{value: value * u.scale, unit: u.base}
}

// sortedTrigrams is the deduplicated, ordered trigram set of a token list, so
// two sets can be intersected by a two-pointer walk with no allocation.
func sortedTrigrams(tokens []string) []string {
	if len(tokens) == 0 {
		return nil
	}
	tri := sheet.Trigrams(strings.Join(tokens, ""))
	if len(tri) == 0 {
		return nil
	}
	sort.Strings(tri)
	out := tri[:1]
	for _, t := range tri[1:] {
		if t != out[len(out)-1] {
			out = append(out, t)
		}
	}
	return out
}

// jaccardSorted is the overlap of two sorted, deduplicated string sets.
func jaccardSorted(a, b []string) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	i, j, inter := 0, 0, 0
	for i < len(a) && j < len(b) {
		switch {
		case a[i] == b[j]:
			inter++
			i++
			j++
		case a[i] < b[j]:
			i++
		default:
			j++
		}
	}
	union := len(a) + len(b) - inter
	if union == 0 {
		return 0
	}
	return float64(inter) / float64(union)
}

// numberSignature is the set of plain numbers a product name carries, sorted.
//
// It is what separates "ستار فيل مناديل ميسيلار 25 منديل" from the same product
// in a fifty-wipe pack. The identifying words are identical, the dose is absent
// from both, and without the count the two score the same and the engine can
// only report that it cannot choose.
func numberSignature(name string) []float64 {
	fields := strings.FieldsFunc(sheet.NormalizeDigits(name), func(r rune) bool {
		return !(r >= '0' && r <= '9') && r != '.'
	})
	var out []float64
	for _, f := range fields {
		v, err := strconv.ParseFloat(strings.Trim(f, "."), 64)
		if err != nil || v <= 0 || v > 100000 {
			continue
		}
		out = append(out, v)
	}
	sort.Float64s(out)
	// Deduplicate: "1+1_60ملى" repeating a figure says nothing extra.
	if len(out) > 1 {
		kept := out[:1]
		for _, v := range out[1:] {
			if v != kept[len(kept)-1] {
				kept = append(kept, v)
			}
		}
		out = kept
	}
	return out
}

// DebugCoreTokens exposes the identifying words of a name, for diagnostics.
func DebugCoreTokens(name string) []string { return coreTokens(name) }
