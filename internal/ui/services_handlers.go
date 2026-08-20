package ui

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	platformadmin "github.com/muhiya/dawa24-store/internal/modules/platform_admin"
	"github.com/muhiya/dawa24-store/internal/modules/workflow"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/features"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// ServicesPage renders the institutional services catalogue.
func (h *UIHandler) ServicesPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !features.Enabled(ctx, "services.enabled") {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	lang, dir := h.localeAndDir(r)

	var services []*workflow.InstitutionalService
	if h.wfSvc != nil {
		services, _ = h.wfSvc.ListServices(ctx, nil)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.ServicesPage(lang, dir, services).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render services page", "error", err)
	}
}

// ServiceDetailPage renders one institutional service.
func (h *UIHandler) ServiceDetailPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || h.wfSvc == nil {
		h.renderError(w, r, err)
		return
	}

	s, err := h.wfSvc.GetService(ctx, id)
	if err != nil {
		h.renderError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.ServiceDetail(lang, dir, s).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render service detail", "error", err)
	}
}

// ServiceRequestSubmit records an institutional-service request as a platform
// contact inquiry so admins see it in /admin/messages.
func (h *UIHandler) ServiceRequestSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if h.wfSvc == nil || h.adminSvc == nil {
		h.redirectWithNotice(w, r, "/services", "error", "الخدمة غير متاحة حالياً.")
		return
	}

	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	svc, err := h.wfSvc.GetService(ctx, id)
	if err != nil {
		h.redirectWithNotice(w, r, "/services", "error", "الخدمة غير موجودة.")
		return
	}

	name, email := "زائر", "guest@dawa24.app"
	if actor, ok := authctx.From(ctx); ok {
		name = fmt.Sprintf("user-%d", actor.UserID)
		email = fmt.Sprintf("user-%d@dawa24.app", actor.UserID)
	}

	_ = h.adminSvc.SubmitContactMessage(ctx, &platformadmin.ContactMessage{
		Name:    name,
		Email:   email,
		Subject: "طلب خدمة مؤسسية",
		Message: "طلب خدمة: " + svc.Title.Get(i18n.AR),
	})

	h.redirectWithNotice(w, r, "/services/"+strconv.FormatInt(id, 10), "success", "تم استلام طلب الخدمة وسيتواصل معك فريق الدعم.")
}
