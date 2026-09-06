package postgres

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/compare"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

func (r *Repository) FindCandidateProducts(ctx context.Context, orgID *int64, query, sku string, limit int) ([]*compare.CandidateProduct, error) {
	if limit <= 0 {
		limit = 30
	}
	var candidates []*compare.CandidateProduct

	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		// Try catalog.product_index first
		// The barcode is joined in rather than read from the index, which has no
		// barcode column. Without it the match ladder had nothing to compare a
		// row's barcode against and compared it to the SKU instead — two
		// unrelated numbering schemes tested for equality.
		sql := `
			SELECT i.product_id, COALESCE(i.sku, ''), COALESCE(i.name_ar, ''), COALESCE(i.name_en, ''),
			       COALESCE(i.scientific_name, ''), COALESCE(i.search_simple, ''), COALESCE(p.barcode, '')
			FROM catalog.product_index i
			LEFT JOIN catalog.products p ON p.id = i.product_id
			WHERE ($1 = '' OR i.sku = $1)
			   OR ($2 != '' AND (i.search_simple ILIKE '%' || $2 || '%' OR i.name_ar ILIKE '%' || $2 || '%' OR i.name_en ILIKE '%' || $2 || '%'))
			LIMIT $3;
		`
		rows, err := tx.Query(txCtx, sql, sku, query, limit)
		if err != nil {
			// Fall back to catalog.products
			fallbackSQL := `
				SELECT id, COALESCE(sku, ''), COALESCE(name->>'ar', ''), COALESCE(name->>'en', ''),
				       COALESCE(scientific_name, ''), '', COALESCE(barcode, '')
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
				if err := fbRows.Scan(&c.ID, &c.SKU, &c.NameAr, &c.NameEn, &c.ScientificName, &c.SearchSimple, &c.Barcode); err != nil {
					return err
				}
				candidates = append(candidates, &c)
			}
			return fbRows.Err()
		}
		defer rows.Close()

		for rows.Next() {
			var c compare.CandidateProduct
			if err := rows.Scan(&c.ID, &c.SKU, &c.NameAr, &c.NameEn, &c.ScientificName, &c.SearchSimple, &c.Barcode); err != nil {
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
			  AND f.is_temp_warehouse = FALSE
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

// PurgeExpiredCompareFiles soft-deletes compare files that have exceeded their retention period.
// It checks both the user's/org's subscription plan retention limit and the fallback defaultRetentionDays.
func (r *Repository) PurgeExpiredCompareFiles(ctx context.Context, defaultRetentionDays int) (int64, error) {
	if defaultRetentionDays <= 0 {
		defaultRetentionDays = 30
	}
	var purgedCount int64
	err := r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			WITH target_files AS (
				SELECT f.id, f.storage_key
				FROM compare.files f
				LEFT JOIN billing.subscriptions s ON s.organization_id = f.organization_id AND s.status = 'active'
				LEFT JOIN billing.plans p ON p.id = s.plan_id
				WHERE f.deleted_at IS NULL
				  AND f.is_temp_warehouse = false
				  AND (
				      (COALESCE(NULLIF(p.features->>'compare_file_retention_days', ''), '0')::int > 0 
				       AND f.created_at < (now() - (COALESCE(NULLIF(p.features->>'compare_file_retention_days', ''), '0')::int || ' days')::interval))
				      OR
				      ($1::int > 0 AND (COALESCE(NULLIF(p.features->>'compare_file_retention_days', ''), '0')::int <= 0)
				       AND f.created_at < (now() - ($1::int || ' days')::interval))
				  )
			),
			deleted_rows AS (
				DELETE FROM compare.file_rows
				WHERE file_id IN (SELECT id FROM target_files)
			),
			updated_files AS (
				UPDATE compare.files
				SET deleted_at = now(), updated_at = now()
				WHERE id IN (SELECT id FROM target_files)
				RETURNING id
			)
			SELECT COUNT(*) FROM updated_files;
		`
		return tx.QueryRow(txCtx, query, defaultRetentionDays).Scan(&purgedCount)
	})
	return purgedCount, err
}
