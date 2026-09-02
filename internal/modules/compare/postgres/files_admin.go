package postgres

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/compare"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// uploaderLabelExpr builds a human label for a compare.files.user_id, falling
// back through first/last name, the localized name JSON, then the email.
const uploaderLabelExpr = `COALESCE(NULLIF(TRIM(CONCAT_WS(' ', u.first_name, u.last_name)), ''), ` +
	`u.name->>'ar', u.name->>'en', u.email::text, '#' || f.user_id)`

// adminTempWarehouseScope is the base predicate shared by the Super Admin and
// "my uploads" listings: a temporary warehouse is a moderator upload
// (is_temp_warehouse) or any vendor compare-tool upload (organization_id set).
const adminTempWarehouseScope = `f.deleted_at IS NULL AND (f.is_temp_warehouse = TRUE OR f.organization_id IS NOT NULL)`

// fileColumnsF is fileColumns qualified with the "f" alias, for queries that
// JOIN identity.users / org.organizations where bare column names are ambiguous.
const fileColumnsF = `f.id, f.public_id, f.organization_id, f.user_id, f.supplier_name, f.original_filename, ` +
	`COALESCE(f.storage_key, ''), COALESCE(f.mime_type, ''), f.size_bytes, f.row_count, f.status, ` +
	`COALESCE(f.mapping_config, '{}'::jsonb), f.archived_at, COALESCE(f.archive_reason, ''), ` +
	`COALESCE(f.error_message, ''), f.is_temp_warehouse, COALESCE(f.visibility, 'private'), ` +
	`f.created_at, f.updated_at, f.deleted_at`

// SetFileVisibility flips a compare file between 'private' and 'public'. It runs
// in the caller's tenant context so a vendor updating their own organization's
// file passes the tenant_isolation_compare_files policy.
func (r *Repository) SetFileVisibility(ctx context.Context, id int64, visibility string) error {
	if visibility != compare.VisibilityPrivate && visibility != compare.VisibilityPublic {
		return apperr.Validation("invalid_visibility", "invalid visibility value", nil)
	}
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		res, err := tx.Exec(txCtx,
			`UPDATE compare.files SET visibility = $2, updated_at = now() WHERE id = $1 AND deleted_at IS NULL;`,
			id, visibility)
		if err != nil {
			return err
		}
		if res.RowsAffected() == 0 {
			return apperr.NotFound("compare file")
		}
		return nil
	})
}

func buildAdminTempWarehouseWhere(filter compare.AdminTempWarehouseFilter) ([]string, []any) {
	where := []string{adminTempWarehouseScope}
	var args []any
	i := 1

	if filter.Status != nil {
		where = append(where, fmt.Sprintf("f.status = $%d", i))
		args = append(args, string(*filter.Status))
		i++
	}
	if s := strings.TrimSpace(filter.Search); s != "" {
		where = append(where, fmt.Sprintf("(f.supplier_name ILIKE $%d OR f.original_filename ILIKE $%d)", i, i))
		args = append(args, "%"+s+"%")
		i++
	}
	if filter.OwnerOnly != nil {
		where = append(where, fmt.Sprintf("f.user_id = $%d", i))
		args = append(args, *filter.OwnerOnly)
		i++
	} else if filter.UploaderID != nil {
		where = append(where, fmt.Sprintf("f.user_id = $%d", i))
		args = append(args, *filter.UploaderID)
		i++
	}
	switch filter.Source {
	case "moderator":
		where = append(where, "f.is_temp_warehouse = TRUE AND f.organization_id IS NULL")
	case "vendor":
		where = append(where, "f.organization_id IS NOT NULL")
	}
	return where, args
}

// ListAdminTempWarehouses returns temporary warehouses (moderator uploads plus
// vendor compare-tool files) enriched with the uploader and vendor labels.
func (r *Repository) ListAdminTempWarehouses(ctx context.Context, filter compare.AdminTempWarehouseFilter) ([]*compare.AdminTempWarehouse, error) {
	where, args := buildAdminTempWarehouseWhere(filter)
	sql := `
		SELECT ` + fileColumnsF + `,
		       ` + uploaderLabelExpr + ` AS uploader_name,
		       COALESCE(o.name->>'ar', o.name->>'en', '') AS org_name
		FROM compare.files f
		LEFT JOIN identity.users u ON u.id = f.user_id
		LEFT JOIN org.organizations o ON o.id = f.organization_id
		WHERE ` + strings.Join(where, " AND ") + `
		ORDER BY f.created_at DESC;`

	var out []*compare.AdminTempWarehouse
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(txCtx, sql, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			item, err := scanAdminTempWarehouse(rows)
			if err != nil {
				return err
			}
			out = append(out, item)
		}
		return rows.Err()
	})
	return out, err
}

// ListAdminTempWarehousesWithTotal returns paginated temporary warehouses and total count.
func (r *Repository) ListAdminTempWarehousesWithTotal(ctx context.Context, filter compare.AdminTempWarehouseFilter, limit, offset int) ([]*compare.AdminTempWarehouse, int, error) {
	where, args := buildAdminTempWarehouseWhere(filter)
	whereClause := strings.Join(where, " AND ")

	if limit <= 0 || limit > 100 {
		limit = 25
	}
	if offset < 0 {
		offset = 0
	}

	countSQL := fmt.Sprintf("SELECT count(*) FROM compare.files f WHERE %s;", whereClause)
	dataSQL := fmt.Sprintf(`
		SELECT %s,
		       %s AS uploader_name,
		       COALESCE(o.name->>'ar', o.name->>'en', '') AS org_name
		FROM compare.files f
		LEFT JOIN identity.users u ON u.id = f.user_id
		LEFT JOIN org.organizations o ON o.id = f.organization_id
		WHERE %s
		ORDER BY f.created_at DESC, f.id DESC
		LIMIT $%d OFFSET $%d;`,
		fileColumnsF, uploaderLabelExpr, whereClause, len(args)+1, len(args)+2)

	var total int
	var out []*compare.AdminTempWarehouse

	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		if err := tx.QueryRow(txCtx, countSQL, args...).Scan(&total); err != nil {
			return err
		}

		dataArgs := append(append([]any{}, args...), limit, offset)
		rows, err := tx.Query(txCtx, dataSQL, dataArgs...)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			item, err := scanAdminTempWarehouse(rows)
			if err != nil {
				return err
			}
			out = append(out, item)
		}
		return rows.Err()
	})
	return out, total, err
}

// AdminTempWarehouseStats aggregates total rows, active count, and archived count for temp warehouses.
func (r *Repository) AdminTempWarehouseStats(ctx context.Context, filter compare.AdminTempWarehouseFilter) (int64, int, int, error) {
	where, args := buildAdminTempWarehouseWhere(filter)
	whereClause := strings.Join(where, " AND ")

	sql := fmt.Sprintf(`
		SELECT COALESCE(SUM(f.row_count), 0)::bigint,
		       COUNT(*) FILTER (WHERE f.status = 'ready'),
		       COUNT(*) FILTER (WHERE f.status = 'archived')
		FROM compare.files f
		WHERE %s;`, whereClause)

	var totalRows int64
	var activeCount, archivedCount int

	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(txCtx, sql, args...).Scan(&totalRows, &activeCount, &archivedCount)
	})
	return totalRows, activeCount, archivedCount, err
}

// ListTempWarehouseUploaders returns the distinct uploaders behind the admin
// temporary warehouse listing, for the filter dropdown.
func (r *Repository) ListTempWarehouseUploaders(ctx context.Context) ([]compare.FileUploader, error) {
	sql := `
		SELECT DISTINCT f.user_id, ` + uploaderLabelExpr + ` AS uploader_name
		FROM compare.files f
		LEFT JOIN identity.users u ON u.id = f.user_id
		WHERE ` + adminTempWarehouseScope + `
		ORDER BY uploader_name ASC;`
	var out []compare.FileUploader
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(txCtx, sql)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var u compare.FileUploader
			if err := rows.Scan(&u.UserID, &u.Name); err != nil {
				return err
			}
			out = append(out, u)
		}
		return rows.Err()
	})
	return out, err
}

func scanAdminTempWarehouse(row pgx.Row) (*compare.AdminTempWarehouse, error) {
	var f compare.CompareFile
	var statusStr string
	var mappingBytes []byte
	var uploaderName, orgName string
	if err := row.Scan(
		&f.ID, &f.PublicID, &f.OrganizationID, &f.UserID,
		&f.SupplierName, &f.OriginalFilename, &f.StorageKey, &f.MIMEType,
		&f.SizeBytes, &f.RowCount, &statusStr, &mappingBytes,
		&f.ArchivedAt, &f.ArchiveReason, &f.ErrorMessage, &f.IsTempWarehouse, &f.Visibility,
		&f.CreatedAt, &f.UpdatedAt, &f.DeletedAt,
		&uploaderName, &orgName,
	); err != nil {
		return nil, err
	}
	f.Status = compare.CompareFileStatus(statusStr)
	return &compare.AdminTempWarehouse{CompareFile: &f, UploaderName: uploaderName, OrgName: orgName}, nil
}
