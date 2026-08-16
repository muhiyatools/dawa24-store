package ui

import (
	"net/http"

	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

func (h *UIHandler) AdminDashboardPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	stats := pages.AdminDashboardStats{}

	if h.idSvc != nil {
		if users, err := h.idSvc.AdminListUsers(ctx, "", ""); err == nil {
			stats.TotalUsers = len(users)
		}
	}
	if h.orgSvc != nil {
		if orgs, err := h.orgSvc.ListOrganizations(ctx, nil, nil, 100, 0); err == nil {
			stats.TotalOrganizations = len(orgs)
		}
		pending := org.StatusPending
		if pendingOrgs, err := h.orgSvc.ListOrganizations(ctx, nil, &pending, 100, 0); err == nil {
			stats.PendingApprovals = len(pendingOrgs)
		}
	}
	if h.commSvc != nil {
		if orders, err := h.commSvc.ListCustomerOrders(ctx, 0, 100, 0); err == nil {
			stats.TotalOrders = len(orders)
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminDashboard(stats, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin dashboard page", "error", err)
	}
}

func (h *UIHandler) AdminUsersPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	if h.idSvc == nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = pages.AdminUsers(nil, lang, dir, h.isHTMX(r)).Render(ctx, w)
		return
	}

	users, err := h.idSvc.AdminListUsers(ctx, "", "")
	if err != nil {
		h.renderError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminUsers(users, lang, dir, h.isHTMX(r)).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin users page", "error", err)
	}
}

func (h *UIHandler) AdminApprovalsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	var pending []*org.Organization
	if h.orgSvc != nil {
		pendingStatus := org.StatusPending
		list, err := h.orgSvc.ListOrganizations(ctx, nil, &pendingStatus, 50, 0)
		if err == nil {
			pending = list
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminApprovals(pending, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin approvals page", "error", err)
	}
}

func (h *UIHandler) AdminSettingsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminSettings(lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin settings page", "error", err)
	}
}

func (h *UIHandler) AdminSettingsSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	h.log.InfoContext(ctx, "admin updated platform settings", "support_email", r.PostFormValue("support_email"))
	http.Redirect(w, r, "/admin/settings?saved=true", http.StatusSeeOther)
}

