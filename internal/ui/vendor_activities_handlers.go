package ui

import (
	"net/http"

	platformadmin "github.com/muhiya/dawa24-store/internal/modules/platform_admin"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// VendorActivitiesPage renders the organization's employee activity logs.
func (h *UIHandler) VendorActivitiesPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/activities", http.StatusSeeOther)
		return
	}

	var entries []*platformadmin.AuditEntry
	if h.adminSvc != nil {
		entries, _ = h.adminSvc.ListAuditLogByOrg(ctx, actor.OrganizationID, 50, 0)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.VendorActivitiesPage(entries, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render vendor activities", "error", err)
	}
}
