package catalog

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"time"

	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// The reviewed import.
//
// Three steps, each with its own transaction and its own failure mode:
//
//	Analyse   read the file, work out its shape, keep it. Writes nothing to the
//	          catalogue and nothing the admin has to undo.
//	Prepare   parse it under the admin's corrections, match every row against the
//	          catalogue, fill gaps, and stage the result for review. Still writes
//	          nothing to the catalogue — re-runnable as often as the admin likes.
//	Commit    apply the staged rows under the chosen mode, all or nothing.
//
// Keeping the first two free of catalogue writes is the whole point: an admin
// can upload a nine-thousand-row file, discover the price column was misread,
// fix the mapping, and run it again without ever having touched the catalogue.

// ImportSessionStore is the persistence the reviewed import needs. It is
// declared here, beside its only consumer, rather than beside the PostgreSQL
// implementation that satisfies it.
type ImportSessionStore interface {
	CreateImportSession(ctx context.Context, s *ImportSession, sourceFile []byte) error
	GetImportSession(ctx context.Context, publicID string) (*ImportSession, error)
	// UpdateImportSession saves the whole session row. Any fromStatuses turn it
	// into a guarded transition that fails when the stored status is not among
	// them, which is how a background run refuses to overwrite a state change
	// it did not see.
	UpdateImportSession(ctx context.Context, s *ImportSession, fromStatuses ...SessionStatus) error
	// SaveImportProgress records where a background run has reached. It is a
	// narrow write on purpose: the run does not own the rest of the row while
	// it is in flight, and a full save would clobber a concurrent edit.
	SaveImportProgress(ctx context.Context, publicID string, p ImportProgress) error
	// ClaimImportSessionForCommit atomically flips one reviewable session to
	// 'committing' and returns the fresh row, so two concurrent commits cannot
	// both pass whatever process-local guards say.
	ClaimImportSessionForCommit(ctx context.Context, publicID string) (*ImportSession, error)
	ImportSourceFile(ctx context.Context, sessionID int64) ([]byte, error)
	ReleaseImportSourceFile(ctx context.Context, sessionID int64) error
	ListRecentImportSessions(ctx context.Context, orgID int64, limit int) ([]*ImportSession, error)

	ReplaceStagingRows(ctx context.Context, sessionID int64, rows []*StagingRow) error
	ClearStagingRows(ctx context.Context, sessionID int64) error
	ListStagingRows(ctx context.Context, sessionID int64, filter StagingFilter) ([]*StagingRow, int, error)
	GetStagingRow(ctx context.Context, sessionID, rowID int64) (*StagingRow, error)
	LoadCommittableRows(ctx context.Context, sessionID int64) ([]*StagingRow, error)
	SetRowIncluded(ctx context.Context, sessionID, rowID int64, included bool) error
	SetRowsIncludedByAction(ctx context.Context, sessionID int64, action RowAction, included bool) (int64, error)
	CountStagingActions(ctx context.Context, sessionID int64) (StagingCounts, error)

	DefaultCatalogOrg(ctx context.Context) (int64, error)
	MatchExistingProducts(ctx context.Context, prods []*Product) (map[int]ExistingMatch, error)
	// ListMatchProducts loads the catalogue as a matching projection, for the
	// similarity tier that runs on whatever the exact identifiers missed.
	ListMatchProducts(ctx context.Context) ([]MatchProduct, error)
	ImportVocabulary(ctx context.Context, orgID int64) (EnrichVocabulary, error)
	// BulkCommitProducts is the write side of commit as one transaction: an
	// optional archive of the organisation's catalogue followed by the upsert,
	// together or not at all. archiveOrg of zero skips the archive.
	BulkCommitProducts(
		ctx context.Context, prods []*Product, opts BulkWriteOptions, archiveOrg int64,
	) (archived int64, result BulkWriteResult, err error)
}

// SetImportStore installs the staging persistence.
func (s *Service) SetImportStore(store ImportSessionStore) {
	s.imports = store
	if s.progress == nil {
		s.progress = NewProgressTracker()
	}
	if s.sheets == nil {
		s.sheets = newSheetCache()
	}
}

// SetAIMapper installs the AI mapping port. Leaving it unset disables the AI
// switch in the wizard rather than failing an import that asked for it.
func (s *Service) SetAIMapper(m AIMapper) { s.mapper = m }

// AIAvailable reports whether AI can be offered on the upload screen.
func (s *Service) AIAvailable(ctx context.Context) bool {
	return s.mapper != nil && s.mapper.Available(ctx)
}

// ErrImportUnavailable means the staging store was never wired.
var ErrImportUnavailable = errors.New("catalog: import sessions are not configured")

// ImportSettings are the choices the admin makes before processing.
type ImportSettings struct {
	Mode      ImportMode
	Options   ImportOptions
	Overrides LayoutOverrides
}

// applyDefaultCategory fills the fallback category the admin chose, for every
// product that ends without one. It runs whether or not AI is on, so a file with
// no category column still lands somewhere sensible.
func applyDefaultCategory(prods []*Product, opts ImportOptions) {
	if !opts.AssignCategory || opts.DefaultCategoryID <= 0 {
		return
	}
	for _, p := range prods {
		if p != nil && (p.CategoryID == nil || *p.CategoryID <= 0) {
			id := opts.DefaultCategoryID
			p.CategoryID = &id
		}
	}
}

// buildStagingRows decides what each parsed product would do under the chosen
// mode, and packages it for review.
func buildStagingRows(
	parsed *ParseResult, matches map[int]ExistingMatch, mode ImportMode,
) []*StagingRow {
	issuesByRow := groupIssuesByRow(parsed.Issues)
	rows := make([]*StagingRow, 0, len(parsed.Products))

	for i, product := range parsed.Products {
		sourceRow := 0
		if i < len(parsed.SourceRows) {
			sourceRow = parsed.SourceRows[i]
		}

		row := &StagingRow{
			SourceRow: sourceRow,
			Product:   product,
			Included:  true,
			Issues:    issuesByRow[sourceRow],
		}
		if match, matched := matches[i]; matched {
			id := match.ProductID
			row.MatchedProductID = &id
			row.MatchReason = match.Reason
		}
		row.Action = actionFor(mode, row.MatchedProductID != nil)

		// A row carrying a blocking finding is never silently committed, whatever
		// the mode says. The admin sees it in the review table as excluded.
		if row.HasErrors() {
			row.Action = ActionSkip
			row.Included = false
		}
		rows = append(rows, row)
	}
	return rows
}

// actionFor applies the chosen strategy to one row.
func actionFor(mode ImportMode, matched bool) RowAction {
	switch mode {
	case ModeAddNewOnly:
		if matched {
			return ActionSkip
		}
		return ActionInsert

	case ModeUpdateExistingOnly:
		if matched {
			return ActionUpdate
		}
		return ActionSkip

	case ModeClearAndAdd:
		// The catalogue is archived first, so every row is new by definition —
		// including the ones that match something about to be retired.
		return ActionInsert

	default: // ModeUpdateAndAdd
		if matched {
			return ActionUpdate
		}
		return ActionInsert
	}
}

// groupIssuesByRow indexes findings so each staged row carries its own.
func groupIssuesByRow(issues []RowIssue) map[int][]RowIssue {
	out := map[int][]RowIssue{}
	for _, issue := range issues {
		out[issue.Row] = append(out[issue.Row], issue)
	}
	return out
}

// collectNewBrands lists manufacturers the catalogue has never seen, so the
// review screen can show what the import would add to the brand list.
func collectNewBrands(prods []*Product, matches map[int]ExistingMatch) []string {
	seen := map[string]bool{}
	var out []string

	for i, p := range prods {
		if p == nil || p.ManufacturingCompanies == "" {
			continue
		}
		// A matched product already has whatever brand it has; only a new
		// product can introduce a new manufacturer.
		if _, matched := matches[i]; matched {
			continue
		}
		key := NormalizeKey(p.ManufacturingCompanies)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, p.ManufacturingCompanies)
		if len(out) >= 200 {
			break
		}
	}
	return out
}

func applyParseStats(session *ImportSession, parsed *ParseResult) {
	session.TotalRows = parsed.Stats.TotalRowsRead
	session.ParsedRows = len(parsed.Products)
	session.BlockCount = len(parsed.Layout.Blocks)
}

func applyRowStats(session *ImportSession, rows []*StagingRow) {
	session.InsertRows, session.UpdateRows, session.SkipRows = 0, 0, 0
	session.ErrorRows, session.WarningRows = 0, 0

	for _, row := range rows {
		switch row.Action {
		case ActionInsert:
			session.InsertRows++
		case ActionUpdate:
			session.UpdateRows++
		default:
			session.SkipRows++
		}
		if row.HasErrors() {
			session.ErrorRows++
		} else if len(row.Issues) > 0 {
			session.WarningRows++
		}
	}
}

// CommitImport applies a reviewed session to the catalogue.
//
// What commits is exactly what the review screen showed: the rows are read back
// from staging with their recorded action, not recomputed. A row the admin
// deselected does not come back, and a row shown as an update cannot arrive as
// an insert.
//
// Ownership is taken atomically before anything else happens: the session is
// flipped to 'committing' by a single conditional UPDATE, so two commits — two
// tabs, two processes — cannot both write. The archive and the write then share
// one transaction, so clear-and-add can never leave the catalogue archived with
// nothing imported.
func (s *Service) CommitImport(ctx context.Context, publicID string) (*ImportSession, BulkWriteResult, error) {
	empty := BulkWriteResult{Matches: map[int]MatchReason{}}
	if s.imports == nil {
		return nil, empty, ErrImportUnavailable
	}
	session, err := s.imports.ClaimImportSessionForCommit(ctx, publicID)
	if err != nil {
		return nil, empty, err
	}

	rows, err := s.imports.LoadCommittableRows(ctx, session.ID)
	if err != nil {
		s.releaseClaim(ctx, session)
		return session, empty, err
	}
	if len(rows) == 0 {
		s.releaseClaim(ctx, session)
		return session, empty, apperr.Validation("catalog.import_nothing_selected",
			"لم يتم تحديد أي صنف للاستيراد. يرجى مراجعة الصفوف واختيار ما تريد حفظه.", nil)
	}

	prods := make([]*Product, 0, len(rows))
	for _, row := range rows {
		product := row.Product
		if product == nil {
			continue
		}
		// Carry the resolved identity forward so the write updates the row the
		// preview named. Under clear-and-add everything inserts: the matched
		// rows were archived with the rest of the catalogue inside the same
		// transaction, so no stale identity survives to be updated.
		if session.Mode != ModeClearAndAdd && row.Action == ActionUpdate && row.MatchedProductID != nil {
			product.ID = *row.MatchedProductID
		}
		prods = append(prods, product)
	}

	archiveOrg := int64(0)
	if session.Mode == ModeClearAndAdd {
		archiveOrg = session.OrganizationID
	}

	// Validation runs before the transaction: a row the domain refuses is
	// dropped and named, not fed to the database to abort the batch with.
	if len(prods) > maxImportBatch {
		s.releaseClaim(ctx, session)
		return session, empty, apperr.Validation("catalog.import_too_large",
			fmt.Sprintf("الملف يحتوي على %d صنف، والحد الأقصى المسموح به في عملية الاستيراد الواحدة هو %d صنف. يرجى تقسيم الملف.",
				len(prods), maxImportBatch), nil)
	}
	valid, issues, err := s.validateImportBatch(prods)
	if err != nil {
		session.ErrorRows += len(issues)
		s.releaseClaim(ctx, session)
		return session, empty, err
	}

	// The toggles decide what the write may create. A taxonomy row that already
	// exists is reused either way; these govern adding one that does not.
	archived, written, err := s.imports.BulkCommitProducts(ctx, valid, BulkWriteOptions{
		CreateBrands:     session.Options.AutoCreateBrands,
		CreateCategories: session.Options.AssignCategory && session.Options.AutoCreateCategories,
	}, archiveOrg)
	if archived > 0 {
		s.log.WarnContext(ctx, "catalogue archived before import",
			"session", session.PublicID, "archived", archived)
	}
	if err != nil {
		session.Status = SessionFailed
		session.ErrorMessage = err.Error()
		if updateErr := s.imports.UpdateImportSession(ctx, session, SessionCommitting); updateErr != nil {
			s.log.ErrorContext(ctx, "could not record import failure", "error", updateErr)
		}
		return session, written, err
	}

	now := time.Now()
	session.Status = SessionCommitted
	session.CommittedAt = &now
	session.InsertRows = written.Inserted
	session.UpdateRows = written.Updated
	session.ErrorMessage = ""
	if len(issues) > 0 {
		session.ErrorRows += len(issues)
	}

	if err := s.imports.UpdateImportSession(ctx, session, SessionCommitting); err != nil {
		return session, written, err
	}
	// The file and the staged rows have done their job; keeping the admin's
	// workbook and nine thousand staging rows past the commit serves nothing,
	// and the session row keeps the counts that history needs.
	s.releaseSessionWorkspace(ctx, session)

	s.log.InfoContext(ctx, "catalogue import committed",
		"session", session.PublicID, "mode", session.Mode,
		"inserted", written.Inserted, "updated", written.Updated,
		"brands_created", written.BrandsCreated)
	return session, written, nil
}

// releaseClaim hands a claimed session back to review. Used when commit stops
// before writing anything — nothing selected, an oversized batch — where the
// right outcome is the admin back on the review screen, not a failed session.
func (s *Service) releaseClaim(ctx context.Context, session *ImportSession) {
	session.Status = SessionReady
	if err := s.imports.UpdateImportSession(ctx, session, SessionCommitting); err != nil {
		s.log.ErrorContext(ctx, "could not release import commit claim",
			"session", session.PublicID, "error", err)
	}
}

// CancelImport discards a session without touching the catalogue.
//
// The transition is guarded: cancelling an already-committed session is refused
// rather than reported as success, which is what the unguarded overwrite used
// to do.
func (s *Service) CancelImport(ctx context.Context, publicID string) error {
	if s.imports == nil {
		return ErrImportUnavailable
	}
	session, err := s.imports.GetImportSession(ctx, publicID)
	if err != nil {
		return err
	}
	// The session row is the authority on whether a run is live. The in-memory
	// tracker lags it — it is updated after the transaction commits — so
	// consulting it here refused a cancel for the few milliseconds after a run
	// had already finished and durably said so.
	if session.IsProcessing() {
		return apperr.Conflict("catalog.import_still_processing",
			"لا تزال معالجة الملف جارية. انتظر حتى تكتمل ثم أعد المحاولة.")
	}
	if !session.IsReviewable() && session.Status != SessionFailed {
		return apperr.Conflict("catalog.import_not_cancellable",
			"لا يمكن إلغاء هذه الجلسة لأنها اكتملت بالفعل.")
	}
	from := session.Status
	session.Status = SessionCancelled
	if err := s.imports.UpdateImportSession(ctx, session, from); err != nil {
		return err
	}
	s.releaseSessionWorkspace(ctx, session)
	return nil
}

// releaseSessionWorkspace frees what a finished session no longer needs.
//
// Best-effort by design: an import that has already been written must not be
// reported as failed because its scratch space could not be tidied. The reaper
// that runs when the next session opens collects whatever is left.
func (s *Service) releaseSessionWorkspace(ctx context.Context, session *ImportSession) {
	s.sheets.drop(session.PublicID)
	if err := s.imports.ReleaseImportSourceFile(ctx, session.ID); err != nil {
		s.log.WarnContext(ctx, "could not release import source file",
			"session", session.PublicID, "error", err)
	}
	if err := s.imports.ClearStagingRows(ctx, session.ID); err != nil {
		s.log.WarnContext(ctx, "could not clear staged rows",
			"session", session.PublicID, "error", err)
	}
}

// GetImportSession loads a session for the review screen.
func (s *Service) GetImportSession(ctx context.Context, publicID string) (*ImportSession, error) {
	if s.imports == nil {
		return nil, ErrImportUnavailable
	}
	return s.imports.GetImportSession(ctx, publicID)
}

// ListStagingRows reads a page of the review table.
func (s *Service) ListStagingRows(
	ctx context.Context, publicID string, filter StagingFilter,
) (*ImportSession, []*StagingRow, int, error) {
	if s.imports == nil {
		return nil, nil, 0, ErrImportUnavailable
	}
	session, err := s.imports.GetImportSession(ctx, publicID)
	if err != nil {
		return nil, nil, 0, err
	}
	rows, total, err := s.ListStagingRowsFor(ctx, session.ID, filter)
	if err != nil {
		return nil, nil, 0, err
	}
	return session, rows, total, nil
}

// ListStagingRowsFor reads a page of the review table for a session already in
// hand, so a caller that has just loaded the session does not load it again.
//
// The extra read is not free and it is not harmless: it is a second answer to
// "what state is this import in", and a run finishing between the two is enough
// for a screen to disagree with itself.
func (s *Service) ListStagingRowsFor(
	ctx context.Context, sessionID int64, filter StagingFilter,
) ([]*StagingRow, int, error) {
	if s.imports == nil {
		return nil, 0, ErrImportUnavailable
	}
	return s.imports.ListStagingRows(ctx, sessionID, filter)
}

// GetStagingRow returns one staged row for review.
func (s *Service) GetStagingRow(ctx context.Context, publicID string, rowID int64) (*StagingRow, error) {
	if s.imports == nil {
		return nil, ErrImportUnavailable
	}
	session, err := s.imports.GetImportSession(ctx, publicID)
	if err != nil {
		return nil, err
	}
	return s.imports.GetStagingRow(ctx, session.ID, rowID)
}

// SetRowIncluded flips one row's inclusion switch in the review table.
func (s *Service) SetRowIncluded(ctx context.Context, publicID string, rowID int64, included bool) error {
	if s.imports == nil {
		return ErrImportUnavailable
	}
	session, err := s.imports.GetImportSession(ctx, publicID)
	if err != nil {
		return err
	}
	return s.imports.SetRowIncluded(ctx, session.ID, rowID, included)
}

// SetRowsIncludedByAction flips every row sharing an action.
func (s *Service) SetRowsIncludedByAction(
	ctx context.Context, publicID string, action RowAction, included bool,
) (int64, error) {
	if s.imports == nil {
		return 0, ErrImportUnavailable
	}
	session, err := s.imports.GetImportSession(ctx, publicID)
	if err != nil {
		return 0, err
	}
	return s.imports.SetRowsIncludedByAction(ctx, session.ID, action, included)
}

// StagingCounts tallies what the admin has left selected.
func (s *Service) StagingCounts(ctx context.Context, sessionID int64) (StagingCounts, error) {
	if s.imports == nil {
		return StagingCounts{}, ErrImportUnavailable
	}
	return s.imports.CountStagingActions(ctx, sessionID)
}

// ImportVocabulary is the taxonomy offered in the wizard's category chooser.
//
// An organisation of zero means "the master catalogue", which the wizard uses
// before a session exists and therefore before it knows which organisation owns
// it.
func (s *Service) ImportVocabulary(ctx context.Context, orgID int64) (EnrichVocabulary, error) {
	if s.imports == nil {
		return EnrichVocabulary{}, ErrImportUnavailable
	}
	if orgID <= 0 {
		resolved, err := s.imports.DefaultCatalogOrg(ctx)
		if err != nil {
			return EnrichVocabulary{}, err
		}
		orgID = resolved
	}
	return s.imports.ImportVocabulary(ctx, orgID)
}

// RecentImportSessions backs the history panel on the upload screen.
func (s *Service) RecentImportSessions(ctx context.Context, orgID int64, limit int) ([]*ImportSession, error) {
	if s.imports == nil {
		return nil, nil
	}
	if orgID <= 0 {
		resolved, err := s.imports.DefaultCatalogOrg(ctx)
		if err != nil {
			return nil, err
		}
		orgID = resolved
	}
	return s.imports.ListRecentImportSessions(ctx, orgID, limit)
}

// SummarizeProduct renders a staged product as one readable line for the review
// table's detail column.
func SummarizeProduct(p *Product) string {
	if p == nil {
		return ""
	}
	var parts []string
	if p.SKU != "" {
		parts = append(parts, "كود: "+p.SKU)
	}
	if p.Price.IsPositive() {
		parts = append(parts, "سعر: "+p.Price.String())
	}
	if p.DosageForm != "" {
		parts = append(parts, p.DosageForm)
	}
	if p.ManufacturingCompanies != "" {
		parts = append(parts, p.ManufacturingCompanies)
	}
	if en := p.Name.Get(i18n.EN); en != "" && en != p.Name.Get(i18n.AR) {
		parts = append(parts, en)
	}
	return strings.Join(parts, " · ")
}

// ImportStructure re-reads a session's file and reports how its columns map.
//
// The review screen needs this to draw the mapping panel. It re-parses rather
// than storing the plan because the admin's overrides change it, and a stored
// copy would drift from what the next run actually does.
func (s *Service) ImportStructure(ctx context.Context, publicID string) (FileStructure, error) {
	if s.imports == nil {
		return FileStructure{}, ErrImportUnavailable
	}
	session, err := s.imports.GetImportSession(ctx, publicID)
	if err != nil {
		return FileStructure{}, err
	}
	if !session.Structure.IsEmpty() {
		return session.Structure, nil
	}

	// A session opened before the structure was stored, or one whose analysis
	// predates this build. Re-read it once and keep the result, so the next
	// render is free.
	sheet, err := s.sheetFor(ctx, session)
	if err != nil {
		return FileStructure{}, err
	}
	session.Structure = BuildFileStructure(sheet, AnalyzeLayout(sheet).Apply(sheet, session.Overrides))
	if err := s.imports.UpdateImportSession(ctx, session, session.Status); err != nil {
		s.log.DebugContext(ctx, "could not backfill import structure",
			"session", publicID, "error", err)
	}
	return session.Structure, nil
}

// EnricherRunning reports whether a background preparation is in flight. Tests
// use it to catch a run mid-flight; the UI uses ImportProgress.
func (s *Service) EnricherRunning(publicID string) bool {
	return s.progress.Running(publicID)
}
