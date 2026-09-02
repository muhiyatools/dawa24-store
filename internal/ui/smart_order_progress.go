package ui

import (
	"encoding/json"
	"net/http"

	"github.com/muhiya/dawa24-store/internal/modules/smartorder"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/importprogress"
)

// The smart order progress poll.
//
// Split out of smart_order_pages.go, which had grown past the 400-line limit
// AGENTS.md sets.

// SmartOrderProgressJSON is the poll behind the progress ring.
//
// The page used to refresh itself whole every three seconds with a
// <meta http-equiv="refresh">, which reloads the shell, the sidebar and the
// assistant to move one number, loses the scroll position each time, and makes
// the ring advance in visible three-second jumps. It polls this instead and
// eases between the values, using the same bar every other import tool uses.
func (h *UIHandler) SmartOrderProgressJSON(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, _ := h.localeAndDir(r)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")

	run, ok := h.smartOrderRun(w, r)
	if !ok {
		return
	}

	events, _ := h.smartOrderSvc.Events(ctx, run.ID, 0)
	percent := smartorder.RunPercent(events)
	caption := i18n.T(lang, "smartorder.staging_caption")
	if stage := smartorder.CurrentStage(events); stage != "" {
		caption = stage.Label()
	}

	// The run is over when the run says so, never when the arithmetic reaches
	// the end of the last band. A percentage that hits 100 while the finalise
	// step is still writing is the bug this whole pass exists to remove.
	done := false
	switch run.Status {
	case smartorder.StatusCompleted, smartorder.StatusStale,
		smartorder.StatusPlaced, smartorder.StatusFailed:
		done = true
		percent = importprogress.Complete
	default:
		if percent >= importprogress.Complete {
			percent = importprogress.Complete - 1
		}
		if percent <= 0 {
			percent = 2
		}
	}

	payload := map[string]any{
		"percent": percent,
		"message": caption,
		"status":  run.Status,
		"done":    done,
		"failed":  run.Status == smartorder.StatusFailed,
		"error":   run.FailureReason,
	}
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		h.log.WarnContext(ctx, "smart order progress encode failed", "error", err)
	}
}
