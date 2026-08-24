package postgres

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// CreateVariant inserts a product variant.
func (r *Repository) CreateVariant(ctx context.Context, v *catalog.ProductVariant) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			INSERT INTO catalog.product_variants (
				organization_id, product_id, name, sku, barcode, price, cost_price,
				discount, unit, image, status, is_featured, batch_number, expiry_date,
				min_order_qty, branch_id
			) VALUES (
				$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16
			) RETURNING id, public_id, created_at, updated_at;
		`
		minQty := v.MinOrderQty
		if minQty <= 0 {
			minQty = 1
		}
		err := tx.QueryRow(txCtx, query,
			v.OrganizationID, v.ProductID, v.Name, v.SKU, v.Barcode, v.Price,
			v.CostPrice, v.Discount, v.Unit, v.Image, string(v.Status), v.IsFeatured,
			v.BatchNumber, v.ExpiryDate, minQty, v.BranchID,
		).Scan(&v.ID, &v.PublicID, &v.CreatedAt, &v.UpdatedAt)

		if err != nil {
			return fmt.Errorf("catalog postgres: create variant: %w", err)
		}
		return nil
	})
}

// GetVariantByID retrieves a variant by ID.
func (r *Repository) GetVariantByID(ctx context.Context, id int64) (*catalog.ProductVariant, error) {
	var v catalog.ProductVariant
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, public_id, organization_id, product_id, name, sku, barcode,
			       price, cost_price, discount, unit, image, status, is_featured,
			       batch_number, expiry_date, min_order_qty, branch_id,
			       created_at, updated_at, deleted_at
			FROM catalog.product_variants
			WHERE id = $1 AND deleted_at IS NULL;
		`
		var statusStr string
		err := tx.QueryRow(txCtx, query, id).Scan(
			&v.ID, &v.PublicID, &v.OrganizationID, &v.ProductID, &v.Name, &v.SKU,
			&v.Barcode, &v.Price, &v.CostPrice, &v.Discount, &v.Unit, &v.Image,
			&statusStr, &v.IsFeatured, &v.BatchNumber, &v.ExpiryDate, &v.MinOrderQty,
			&v.BranchID, &v.CreatedAt, &v.UpdatedAt, &v.DeletedAt,
		)
		if err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("product_variant")
			}
			return fmt.Errorf("catalog postgres: get variant: %w", err)
		}
		v.Status = catalog.ProductStatus(statusStr)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &v, nil
}

// ListVariantsByProduct retrieves all active variants of a product.
func (r *Repository) ListVariantsByProduct(ctx context.Context, productID int64) ([]*catalog.ProductVariant, error) {
	var variants []*catalog.ProductVariant
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, public_id, organization_id, product_id, name, sku, barcode,
			       price, cost_price, discount, unit, image, status, is_featured,
			       batch_number, expiry_date, min_order_qty, branch_id,
			       created_at, updated_at, deleted_at
			FROM catalog.product_variants
			WHERE product_id = $1 AND deleted_at IS NULL
			ORDER BY id ASC;
		`
		rows, err := tx.Query(txCtx, query, productID)
		if err != nil {
			return fmt.Errorf("catalog postgres: list variants: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var v catalog.ProductVariant
			var statusStr string
			if err := rows.Scan(
				&v.ID, &v.PublicID, &v.OrganizationID, &v.ProductID, &v.Name, &v.SKU,
				&v.Barcode, &v.Price, &v.CostPrice, &v.Discount, &v.Unit, &v.Image,
				&statusStr, &v.IsFeatured, &v.BatchNumber, &v.ExpiryDate, &v.MinOrderQty,
				&v.BranchID, &v.CreatedAt, &v.UpdatedAt, &v.DeletedAt,
			); err != nil {
				return err
			}
			v.Status = catalog.ProductStatus(statusStr)
			variants = append(variants, &v)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return variants, nil
}

// ListVariantsByOrganization retrieves variants belonging to an organization with pagination and search.
func (r *Repository) ListVariantsByOrganization(ctx context.Context, orgID int64, params catalog.VariantSearchParams) ([]*catalog.ProductVariant, int, error) {
	var variants []*catalog.ProductVariant
	var total int

	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		whereClauses := []string{"v.organization_id = $1", "v.deleted_at IS NULL"}
		args := []any{orgID}
		argIdx := 2

		if params.Status != "" {
			whereClauses = append(whereClauses, fmt.Sprintf("v.status = $%d", argIdx))
			args = append(args, params.Status)
			argIdx++
		}

		if params.Query != "" {
			whereClauses = append(whereClauses, fmt.Sprintf("(v.name->>'ar' ILIKE $%d OR v.name->>'en' ILIKE $%d OR v.sku ILIKE $%d OR v.barcode ILIKE $%d OR v.batch_number ILIKE $%d)", argIdx, argIdx, argIdx, argIdx, argIdx))
			args = append(args, "%"+strings.TrimSpace(params.Query)+"%")
			argIdx++
		}

		whereSQL := strings.Join(whereClauses, " AND ")

		// 1. Count query
		countQuery := fmt.Sprintf("SELECT COUNT(*) FROM catalog.product_variants v WHERE %s;", whereSQL)
		if err := tx.QueryRow(txCtx, countQuery, args...).Scan(&total); err != nil {
			return fmt.Errorf("count variants by org: %w", err)
		}

		limit := params.Limit
		if limit <= 0 {
			limit = 24
		}
		offset := params.Offset
		if offset < 0 {
			offset = 0
		}

		// 2. Data query
		dataQuery := fmt.Sprintf(`
			SELECT v.id, v.public_id, v.organization_id, v.product_id, v.name, v.sku, v.barcode,
			       v.price, v.cost_price, v.discount, v.unit, v.image, v.status, v.is_featured,
			       v.batch_number, v.expiry_date, v.min_order_qty, v.branch_id,
			       v.created_at, v.updated_at, v.deleted_at
			FROM catalog.product_variants v
			WHERE %s
			ORDER BY v.id DESC
			LIMIT $%d OFFSET $%d;
		`, whereSQL, argIdx, argIdx+1)

		args = append(args, limit, offset)
		rows, err := tx.Query(txCtx, dataQuery, args...)
		if err != nil {
			return fmt.Errorf("list variants by org: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var v catalog.ProductVariant
			var statusStr string
			if err := rows.Scan(
				&v.ID, &v.PublicID, &v.OrganizationID, &v.ProductID, &v.Name, &v.SKU,
				&v.Barcode, &v.Price, &v.CostPrice, &v.Discount, &v.Unit, &v.Image,
				&statusStr, &v.IsFeatured, &v.BatchNumber, &v.ExpiryDate, &v.MinOrderQty,
				&v.BranchID, &v.CreatedAt, &v.UpdatedAt, &v.DeletedAt,
			); err != nil {
				return err
			}
			v.Status = catalog.ProductStatus(statusStr)
			variants = append(variants, &v)
		}
		return rows.Err()
	})

	if err != nil {
		return nil, 0, err
	}
	return variants, total, nil
}

// ListAllVariants retrieves all product variants across all organizations (for platform admin) with pagination, search, and stock quantity aggregation.
func (r *Repository) ListAllVariants(ctx context.Context, params catalog.VariantSearchParams) ([]*catalog.ProductVariant, int, error) {
	var variants []*catalog.ProductVariant
	var total int

	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		whereClauses := []string{"v.deleted_at IS NULL"}
		args := []any{}
		argIdx := 1

		if params.Status != "" {
			whereClauses = append(whereClauses, fmt.Sprintf("v.status = $%d", argIdx))
			args = append(args, params.Status)
			argIdx++
		}

		if params.Query != "" {
			whereClauses = append(whereClauses, fmt.Sprintf("(v.name->>'ar' ILIKE $%d OR v.name->>'en' ILIKE $%d OR v.sku ILIKE $%d OR v.barcode ILIKE $%d OR v.batch_number ILIKE $%d)", argIdx, argIdx, argIdx, argIdx, argIdx))
			args = append(args, "%"+strings.TrimSpace(params.Query)+"%")
			argIdx++
		}

		whereSQL := strings.Join(whereClauses, " AND ")

		// 1. Count query
		countQuery := fmt.Sprintf("SELECT COUNT(*) FROM catalog.product_variants v WHERE %s;", whereSQL)
		if err := tx.QueryRow(txCtx, countQuery, args...).Scan(&total); err != nil {
			return fmt.Errorf("count all variants: %w", err)
		}

		limit := params.Limit
		if limit <= 0 {
			limit = 50
		}
		offset := params.Offset
		if offset < 0 {
			offset = 0
		}

		// 2. Data query with aggregated stock quantity
		dataQuery := fmt.Sprintf(`
			SELECT v.id, v.public_id, v.organization_id, v.product_id, v.name, v.sku, v.barcode,
			       v.price, v.cost_price, v.discount, v.unit, v.image, v.status, v.is_featured,
			       v.batch_number, v.expiry_date, v.min_order_qty, v.branch_id,
			       COALESCE(SUM(s.quantity), 0) as stock_qty,
			       v.created_at, v.updated_at, v.deleted_at
			FROM catalog.product_variants v
			LEFT JOIN inventory.stocks s ON s.product_variant_id = v.id
			WHERE %s
			GROUP BY v.id
			ORDER BY v.id DESC
			LIMIT $%d OFFSET $%d;
		`, whereSQL, argIdx, argIdx+1)

		args = append(args, limit, offset)
		rows, err := tx.Query(txCtx, dataQuery, args...)
		if err != nil {
			return fmt.Errorf("list all variants: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var v catalog.ProductVariant
			var statusStr string
			if err := rows.Scan(
				&v.ID, &v.PublicID, &v.OrganizationID, &v.ProductID, &v.Name, &v.SKU,
				&v.Barcode, &v.Price, &v.CostPrice, &v.Discount, &v.Unit, &v.Image,
				&statusStr, &v.IsFeatured, &v.BatchNumber, &v.ExpiryDate, &v.MinOrderQty,
				&v.BranchID, &v.StockQty, &v.CreatedAt, &v.UpdatedAt, &v.DeletedAt,
			); err != nil {
				return err
			}
			v.Status = catalog.ProductStatus(statusStr)
			variants = append(variants, &v)
		}
		return rows.Err()
	})

	if err != nil {
		return nil, 0, err
	}
	return variants, total, nil
}

// UpdateVariant updates a variant.
func (r *Repository) UpdateVariant(ctx context.Context, v *catalog.ProductVariant) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			UPDATE catalog.product_variants
			SET name = $2, sku = $3, barcode = $4, price = $5, cost_price = $6,
			    discount = $7, unit = $8, image = $9, status = $10,
			    is_featured = $11, batch_number = $12, expiry_date = $13,
			    min_order_qty = $14, branch_id = $15, updated_at = now()
			WHERE id = $1 AND deleted_at IS NULL;
		`
		minQty := v.MinOrderQty
		if minQty <= 0 {
			minQty = 1
		}
		res, err := tx.Exec(txCtx, query,
			v.ID, v.Name, v.SKU, v.Barcode, v.Price, v.CostPrice, v.Discount,
			v.Unit, v.Image, string(v.Status), v.IsFeatured, v.BatchNumber,
			v.ExpiryDate, minQty, v.BranchID,
		)
		if err != nil {
			return fmt.Errorf("catalog postgres: update variant: %w", err)
		}
		if res.RowsAffected() == 0 {
			return apperr.NotFound("product_variant")
		}
		return nil
	})
}

// DeleteVariant soft-deletes a product variant.
func (r *Repository) DeleteVariant(ctx context.Context, id int64) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `UPDATE catalog.product_variants SET deleted_at = now() WHERE id = $1 AND deleted_at IS NULL;`
		res, err := tx.Exec(txCtx, query, id)
		if err != nil {
			return fmt.Errorf("catalog postgres: delete variant: %w", err)
		}
		if res.RowsAffected() == 0 {
			return apperr.NotFound("product_variant")
		}
		return nil
	})
}
