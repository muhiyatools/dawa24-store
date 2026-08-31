package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/compare"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

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
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(txCtx, `
			SELECT DISTINCT f.supplier_name
			FROM compare.files f
			JOIN compare.file_rows r ON r.file_id = f.id
			WHERE (f.is_temp_warehouse = TRUE OR f.visibility = 'public')
			  AND f.status != 'failed'
			  AND (f.deleted_at IS NULL OR f.status = 'ready') 
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
		"(f.is_temp_warehouse = TRUE OR f.visibility = 'public')",
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

	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
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
