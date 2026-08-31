package compare

// Tying a supplier's price list to the shared catalogue.
//
// The compare tool has carried a matched_product_id on every row since it was
// written, and nothing has ever set it. MatchLadder existed and was called from
// nowhere but its own tests, so every comparison the tool has ever produced was
// made by joining supplier lines to each other on a normalised string — which
// is why i18n.TDefault("w4_ui.24_57") from one supplier and i18n.TDefault("w4m_mod.s_12_12") from another appear as two different products in a price comparison.
//
// This is that missing stage, and it is deliberately NOT a fifth
// implementation of matching. It builds the same productmatch.Index the vendor
// import and the smart order build, scores rows through the same engine with
// the same vetoes, asks the same model the same question through the same
// prompt when the deterministic pass cannot settle a row, and reads and writes
// the same decision cache. What is specific to compare is only what a match
// means here: nothing is written to a catalogue and nothing is ordered, so a
// wrong link mislabels one row of one private price list.

import (
	"context"
	"fmt"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"strings"

	"github.com/muhiya/dawa24-store/internal/shared/matchflow"
	"github.com/muhiya/dawa24-store/internal/shared/productmatch"
)

// CatalogProduct is one shared-catalogue product as the compare tool needs it.
//
// Declared here rather than imported from the catalogue module, because a
// module may not import another module's types (AGENTS.md). The composition
// root adapts one to the other, which is the only place the two need to know
// about each other.
type CatalogProduct struct {
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
}

// CatalogSource supplies the shared catalogue. Unset means the matching stage
// is unavailable and says so, rather than matching nothing quietly.
type CatalogSource interface {
	ListMatchProducts(ctx context.Context) ([]CatalogProduct, error)
}

// SetCatalogSource attaches the shared catalogue.
func (s *Service) SetCatalogSource(src CatalogSource) { s.catalog = src }

// SetMatchEnhancer attaches the shared AI matching stage — the same one the
// vendor import and the smart order run.
func (s *Service) SetMatchEnhancer(e matchflow.Enhancer) { s.enhancer = e }

// SetMatchMemory attaches the shared decision cache.
func (s *Service) SetMatchMemory(m matchflow.Memory) { s.memory = m }

// MatchingAvailable reports whether the catalogue is wired, so the screen can
// explain a missing button instead of offering one that does nothing.
func (s *Service) MatchingAvailable() bool { return s != nil && s.catalog != nil }

// AIMatchingAvailable reports whether the AI tier can run at all.
func (s *Service) AIMatchingAvailable() bool { return s != nil && s.enhancer != nil }

// MatchStats is what one matching run resolved, for the notice the user reads.
type MatchStats struct {
	Rows      int
	Settled   int
	Review    int
	Unmatched int
	Saved     int
	AI        int
	CacheHits int
	Requests  int
	// CeilingHit means the AI tier stopped early and the remaining rows kept
	// their deterministic outcome.
	CeilingHit bool
}

// Matched is how many rows ended the run tied to a catalogue product.
func (m MatchStats) Matched() int { return m.Settled + m.Saved + m.AI }

// matchCeilings are what one compare run may spend.
//
// The order profile: these files are a person's own price lists, someone is
// watching the screen, and a wrong link is visible in their own table rather
// than in a shared catalogue.
var matchCeilings = matchflow.For(matchflow.ProfileOrder)

// MatchFileRows ties every row of one uploaded price list to the shared
// catalogue, and records what it resolved.
//
// useAI is the user's switch. With it off the run is pure arithmetic against
// the in-memory index and costs nothing; with it on, the rows the engine could
// not settle get a second opinion, under the same guards and the same ceilings
// as every other importer.
func (s *Service) MatchFileRows(
	ctx context.Context, fileID int64, useAI bool, orgID *int64,
) (MatchStats, error) {
	var stats MatchStats
	if s == nil || s.repo == nil {
		return stats, fmt.Errorf("compare: service not configured")
	}
	if s.catalog == nil {
		return stats, fmt.Errorf("%s", i18n.TDefault("w4_mod.s_362_362"))
	}

	rows, err := s.repo.ListFileRows(ctx, fileID, matchRowCap, 0)
	if err != nil {
		return stats, err
	}
	if len(rows) == 0 {
		return stats, nil
	}
	stats.Rows = len(rows)

	index, err := s.catalogIndex(ctx)
	if err != nil {
		return stats, err
	}

	updates := make([]RowMatch, 0, len(rows))
	var pending []*openRow

	opts := productmatch.DefaultMatchOptions()
	for _, row := range rows {
		name := strings.TrimSpace(row.RawName)
		if name == "" {
			stats.Unmatched++
			continue
		}

		// A decision this user already made by hand outranks anything the
		// engine can say, and it is free.
		if id, mErr := s.repo.GetSavedProductMapping(ctx, orgID, row.RawName); mErr == nil && id != nil {
			updates = append(updates, RowMatch{
				RowID: row.ID, ProductID: id,
				Method: MatchMethodSavedMapping, Confidence: 100,
			})
			stats.Saved++
			continue
		}

		q := &productmatch.Row{Name: name, SKU: row.SKU}
		res := index.Match(q, opts)
		switch {
		case res.Matched() && res.Level.Settled():
			id := res.ProductID
			updates = append(updates, RowMatch{
				RowID: row.ID, ProductID: &id,
				Method: methodFor(res.Level), Confidence: res.Score * 100,
			})
			stats.Settled++
		default:
			// Everything else is left unlinked and offered to the AI tier. A
			// suggestion the engine would not stand behind is not written as a
			// match here any more than it is in the vendor import: a comparison
			// built on a guess is worse than one with a gap in it.
			if res.Level == productmatch.MatchReview || res.Level == productmatch.MatchAmbiguous {
				stats.Review++
			} else {
				stats.Unmatched++
			}
			pending = append(pending, &openRow{row: row, query: q})
		}
	}

	if useAI && s.enhancer != nil && len(pending) > 0 {
		s.enhanceMatches(ctx, index, pending, &updates, &stats)
	}

	if len(updates) > 0 {
		if err := s.repo.BulkUpdateFileRowMatches(ctx, fileID, updates); err != nil {
			return stats, err
		}
	}
	return stats, nil
}

// matchRowCap bounds one matching run. A compare file above it is not a price
// list, and the tool's own reader refuses far larger files anyway.
const matchRowCap = 60000

// RowMatch is one resolved row, ready to be written.
type RowMatch struct {
	RowID      int64
	ProductID  *int64
	Method     MatchMethod
	Confidence float64
}

// openRow is one row the deterministic pass could not settle, carried to the AI
// tier with the parsed query the identity guard re-reads.
type openRow struct {
	row   *CompareFileRow
	query *productmatch.Row
	// candidates is filled by retrieval, and is also the shortlist an answer
	// must come from.
	candidates []productmatch.MatchCandidate
}

// catalogIndex builds the in-memory catalogue this run scores against.
func (s *Service) catalogIndex(ctx context.Context) (*productmatch.Index, error) {
	products, err := s.catalog.ListMatchProducts(ctx)
	if err != nil {
		return nil, fmt.Errorf("تعذر تحميل الكتالوج المركزي: %w", err)
	}
	if len(products) == 0 {
		return nil, fmt.Errorf("%s", i18n.TDefault("w4_mod.s_363_363"))
	}
	masters := make([]productmatch.MasterProduct, 0, len(products))
	for _, p := range products {
		masters = append(masters, productmatch.MasterProduct{
			ID: p.ID, NameAR: p.NameAR, NameEN: p.NameEN, SKU: p.SKU,
			Barcode: p.Barcode, Scientific: p.Scientific, DosageForm: p.DosageForm,
			Concentration: p.Concentration, Unit: p.Unit,
			Manufacturer: p.Manufacturer, PublicPrice: p.PublicPrice,
		})
	}
	return productmatch.NewIndex(masters), nil
}

// methodFor renders the engine's level in the vocabulary compare's own rows and
// screens already use.
func methodFor(level productmatch.MatchLevel) MatchMethod {
	switch level {
	case productmatch.MatchBarcode, productmatch.MatchCode:
		return MatchMethodSKU
	case productmatch.MatchExact:
		return MatchMethodExactName
	default:
		return MatchMethodFuzzy
	}
}
