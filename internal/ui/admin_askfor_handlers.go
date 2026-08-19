package ui

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/workflow"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// AdminAskForPage renders the list of administrative document & action requests.
func (h *UIHandler) AdminAskForPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	var reqs []*workflow.Request
	if h.wfSvc != nil {
		// AsSystem justified: platform administrator managing all cross-organization document requests
		reqs, _ = h.wfSvc.ListInbox(database.AsSystem(ctx), 0, "", 100, 0)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminAskForPage(reqs, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render ask-for page", "error", err)
	}
}

// AdminAskForDetailPage renders details of an action/document request.
func (h *UIHandler) AdminAskForDetailPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	idStr := chi.URLParam(r, "id")
	reqID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || reqID <= 0 {
		http.Redirect(w, r, "/admin/ask-for", http.StatusSeeOther)
		return
	}

	var req *workflow.Request
	if h.wfSvc != nil {
		reqs, _ := h.wfSvc.ListInbox(database.AsSystem(ctx), 0, "", 100, 0)
		for _, rCand := range reqs {
			if rCand.ID == reqID {
				req = rCand
				break
			}
		}
	}

	if req == nil {
		http.Redirect(w, r, "/admin/ask-for", http.StatusSeeOther)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = pages.AdminAskForDetailPage(req, lang, dir).Render(ctx, w)
}

// AdminAskForRespondSubmit handles admin accepting/declining/cancelling request.
func (h *UIHandler) AdminAskForRespondSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	idStr := chi.URLParam(r, "id")
	reqID, _ := strconv.ParseInt(idStr, 10, 64)
	statusStr := r.PostFormValue("status")

	if h.wfSvc != nil && reqID > 0 {
		_ = h.wfSvc.RespondRequest(database.AsSystem(ctx), reqID, workflow.RequestStatus(statusStr))
	}

	http.Redirect(w, r, "/admin/ask-for", http.StatusSeeOther)
}
