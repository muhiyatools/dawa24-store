package productmatch

import (
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/muhiya/dawa24-store/internal/shared/sheet"
)

// Product matching.
//
// A vendor's row has to be tied to a product in the shared catalogue, because
// that is what makes their offer comparable with anyone else's. The name they
// wrote is not the name in the catalogue: it is abbreviated, transliterated
// differently, carries the pack count and the word "سعر جديد", and was typed by
// a different person on a different keyboard.
//
// The index below is built once per import and answers, for each row, which
// catalogue products could plausibly be it. The scoring that follows decides
// which one, and — as importantly — when it should refuse to decide.

// MasterProduct is the catalogue side of a match, projected down to the fields
// matching actually reads. Holding thirty thousand of these costs a few
// megabytes; holding thirty thousand full products would not.
type MasterProduct struct {
	ID            int64
	NameAR        string
	NameEN        string
	SKU           string
	Barcode       string
	Scientific    string
	DosageForm    string
	Concentration string
	Unit          string
	Manufacturer  string
	PublicPrice   string

	// Derived once at index time. Everything the scorer needs is precomputed
	// here rather than per comparison: a nine-thousand-row file scores three
	// million pairs, and recomputing one product's trigrams inside that loop is
	// the difference between two seconds and forty.
	coreAR   []string
	coreEN   []string
	triAR    []string
	triEN    []string
	nums     []float64
	nameKey  string
	formKey  string
	strength strength
	packSize int
	makerKey string
	sciKey   string
}

// strength is a parsed dose: a number and the unit it was written in, converted
// to a common base so 1g and 1000mg compare equal.
type strength struct {
	value float64
	unit  string
}

func (s strength) known() bool { return s.value > 0 }

// Index is the in-memory catalogue, keyed several ways.
type Index struct {
	products  []*MasterProduct
	byID      map[int64]*MasterProduct
	byBarcode map[string][]*MasterProduct
	bySKU     map[string][]*MasterProduct
	byName    map[string][]*MasterProduct
	// tokens maps a significant word to the products whose name contains it.
	tokens map[string][]*MasterProduct
	// df is the document frequency of each token, which is what makes a rare
	// brand name count for more than the word "شراب".
	df    map[string]int
	total int
}

// NewIndex builds the matching index. Ownership of the slice passes to it.
func NewIndex(products []MasterProduct) *Index {
	idx := &Index{
		byID:      make(map[int64]*MasterProduct, len(products)),
		byBarcode: make(map[string][]*MasterProduct),
		bySKU:     make(map[string][]*MasterProduct),
		byName:    make(map[string][]*MasterProduct),
		tokens:    make(map[string][]*MasterProduct, len(products)),
		df:        make(map[string]int, len(products)),
	}

	for i := range products {
		p := &products[i]
		if p.ID <= 0 {
			continue
		}
		prepare(p)
		idx.products = append(idx.products, p)
		idx.byID[p.ID] = p
		idx.total++

		if code := sheet.DigitsOnly(p.Barcode); code != "" {
			idx.byBarcode[code] = append(idx.byBarcode[code], p)
		}
		if key := sheet.NormalizeKey(p.SKU); key != "" {
			idx.bySKU[key] = append(idx.bySKU[key], p)
		}
		if p.nameKey != "" {
			idx.byName[p.nameKey] = append(idx.byName[p.nameKey], p)
		}

		seen := map[string]bool{}
		for _, tok := range append(append([]string{}, p.coreAR...), p.coreEN...) {
			if seen[tok] {
				continue
			}
			seen[tok] = true
			idx.tokens[tok] = append(idx.tokens[tok], p)
			idx.df[tok]++
		}
	}
	return idx
}

// Size is how many products the index holds.
func (idx *Index) Size() int { return idx.total }

// prepare derives a product's matching keys.
func prepare(p *MasterProduct) {
	p.coreAR = coreTokens(p.NameAR)
	p.coreEN = coreTokens(p.NameEN)
	p.triAR = sortedTrigrams(p.coreAR)
	p.triEN = sortedTrigrams(p.coreEN)
	p.nums = numberSignature(p.NameAR + " " + p.NameEN)
	p.nameKey = strings.Join(p.coreAR, " ")
	if p.nameKey == "" {
		p.nameKey = strings.Join(p.coreEN, " ")
	}
	full := p.NameAR + " " + p.NameEN + " " + p.DosageForm
	p.formKey = formKeyOf(full)
	p.strength = parseStrength(p.NameAR + " " + p.NameEN + " " + p.Concentration)
	p.packSize = InferPackSize(p.NameAR + " " + p.NameEN)
	p.makerKey = sheet.NormalizeKey(p.Manufacturer)
	p.sciKey = sheet.NormalizeKey(p.Scientific)
}

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
		if len([]rune(w)) < 2 {
			continue
		}
		out = append(out, w)
	}
	return out
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

// idf weights a token by how rare it is in the catalogue.
func (idx *Index) idf(token string) float64 {
	n := idx.df[token]
	if n <= 0 {
		return 0
	}
	return math.Log(float64(idx.total+1) / float64(n+1))
}

// candidatePool gathers the products worth scoring for one query.
//
// Rare tokens are consulted first and the pool is capped, which is what keeps
// matching linear in the file rather than quadratic against the catalogue: a
// query for "بانادول اكسترا" reads two postings lists rather than thirty
// thousand products.
func (idx *Index) candidatePool(tokens []string, limit int) []*MasterProduct {
	if len(tokens) == 0 {
		return nil
	}
	ordered := make([]string, len(tokens))
	copy(ordered, tokens)
	sort.SliceStable(ordered, func(i, j int) bool { return idx.idf(ordered[i]) > idx.idf(ordered[j]) })

	// A token carried by a large share of the catalogue tells us nothing and
	// costs everything: adding its postings list drags two thousand products
	// into a comparison the rarest token has already answered.
	crowded := idx.total / 20
	if crowded < 64 {
		crowded = 64
	}

	pool := make(map[int64]*MasterProduct, limit)
	for _, tok := range ordered {
		postings := idx.tokens[tok]
		if len(postings) > crowded && len(pool) > 0 {
			continue
		}
		for _, p := range postings {
			pool[p.ID] = p
		}
		// Stop as soon as there is plenty to score. The remaining tokens are the
		// common ones, which would add thousands of products that the rarest
		// token has already told us are not it.
		if len(pool) >= limit {
			break
		}
	}
	out := make([]*MasterProduct, 0, len(pool))
	for _, p := range pool {
		out = append(out, p)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
