package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/notifications"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// MarkAllAsRead clears every unread notification for one user.
//
// Scoped by user_id in the statement itself, not by a caller-supplied filter:
// notifications are per-user, not per-organization, so row-level security has
// nothing to say about them. The predicate here is the whole protection.
func (r *Repository) MarkAllAsRead(ctx context.Context, userID int64) (int64, error) {
	var affected int64
	err := r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			UPDATE notifications.logs
			SET is_read = true, read_at = now()
			WHERE user_id = $1 AND is_read = false;
		`
		res, err := tx.Exec(txCtx, query, userID)
		if err != nil {
			return err
		}
		affected = res.RowsAffected()
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("notifications postgres: mark all read: %w", err)
	}
	return affected, nil
}

// ListUnread returns unread notifications for one user, newest first.
func (r *Repository) ListUnread(ctx context.Context, userID int64, limit, offset int) ([]*notifications.NotificationLog, error) {
	var list []*notifications.NotificationLog
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, public_id, user_id, organization_id, channel, recipient, title, body,
			       status, error_message, is_read, read_at, sent_at, created_at
			FROM notifications.logs
			WHERE user_id = $1 AND is_read = false
			ORDER BY created_at DESC, id DESC
			LIMIT $2 OFFSET $3;
		`
		rows, err := tx.Query(txCtx, query, userID, limit, offset)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var n notifications.NotificationLog
			if err := rows.Scan(
				&n.ID, &n.PublicID, &n.UserID, &n.OrganizationID, &n.Channel,
				&n.Recipient, &n.Title, &n.Body, &n.Status, &n.ErrorMessage,
				&n.IsRead, &n.ReadAt, &n.SentAt, &n.CreatedAt,
			); err != nil {
				return err
			}
			list = append(list, &n)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("notifications postgres: list unread: %w", err)
	}
	return list, nil
}
