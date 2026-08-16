package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// CreateCategory adds a category.
func (r *Repository) CreateCategory(ctx context.Context, c *catalog.Category) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			INSERT INTO catalog.categories (
				parent_id, name, description, icon, image, status, sort_order
			) VALUES (
				$1, $2, $3, $4, $5, $6, $7
			) RETURNING id, public_id, created_at, updated_at;
		`
		return tx.QueryRow(txCtx, query,
			c.ParentID, c.Name, c.Description, c.Icon, c.Image, c.Status, c.SortOrder,
		).Scan(&c.ID, &c.PublicID, &c.CreatedAt, &c.UpdatedAt)
	})
}

// ListCategories returns all categories.
func (r *Repository) ListCategories(ctx context.Context) ([]*catalog.Category, error) {
	var categories []*catalog.Category
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, public_id, parent_id, name, description, icon, image, status,
			       sort_order, created_at, updated_at, deleted_at
			FROM catalog.categories
			WHERE deleted_at IS NULL
			ORDER BY sort_order ASC, name->>'ar' ASC;
		`
		rows, err := tx.Query(txCtx, query)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var c catalog.Category
			if err := rows.Scan(
				&c.ID, &c.PublicID, &c.ParentID, &c.Name, &c.Description, &c.Icon,
				&c.Image, &c.Status, &c.SortOrder, &c.CreatedAt, &c.UpdatedAt, &c.DeletedAt,
			); err != nil {
				return err
			}
			categories = append(categories, &c)
		}
		return rows.Err()
	})
	return categories, err
}

// CreateBrand adds a brand.
func (r *Repository) CreateBrand(ctx context.Context, b *catalog.Brand) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			INSERT INTO catalog.brands (name, description, image, status)
			VALUES ($1, $2, $3, $4)
			RETURNING id, public_id, created_at, updated_at;
		`
		return tx.QueryRow(txCtx, query, b.Name, b.Description, b.Image, b.Status).Scan(
			&b.ID, &b.PublicID, &b.CreatedAt, &b.UpdatedAt,
		)
	})
}

// ListBrands returns all brands.
func (r *Repository) ListBrands(ctx context.Context) ([]*catalog.Brand, error) {
	var brands []*catalog.Brand
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, public_id, name, description, image, status, created_at, updated_at, deleted_at
			FROM catalog.brands
			WHERE deleted_at IS NULL
			ORDER BY name->>'ar' ASC;
		`
		rows, err := tx.Query(txCtx, query)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var b catalog.Brand
			if err := rows.Scan(
				&b.ID, &b.PublicID, &b.Name, &b.Description, &b.Image, &b.Status,
				&b.CreatedAt, &b.UpdatedAt, &b.DeletedAt,
			); err != nil {
				return err
			}
			brands = append(brands, &b)
		}
		return rows.Err()
	})
	return brands, err
}
