package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"

	platformadmin "github.com/muhiya/dawa24-store/internal/modules/platform_admin"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// ListContentBlocks returns all CMS blocks ordered by sort order.
func (r *Repository) ListContentBlocks(ctx context.Context) ([]*platformadmin.ContentBlock, error) {
	var list []*platformadmin.ContentBlock
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const query = `
			SELECT id, key, title, body, position, sort_order, is_active, updated_at
			FROM platform_admin.content_blocks
			ORDER BY sort_order ASC, id ASC;
		`
		rows, err := tx.Query(txCtx, query)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var b platformadmin.ContentBlock
			if err := rows.Scan(&b.ID, &b.Key, &b.Title, &b.Body, &b.Position, &b.SortOrder, &b.IsActive, &b.UpdatedAt); err != nil {
				return err
			}
			list = append(list, &b)
		}
		return rows.Err()
	})
	return list, err
}

// GetContentBlockByKey returns one active CMS block by key.
func (r *Repository) GetContentBlockByKey(ctx context.Context, key string) (*platformadmin.ContentBlock, error) {
	var b platformadmin.ContentBlock
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const query = `
			SELECT id, key, title, body, position, sort_order, is_active, updated_at
			FROM platform_admin.content_blocks
			WHERE key = $1 AND is_active = true
			ORDER BY id ASC LIMIT 1;
		`
		err := tx.QueryRow(txCtx, query, key).Scan(&b.ID, &b.Key, &b.Title, &b.Body, &b.Position, &b.SortOrder, &b.IsActive, &b.UpdatedAt)
		if err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("content_block")
			}
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &b, nil
}

// UpsertContentBlock creates or updates a CMS block by key.
func (r *Repository) UpsertContentBlock(ctx context.Context, b *platformadmin.ContentBlock) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const query = `
			INSERT INTO platform_admin.content_blocks (key, title, body, position, sort_order, is_active, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, now())
			ON CONFLICT (key) DO UPDATE SET
				title = EXCLUDED.title, body = EXCLUDED.body, position = EXCLUDED.position,
				sort_order = EXCLUDED.sort_order, is_active = EXCLUDED.is_active, updated_at = now()
			RETURNING id, updated_at;
		`
		return tx.QueryRow(txCtx, query, b.Key, b.Title, b.Body, b.Position, b.SortOrder, b.IsActive).Scan(&b.ID, &b.UpdatedAt)
	})
}

// ListTranslations returns all UI translations.
func (r *Repository) ListTranslations(ctx context.Context) ([]*platformadmin.Translation, error) {
	var list []*platformadmin.Translation
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const query = `SELECT id, key, translation_group, text, updated_at FROM platform_admin.translations ORDER BY key ASC;`
		rows, err := tx.Query(txCtx, query)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var t platformadmin.Translation
			if err := rows.Scan(&t.ID, &t.Key, &t.Group, &t.Text, &t.UpdatedAt); err != nil {
				return err
			}
			list = append(list, &t)
		}
		return rows.Err()
	})
	return list, err
}

// UpsertTranslation creates or updates a translation by key.
func (r *Repository) UpsertTranslation(ctx context.Context, t *platformadmin.Translation) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const query = `INSERT INTO platform_admin.translations (key, translation_group, text, updated_at) VALUES ($1, $2, $3, now()) ON CONFLICT (key) DO UPDATE SET translation_group = EXCLUDED.translation_group, text = EXCLUDED.text, updated_at = now() RETURNING id, updated_at;`
		return tx.QueryRow(txCtx, query, t.Key, t.Group, t.Text).Scan(&t.ID, &t.UpdatedAt)
	})
}

// ListAuditLog returns the platform audit trail, newest first.
func (r *Repository) ListAuditLog(ctx context.Context, limit, offset int) ([]*platformadmin.AuditEntry, error) {
	var list []*platformadmin.AuditEntry
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const query = `
			SELECT a.id, a.organization_id,
			       COALESCE(NULLIF(o.trade_name->>'ar', ''), NULLIF(o.legal_name, ''), 'المنصة الرئيسية') AS org_name,
			       a.actor_user_id,
			       COALESCE(NULLIF(u.name->>'ar', ''), NULLIF(u.name->>'en', ''), u.email, 'النظام / System') AS actor_name,
			       COALESCE(u.email, '') AS actor_email,
			       a.action, a.entity_type, a.entity_id,
			       COALESCE(HOST(a.ip), '') AS ip_addr,
			       COALESCE(a.request_id, '') AS req_id,
			       a.before, a.after, a.created_at
			FROM platform.audit_log a
			LEFT JOIN identity.users u ON a.actor_user_id = u.id
			LEFT JOIN org.organizations o ON a.organization_id = o.id
			ORDER BY a.created_at DESC
			LIMIT $1 OFFSET $2;
		`
		if limit <= 0 || limit > 100 {
			limit = 50
		}
		rows, err := tx.Query(txCtx, query, limit, offset)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var e platformadmin.AuditEntry
			var ipAddr, reqID string
			if err := rows.Scan(
				&e.ID, &e.OrganizationID, &e.OrganizationName, &e.ActorUserID,
				&e.ActorName, &e.ActorEmail, &e.Action, &e.EntityType, &e.EntityID,
				&ipAddr, &reqID, &e.Before, &e.After, &e.CreatedAt,
			); err != nil {
				return err
			}
			e.IPAddress = ipAddr
			e.Route = reqID
			list = append(list, &e)
		}
		return rows.Err()
	})
	return list, err
}

// QueueStats returns River job counts grouped by state.
func (r *Repository) QueueStats(ctx context.Context) (map[string]int, error) {
	stats := map[string]int{}
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const query = `SELECT state, COUNT(*) FROM river_job GROUP BY state;`
		rows, err := tx.Query(txCtx, query)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var state string
			var n int
			if err := rows.Scan(&state, &n); err != nil {
				return err
			}
			stats[state] = n
		}
		return rows.Err()
	})
	return stats, err
}
