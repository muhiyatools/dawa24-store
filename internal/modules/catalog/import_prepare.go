package catalog

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// Analysing a file, previewing a mapping, and running the preparation.
//
// The three of them are one story and it is worth reading in order. Analysis
// reads the upload and describes its shape without interpreting a single row.
// Preview re-reads it under whatever the admin has changed and answers "what
// would this produce", still without staging anything. Preparation is the only
// expensive one, and it is the only one that runs in the background.
//
// Keeping them apart is what the wizard's mapping step is made of: an admin can
// go round analyse-and-preview as many times as they like, in under a second
// each, before committing a machine to the fifteen seconds that stage a real
// distributor export.

// AnalyzeImport reads an uploaded file and opens a mapping session for it.
//
// It reads the sheet and works out its shape — where the titles are, how many
// blocks, which column looks like which field, what the values in each column
// look like — and stores that description on the session. It deliberately does
// not parse the rows into products: that is the expensive half, it depends on a
// mapping the admin has not confirmed yet, and doing it here is what made the
// upload button feel like it hung.
func (s *Service) AnalyzeImport(
	ctx context.Context, content []byte, filename string, actorID int64,
) (*ImportSession, FileStructure, error) {
	if s.imports == nil {
		return nil, FileStructure{}, ErrImportUnavailable
	}

	sheet, err := ReadSpreadsheet(content, filename)
	if err != nil {
		return nil, FileStructure{}, err
	}
	if len(sheet.Rows) == 0 {
		return nil, FileStructure{}, apperr.Validation("catalog.import_file_empty",
			"الملف المرفوع لا يحتوي على أي صفوف قابلة للقراءة. يرجى التأكد من الملف ثم إعادة رفعه.", nil)
	}

	layout := AnalyzeLayout(sheet)
	structure := BuildFileStructure(sheet, layout)

	// The session is scoped to the organisation that owns the master catalogue,
	// so the rows it matches against during review are the same rows the commit
	// will write into.
	orgID, err := s.imports.DefaultCatalogOrg(ctx)
	if err != nil {
		return nil, structure, err
	}

	session := &ImportSession{
		OrganizationID: orgID,
		Filename:       filename,
		FileSizeBytes:  int64(len(content)),
		SourceFormat:   sheet.Format,
		SheetName:      sheet.Sheet,
		Delimiter:      sheet.Delimiter,
		Status:         SessionDraft,
		Mode:           ModeUpdateAndAdd,
		Options:        DefaultImportOptions(),
		Structure:      structure,
		TotalRows:      structure.TotalRows,
		BlockCount:     structure.BlockCount,
	}
	if actorID > 0 {
		session.CreatedBy = &actorID
	}

	if err := s.imports.CreateImportSession(ctx, session, content); err != nil {
		return nil, structure, err
	}
	s.sheets.put(session.PublicID, sheet)

	s.log.InfoContext(ctx, "catalogue import session opened",
		"session", session.PublicID, "file", filename,
		"rows", structure.TotalRows, "blocks", structure.BlockCount,
		"mapped_fields", structure.MappedFields(), "positional", structure.Positional)

	return session, structure, nil
}

// ImportPreviewRows is how many parsed products the mapping screen shows. It is
// a sample, not a page: the admin is checking that the columns line up, and
// twenty-five rows answers that on any file.
const ImportPreviewRows = 25

// ImportPreview is what the mapping screen renders: the file re-read under the
// admin's current corrections, with a handful of products to look at.
type ImportPreview struct {
	Structure FileStructure
	// Products is the first ImportPreviewRows products the mapping would yield.
	Products []*Product
	// SourceRows are the spreadsheet row numbers those products came from.
	SourceRows []int
	// Issues are the findings from the previewed rows only.
	Issues []RowIssue
	// TotalProducts is how many products the whole file would yield under this
	// mapping, so the admin sees the real number before committing to a run.
	TotalProducts int
	// RejectedRows is how many rows this mapping would drop entirely.
	RejectedRows int
	// DuplicateRows is how many rows fold into an earlier one.
	DuplicateRows int
}

// PreviewImport re-reads the session's file under a proposed mapping without
// staging anything, and stores the resulting structure on the session.
//
// This is the whole point of the mapping step: the admin sees what their
// correction actually does — to the columns, to the row count, to the first
// twenty-five products — before a single row is staged and long before the
// catalogue is touched.
func (s *Service) PreviewImport(
	ctx context.Context, publicID string, settings ImportSettings,
) (*ImportSession, *ImportPreview, error) {
	if s.imports == nil {
		return nil, nil, ErrImportUnavailable
	}

	session, err := s.imports.GetImportSession(ctx, publicID)
	if err != nil {
		return nil, nil, err
	}
	if session.IsProcessing() {
		return session, nil, apperr.Conflict("catalog.import_already_running",
			"جارٍ بالفعل معالجة هذا الملف. يرجى الانتظار حتى تكتمل العملية.")
	}
	if !session.IsRetryable() {
		return session, nil, apperr.Validation("catalog.import_not_reviewable",
			"لا يمكن تعديل هذه الجلسة لأنها اكتملت أو أُلغيت. يرجى بدء عملية استيراد جديدة.", nil)
	}

	sheet, err := s.sheetFor(ctx, session)
	if err != nil {
		return session, nil, err
	}

	session.Mode = settings.Mode
	session.Options = settings.Options
	session.Overrides = settings.Overrides

	parsed := ParseSheet(sheet, session.Overrides, session.Options)
	structure := BuildFileStructure(sheet, parsed.Layout)

	session.Structure = structure
	session.TotalRows = parsed.Stats.TotalRowsRead
	session.ParsedRows = len(parsed.Products)
	session.BlockCount = len(parsed.Layout.Blocks)
	// A preview clears the last run's failure: the admin is looking at a fresh
	// reading, and leaving the old error on the row would have the review screen
	// report a failure that no longer describes anything.
	if session.Status == SessionFailed {
		session.Status = SessionDraft
	}
	session.ErrorMessage = ""
	if err := s.imports.UpdateImportSession(ctx, session, retryableStatuses()...); err != nil {
		return session, nil, err
	}

	preview := &ImportPreview{
		Structure:     structure,
		TotalProducts: len(parsed.Products),
		RejectedRows:  parsed.Stats.RejectedRows,
		DuplicateRows: parsed.Stats.DuplicateRows,
	}
	preview.Products = parsed.Products[:min(len(parsed.Products), ImportPreviewRows)]
	preview.SourceRows = parsed.SourceRows[:min(len(parsed.SourceRows), ImportPreviewRows)]

	lastRow := 0
	if n := len(preview.SourceRows); n > 0 {
		lastRow = preview.SourceRows[n-1]
	}
	for _, issue := range parsed.Issues {
		if issue.Row <= lastRow || issue.Row <= 1 {
			preview.Issues = append(preview.Issues, issue)
		}
	}
	return session, preview, nil
}

// sheetFor decodes a session's uploaded file, reusing the warm copy when the
// admin is working through the mapping screen.
func (s *Service) sheetFor(ctx context.Context, session *ImportSession) (*SheetData, error) {
	if cached := s.sheets.get(session.PublicID); cached != nil {
		return cached, nil
	}

	content, err := s.imports.ImportSourceFile(ctx, session.ID)
	if err != nil {
		return nil, err
	}
	if len(content) == 0 {
		return nil, apperr.Validation("catalog.import_file_expired",
			"انتهت صلاحية الملف المرفوع لهذه الجلسة. يرجى رفع الملف من جديد.", nil)
	}

	sheet, err := ReadSpreadsheet(content, session.Filename)
	if err != nil {
		return nil, err
	}
	s.sheets.put(session.PublicID, sheet)
	return sheet, nil
}

// PrepareImport re-reads the session's file under the admin's settings, decides
// what each row would do, enriches what it can, and stages the result.
//
// It is idempotent by construction: staging rows are replaced wholesale, so the
// admin can adjust the column mapping and run it again as many times as they
// need without accumulating anything.
func (s *Service) PrepareImport(ctx context.Context, publicID string, settings ImportSettings) (*ImportSession, error) {
	return s.prepare(ctx, publicID, settings, nil)
}

// PrepareImportAsync starts preparation in the background and returns at once.
//
// A nine-thousand-row file is ten to fifteen seconds of matching, and with AI
// on it is minutes; running it inside the request that asked for it gives the
// admin a browser hanging on something that may outlive its own timeout.
//
// The session is flipped to 'processing' synchronously, before the goroutine
// starts, and that flip is the durable record of the run. Progress used to live
// only in a map in this process: when a run failed, or the process restarted,
// the session was left at 'draft' with no counts and no error message, and the
// review screen showed an import of nine thousand products as zero rows with
// nothing to explain it.
func (s *Service) PrepareImportAsync(ctx context.Context, publicID string, settings ImportSettings) error {
	if s.imports == nil {
		return ErrImportUnavailable
	}

	session, err := s.imports.GetImportSession(ctx, publicID)
	if err != nil {
		return err
	}
	if session.IsProcessing() {
		return apperr.Conflict("catalog.import_already_running",
			"جارٍ بالفعل معالجة هذا الملف. يرجى الانتظار حتى تكتمل العملية.")
	}
	if !session.IsRetryable() {
		return apperr.Validation("catalog.import_not_reviewable",
			"لا يمكن تعديل هذه الجلسة لأنها اكتملت أو أُلغيت. يرجى بدء عملية استيراد جديدة.", nil)
	}

	report, claimed := s.progress.TryBegin(publicID)
	if !claimed {
		return apperr.Conflict("catalog.import_already_running",
			"جارٍ بالفعل معالجة هذا الملف. يرجى الانتظار حتى تكتمل العملية.")
	}

	// Taking the session before the goroutine starts means the caller learns
	// immediately that the run could not be claimed, and means a second submit
	// arriving a millisecond later is refused by the database rather than by a
	// map this process happens to hold.
	session.Mode, session.Options, session.Overrides = settings.Mode, settings.Options, settings.Overrides
	session.Status, session.ErrorMessage = SessionProcessing, ""
	session.Progress = ImportProgress{Phase: ImportPhaseReading, Message: ImportPhaseReading.Label()}
	if err := s.imports.UpdateImportSession(ctx, session, retryableStatuses()...); err != nil {
		s.progress.Finish(publicID, err)
		return err
	}

	// Detached from the request: the admin's browser navigating away, or the
	// request timing out, must not abandon a run that is already underway.
	runCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), prepareTimeout)

	go func() {
		defer cancel()
		_, err := s.prepare(runCtx, publicID, settings, s.persistedProgress(runCtx, publicID, report))
		s.progress.Finish(publicID, err)
		if err != nil {
			s.log.ErrorContext(runCtx, "background import preparation failed",
				"session", publicID, "error", err)
			s.recordPrepareFailure(runCtx, publicID, err)
		}
	}()
	return nil
}

// persistedProgress wraps the in-memory reporter so each phase change is also
// written to the session row.
//
// Five writes per run, one per phase — not per batch. That is what lets a page
// loaded in a second browser tab, or after a deploy, say what the run is doing
// instead of guessing from an empty map.
func (s *Service) persistedProgress(ctx context.Context, publicID string, report ProgressFunc) ProgressFunc {
	var last ImportPhase
	return func(phase ImportPhase, current, total int) {
		report.report(phase, current, total)
		if phase == last {
			return
		}
		last = phase
		if err := s.imports.SaveImportProgress(ctx, publicID, ImportProgress{
			Phase: phase, Current: current, Total: total, Message: phase.Label(),
		}); err != nil {
			s.log.DebugContext(ctx, "persist import progress",
				"session", publicID, "phase", phase, "error", err)
		}
	}
}

// recordPrepareFailure writes the reason a background run stopped onto the
// session, so the screen the admin reloads explains itself.
func (s *Service) recordPrepareFailure(ctx context.Context, publicID string, cause error) {
	session, err := s.imports.GetImportSession(ctx, publicID)
	if err != nil {
		return
	}
	// A session the admin cancelled, or one a commit has taken, is not this
	// run's to mark: the guarded write below would fail anyway, and the state
	// it already carries is the truthful one.
	if session.Status != SessionProcessing {
		return
	}

	session.Status = SessionFailed
	session.ErrorMessage = failureMessage(cause)
	session.Progress = ImportProgress{Phase: ImportPhaseFailed, Message: session.ErrorMessage}
	if err := s.imports.UpdateImportSession(ctx, session, SessionProcessing); err != nil {
		s.log.WarnContext(ctx, "could not record import failure",
			"session", publicID, "error", err)
	}
}

// failureMessage prefers the domain's own Arabic wording over a raw Go error,
// which an admin cannot act on.
func failureMessage(err error) string {
	var appErr *apperr.Error
	if errors.As(err, &appErr) && appErr.Msg != "" {
		return appErr.Msg
	}
	return "تعذرت معالجة الملف: " + err.Error()
}

// prepareTimeout bounds one background run. A file large enough to exceed this
// needs splitting, and an admin should be told so rather than left watching a
// bar that never moves.
const prepareTimeout = 30 * time.Minute

// ImportProgress reports where a background preparation has reached, according
// to this process's own memory. SessionProgress is what a page should ask.
func (s *Service) ImportProgress(publicID string) (ImportProgress, bool) {
	return s.progress.Progress(publicID)
}

// SessionProgress answers "what is this import doing" from whichever source
// knows: the live run in this process, or the session row.
//
// The row is the authority on whether a run exists at all. A poll answered from
// process memory alone reported "failed" for a perfectly healthy run started by
// another replica, and reported nothing at all after a restart — which is how a
// finished import of nine thousand products came to render as an empty screen.
func (s *Service) SessionProgress(ctx context.Context, publicID string) (ImportProgress, *ImportSession, error) {
	session, err := s.GetImportSession(ctx, publicID)
	if err != nil {
		return ImportProgress{}, nil, err
	}

	if live, running := s.progress.Progress(publicID); running && session.IsProcessing() {
		return live, session, nil
	}

	progress := session.Progress
	switch {
	case session.IsProcessing():
		// Claimed but nothing reported yet, or reported by another process.
		if progress.Phase == "" {
			progress.Phase = ImportPhaseReading
		}
	case session.Status == SessionFailed:
		progress.Phase = ImportPhaseFailed
		progress.Message = session.ErrorMessage
	case session.Status == SessionDraft:
		// Analysed and waiting on the mapping step: no run has been asked for.
		progress.Phase = PhaseQueued
	default:
		progress.Phase = ImportPhaseDone
	}
	if progress.Message == "" {
		progress.Message = progress.Phase.Label()
	}
	return progress, session, nil
}

func (s *Service) prepare(
	ctx context.Context, publicID string, settings ImportSettings, progress ProgressFunc,
) (*ImportSession, error) {
	if s.imports == nil {
		return nil, ErrImportUnavailable
	}
	progress.report(ImportPhaseReading, 0, 0)

	session, err := s.imports.GetImportSession(ctx, publicID)
	if err != nil {
		return nil, err
	}
	// A run reaches here having already claimed the session, so 'processing' is
	// the expected state; the synchronous PrepareImport used by tests and the
	// CLI arrives on a session that is merely retryable.
	if !session.IsRetryable() && !session.IsProcessing() {
		return nil, apperr.Validation("catalog.import_not_reviewable",
			"لا يمكن تعديل هذه الجلسة لأنها اكتملت أو أُلغيت. يرجى بدء عملية استيراد جديدة.", nil)
	}

	sheet, err := s.sheetFor(ctx, session)
	if err != nil {
		return nil, err
	}
	from := session.Status

	session.Mode = settings.Mode
	session.Options = settings.Options

	// Request one: which column is which field. It runs before parsing because
	// its whole purpose is to change how the sheet is read.
	session.AICalls, session.AIApplied, session.AIFallback, session.AINote = 0, 0, false, ""
	session.Overrides = s.resolveColumnMapping(ctx, session, sheet, settings.Overrides)

	progress.report(ImportPhaseParsing, 0, 0)
	parsed := ParseSheet(sheet, session.Overrides, settings.Options)
	applyParseStats(session, parsed)
	session.Structure = BuildFileStructure(sheet, parsed.Layout)
	session.SheetName = sheet.Sheet
	session.SourceFormat = sheet.Format
	session.Delimiter = sheet.Delimiter

	// A mapping that yields nothing is a mapping problem, not an empty file,
	// and the admin needs to be sent back to fix it rather than handed a review
	// screen listing zero products with no explanation.
	if len(parsed.Products) == 0 {
		return nil, apperr.Validation("catalog.import_no_products",
			emptyParseMessage(parsed), nil)
	}

	s.resolveTaxonomies(ctx, session, parsed, progress)

	progress.report(ImportPhaseMatching, 0, len(parsed.Products))
	matches, err := s.imports.MatchExistingProducts(ctx, parsed.Products)
	if err != nil {
		return nil, err
	}
	// The exact identifiers have had their turn. Everything they missed is
	// scored against the catalogue in memory, and — where the buyer left the AI
	// switch on — what similarity still cannot settle is adjudicated in batches.
	// Without this the importer matched under a tenth of a real supplier file
	// and staged the rest as new products, quietly duplicating the catalogue.
	matchStats := s.resolveSimilarMatches(ctx, session, parsed.Products, matches)
	session.AIMatched = matchStats.Similar + matchStats.AI
	if matchStats.CeilingHit {
		session.AIFallback = true
	}

	progress.report(ImportPhaseStaging, 0, len(parsed.Products))
	rows := buildStagingRows(parsed, matches, session.Mode)
	applyRowStats(session, rows)
	session.NewBrands = collectNewBrands(parsed.Products, matches)
	// NewCategories was already computed by resolveTaxonomies; both proposals
	// are persisted here so the review screen can show them.
	// The rows land before the status does, and that order matters.
	//
	// Flipping the session to 'ready' first opened a window — a second on a
	// nine-thousand-row file — in which the review screen, the progress poll
	// and a commit all saw a finished import with an empty staging table. That
	// window is the "it says it imported nine thousand products and shows me
	// nothing" report. The session stays 'processing' until the rows it
	// promises are actually there, and 'processing' is not reviewable, so
	// nothing can cancel or commit underneath the write.
	if err := s.imports.ReplaceStagingRows(ctx, session.ID, rows); err != nil {
		return nil, err
	}

	session.Status = SessionReady
	session.Progress = ImportProgress{Phase: ImportPhaseDone, Message: ImportPhaseDone.Label()}
	// Guarded: if the admin cancelled or a commit landed while this prepare
	// ran, the transition fails and the staging rows are left for the reaper
	// rather than resurrecting a dead session with a ready status.
	if err := s.imports.UpdateImportSession(ctx, session, from); err != nil {
		return nil, err
	}

	s.log.InfoContext(ctx, "catalogue import prepared",
		"session", session.PublicID, "mode", session.Mode,
		"insert", session.InsertRows, "update", session.UpdateRows,
		"skip", session.SkipRows, "errors", session.ErrorRows,
		"match_exact", matchStats.Exact, "match_similar", matchStats.Similar,
		"match_ai", matchStats.AI, "match_unmatched", matchStats.Unmatched,
		"match_rate_pct", matchStats.RatePercent(),
		"ai_calls", session.AICalls, "ai_applied", session.AIApplied)
	return session, nil
}

// emptyParseMessage explains a run that produced no products, naming the reason
// the parse itself recorded rather than leaving the admin to guess.
func emptyParseMessage(parsed *ParseResult) string {
	switch {
	case parsed.Stats.TotalRowsRead == 0:
		return "الملف لا يحتوي على أي صفوف. يرجى التأكد من الملف ثم رفعه من جديد."
	case parsed.Stats.RejectedRows > 0:
		return fmt.Sprintf(
			"لم يُقرأ أي صنف من الملف: تم رفض %d صف لعدم احتوائها على اسم صنف أو كود. "+
				"راجع ربط الأعمدة في خطوة «مراجعة الأعمدة» ثم أعد المعالجة.",
			parsed.Stats.RejectedRows)
	default:
		return "لم يُقرأ أي صنف من الملف بالإعدادات الحالية. " +
			"راجع صف العناوين ونطاق الصفوف وربط الأعمدة في خطوة «مراجعة الأعمدة» ثم أعد المعالجة."
	}
}
