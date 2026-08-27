package pipeline

import (
	"context"

	"github.com/muhiya/dawa24-store/internal/modules/smartorder"
	"github.com/muhiya/dawa24-store/internal/shared/productmatch"
)

// The fuzzy tier.
//
// Everything the exact tiers could not settle is scored against an in-memory
// projection of the catalogue. The index is built **once per run** — roughly
// twenty thousand products and a few megabytes — and every remaining line is
// scored against it in memory. That is the whole trick: one query loads the
// catalogue, and the per-row cost after that is CPU, which parallelises and does
// not queue behind a network.
//
// The scorer is internal/shared/productmatch, the same engine the vendor import
// uses. Its strength veto matters more here than anywhere: a 500 mg row matched
// to a 1 g product is not a ranking inaccuracy, it is the wrong medicine.

// Matcher scores unresolved lines against the catalogue.
type Matcher struct {
	repo  smartorder.Repository
	index *productmatch.Index
	opts  productmatch.MatchOptions
}

// NewMatcher constructs the fuzzy stage.
func NewMatcher(repo smartorder.Repository) *Matcher {
	opts := productmatch.DefaultMatchOptions()
	// TrustSupplierCode stays off. A pharmacy's internal code coinciding with a
	// catalogue code is more often accident than design, and the SKU tier has
	// already had its chance at genuine codes.
	opts.MaxCandidates = 5 // the shortlist the review screen shows the buyer
	return &Matcher{repo: repo, opts: opts}
}

// Load builds the in-memory catalogue index for this run.
func (m *Matcher) Load(ctx context.Context) error {
	products, err := m.repo.LoadMatchIndex(ctx)
	if err != nil {
		return err
	}
	masters := make([]productmatch.MasterProduct, 0, len(products))
	for _, p := range products {
		masters = append(masters, productmatch.MasterProduct{
			ID:            p.ID,
			NameAR:        p.NameAR,
			NameEN:        p.NameEN,
			SKU:           p.SKU,
			Barcode:       p.Barcode,
			Scientific:    p.Scientific,
			DosageForm:    p.DosageForm,
			Concentration: p.Concentration,
			Unit:          p.Unit,
			Manufacturer:  p.Manufacturer,
		})
	}
	m.index = productmatch.NewIndex(masters)
	return nil
}

// Size reports how many catalogue products are indexed, for telemetry and for
// the "is the catalogue empty?" check that otherwise looks like "nothing
// matched".
func (m *Matcher) Size() int {
	if m.index == nil {
		return 0
	}
	return m.index.Size()
}

// Index exposes the loaded catalogue to the AI stage, which needs it for two
// things the shortlist cannot supply: the English name of a candidate, and the
// deterministic strength re-check that validates an answer before it is written.
func (m *Matcher) Index() *productmatch.Index { return m.index }

// Score matches every unresolved line and returns those still needing help.
//
// Lines that clear the cutoff are marked resolved here and are then off-limits
// to AI — FR-018 forbids overwriting a confident deterministic result. What
// comes back is exactly the two populations the buyer sees as غير مطابق and
// مطلوب للمراجعة: nothing matched at all, or matched but below the cutoff.
func (m *Matcher) Score(lines []*smartorder.Line) []Review {
	if m.index == nil || m.index.Size() == 0 {
		// An empty index is not "nothing matched": it is a broken run, and
		// reporting it as unmatched lines would send the buyer hunting for
		// products that are in fact right there.
		return nil
	}

	var reviews []Review
	for _, l := range lines {
		if l.Matched() && l.MatchConfidence >= Cutoff {
			continue
		}
		// The line is decomposed first: strength, form and pack move into the
		// structured fields the scorer compares separately, and therapeutic
		// prose is dropped. See query.go for what happens without this.
		row := BuildRow(l)
		res := m.index.Match(row, m.opts)

		switch {
		case res.Matched() && res.Score >= Cutoff:
			setMatch(l, res.ProductID, methodFor(res.Level), res.Score)

		case res.Matched() && res.Level == productmatch.MatchStrong:
			// Below the cutoff but plausible: record it so the buyer sees the
			// system's best guess, and carry it into the AI stage anyway.
			setMatch(l, res.ProductID, smartorder.MethodFuzzy, res.Score)
			reviews = append(reviews, Review{Line: l, Row: row, Candidates: res.Candidates})

		default:
			reviews = append(reviews, Review{Line: l, Row: row, Candidates: res.Candidates})
		}
	}
	return reviews
}

// Retrieve widens each review's shortlist for the AI stage.
//
// Match's candidates are the by-product of a decision that has already failed:
// they come from the pool that defeated the scorer, ranked by the scoring that
// could not separate them, and when the scorer found nothing at all there are
// none. Handing those to a model asks it to re-rank a list that does not contain
// the answer.
//
// Recall runs a different retrieval — token, trigram and molecule strategies
// unioned, with the disqualifying penalties removed — and replaces the
// shortlist with a wider one. The deterministic candidates are kept at the head
// of it, because when the scorer *did* find something plausible its ranking is
// still the best evidence available about what to look at first.
//
// Called only when the AI stage will actually run, so an import that resolves
// deterministically never pays for retrieval nothing reads.
func (m *Matcher) Retrieve(reviews []Review) {
	if m.index == nil {
		return
	}
	opts := productmatch.DefaultRecallOptions()
	opts.Limit = RecallLimit

	for i := range reviews {
		wide := m.index.Recall(reviews[i].Row, opts)
		reviews[i].Candidates = mergeCandidates(reviews[i].Candidates, wide, RecallLimit)
	}
}

// mergeCandidates keeps the deterministic shortlist first, then fills from the
// wider retrieval, de-duplicated, up to the limit.
func mergeCandidates(primary, wide []productmatch.MatchCandidate, limit int) []productmatch.MatchCandidate {
	out := make([]productmatch.MatchCandidate, 0, limit)
	seen := make(map[int64]bool, limit)

	for _, sets := range [][]productmatch.MatchCandidate{primary, wide} {
		for _, c := range sets {
			if len(out) >= limit {
				return out
			}
			if c.ProductID <= 0 || seen[c.ProductID] {
				continue
			}
			seen[c.ProductID] = true
			out = append(out, c)
		}
	}
	return out
}

// methodFor maps the matcher's level onto the method the buyer sees.
func methodFor(level productmatch.MatchLevel) smartorder.MatchMethod {
	switch level {
	case productmatch.MatchBarcode:
		return smartorder.MethodBarcode
	case productmatch.MatchCode:
		return smartorder.MethodSKU
	case productmatch.MatchExact:
		return smartorder.MethodExactName
	case productmatch.MatchStrong:
		return smartorder.MethodIdentityKey
	default:
		return smartorder.MethodFuzzy
	}
}
