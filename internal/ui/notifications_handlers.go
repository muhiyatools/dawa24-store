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
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := pages.NotificationsDropdownPanel(nil, 0).Render(ctx, w); err != nil {
			h.log.ErrorContext(ctx, "render notifications dropdown fallback", "error", err)
		}
		return
	}

	var logs []*notifications.NotificationLog
	unread := 0
	if h.notifSvc != nil {
		logs, _ = h.notifSvc.ListUserNotifications(ctx, userID, 8, 0)
		unread, _ = h.notifSvc.GetUnreadCount(ctx, userID)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.NotificationsDropdownPanel(logs, unread).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render notifications dropdown", "error", err)
	}
}

// NotificationsUnreadBadgePartial renders the polled unread badge.
func (h *UIHandler) NotificationsUnreadBadgePartial(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID, err := authctx.UserID(ctx)
	if err != nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := pages.NotificationsUnreadBadge(0).Render(ctx, w); err != nil {
			h.log.ErrorContext(ctx, "render unread badge fallback", "error", err)
		}
		return
	}

	unread := 0
	if h.notifSvc != nil {
		unread, _ = h.notifSvc.GetUnreadCount(ctx, userID)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.NotificationsUnreadBadge(unread).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render unread badge", "error", err)
	}
}

// NotificationsReadAllSubmit marks every notification as read and returns the
// refreshed panel so the badge clears without a full page reload.
func (h *UIHandler) NotificationsReadAllSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID, err := authctx.UserID(ctx)
	if err != nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := pages.NotificationsDropdownPanel(nil, 0).Render(ctx, w); err != nil {
			h.log.ErrorContext(ctx, "render notifications dropdown fallback", "error", err)
		}
		return
	}

	if h.notifSvc != nil {
		_, _ = h.notifSvc.MarkAllRead(ctx, userID)
	}

	var logs []*notifications.NotificationLog
	if h.notifSvc != nil {
		logs, _ = h.notifSvc.ListUserNotifications(ctx, userID, 8, 0)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.NotificationsDropdownPanel(logs, 0).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render notifications dropdown panel", "error", err)
	}
}
