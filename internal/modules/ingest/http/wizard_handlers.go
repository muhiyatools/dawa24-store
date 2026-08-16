package http

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/httpx"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// ListSessions lists import sessions for the current tenant.
func (h *Handler) ListSessions(w http.ResponseWriter, r *http.Request) {
	orgID, ok := database.TenantFrom(r.Context())
	if !ok {
		httpx.Error(w, r, h.log, database.ErrNoTenant)
		return
	}

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	sessions, err := h.service.ListSessions(r.Context(), orgID, limit, offset)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"sessions": sessions, "count": len(sessions)})
}

// ListRows lists staged rows for review.
func (h *Handler) ListRows(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		httpx.Error(w, r, h.log, apperr.Validation("id.invalid", "Invalid session ID", nil))
		return
	}

	status := r.URL.Query().Get("status")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	rows, err := h.service.ListImportRows(r.Context(), id, status, limit, offset)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"rows": rows, "count": len(rows)})
}

// UpdateMapping updates column mapping for a session.
func (h *Handler) UpdateMapping(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		httpx.Error(w, r, h.log, apperr.Validation("id.invalid", "Invalid session ID", nil))
		return
	}

	var body struct {
		Mapping map[string]string `json:"mapping"`
	}
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	if err := h.service.UpdateColumnMapping(r.Context(), id, body.Mapping); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// CommitSession marks import session committed.
func (h *Handler) CommitSession(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		httpx.Error(w, r, h.log, apperr.Validation("id.invalid", "Invalid session ID", nil))
		return
	}

	if err := h.service.CommitSession(r.Context(), id); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "completed"})
}

// CancelSession marks import session cancelled.
func (h *Handler) CancelSession(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		httpx.Error(w, r, h.log, apperr.Validation("id.invalid", "Invalid session ID", nil))
		return
	}

	if err := h.service.CancelSession(r.Context(), id); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "cancelled"})
}

// OverrideRowMatch overrides product match for a staged row.
func (h *Handler) OverrideRowMatch(w http.ResponseWriter, r *http.Request) {
	rid, err := strconv.ParseInt(chi.URLParam(r, "rid"), 10, 64)
	if err != nil {
		httpx.Error(w, r, h.log, apperr.Validation("rid.invalid", "Invalid row ID", nil))
		return
	}

	var body struct {
		ProductID int64 `json:"product_id"`
	}
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	if err := h.service.OverrideRowMatch(r.Context(), rid, body.ProductID); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// StreamEvents handles Server-Sent Events (SSE) for real-time import progress.
func (h *Handler) StreamEvents(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		httpx.Error(w, r, h.log, apperr.Validation("id.invalid", "Invalid session ID", nil))
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		httpx.Error(w, r, h.log, apperr.New(apperr.KindUnavailable, "sse.unsupported", "Streaming unsupported"))
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			session, err := h.service.GetSessionProgress(r.Context(), id)
			if err != nil {
				return
			}
			data, _ := json.Marshal(session)
			fmt.Fprintf(w, "event: progress\ndata: %s\n\n", data)
			flusher.Flush()

			if session.Status == "completed" || session.Status == "failed" {
				return
			}
		}
	}
}
