package ui

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/importrun"
	"github.com/muhiya/dawa24-store/internal/platform/progress"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// ImportProgressStream is the live half of /imports/{id}/progress.
//
// Route: GET /imports/{id}/stream
//
// The JSON poll beside it stays, and stays supported: a proxy that buffers
// responses, a browser with EventSource disabled, and a network that drops the
// connection all fall back to it, and the bar cannot tell the difference
// because both deliver the same snapshot shape. What the stream changes is the
// ordinary case — the one where somebody is actually watching an import — from
// two requests a second to one connection that is written to when a number
// moves.
//
// Authorisation is done once here rather than inside the stream loop. The run
// is fetched through the same tenant-scoped repository the poll uses, so a
// stream cannot show a run its viewer could not have opened; and because the
// fetch closure re-reads through that repository on every safety tick, a
// membership revoked mid-import ends the stream at the next tick rather than
// running to completion.
func (h *UIHandler) ImportProgressStream(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Error(w, i18n.T(lang, "common.unauthorized"), http.StatusUnauthorized)
		return
	}
	publicID := chi.URLParam(r, "id")
	if publicID == "" {
		http.Error(w, "missing import id", http.StatusBadRequest)
		return
	}
	if h.importRunRepo == nil || h.progressHub == nil {
		// Nothing to stream from. Say so plainly rather than holding a
		// connection open; the client falls back to the poll.
		http.Error(w, "streaming unavailable", http.StatusServiceUnavailable)
		return
	}

	isAdmin := actor.IsPlatformAdmin()
	orgID := actor.OrganizationID

	fetch := func(ctx context.Context) (progress.Snapshot, bool) {
		var (
			run *importrun.Run
			err error
		)
		if isAdmin {
			run, err = h.importRunRepo.GetRunByPublicIDSystem(ctx, publicID)
		} else {
			run, err = h.importRunRepo.GetRunByPublicID(ctx, publicID, orgID)
		}
		if err != nil || run == nil {
			return progress.Snapshot{}, false
		}
		return snapshotOfRun(run, lang), true
	}

	progress.Stream(w, r, h.progressHub, publicID, fetch)
}

// snapshotOfRun renders a durable run as the shape the bar consumes.
//
// The phase caption is translated here, at the edge, rather than stored on the
// run: the same import can be watched by an Arabic screen and an English one,
// and a caption baked into the row would be wrong for one of them.
func snapshotOfRun(run *importrun.Run, lang string) progress.Snapshot {
	s := progress.Snapshot{
		ID:      run.PublicID,
		Percent: run.Percent,
		Message: importPhaseLabel(run, lang),
		Current: run.ProcessedRows,
		Total:   run.TotalRows,
		State:   run.State,
		Done:    run.IsDone(),
		Error:   run.ErrorMessage,
		At:      run.UpdatedAt,
	}
	// "Ready" ends the phase the viewer is watching.
	//
	// A saving-products or catalogue import runs in two acts: it stages and
	// matches, the vendor reviews what it found, and only then does it commit.
	// `ready` is the end of the first act — the processing bar has genuinely
	// finished — but importrun.IsDone is about the run as a whole and says
	// false there. A stream that took its word for it held the connection open
	// at ninety-nine per cent while the review screen sat waiting to be drawn.
	//
	// The commit is watched separately when the vendor starts it, and its
	// states (committing → committed) come through the same stream.
	if run.State == importrun.StateReady {
		s.Done = true
	}
	// 100 is written by a terminal state and never inferred. Every "stuck at
	// 100%" report came from arithmetic reaching the end of the last band while
	// a commit was still writing.
	if s.Done && !s.IsFailure() {
		s.Percent = 100
	}
	return s
}

// importPhaseLabel is the caption under the bar.
//
// The workers write the phase as a ready-made Arabic sentence rather than as a
// translatable key ("جارٍ مطابقة الأصناف..."), so it is shown as written. A
// state caption stands in while a run has not reached a phase yet, because a
// blank line under a moving bar reads as a stall.
func importPhaseLabel(run *importrun.Run, lang string) string {
	if run.State == importrun.StateFailed && run.ErrorMessage != "" {
		return run.ErrorMessage
	}
	if run.Phase != "" {
		return run.Phase
	}
	switch run.State {
	case importrun.StateQueued:
		return i18n.T(lang, "import.state.queued")
	case importrun.StateProcessing:
		return i18n.T(lang, "import.state.processing")
	case importrun.StateReady:
		return i18n.T(lang, "import.state.ready")
	case importrun.StateCommitting:
		return i18n.T(lang, "import.state.committing")
	case importrun.StateCommitted:
		return i18n.T(lang, "import.state.committed")
	case importrun.StateFailed:
		return i18n.T(lang, "import.state.failed")
	case importrun.StateCancelled:
		return i18n.T(lang, "import.state.cancelled")
	}
	return run.State
}
