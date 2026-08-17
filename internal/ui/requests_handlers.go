package ui

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/modules/workflow"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// RequestsPage renders the document/action request inbox.
func (h *UIHandler) RequestsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/requests", http.StatusSeeOther)
		return
	}

	data := pages.RequestsData{CurrentOrgID: actor.OrganizationID}
	if h.wfSvc != nil && actor.OrganizationID > 0 {
		data.Requests, _ = h.wfSvc.ListInbox(ctx, actor.OrganizationID, r.URL.Query().Get("status"), 50, 0)
	}
	if h.orgSvc != nil {
		typ := org.TypeSupplier
		status := org.StatusApproved
		data.Suppliers, _ = h.orgSvc.ListOrganizations(ctx, &typ, &status, 50, 0)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.RequestsPage(lang, dir, r.URL.Query().Get("status"), data).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render requests page", "error", err)
	}
}

// RequestCreateSubmit sends a document/action request to another organization.
func (h *UIHandler) RequestCreateSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/requests", http.StatusSeeOther)
		return
	}

	toOrgID, _ := strconv.ParseInt(r.PostFormValue("to_org_id"), 10, 64)
	title := i18n.New(r.PostFormValue("title"), r.PostFormValue("title"))
	if h.wfSvc == nil {
		h.redirectWithNotice(w, r, "/requests", "error", "الخدمة غير متاحة حالياً.")
		return
	}

	_, err := h.wfSvc.CreateRequest(ctx, actor.UserID, actor.OrganizationID, toOrgID,
		workflow.RequestType(r.PostFormValue("type")), title, r.PostFormValue("description"))
	if err != nil {
		h.redirectWithNotice(w, r, "/requests", "error", h.safeMessage(err, langOf(r)))
		return
	}
	h.redirectWithNotice(w, r, "/requests", "success", "تم إرسال الطلب.")
}

// RequestRespondSubmit accepts or declines an incoming request.
func (h *UIHandler) RequestRespondSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/requests", http.StatusSeeOther)
		return
	}

	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	status := workflow.RequestStatus(r.PostFormValue("status"))
	if h.wfSvc != nil && id > 0 {
		_ = h.wfSvc.RespondRequest(ctx, id, status)
	}
	http.Redirect(w, r, "/requests", http.StatusSeeOther)
}
