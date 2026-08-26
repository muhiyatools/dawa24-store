package catalog_test

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// memoryImportStore is an in-memory ImportSessionStore.
//
// It exists so the three-step flow — analyse, prepare, commit — can be driven
// without a database, and because implementing the port twice is the cheapest
// check that it is actually an abstraction and not a description of one
// PostgreSQL schema.
type memoryImportStore struct {
	session  *catalog.ImportSession
	file     []byte
	rows     []*catalog.StagingRow
	existing map[string]int64 // folded name or SKU -> product id
	// catalogue backs the similarity tier. It is empty unless a test populates
	// it, so tests written before that tier existed still exercise exactly the
	// exact-identifier path they were written for.
	catalogue []catalog.MatchProduct
	vocab     catalog.EnrichVocabulary
	archived  int64
	written   []*catalog.Product
	nextID    int64
}

func newMemoryStore() *memoryImportStore {
	return &memoryImportStore{
		existing: map[string]int64{},
		nextID:   1,
	}
}

func (m *memoryImportStore) CreateImportSession(_ context.Context, s *catalog.ImportSession, file []byte) error {
	s.ID, s.PublicID = 1, "11111111-1111-1111-1111-111111111111"
	m.session, m.file = s, file
	return nil
}

func (m *memoryImportStore) GetImportSession(_ context.Context, publicID string) (*catalog.ImportSession, error) {
	if m.session == nil || m.session.PublicID != publicID {
		return nil, fmt.Errorf("no such session %q", publicID)
	}
	return m.session, nil
}

func (m *memoryImportStore) UpdateImportSession(
	_ context.Context, s *catalog.ImportSession, _ ...catalog.SessionStatus,
) error {
	m.session = s
	return nil
}

func (m *memoryImportStore) ClaimImportSessionForCommit(
	_ context.Context, publicID string,
) (*catalog.ImportSession, error) {
	if m.session == nil || m.session.PublicID != publicID {
		return nil, fmt.Errorf("no such session %q", publicID)
	}
	if !m.session.IsReviewable() {
		return nil, fmt.Errorf("session %s is not reviewable (status %s)", publicID, m.session.Status)
	}
	m.session.Status = catalog.SessionCommitting
	return m.session, nil
}

func (m *memoryImportStore) ImportSourceFile(context.Context, int64) ([]byte, error) {
	return m.file, nil
}

func (m *memoryImportStore) ReleaseImportSourceFile(context.Context, int64) error {
	m.file = nil
	return nil
}

func (m *memoryImportStore) ListRecentImportSessions(context.Context, int64, int) ([]*catalog.ImportSession, error) {
	return nil, nil
}

func (m *memoryImportStore) ReplaceStagingRows(_ context.Context, _ int64, rows []*catalog.StagingRow) error {
	m.rows = rows
	for _, row := range rows {
		row.ID = m.nextID
		m.nextID++
	}
	return nil
}

func (m *memoryImportStore) ClearStagingRows(context.Context, int64) error {
	m.rows = nil
	return nil
}

func (m *memoryImportStore) ListStagingRows(
	context.Context, int64, catalog.StagingFilter,
) ([]*catalog.StagingRow, int, error) {
	return m.rows, len(m.rows), nil
}

func (m *memoryImportStore) LoadCommittableRows(context.Context, int64) ([]*catalog.StagingRow, error) {
	var out []*catalog.StagingRow
	for _, row := range m.rows {
		if row.Included && row.Action != catalog.ActionSkip && !row.HasErrors() {
			out = append(out, row)
		}
	}
	return out, nil
}

func (m *memoryImportStore) GetStagingRow(_ context.Context, _, rowID int64) (*catalog.StagingRow, error) {
	for _, row := range m.rows {
		if row.ID == rowID {
			return row, nil
		}
	}
	return nil, fmt.Errorf("row not found: %d", rowID)
}

func (m *memoryImportStore) SetRowIncluded(_ context.Context, _, rowID int64, included bool) error {
	for _, row := range m.rows {
		if row.ID == rowID {
			row.Included = included
		}
	}
	return nil
}

func (m *memoryImportStore) SetRowsIncludedByAction(
	_ context.Context, _ int64, action catalog.RowAction, included bool,
) (int64, error) {
	var n int64
	for _, row := range m.rows {
		if action == "" || row.Action == action {
			row.Included = included
			n++
		}
	}
	return n, nil
}

func (m *memoryImportStore) CountStagingActions(context.Context, int64) (catalog.StagingCounts, error) {
	var counts catalog.StagingCounts
	for _, row := range m.rows {
		counts.Total++
		switch {
		case !row.Included || row.Action == catalog.ActionSkip:
			counts.Skip++
		case row.Action == catalog.ActionInsert:
			counts.Insert++
		case row.Action == catalog.ActionUpdate:
			counts.Update++
		}
		if len(row.AIChanges) > 0 {
			counts.AIChanged++
		}
	}
	return counts, nil
}

func (m *memoryImportStore) DefaultCatalogOrg(context.Context) (int64, error) { return 50, nil }

func (m *memoryImportStore) MatchExistingProducts(
	_ context.Context, prods []*catalog.Product,
) (map[int]catalog.ExistingMatch, error) {
	out := map[int]catalog.ExistingMatch{}
	for i, p := range prods {
		if id, ok := m.existing[strings.ToLower(p.SKU)]; ok && p.SKU != "" {
			out[i] = catalog.ExistingMatch{ProductID: id, Reason: catalog.MatchSKU}
			continue
		}
		if id, ok := m.existing[catalog.NormalizeName(p.Name.Get(i18n.AR))]; ok {
			out[i] = catalog.ExistingMatch{ProductID: id, Reason: catalog.MatchName}
		}
	}
	return out, nil
}

// ListMatchProducts backs the similarity tier. The double returns the same
// products the exact matcher knows about, so a test that expects a row to match
// exactly still does, and one that expects no similarity match gets none.
func (m *memoryImportStore) ListMatchProducts(context.Context) ([]catalog.MatchProduct, error) {
	out := make([]catalog.MatchProduct, 0, len(m.catalogue))
	for _, p := range m.catalogue {
		out = append(out, p)
	}
	return out, nil
}

func (m *memoryImportStore) ImportVocabulary(context.Context, int64) (catalog.EnrichVocabulary, error) {
	return m.vocab, nil
}

func (m *memoryImportStore) BulkCommitProducts(
	_ context.Context, prods []*catalog.Product, _ catalog.BulkWriteOptions, archiveOrg int64,
) (int64, catalog.BulkWriteResult, error) {
	archived := int64(0)
	if archiveOrg > 0 {
		m.archived++
		archived = 7
	}
	m.written = append(m.written, prods...)
	res := catalog.BulkWriteResult{Matches: map[int]catalog.MatchReason{}}
	for _, p := range prods {
		if p.ID > 0 {
			res.Updated++
			continue
		}
		res.Inserted++
	}
	return archived, res, nil
}

// mockCatalogRepoStub implements dummy methods for catalog.Repository.
type mockCatalogRepoStub struct{}

func (mockCatalogRepoStub) CreateProduct(context.Context, *catalog.Product) error { return nil }
func (mockCatalogRepoStub) BulkUpsertProducts(context.Context, []*catalog.Product) (catalog.BulkWriteResult, error) {
	return catalog.BulkWriteResult{}, nil
}
func (mockCatalogRepoStub) GetProductByID(context.Context, int64) (*catalog.Product, error) {
	return nil, nil
}
func (mockCatalogRepoStub) UpdateProduct(context.Context, *catalog.Product) error { return nil }
func (mockCatalogRepoStub) DeleteProduct(context.Context, int64) error            { return nil }
func (mockCatalogRepoStub) SearchProducts(context.Context, catalog.SearchParams) ([]*catalog.Product, error) {
	return nil, nil
}
func (mockCatalogRepoStub) CountProducts(context.Context, catalog.SearchParams) (int, error) {
	return 0, nil
}
func (mockCatalogRepoStub) ListProducts(context.Context, string, int, int) ([]*catalog.Product, error) {
	return nil, nil
}
func (mockCatalogRepoStub) SetProductsStatus(context.Context, []int64, catalog.ProductStatus) (int64, error) {
	return 0, nil
}
func (mockCatalogRepoStub) CreateVariant(context.Context, *catalog.ProductVariant) error { return nil }
func (mockCatalogRepoStub) GetVariantByID(context.Context, int64) (*catalog.ProductVariant, error) {
	return nil, nil
}
func (mockCatalogRepoStub) GetVariantBySKUOrBarcode(context.Context, int64, string, string) (*catalog.ProductVariant, error) {
	return nil, nil
}
func (mockCatalogRepoStub) GetVariantByProductAndOrg(context.Context, int64, int64) (*catalog.ProductVariant, error) {
	return nil, nil
}
func (mockCatalogRepoStub) ListVariantsByProduct(context.Context, int64) ([]*catalog.ProductVariant, error) {
	return nil, nil
}
func (mockCatalogRepoStub) ListVariantsByProducts(context.Context, []int64) ([]*catalog.ProductVariant, error) {
	return nil, nil
}
func (mockCatalogRepoStub) ListVariantsByOrganization(context.Context, int64, catalog.VariantSearchParams) ([]*catalog.ProductVariant, int, error) {
	return nil, 0, nil
}
func (mockCatalogRepoStub) ListAllVariants(context.Context, catalog.VariantSearchParams) ([]*catalog.ProductVariant, int, error) {
	return nil, 0, nil
}
func (mockCatalogRepoStub) UpdateVariant(context.Context, *catalog.ProductVariant) error { return nil }
func (mockCatalogRepoStub) DeleteVariant(context.Context, int64) error                   { return nil }
func (mockCatalogRepoStub) SearchVariants(context.Context, catalog.VariantSearchParams) ([]*catalog.ProductVariant, error) {
	return nil, nil
}
func (mockCatalogRepoStub) SetVariantsStatus(context.Context, []int64, catalog.ProductStatus) (int64, error) {
	return 0, nil
}
func (mockCatalogRepoStub) CreateCategory(context.Context, *catalog.Category) error { return nil }
func (mockCatalogRepoStub) GetCategoryByID(context.Context, int64) (*catalog.Category, error) {
	return nil, nil
}
func (mockCatalogRepoStub) ListCategories(context.Context) ([]*catalog.Category, error) {
	return nil, nil
}
func (mockCatalogRepoStub) UpdateCategory(context.Context, *catalog.Category) error { return nil }
func (mockCatalogRepoStub) DeleteCategory(context.Context, int64) error             { return nil }
func (mockCatalogRepoStub) CountProductsByOrg(context.Context, int64, string) (int, error) {
	return 0, nil
}
func (mockCatalogRepoStub) CountProductsInCategory(context.Context, int64) (int, error) {
	return 0, nil
}
func (mockCatalogRepoStub) CreateBrand(context.Context, *catalog.Brand) error { return nil }
func (mockCatalogRepoStub) GetBrandByID(context.Context, int64) (*catalog.Brand, error) {
	return nil, nil
}
func (mockCatalogRepoStub) ListBrands(context.Context) ([]*catalog.Brand, error) { return nil, nil }
func (mockCatalogRepoStub) UpdateBrand(context.Context, *catalog.Brand) error    { return nil }
func (mockCatalogRepoStub) DeleteBrand(context.Context, int64) error             { return nil }
func (mockCatalogRepoStub) ListBrandsByCategory(context.Context, int64) ([]*catalog.Brand, error) {
	return nil, nil
}
func (mockCatalogRepoStub) BrandInCategory(context.Context, int64, int64) (bool, error) {
	return true, nil
}
func (mockCatalogRepoStub) SetBrandCategories(context.Context, int64, []int64) error { return nil }
func (mockCatalogRepoStub) CountProductsInBrand(context.Context, int64) (int, error) { return 0, nil }
func (mockCatalogRepoStub) SetCustomerPricing(context.Context, *catalog.CustomerProductMapping) error {
	return nil
}
func (mockCatalogRepoStub) GetCustomerPricing(context.Context, int64, int64, int64) (*catalog.CustomerProductMapping, error) {
	return nil, nil
}
func (mockCatalogRepoStub) CreateProductAlert(context.Context, *catalog.ProductAlert) error {
	return nil
}
func (mockCatalogRepoStub) ListProductAlertsByUser(context.Context, int64) ([]*catalog.ProductAlert, error) {
	return nil, nil
}
func (mockCatalogRepoStub) UpsertProductIndex(context.Context, *catalog.ProductIndexItem) error {
	return nil
}
func (mockCatalogRepoStub) DeleteProductIndex(context.Context, string) error         { return nil }
func (mockCatalogRepoStub) DeleteProductIndexByProduct(context.Context, int64) error { return nil }
func (mockCatalogRepoStub) SearchProductIndex(context.Context, catalog.SearchParams) ([]*catalog.ProductIndexItem, error) {
	return nil, nil
}
func (mockCatalogRepoStub) RebuildProductIndex(context.Context) (int64, error) { return 0, nil }
func (mockCatalogRepoStub) CreateSavingProduct(context.Context, *catalog.SavingProduct) error {
	return nil
}
func (mockCatalogRepoStub) UpdateSavingProduct(context.Context, *catalog.SavingProduct) error {
	return nil
}
func (mockCatalogRepoStub) ListSavingProductsByOrg(context.Context, int64, int, int) ([]*catalog.SavingProduct, error) {
	return nil, nil
}
func (mockCatalogRepoStub) ListSavingProductsEnriched(context.Context, int64, string, string, int, int) ([]*catalog.SavingProductEnriched, *catalog.SavingProductStats, error) {
	return nil, nil, nil
}
func (mockCatalogRepoStub) ListAllSavingProductsAdmin(context.Context, *int64, *int64, string, string, int, int) ([]*catalog.SavingProductAdminView, *catalog.SavingProductAdminStats, error) {
	return nil, nil, nil
}
func (mockCatalogRepoStub) GetSavingProductByID(context.Context, int64) (*catalog.SavingProduct, error) {
	return nil, nil
}
func (mockCatalogRepoStub) DeleteSavingProduct(context.Context, int64, int64) error { return nil }
func (mockCatalogRepoStub) GetProductProviders(context.Context, int64) ([]*catalog.ProductProviderInfo, error) {
	return nil, nil
}
func (mockCatalogRepoStub) BatchUpsertSavingProducts(context.Context, int64, *int64, []*catalog.SavingProduct) (int, int, error) {
	return 0, 0, nil
}
func (mockCatalogRepoStub) ListAllMasterProductsForMatching(context.Context) ([]*catalog.CatalogMatchSource, error) {
	return nil, nil
}

// stagingRepo is the minimal catalog.Repository the commit path needs.
type stagingRepo struct {
	mockCatalogRepoStub
	written []*catalog.Product
}

func (r *stagingRepo) BulkUpsertProducts(
	_ context.Context, prods []*catalog.Product, _ catalog.BulkWriteOptions,
) (catalog.BulkWriteResult, error) {
	r.written = append(r.written, prods...)
	res := catalog.BulkWriteResult{Matches: map[int]catalog.MatchReason{}}
	for _, p := range prods {
		if p.ID > 0 {
			res.Updated++
			continue
		}
		res.Inserted++
	}
	return res, nil
}

func newImportService(t *testing.T, store *memoryImportStore) (*catalog.Service, *stagingRepo) {
	t.Helper()
	repo := &stagingRepo{}
	svc := catalog.NewService(repo, slog.New(slog.NewTextHandler(io.Discard, nil)))
	svc.SetImportStore(store)
	return svc, repo
}

const serviceFixture = "اسم الصنف,كود الصنف,سعر البيع\n" +
	"بانادول اكسترا,SVC-1,55.00\n" +
	"كتافلام أقراص,SVC-2,42.00\n" +
	"اوجمنتين حقن,SVC-3,115.00\n"

// Analysing and preparing must leave the catalogue untouched. That property is
// the entire justification for the review step.
func TestPrepareImportWritesNothingToTheCatalogue(t *testing.T) {
	store := newMemoryStore()
	svc, repo := newImportService(t, store)
	ctx := context.Background()

	session, parsed, err := svc.AnalyzeImport(ctx, []byte(serviceFixture), "list.csv", 7)
	if err != nil {
		t.Fatalf("analyse failed: %v", err)
	}
	if len(parsed.Products) != 3 {
		t.Fatalf("parsed %d products, want 3", len(parsed.Products))
	}

	if _, err := svc.PrepareImport(ctx, session.PublicID, catalog.ImportSettings{
		Mode: catalog.ModeUpdateAndAdd, Options: catalog.DefaultImportOptions(),
	}); err != nil {
		t.Fatalf("prepare failed: %v", err)
	}

	if len(store.written) != 0 {
		t.Fatalf("preparing wrote %d products to the catalogue; it must write none", len(store.written))
	}
	if len(repo.written) != 0 {
		t.Fatalf("preparing wrote %d products via the catalogue repo; it must write none", len(repo.written))
	}
	if len(store.rows) != 3 {
		t.Fatalf("staged %d rows, want 3", len(store.rows))
	}
	if store.session.Status != catalog.SessionReady {
		t.Errorf("status = %q, want ready", store.session.Status)
	}
}

func TestPrepareImportAssignsActionsPerMode(t *testing.T) {
	tests := []struct {
		mode                 catalog.ImportMode
		insert, update, skip int
	}{
		{catalog.ModeUpdateAndAdd, 2, 1, 0},
		{catalog.ModeAddNewOnly, 2, 0, 1},
		{catalog.ModeUpdateExistingOnly, 0, 1, 2},
		{catalog.ModeClearAndAdd, 3, 0, 0},
	}

	for _, tc := range tests {
		t.Run(string(tc.mode), func(t *testing.T) {
			store := newMemoryStore()
			// One of the three already exists in the catalogue.
			store.existing["svc-1"] = 4812
			svc, _ := newImportService(t, store)
			ctx := context.Background()

			session, _, err := svc.AnalyzeImport(ctx, []byte(serviceFixture), "list.csv", 0)
			if err != nil {
				t.Fatalf("analyse failed: %v", err)
			}
			prepared, err := svc.PrepareImport(ctx, session.PublicID, catalog.ImportSettings{
				Mode: tc.mode, Options: catalog.DefaultImportOptions(),
			})
			if err != nil {
				t.Fatalf("prepare failed: %v", err)
			}

			if prepared.InsertRows != tc.insert || prepared.UpdateRows != tc.update || prepared.SkipRows != tc.skip {
				t.Errorf("insert=%d update=%d skip=%d; want %d, %d and %d",
					prepared.InsertRows, prepared.UpdateRows, prepared.SkipRows,
					tc.insert, tc.update, tc.skip)
			}
		})
	}
}

func TestCommitImportArchivesFirstOnlyForClearAndAdd(t *testing.T) {
	for _, mode := range []catalog.ImportMode{catalog.ModeUpdateAndAdd, catalog.ModeClearAndAdd} {
		t.Run(string(mode), func(t *testing.T) {
			store := newMemoryStore()
			svc, _ := newImportService(t, store)
			ctx := context.Background()

			session, _, _ := svc.AnalyzeImport(ctx, []byte(serviceFixture), "list.csv", 0)
			if _, err := svc.PrepareImport(ctx, session.PublicID, catalog.ImportSettings{
				Mode: mode, Options: catalog.DefaultImportOptions(),
			}); err != nil {
				t.Fatalf("prepare failed: %v", err)
			}

			committed, result, err := svc.CommitImport(ctx, session.PublicID)
			if err != nil {
				t.Fatalf("commit failed: %v", err)
			}
			if committed.Status != catalog.SessionCommitted {
				t.Errorf("status = %q, want committed", committed.Status)
			}
			if result.Total() != 3 {
				t.Errorf("wrote %d products, want 3", result.Total())
			}
			if len(store.written) != 3 {
				t.Errorf("repository received %d products, want 3", len(store.written))
			}

			wantArchive := int64(0)
			if mode == catalog.ModeClearAndAdd {
				wantArchive = 1
			}
			if store.archived != wantArchive {
				t.Errorf("archived %d times, want %d for mode %q", store.archived, wantArchive, mode)
			}
		})
	}
}

func TestCommitImportSkipsDeselectedRows(t *testing.T) {
	store := newMemoryStore()
	svc, _ := newImportService(t, store)
	ctx := context.Background()

	session, _, _ := svc.AnalyzeImport(ctx, []byte(serviceFixture), "list.csv", 0)
	if _, err := svc.PrepareImport(ctx, session.PublicID, catalog.ImportSettings{
		Mode: catalog.ModeUpdateAndAdd, Options: catalog.DefaultImportOptions(),
	}); err != nil {
		t.Fatalf("prepare failed: %v", err)
	}

	if err := svc.SetRowIncluded(ctx, session.PublicID, store.rows[1].ID, false); err != nil {
		t.Fatalf("deselect failed: %v", err)
	}

	if _, result, err := svc.CommitImport(ctx, session.PublicID); err != nil {
		t.Fatalf("commit failed: %v", err)
	} else if result.Total() != 2 {
		t.Errorf("wrote %d products, want 2", result.Total())
	}
	if len(store.written) != 2 {
		t.Errorf("repository received %d products, want 2", len(store.written))
	}
}

func TestCommitImportRefusesASecondTime(t *testing.T) {
	store := newMemoryStore()
	svc, _ := newImportService(t, store)
	ctx := context.Background()

	session, _, _ := svc.AnalyzeImport(ctx, []byte(serviceFixture), "list.csv", 0)
	if _, err := svc.PrepareImport(ctx, session.PublicID, catalog.ImportSettings{
		Mode: catalog.ModeUpdateAndAdd, Options: catalog.DefaultImportOptions(),
	}); err != nil {
		t.Fatalf("prepare failed: %v", err)
	}
	if _, _, err := svc.CommitImport(ctx, session.PublicID); err != nil {
		t.Fatalf("first commit failed: %v", err)
	}

	// A double submit must not import the same file twice.
	if _, _, err := svc.CommitImport(ctx, session.PublicID); err == nil {
		t.Fatal("a committed session was committed again")
	}
	if len(store.written) != 3 {
		t.Errorf("repository received %d products, want 3 — the second commit must write nothing", len(store.written))
	}
}

func TestCancelImportPreventsCommit(t *testing.T) {
	store := newMemoryStore()
	svc, _ := newImportService(t, store)
	ctx := context.Background()

	session, _, _ := svc.AnalyzeImport(ctx, []byte(serviceFixture), "list.csv", 0)
	if _, err := svc.PrepareImport(ctx, session.PublicID, catalog.ImportSettings{
		Mode: catalog.ModeUpdateAndAdd, Options: catalog.DefaultImportOptions(),
	}); err != nil {
		t.Fatalf("prepare failed: %v", err)
	}
	if err := svc.CancelImport(ctx, session.PublicID); err != nil {
		t.Fatalf("cancel failed: %v", err)
	}

	if _, _, err := svc.CommitImport(ctx, session.PublicID); err == nil {
		t.Fatal("a cancelled session was committed")
	}
	if len(store.written) != 0 {
		t.Errorf("repository received %d products after a cancel, want 0", len(store.written))
	}
}

// AI is an enhancement, never a dependency. A Gateway that is down must leave
// the deterministic values in place and let the import finish.
func TestPrepareImportSurvivesAnUnavailableMapper(t *testing.T) {
	store := newMemoryStore()
	store.vocab = testVocabulary()
	svc, _ := newImportService(t, store)
	svc.SetAIMapper(&stubMapper{available: true, fail: errGatewayDown})
	ctx := context.Background()

	session, _, _ := svc.AnalyzeImport(ctx, []byte(serviceFixture), "list.csv", 0)
	prepared, err := svc.PrepareImport(ctx, session.PublicID, catalog.ImportSettings{
		Mode: catalog.ModeUpdateAndAdd,
		Options: catalog.ImportOptions{
			UseAI: true, AssignCategory: true, AssignDosageForm: true,
		},
	})
	if err != nil {
		t.Fatalf("prepare failed when the Gateway was down: %v", err)
	}

	if prepared.Status != catalog.SessionReady {
		t.Errorf("status = %q, want ready — an unavailable model must not fail the import", prepared.Status)
	}
	if prepared.ParsedRows != 3 {
		t.Errorf("parsed %d rows, want 3", prepared.ParsedRows)
	}
	if !prepared.AIFallback {
		t.Error("the session does not record that it fell back")
	}
	// The deterministic pass still ran: every row carries a form, inferred from
	// the name where it could be and the placeholder where it could not.
	for _, row := range store.rows {
		if row.Product.DosageForm == "" {
			t.Errorf("row %d has no dosage form; the name-based rules did not run",
				row.SourceRow)
		}
	}
}

// The model must translate onto an existing value, never invent one.
func TestValueMappingRefusesInventedTargets(t *testing.T) {
	sources := []string{"اقراص مغلفه"}
	targets := []string{"أقراص", "شراب"}

	mapping := catalog.BuildValueMapping(sources, targets, catalog.ValueMapResult{
		Matches: []catalog.ValueMatch{{Source: "اقراص مغلفه", Target: "حبوب مغلفة", Confidence: 0.99}},
	})

	if _, ok := mapping.Lookup("اقراص مغلفه"); ok {
		t.Error("a value the catalogue does not have was accepted as a target")
	}
	if len(mapping.Unmatched()) != 1 {
		t.Errorf("unmatched = %v, want the source left for the admin", mapping.Unmatched())
	}
}

// Exact folding settles what it can without asking, and outranks the model.
func TestValueMappingPrefersExactFolding(t *testing.T) {
	mapping := catalog.BuildValueMapping(
		[]string{"اقراص"}, []string{"أقراص"},
		catalog.ValueMapResult{
			Matches: []catalog.ValueMatch{{Source: "اقراص", Target: "شراب", Confidence: 1}},
		})

	got, ok := mapping.Lookup("اقراص")
	if !ok || got != "أقراص" {
		t.Errorf("lookup = %q,%v; folding must win over the model", got, ok)
	}
}

// A row the parser rejected must never be committed, whatever the mode says.
func TestPrepareImportExcludesRejectedRows(t *testing.T) {
	store := newMemoryStore()
	svc, _ := newImportService(t, store)
	ctx := context.Background()

	fixture := "اسم الصنف,كود الصنف,سعر البيع\n" +
		"صنف سليم,SVC-1,55.00\n" +
		"صنف بسعر سالب,SVC-2,-15.00\n"

	session, _, _ := svc.AnalyzeImport(ctx, []byte(fixture), "list.csv", 0)
	prepared, err := svc.PrepareImport(ctx, session.PublicID, catalog.ImportSettings{
		Mode: catalog.ModeUpdateAndAdd, Options: catalog.DefaultImportOptions(),
	})
	if err != nil {
		t.Fatalf("prepare failed: %v", err)
	}
	if prepared.ParsedRows != 1 {
		t.Fatalf("staged %d rows, want 1 — the negative price must be rejected", prepared.ParsedRows)
	}

	if _, result, err := svc.CommitImport(ctx, session.PublicID); err != nil {
		t.Fatalf("commit failed: %v", err)
	} else if result.Total() != 1 {
		t.Errorf("wrote %d products, want 1", result.Total())
	}
	if len(store.written) != 1 {
		t.Errorf("repository received %d products, want 1", len(store.written))
	}
}

// blockingMapper holds a mapping call until released, so a preparation run can
// be caught mid-flight.
type blockingMapper struct{ release chan struct{} }

func (b *blockingMapper) Available(context.Context) bool { return true }

func (b *blockingMapper) MapColumns(
	context.Context, catalog.ColumnMapRequest,
) (catalog.ColumnMapResult, error) {
	return catalog.ColumnMapResult{}, nil
}

func (b *blockingMapper) MapValues(
	ctx context.Context, _ catalog.ValueMapRequest,
) (catalog.ValueMapResult, error) {
	select {
	case <-b.release:
	case <-ctx.Done():
	}
	return catalog.ValueMapResult{}, nil
}

// Preparation runs in the background, so a commit can arrive while the staging
// table is still being filled. Committing then would write a partial catalogue
// built from a partial read.
func TestCommitImportRefusesWhilePreparationIsRunning(t *testing.T) {
	store := newMemoryStore()
	store.vocab = testVocabulary()
	svc, _ := newImportService(t, store)

	blocker := &blockingMapper{release: make(chan struct{})}
	svc.SetAIMapper(blocker)
	ctx := context.Background()

	session, _, err := svc.AnalyzeImport(ctx, []byte(serviceFixture), "list.csv", 0)
	if err != nil {
		t.Fatalf("analyse failed: %v", err)
	}

	if err := svc.PrepareImportAsync(ctx, session.PublicID, catalog.ImportSettings{
		Mode: catalog.ModeUpdateAndAdd,
		Options: catalog.ImportOptions{
			UseAI: true, AssignCategory: true, AssignDosageForm: true,
		},
	}); err != nil {
		t.Fatalf("prepare could not start: %v", err)
	}

	// Wait until the run is genuinely in flight before trying to commit.
	deadline := time.Now().Add(5 * time.Second)
	for !svc.EnricherRunning(session.PublicID) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}

	if _, _, err := svc.CommitImport(ctx, session.PublicID); err == nil {
		t.Error("a session still being prepared was committed")
	}
	if len(store.written) != 0 {
		t.Errorf("a mid-preparation commit wrote %d products", len(store.written))
	}

	close(blocker.release)

	// Once it finishes, the same session commits normally.
	for i := 0; i < 200; i++ {
		if progress, ok := svc.ImportProgress(session.PublicID); ok && progress.Phase.Terminal() {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if _, _, err := svc.CommitImport(ctx, session.PublicID); err != nil {
		t.Fatalf("commit after preparation finished: %v", err)
	}
	if len(store.written) == 0 {
		t.Error("nothing was written after a successful preparation")
	}
}
