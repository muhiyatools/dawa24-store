package productmatch

import (
	"fmt"
	"sort"

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
	// TrustSupplierCode allows the file's item code to match the catalogue's.
	//
	// Off by default: a vendor's "951" is their internal numbering and
	// coincides with a catalogue code by accident more often than by design.
	// Set it through WithIdentifiers rather than directly, so it can never be
	// on for a file whose code column was never mapped.
	TrustSupplierCode bool
	// TrustBarcode allows the file's barcode to settle a match on its own.
	//
	// Off by default, and it did not used to be a choice at all: the barcode
	// tier ran unconditionally, before the name, the dose or the form was
	// consulted, and any eight-digit value with a single catalogue hit won at
	// confidence 1.0. Pharmacy item numbering is routinely eight or nine
	// digits, so a file that mapped its own reference numbers into that field
	// produced confident, unreviewable links to unrelated medicines.
	//
	// A GTIN really does identify a package, so when the column holds one this
	// is the strongest evidence available. The question is only ever whether
	// the column holds one, and that is the user's to answer.
	TrustBarcode bool
	// CodeIsAuthoritative accepts a code match without corroboration from the
	// name.
	//
	// Off by default, and the default is right for a supplier file: a vendor's
	// "951" collides with a catalogue code by accident more often than by
	// design, so a bare numeric hit must be seconded by the name.
	//
	// On where the user has said the column *is* the catalogue's code — a
	// pharmacy's own reference list is kept by the code they look products up
	// by, and demanding the name agree as well refuses the exact rows the
	// column was mapped for.
	CodeIsAuthoritative bool
	// PoolLimit caps how many catalogue products are scored per row.
	PoolLimit int
	// MaxCandidates is how many alternatives are kept for the review screen.
	MaxCandidates int
}

// The thresholds every tool starts on.
//
// DefaultMinStrong is stated once, here, because four screens print it and four
// importers compare against it, and a figure that lives in six places is a
// figure that means six things by the end of the year. The vendor catalogue
// import, the admin main-catalogue import, the saving-products import and the
// smart order all begin here; each may raise it, none may lower it past
// DefaultMinReview.
//
// Fifty per cent, at the client's direction. It was forty, and before that
// thirty. The band between forty and fifty is, on the live supplier files,
// almost entirely line extensions and coincidences — the same brand at another
// dose, or two products agreeing on a category word — and applying those
// silently is what a vendor discovers three weeks later in their own catalogue.
// Everything below it is still shown, still scored, and still one click from
// being accepted; it is simply not accepted FOR them.
const (
	DefaultMinStrong = 0.50
	// DefaultMinReview is the floor below which nothing is offered at all.
	//
	// It used to be 0.15, which is how a review screen came to be full of
	// sixteen-per-cent suggestions. A score that low is not a weak opinion, it
	// is the absence of one, and printing a product name beside it invites a
	// reviewer to accept something the engine never believed.
	DefaultMinReview = 0.25
)

// DefaultMatchOptions are the thresholds the wizard starts on.
//
// Every identifier tier is off. That is deliberate and it is the important part
// of this function: a tier that settles a match without consulting the name is
// only safe when the user has said the column holds the catalogue's own
// identifier, and a default of "on" asks nobody. Tools switch them on through
// WithIdentifiers once the column is mapped and the user has chosen it.
func DefaultMatchOptions() MatchOptions {
	return MatchOptions{
		MinStrong:     DefaultMinStrong,
		MinReview:     DefaultMinReview,
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

	if opts.TrustBarcode {
		if res, ok := idx.matchByBarcode(row); ok {
			return res
		}
	}
	if opts.TrustSupplierCode {
		if res, ok := idx.matchByCode(row, q, opts); ok {
			return res
		}
	}

	scored := idx.score(q, opts)
	if len(scored) == 0 {
		return MatchResult{Level: MatchNone, Reason: "لم يتم العثور على صنف مشابه في الكتالوج المركزي"}
	}
	return decide(scored, opts)
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
// It demands a corroborating name similarity by default, because a bare numeric
// code colliding across two numbering schemes is common and the consequence —
// the wrong medicine priced — is not recoverable by looking at the result.
// CodeIsAuthoritative lifts that where the user has stated the column holds the
// catalogue's own code.
func (idx *Index) matchByCode(row *Row, q *query, opts MatchOptions) (MatchResult, bool) {
	key := sheet.NormalizeCode(row.SKU)
	if len([]rune(key)) < 4 {
		return MatchResult{}, false
	}
	hits := idx.bySKU[key]
	if len(hits) != 1 {
		return MatchResult{}, false
	}
	p := hits[0]
	if !opts.CodeIsAuthoritative && len(q.tokens) > 0 && idx.nameSimilarity(q, p) < 0.35 {
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
	pool := idx.candidatePoolWithKeys(q.tokens, q.keys, opts.PoolLimit)
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
