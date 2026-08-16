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

// GetCategoryByID retrieves a category by ID.
func (r *Repository) GetCategoryByID(ctx context.Context, id int64) (*catalog.Category, error) {
	var c catalog.Category
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `SELECT id, public_id, parent_id, name, description, icon, image, status, sort_order, created_at, updated_at, deleted_at FROM catalog.categories WHERE id = $1 AND deleted_at IS NULL;`
		return tx.QueryRow(txCtx, query, id).Scan(&c.ID, &c.PublicID, &c.ParentID, &c.Name, &c.Description, &c.Icon, &c.Image, &c.Status, &c.SortOrder, &c.CreatedAt, &c.UpdatedAt, &c.DeletedAt)
	})
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// UpdateCategory updates category details.
func (r *Repository) UpdateCategory(ctx context.Context, c *catalog.Category) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `UPDATE catalog.categories SET parent_id = $1, name = $2, description = $3, icon = $4, image = $5, status = $6, sort_order = $7, updated_at = now() WHERE id = $8;`
		_, err := tx.Exec(txCtx, query, c.ParentID, c.Name, c.Description, c.Icon, c.Image, c.Status, c.SortOrder, c.ID)
		return err
	})
}

// GetBrandByID retrieves a brand by ID.
func (r *Repository) GetBrandByID(ctx context.Context, id int64) (*catalog.Brand, error) {
	var b catalog.Brand
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `SELECT id, public_id, name, description, image, status, created_at, updated_at, deleted_at FROM catalog.brands WHERE id = $1 AND deleted_at IS NULL;`
		return tx.QueryRow(txCtx, query, id).Scan(&b.ID, &b.PublicID, &b.Name, &b.Description, &b.Image, &b.Status, &b.CreatedAt, &b.UpdatedAt, &b.DeletedAt)
	})
	if err != nil {
		return nil, err
	}
	return &b, nil
}

// UpdateBrand updates brand details.
func (r *Repository) UpdateBrand(ctx context.Context, b *catalog.Brand) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `UPDATE catalog.brands SET name = $1, description = $2, image = $3, status = $4, updated_at = now() WHERE id = $5;`
		_, err := tx.Exec(txCtx, query, b.Name, b.Description, b.Image, b.Status, b.ID)
		return err
	})
}
