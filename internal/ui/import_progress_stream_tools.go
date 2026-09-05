package ui

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/modules/ingest"
	"github.com/muhiya/dawa24-store/internal/modules/smartorder"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/progress"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/importprogress"
)

// The live half of the two import screens that do not use durable runs.
//
// The vendor catalogue import and the administrative catalogue import each keep
// their own session row rather than a platform.import_runs record, so they
// cannot share /imports/{id}/stream — the run it looks for does not exist. They
// get their own endpoints here, feeding the same hub from the same decorators
// (internal/modules/ingest and internal/modules/catalog), so all four import
// tools now behave identically from the browser's point of view.
//
// Each keeps its JSON poll beside the stream, and the shared bar falls back to
// it by itself when the stream cannot be established.

// VendorIngestProgressStream streams one vendor import's progress.
//
// Route: GET /vendor/ingest/{id}/stream
//
// The hub key is the session's INTERNAL id — see ingest.ProgressKey for why —
// so this resolves the public id once, here, where the session has to be loaded
// for authorisation anyway.
func (h *UIHandler) VendorIngestProgressStream(w http.ResponseWriter, r *http.Request) {
	publicID := chi.URLParam(r, "id")
	if publicID == "" || h.ingSvc == nil || h.progressHub == nil {
		http.Error(w, "streaming unavailable", http.StatusServiceUnavailable)
		return
	}

	session, err := h.ingSvc.LoadImport(r.Context(), publicID)
	if err != nil || session == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	fetch := func(ctx context.Context) (progress.Snapshot, bool) {
		s, err := h.ingSvc.LoadImport(ctx, publicID)
		if err != nil || s == nil {
			return progress.Snapshot{}, false
		}
		return ingestSnapshot(s), true
	}

	progress.Stream(w, r, h.progressHub, ingest.ProgressKey(session.ID), fetch)
}

// ingestSnapshot renders a vendor import session for the bar.
//
// "done" means the run has STOPPED, not that the import is finished. A staging
// pass ends in 'review', and a bar waiting for a terminal phase sat at
// ninety-nine per cent while the review screen was already built — the same bug
// the JSON endpoint beside this one had to fix.
func ingestSnapshot(s *ingest.Session) progress.Snapshot {
	running := s.Phase == ingest.PhaseProcessing
	percent := s.ProgressPercent
	switch {
	case !running && s.Phase != ingest.PhaseFailed:
		percent = 100
	case percent >= 100:
		percent = 99
	}
	return progress.Snapshot{
		ID:      ingest.ProgressKey(s.ID),
		Percent: percent,
		Message: s.ProgressNote,
		State:   string(s.Phase),
		Done:    !running,
		Error:   s.ErrorMessage,
	}
}

// AdminProductsImportProgressStream streams one administrative import's
// progress.
//
// Route: GET /admin/products/import/{id}/stream
//
// Keyed on the public id, which is what catalog.SaveImportProgress is given and
// therefore what its decorator publishes under.
func (h *UIHandler) AdminProductsImportProgressStream(w http.ResponseWriter, r *http.Request) {
	publicID := chi.URLParam(r, "id")
	if !h.requirePlatformAdmin(w, r) {
		return
	}
	if publicID == "" || h.catSvc == nil || h.progressHub == nil {
		http.Error(w, "streaming unavailable", http.StatusServiceUnavailable)
		return
	}

	fetch := func(ctx context.Context) (progress.Snapshot, bool) {
		p, session, err := h.catSvc.SessionProgress(database.AsSystem(ctx), publicID)
		if err != nil || session == nil {
			return progress.Snapshot{}, false
		}
		return progress.Snapshot{
			ID:      publicID,
			Percent: p.Percent(),
			Message: p.Message,
			Current: p.Current,
			Total:   p.Total,
			State:   string(session.Status),
			Done:    !session.IsProcessing(),
			Error:   adminImportFailure(p),
			At:      p.UpdatedAt,
		}, true
	}

	progress.Stream(w, r, h.progressHub, publicID, fetch)
}

// adminImportFailure reports the reason only when the phase says the run
// failed. The message field carries the phase label the rest of the time, and
// reporting that as an error would stop the bar on every ordinary tick.
func adminImportFailure(p catalog.ImportProgress) string {
	if p.Phase == catalog.ImportPhaseFailed {
		return p.Message
	}
	return ""
}

// SmartOrderProgressStream streams one smart-order run's progress.
//
// Route: GET /customer/smart-order/{id}/stream
//
// Keyed on the run's internal id — see smartorder.ProgressKey — which this
// resolves from the run it must load for authorisation anyway.
func (h *UIHandler) SmartOrderProgressStream(w http.ResponseWriter, r *http.Request) {
	if h.progressHub == nil || h.smartOrderSvc == nil {
		http.Error(w, "streaming unavailable", http.StatusServiceUnavailable)
		return
	}
	run, ok := h.smartOrderRun(w, r)
	if !ok {
		return
	}
	lang, _ := h.localeAndDir(r)
	runID, orgID, publicID := run.ID, run.OrganizationID, run.PublicID

	// Re-read through Get, which is tenant-scoped: a stream must not outlive
	// the viewer's right to see the run it is streaming.
	fetch := func(ctx context.Context) (progress.Snapshot, bool) {
		current, err := h.smartOrderSvc.Get(ctx, orgID, publicID)
		if err != nil || current == nil {
			return progress.Snapshot{}, false
		}
		events, _ := h.smartOrderSvc.Events(ctx, runID, 0)
		return smartOrderSnapshot(current, events, lang), true
	}

	progress.Stream(w, r, h.progressHub, smartorder.ProgressKey(runID), fetch)
}

// smartOrderSnapshot renders a run for the bar, with the same rules the JSON
// poll beside it uses: the run is over when the RUN says so, and the arithmetic
// may never reach 100 on its own while a finalise step is still writing.
func smartOrderSnapshot(run *smartorder.Run, events []*smartorder.Event, lang string) progress.Snapshot {
	percent := smartorder.RunPercent(events)
	caption := i18n.T(lang, "smartorder.staging_caption")
	if stage := smartorder.CurrentStage(events); stage != "" {
		caption = stage.Label()
	}

	done := false
	switch run.Status {
	case smartorder.StatusCompleted, smartorder.StatusStale,
		smartorder.StatusPlaced, smartorder.StatusFailed:
		done = true
		if run.Status != smartorder.StatusFailed {
			percent = importprogress.Complete
		}
	default:
		if percent >= importprogress.Complete {
			percent = importprogress.Complete - 1
		}
		if percent <= 0 {
			percent = 2
		}
	}

	return progress.Snapshot{
		ID:      smartorder.ProgressKey(run.ID),
		Percent: percent,
		Message: caption,
		State:   string(run.Status),
		Done:    done,
		Error:   run.FailureReason,
	}
}
