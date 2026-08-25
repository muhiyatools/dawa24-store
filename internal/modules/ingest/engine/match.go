package engine

import (
	"fmt"
	"sort"
	"strings"

	"github.com/muhiya/dawa24-store/internal/shared/sheet"
)

// Match scoring.
//
// Every signal contributes, and two of them can veto. A dose that disagrees and
// a form that disagrees are not weak evidence against a match — they are proof
// it is a different product, and a name similarity of 0.95 between "أوجمنتين
// 1جم" and "أوجمنتين 625مجم" is exactly the case where the name is the least
// reliable thing in the row.

// MatchLevel says how a row was tied to a catalogue product.
type MatchLevel string

const (
	// MatchBarcode is a GTIN hit: the same physical package.
	MatchBarcode MatchLevel = "barcode"
	// MatchCode is a supplier item code that the catalogue also carries.
	MatchCode MatchLevel = "code"
	// MatchExact is an identical name once folded, with no attribute conflict.
	MatchExact MatchLevel = "exact"
	// MatchStrong is a high-scoring similarity match with corroborating
	// attributes.
	MatchStrong MatchLevel = "strong"
	// MatchReview is plausible and must be confirmed by the vendor.
	MatchReview MatchLevel = "review"
	// MatchAmbiguous means two catalogue products fit equally well.
	MatchAmbiguous MatchLevel = "ambiguous"
	// MatchNone means nothing in the catalogue fits.
	MatchNone MatchLevel = "none"
)

// Label renders a level in Arabic.
func (l MatchLevel) Label() string {
	switch l {
	case MatchBarcode:
		return "مطابقة بالباركود"
	case MatchCode:
		return "مطابقة بالكود"
	case MatchExact:
		return "مطابقة تامة للاسم"
	case MatchStrong:
		return "مطابقة قوية"
	case MatchReview:
		return "تحتاج مراجعة"
	case MatchAmbiguous:
		return "أكثر من صنف محتمل"
	default:
		return "غير مطابق"
	}
}

// Settled reports whether the match may be applied without asking.
func (l MatchLevel) Settled() bool {
	switch l {
	case MatchBarcode, MatchCode, MatchExact, MatchStrong:
		return true
	}
	return false
}

// MatchCandidate is one catalogue product a row could be.
type MatchCandidate struct {
	ProductID     int64   `json:"product_id"`
	Name          string  `json:"name"`
	Scientific    string  `json:"scientific,omitempty"`
	DosageForm    string  `json:"dosage_form,omitempty"`
	Concentration string  `json:"concentration,omitempty"`
	Manufacturer  string  `json:"manufacturer,omitempty"`
	PublicPrice   string  `json:"public_price,omitempty"`
	Score         float64 `json:"score"`
	Reason        string  `json:"reason,omitempty"`
}

// MatchResult is the outcome for one row.
type MatchResult struct {
	ProductID  int64            `json:"product_id,omitempty"`
	Level      MatchLevel       `json:"level"`
	Score      float64          `json:"score"`
	Reason     string           `json:"reason"`
	Candidates []MatchCandidate `json:"candidates,omitempty"`
}

// Matched reports whether a product was tied to the row at all.
func (r MatchResult) Matched() bool { return r.ProductID > 0 }

// MatchOptions tune the decision thresholds.
type MatchOptions struct {
	// MinStrong is the score at or above which a match is applied without
	// asking. Lowering it trades false negatives for false positives, and in a
	// catalogue where a false positive prices the wrong medicine, the default
	// is deliberately conservative.
	MinStrong float64
	// MinReview is the score below which nothing is offered at all.
	MinReview float64
	// TrustSupplierCode allows the vendor's own item code to match the
	// catalogue's. Off by default: a vendor's "951" is their internal
	// numbering and coincides with a catalogue code by accident more often
	// than by design.
	TrustSupplierCode bool
	// PoolLimit caps how many catalogue products are scored per row.
	PoolLimit int
	// MaxCandidates is how many alternatives are kept for the review screen.
	MaxCandidates int
}

// DefaultMatchOptions are the thresholds the wizard starts on.
func DefaultMatchOptions() MatchOptions {
	return MatchOptions{
		MinStrong:     0.78,
		MinReview:     0.42,
		PoolLimit:     400,
		MaxCandidates: 5,
	}
}

// Match ties one parsed row to a catalogue product.
func (idx *Index) Match(row *Row, opts MatchOptions) MatchResult {
	if idx == nil || idx.total == 0 {
		return MatchResult{Level: MatchNone, Reason: "الكتالوج المركزي فارغ؛ لا يوجد ما يُطابَق عليه"}
	}
	if opts.PoolLimit <= 0 {
		opts.PoolLimit = 400
	}
	if opts.MaxCandidates <= 0 {
		opts.MaxCandidates = 5
	}

	q := idx.newQuery(row)

	if res, ok := idx.matchByBarcode(row); ok {
		return res
	}
	if opts.TrustSupplierCode {
		if res, ok := idx.matchByCode(row, q); ok {
			return res
		}
	}

	scored := idx.score(q, opts)
	if len(scored) == 0 {
		return MatchResult{Level: MatchNone, Reason: "لم يتم العثور على صنف مشابه في الكتالوج المركزي"}
	}
	return decide(scored, opts)
}

// query is one row reduced to its matching signals, with the scratch space the
// scorer reuses across every candidate so a comparison allocates nothing.
type query struct {
	tokens []string
	// weights is the inverse document frequency of each distinct query token,
	// parallel to tokens; pos indexes back into both.
	weights     []float64
	pos         map[string]int
	totalWeight float64
	// stamp and epoch mark the tokens consumed by the candidate under
	// comparison, so the map never has to be cleared between candidates.
	stamp []int32
	epoch int32

	tri      []string
	nums     []float64
	nameKey  string
	formKey  string
	strength strength
	packSize int
	makerKey string
	sciKey   string
}

func (idx *Index) newQuery(row *Row) *query {
	full := row.Name + " " + row.NameEN
	nameTokens := coreTokens(full)
	q := &query{
		tokens:   coreTokens(full + " " + row.Scientific),
		tri:      sortedTrigrams(nameTokens),
		nums:     numberSignature(full),
		nameKey:  strings.Join(nameTokens, " "),
		formKey:  formKeyOf(full + " " + row.DosageForm),
		strength: parseStrength(full + " " + row.Concentration),
		packSize: row.PackSize,
		makerKey: sheet.NormalizeKey(row.Manufacturer),
		sciKey:   sheet.NormalizeKey(row.Scientific),
	}
	if q.packSize == 0 {
		q.packSize = InferPackSize(full)
	}

	q.pos = make(map[string]int, len(q.tokens))
	for _, t := range q.tokens {
		if _, dup := q.pos[t]; dup {
			continue
		}
		w := idx.idf(t)
		if w <= 0 {
			// A word the catalogue has never seen carries no matching weight,
			// but it is still information the candidate lacks.
			w = 0.5
		}
		q.pos[t] = len(q.weights)
		q.weights = append(q.weights, w)
		q.totalWeight += w
	}
	q.stamp = make([]int32, len(q.weights))
	return q
}

// matchByBarcode resolves the one identifier that means the same package.
func (idx *Index) matchByBarcode(row *Row) (MatchResult, bool) {
	code := sheet.DigitsOnly(row.Barcode)
	if code == "" || len(code) < 8 {
		return MatchResult{}, false
	}
	hits := idx.byBarcode[code]
	switch len(hits) {
	case 0:
		return MatchResult{}, false
	case 1:
		return MatchResult{
			ProductID: hits[0].ID, Level: MatchBarcode, Score: 1,
			Reason: "مطابقة تامة عبر الباركود الدولي",
		}, true
	}
	// Several catalogue products share the barcode, which happens for a
	// multi-pack registered under one GTIN. Fall through to scoring so the name
	// and the dose can separate them, but keep the shortlist.
	return MatchResult{}, false
}

// matchByCode resolves a supplier code the catalogue also carries.
//
// It demands a corroborating name similarity, because a bare numeric code
// colliding across two numbering schemes is common and the consequence — the
// wrong medicine priced — is not recoverable by looking at the result.
func (idx *Index) matchByCode(row *Row, q *query) (MatchResult, bool) {
	key := sheet.NormalizeKey(row.SKU)
	if len([]rune(key)) < 4 {
		return MatchResult{}, false
	}
	hits := idx.bySKU[key]
	if len(hits) != 1 {
		return MatchResult{}, false
	}
	p := hits[0]
	if len(q.tokens) > 0 && idx.nameSimilarity(q, p) < 0.35 {
		return MatchResult{}, false
	}
	return MatchResult{
		ProductID: p.ID, Level: MatchCode, Score: 0.95,
		Reason: "مطابقة عبر كود الصنف مع تشابه في الاسم",
	}, true
}

// scoredProduct is one candidate and why it scored what it did.
type scoredProduct struct {
	product *MasterProduct
	score   float64
	reason  string
	exact   bool
}

// score rates every plausible catalogue product for one query.
func (idx *Index) score(q *query, opts MatchOptions) []scoredProduct {
	pool := idx.candidatePool(q.tokens, opts.PoolLimit)
	if q.nameKey != "" {
		pool = append(pool, idx.byName[q.nameKey]...)
	}
	if len(pool) == 0 {
		return nil
	}

	seen := make(map[int64]bool, len(pool))
	out := make([]scoredProduct, 0, len(pool))
	for _, p := range pool {
		if seen[p.ID] {
			continue
		}
		seen[p.ID] = true
		if sp, ok := idx.rate(q, p); ok && sp.score >= opts.MinReview {
			out = append(out, sp)
		}
	}

	sort.SliceStable(out, func(i, j int) bool {
		if out[i].score != out[j].score {
			return out[i].score > out[j].score
		}
		return out[i].product.ID < out[j].product.ID
	})
	return out
}

// decide turns the ranked candidates into an answer, including the answer
// "these two fit equally well and I will not choose between them".
func decide(scored []scoredProduct, opts MatchOptions) MatchResult {
	best := scored[0]
	res := MatchResult{
		ProductID:  best.product.ID,
		Score:      best.score,
		Candidates: describe(scored, opts.MaxCandidates),
	}

	tied := len(scored) > 1 && best.score-scored[1].score < 0.03 && best.score < 0.97
	switch {
	case tied:
		res.Level = MatchAmbiguous
		res.Reason = fmt.Sprintf(
			"صنفان في الكتالوج بنفس درجة التطابق (%d%%)؛ يلزم اختيار الصحيح يدوياً.",
			int(best.score*100))
	case best.exact:
		res.Level = MatchExact
		res.Reason = "تطابق تام لاسم الصنف بعد المعايرة مع توافق الخصائص"
	case best.score >= opts.MinStrong:
		res.Level = MatchStrong
		res.Reason = fmt.Sprintf("%s (%d%%)", best.reason, int(best.score*100))
	default:
		res.Level = MatchReview
		res.Reason = fmt.Sprintf("%s (%d%%) — يحتاج تأكيداً", best.reason, int(best.score*100))
	}
	return res
}

// describe renders the shortlist for the review screen.
func describe(scored []scoredProduct, limit int) []MatchCandidate {
	if len(scored) > limit {
		scored = scored[:limit]
	}
	out := make([]MatchCandidate, 0, len(scored))
	for _, s := range scored {
		name := s.product.NameAR
		if name == "" {
			name = s.product.NameEN
		}
		out = append(out, MatchCandidate{
			ProductID:     s.product.ID,
			Name:          name,
			Scientific:    s.product.Scientific,
			DosageForm:    s.product.DosageForm,
			Concentration: s.product.Concentration,
			Manufacturer:  s.product.Manufacturer,
			PublicPrice:   s.product.PublicPrice,
			Score:         s.score,
			Reason:        s.reason,
		})
	}
	return out
}
