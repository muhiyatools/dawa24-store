package postgres

import (
	"context"
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5"

	platformadmin "github.com/muhiya/dawa24-store/internal/modules/platform_admin"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// Trash operates across every tenant's rows by definition — it is the platform
// admin's recovery tool — so all of it runs AsSystem. The route is gated by
// RequirePagePermission, which is what keeps it away from non-staff.

// ListSoftDeletableTables discovers every table carrying a deleted_at column
// and counts its rows. Discovery beats a hardcoded list: a new soft-deletable
// table appears here without anyone remembering to register it.
func (r *Repository) ListSoftDeletableTables(ctx context.Context) ([]*platformadmin.TrashModel, error) {
	var models []*platformadmin.TrashModel
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const discover = `
			SELECT c.table_schema, c.table_name
			FROM information_schema.columns c
			JOIN information_schema.tables t
			  ON t.table_schema = c.table_schema AND t.table_name = c.table_name
			WHERE c.column_name = 'deleted_at'
			  AND t.table_type = 'BASE TABLE'
			  AND c.table_schema NOT IN ('pg_catalog', 'information_schema')
			ORDER BY c.table_schema, c.table_name;
		`
		rows, err := tx.Query(txCtx, discover)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var m platformadmin.TrashModel
			if err := rows.Scan(&m.Schema, &m.Table); err != nil {
				return err
			}
			m.Key = m.Schema + "." + m.Table
			models = append(models, &m)
		}
		if err := rows.Err(); err != nil {
			return err
		}

		// Counts are real queries. The screen used to display invented numbers
		// (1240 products, 14200 orders) that no query produced.
		for _, m := range models {
			// Identifiers come from information_schema above, never from user
			// input, so quoting them is safe here.
			q := fmt.Sprintf(
				`SELECT COUNT(*), COUNT(*) FILTER (WHERE deleted_at IS NOT NULL) FROM %q.%q`,
				m.Schema, m.Table)
			if err := tx.QueryRow(txCtx, q).Scan(&m.TotalCount, &m.TrashedRows); err != nil {
				return fmt.Errorf("count %s: %w", m.Key, err)
			}
		}
		return nil
	})
	return models, err
}

// ListTrashedRows returns the soft-deleted rows of one table with a readable
// label taken from whichever of the usual name columns the table has.
func (r *Repository) ListTrashedRows(ctx context.Context, schema, table string, limit, offset int) ([]*platformadmin.TrashRow, error) {
	var out []*platformadmin.TrashRow
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		labelExpr, err := trashLabelExpr(txCtx, tx, schema, table)
		if err != nil {
			return err
		}
		q := fmt.Sprintf(`
			SELECT id, %s, to_char(deleted_at, 'YYYY-MM-DD HH24:MI')
			FROM %q.%q
			WHERE deleted_at IS NOT NULL
			ORDER BY deleted_at DESC
			LIMIT $1 OFFSET $2`, labelExpr, schema, table)
		rows, err := tx.Query(txCtx, q, limit, offset)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var row platformadmin.TrashRow
			if err := rows.Scan(&row.ID, &row.Label, &row.DeletedAt); err != nil {
				return err
			}
			out = append(out, &row)
		}
		return rows.Err()
	})
	return out, err
}

// RestoreTrashedRow clears deleted_at, refusing when the row's parent
// organization is itself still deleted — restoring a child under a deleted
// parent produces a row nothing can reach.
func (r *Repository) RestoreTrashedRow(ctx context.Context, schema, table string, id, actorID int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		if err := assertSoftDeletable(txCtx, tx, schema, table); err != nil {
			return err
		}

		hasOrg, err := columnExists(txCtx, tx, schema, table, "organization_id")
		if err != nil {
			return err
		}
		if hasOrg {
			var orphaned bool
			q := fmt.Sprintf(`
				SELECT EXISTS (
					SELECT 1 FROM %q.%q t
					JOIN org.organizations o ON o.id = t.organization_id
					WHERE t.id = $1 AND o.deleted_at IS NOT NULL
				)`, schema, table)
			if err := tx.QueryRow(txCtx, q, id).Scan(&orphaned); err != nil {
				return err
			}
			if orphaned {
				return apperr.Conflict("trash.parent_deleted",
					"Restore the owning organization first; this record's organization is still deleted.")
			}
		}

		q := fmt.Sprintf(`UPDATE %q.%q SET deleted_at = NULL WHERE id = $1 AND deleted_at IS NOT NULL`, schema, table)
		tag, err := tx.Exec(txCtx, q, id)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("trashed_row")
		}
		return writeTrashAudit(txCtx, tx, "restore", schema, table, id, actorID, "")
	})
}

// PurgeTrashedRow hard-deletes a row. Irreversible, so the row is serialised
// into the audit log before it goes.
func (r *Repository) PurgeTrashedRow(ctx context.Context, schema, table string, id, actorID int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		if err := assertSoftDeletable(txCtx, tx, schema, table); err != nil {
			return err
		}

		var snapshot string
		snapQ := fmt.Sprintf(`SELECT to_jsonb(t)::text FROM %q.%q t WHERE t.id = $1 AND t.deleted_at IS NOT NULL`, schema, table)
		if err := tx.QueryRow(txCtx, snapQ, id).Scan(&snapshot); err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("trashed_row")
			}
			return err
		}

		q := fmt.Sprintf(`DELETE FROM %q.%q WHERE id = $1 AND deleted_at IS NOT NULL`, schema, table)
		tag, err := tx.Exec(txCtx, q, id)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("trashed_row")
		}
		return writeTrashAudit(txCtx, tx, "purge", schema, table, id, actorID, snapshot)
	})
}

// assertSoftDeletable re-validates the identifier against information_schema.
// The schema and table arrive from a URL segment, so they are never trusted on
// shape alone — if the pair is not a real soft-deletable table, nothing runs.
func assertSoftDeletable(ctx context.Context, tx pgx.Tx, schema, table string) error {
	ok, err := columnExists(ctx, tx, schema, table, "deleted_at")
	if err != nil {
		return err
	}
	if !ok {
		return apperr.NotFound("trash_model")
	}
	return nil
}

func columnExists(ctx context.Context, tx pgx.Tx, schema, table, column string) (bool, error) {
	var exists bool
	const q = `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_schema = $1 AND table_name = $2 AND column_name = $3
		)`
	err := tx.QueryRow(ctx, q, schema, table, column).Scan(&exists)
	return exists, err
}

// trashLabelExpr picks the first present display column so one generic query
// can label rows from tables with very different shapes.
func trashLabelExpr(ctx context.Context, tx pgx.Tx, schema, table string) (string, error) {
	for _, col := range []string{"trade_name", "name", "title", "order_number", "email", "public_id"} {
		ok, err := columnExists(ctx, tx, schema, table, col)
		if err != nil {
			return "", err
		}
		if ok {
			// JSONB bilingual columns need the Arabic key pulled out.
			return fmt.Sprintf(
				`COALESCE(CASE WHEN jsonb_typeof(to_jsonb(%q)) = 'object' THEN to_jsonb(%q)->>'ar' ELSE %q::text END, '')`,
				col, col, col), nil
		}
	}
	return `''`, nil
}

// writeTrashAudit records the action in the append-only audit trail. For a
// purge, `before` carries the row that was destroyed — that snapshot is the
// only remaining trace of it.
func writeTrashAudit(ctx context.Context, tx pgx.Tx, action, schema, table string, id, actorID int64, snapshot string) error {
	const q = `
		INSERT INTO platform.audit_log (actor_user_id, action, entity_type, entity_id, before, created_at)
		VALUES ($1, $2, $3, $4, $5::jsonb, now())`
	var before *string
	if snapshot != "" {
		before = &snapshot
	}
	_, err := tx.Exec(ctx, q, actorID, "trash."+action, schema+"."+table, strconv.FormatInt(id, 10), before)
	return err
}
