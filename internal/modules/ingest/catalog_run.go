package ingest

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/productmatch"
	"github.com/muhiya/dawa24-store/internal/shared/sheet"
)

// The processing stage.
//
// It runs detached from the request that asked for it. A nine-thousand-row file
// against a thirty-thousand-product catalogue is seconds, not minutes, but a
// vendor's browser navigating away must never abandon a run that has already
// begun writing — half an imported catalogue is worse than none.

// runTimeout bounds one run. A file large enough to exceed it needs splitting,
// and the vendor should be told so rather than left watching a bar that has
// stopped moving.
const runTimeout = 30 * time.Minute

// ConfirmImport executes the final commit of reviewed staged rows.
func (s *Service) ConfirmImport(ctx context.Context, publicID string) (*Session, error) {
	return s.CommitImport(ctx, publicID)
}

// ImportRunning reports whether a run is executing in this process.
func (s *Service) ImportRunning(publicID string) bool { return s.runs.running(publicID) }

// runImport is the whole processing stage.
func (s *Service) runImport(ctx context.Context, session *Session) error {
	analysis, err := s.analyse(ctx, session)
	if err != nil {
		return err
	}
	// The completion pass ran when the vendor confirmed the mapping; running it
	// again reproduces the same bindings, which is the point of the engine being
	// deterministic.
	analysis.Complete()

	if err := s.imports.Progress(ctx, session.ID, 1, "جارٍ تحميل الكتالوج المركزي"); err != nil {
		s.log.WarnContext(ctx, "import progress not recorded", "import", session.PublicID, "error", err)
	}

	writer, err := s.prepareWriter(ctx, session)
	if err != nil {
		return err
	}

	content, err := s.imports.File(ctx, session.ID)
	if err != nil {
		return err
	}
	book, err := sheet.Open(content, session.Filename)
	if err != nil {
		return apperr.Validation("import.unreadable", err.Error(), nil)
	}
	defer func() { _ = book.Close() }()
	if session.Source.Sheet != "" {
		_ = book.Use(session.Source.Sheet)
	}

	opts := productmatch.DefaultProcessOptions()
	opts.Parse = parseOptionsFrom(session.Settings)
	opts.Duplicates = session.Settings.Duplicates
	opts.Vocabulary = s.vocabulary(ctx, session.OrganizationID)

	result, err := productmatch.Process(book, analysis.Layout, analysis.Mapping, opts,
		func(batch []*productmatch.Row) error { return writer.write(ctx, batch) })
	if err != nil {
		return err
	}

	applyRunResult(session, result, writer)

	if err := s.retireAbsent(ctx, session, writer); err != nil {
		return err
	}
	if err := s.imports.Finish(ctx, session); err != nil {
		return err
	}
	s.log.InfoContext(ctx, "vendor catalogue import completed",
		"import", session.PublicID, "inserted", session.InsertedRows,
		"updated", session.UpdatedRows, "skipped", session.SkippedRows,
		"errors", session.ErrorRows, "unmatched", session.UnmatchedRows)
	return nil
}

// prepareWriter loads the two indexes a run needs and returns the batch writer.
func (s *Service) prepareWriter(ctx context.Context, session *Session) (*importWriter, error) {
	products, err := s.catalog.ListMatchProducts(database.AsSystem(ctx))
	if err != nil {
		return nil, fmt.Errorf("load shared catalogue: %w", err)
	}
	master := make([]productmatch.MasterProduct, 0, len(products))
	for _, p := range products {
		master = append(master, productmatch.MasterProduct{
			ID: p.ID, NameAR: p.NameAR, NameEN: p.NameEN, SKU: p.SKU,
			Barcode: p.Barcode, Scientific: p.Scientific, DosageForm: p.DosageForm,
			Concentration: p.Concentration, Unit: p.Unit,
			Manufacturer: p.Manufacturer, PublicPrice: p.PublicPrice,
		})
	}

	keys, err := s.catalog.ListVariantKeys(ctx, session.OrganizationID)
	if err != nil {
		return nil, fmt.Errorf("load existing variants: %w", err)
	}

	matchOpts := productmatch.DefaultMatchOptions()
	matchOpts.MinStrong = session.Settings.MinMatchScore
	matchOpts.TrustSupplierCode = session.Settings.TrustSupplierCode

	return &importWriter{
		svc:      s,
		session:  session,
		settings: session.Settings,
		index:    productmatch.NewIndex(master),
		variants: newVariantIndex(keys),
		match:    matchOpts,
	}, nil
}

// retireAbsent takes the vendor's other variants off sale, for the mode that
// declares the file to be the whole catalogue.
//
// Not when the run was imperfect. The keep-list holds only the variants this
// run actually wrote, so a row that failed its write — or never resolved onto
// a catalogue product — would see its existing variant retired by the same run
// that reported the problem: one bad cell out of nine thousand delisting a
// product the vendor actively stocks. An imperfect file retires nothing; the
// vendor fixes the rows and runs it again.
func (s *Service) retireAbsent(ctx context.Context, session *Session, w *importWriter) error {
	if session.Settings.Mode != ModeReplace {
		return nil
	}
	if w.counts.errors > 0 || session.ErrorRows > 0 {
		s.log.WarnContext(ctx, "replace-mode retirement skipped: run had failed rows",
			"import", session.PublicID, "errors", w.counts.errors)
		return nil
	}
	if err := s.imports.Progress(ctx, session.ID, 99, "جارٍ إيقاف الأصناف غير الموجودة في الملف"); err != nil {
		s.log.WarnContext(ctx, "import progress not recorded", "import", session.PublicID, "error", err)
	}
	retired, err := s.catalog.DeactivateVariantsExcept(ctx, session.OrganizationID, w.touched)
	if err != nil {
		return fmt.Errorf("retire absent variants: %w", err)
	}
	if retired > 0 {
		s.log.InfoContext(ctx, "variants retired by replace-mode import",
			"import", session.PublicID, "retired", retired)
	}
	return nil
}

// applyRunResult folds the engine's account and the writer's counters onto the
// session.
func applyRunResult(session *Session, result *productmatch.Result, w *importWriter) {
	session.Stats = result.Stats
	session.Findings = result.Issues
	session.TotalRows = result.Stats.SheetRows
	session.InsertedRows = w.counts.inserted
	session.UpdatedRows = w.counts.updated
	session.SkippedRows = w.counts.skipped
	session.ErrorRows = w.counts.errors + result.Stats.Rejected
	session.MatchedRows = w.counts.matched
	session.ReviewRows = w.counts.review
	session.UnmatchedRows = w.counts.unmatched
	session.Phase = PhaseCompleted
}

// parseOptionsFrom translates the vendor's settings into reading rules.
func parseOptionsFrom(s Settings) productmatch.ParseOptions {
	return productmatch.ParseOptions{
		DefaultMinOrderQty:  s.DefaultMinOrderQty,
		DefaultMinThreshold: s.DefaultMinThreshold,
		BlankQuantityIsZero: s.BlankQuantityIsZero,
		InferDosageForm:     s.InferDosageForm,
		InferConcentration:  s.InferConcentration,
		RejectExpired:       s.RejectExpired,
	}
}

// importFailureMessage renders a run failure for the vendor.
//
// A domain error already carries a message written for them; anything else is
// ours and is prefixed rather than dressed up, because a vendor reading "تعذر
// إتمام الاستيراد" needs to know it was not their file.
func importFailureMessage(err error) string {
	if err == nil {
		return ""
	}
	var domain *apperr.Error
	if errors.As(err, &domain) && domain.Msg != "" {
		return domain.Msg
	}
	return "تعذر إتمام الاستيراد: " + err.Error()
}
