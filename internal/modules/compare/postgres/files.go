package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/compare"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
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
			if row.MatchMethod == "" {
				row.MatchMethod = compare.MatchMethodUnmatched
			}
			batch.Queue(query,
				row.FileID, row.OrganizationID, row.RowNumber, row.RawName, row.NormalizedName, row.SKU,
				row.Price, row.Discount, row.PriceAfterDiscount, row.MatchedProductID, row.MatchConfidence, string(row.MatchMethod), metaJSON,
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

func (r *Repository) DeleteFileRows(ctx context.Context, fileID int64) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(txCtx, `DELETE FROM compare.file_rows WHERE file_id = $1;`, fileID)
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
		_, err := tx.Exec(txCtx, query, matchedProductID, string(method), confidence, rowID)
		return err
	})
}

func (r *Repository) SaveCustomerProductMapping(ctx context.Context, orgID *int64, rawName string, productID int64, source string) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		// Use default org 1 if nil for platform-wide manual mapping
		effectiveOrgID := int64(1)
		if orgID != nil && *orgID > 0 {
			effectiveOrgID = *orgID
		}
		query := `
			INSERT INTO catalog.customer_product_mappings (
				organization_id, product_id, raw_name, source, status, is_active, created_at, updated_at
			) VALUES ($1, $2, $3, $4, 'processed', true, now(), now())
			ON CONFLICT DO NOTHING;
		`
		_, err := tx.Exec(txCtx, query, effectiveOrgID, productID, rawName, source)
		return err
	})
}

func (r *Repository) GetSavedProductMapping(ctx context.Context, orgID *int64, rawName string) (*int64, error) {
	var productID int64
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT product_id
			FROM catalog.customer_product_mappings
			WHERE raw_name = $1 AND is_active = true AND status = 'processed'
			  AND ($2::bigint IS NULL OR organization_id = $2 OR organization_id = 1)
			ORDER BY (organization_id = $2) DESC, id DESC
			LIMIT 1;
		`
		return tx.QueryRow(txCtx, query, rawName, orgID).Scan(&productID)
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
