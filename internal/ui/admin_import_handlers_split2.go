package ui

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// AdminProductsImportProgress reports a run's state as JSON.
//
// The review screen polls it while a file is being processed. It reads the
// session rather than a map in this process: progress held only in memory
// reported "failed" for a healthy run after a restart, and reported nothing at
// all for one started by another replica — which is how a completed import came
// to render as an empty screen with no explanation.
func (h *UIHandler) AdminProductsImportProgress(w http.ResponseWriter, r *http.Request) {
	publicID := chi.URLParam(r, "id")

	if !h.requirePlatformAdmin(w, r) {
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")

	if h.catSvc == nil {
		http.Error(w, `{"phase":"failed","message":"catalog service unavailable"}`,
			http.StatusServiceUnavailable)
		return
	}

	progress, session, err := h.catSvc.SessionProgress(database.AsSystem(r.Context()), publicID)
	if err != nil {
		http.Error(w, `{"phase":"failed","message":"import session unavailable"}`, http.StatusNotFound)
		return
	}

	payload := map[string]any{
		"phase":   string(progress.Phase),
		"message": progress.Message,
		"current": progress.Current,
		"total":   progress.Total,
		"percent": progress.Percent(),
		"done":    !session.IsProcessing(),
		"failed":  progress.Phase == catalog.ImportPhaseFailed,
		"status":  string(session.Status),
		"elapsed": int(progress.Elapsed().Seconds()),
	}
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		h.log.WarnContext(r.Context(), "write import progress", "session", publicID, "error", err)
	}
}

// refreshProductIndex rebuilds the denormalised search table after an import.
//
// catalog.product_index is what the storefront and the fast search read; it is
// populated from catalog.products and does not update itself. Without this an
// admin imports nine thousand products, sees them in the admin list, and cannot
// find a single one from the customer-facing search.
func (h *UIHandler) refreshProductIndex(ctx context.Context) {
	if h.catSvc == nil {
		return
	}
	// Detached from the request: the admin should not wait on it, and a client
	// disconnect must not abort a rebuild that is already underway.
	go func() {
		bg, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Minute)
		defer cancel()
		count, err := h.catSvc.RebuildProductIndex(database.AsSystem(bg))
		if err != nil {
			h.log.ErrorContext(bg, "rebuild product index after import", "error", err)
			return
		}
		h.log.InfoContext(bg, "product index rebuilt after import", "rows", count)
	}()
}
