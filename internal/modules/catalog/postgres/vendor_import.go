package postgres

import (
	"context"
	"errors"
	"fmt"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// Bulk variant persistence for the vendor catalogue import.
//
// Both operations run inside one transaction per call, so a batch either lands
// whole or not at all, and a failure on one row is reported against that row
// rather than taking the batch down with it.

// ListVariantKeys loads every live variant of one organisation, reduced to the
// fields the importer matches on.
//
// One query for a vendor's whole catalogue is deliberate. The alternative — a
// lookup per spreadsheet row — is what made the previous importer take minutes
// on a file that this reads in seconds, and it issued those lookups against
// three different keys, so a nine-thousand-row file cost twenty-seven thousand
// round trips before it wrote anything.
func (r *Repository) ListVariantKeys(ctx context.Context, orgID int64) ([]catalog.VariantKey, error) {
	var out []catalog.VariantKey
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(txCtx, `
			SELECT id, COALESCE(product_id, 0), sku, barcode, unit, batch_number, branch_id
			FROM catalog.product_variants
			WHERE organization_id = $1 AND deleted_at IS NULL
			ORDER BY id`, orgID)
		if err != nil {
			return fmt.Errorf("catalog postgres: list variant keys: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var k catalog.VariantKey
			if err := rows.Scan(&k.ID, &k.ProductID, &k.SKU, &k.Barcode,
				&k.Unit, &k.BatchNumber, &k.BranchID); err != nil {
				return fmt.Errorf("catalog postgres: scan variant key: %w", err)
			}
			out = append(out, k)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

const insertVariantSQL = `
	INSERT INTO catalog.product_variants (
		organization_id, product_id, name, sku, barcode, price, cost_price,
		discount, unit, image, status, is_featured, is_negotiable, batch_number,
		expiry_date, min_order_qty, branch_id
	) VALUES (
		$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16,
		COALESCE($17, (SELECT b.id FROM org.branches b WHERE b.organization_id = $1 AND b.deleted_at IS NULL ORDER BY b.is_main DESC, b.id ASC LIMIT 1))
	)
	RETURNING id`

// updateVariantSQL refreshes an existing variant.
//
// Every identity column is guarded: a price-list re-upload that carries names
// and prices but no barcode, batch or expiry column must not wipe the very
// identifiers future imports match on. The previous version wrote sku, barcode,
// name, batch and expiry unconditionally, so one routine file silently blanked
// them across a vendor's whole catalogue.
//
// Status is deliberately not written at all. It is derived from the import's
// "publish immediately" switch, which documents what new rows get; applying it
// to updates meant an unticked box on a routine refresh delisted every matched
// variant.
const updateVariantSQL = `
	UPDATE catalog.product_variants
	SET name = COALESCE(NULLIF($3, ''), name),
	    sku = COALESCE(NULLIF($4, ''), sku),
	    barcode = COALESCE(NULLIF($5, ''), barcode),
	    price = $6, cost_price = $7,
	    discount = $8, unit = COALESCE(NULLIF($9, ''), unit),
	    image = COALESCE(NULLIF($10, ''), image),
	    is_negotiable = $11,
	    batch_number = COALESCE(NULLIF($12, ''), batch_number),
	    expiry_date = COALESCE($13, expiry_date),
	    min_order_qty = $14,
	    branch_id = COALESCE($15, branch_id, (SELECT b.id FROM org.branches b WHERE b.organization_id = $2 AND b.deleted_at IS NULL ORDER BY b.is_main DESC, b.id ASC LIMIT 1)),
	    product_id = COALESCE($16, product_id),
	    updated_at = now()
	WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL
	RETURNING id`

// errRowRejected marks a batch in which at least one statement was refused. It
// never reaches a caller; it rolls the batch back so the rows can be retried
// one at a time.
var errRowRejected = errors.New("catalog postgres: a row in the batch was rejected")

// BulkWriteVariants inserts and updates a batch of variants.
//
// The happy path is one pipelined round trip for the whole batch. Postgres
// aborts a transaction at the first failed statement, though, so a single bad
// row would otherwise take five hundred good ones with it — and "one row in
// this file has a duplicate code" must not mean "none of your catalogue
// imported". When the batch is refused, it is rolled back and rewritten row by
// row, each in its own transaction, so the cost of isolating the bad row falls
// only on the file that has one.
func (r *Repository) BulkWriteVariants(
	ctx context.Context, orgID int64, rows []catalog.VariantWriteRow,
) (catalog.VariantWriteResult, error) {
	result, err := r.batchVariants(ctx, orgID, rows)
	if err == nil {
		return result, nil
	}
	if !errors.Is(err, errRowRejected) {
		return result, fmt.Errorf("catalog postgres: bulk write variants: %w", err)
	}
	return r.writeVariantsOneByOne(ctx, orgID, rows)
}

// batchVariants writes the whole batch in one pipelined transaction.
func (r *Repository) batchVariants(
	ctx context.Context, orgID int64, rows []catalog.VariantWriteRow,
) (catalog.VariantWriteResult, error) {
	result := catalog.VariantWriteResult{IDs: make(map[int]int64, len(rows))}
	if len(rows) == 0 {
		return result, nil
	}

	err := r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		batch := &pgx.Batch{}
		queued := make([]catalog.VariantWriteRow, 0, len(rows))
		for _, row := range rows {
			if row.Variant == nil {
				continue
			}
			queued = append(queued, row)
			queueVariant(batch, orgID, row.Variant)
		}

		br := tx.SendBatch(txCtx, batch)
		ids := make(map[int]int64, len(queued))
		var rejected error
		for _, row := range queued {
			var id int64
			if err := br.QueryRow().Scan(&id); err != nil && rejected == nil {
				rejected = err
			}
			ids[row.Ref] = id
		}
		if closeErr := br.Close(); closeErr != nil && rejected == nil {
			rejected = closeErr
		}
		if rejected != nil {
			return errRowRejected
		}

		for _, row := range queued {
			result.IDs[row.Ref] = ids[row.Ref]
			if row.Variant.ID > 0 {
				result.Updated++
			} else {
				row.Variant.ID = ids[row.Ref]
				result.Inserted++
			}
		}
		return nil
	})
	if err != nil {
		return catalog.VariantWriteResult{IDs: map[int]int64{}}, err
	}
	return result, nil
}

// writeVariantsOneByOne is the isolation path: every row gets its own
// transaction, so a rejection costs exactly that row.
func (r *Repository) writeVariantsOneByOne(
	ctx context.Context, orgID int64, rows []catalog.VariantWriteRow,
) (catalog.VariantWriteResult, error) {
	result := catalog.VariantWriteResult{IDs: make(map[int]int64, len(rows))}
	for _, row := range rows {
		if row.Variant == nil {
			continue
		}
		var id int64
		err := r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
			batch := &pgx.Batch{}
			queueVariant(batch, orgID, row.Variant)
			br := tx.SendBatch(txCtx, batch)
			if err := br.QueryRow().Scan(&id); err != nil {
				_ = br.Close()
				return err
			}
			return br.Close()
		})
		if err != nil {
			result.Failures = append(result.Failures, catalog.VariantWriteFailure{
				Ref:     row.Ref,
				Message: writeFailureMessage(err),
			})
			continue
		}
		result.IDs[row.Ref] = id
		if row.Variant.ID > 0 {
			result.Updated++
		} else {
			row.Variant.ID = id
			result.Inserted++
		}
	}
	return result, nil
}

// queueVariant appends one insert or update to a batch.
func queueVariant(batch *pgx.Batch, orgID int64, v *catalog.ProductVariant) {
	if v.ID > 0 {
		// No status on the update path: see updateVariantSQL.
		batch.Queue(updateVariantSQL,
			v.ID, orgID, v.Name, v.SKU, v.Barcode, v.Price, v.CostPrice,
			v.Discount, v.Unit, v.Image, v.IsNegotiable,
			v.BatchNumber, v.ExpiryDate, v.MinOrderQty, v.BranchID, nullableID(v.ProductID))
		return
	}
	batch.Queue(insertVariantSQL,
		orgID, nullableID(v.ProductID), v.Name, v.SKU, v.Barcode, v.Price,
		v.CostPrice, v.Discount, v.Unit, v.Image, string(v.Status),
		v.IsFeatured, v.IsNegotiable, v.BatchNumber, v.ExpiryDate,
		v.MinOrderQty, v.BranchID)
}

// nullableID turns a zero product id into a NULL, which catalog.product_variants
// accepts for an offer bundle that belongs to no catalogue product.
func nullableID(id int64) *int64 {
	if id <= 0 {
		return nil
	}
	return &id
}

// writeFailureMessage turns a database refusal into something a vendor can act
// on, without leaking constraint names or driver internals — those go to the
// logs through the run's failure record, not to the results screen.
func writeFailureMessage(err error) string {
	if database.IsNotFound(err) {
		return i18n.TDefault("w4_mod.s_351_351")
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "duplicate key"):
		return i18n.TDefault("w4_mod.s_352_352")
	case strings.Contains(msg, "violates check constraint"):
		return i18n.TDefault("w4_mod.s_353_353")
	case strings.Contains(msg, "violates foreign key"):
		return i18n.TDefault("w4_mod.s_354_354")
	case strings.Contains(msg, "numeric field overflow"):
		return i18n.TDefault("w4_mod.s_355_355")
	}
	return "تعذر حفظ الصنف بسبب خطأ غير متوقع؛ أعد المحاولة، وإن تكرر راجع الدعم الفني."
}
