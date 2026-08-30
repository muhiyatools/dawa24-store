package ui

import (
	"net/http"

	"github.com/muhiya/dawa24-store/internal/modules/notifications"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// NotificationsDropdownPartial renders the bell dropdown panel as an HTMX partial.
func (h *UIHandler) NotificationsDropdownPartial(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID, err := authctx.UserID(ctx)
	if err != nil {
		h.renderPage(ctx, w, "render notifications dropdown fallback", pages.NotificationsDropdownPanel(nil, 0))
		return
	}

	var logs []*notifications.NotificationLog
	unread := 0
	if h.notifSvc != nil {
		logs, _ = h.notifSvc.ListUserNotifications(ctx, userID, 8, 0)
		unread, _ = h.notifSvc.GetUnreadCount(ctx, userID)
	}

	h.renderPage(ctx, w, "render notifications dropdown", pages.NotificationsDropdownPanel(logs, unread))
}

// NotificationsUnreadBadgePartial renders the polled unread badge.
func (h *UIHandler) NotificationsUnreadBadgePartial(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID, err := authctx.UserID(ctx)
	if err != nil {
		h.renderPage(ctx, w, "render unread badge fallback", pages.NotificationsUnreadBadge(0))
		return
	}

	unread := 0
	if h.notifSvc != nil {
		unread, _ = h.notifSvc.GetUnreadCount(ctx, userID)
	}

	h.renderPage(ctx, w, "render unread badge", pages.NotificationsUnreadBadge(unread))
}

// NotificationsReadAllSubmit marks every notification as read and returns the
// refreshed panel so the badge clears without a full page reload.
func (h *UIHandler) NotificationsReadAllSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID, err := authctx.UserID(ctx)
	if err != nil {
		h.renderPage(ctx, w, "render notifications dropdown fallback", pages.NotificationsDropdownPanel(nil, 0))
		return
	}

	if h.notifSvc != nil {
		_, _ = h.notifSvc.MarkAllRead(ctx, userID)
	}

	var logs []*notifications.NotificationLog
	if h.notifSvc != nil {
		logs, _ = h.notifSvc.ListUserNotifications(ctx, userID, 8, 0)
	}

	h.renderPage(ctx, w, "render notifications dropdown panel", pages.NotificationsDropdownPanel(logs, 0))
}
