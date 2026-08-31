package ingest

import (
	"context"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"sync"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/modules/inventory"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/productmatch"
	"github.com/muhiya/dawa24-store/internal/shared/sheet"
)

// The vendor catalogue import service.
//
// It owns the order of the seven stages and nothing else. The reading is the
// engine's, the writing is the catalogue's and the inventory's, and the state
// between stages is the store's — which leaves this file free to be about the
// one thing that is genuinely hard here: making sure what the vendor confirmed
// is exactly what runs.
//
// That guarantee comes from re-deriving the analysis from the stored file at
// every stage rather than carrying a snapshot forward. The engine is
// deterministic, so re-derivation is free of surprises, and a mapping cannot
// drift between the screen that showed it and the run that used it.

// CatalogPort is the shared catalogue as this import needs it.
type CatalogPort interface {
	ListMatchProducts(ctx context.Context) ([]catalog.MatchProduct, error)
	ImportVocabulary(ctx context.Context, orgID int64) (catalog.EnrichVocabulary, error)
	ListVariantKeys(ctx context.Context, orgID int64) ([]catalog.VariantKey, error)
	BulkWriteVariants(ctx context.Context, orgID int64, rows []catalog.VariantWriteRow) (catalog.VariantWriteResult, error)
	DeactivateVariantsExcept(ctx context.Context, orgID int64, keep []int64) (int64, error)
	GetProduct(ctx context.Context, id int64) (*catalog.Product, []*catalog.ProductVariant, error)
	Search(ctx context.Context, params catalog.SearchParams) ([]*catalog.Product, error)
}

// InventoryPort is the warehouse side of the same import.
type InventoryPort interface {
	ListWarehouses(ctx context.Context) ([]*inventory.Warehouse, error)
	BulkWriteStocks(ctx context.Context, mode inventory.StockMode, rows []inventory.StockWriteRow) (inventory.StockWriteResult, error)
	ClearWarehouseStocks(ctx context.Context, warehouseID int64) error
}

// MaxImportBytes bounds an upload. It is the size of the largest real
// distributor catalogue seen — 350 KB — with two orders of magnitude of room,
// and it is a hard limit because the file is held on the session row until the
// import completes.
const MaxImportBytes = 25 << 20

// SetImportStore installs the catalogue import persistence.
func (s *Service) SetImportStore(store ImportStore) { s.imports = store }

// SetCatalogPort installs the shared catalogue this import matches against.
func (s *Service) SetCatalogPort(port CatalogPort) { s.catalog = port }

// SetInventoryPort installs the warehouse writer.
func (s *Service) SetInventoryPort(port InventoryPort) { s.inventory = port }

// ImportAvailable reports whether the vendor import screen can be offered.
func (s *Service) ImportAvailable() bool {
	return s.imports != nil && s.catalog != nil
}

// runs tracks which imports are executing in this process, so a vendor who
// double-clicks Confirm does not get two runs writing the same rows.
type runRegistry struct {
	mu     sync.Mutex
	active map[string]bool
}

func (r *runRegistry) claim(id string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.active == nil {
		r.active = map[string]bool{}
	}
	if r.active[id] {
		return false
	}
	r.active[id] = true
	return true
}

func (r *runRegistry) release(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.active, id)
}

func (r *runRegistry) running(id string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.active[id]
}

// StartImport analyses an uploaded file and opens an import for review.
//
// Nothing about the vendor's catalogue is touched. The file is stored, read as
// far as the analyser needs, and the resolved column mapping is handed back for
// the review screen — which is the first of the three stages the vendor owns.
func (s *Service) StartImport(
	ctx context.Context, userID int64, filename string, content []byte,
) (*Session, *productmatch.Analysis, error) {
	if s.imports == nil {
		return nil, nil, ErrImportStoreUnavailable
	}
	orgID, ok := database.TenantFrom(ctx)
	if !ok {
		return nil, nil, database.ErrNoTenant
	}
	if len(content) == 0 {
		return nil, nil, apperr.Validation("import.empty_file",
			i18n.TDefault("w4_mod.w4str_195_195"), nil)
	}
	if len(content) > MaxImportBytes {
		return nil, nil, apperr.Validation("import.file_too_large",
			i18n.TDefault("w4_mod.25_196"), nil)
	}

	// Reclaim abandoned sessions before adding another; the files they hold are
	// the largest thing this feature stores.
	if err := s.imports.Sweep(ctx); err != nil {
		s.log.WarnContext(ctx, "import sweep failed", "error", err)
	}

	book, err := sheet.Open(content, filename)
	if err != nil {
		return nil, nil, apperr.Validation("import.unreadable", err.Error(), nil)
	}
	defer func() { _ = book.Close() }()

	analysis, err := productmatch.Analyze(book, s.vocabulary(ctx, orgID))
	if err != nil {
		return nil, nil, apperr.Validation("import.unreadable", err.Error(), nil)
	}

	session := &Session{
		OrganizationID: orgID,
		Filename:       filename,
		FileSizeBytes:  int64(len(content)),
		Phase:          PhaseMapping,
		Source:         analysis.Source,
		Settings:       DefaultSettings(),
		TotalRows:      analysis.Source.TotalRows,
	}
	if userID > 0 {
		session.CreatedBy = &userID
	}
	if err := s.imports.Create(ctx, session, content); err != nil {
		return nil, nil, err
	}

	s.log.InfoContext(ctx, "vendor catalogue import opened",
		"import", session.PublicID, "filename", filename,
		"rows", analysis.Source.TotalRows, "columns", analysis.Layout.Width)
	return session, analysis, nil
}

// Analysis re-derives the reading of a stored file, with the vendor's own
// column corrections applied.
func (s *Service) Analysis(ctx context.Context, publicID string) (*Session, *productmatch.Analysis, error) {
	session, err := s.LoadImport(ctx, publicID)
	if err != nil {
		return nil, nil, err
	}
	analysis, err := s.analyse(ctx, session)
	if err != nil {
		return nil, nil, err
	}
	return session, analysis, nil
}

// analyse reads the stored file and applies whatever the vendor has decided so
// far.
func (s *Service) analyse(ctx context.Context, session *Session) (*productmatch.Analysis, error) {
	content, err := s.imports.File(ctx, session.ID)
	if err != nil {
		return nil, err
	}
	book, err := sheet.Open(content, session.Filename)
	if err != nil {
		return nil, apperr.Validation("import.unreadable", err.Error(), nil)
	}
	defer func() { _ = book.Close() }()

	if session.Source.Sheet != "" {
		if err := book.Use(session.Source.Sheet); err != nil {
			s.log.WarnContext(ctx, "import sheet no longer available",
				"import", session.PublicID, "sheet", session.Source.Sheet, "error", err)
		}
	}

	analysis, err := productmatch.Analyze(book, s.vocabulary(ctx, session.OrganizationID))
	if err != nil {
		return nil, apperr.Validation("import.unreadable", err.Error(), nil)
	}
	analysis.ApplyOverrides(session.Overrides)
	return analysis, nil
}

// LoadImport reads an import and refuses one belonging to another tenant.
func (s *Service) LoadImport(ctx context.Context, publicID string) (*Session, error) {
	if s.imports == nil {
		return nil, ErrImportStoreUnavailable
	}
	session, err := s.imports.Get(ctx, publicID)
	if err != nil {
		return nil, err
	}
	if orgID, ok := database.TenantFrom(ctx); ok && session.OrganizationID != orgID {
		return nil, apperr.NotFound("catalog_import")
	}
	return session, nil
}

// SaveMapping records the vendor's column corrections and moves to the settings
// stage.
//
// Completion runs here rather than at processing time so that what the vendor
// sees on the confirmation screen already includes every binding the engine
// filled in on their behalf. An import must never write a column the vendor was
// not shown.
func (s *Service) SaveMapping(
	ctx context.Context, publicID string, overrides map[int]productmatch.Field,
) (*Session, *productmatch.Analysis, error) {
	session, err := s.LoadImport(ctx, publicID)
	if err != nil {
		return nil, nil, err
	}
	if !session.Phase.Open() {
		return nil, nil, apperr.Conflict("import.closed",
			i18n.TDefault("w4_mod.w4str_197_197"))
	}

	session.Overrides = overrides
	analysis, err := s.analyse(ctx, session)
	if err != nil {
		return nil, nil, err
	}
	analysis.Complete()

	if blocking := analysis.Blocking(); len(blocking) > 0 {
		return session, analysis, apperr.Validation("import.mapping_incomplete",
			blocking[0].Message, nil)
	}

	session.Mapping = SnapshotMapping(analysis.Layout, analysis.Mapping)
	session.Phase = PhaseSettings
	if err := s.imports.SaveDraft(ctx, session); err != nil {
		return nil, nil, err
	}
	return session, analysis, nil
}

// SaveSettings records the import rules and stages the products for review.
func (s *Service) SaveSettings(ctx context.Context, publicID string, settings Settings) (*Session, error) {
	session, err := s.LoadImport(ctx, publicID)
	if err != nil {
		return nil, err
	}
	if !session.Phase.Open() {
		return nil, apperr.Conflict("import.closed",
			i18n.TDefault("w4_mod.w4str_197_197"))
	}
	settings = settings.Normalize()
	if settings.WarehouseID <= 0 {
		return nil, apperr.Validation("import.warehouse_required",
			i18n.TDefault("w4_mod.w4str_198_198"), nil)
	}
	if err := s.assertWarehouse(ctx, settings.WarehouseID); err != nil {
		return nil, err
	}

	session.Settings = settings

	// Detached, not inline. Staging a thirty-thousand-row file against the
	// shared catalogue and then asking a model about the residue is minutes of
	// work, and it used to happen inside this POST — so the browser timed out,
	// and a vendor who navigated away cancelled the run's context and lost the
	// pass halfway through.
	if err := s.StageInBackground(ctx, session); err != nil {
		return nil, err
	}
	return session, nil
}

// assertWarehouse refuses a warehouse that is not the vendor's own.
func (s *Service) assertWarehouse(ctx context.Context, warehouseID int64) error {
	if s.inventory == nil {
		return apperr.Unavailable("inventory", nil)
	}
	warehouses, err := s.inventory.ListWarehouses(ctx)
	if err != nil {
		return err
	}
	for _, w := range warehouses {
		if w.ID == warehouseID {
			return nil
		}
	}
	return apperr.Validation("import.warehouse_unknown",
		i18n.TDefault("w4_mod.w4str_199_199"), nil)
}

// BackToSettings reopens the settings step from the review screen.
func (s *Service) BackToSettings(ctx context.Context, publicID string) (*Session, error) {
	session, err := s.LoadImport(ctx, publicID)
	if err != nil {
		return nil, err
	}
	if session.Phase.Terminal() || session.Phase == PhaseProcessing {
		return nil, apperr.Conflict("import.closed", i18n.TDefault("w4_mod.w4str_200_200"))
	}
	session.Phase = PhaseSettings
	if err := s.imports.SaveDraft(ctx, session); err != nil {
		return nil, err
	}
	return session, nil
}

// BackToMapping reopens the column review after the vendor has moved past it.
func (s *Service) BackToMapping(ctx context.Context, publicID string) (*Session, error) {
	session, err := s.LoadImport(ctx, publicID)
	if err != nil {
		return nil, err
	}
	if session.Phase.Terminal() || session.Phase == PhaseProcessing {
		return nil, apperr.Conflict("import.closed", i18n.TDefault("w4_mod.w4str_200_200"))
	}
	session.Phase = PhaseMapping
	if err := s.imports.SaveDraft(ctx, session); err != nil {
		return nil, err
	}
	return session, nil
}

// CancelImport discards an import without touching the catalogue.
func (s *Service) CancelImport(ctx context.Context, publicID string) error {
	session, err := s.LoadImport(ctx, publicID)
	if err != nil {
		return err
	}
	if session.Phase == PhaseProcessing {
		return apperr.Conflict("import.running",
			i18n.TDefault("w4_mod.w4str_201_201"))
	}
	return s.imports.Cancel(ctx, session.ID)
}

// RecentImports backs the history panel on the upload screen.
func (s *Service) RecentImports(ctx context.Context, orgID int64, limit int) ([]*Session, error) {
	if s.imports == nil {
		return nil, ErrImportStoreUnavailable
	}
	return s.imports.List(ctx, orgID, limit)
}

// ImportRows reads a page of the results table.
func (s *Service) ImportRows(
	ctx context.Context, publicID string, filter RowFilter,
) ([]*RowOutcome, int, error) {
	session, err := s.LoadImport(ctx, publicID)
	if err != nil {
		return nil, 0, err
	}
	return s.imports.Rows(ctx, session.ID, filter)
}

// ImportRowCounts tallies the results ledger by outcome.
func (s *Service) ImportRowCounts(ctx context.Context, publicID string) (map[string]int, error) {
	session, err := s.LoadImport(ctx, publicID)
	if err != nil {
		return nil, err
	}
	return s.imports.RowCounts(ctx, session.ID)
}

// Warehouses lists the vendor's warehouses for the settings screen.
func (s *Service) Warehouses(ctx context.Context) ([]*inventory.Warehouse, error) {
	if s.inventory == nil {
		return nil, apperr.Unavailable("inventory", nil)
	}
	return s.inventory.ListWarehouses(ctx)
}
