package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// Repository implements catalog.Repository using PostgreSQL.
type Repository struct {
	db *database.DB
}

// NewRepository creates a PostgreSQL catalog repository.
func NewRepository(db *database.DB) *Repository {
	return &Repository{db: db}
}

// CreateProduct inserts a new product for the active organization.
func (r *Repository) CreateProduct(ctx context.Context, p *catalog.Product) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		if p.OrganizationID <= 0 {
			var firstOrgID int64
			err := tx.QueryRow(txCtx, `SELECT id FROM org.organizations WHERE status = 'approved' OR type = 'vendor' ORDER BY id ASC LIMIT 1`).Scan(&firstOrgID)
			if err != nil || firstOrgID <= 0 {
				_ = tx.QueryRow(txCtx, `SELECT id FROM org.organizations ORDER BY id ASC LIMIT 1`).Scan(&firstOrgID)
			}
			if firstOrgID > 0 {
				p.OrganizationID = firstOrgID
			} else {
				err = tx.QueryRow(txCtx, `
					INSERT INTO org.organizations (name, legal_name, trade_name, type, status)
					VALUES ('{"ar":"دواء 24 - الكتالوج المعتمد","en":"Dawa24 Master Catalog"}'::jsonb, '{"ar":"دواء 24 - الكتالوج المعتمد","en":"Dawa24 Master Catalog"}'::jsonb, '{"ar":"دواء 24","en":"Dawa24"}'::jsonb, 'vendor', 'approved')
					RETURNING id
				`).Scan(&firstOrgID)
				if err == nil {
					p.OrganizationID = firstOrgID
				}
			}
		}

		query := `
			INSERT INTO catalog.products (
				organization_id, category_id, brand_id, branch_id, name, description,
				sku, barcode, price, discount, old_price, image, image_link, status,
				is_featured, dosage_form, scientific_name, pharmacology, active,
				concentration, unit, manufacturing_companies, institutional_work_ids
			) VALUES (
				$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23
			) RETURNING id, public_id, created_at, updated_at;
		`
		err := tx.QueryRow(txCtx, query,
			p.OrganizationID, p.CategoryID, p.BrandID, p.BranchID, p.Name, p.Description,
			p.SKU, p.Barcode, p.Price, p.Discount, p.OldPrice, p.Image, p.ImageLink,
			string(p.Status), p.IsFeatured, p.DosageForm, p.ScientificName,
			p.Pharmacology, p.Active, p.Concentration, p.Unit, p.ManufacturingCompanies,
			p.InstitutionalWorkIDs,
		).Scan(&p.ID, &p.PublicID, &p.CreatedAt, &p.UpdatedAt)

		if err != nil {
			return fmt.Errorf("catalog postgres: create product: %w", err)
		}
		return nil
	})
}

// GetProductByID retrieves a product by its primary key.
func (r *Repository) GetProductByID(ctx context.Context, id int64) (*catalog.Product, error) {
	var p catalog.Product
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, public_id, organization_id, category_id, brand_id, branch_id,
			       name, description, sku, barcode, price, discount, old_price, image,
			       image_link, status, sold_times, is_featured, dosage_form,
			       scientific_name, pharmacology, active, concentration, unit,
			       manufacturing_companies, COALESCE(institutional_work_ids, '{}'::bigint[]),
			       created_at, updated_at, deleted_at
			FROM catalog.products
			WHERE id = $1 AND deleted_at IS NULL;
		`
		var statusStr string
		err := tx.QueryRow(txCtx, query, id).Scan(
			&p.ID, &p.PublicID, &p.OrganizationID, &p.CategoryID, &p.BrandID, &p.BranchID,
			&p.Name, &p.Description, &p.SKU, &p.Barcode, &p.Price, &p.Discount,
			&p.OldPrice, &p.Image, &p.ImageLink, &statusStr, &p.SoldTimes, &p.IsFeatured,
			&p.DosageForm, &p.ScientificName, &p.Pharmacology, &p.Active,
			&p.Concentration, &p.Unit, &p.ManufacturingCompanies, &p.InstitutionalWorkIDs,
			&p.CreatedAt, &p.UpdatedAt, &p.DeletedAt,
		)
		if err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("product")
			}
			return fmt.Errorf("catalog postgres: get product: %w", err)
		}
		p.Status = catalog.ProductStatus(statusStr)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// UpdateProduct updates product attributes.
func (r *Repository) UpdateProduct(ctx context.Context, p *catalog.Product) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			UPDATE catalog.products
			SET category_id = $2, brand_id = $3, branch_id = $4, name = $5,
			    description = $6, sku = $7, barcode = $8, price = $9, discount = $10,
			    old_price = $11, image = $12, image_link = $13, status = $14,
			    is_featured = $15, dosage_form = $16, scientific_name = $17,
			    pharmacology = $18, active = $19, concentration = $20, unit = $21,
			    manufacturing_companies = $22, institutional_work_ids = $23, updated_at = now()
			WHERE id = $1 AND deleted_at IS NULL;
		`
		res, err := tx.Exec(txCtx, query,
			p.ID, p.CategoryID, p.BrandID, p.BranchID, p.Name, p.Description,
			p.SKU, p.Barcode, p.Price, p.Discount, p.OldPrice, p.Image, p.ImageLink,
			string(p.Status), p.IsFeatured, p.DosageForm, p.ScientificName,
			p.Pharmacology, p.Active, p.Concentration, p.Unit, p.ManufacturingCompanies,
			p.InstitutionalWorkIDs,
		)
		if err != nil {
			return fmt.Errorf("catalog postgres: update product: %w", err)
		}
		if res.RowsAffected() == 0 {
			return apperr.NotFound("product")
		}
		return nil
	})
}

// DeleteProduct soft-deletes a product, its variants, and associated warehouse stocks.
func (r *Repository) DeleteProduct(ctx context.Context, id int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `UPDATE catalog.products SET deleted_at = now() WHERE id = $1 AND deleted_at IS NULL;`
		res, err := tx.Exec(txCtx, query, id)
		if err != nil {
			return fmt.Errorf("catalog postgres: delete product: %w", err)
		}
		if res.RowsAffected() == 0 {
			return apperr.NotFound("product")
		}

		// Cascade soft-delete to child variants and stocks
		_, _ = tx.Exec(txCtx, `UPDATE catalog.product_variants SET deleted_at = now() WHERE product_id = $1 AND deleted_at IS NULL;`, id)
		_, _ = tx.Exec(txCtx, `UPDATE inventory.stocks SET deleted_at = now() WHERE product_id = $1 AND deleted_at IS NULL;`, id)

		return nil
	})
}

// GetProductBySKU retrieves a master product by its exact SKU (or barcode).
func (r *Repository) GetProductBySKU(ctx context.Context, sku string) (*catalog.Product, error) {
	var p catalog.Product
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, public_id, organization_id, category_id, brand_id, branch_id,
			       name, description, sku, barcode, price, discount, old_price, image,
			       image_link, status, sold_times, is_featured, dosage_form,
			       scientific_name, pharmacology, active, concentration, unit,
			       manufacturing_companies, COALESCE(institutional_work_ids, '{}'::bigint[]),
			       created_at, updated_at, deleted_at
			FROM catalog.products
			WHERE (sku = $1 OR barcode = $1) AND deleted_at IS NULL
			ORDER BY id ASC LIMIT 1;
		`
		var statusStr string
		err := tx.QueryRow(txCtx, query, sku).Scan(
			&p.ID, &p.PublicID, &p.OrganizationID, &p.CategoryID, &p.BrandID, &p.BranchID,
			&p.Name, &p.Description, &p.SKU, &p.Barcode, &p.Price, &p.Discount,
			&p.OldPrice, &p.Image, &p.ImageLink, &statusStr, &p.SoldTimes, &p.IsFeatured,
			&p.DosageForm, &p.ScientificName, &p.Pharmacology, &p.Active,
			&p.Concentration, &p.Unit, &p.ManufacturingCompanies, &p.InstitutionalWorkIDs,
			&p.CreatedAt, &p.UpdatedAt, &p.DeletedAt,
		)
		if err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("product")
			}
			return fmt.Errorf("catalog postgres: get product by sku: %w", err)
		}
		p.Status = catalog.ProductStatus(statusStr)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// UpdateProductImageBySKU sets the downloaded image path and source URL for a product matching the SKU.
func (r *Repository) UpdateProductImageBySKU(ctx context.Context, sku string, imagePath string, imageLink string) (*catalog.Product, error) {
	prod, err := r.GetProductBySKU(ctx, sku)
	if err != nil {
		return nil, err
	}
	prod.Image = imagePath
	if imageLink != "" {
		prod.ImageLink = imageLink
	}
	if err := r.UpdateProduct(ctx, prod); err != nil {
		return nil, err
	}
	return prod, nil
}

// SearchProducts performs fuzzy Arabic search and filters including institutional works.
func (r *Repository) SearchProducts(ctx context.Context, params catalog.SearchParams) ([]*catalog.Product, error) {
	var products []*catalog.Product
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, public_id, organization_id, category_id, brand_id, branch_id,
			       name, description, sku, barcode, price, discount, old_price, image,
			       image_link, status, sold_times, is_featured, dosage_form,
			       scientific_name, pharmacology, active, concentration, unit,
			       manufacturing_companies, COALESCE(institutional_work_ids, '{}'::bigint[]),
			       created_at, updated_at, deleted_at
			FROM catalog.products
			WHERE deleted_at IS NULL
			  AND ($1 = '' 
			       OR platform.normalize_arabic(name->>'ar') ILIKE '%' || platform.normalize_arabic($1) || '%'
			       OR name->>'en' ILIKE '%' || $1 || '%'
			       OR sku ILIKE '%' || $1 || '%'
			       OR barcode ILIKE '%' || $1 || '%'
			       OR COALESCE(scientific_name, '') ILIKE '%' || $1 || '%'
			       OR COALESCE(active, '') ILIKE '%' || $1 || '%'
			       OR COALESCE(manufacturing_companies, '') ILIKE '%' || $1 || '%'
		       OR word_similarity(platform.normalize_arabic($1), platform.normalize_arabic(name->>'ar')) >= 0.25
		       OR similarity(platform.normalize_arabic(name->>'ar'), platform.normalize_arabic($1)) >= 0.15
		       OR regexp_replace(platform.normalize_arabic(name->>'ar'), '[اوي]', '', 'g') ILIKE '%' || regexp_replace(platform.normalize_arabic($1), '[اوي]', '', 'g') || '%'
		       OR ($13 <> '' AND platform.normalize_arabic(name->>'ar') ILIKE '%' || platform.normalize_arabic($13) || '%')
		       OR ($13 <> '' AND name->>'en' ILIKE '%' || $13 || '%'))
		  AND ($2::bigint IS NULL OR category_id = $2)
		  AND ($3::bigint IS NULL OR brand_id = $3)
		  AND ($6::numeric IS NULL OR price >= $6)
		  AND ($7::numeric IS NULL OR price <= $7)
		  AND ($10::text = '' OR status = $10)
		  AND ($11::text = '' OR dosage_form ILIKE '%' || $11 || '%')
			  AND (
			      ($8::int = 0 AND ($9::bigint[] IS NULL OR cardinality($9::bigint[]) = 0 OR cardinality(institutional_work_ids) = 0 OR institutional_work_ids && $9))
			      OR
			      ($8::int = 1 AND ($9::bigint[] IS NOT NULL AND cardinality($9::bigint[]) > 0 AND institutional_work_ids && $9))
			  )
			  AND ($12::boolean = false OR EXISTS (
			      SELECT 1 FROM catalog.product_variants pv
			      JOIN inventory.stocks st ON st.product_variant_id = pv.id AND st.deleted_at IS NULL
			      WHERE pv.product_id = catalog.products.id
			        AND pv.deleted_at IS NULL
			        AND st.quantity > 0
			  ))
		ORDER BY
		  CASE
		    WHEN $1 = '' THEN 0
		    WHEN platform.normalize_arabic(name->>'ar') ILIKE platform.normalize_arabic($1) || '%' THEN 1
		    WHEN name->>'en' ILIKE $1 || '%' THEN 2
		    WHEN $13 <> '' AND platform.normalize_arabic(name->>'ar') ILIKE platform.normalize_arabic($13) || '%' THEN 3
		    WHEN $13 <> '' AND name->>'en' ILIKE $13 || '%' THEN 4
		    WHEN platform.normalize_arabic(name->>'ar') ILIKE '%' || platform.normalize_arabic($1) || '%' THEN 5
		    WHEN name->>'en' ILIKE '%' || $1 || '%' THEN 6
		    WHEN $13 <> '' AND platform.normalize_arabic(name->>'ar') ILIKE '% ' || platform.normalize_arabic($13) || '%' THEN 7
		    ELSE 8
		  END,
			  ` + catalogOrderBy(params.Sort) + `
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
			params.Status, params.DosageForm, params.InStock, params.FirstWord,
		)
		if err != nil {
			return fmt.Errorf("catalog postgres: search products: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var p catalog.Product
			var statusStr string
			if err := rows.Scan(
				&p.ID, &p.PublicID, &p.OrganizationID, &p.CategoryID, &p.BrandID, &p.BranchID,
				&p.Name, &p.Description, &p.SKU, &p.Barcode, &p.Price, &p.Discount,
				&p.OldPrice, &p.Image, &p.ImageLink, &statusStr, &p.SoldTimes, &p.IsFeatured,
				&p.DosageForm, &p.ScientificName, &p.Pharmacology, &p.Active,
				&p.Concentration, &p.Unit, &p.ManufacturingCompanies, &p.InstitutionalWorkIDs,
				&p.CreatedAt, &p.UpdatedAt, &p.DeletedAt,
			); err != nil {
				return err
			}
			p.Status = catalog.ProductStatus(statusStr)
			products = append(products, &p)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return products, nil
}

// CountProducts returns the total count of products matching search filters.
func (r *Repository) CountProducts(ctx context.Context, params catalog.SearchParams) (int, error) {
	var total int
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT COUNT(*)
			FROM catalog.products
			WHERE deleted_at IS NULL
			  AND ($1 = '' 
			       OR platform.normalize_arabic(name->>'ar') ILIKE '%' || platform.normalize_arabic($1) || '%'
			       OR name->>'en' ILIKE '%' || $1 || '%'
			       OR sku ILIKE '%' || $1 || '%'
			       OR barcode ILIKE '%' || $1 || '%'
			       OR COALESCE(scientific_name, '') ILIKE '%' || $1 || '%'
			       OR COALESCE(active, '') ILIKE '%' || $1 || '%'
			       OR COALESCE(manufacturing_companies, '') ILIKE '%' || $1 || '%'
		       OR word_similarity(platform.normalize_arabic($1), platform.normalize_arabic(name->>'ar')) >= 0.25
		       OR similarity(platform.normalize_arabic(name->>'ar'), platform.normalize_arabic($1)) >= 0.15
		       OR regexp_replace(platform.normalize_arabic(name->>'ar'), '[اوي]', '', 'g') ILIKE '%' || regexp_replace(platform.normalize_arabic($1), '[اوي]', '', 'g') || '%'
		       OR ($11 <> '' AND platform.normalize_arabic(name->>'ar') ILIKE '%' || platform.normalize_arabic($11) || '%')
		       OR ($11 <> '' AND name->>'en' ILIKE '%' || $11 || '%'))
		  AND ($2::bigint IS NULL OR category_id = $2)
		  AND ($3::bigint IS NULL OR brand_id = $3)
		  AND ($4::numeric IS NULL OR price >= $4)
		  AND ($5::numeric IS NULL OR price <= $5)
		  AND ($6::text = '' OR status = $6)
		  AND ($7::text = '' OR dosage_form ILIKE '%' || $7 || '%')
			  AND (
			      ($8::int = 0 AND ($9::bigint[] IS NULL OR cardinality($9::bigint[]) = 0 OR cardinality(institutional_work_ids) = 0 OR institutional_work_ids && $9))
			      OR
			      ($8::int = 1 AND ($9::bigint[] IS NOT NULL AND cardinality($9::bigint[]) > 0 AND institutional_work_ids && $9))
			  )
			  AND ($10::boolean = false OR EXISTS (
			      SELECT 1 FROM catalog.product_variants pv
			      JOIN inventory.stocks st ON st.product_variant_id = pv.id AND st.deleted_at IS NULL
			      WHERE pv.product_id = catalog.products.id
			        AND pv.deleted_at IS NULL
			        AND st.quantity > 0
			  ));
		`
		return tx.QueryRow(txCtx, query,
			params.Query, params.CategoryID, params.BrandID,
			params.MinPrice, params.MaxPrice, params.Status, params.DosageForm,
			params.FilterMode, params.AllowedWorkIDs, params.InStock, params.FirstWord,
		).Scan(&total)
	})
	return total, err
}

// catalogOrderBy maps a whitelisted sort key onto a safe ORDER BY clause.
func catalogOrderBy(sort string) string {
	// Returns the ORDER BY *expression* only, without the keyword - the caller
	// supplies "ORDER BY". Pasted in without it, the SQL read
	// "... price <= $7 sold_times DESC" and every product search failed with a
	// syntax error at "sold_times".
	switch sort {
	case "price_asc":
		return "price ASC, created_at DESC"
	case "price_desc":
		return "price DESC, created_at DESC"
	case "newest":
		return "created_at DESC"
	case "name":
		return "name->>'ar' ASC"
	default:
		return "sold_times DESC, created_at DESC"
	}
}

// CountProductsByOrg returns how many products an organization has in a status.
//
// The supplier dashboard previously derived this by iterating a page capped at
// 100 rows, so a supplier with more products than that saw "100" and no way to
// tell it was a ceiling rather than a count.
func (r *Repository) CountProductsByOrg(ctx context.Context, orgID int64, status string) (int, error) {
	var total int
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		const query = `
			SELECT COUNT(*) FROM catalog.products
			WHERE organization_id = $1
			  AND deleted_at IS NULL
			  AND ($2::text = '' OR status = $2);
		`
		return tx.QueryRow(txCtx, query, orgID, status).Scan(&total)
	})
	return total, err
}
