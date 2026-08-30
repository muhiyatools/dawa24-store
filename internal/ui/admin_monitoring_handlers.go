package ui

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/notifications"
	platformadmin "github.com/muhiya/dawa24-store/internal/modules/platform_admin"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// AdminFullErrorLogsPage renders system error logs with severity and status filters.
func (h *UIHandler) AdminFullErrorLogsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	statusFilter := r.URL.Query().Get("status")
	severityFilter := r.URL.Query().Get("severity")

	var logs []*platformadmin.ErrorLog
	var total int
	if h.adminSvc != nil {
		logs, total, _ = h.adminSvc.ListErrorLogs(database.AsSystem(ctx), platformadmin.ErrorLogFilter{
			Status: statusFilter,
			Level:  severityFilter,
			Limit:  50,
			Offset: 0,
		})
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminFullErrorLogsPage(logs, total, statusFilter, severityFilter, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render full error logs", "error", err)
	}
}

// AdminFullErrorLogDetailPage renders single diagnostic error log details.
func (h *UIHandler) AdminFullErrorLogDetailPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	idStr := chi.URLParam(r, "id")
	logID, _ := strconv.ParseInt(idStr, 10, 64)

	var logEntry *platformadmin.ErrorLog
	if h.adminSvc != nil && logID > 0 {
		logEntry, _ = h.adminSvc.GetErrorLogByID(database.AsSystem(ctx), logID)
	}

	if logEntry == nil {
		http.Redirect(w, r, "/admin/full-error-logs", http.StatusSeeOther)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminFullErrorLogDetailPage(logEntry, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin full error log detail", "error", err)
	}
}

// AdminFullActivityLogsPage renders system audit logs.
func (h *UIHandler) AdminFullActivityLogsPage(w http.ResponseWriter, r *http.Request) {
	h.AdminAuditPage(w, r)
}

// AdminFullActivityLogDetailPage renders single audit log entry.
func (h *UIHandler) AdminFullActivityLogDetailPage(w http.ResponseWriter, r *http.Request) {
	h.AdminAuditPage(w, r)
}

// AdminFullNotificationsPage renders admin notifications oversight.
func (h *UIHandler) AdminFullNotificationsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	userID, err := authctx.UserID(ctx)
	if err != nil {
		http.Redirect(w, r, "/auth/login?redirect=/admin/notifications", http.StatusSeeOther)
		return
	}

	var logs []*notifications.NotificationLog
	unread := 0
	if h.notifSvc != nil {
		logs, _ = h.notifSvc.ListUserNotifications(ctx, userID, 50, 0)
		unread, _ = h.notifSvc.GetUnreadCount(ctx, userID)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.Notifications(logs, unread, lang, dir, false).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin notifications", "error", err)
	}
}

// AdminSystemResourcesPage renders system resource availability and health.
func (h *UIHandler) AdminSystemResourcesPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	systemName := chi.URLParam(r, "system")
	if systemName == "" {
		systemName = "all"
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminSystemResourcesPage(systemName, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin system resources", "error", err)
	}
}

// AdminFirstLookPage renders admin onboarding introduction.
func (h *UIHandler) AdminFirstLookPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminFirstLookPage(lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin first look", "error", err)
	}
}

// AdminErrorLogTransitionSubmit updates an error status (INVESTIGATING, RESOLVED, IGNORED).
func (h *UIHandler) AdminErrorLogTransitionSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	idStr := chi.URLParam(r, "id")
	logID, _ := strconv.ParseInt(idStr, 10, 64)
	newStatus := r.PostFormValue("status")

	if h.adminSvc != nil && logID > 0 && newStatus != "" {
		_ = h.adminSvc.UpdateErrorLogStatus(database.AsSystem(ctx), logID, newStatus)
	}

	http.Redirect(w, r, fmt.Sprintf("/admin/full-error-logs/%d", logID), http.StatusSeeOther)
}
