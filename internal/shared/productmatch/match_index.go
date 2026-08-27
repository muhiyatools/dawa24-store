package productmatch

import (
	"math"
	"sort"
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
	// tri holds the trigram and scientific-name posting lists that only Recall
	// needs, built lazily. Most runs never reach the AI stage, and an index
	// nothing reads should not be paid for on every import.
	tri recallIndex
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

// Name returns the catalogue's own label for a product, Arabic first.
//
// It exists so a caller that has resolved a match can report *which* product it
// resolved to without a second query: the index already holds every name it
// scored against. A results table or an import ledger that prints only the id
// has told the reader nothing they can check.
func (idx *Index) Name(id int64) string {
	if idx == nil {
		return ""
	}
	p, ok := idx.byID[id]
	if !ok {
		return ""
	}
	if p.NameAR != "" {
		return p.NameAR
	}
	return p.NameEN
}

// Lookup returns the indexed projection of one catalogue product.
func (idx *Index) Lookup(id int64) (*MasterProduct, bool) {
	if idx == nil {
		return nil, false
	}
	p, ok := idx.byID[id]
	return p, ok
}

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

// maxIDF is the weight of a token no catalogue product carries — the ceiling
// the idf curve approaches as a word gets rarer.
func (idx *Index) maxIDF() float64 {
	return math.Log(float64(idx.total + 1))
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
