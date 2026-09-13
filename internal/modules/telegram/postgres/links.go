// Package postgres implements telegram.Repository.
//
// Every statement names the user or link it acts on in its WHERE clause.
// These tables are closed to everything but system callers (migration 213),
// and the service reaches them through database.AsSystem, so the predicates
// here are the whole of the scoping — there is no tenant GUC to fall back on.
package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/muhiya/dawa24-store/internal/modules/telegram"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// Repository implements telegram.Repository.
type Repository struct {
	db *database.DB
}

// New constructs the repository.
func New(db *database.DB) *Repository { return &Repository{db: db} }

var _ telegram.Repository = (*Repository)(nil)

const linkColumns = `
	id, public_id::text, user_id, telegram_user_id, chat_id, username, display_name,
	status, active_organization_id, conversation_id, muted_categories,
	confirm_expires_at, confirmed_at`

const liveStatuses = `('pending', 'active', 'blocked')`

func scanLink(row pgx.Row) (*telegram.Link, error) {
	var (
		l      telegram.Link
		status string
	)
	err := row.Scan(&l.ID, &l.PublicID, &l.UserID, &l.TelegramUserID, &l.ChatID, &l.Username, &l.DisplayName,
		&status, &l.ActiveOrgID, &l.ConversationID, &l.MutedCategories,
		&l.ConfirmExpiresAt, &l.ConfirmedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	l.Status = telegram.LinkStatus(status)
	return &l, nil
}

// CountLinkTokensSince counts codes a user created since a time.
func (r *Repository) CountLinkTokensSince(ctx context.Context, userID int64, since time.Time) (int, error) {
	var n int
	err := r.db.InReadTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT count(*) FROM telegram.link_tokens WHERE user_id = $1 AND created_at >= $2;`,
			userID, since).Scan(&n)
	})
	return n, err
}

// CreateLinkToken stores a code's hash and retires the user's older codes.
func (r *Repository) CreateLinkToken(ctx context.Context, userID int64, orgID *int64, hash []byte, expires time.Time) error {
	return r.db.InTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			UPDATE telegram.link_tokens SET expires_at = now()
			 WHERE user_id = $1 AND consumed_at IS NULL AND expires_at > now();`, userID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO telegram.link_tokens (user_id, organization_id, token_hash, expires_at)
			VALUES ($1, $2, $3, $4);`, userID, orgID, hash, expires)
		return err
	})
}

// ConsumeLinkToken spends a code exactly once. The UPDATE is the check: two
// simultaneous uses race on one row and only one of them gets it back.
func (r *Repository) ConsumeLinkToken(ctx context.Context, hash []byte) (int64, *int64, error) {
	var (
		userID int64
		orgID  *int64
	)
	err := r.db.InTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			UPDATE telegram.link_tokens SET consumed_at = now()
			 WHERE token_hash = $1 AND consumed_at IS NULL AND expires_at > now()
			RETURNING user_id, organization_id;`, hash).Scan(&userID, &orgID)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil, telegram.ErrTokenInvalid
	}
	return userID, orgID, err
}

// LiveLinkByTelegramUser returns the non-revoked link for a Telegram account.
func (r *Repository) LiveLinkByTelegramUser(ctx context.Context, telegramUserID int64) (*telegram.Link, error) {
	var link *telegram.Link
	err := r.db.InReadTx(ctx, func(ctx context.Context, tx pgx.Tx) (err error) {
		link, err = scanLink(tx.QueryRow(ctx, `SELECT `+linkColumns+`
			  FROM telegram.links
			 WHERE telegram_user_id = $1 AND status IN `+liveStatuses+`
			 LIMIT 1;`, telegramUserID))
		return err
	})
	return link, err
}

// CurrentLinkForUser returns what the settings page shows: an unexpired
// pending link awaiting confirmation first, otherwise the confirmed one.
func (r *Repository) CurrentLinkForUser(ctx context.Context, userID int64) (*telegram.Link, error) {
	var link *telegram.Link
	err := r.db.InReadTx(ctx, func(ctx context.Context, tx pgx.Tx) (err error) {
		link, err = scanLink(tx.QueryRow(ctx, `SELECT `+linkColumns+`
			  FROM telegram.links
			 WHERE user_id = $1
			   AND (status IN ('active', 'blocked')
			        OR (status = 'pending' AND confirm_expires_at > now()))
			 ORDER BY (status = 'pending') DESC, created_at DESC
			 LIMIT 1;`, userID))
		return err
	})
	return link, err
}

// CreatePendingLink records that a Telegram account opened a user's code.
func (r *Repository) CreatePendingLink(ctx context.Context, l *telegram.Link, confirmBy time.Time) error {
	err := r.db.InTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var (
			existingID     int64
			existingUser   int64
			existingStatus string
		)
		err := tx.QueryRow(ctx, `
			SELECT id, user_id, status FROM telegram.links
			 WHERE telegram_user_id = $1 AND status IN `+liveStatuses+`
			 FOR UPDATE;`, l.TelegramUserID).Scan(&existingID, &existingUser, &existingStatus)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
		case err != nil:
			return err
		case existingStatus != string(telegram.LinkPending) && existingUser != l.UserID:
			return telegram.ErrLinkedElsewhere
		case existingStatus != string(telegram.LinkPending):
			l.ID = existingID
			return nil
		default:
			if _, err := tx.Exec(ctx, `
				UPDATE telegram.links SET status = 'revoked', revoked_at = now(), updated_at = now()
				 WHERE id = $1;`, existingID); err != nil {
				return err
			}
		}

		// One pending confirmation per user: the newest code opened wins.
		if _, err := tx.Exec(ctx, `
			UPDATE telegram.links SET status = 'revoked', revoked_at = now(), updated_at = now()
			 WHERE user_id = $1 AND status = 'pending';`, l.UserID); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `
			INSERT INTO telegram.links
			       (user_id, telegram_user_id, chat_id, username, display_name, status,
			        active_organization_id, confirm_expires_at)
			VALUES ($1, $2, $3, $4, $5, 'pending', $6, $7)
			RETURNING id, public_id::text;`,
			l.UserID, l.TelegramUserID, l.ChatID, l.Username, l.DisplayName, l.ActiveOrgID, confirmBy,
		).Scan(&l.ID, &l.PublicID)
	})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return telegram.ErrLinkedElsewhere
	}
	return err
}

// ConfirmPendingLink activates the user's own unexpired pending link and
// retires any Telegram account they had confirmed before.
func (r *Repository) ConfirmPendingLink(ctx context.Context, userID int64, publicID string) (*telegram.Link, error) {
	var link *telegram.Link
	err := r.db.InTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var id int64
		err := tx.QueryRow(ctx, `
			SELECT id FROM telegram.links
			 WHERE user_id = $1 AND public_id::text = $2
			   AND status = 'pending' AND confirm_expires_at > now()
			 FOR UPDATE;`, userID, publicID).Scan(&id)
		if errors.Is(err, pgx.ErrNoRows) {
			return telegram.ErrNoPendingLink
		}
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			UPDATE telegram.links SET status = 'revoked', revoked_at = now(), busy_until = NULL, updated_at = now()
			 WHERE user_id = $1 AND status IN ('active', 'blocked') AND id <> $2;`, userID, id); err != nil {
			return err
		}
		link, err = scanLink(tx.QueryRow(ctx, `
			UPDATE telegram.links
			   SET status = 'active', confirmed_at = now(), confirm_expires_at = NULL, updated_at = now()
			 WHERE id = $1
			RETURNING `+linkColumns+`;`, id))
		return err
	})
	return link, err
}

// RevokeLink ends one of the user's links and drops anything still queued.
func (r *Repository) RevokeLink(ctx context.Context, userID, linkID int64) error {
	return r.db.InTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			UPDATE telegram.links
			   SET status = 'revoked', revoked_at = now(), busy_until = NULL, updated_at = now()
			 WHERE id = $1 AND user_id = $2 AND status <> 'revoked';`, linkID, userID)
		if err != nil || tag.RowsAffected() == 0 {
			return err
		}
		_, err = tx.Exec(ctx, `
			UPDATE telegram.deliveries SET status = 'dropped', drop_reason = 'link_revoked', lease_until = NULL
			 WHERE link_id = $1 AND status IN ('queued', 'leased');`, linkID)
		return err
	})
}

// SetLinkStatus moves a confirmed link between active and blocked. It cannot
// confirm a pending link or revive a revoked one.
func (r *Repository) SetLinkStatus(ctx context.Context, linkID int64, status telegram.LinkStatus) error {
	if status != telegram.LinkActive && status != telegram.LinkBlocked {
		return errors.New("telegram: SetLinkStatus only toggles active and blocked")
	}
	return r.exec(ctx, `
		UPDATE telegram.links SET status = $2, updated_at = now()
		 WHERE id = $1 AND status IN ('active', 'blocked');`, linkID, string(status))
}

// TouchLink refreshes what Telegram says about the account.
func (r *Repository) TouchLink(ctx context.Context, linkID, chatID int64, username, displayName string) error {
	return r.exec(ctx, `
		UPDATE telegram.links
		   SET chat_id = $2, username = $3, display_name = $4, last_seen_at = now(), updated_at = now()
		 WHERE id = $1;`, linkID, chatID, username, displayName)
}

// SetActiveOrganization changes the chat's منشأة and starts a new conversation,
// because a conversation belongs to one organisation.
func (r *Repository) SetActiveOrganization(ctx context.Context, linkID int64, orgID *int64) error {
	return r.exec(ctx, `
		UPDATE telegram.links SET active_organization_id = $2, conversation_id = NULL, updated_at = now()
		 WHERE id = $1;`, linkID, orgID)
}

// SetConversation records the assistant conversation the chat continues.
func (r *Repository) SetConversation(ctx context.Context, linkID int64, conversationID *int64) error {
	return r.exec(ctx, `UPDATE telegram.links SET conversation_id = $2, updated_at = now() WHERE id = $1;`,
		linkID, conversationID)
}

// SetMutedCategories replaces the muted notification categories.
func (r *Repository) SetMutedCategories(ctx context.Context, linkID int64, muted []string) error {
	if muted == nil {
		muted = []string{}
	}
	return r.exec(ctx, `UPDATE telegram.links SET muted_categories = $2, updated_at = now() WHERE id = $1;`,
		linkID, muted)
}

// AcquireBusy takes the chat's one-question lock. The conditional UPDATE is
// atomic across server replicas.
func (r *Repository) AcquireBusy(ctx context.Context, linkID int64, until time.Time) (bool, error) {
	var acquired bool
	err := r.db.InTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			UPDATE telegram.links SET busy_until = $2
			 WHERE id = $1 AND (busy_until IS NULL OR busy_until < now());`, linkID, until)
		acquired = err == nil && tag.RowsAffected() == 1
		return err
	})
	return acquired, err
}

// ReleaseBusy releases the lock.
func (r *Repository) ReleaseBusy(ctx context.Context, linkID int64) error {
	return r.exec(ctx, `UPDATE telegram.links SET busy_until = NULL WHERE id = $1;`, linkID)
}

// MarkUpdateProcessed reports whether this is the first time an update id has
// been seen.
func (r *Repository) MarkUpdateProcessed(ctx context.Context, updateID int64) (bool, error) {
	var first bool
	err := r.db.InTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(ctx,
			`INSERT INTO telegram.processed_updates (update_id) VALUES ($1) ON CONFLICT DO NOTHING;`, updateID)
		first = err == nil && tag.RowsAffected() == 1
		return err
	})
	return first, err
}

// PurgeProcessedUpdates forgets update ids older than a time.
func (r *Repository) PurgeProcessedUpdates(ctx context.Context, olderThan time.Time) error {
	return r.exec(ctx, `DELETE FROM telegram.processed_updates WHERE received_at < $1;`, olderThan)
}

func (r *Repository) exec(ctx context.Context, sql string, args ...any) error {
	return r.db.InTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, sql, args...)
		return err
	})
}
