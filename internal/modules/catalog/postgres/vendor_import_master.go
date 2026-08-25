package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
)

// Shared-catalogue reads and writes for the vendor import.

// ListMatchProducts loads the shared catalogue as a matching projection.
//
// One query, no joins, eleven columns. Thirty thousand rows come back in about
// a second and a half over a remote connection, which is why the index is built
// once per import and not once per row.
func (r *Repository) ListMatchProducts(ctx context.Context) ([]catalog.MatchProduct, error) {
	var out []catalog.MatchProduct
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(txCtx, `
			SELECT id,
			       COALESCE(name->>'ar', ''), COALESCE(name->>'en', ''),
			       sku, barcode, scientific_name, dosage_form, concentration,
			       unit, manufacturing_companies, price::text
			FROM catalog.products
			WHERE deleted_at IS NULL AND status <> 'rejected'
			ORDER BY id`)
		if err != nil {
			return fmt.Errorf("catalog postgres: list match products: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var p catalog.MatchProduct
			if err := rows.Scan(&p.ID, &p.NameAR, &p.NameEN, &p.SKU, &p.Barcode,
				&p.Scientific, &p.DosageForm, &p.Concentration, &p.Unit,
				&p.Manufacturer, &p.PublicPrice); err != nil {
				return fmt.Errorf("catalog postgres: scan match product: %w", err)
			}
			out = append(out, p)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// CreateImportProducts registers the products a vendor's file carried that the
// shared catalogue does not have, returning their ids in the order given.
//
// One batch, one transaction, and a failure isolates to its own row for the
// same reason the variant writer's does: one unparseable product must not cost
// a vendor the other eight thousand.
func (r *Repository) CreateImportProducts(
	ctx context.Context, orgID int64, prods []*catalog.Product,
) ([]int64, error) {
	if len(prods) == 0 {
		return nil, nil
	}
	ids := make([]int64, len(prods))
	err := r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		batch := &pgx.Batch{}
		for _, p := range prods {
			batch.Queue(`
				INSERT INTO catalog.products (
					organization_id, name, sku, barcode, price, old_price, discount,
					status, dosage_form, scientific_name, active, concentration, unit,
					manufacturing_companies, institutional_work_ids
				) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, '', $11, $12, $13, '{}')
				RETURNING id`,
				orgID, p.Name, p.SKU, p.Barcode, p.Price, p.OldPrice, p.Discount,
				string(p.Status), p.DosageForm, p.ScientificName, p.Concentration,
				p.Unit, p.ManufacturingCompanies)
		}

		br := tx.SendBatch(txCtx, batch)
		var rejected error
		for i := range prods {
			var id int64
			if err := br.QueryRow().Scan(&id); err != nil && rejected == nil {
				rejected = err
			}
			ids[i] = id
		}
		if closeErr := br.Close(); closeErr != nil && rejected == nil {
			rejected = closeErr
		}
		if rejected != nil {
			return errRowRejected
		}
		for i, p := range prods {
			p.ID = ids[i]
		}
		return nil
	})
	if err == nil {
		return ids, nil
	}
	if err != errRowRejected {
		return nil, fmt.Errorf("catalog postgres: create import products: %w", err)
	}
	return r.createImportProductsOneByOne(ctx, orgID, prods)
}

// createImportProductsOneByOne is the isolation path. A product that cannot be
// written leaves a zero in its slot, and the caller reports that row rather
// than linking a variant to nothing.
func (r *Repository) createImportProductsOneByOne(
	ctx context.Context, orgID int64, prods []*catalog.Product,
) ([]int64, error) {
	ids := make([]int64, len(prods))
	for i, p := range prods {
		var id int64
		err := r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
			return tx.QueryRow(txCtx, `
				INSERT INTO catalog.products (
					organization_id, name, sku, barcode, price, old_price, discount,
					status, dosage_form, scientific_name, active, concentration, unit,
					manufacturing_companies, institutional_work_ids
				) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, '', $11, $12, $13, '{}')
				RETURNING id`,
				orgID, p.Name, p.SKU, p.Barcode, p.Price, p.OldPrice, p.Discount,
				string(p.Status), p.DosageForm, p.ScientificName, p.Concentration,
				p.Unit, p.ManufacturingCompanies).Scan(&id)
		})
		if err != nil {
			continue
		}
		ids[i] = id
		p.ID = id
	}
	return ids, nil
}

// DeactivateVariantsExcept takes an organisation's variants off sale except the
// ones listed, and returns how many were affected.
func (r *Repository) DeactivateVariantsExcept(
	ctx context.Context, orgID int64, keep []int64,
) (int64, error) {
	if keep == nil {
		keep = []int64{}
	}
	var affected int64
	err := r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(txCtx, `
			UPDATE catalog.product_variants
			SET status = 'inactive', updated_at = now()
			WHERE organization_id = $1 AND deleted_at IS NULL
			  AND status = 'active' AND NOT (id = ANY($2))`, orgID, keep)
		if err != nil {
			return fmt.Errorf("catalog postgres: deactivate variants: %w", err)
		}
		affected = tag.RowsAffected()
		return nil
	})
	return affected, err
}
