package postgres

import (
	"context"
	"fmt"

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
				discount, unit, image, status, is_featured
			) VALUES (
				$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12
			) RETURNING id, public_id, created_at, updated_at;
		`
		err := tx.QueryRow(txCtx, query,
			v.OrganizationID, v.ProductID, v.Name, v.SKU, v.Barcode, v.Price,
			v.CostPrice, v.Discount, v.Unit, v.Image, string(v.Status), v.IsFeatured,
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
			       created_at, updated_at, deleted_at
			FROM catalog.product_variants
			WHERE id = $1 AND deleted_at IS NULL;
		`
		var statusStr string
		err := tx.QueryRow(txCtx, query, id).Scan(
			&v.ID, &v.PublicID, &v.OrganizationID, &v.ProductID, &v.Name, &v.SKU,
			&v.Barcode, &v.Price, &v.CostPrice, &v.Discount, &v.Unit, &v.Image,
			&statusStr, &v.IsFeatured, &v.CreatedAt, &v.UpdatedAt, &v.DeletedAt,
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
				&statusStr, &v.IsFeatured, &v.CreatedAt, &v.UpdatedAt, &v.DeletedAt,
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

// UpdateVariant updates a variant.
func (r *Repository) UpdateVariant(ctx context.Context, v *catalog.ProductVariant) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			UPDATE catalog.product_variants
			SET name = $2, sku = $3, barcode = $4, price = $5, cost_price = $6,
			    discount = $7, unit = $8, image = $9, status = $10,
			    is_featured = $11, updated_at = now()
			WHERE id = $1 AND deleted_at IS NULL;
		`
		res, err := tx.Exec(txCtx, query,
			v.ID, v.Name, v.SKU, v.Barcode, v.Price, v.CostPrice, v.Discount,
			v.Unit, v.Image, string(v.Status), v.IsFeatured,
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
