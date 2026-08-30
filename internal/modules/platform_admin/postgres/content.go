package postgres

import (
	"context"
	"fmt"
	"strings"

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

// ToggleContentBlockStatus toggles is_active for a content block.
func (r *Repository) ToggleContentBlockStatus(ctx context.Context, id int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const query = `
			UPDATE platform_admin.content_blocks
			SET is_active = NOT is_active, updated_at = now()
			WHERE id = $1;
		`
		_, err := tx.Exec(txCtx, query, id)
		return err
	})
}

// DeleteContentBlock removes a content block by ID.
func (r *Repository) DeleteContentBlock(ctx context.Context, id int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const query = `DELETE FROM platform_admin.content_blocks WHERE id = $1;`
		_, err := tx.Exec(txCtx, query, id)
		return err
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

// ListAuditLogByOrg returns audit trail entries filtered to a specific organization.
func (r *Repository) ListAuditLogByOrg(ctx context.Context, orgID int64, limit, offset int) ([]*platformadmin.AuditEntry, error) {
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
			WHERE a.organization_id = $1
			ORDER BY a.created_at DESC
			LIMIT $2 OFFSET $3;
		`
		if limit <= 0 || limit > 100 {
			limit = 50
		}
		rows, err := tx.Query(txCtx, query, orgID, limit, offset)
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

// ListAuditLogWithFilter returns filtered audit log entries and total count matching the criteria.
func (r *Repository) ListAuditLogWithFilter(ctx context.Context, filter platformadmin.AuditLogFilter) ([]*platformadmin.AuditEntry, int, error) {
	if filter.Limit <= 0 || filter.Limit > 100 {
		filter.Limit = 50
	}
	if filter.Offset < 0 {
		filter.Offset = 0
	}

	var list []*platformadmin.AuditEntry
	var total int

	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		var conditions []string
		var args []any
		argIdx := 1

		if filter.OrganizationID != nil && *filter.OrganizationID > 0 {
			conditions = append(conditions, fmt.Sprintf("a.organization_id = $%d", argIdx))
			args = append(args, *filter.OrganizationID)
			argIdx++
		}

		if filter.ActorUserID != nil && *filter.ActorUserID > 0 {
			conditions = append(conditions, fmt.Sprintf("a.actor_user_id = $%d", argIdx))
			args = append(args, *filter.ActorUserID)
			argIdx++
		}

		if strings.TrimSpace(filter.Action) != "" {
			conditions = append(conditions, fmt.Sprintf("(a.action ILIKE '%%' || $%d || '%%' OR a.entity_type ILIKE '%%' || $%d || '%%')", argIdx, argIdx))
			args = append(args, strings.TrimSpace(filter.Action))
			argIdx++
		}

		if strings.TrimSpace(filter.EntityType) != "" {
			conditions = append(conditions, fmt.Sprintf("a.entity_type = $%d", argIdx))
			args = append(args, strings.TrimSpace(filter.EntityType))
			argIdx++
		}

		if strings.TrimSpace(filter.Search) != "" {
			s := strings.TrimSpace(filter.Search)
			conditions = append(conditions, fmt.Sprintf("(u.name->>'ar' ILIKE '%%' || $%d || '%%' OR u.name->>'en' ILIKE '%%' || $%d || '%%' OR u.email ILIKE '%%' || $%d || '%%' OR o.trade_name->>'ar' ILIKE '%%' || $%d || '%%' OR a.action ILIKE '%%' || $%d || '%%' OR a.entity_id ILIKE '%%' || $%d || '%%')", argIdx, argIdx, argIdx, argIdx, argIdx, argIdx))
			args = append(args, s)
			argIdx++
		}

		whereClause := ""
		if len(conditions) > 0 {
			whereClause = "WHERE " + strings.Join(conditions, " AND ")
		}

		countQuery := fmt.Sprintf(`
			SELECT COUNT(*)
			FROM platform.audit_log a
			LEFT JOIN identity.users u ON a.actor_user_id = u.id
			LEFT JOIN org.organizations o ON a.organization_id = o.id
			%s;
		`, whereClause)

		if err := tx.QueryRow(txCtx, countQuery, args...).Scan(&total); err != nil {
			return err
		}

		query := fmt.Sprintf(`
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
			%s
			ORDER BY a.created_at DESC
			LIMIT $%d OFFSET $%d;
		`, whereClause, argIdx, argIdx+1)

		args = append(args, filter.Limit, filter.Offset)

		rows, err := tx.Query(txCtx, query, args...)
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

	return list, total, err
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
