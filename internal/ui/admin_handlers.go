package ui

import (
	"net/http"

	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

func (h *UIHandler) AdminDashboardPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	// Every figure here used to be len() of a page capped at 100 rows, so the
	// dashboard silently stopped counting at 100 and reported "100 users" to a
	// platform with a thousand. Totals come from COUNT queries.
	stats := pages.AdminDashboardStats{}

	if h.idSvc != nil {
		if n, err := h.idSvc.AdminCountUsers(ctx); err != nil {
			h.log.ErrorContext(ctx, "dashboard: count users", "error", err)
		} else {
			stats.TotalUsers = n
		}
	}
	if h.orgSvc != nil {
		if n, err := h.orgSvc.CountOrganizations(ctx, nil, nil); err != nil {
			h.log.ErrorContext(ctx, "dashboard: count organizations", "error", err)
		} else {
			stats.TotalOrganizations = n
		}
		pending := org.StatusPending
		if n, err := h.orgSvc.CountOrganizations(ctx, nil, &pending); err != nil {
			h.log.ErrorContext(ctx, "dashboard: count pending organizations", "error", err)
		} else {
			stats.PendingApprovals = n
		}
	}
	if h.commSvc != nil {
		if n, err := h.commSvc.CountOrders(ctx); err != nil {
			h.log.ErrorContext(ctx, "dashboard: count orders", "error", err)
		} else {
			stats.TotalOrders = n
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
