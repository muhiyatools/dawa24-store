package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// UpsertProductIndex inserts or updates a read-model item in catalog.product_index.
func (r *Repository) UpsertProductIndex(ctx context.Context, item *catalog.ProductIndexItem) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			INSERT INTO catalog.product_index (
				unique_row_id, product_id, variant_id, sku, name_ar, name_en,
				search_text, search_ar, search_en, search_simple,
				organization_name, branch_city, scientific_name, price, discount,
				stock_quantity, category_id, brand_id, has_discount,
				discount_percentage, price_after_discount, organization_id, branch_id,
				status, product_type, institutional_work_ids, updated_at
			) VALUES (
				$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15,
				$16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26, now()
			)
			ON CONFLICT (unique_row_id) DO UPDATE SET
				sku = EXCLUDED.sku,
				name_ar = EXCLUDED.name_ar,
				name_en = EXCLUDED.name_en,
				search_text = EXCLUDED.search_text,
				search_ar = EXCLUDED.search_ar,
				search_en = EXCLUDED.search_en,
				search_simple = EXCLUDED.search_simple,
				organization_name = EXCLUDED.organization_name,
				branch_city = EXCLUDED.branch_city,
				scientific_name = EXCLUDED.scientific_name,
				price = EXCLUDED.price,
				discount = EXCLUDED.discount,
				stock_quantity = EXCLUDED.stock_quantity,
				category_id = EXCLUDED.category_id,
				brand_id = EXCLUDED.brand_id,
				has_discount = EXCLUDED.has_discount,
				discount_percentage = EXCLUDED.discount_percentage,
				price_after_discount = EXCLUDED.price_after_discount,
				organization_id = EXCLUDED.organization_id,
				branch_id = EXCLUDED.branch_id,
				status = EXCLUDED.status,
				product_type = EXCLUDED.product_type,
				institutional_work_ids = EXCLUDED.institutional_work_ids,
				updated_at = now();
		`
		_, err := tx.Exec(txCtx, query,
			item.UniqueRowID, item.ProductID, item.VariantID, item.SKU,
			item.NameAR, item.NameEN, item.SearchText, item.SearchAR, item.SearchEN, item.SearchSimple,
			item.OrganizationName, item.BranchCity, item.ScientificName, item.Price, item.Discount,
			item.StockQuantity, item.CategoryID, item.BrandID, item.HasDiscount,
			item.DiscountPercentage, item.PriceAfterDiscount, item.OrganizationID, item.BranchID,
			item.Status, item.ProductType, item.InstitutionalWorkIDs,
		)
		if err != nil {
			return fmt.Errorf("catalog postgres: upsert product_index: %w", err)
		}
		return nil
	})
}

// DeleteProductIndex removes a single read model entry by unique_row_id.
func (r *Repository) DeleteProductIndex(ctx context.Context, uniqueRowID string) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `DELETE FROM catalog.product_index WHERE unique_row_id = $1;`
		_, err := tx.Exec(txCtx, query, uniqueRowID)
		if err != nil {
			return fmt.Errorf("catalog postgres: delete product_index: %w", err)
		}
		return nil
	})
}

// DeleteProductIndexByProduct removes all index entries for a master product.
func (r *Repository) DeleteProductIndexByProduct(ctx context.Context, productID int64) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `DELETE FROM catalog.product_index WHERE product_id = $1;`
		_, err := tx.Exec(txCtx, query, productID)
		if err != nil {
			return fmt.Errorf("catalog postgres: delete product_index by product: %w", err)
		}
		return nil
	})
}

// SearchProductIndex queries the denormalized read model with fast fulltext, trigrams, and institutional filtering.
// Uses database.AsSystem because cross-tenant catalogue discovery is the core function of the multi-vendor marketplace.
func (r *Repository) SearchProductIndex(ctx context.Context, params catalog.SearchParams) ([]*catalog.ProductIndexItem, error) {
	var items []*catalog.ProductIndexItem
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT unique_row_id, product_id, variant_id, sku, name_ar, name_en,
			       search_text, search_ar, search_en, search_simple,
			       organization_name, branch_city, scientific_name, price, discount,
			       stock_quantity, category_id, brand_id, has_discount,
			       discount_percentage, price_after_discount, organization_id, branch_id,
			       status, product_type, COALESCE(institutional_work_ids, '{}'::bigint[]),
			       created_at, updated_at
			FROM catalog.product_index
			WHERE status = 'active'
			  AND ($1 = '' 
			       OR search_vector @@ plainto_tsquery('simple', $1)
			       OR search_text ILIKE '%' || $1 || '%'
			       OR search_simple ILIKE '%' || platform.normalize_arabic($1) || '%'
			       OR name_ar ILIKE '%' || $1 || '%'
			       OR name_en ILIKE '%' || $1 || '%'
			       OR sku ILIKE '%' || $1 || '%'
			       OR COALESCE(scientific_name, '') ILIKE '%' || $1 || '%'
			       OR word_similarity(platform.normalize_arabic($1), search_simple) >= 0.25
			       OR similarity(search_simple, platform.normalize_arabic($1)) >= 0.15
			       OR regexp_replace(search_simple, '[اوي]', '', 'g') ILIKE '%' || regexp_replace(platform.normalize_arabic($1), '[اوي]', '', 'g') || '%')
			  AND ($2::bigint IS NULL OR category_id = $2)
			  AND ($3::bigint IS NULL OR brand_id = $3)
			  AND ($6::numeric IS NULL OR price_after_discount >= $6)
			  AND ($7::numeric IS NULL OR price_after_discount <= $7)
			  AND (
			      ($8::int = 0 AND ($9::bigint[] IS NULL OR cardinality($9::bigint[]) = 0 OR cardinality(institutional_work_ids) = 0 OR institutional_work_ids && $9))
			      OR
			      ($8::int = 1 AND ($9::bigint[] IS NOT NULL AND cardinality($9::bigint[]) > 0 AND institutional_work_ids && $9))
			  )
			ORDER BY 
			  CASE 
			    WHEN $1 = '' THEN 0
			    WHEN search_simple ILIKE platform.normalize_arabic($1) || '%' THEN 1
			    WHEN search_en ILIKE $1 || '%' THEN 2
			    WHEN search_simple ILIKE '%' || platform.normalize_arabic($1) || '%' THEN 3
			    WHEN search_en ILIKE '%' || $1 || '%' THEN 4
			    ELSE 5
			  END,
			  price_after_discount ASC, updated_at DESC
			LIMIT $4 OFFSET $5;
		`
		limit := params.Limit
		if limit <= 0 {
			limit = 50
		} else if limit > 1000 {
			limit = 1000
		}

		rows, err := tx.Query(txCtx, query,
			params.Query, params.CategoryID, params.BrandID, limit, params.Offset,
			params.MinPrice, params.MaxPrice, params.FilterMode, params.AllowedWorkIDs,
		)
		if err != nil {
			return fmt.Errorf("catalog postgres: search product_index: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var it catalog.ProductIndexItem
			if err := rows.Scan(
				&it.UniqueRowID, &it.ProductID, &it.VariantID, &it.SKU,
				&it.NameAR, &it.NameEN, &it.SearchText, &it.SearchAR, &it.SearchEN, &it.SearchSimple,
				&it.OrganizationName, &it.BranchCity, &it.ScientificName, &it.Price, &it.Discount,
				&it.StockQuantity, &it.CategoryID, &it.BrandID, &it.HasDiscount,
				&it.DiscountPercentage, &it.PriceAfterDiscount, &it.OrganizationID, &it.BranchID,
				&it.Status, &it.ProductType, &it.InstitutionalWorkIDs,
				&it.CreatedAt, &it.UpdatedAt,
			); err != nil {
				return err
			}
			items = append(items, &it)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return items, nil
}

// RebuildProductIndex truncates and fully repopulates catalog.product_index from master tables.
func (r *Repository) RebuildProductIndex(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(txCtx, `TRUNCATE TABLE catalog.product_index;`); err != nil {
			return fmt.Errorf("truncate product_index: %w", err)
		}

		query := `
			INSERT INTO catalog.product_index (
				unique_row_id, product_id, variant_id, sku, name_ar, name_en,
				search_text, search_ar, search_en, search_simple,
				organization_name, branch_city, scientific_name, price, discount,
				stock_quantity, category_id, brand_id, has_discount,
				discount_percentage, price_after_discount, organization_id, branch_id,
				status, product_type, institutional_work_ids
			)
			SELECT 
				'p_' || p.id::text AS unique_row_id,
				p.id AS product_id,
				NULL::bigint AS variant_id,
				p.sku,
				p.name->>'ar' AS name_ar,
				p.name->>'en' AS name_en,
				CONCAT_WS(' ', COALESCE(p.name->>'ar', ''), COALESCE(p.name->>'en', ''), COALESCE(p.scientific_name, ''), COALESCE(p.pharmacology, ''), COALESCE(p.manufacturing_companies, ''), COALESCE(o.name->>'ar', ''), COALESCE(p.sku, '')) AS search_text,
				CONCAT_WS(' ', COALESCE(p.name->>'ar', ''), COALESCE(p.scientific_name, ''), COALESCE(p.pharmacology, ''), COALESCE(p.manufacturing_companies, ''), COALESCE(o.name->>'ar', '')) AS search_ar,
				CONCAT_WS(' ', COALESCE(p.name->>'en', ''), COALESCE(p.scientific_name, ''), COALESCE(p.pharmacology, ''), COALESCE(p.manufacturing_companies, ''), COALESCE(o.name->>'en', '')) AS search_en,
				CONCAT_WS(' ', platform.normalize_arabic(COALESCE(p.name->>'ar', '')), COALESCE(p.name->>'en', ''), COALESCE(p.sku, '')) AS search_simple,
				COALESCE(o.name->>'ar', o.name->>'en', 'دواء 24') AS organization_name,
				COALESCE(b.name->>'ar', 'المستودع الرئيسي') AS branch_city,
				p.scientific_name,
				p.price,
				p.discount,
				0 AS stock_quantity,
				p.category_id,
				p.brand_id,
				(p.discount > 0) AS has_discount,
				p.discount AS discount_percentage,
				ROUND(p.price * (1 - p.discount / 100), 2) AS price_after_discount,
				p.organization_id,
				p.branch_id,
				p.status::text AS status,
				'parent' AS product_type,
				COALESCE(p.institutional_work_ids, '{}'::bigint[])
			FROM catalog.products p
			JOIN org.organizations o ON p.organization_id = o.id
			LEFT JOIN org.branches b ON p.branch_id = b.id
			WHERE p.deleted_at IS NULL AND o.deleted_at IS NULL AND p.status = 'active';
		`
		res, err := tx.Exec(txCtx, query)
		if err != nil {
			return fmt.Errorf("rebuild product_index: %w", err)
		}
		count = res.RowsAffected()
		return nil
	})
	if err != nil {
		return 0, err
	}
	return count, nil
}
