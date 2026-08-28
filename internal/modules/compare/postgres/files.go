package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/compare"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

const fileColumns = `id, public_id, organization_id, user_id, supplier_name, original_filename, storage_key, mime_type, size_bytes, row_count, status, mapping_config, archived_at, archive_reason, error_message, created_at, updated_at, deleted_at`

func scanFile(row pgx.Row) (*compare.CompareFile, error) {
	var f compare.CompareFile
	var statusStr string
	var mappingBytes []byte
	if err := row.Scan(
		&f.ID, &f.PublicID, &f.OrganizationID, &f.UserID,
		&f.SupplierName, &f.OriginalFilename, &f.StorageKey, &f.MIMEType,
		&f.SizeBytes, &f.RowCount, &statusStr, &mappingBytes,
		&f.ArchivedAt, &f.ArchiveReason, &f.ErrorMessage,
		&f.CreatedAt, &f.UpdatedAt, &f.DeletedAt,
	); err != nil {
		return nil, err
	}
	f.Status = compare.CompareFileStatus(statusStr)
	if len(mappingBytes) > 0 {
		_ = json.Unmarshal(mappingBytes, &f.MappingConfig)
	}
	return &f, nil
}

func (r *Repository) CreateFile(ctx context.Context, f *compare.CompareFile) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			INSERT INTO compare.files (
				organization_id, user_id, supplier_name, original_filename, storage_key,
				mime_type, size_bytes, row_count, status, mapping_config, error_message
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
			RETURNING id, public_id, created_at, updated_at;
		`
		mappingJSON, _ := json.Marshal(f.MappingConfig)
		if f.Status == "" {
			f.Status = compare.FileUploaded
		}
		return tx.QueryRow(txCtx, query,
			f.OrganizationID, f.UserID, f.SupplierName, f.OriginalFilename, f.StorageKey,
			f.MIMEType, f.SizeBytes, f.RowCount, string(f.Status), mappingJSON, f.ErrorMessage,
		).Scan(&f.ID, &f.PublicID, &f.CreatedAt, &f.UpdatedAt)
	})
}

func (r *Repository) GetFileByID(ctx context.Context, id int64) (*compare.CompareFile, error) {
	var f *compare.CompareFile
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `SELECT ` + fileColumns + ` FROM compare.files WHERE id = $1 AND deleted_at IS NULL;`
		var err error
		f, err = scanFile(tx.QueryRow(txCtx, query, id))
		if err != nil {
			if err == pgx.ErrNoRows {
				return apperr.NotFound("compare file")
			}
			return err
		}
		return nil
	})
	return f, err
}

func (r *Repository) GetFileByPublicID(ctx context.Context, publicID string) (*compare.CompareFile, error) {
	var f *compare.CompareFile
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `SELECT ` + fileColumns + ` FROM compare.files WHERE public_id = $1::uuid AND deleted_at IS NULL;`
		var err error
		f, err = scanFile(tx.QueryRow(txCtx, query, publicID))
		if err != nil {
			if err == pgx.ErrNoRows {
				return apperr.NotFound("compare file")
			}
			return err
		}
		return nil
	})
	return f, err
}

func (r *Repository) ListFiles(ctx context.Context, userID int64, orgID *int64, status *compare.CompareFileStatus) ([]*compare.CompareFile, error) {
	var list []*compare.CompareFile
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT ` + fileColumns + `
			FROM compare.files
			WHERE deleted_at IS NULL
			  AND ($1::text IS NULL OR status = $1)
			  AND (
			      user_id = $3
			      OR ($2::bigint IS NOT NULL AND organization_id = $2)
			  )
			ORDER BY created_at DESC;
		`
		var statusStr *string
		if status != nil {
			s := string(*status)
			statusStr = &s
		}
		rows, err := tx.Query(txCtx, query, statusStr, orgID, userID)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			f, err := scanFile(rows)
			if err != nil {
				return err
			}
			list = append(list, f)
		}
		return rows.Err()
	})
	return list, err
}

func (r *Repository) ListAllFiles(ctx context.Context, search string, status *compare.CompareFileStatus) ([]*compare.CompareFile, error) {
	var list []*compare.CompareFile
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		var whereClauses []string
		var args []any
		argIdx := 1

		whereClauses = append(whereClauses, "deleted_at IS NULL")

		if status != nil {
			whereClauses = append(whereClauses, fmt.Sprintf("status = $%d", argIdx))
			args = append(args, string(*status))
			argIdx++
		}

		if search != "" {
			q := "%" + strings.TrimSpace(search) + "%"
			whereClauses = append(whereClauses, fmt.Sprintf("(supplier_name ILIKE $%d OR original_filename ILIKE $%d)", argIdx, argIdx))
			args = append(args, q)
			argIdx++
		}

		sql := `SELECT ` + fileColumns + ` FROM compare.files WHERE ` + strings.Join(whereClauses, " AND ") + ` ORDER BY created_at DESC;`
		rows, err := tx.Query(txCtx, sql, args...)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			f, err := scanFile(rows)
			if err != nil {
				return err
			}
			list = append(list, f)
		}
		return rows.Err()
	})
	return list, err
}

func (r *Repository) CountActiveFiles(ctx context.Context, userID int64, orgID *int64) (int, error) {
	var count int
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT COUNT(*)
			FROM compare.files
			WHERE deleted_at IS NULL
			  AND status != 'archived'
			  AND (
			      user_id = $2
			      OR ($1::bigint IS NOT NULL AND organization_id = $1)
			  );
		`
		return tx.QueryRow(txCtx, query, orgID, userID).Scan(&count)
	})
	return count, err
}

func (r *Repository) UpdateFile(ctx context.Context, f *compare.CompareFile) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			UPDATE compare.files
			SET supplier_name = $2, row_count = $3, status = $4, mapping_config = $5,
			    archived_at = $6, archive_reason = $7, error_message = $8, updated_at = now()
			WHERE id = $1 AND deleted_at IS NULL;
		`
		mappingJSON, _ := json.Marshal(f.MappingConfig)
		res, err := tx.Exec(txCtx, query,
			f.ID, f.SupplierName, f.RowCount, string(f.Status), mappingJSON,
			f.ArchivedAt, f.ArchiveReason, f.ErrorMessage,
		)
		if err != nil {
			return err
		}
		if res.RowsAffected() == 0 {
			return apperr.NotFound("compare file")
		}
		return nil
	})
}

func (r *Repository) RenameFile(ctx context.Context, id int64, newSupplierName string) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		res, err := tx.Exec(txCtx, `UPDATE compare.files SET supplier_name = $2, updated_at = now() WHERE id = $1 AND deleted_at IS NULL;`, id, newSupplierName)
		if err != nil {
			return err
		}
		if res.RowsAffected() == 0 {
			return apperr.NotFound("compare file")
		}
		return nil
	})
}

// ArchiveOldestFiles implements the retention policy: archives oldest active files to maintain max quota limit (Laravel parity).
func (r *Repository) ArchiveOldestFiles(ctx context.Context, userID int64, orgID *int64, keepCount int, reason string) ([]string, error) {
	var archivedNames []string
	err := r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			WITH active AS (
				SELECT id, supplier_name, ROW_NUMBER() OVER (ORDER BY updated_at ASC) AS rn,
				       COUNT(*) OVER () AS total_active
				FROM compare.files
				WHERE deleted_at IS NULL
				  AND status != 'archived'
				  AND (
				      user_id = $2
				      OR ($1::bigint IS NOT NULL AND organization_id = $1)
				  )
			),
			to_archive AS (
				SELECT id, supplier_name
				FROM active
				WHERE total_active > $3 AND rn <= (total_active - $3)
			)
			UPDATE compare.files f
			SET status = 'archived',
			    supplier_name = f.supplier_name || ' - مؤرشف ' || (
			        SELECT COUNT(*) + 1 
			        FROM compare.files f2 
			        WHERE f2.status = 'archived' 
			          AND f2.supplier_name LIKE f.supplier_name || ' - مؤرشف %'
			    ),
			    archived_at = now(),
			    archive_reason = $4,
			    updated_at = now()
			FROM to_archive ta
			WHERE f.id = ta.id
			RETURNING ta.supplier_name;
		`
		rows, err := tx.Query(txCtx, query, orgID, userID, keepCount, reason)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var name string
			if err := rows.Scan(&name); err != nil {
				return err
			}
			archivedNames = append(archivedNames, name)
		}
		return rows.Err()
	})
	return archivedNames, err
}

func (r *Repository) ArchiveFile(ctx context.Context, id int64, reason string) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		now := time.Now().UTC()
		res, err := tx.Exec(txCtx, `UPDATE compare.files SET status = 'archived', archived_at = $2, archive_reason = $3, updated_at = $2 WHERE id = $1 AND deleted_at IS NULL;`, id, now, reason)
		if err != nil {
			return err
		}
		if res.RowsAffected() == 0 {
			return apperr.NotFound("compare file")
		}
		return nil
	})
}

func (r *Repository) UnarchiveFile(ctx context.Context, id int64) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		res, err := tx.Exec(txCtx, `UPDATE compare.files SET status = 'ready', archived_at = NULL, archive_reason = NULL, updated_at = now() WHERE id = $1 AND deleted_at IS NULL;`, id)
		if err != nil {
			return err
		}
		if res.RowsAffected() == 0 {
			return apperr.NotFound("compare file")
		}
		return nil
	})
}

func (r *Repository) DeleteFile(ctx context.Context, id int64) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		res, err := tx.Exec(txCtx, `UPDATE compare.files SET deleted_at = now() WHERE id = $1 AND deleted_at IS NULL;`, id)
		if err != nil {
			return err
		}
		if res.RowsAffected() == 0 {
			return apperr.NotFound("compare file")
		}
		return nil
	})
}

// ---------------------------------------------------------------------------
// File Rows
// ---------------------------------------------------------------------------

const rowColumns = `id, file_id, organization_id, row_number, raw_name, normalized_name, sku, price, discount, price_after_discount, matched_product_id, match_confidence, match_method, meta, created_at`

func toDBMatchMethod(m compare.MatchMethod) string {
	switch string(m) {
	case "exact", "exact_name", "direct_id":
		return "exact"
	case "sku", "barcode":
		return "sku"
	case "normalized", "partial":
		return "normalized"
	case "fuzzy":
		return "fuzzy"
	case "ai":
		return "ai"
	case "manual", "saved_mapping":
		return "manual"
	default:
		return "none"
	}
}

func (r *Repository) InsertFileRows(ctx context.Context, rows []*compare.CompareFileRow) error {
	if len(rows) == 0 {
		return nil
	}
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		batch := &pgx.Batch{}
		query := `
			INSERT INTO compare.file_rows (
				file_id, organization_id, row_number, raw_name, normalized_name, sku,
				price, discount, price_after_discount, matched_product_id, match_confidence, match_method, meta
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13);
		`
		for _, row := range rows {
			metaJSON, _ := json.Marshal(row.Meta)
			dbMethod := toDBMatchMethod(row.MatchMethod)
			batch.Queue(query,
				row.FileID, row.OrganizationID, row.RowNumber, row.RawName, row.NormalizedName, row.SKU,
				row.Price, row.Discount, row.PriceAfterDiscount, row.MatchedProductID, row.MatchConfidence, dbMethod, metaJSON,
			)
		}

		br := tx.SendBatch(txCtx, batch)
		for i := 0; i < len(rows); i++ {
			if _, err := br.Exec(); err != nil {
				_ = br.Close()
				return fmt.Errorf("batch insert row %d: %w", i, err)
			}
		}
		return br.Close()
	})
}

func (r *Repository) ListFileRows(ctx context.Context, fileID int64, limit, offset int) ([]*compare.CompareFileRow, error) {
	if limit <= 0 {
		limit = 100
	}
	var list []*compare.CompareFileRow
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT ` + rowColumns + `
			FROM compare.file_rows
			WHERE file_id = $1
			ORDER BY row_number ASC
			LIMIT $2 OFFSET $3;
		`
		rows, err := tx.Query(txCtx, query, fileID, limit, offset)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var row compare.CompareFileRow
			var matchMethodStr string
			var metaBytes []byte
			if err := rows.Scan(
				&row.ID, &row.FileID, &row.OrganizationID, &row.RowNumber,
				&row.RawName, &row.NormalizedName, &row.SKU,
				&row.Price, &row.Discount, &row.PriceAfterDiscount,
				&row.MatchedProductID, &row.MatchConfidence, &matchMethodStr,
				&metaBytes, &row.CreatedAt,
			); err != nil {
				return err
			}
			row.MatchMethod = compare.MatchMethod(matchMethodStr)
			if len(metaBytes) > 0 {
				_ = json.Unmarshal(metaBytes, &row.Meta)
			}
			list = append(list, &row)
		}
		return rows.Err()
	})
	return list, err
}

func (r *Repository) GetFileRowsPaginated(ctx context.Context, fileID int64, page, limit int) ([]*compare.CompareFileRow, int64, error) {
	if limit <= 0 {
		limit = 10
	}
	if limit > 100 {
		limit = 100
	}
	if page <= 0 {
		page = 1
	}
	offset := (page - 1) * limit

	var total int64
	var list []*compare.CompareFileRow
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		if err := tx.QueryRow(txCtx, `SELECT COUNT(*) FROM compare.file_rows WHERE file_id = $1;`, fileID).Scan(&total); err != nil {
			return err
		}
		query := `
			SELECT ` + rowColumns + `
			FROM compare.file_rows
			WHERE file_id = $1
			ORDER BY row_number ASC, id ASC
			LIMIT $2 OFFSET $3;
		`
		rows, err := tx.Query(txCtx, query, fileID, limit, offset)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var row compare.CompareFileRow
			var matchMethodStr string
			var metaBytes []byte
			if err := rows.Scan(
				&row.ID, &row.FileID, &row.OrganizationID, &row.RowNumber,
				&row.RawName, &row.NormalizedName, &row.SKU,
				&row.Price, &row.Discount, &row.PriceAfterDiscount,
				&row.MatchedProductID, &row.MatchConfidence, &matchMethodStr,
				&metaBytes, &row.CreatedAt,
			); err != nil {
				return err
			}
			row.MatchMethod = compare.MatchMethod(matchMethodStr)
			if len(metaBytes) > 0 {
				_ = json.Unmarshal(metaBytes, &row.Meta)
			}
			list = append(list, &row)
		}
		return rows.Err()
	})
	return list, total, err
}

func (r *Repository) DeleteFileRows(ctx context.Context, fileID int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(txCtx, `DELETE FROM compare.file_rows WHERE file_id = $1;`, fileID)
		return err
	})
}

func (r *Repository) DeleteFileRow(ctx context.Context, rowID int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		var fileID int64
		err := tx.QueryRow(txCtx, `DELETE FROM compare.file_rows WHERE id = $1 RETURNING file_id;`, rowID).Scan(&fileID)
		if err != nil {
			return err
		}
		_, err = tx.Exec(txCtx, `
			UPDATE compare.files
			SET row_count = (SELECT COUNT(*) FROM compare.file_rows WHERE file_id = $1),
			    updated_at = NOW()
			WHERE id = $1;
		`, fileID)
		return err
	})
}

func (r *Repository) UpdateFileRowMatch(ctx context.Context, rowID int64, matchedProductID *int64, method compare.MatchMethod, confidence float64) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			UPDATE compare.file_rows
			SET matched_product_id = $1,
			    match_method = $2,
			    match_confidence = $3
			WHERE id = $4;
		`
		dbMethod := toDBMatchMethod(method)
		_, err := tx.Exec(txCtx, query, matchedProductID, dbMethod, confidence, rowID)
		return err
	})
}

func (r *Repository) SaveCustomerProductMapping(ctx context.Context, orgID *int64, rawName string, productID int64, source string) error {
	normName := strings.ToLower(strings.TrimSpace(rawName))
	decKey := "manual:" + normName

	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		var effectiveOrgID *int64
		if orgID != nil && *orgID > 0 {
			effectiveOrgID = orgID
		} else if actor, ok := authctx.From(ctx); ok && actor.OrganizationID > 0 {
			effectiveOrgID = &actor.OrganizationID
		}

		if effectiveOrgID == nil {
			return nil
		}

		// 1. Insert into catalog.customer_product_mappings
		query := `
			INSERT INTO catalog.customer_product_mappings (
				organization_id, customer_org_id, product_id, raw_name, source, status, is_active, created_at, updated_at
			) VALUES ($1, $1, $2, $3, $4, 'processed', true, now(), now())
			ON CONFLICT DO NOTHING;
		`
		if _, err := tx.Exec(txCtx, query, *effectiveOrgID, productID, rawName, source); err != nil {
			return err
		}

		// 2. Insert or update in catalog.match_decisions
		var userID *int64
		if actor, ok := authctx.From(ctx); ok && actor.UserID > 0 {
			userID = &actor.UserID
		}

		if normName != "" && productID > 0 {
			_, err := tx.Exec(txCtx, `
				INSERT INTO catalog.match_decisions (
					organization_id, user_id, decision_key, norm_name, chosen_product_id,
					confidence, reason, prompt_version, hit_count, created_at, last_used_at
				) VALUES (
					$1, $2, $3, $4, $5,
					1.000, 'قرار تصحيح يدوي من أداة المقارنة', 'manual:v1', 1, now(), now()
				)
				ON CONFLICT (COALESCE(organization_id, 0), decision_key)
				DO UPDATE SET
					chosen_product_id = EXCLUDED.chosen_product_id,
					confidence = 1.000,
					user_id = COALESCE(EXCLUDED.user_id, catalog.match_decisions.user_id),
					hit_count = catalog.match_decisions.hit_count + 1,
					last_used_at = now();
			`, *effectiveOrgID, userID, decKey, normName, productID)
			return err
		}

		return nil
	})
}

func (r *Repository) GetSavedProductMapping(ctx context.Context, orgID *int64, rawName string) (*int64, error) {
	var targetOrgID int64
	if orgID != nil && *orgID > 0 {
		targetOrgID = *orgID
	} else if actor, ok := authctx.From(ctx); ok && actor.OrganizationID > 0 {
		targetOrgID = actor.OrganizationID
	}

	if targetOrgID <= 0 {
		return nil, nil
	}

	var productID int64
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT product_id
			FROM catalog.customer_product_mappings
			WHERE raw_name = $1 AND is_active = true AND status = 'processed'
			  AND (organization_id = $2 OR customer_org_id = $2)
			ORDER BY updated_at DESC, id DESC
			LIMIT 1;
		`
		return tx.QueryRow(txCtx, query, rawName, targetOrgID).Scan(&productID)
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &productID, nil
}

func (r *Repository) FindCandidateProducts(ctx context.Context, orgID *int64, query, sku string, limit int) ([]*compare.CandidateProduct, error) {
	if limit <= 0 {
		limit = 30
	}
	var candidates []*compare.CandidateProduct

	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		// Try catalog.product_index first
		sql := `
			SELECT product_id, COALESCE(sku, ''), COALESCE(name_ar, ''), COALESCE(name_en, ''),
			       COALESCE(scientific_name, ''), COALESCE(search_simple, '')
			FROM catalog.product_index
			WHERE ($1 = '' OR sku = $1)
			   OR ($2 != '' AND (search_simple ILIKE '%' || $2 || '%' OR name_ar ILIKE '%' || $2 || '%' OR name_en ILIKE '%' || $2 || '%'))
			LIMIT $3;
		`
		rows, err := tx.Query(txCtx, sql, sku, query, limit)
		if err != nil {
			// Fall back to catalog.products
			fallbackSQL := `
				SELECT id, COALESCE(sku, ''), COALESCE(name->>'ar', ''), COALESCE(name->>'en', ''),
				       COALESCE(scientific_name, ''), ''
				FROM catalog.products
				WHERE deleted_at IS NULL
				  AND (($1 = '' OR sku = $1)
				   OR ($2 != '' AND (name->>'ar' ILIKE '%' || $2 || '%' OR name->>'en' ILIKE '%' || $2 || '%')))
				LIMIT $3;
			`
			fbRows, fbErr := tx.Query(txCtx, fallbackSQL, sku, query, limit)
			if fbErr != nil {
				return fbErr
			}
			defer fbRows.Close()
			for fbRows.Next() {
				var c compare.CandidateProduct
				if err := fbRows.Scan(&c.ID, &c.SKU, &c.NameAr, &c.NameEn, &c.ScientificName, &c.SearchSimple); err != nil {
					return err
				}
				candidates = append(candidates, &c)
			}
			return fbRows.Err()
		}
		defer rows.Close()

		for rows.Next() {
			var c compare.CandidateProduct
			if err := rows.Scan(&c.ID, &c.SKU, &c.NameAr, &c.NameEn, &c.ScientificName, &c.SearchSimple); err != nil {
				return err
			}
			candidates = append(candidates, &c)
		}
		return rows.Err()
	})

	return candidates, err
}

func (r *Repository) SearchFileRows(ctx context.Context, userID int64, orgID *int64, query string, limit int) ([]*compare.CompareFileRowWithSupplier, error) {
	if limit <= 0 {
		limit = 50
	}
	var results []*compare.CompareFileRowWithSupplier

	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		sql := `
			SELECT 
				r.id, r.file_id, f.supplier_name, r.row_number, r.raw_name, r.normalized_name, COALESCE(r.sku, ''),
				r.price, r.discount, r.price_after_discount, r.matched_product_id, r.match_confidence, r.match_method,
				COALESCE(r.meta, '{}'::jsonb), r.created_at
			FROM compare.file_rows r
			JOIN compare.files f ON r.file_id = f.id
			WHERE f.deleted_at IS NULL AND f.status = 'ready'
			  AND ($1::bigint <= 0 OR f.user_id = $1 OR ($2::bigint IS NOT NULL AND f.organization_id = $2))
			  AND ($3 = '' OR r.raw_name ILIKE '%' || $3 || '%' OR r.normalized_name ILIKE '%' || $3 || '%' OR r.sku ILIKE '%' || $3 || '%' OR f.supplier_name ILIKE '%' || $3 || '%')
			ORDER BY r.price_after_discount ASC
			LIMIT $4;
		`
		rows, err := tx.Query(txCtx, sql, userID, orgID, query, limit)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var row compare.CompareFileRowWithSupplier
			var matchMethodStr string
			var metaBytes []byte
			if err := rows.Scan(
				&row.ID, &row.FileID, &row.SupplierName, &row.RowNumber,
				&row.RawName, &row.NormalizedName, &row.SKU,
				&row.Price, &row.Discount, &row.PriceAfterDiscount,
				&row.MatchedProductID, &row.MatchConfidence, &matchMethodStr,
				&metaBytes, &row.CreatedAt,
			); err != nil {
				return err
			}
			row.MatchMethod = compare.MatchMethod(matchMethodStr)
			if len(metaBytes) > 0 {
				_ = json.Unmarshal(metaBytes, &row.Meta)
			}
			results = append(results, &row)
		}
		return rows.Err()
	})

	return results, err
}

func (r *Repository) ListDistinctSuppliers(ctx context.Context) ([]string, error) {
	var suppliers []string
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(txCtx, `
			SELECT DISTINCT f.supplier_name 
			FROM compare.files f
			JOIN compare.file_rows r ON r.file_id = f.id
			WHERE f.status != 'failed' AND (f.deleted_at IS NULL OR f.status = 'ready') 
			ORDER BY f.supplier_name ASC;
		`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var sup string
			if err := rows.Scan(&sup); err == nil && strings.TrimSpace(sup) != "" {
				suppliers = append(suppliers, sup)
			}
		}
		return rows.Err()
	})
	return suppliers, err
}

func (r *Repository) ListMarketDiscounts(ctx context.Context, filter compare.MarketDiscountsFilter) (*compare.MarketDiscountsResult, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 24
	}
	if limit > 100 {
		limit = 100
	}
	page := filter.Page
	if page <= 0 {
		page = 1
	}
	offset := (page - 1) * limit

	var args []any
	argIdx := 1

	whereClauses := []string{
		"f.status != 'failed'",
		"(f.deleted_at IS NULL OR f.status = 'ready')",
	}

	if filter.Query != "" {
		q := strings.TrimSpace(filter.Query)
		whereClauses = append(whereClauses, fmt.Sprintf("(r.raw_name ILIKE $%d OR r.normalized_name ILIKE $%d OR r.sku ILIKE $%d OR f.supplier_name ILIKE $%d)", argIdx, argIdx, argIdx, argIdx))
		args = append(args, "%"+q+"%")
		argIdx++
	}

	if filter.Supplier != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("f.supplier_name = $%d", argIdx))
		args = append(args, strings.TrimSpace(filter.Supplier))
		argIdx++
	}

	if filter.MinPrice != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("r.price_after_discount >= $%d", argIdx))
		args = append(args, int64(*filter.MinPrice*100))
		argIdx++
	}
	if filter.MaxPrice != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("r.price_after_discount <= $%d", argIdx))
		args = append(args, int64(*filter.MaxPrice*100))
		argIdx++
	}

	if filter.MinDiscount != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("r.discount >= $%d", argIdx))
		args = append(args, *filter.MinDiscount)
		argIdx++
	}
	if filter.MaxDiscount != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("r.discount <= $%d", argIdx))
		args = append(args, *filter.MaxDiscount)
		argIdx++
	}

	orderBy := "r.created_at DESC"
	switch filter.SortBy {
	case "oldest":
		orderBy = "r.created_at ASC"
	case "discount_desc":
		orderBy = "r.discount DESC, r.price_after_discount ASC"
	case "price_asc":
		orderBy = "r.price_after_discount ASC, r.discount DESC"
	case "price_desc":
		orderBy = "r.price_after_discount DESC, r.discount DESC"
	case "newest":
		fallthrough
	default:
		orderBy = "r.created_at DESC"
	}

	sql := fmt.Sprintf(`
		SELECT 
			r.id, r.file_id, f.supplier_name, r.raw_name, COALESCE(r.sku, ''),
			r.price, r.discount, r.price_after_discount, r.matched_product_id, r.created_at,
			COUNT(*) OVER() AS total_count
		FROM compare.file_rows r
		JOIN compare.files f ON r.file_id = f.id
		WHERE %s
		ORDER BY %s
		LIMIT $%d OFFSET $%d;
	`, strings.Join(whereClauses, " AND "), orderBy, argIdx, argIdx+1)

	args = append(args, limit, offset)

	result := &compare.MarketDiscountsResult{
		Items:      make([]*compare.MarketDiscountRow, 0),
		Page:       page,
		Limit:      limit,
		TotalPages: 1,
	}

	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(txCtx, sql, args...)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var item compare.MarketDiscountRow
			var totalCount int64
			if err := rows.Scan(
				&item.ID, &item.FileID, &item.SupplierName, &item.ProductName, &item.SKU,
				&item.OriginalPrice, &item.DiscountPercent, &item.PriceAfterDiscount,
				&item.MatchedProductID, &item.CreatedAt, &totalCount,
			); err != nil {
				return err
			}
			result.TotalCount = totalCount
			if item.PriceAfterDiscount.IsZero() && item.OriginalPrice.IsPositive() {
				item.PriceAfterDiscount = compare.CalculatePriceAfterDiscount(item.OriginalPrice, item.DiscountPercent)
			}
			if item.OriginalPrice.Minor() > item.PriceAfterDiscount.Minor() {
				item.DiscountValue = money.FromMinor(item.OriginalPrice.Minor() - item.PriceAfterDiscount.Minor())
			}
			item.InCatalog = (item.MatchedProductID != nil && *item.MatchedProductID > 0)
			result.Items = append(result.Items, &item)
		}
		return rows.Err()
	})

	if result.TotalCount > 0 {
		result.TotalPages = int((result.TotalCount + int64(limit) - 1) / int64(limit))
	}
	result.HasPrev = page > 1
	result.HasNext = page < result.TotalPages

	suppliers, _ := r.ListDistinctSuppliers(ctx)
	result.AvailableSuppliers = suppliers

	return result, err
}
