package notifications

import (
	"context"
	"log/slog"
	"time"

	"github.com/muhiya/dawa24-store/internal/shared/apperr"

	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// SendInput specifies parameters for dispatching a notification.
type SendInput struct {
	UserID         int64
	OrganizationID *int64
	Channel        Channel
	Recipient      string
	Title          string
	Body           string
}

// Service manages multi-channel message dispatching and notification delivery auditing.
type Service struct {
	repo Repository
	log  *slog.Logger
}

// NewService creates a new notification service.
func NewService(repo Repository, log *slog.Logger) *Service {
	return &Service{
		repo: repo,
		log:  log,
	}
}

// Send records and dispatches a notification.
func (s *Service) Send(ctx context.Context, input SendInput) (*NotificationLog, error) {
	now := time.Now().UTC()
	l := &NotificationLog{
		UserID:         input.UserID,
		OrganizationID: input.OrganizationID,
		Channel:        input.Channel,
		Recipient:      input.Recipient,
		Title:          input.Title,
		Body:           input.Body,
		Status:         StatusSent,
		SentAt:         &now,
	}

	if err := l.Validate(); err != nil {
		return nil, err
	}

	if err := s.repo.CreateLog(ctx, l); err != nil {
		return nil, err
	}

	s.log.InfoContext(ctx, "notification sent", "user_id", l.UserID, "channel", l.Channel, "title", l.Title)
	return l, nil
}

// SendTemplated retrieves a localized template and interpolates parameters before sending.
func (s *Service) SendTemplated(
	ctx context.Context,
	userID int64,
	recipient string,
	templateSlug string,
	vars map[string]string,
) (*NotificationLog, error) {
	tmpl, err := s.repo.GetTemplateBySlug(ctx, templateSlug)
	if err != nil {
		return nil, err
	}

	title := InterpolateTemplate(tmpl.Title.Get(i18n.AR), vars)
	body := InterpolateTemplate(tmpl.Body.Get(i18n.AR), vars)

	var orgIDPtr *int64
	if orgID, ok := database.TenantFrom(ctx); ok {
		orgIDPtr = &orgID
	}

	return s.Send(ctx, SendInput{
		UserID:         userID,
		OrganizationID: orgIDPtr,
		Channel:        tmpl.Channel,
		Recipient:      recipient,
		Title:          title,
		Body:           body,
	})
}

// ListUserNotifications returns paginated notification feed.
func (s *Service) ListUserNotifications(ctx context.Context, userID int64, limit, offset int) ([]*NotificationLog, error) {
	return s.repo.ListUserNotifications(ctx, userID, limit, offset)
}

// MarkRead flags a notification as seen.
func (s *Service) MarkRead(ctx context.Context, id int64, userID int64) error {
	return s.repo.MarkAsRead(ctx, id, userID)
}

// MarkAllRead clears the caller's entire notification feed.
func (s *Service) MarkAllRead(ctx context.Context, userID int64) (int64, error) {
	if userID <= 0 {
		return 0, apperr.Unauthorized()
	}
	return s.repo.MarkAllAsRead(ctx, userID)
}

// ListUnread returns only unread notifications, for the badge dropdown.
func (s *Service) ListUnread(ctx context.Context, userID int64, limit, offset int) ([]*NotificationLog, error) {
	if userID <= 0 {
		return nil, apperr.Unauthorized()
	}
	return s.repo.ListUnread(ctx, userID, limit, offset)
}

// GetUnreadCount returns total unread messages.
func (s *Service) GetUnreadCount(ctx context.Context, userID int64) (int, error) {
	return s.repo.GetUnreadCount(ctx, userID)
}
