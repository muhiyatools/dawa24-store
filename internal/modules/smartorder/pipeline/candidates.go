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
// thirty thousand products and a few megabytes — and every remaining line is
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
	opts.MaxCandidates = 5 // the shortlist AI adjudication receives
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

// Residual is a line the fuzzy tier could not settle, carried forward to
// adjudication with the shortlist already retrieved.
type Residual struct {
	Line       *smartorder.Line
	Candidates []productmatch.MatchCandidate
}

// Score matches every unresolved line and returns those still below the cutoff.
//
// Lines that clear the cutoff are marked resolved here and are then off-limits
// to AI — FR-018 forbids overwriting a confident deterministic result.
func (m *Matcher) Score(lines []*smartorder.Line) []Residual {
	if m.index == nil || m.index.Size() == 0 {
		// An empty index is not "nothing matched": it is a broken run, and
		// reporting it as unmatched lines would send the buyer hunting for
		// products that are in fact right there.
		return nil
	}

	var residual []Residual
	for _, l := range lines {
		if l.Matched() && l.MatchConfidence >= Cutoff {
			continue
		}
		row := &productmatch.Row{
			Number:  l.RowNumber,
			Name:    l.RawName,
			SKU:     l.RawSKU,
			Barcode: l.RawBarcode,
		}
		res := m.index.Match(row, m.opts)

		switch {
		case res.Matched() && res.Score >= Cutoff:
			setMatch(l, res.ProductID, methodFor(res.Level), res.Score)

		case res.Matched() && res.Level == productmatch.MatchStrong:
			// Below the cutoff but plausible: record it so the buyer sees the
			// system's best guess, and hand it to adjudication.
			setMatch(l, res.ProductID, smartorder.MethodFuzzy, res.Score)
			residual = append(residual, Residual{Line: l, Candidates: res.Candidates})

		default:
			residual = append(residual, Residual{Line: l, Candidates: res.Candidates})
		}
	}
	return residual
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
