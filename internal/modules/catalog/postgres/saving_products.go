package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// CreateSavingProduct inserts a saving product record.
func (r *Repository) CreateSavingProduct(ctx context.Context, sp *catalog.SavingProduct) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			INSERT INTO catalog.saving_products (
				user_id, organization_id, product_id, name_product, sku, qty, price, created_at, updated_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, now(), now())
			RETURNING id, public_id, created_at, updated_at;
		`
		return tx.QueryRow(
			txCtx, query,
			sp.UserID, sp.OrganizationID, sp.ProductID, sp.NameProduct, sp.SKU, sp.Quantity, sp.Price,
		).Scan(&sp.ID, &sp.PublicID, &sp.CreatedAt, &sp.UpdatedAt)
	})
}

// ListSavingProductsByOrg returns saving products for an organization.
func (r *Repository) ListSavingProductsByOrg(ctx context.Context, orgID int64, limit, offset int) ([]*catalog.SavingProduct, error) {
	var list []*catalog.SavingProduct
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, public_id, user_id, organization_id, product_id, name_product,
			       COALESCE(sku, ''), qty, price, created_at, updated_at
			FROM catalog.saving_products
			WHERE organization_id = $1 AND deleted_at IS NULL
			ORDER BY id DESC
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
			var sp catalog.SavingProduct
			if err := rows.Scan(
				&sp.ID, &sp.PublicID, &sp.UserID, &sp.OrganizationID, &sp.ProductID,
				&sp.NameProduct, &sp.SKU, &sp.Quantity, &sp.Price, &sp.CreatedAt, &sp.UpdatedAt,
			); err != nil {
				return err
			}
			list = append(list, &sp)
		}
		return rows.Err()
	})
	return list, err
}

// GetSavingProductByID retrieves a saving product by ID.
func (r *Repository) GetSavingProductByID(ctx context.Context, id int64) (*catalog.SavingProduct, error) {
	var sp catalog.SavingProduct
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, public_id, user_id, organization_id, product_id, name_product,
			       COALESCE(sku, ''), qty, price, created_at, updated_at
			FROM catalog.saving_products
			WHERE id = $1 AND deleted_at IS NULL;
		`
		err := tx.QueryRow(txCtx, query, id).Scan(
			&sp.ID, &sp.PublicID, &sp.UserID, &sp.OrganizationID, &sp.ProductID,
			&sp.NameProduct, &sp.SKU, &sp.Quantity, &sp.Price, &sp.CreatedAt, &sp.UpdatedAt,
		)
		if err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("saving_product")
			}
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &sp, nil
}

// DeleteSavingProduct soft-deletes a saving product record.
func (r *Repository) DeleteSavingProduct(ctx context.Context, id, orgID int64) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `UPDATE catalog.saving_products SET deleted_at = now() WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL;`
		tag, err := tx.Exec(txCtx, query, id, orgID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("saving_product")
		}
		return nil
	})
}
