package ingest

import (
	"context"
	"log/slog"
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/modules/inventory"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/shared/productmatch"
)

// mockImportStore provides an in-memory fake for ImportStore during commit testing.
type mockCommitImportStore struct {
	session        *Session
	stagedRows     []*RowOutcome
	committedRows  []RowOutcome
	finishedCalled bool
	// pendingRows is how many included rows the vendor never confirmed.
	pendingRows int
	// mentions is what every included row of the file refers to, confirmed or
	// not, which is what the replace mode protects from retirement.
	mentions []RowMention
}

func (m *mockCommitImportStore) MentionedRows(_ context.Context, _ int64) ([]RowMention, error) {
	return m.mentions, nil
}

func (m *mockCommitImportStore) Create(_ context.Context, _ *Session, _ []byte) error { return nil }
func (m *mockCommitImportStore) Get(_ context.Context, _ string) (*Session, error) {
	if m.session != nil {
		return m.session, nil
	}
	return nil, ErrImportStoreUnavailable
}
func (m *mockCommitImportStore) File(_ context.Context, _ int64) ([]byte, error) { return nil, nil }
func (m *mockCommitImportStore) SaveDraft(_ context.Context, s *Session) error {
	m.session = s
	return nil
}
func (m *mockCommitImportStore) Begin(_ context.Context, _ int64) error       { return nil }
func (m *mockCommitImportStore) BeginCommit(_ context.Context, _ int64) error { return nil }
func (m *mockCommitImportStore) Progress(_ context.Context, _ int64, _ int, _ string) error {
	return nil
}
func (m *mockCommitImportStore) FinishStaging(_ context.Context, s *Session) error {
	m.session = s
	return nil
}
func (m *mockCommitImportStore) RecoverStaleRuns(_ context.Context) (int, error) { return 0, nil }
func (m *mockCommitImportStore) Finish(_ context.Context, s *Session) error {
	m.session = s
	m.finishedCalled = true
	return nil
}
func (m *mockCommitImportStore) Fail(_ context.Context, _ int64, _ string) error { return nil }
func (m *mockCommitImportStore) Cancel(_ context.Context, _ int64) error         { return nil }
func (m *mockCommitImportStore) List(_ context.Context, _ int64, _ int) ([]*Session, error) {
	return nil, nil
}
func (m *mockCommitImportStore) AppendRows(_ context.Context, _, _ int64, _ []RowOutcome) error {
	return nil
}
func (m *mockCommitImportStore) ClearRows(_ context.Context, _ int64) error { return nil }
func (m *mockCommitImportStore) Rows(_ context.Context, _ int64, _ RowFilter) ([]*RowOutcome, int, error) {
	return m.stagedRows, len(m.stagedRows), nil
}
func (m *mockCommitImportStore) RowCounts(_ context.Context, _ int64) (map[string]int, error) {
	return map[string]int{}, nil
}
func (m *mockCommitImportStore) ApplyAIMatches(_ context.Context, _ int64, _ []AIMatch) error {
	return nil
}
func (m *mockCommitImportStore) UpdateRow(_ context.Context, _, _ int64, _ string, _ string, _, _ *float64, _ *int, _ *bool) error {
	return nil
}
func (m *mockCommitImportStore) SetBatchQuantity(_ context.Context, _ int64, _ int) error { return nil }
func (m *mockCommitImportStore) AssignRowMatch(_ context.Context, _, _, _ int64, _, _ string) error {
	return nil
}
func (m *mockCommitImportStore) ToggleRowExclude(_ context.Context, _, _ int64) (bool, error) {
	return false, nil
}
func (m *mockCommitImportStore) ConfirmRowMatches(_ context.Context, _ int64, _ []int64) (int, error) {
	return 0, nil
}
func (m *mockCommitImportStore) ClearRowMatches(_ context.Context, _ int64, _ []int64) (int, error) {
	return 0, nil
}
func (m *mockCommitImportStore) SetRowsExcluded(_ context.Context, _ int64, _ []int64, _ bool) (int, error) {
	return 0, nil
}
func (m *mockCommitImportStore) PendingRowIDs(_ context.Context, _ int64, _ int) ([]int64, int, error) {
	return nil, m.pendingRows, nil
}
func (m *mockCommitImportStore) RowIDsForFilter(_ context.Context, _ int64, _ RowFilter, _ int) ([]int64, error) {
	return nil, nil
}
func (m *mockCommitImportStore) StagedRowsForCommit(_ context.Context, _ int64) ([]*RowOutcome, error) {
	return m.stagedRows, nil
}
func (m *mockCommitImportStore) UpdateCommittedRows(_ context.Context, _ int64, rows []RowOutcome) error {
	m.committedRows = append(m.committedRows, rows...)
	return nil
}
func (m *mockCommitImportStore) Sweep(_ context.Context) error { return nil }

// mockCommitCatalogPort mocks CatalogPort for testing.
type mockCommitCatalogPort struct {
	keys              []catalog.VariantKey
	writtenVariants   []catalog.VariantWriteRow
	deactivatedExcept []int64
	retired           []catalog.RetiredVariant
	nextVariantID     int64
}

func (m *mockCommitCatalogPort) ListMatchProducts(_ context.Context) ([]catalog.MatchProduct, error) {
	return nil, nil
}
func (m *mockCommitCatalogPort) ImportVocabulary(_ context.Context, _ int64) (catalog.EnrichVocabulary, error) {
	return catalog.EnrichVocabulary{}, nil
}
func (m *mockCommitCatalogPort) ListVariantKeys(_ context.Context, _ int64) ([]catalog.VariantKey, error) {
	return m.keys, nil
}
func (m *mockCommitCatalogPort) BulkWriteVariants(_ context.Context, _ int64, rows []catalog.VariantWriteRow) (catalog.VariantWriteResult, error) {
	res := catalog.VariantWriteResult{
		IDs:          make(map[int]int64, len(rows)),
		InsertedRefs: make(map[int]bool, len(rows)),
	}
	for _, r := range rows {
		m.writtenVariants = append(m.writtenVariants, r)
		if r.Variant.ID > 0 {
			res.IDs[r.Ref] = r.Variant.ID
			res.Updated++
		} else {
			m.nextVariantID++
			res.IDs[r.Ref] = m.nextVariantID
			res.InsertedRefs[r.Ref] = true
			res.Inserted++
		}
	}
	return res, nil
}
func (m *mockCommitCatalogPort) RetireVariantsExcept(
	_ context.Context, _ int64, _ int64, keep []int64,
) ([]catalog.RetiredVariant, error) {
	m.deactivatedExcept = keep
	kept := make(map[int64]bool, len(keep))
	for _, id := range keep {
		kept[id] = true
	}
	var out []catalog.RetiredVariant
	for _, k := range m.keys {
		if !kept[k.ID] {
			out = append(out, catalog.RetiredVariant{ID: k.ID, ProductID: k.ProductID})
		}
	}
	m.retired = out
	return out, nil
}
func (m *mockCommitCatalogPort) GetProduct(_ context.Context, _ int64) (*catalog.Product, []*catalog.ProductVariant, error) {
	return nil, nil, nil
}
func (m *mockCommitCatalogPort) Search(_ context.Context, _ catalog.SearchParams) ([]*catalog.Product, error) {
	return nil, nil
}

// mockCommitInventoryPort mocks InventoryPort for testing.
type mockCommitInventoryPort struct {
	warehouses  []*inventory.Warehouse
	writtenRows []inventory.StockWriteRow
	lastMode    inventory.StockMode
	// inWarehouse is the set of variants already holding a balance in the
	// warehouse an import is writing to.
	inWarehouse map[int64]bool
}

func (m *mockCommitInventoryPort) ListWarehouses(_ context.Context) ([]*inventory.Warehouse, error) {
	return m.warehouses, nil
}
func (m *mockCommitInventoryPort) BulkWriteStocks(_ context.Context, mode inventory.StockMode, rows []inventory.StockWriteRow) (inventory.StockWriteResult, error) {
	m.lastMode = mode
	m.writtenRows = append(m.writtenRows, rows...)
	return inventory.StockWriteResult{Written: len(rows)}, nil
}
func (m *mockCommitInventoryPort) ClearWarehouseStocks(_ context.Context, _ int64) error {
	return nil
}
func (m *mockCommitInventoryPort) VariantIDsInWarehouse(_ context.Context, _ int64) (map[int64]bool, error) {
	if m.inWarehouse == nil {
		return map[int64]bool{}, nil
	}
	return m.inWarehouse, nil
}

func TestCommitImportModes(t *testing.T) {
	ctx := context.Background()

	p1 := int64(101)
	p2 := int64(102)

	// Existing variant in catalog for product 101
	existingVariantKey := catalog.VariantKey{
		ID:        501,
		ProductID: p1,
		SKU:       "SKU-EXISTS",
	}

	staged := []*RowOutcome{
		{
			ID:         1,
			SourceRow:  1,
			ProductID:  &p1,
			SourceCode: "SKU-EXISTS",
			Payload: &productmatch.Row{
				Number:      1,
				SKU:         "SKU-EXISTS",
				Name:        "Product 1 Existing",
				PublicPrice: money.FromMajor(100),
				HasQuantity: true,
				Quantity:    15,
			},
		},
		{
			ID:         2,
			SourceRow:  2,
			ProductID:  &p2,
			SourceCode: "SKU-NEW",
			Payload: &productmatch.Row{
				Number:      2,
				SKU:         "SKU-NEW",
				Name:        "Product 2 New",
				PublicPrice: money.FromMajor(50),
				HasQuantity: true,
				Quantity:    30,
			},
		},
	}

	t.Run("ModeUpsert writes both existing and new variants", func(t *testing.T) {
		catMock := &mockCommitCatalogPort{
			keys:          []catalog.VariantKey{existingVariantKey},
			nextVariantID: 1000,
		}
		invMock := &mockCommitInventoryPort{
			warehouses: []*inventory.Warehouse{{ID: 1, OrganizationID: 10}},
		}
		storeMock := &mockCommitImportStore{
			session: &Session{
				ID:             1,
				PublicID:       "imp-upsert",
				OrganizationID: 10,
				Phase:          PhaseReview,
				Settings: Settings{
					WarehouseID: 1,
					Mode:        ModeUpsert,
					StockMode:   inventory.StockReplace,
				},
			},
			stagedRows: staged,
		}

		svc := NewService(nil, slog.Default())
		svc.SetImportStore(storeMock)
		svc.SetCatalogPort(catMock)
		svc.SetInventoryPort(invMock)

		sess, err := svc.CommitImport(ctx, "imp-upsert")
		if err != nil {
			t.Fatalf("CommitImport failed: %v", err)
		}

		if sess.InsertedRows != 1 {
			t.Errorf("expected 1 inserted row, got %d", sess.InsertedRows)
		}
		if sess.UpdatedRows != 1 {
			t.Errorf("expected 1 updated row, got %d", sess.UpdatedRows)
		}
		if sess.SkippedRows != 0 {
			t.Errorf("expected 0 skipped rows, got %d", sess.SkippedRows)
		}
		if len(catMock.writtenVariants) != 2 {
			t.Errorf("expected 2 variants written, got %d", len(catMock.writtenVariants))
		}
		if len(invMock.writtenRows) != 2 {
			t.Errorf("expected 2 stock rows written, got %d", len(invMock.writtenRows))
		}
	})

	t.Run("ModeAddOnly writes only new variants and skips existing", func(t *testing.T) {
		catMock := &mockCommitCatalogPort{
			keys:          []catalog.VariantKey{existingVariantKey},
			nextVariantID: 1000,
		}
		invMock := &mockCommitInventoryPort{
			warehouses: []*inventory.Warehouse{{ID: 1, OrganizationID: 10}},
		}
		storeMock := &mockCommitImportStore{
			session: &Session{
				ID:             2,
				PublicID:       "imp-add-only",
				OrganizationID: 10,
				Phase:          PhaseReview,
				Settings: Settings{
					WarehouseID: 1,
					Mode:        ModeAddOnly,
					StockMode:   inventory.StockReplace,
				},
			},
			stagedRows: staged,
		}

		svc := NewService(nil, slog.Default())
		svc.SetImportStore(storeMock)
		svc.SetCatalogPort(catMock)
		svc.SetInventoryPort(invMock)

		sess, err := svc.CommitImport(ctx, "imp-add-only")
		if err != nil {
			t.Fatalf("CommitImport failed: %v", err)
		}

		if sess.InsertedRows != 1 {
			t.Errorf("expected 1 inserted row, got %d", sess.InsertedRows)
		}
		if sess.UpdatedRows != 0 {
			t.Errorf("expected 0 updated rows, got %d", sess.UpdatedRows)
		}
		if sess.SkippedRows != 1 {
			t.Errorf("expected 1 skipped row, got %d", sess.SkippedRows)
		}
		if len(catMock.writtenVariants) != 1 {
			t.Errorf("expected 1 variant written, got %d", len(catMock.writtenVariants))
		}
		// Verify written variant is the NEW one
		if catMock.writtenVariants[0].Variant.SKU != "SKU-NEW" {
			t.Errorf("expected written variant to be SKU-NEW, got %s", catMock.writtenVariants[0].Variant.SKU)
		}
	})

	t.Run("ModeUpdateOnly updates only existing variants and skips new", func(t *testing.T) {
		catMock := &mockCommitCatalogPort{
			keys:          []catalog.VariantKey{existingVariantKey},
			nextVariantID: 1000,
		}
		invMock := &mockCommitInventoryPort{
			warehouses: []*inventory.Warehouse{{ID: 1, OrganizationID: 10}},
		}
		storeMock := &mockCommitImportStore{
			session: &Session{
				ID:             3,
				PublicID:       "imp-update-only",
				OrganizationID: 10,
				Phase:          PhaseReview,
				Settings: Settings{
					WarehouseID: 1,
					Mode:        ModeUpdateOnly,
					StockMode:   inventory.StockReplace,
				},
			},
			stagedRows: staged,
		}

		svc := NewService(nil, slog.Default())
		svc.SetImportStore(storeMock)
		svc.SetCatalogPort(catMock)
		svc.SetInventoryPort(invMock)

		sess, err := svc.CommitImport(ctx, "imp-update-only")
		if err != nil {
			t.Fatalf("CommitImport failed: %v", err)
		}

		if sess.InsertedRows != 0 {
			t.Errorf("expected 0 inserted rows, got %d", sess.InsertedRows)
		}
		if sess.UpdatedRows != 1 {
			t.Errorf("expected 1 updated row, got %d", sess.UpdatedRows)
		}
		if sess.SkippedRows != 1 {
			t.Errorf("expected 1 skipped row, got %d", sess.SkippedRows)
		}
		if len(catMock.writtenVariants) != 1 {
			t.Errorf("expected 1 variant written, got %d", len(catMock.writtenVariants))
		}
		// Verify written variant is the EXISTING one (ID = 501)
		if catMock.writtenVariants[0].Variant.ID != 501 {
			t.Errorf("expected written variant ID to be 501, got %d", catMock.writtenVariants[0].Variant.ID)
		}
	})

	t.Run("ModeReplace deactivates unmentioned variants", func(t *testing.T) {
		catMock := &mockCommitCatalogPort{
			keys:          []catalog.VariantKey{existingVariantKey},
			nextVariantID: 1000,
		}
		invMock := &mockCommitInventoryPort{
			warehouses: []*inventory.Warehouse{{ID: 1, OrganizationID: 10}},
		}
		storeMock := &mockCommitImportStore{
			session: &Session{
				ID:             4,
				PublicID:       "imp-replace",
				OrganizationID: 10,
				Phase:          PhaseReview,
				Settings: Settings{
					WarehouseID: 1,
					Mode:        ModeReplace,
					StockMode:   inventory.StockReplace,
				},
			},
			stagedRows: staged,
		}

		svc := NewService(nil, slog.Default())
		svc.SetImportStore(storeMock)
		svc.SetCatalogPort(catMock)
		svc.SetInventoryPort(invMock)

		sess, err := svc.CommitImport(ctx, "imp-replace")
		if err != nil {
			t.Fatalf("CommitImport failed: %v", err)
		}

		if sess.InsertedRows != 1 || sess.UpdatedRows != 1 {
			t.Errorf("expected 1 inserted and 1 updated, got %d ins, %d upd", sess.InsertedRows, sess.UpdatedRows)
		}
		// Verify DeactivateVariantsExcept was called with touched variants
		if len(catMock.deactivatedExcept) != 2 {
			t.Errorf("expected 2 kept variants in DeactivateVariantsExcept, got %d", len(catMock.deactivatedExcept))
		}
	})

	t.Run("StockWriting handles BlankQuantityIsZero and StockMode", func(t *testing.T) {
		catMock := &mockCommitCatalogPort{
			keys:          []catalog.VariantKey{},
			nextVariantID: 2000,
		}
		invMock := &mockCommitInventoryPort{
			warehouses: []*inventory.Warehouse{{ID: 1, OrganizationID: 10}},
		}
		// Row with NO quantity specified
		blankQtyRow := []*RowOutcome{
			{
				ID:         10,
				SourceRow:  1,
				ProductID:  &p1,
				SourceCode: "SKU-BLANK-QTY",
				Payload: &productmatch.Row{
					Number:      1,
					SKU:         "SKU-BLANK-QTY",
					Name:        "Product Without Quantity",
					PublicPrice: money.FromMajor(120),
					HasQuantity: false,
					Quantity:    0,
				},
			},
		}

		storeMock := &mockCommitImportStore{
			session: &Session{
				ID:             5,
				PublicID:       "imp-blank-qty",
				OrganizationID: 10,
				Phase:          PhaseReview,
				Settings: Settings{
					WarehouseID:         1,
					Mode:                ModeUpsert,
					StockMode:           inventory.StockReplace,
					BlankQuantityIsZero: true,
				},
			},
			stagedRows: blankQtyRow,
		}

		svc := NewService(nil, slog.Default())
		svc.SetImportStore(storeMock)
		svc.SetCatalogPort(catMock)
		svc.SetInventoryPort(invMock)

		sess, err := svc.CommitImport(ctx, "imp-blank-qty")
		if err != nil {
			t.Fatalf("CommitImport failed: %v", err)
		}

		if sess.InsertedRows != 1 {
			t.Errorf("expected 1 inserted row, got %d", sess.InsertedRows)
		}
		// Verify stock row was queued with quantity 0 and HasQuantity = true
		if len(invMock.writtenRows) != 1 {
			t.Fatalf("expected 1 stock row written, got %d", len(invMock.writtenRows))
		}
		if invMock.writtenRows[0].Stock.Quantity != 0 || !invMock.writtenRows[0].HasQuantity {
			t.Errorf("expected stock row quantity 0 with HasQuantity true, got qty=%d hasQty=%v",
				invMock.writtenRows[0].Stock.Quantity, invMock.writtenRows[0].HasQuantity)
		}
	})
}
