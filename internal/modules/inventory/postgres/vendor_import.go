package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/inventory"
)

// Bulk stock persistence for the vendor catalogue import.

// errStockRejected marks a batch in which at least one statement was refused,
// so it can be rolled back and retried row by row.
var errStockRejected = errors.New("inventory postgres: a row in the batch was rejected")

// upsertStockSQL writes one balance. The quantity expression is chosen by mode
// rather than parameterised, because Postgres cannot take an operator as a
// parameter and the three forms are a closed set defined in this file.
const upsertStockSQL = `
	INSERT INTO inventory.stocks (
		organization_id, warehouse_id, product_id, product_variant_id,
		quantity, min_threshold, negotiation
	) VALUES ($1, $2, $3, $4, $5, $6, $7)
	ON CONFLICT (warehouse_id, product_variant_id) DO UPDATE SET
		quantity = %s,
		min_threshold = GREATEST(EXCLUDED.min_threshold, 0),
		product_id = EXCLUDED.product_id,
		updated_at = now()
	RETURNING id`

// quantityExpression is what an existing balance becomes under each mode.
func quantityExpression(mode inventory.StockMode, hasQuantity bool) string {
	if !hasQuantity {
		// The file said nothing about this row's balance. Whatever the mode, a
		// missing cell must not be written as a zero.
		return "inventory.stocks.quantity"
	}
	switch mode {
	case inventory.StockAdd:
		return "GREATEST(inventory.stocks.quantity + EXCLUDED.quantity, 0)"
	case inventory.StockKeep:
		return "inventory.stocks.quantity"
	default:
		return "GREATEST(EXCLUDED.quantity, 0)"
	}
}

// BulkWriteStocks writes a batch of balances, isolating a refused row rather
// than losing the batch to it.
func (r *Repository) BulkWriteStocks(
	ctx context.Context, mode inventory.StockMode, rows []inventory.StockWriteRow,
) (inventory.StockWriteResult, error) {
	result, err := r.batchStocks(ctx, mode, rows)
	if err == nil {
		return result, nil
	}
	if !errors.Is(err, errStockRejected) {
		return result, fmt.Errorf("inventory postgres: bulk write stocks: %w", err)
	}
	return r.writeStocksOneByOne(ctx, mode, rows)
}

func (r *Repository) batchStocks(
	ctx context.Context, mode inventory.StockMode, rows []inventory.StockWriteRow,
) (inventory.StockWriteResult, error) {
	var result inventory.StockWriteResult
	if len(rows) == 0 {
		return result, nil
	}

	err := r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		batch := &pgx.Batch{}
		queued := 0
		for _, row := range rows {
			if row.Stock == nil || row.Stock.ProductVariantID <= 0 || row.Stock.WarehouseID <= 0 {
				continue
			}
			queueStock(batch, mode, row)
			queued++
		}
		if queued == 0 {
			return nil
		}

		br := tx.SendBatch(txCtx, batch)
		var rejected error
		for i := 0; i < queued; i++ {
			var id int64
			if err := br.QueryRow().Scan(&id); err != nil && rejected == nil {
				rejected = err
			}
		}
		if closeErr := br.Close(); closeErr != nil && rejected == nil {
			rejected = closeErr
		}
		if rejected != nil {
			return errStockRejected
		}
		result.Written = queued
		return nil
	})
	if err != nil {
		return inventory.StockWriteResult{}, err
	}
	return result, nil
}

func (r *Repository) writeStocksOneByOne(
	ctx context.Context, mode inventory.StockMode, rows []inventory.StockWriteRow,
) (inventory.StockWriteResult, error) {
	var result inventory.StockWriteResult
	for _, row := range rows {
		if row.Stock == nil || row.Stock.ProductVariantID <= 0 || row.Stock.WarehouseID <= 0 {
			continue
		}
		err := r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
			batch := &pgx.Batch{}
			queueStock(batch, mode, row)
			br := tx.SendBatch(txCtx, batch)
			var id int64
			if err := br.QueryRow().Scan(&id); err != nil {
				_ = br.Close()
				return err
			}
			return br.Close()
		})
		if err != nil {
			result.Failures = append(result.Failures, inventory.StockFailure{
				Ref:     row.Ref,
				Message: stockFailureMessage(err),
			})
			continue
		}
		result.Written++
	}
	return result, nil
}

func queueStock(batch *pgx.Batch, mode inventory.StockMode, row inventory.StockWriteRow) {
	s := row.Stock
	quantity := s.Quantity
	if quantity < 0 {
		quantity = 0
	}
	batch.Queue(
		fmt.Sprintf(upsertStockSQL, quantityExpression(mode, row.HasQuantity)),
		s.OrganizationID, s.WarehouseID, s.ProductID, s.ProductVariantID,
		quantity, maxInt(s.MinThreshold, 0), s.Negotiation)
}

func stockFailureMessage(err error) string {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "stocks_quantity_non_negative"):
		return "الكمية الناتجة سالبة؛ لا يمكن أن يقل الرصيد عن صفر"
	case strings.Contains(msg, "violates foreign key"):
		return "المخزن أو الصنف المرتبط غير موجود"
	}
	return "تعذر تحديث الرصيد: " + msg
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
