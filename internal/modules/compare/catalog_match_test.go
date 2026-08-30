package compare

import (
	"context"
	"log/slog"
	"testing"
)

// fakeCatalog is a two-product shared catalogue.
type fakeCatalog struct{}

func (fakeCatalog) ListMatchProducts(context.Context) ([]CatalogProduct, error) {
	return []CatalogProduct{
		{ID: 11, NameAR: "بانادول اكسترا 24 قرص", NameEN: "Panadol Extra 24 tabs"},
		{ID: 12, NameAR: "اوجمنتين 1 جم 14 قرص", NameEN: "Augmentin 1g 14 tabs"},
	}, nil
}

// matchRepo is the slice of the repository MatchFileRows touches.
type matchRepo struct {
	Repository
	rows    []*CompareFileRow
	written []RowMatch
	mapped  map[string]int64
}

func (m *matchRepo) ListFileRows(context.Context, int64, int, int) ([]*CompareFileRow, error) {
	return m.rows, nil
}

func (m *matchRepo) GetSavedProductMapping(_ context.Context, _ *int64, rawName string) (*int64, error) {
	if id, ok := m.mapped[rawName]; ok {
		return &id, nil
	}
	return nil, nil
}

func (m *matchRepo) BulkUpdateFileRowMatches(_ context.Context, _ int64, matches []RowMatch) error {
	m.written = append(m.written, matches...)
	return nil
}

// The stage this tool never had. Every row it can settle must come back with a
// catalogue product; every row it cannot must come back with none, rather than
// with the closest thing in the catalogue.
func TestMatchFileRowsLinksWhatItCanAndRefusesTheRest(t *testing.T) {
	repo := &matchRepo{
		rows: []*CompareFileRow{
			{ID: 1, RawName: "بانادول اكسترا 24 قرص سعر جديد"},
			{ID: 2, RawName: "أوجمنتين ١ جم ١٤ قرص"},
			{ID: 3, RawName: "شامبو للشعر 200 مل"},
			{ID: 4, RawName: "صنف خاص بالمورد"},
		},
		mapped: map[string]int64{"صنف خاص بالمورد": 12},
	}
	svc := NewService(repo, slog.Default())
	svc.SetCatalogSource(fakeCatalog{})

	stats, err := svc.MatchFileRows(context.Background(), 7, false, nil)
	if err != nil {
		t.Fatalf("MatchFileRows: %v", err)
	}

	got := map[int64]int64{}
	for _, m := range repo.written {
		if m.ProductID != nil {
			got[m.RowID] = *m.ProductID
		}
	}
	if got[1] != 11 {
		t.Errorf("row 1 linked to %d, want 11", got[1])
	}
	if got[2] != 12 {
		t.Errorf("row 2 linked to %d, want 12", got[2])
	}
	if _, linked := got[3]; linked {
		t.Errorf("row 3 was linked to %d; nothing in this catalogue is a shampoo", got[3])
	}
	// A decision the user already made outranks the engine and costs nothing.
	if got[4] != 12 {
		t.Errorf("row 4 ignored its saved mapping, linked to %d", got[4])
	}
	if stats.Saved != 1 {
		t.Errorf("saved mappings counted %d, want 1", stats.Saved)
	}
	if stats.Rows != 4 {
		t.Errorf("rows counted %d, want 4", stats.Rows)
	}
}

// With no catalogue wired the stage refuses rather than silently matching
// nothing, because "the button did nothing" is the failure this whole feature
// exists to stop.
func TestMatchFileRowsRefusesWithoutCatalogue(t *testing.T) {
	svc := NewService(&matchRepo{}, slog.Default())
	if _, err := svc.MatchFileRows(context.Background(), 1, false, nil); err == nil {
		t.Fatal("MatchFileRows with no catalogue source returned no error")
	}
	if svc.MatchingAvailable() {
		t.Fatal("MatchingAvailable() is true with no catalogue source")
	}
}
