package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/telegram"
)

// maxLeaseAttempts fails a delivery whose lease has expired this many times —
// n8n claimed it and never reported back, repeatedly.
const maxLeaseAttempts = 5

// staleNotificationSQL is how long a queued notification stays worth sending.
// Kept in step with telegram.notificationWindow.
const staleNotificationSQL = `interval '2 hours'`

// orgNameSQL is the organisation's display name, Arabic first.
const orgNameSQL = `COALESCE(NULLIF(o.trade_name->>'ar', ''), NULLIF(o.trade_name->>'en', ''), o.legal_name, '')`

// Memberships lists the organisations a user is an active member of, with the
// same predicate the RBAC resolver uses to grant anything for one.
func (r *Repository) Memberships(ctx context.Context, userID int64) ([]telegram.Membership, error) {
	var out []telegram.Membership
	err := r.db.InReadTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT o.id, `+orgNameSQL+`, o.type, o.status,
			       COALESCE(NULLIF(b.name->>'ar', ''), NULLIF(b.name->>'en', ''), '')
			  FROM org.members m
			  JOIN org.organizations o ON o.id = m.organization_id
			  LEFT JOIN org.branches b ON b.id = m.branch_id AND b.deleted_at IS NULL
			 WHERE m.user_id = $1
			   AND m.status = 'active' AND m.is_active = true
			   AND o.deleted_at IS NULL
			 ORDER BY o.id;`, userID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var m telegram.Membership
			if err := rows.Scan(&m.OrgID, &m.Name, &m.Type, &m.Status, &m.BranchName); err != nil {
				return err
			}
			out = append(out, m)
		}
		return rows.Err()
	})
	return out, err
}

// OffersTopicEnabled reads the account-wide offers preference. No preferences
// row means the defaults, under which offers are on.
func (r *Repository) OffersTopicEnabled(ctx context.Context, userID int64) (bool, error) {
	enabled := true
	err := r.db.InReadTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `
			SELECT NOT COALESCE(notification_topics->'offers' = 'false'::jsonb, false)
			  FROM profile.user_preferences WHERE user_id = $1;`, userID).Scan(&enabled)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return err
	})
	return enabled, err
}

// NotificationCandidates returns in-app notifications for active links that
// have no recorded decision yet, created after the link was confirmed and not
// before since.
//
// There is deliberately no cursor. A notification row can commit after one
// with a higher id; a cursor would skip it for ever. NOT EXISTS over the
// decision table picks it up on the next claim, and the time bound keeps the
// scan to a recent window per linked user.
func (r *Repository) NotificationCandidates(ctx context.Context, since time.Time, limit int) ([]telegram.Candidate, error) {
	var out []telegram.Candidate
	err := r.db.InReadTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT l.id, k.id, COALESCE(k.active_organization_id, 0), k.muted_categories,
			       l.user_id, COALESCE(l.organization_id, 0), `+orgNameSQL+`,
			       l.title, l.body, l.required_permission
			  FROM telegram.links k
			  JOIN notifications.logs l
			    ON l.user_id = k.user_id
			   AND l.channel = 'in_app'
			   AND l.created_at >= GREATEST(k.confirmed_at, $1)
			  LEFT JOIN org.organizations o ON o.id = l.organization_id
			 WHERE k.status = 'active'
			   AND NOT EXISTS (
			       SELECT 1 FROM telegram.deliveries d
			        WHERE d.link_id = k.id AND d.notification_log_id = l.id)
			 ORDER BY l.id
			 LIMIT $2;`, since, limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var c telegram.Candidate
			if err := rows.Scan(&c.LogID, &c.LinkID, &c.LinkActiveOrgID, &c.MutedCategories,
				&c.UserID, &c.OrganizationID, &c.OrganizationName,
				&c.Title, &c.Body, &c.RequiredPermission); err != nil {
				return err
			}
			out = append(out, c)
		}
		return rows.Err()
	})
	return out, err
}

// RecordDecisions writes one delivery row per decision. Two claims evaluating
// the same notification at once both try; the unique index keeps one.
func (r *Repository) RecordDecisions(ctx context.Context, decisions []telegram.Decision) error {
	if len(decisions) == 0 {
		return nil
	}
	return r.db.InTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		batch := &pgx.Batch{}
		for _, d := range decisions {
			status := "queued"
			if d.DropReason != "" {
				status = "dropped"
			}
			batch.Queue(`
				INSERT INTO telegram.deliveries
				       (link_id, notification_log_id, kind, category, text, status, drop_reason)
				VALUES ($1, $2, 'notification', $3, $4, $5, $6)
				ON CONFLICT (link_id, notification_log_id) WHERE notification_log_id IS NOT NULL DO NOTHING;`,
				d.LinkID, d.LogID, string(d.Category), d.Text, status, d.DropReason)
		}
		return tx.SendBatch(ctx, batch).Close()
	})
}

// EnqueueSystemMessage queues a message the bridge itself writes.
func (r *Repository) EnqueueSystemMessage(ctx context.Context, linkID int64, text string) error {
	return r.exec(ctx, `
		INSERT INTO telegram.deliveries (link_id, kind, category, text)
		VALUES ($1, 'system', 'general', $2);`, linkID, text)
}

// ClaimDeliveries leases due messages for active links.
//
// FOR UPDATE SKIP LOCKED lets two overlapping n8n runs claim disjoint rows. A
// lease that expires without a report is claimable again, up to
// maxLeaseAttempts, so a crashed run neither loses a message nor retries it
// for ever.
func (r *Repository) ClaimDeliveries(ctx context.Context, limit int, lease time.Duration) ([]telegram.Outgoing, error) {
	var out []telegram.Outgoing
	err := r.db.InTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			UPDATE telegram.deliveries
			   SET status = 'failed', last_error = 'lease expired without a report', lease_until = NULL
			 WHERE status = 'leased' AND lease_until < now() AND attempts >= $1;`, maxLeaseAttempts); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			UPDATE telegram.deliveries SET status = 'dropped', drop_reason = 'expired', lease_until = NULL
			 WHERE status IN ('queued', 'leased') AND kind = 'notification'
			   AND created_at < now() - `+staleNotificationSQL+`
			   AND (status = 'queued' OR lease_until < now());`); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `
			WITH due AS (
				SELECT d.id
				  FROM telegram.deliveries d
				  JOIN telegram.links k ON k.id = d.link_id AND k.status = 'active'
				 WHERE (d.status = 'queued' AND d.next_attempt_at <= now())
				    OR (d.status = 'leased' AND d.lease_until < now())
				 ORDER BY d.id
				 LIMIT $1
				   FOR UPDATE OF d SKIP LOCKED
			)
			UPDATE telegram.deliveries d
			   SET status = 'leased',
			       lease_until = now() + make_interval(secs => $2),
			       attempts = d.attempts + 1
			  FROM due, telegram.links k
			 WHERE d.id = due.id AND k.id = d.link_id
			RETURNING d.id, k.chat_id, d.text;`, limit, lease.Seconds())
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var o telegram.Outgoing
			if err := rows.Scan(&o.ID, &o.ChatID, &o.Text); err != nil {
				return err
			}
			out = append(out, o)
		}
		return rows.Err()
	})
	return out, err
}

// MarkDelivered records a successful send of a leased message.
func (r *Repository) MarkDelivered(ctx context.Context, id int64) error {
	return r.exec(ctx, `
		UPDATE telegram.deliveries SET status = 'sent', sent_at = now(), lease_until = NULL, last_error = ''
		 WHERE id = $1 AND status = 'leased';`, id)
}

// RetryDelivery requeues a leased message, or fails it at maxAttempts.
func (r *Repository) RetryDelivery(ctx context.Context, id int64, errText string, retryAfter time.Duration, maxAttempts int) error {
	return r.exec(ctx, `
		UPDATE telegram.deliveries
		   SET status = CASE WHEN attempts >= $4 THEN 'failed' ELSE 'queued' END,
		       last_error = $2,
		       lease_until = NULL,
		       next_attempt_at = CASE
		           WHEN $3::float8 > 0 THEN now() + make_interval(secs => $3::float8)
		           ELSE now() + make_interval(mins => LEAST(power(2, attempts), 60)::int)
		       END
		 WHERE id = $1 AND status = 'leased';`, id, errText, retryAfter.Seconds(), maxAttempts)
}

// FailDeliveryAndBlockLink records a permanent refusal — the user blocked the
// bot or no longer exists — and stops sending to that chat.
func (r *Repository) FailDeliveryAndBlockLink(ctx context.Context, id int64, errText string) error {
	return r.db.InTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var linkID int64
		err := tx.QueryRow(ctx, `
			UPDATE telegram.deliveries SET status = 'failed', last_error = $2, lease_until = NULL
			 WHERE id = $1 AND status = 'leased'
			RETURNING link_id;`, id, errText).Scan(&linkID)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `
			UPDATE telegram.links SET status = 'blocked', updated_at = now()
			 WHERE id = $1 AND status = 'active';`, linkID)
		return err
	})
}
