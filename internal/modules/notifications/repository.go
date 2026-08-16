package notifications

import (
	"context"
)

// Repository defines storage operations for notification templates and message logs.
type Repository interface {
	CreateLog(ctx context.Context, log *NotificationLog) error
	GetTemplateBySlug(ctx context.Context, slug string) (*Template, error)
	ListUserNotifications(ctx context.Context, userID int64, limit, offset int) ([]*NotificationLog, error)
	MarkAsRead(ctx context.Context, id int64, userID int64) error
	GetUnreadCount(ctx context.Context, userID int64) (int, error)
}
