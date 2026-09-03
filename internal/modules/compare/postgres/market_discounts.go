package postgres

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/compare"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// خصومات السوق العامة — supplier stock, not the drug catalogue.
//
// This screen used to read compare.file_rows: whatever a moderator had uploaded
// as a temporary warehouse. That is a spreadsheet of prices, not inventory —
// nothing in it says a product exists, is approved, or can be bought, which is
// why the page showed forty-six thousand rows at "100% خصم / 0.00 ج.م" that no
// pharmacy could order.
//
// It now reads the thing it claims to show: **approved vendors' product
// variants that are actually in stock**. Every row here is a real variant, of a
// real catalogue product, from an approved supplier, with a positive quantity
// in inventory.stocks.

// marketStockCTE is the availability gate, and the reason it is a CTE rather
// than a WHERE clause: stock lives one row per warehouse, so it has to be
// summed per variant before it can be compared to zero. A variant whose
// warehouses sum to zero is not in the CTE, so it cannot appear in the results
// at all — there is no flag, no filter and no request parameter that can bring
// it back. That is the whole point of doing it here rather than in the page.
const marketStockCTE = `
	WITH variant_stock AS (
		SELECT s.product_variant_id, SUM(s.quantity)::bigint AS qty
		FROM inventory.stocks s
		WHERE s.deleted_at IS NULL
		GROUP BY s.product_variant_id
		HAVING SUM(s.quantity) > 0
	)`

// marketSupplierNameSQL prefers the Arabic trade name, then the English one,
// then the registered legal name — the same order org.Organization uses in Go.
const marketSupplierNameSQL = `
	COALESCE(
		NULLIF(TRIM(o.trade_name->>'ar'), ''),
		NULLIF(TRIM(o.trade_name->>'en'), ''),
		o.legal_name
	)`

// marketBaseFrom is shared by the listing and the supplier filter so the two can
// never disagree about what "the market" contains.
const marketBaseFrom = `
	FROM catalog.product_variants v
	JOIN variant_stock st ON st.product_variant_id = v.id
	JOIN catalog.products p ON p.id = v.product_id AND p.deleted_at IS NULL
	JOIN org.organizations o ON o.id = v.organization_id
	WHERE v.deleted_at IS NULL
	  AND v.status = 'active'
	  AND v.price > 0
	  AND o.status = 'approved'
	  AND o.deleted_at IS NULL`

// marketNetPriceSQL is the price a pharmacy pays. catalog.product_variants
// stores `discount` as a percentage (26.40 meaning 26.40%), matching
// catalog.ProductVariant.EffectiveSellingPrice; a value outside 0..100 is
// treated as no discount rather than as a nonsensical price.
const marketNetPriceSQL = `
	CASE WHEN v.discount > 0 AND v.discount < 100
	     THEN ROUND(v.price * (100.0 - v.discount) / 100.0, 2)
	     ELSE v.price END`

// ListDistinctSuppliers names the approved vendors that have something in stock
// right now, for the supplier filter. A vendor whose stock ran out drops off the
// list, because every one of their rows has dropped off the page.
func (r *Repository) ListDistinctSuppliers(ctx context.Context) ([]string, error) {
	var suppliers []string
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		sql := marketStockCTE + `
			SELECT DISTINCT TRIM(` + marketSupplierNameSQL + `) AS supplier_name
			` + marketBaseFrom + `
			  AND TRIM(COALESCE(` + marketSupplierNameSQL + `, '')) <> ''
			ORDER BY 1 ASC;`
		rows, err := tx.Query(txCtx, sql)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var sup string
			if err := rows.Scan(&sup); err == nil && strings.TrimSpace(sup) != "" {
				suppliers = append(suppliers, sup)
			}
		}
		return rows.Err()
	})
	return suppliers, err
}

// ListMarketDiscounts returns one page of in-stock supplier offers.
func (r *Repository) ListMarketDiscounts(
	ctx context.Context, filter compare.MarketDiscountsFilter,
) (*compare.MarketDiscountsResult, error) {
	sql, args, page, limit := buildMarketDiscountsQuery(filter)

	result := &compare.MarketDiscountsResult{
		Items:      make([]*compare.MarketDiscountRow, 0, limit),
		Page:       page,
		Limit:      limit,
		TotalPages: 1,
	}

	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(txCtx, sql, args...)
		if err != nil {
			return fmt.Errorf("compare postgres: list market discounts: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var (
				item       compare.MarketDiscountRow
				productID  int64
				totalCount int64
			)
			if err := rows.Scan(
				&item.VariantID, &productID, &item.SupplierName, &item.ProductName,
				&item.SKU, &item.OriginalPrice, &item.DiscountPercent,
				&item.PriceAfterDiscount, &item.AvailableStock, &item.CreatedAt,
				&totalCount,
			); err != nil {
				return err
			}
			result.TotalCount = totalCount

			item.ID = item.VariantID
			pid := productID
			item.MatchedProductID = &pid
			// Every row is a catalogue product by construction now — the join
			// to catalog.products is what put it here — so it is always
			// orderable, unlike a spreadsheet row that merely resembled one.
			item.InCatalog = true
			if item.OriginalPrice.Minor() > item.PriceAfterDiscount.Minor() {
				item.DiscountValue = money.FromMinor(
					item.OriginalPrice.Minor() - item.PriceAfterDiscount.Minor())
			}
			result.Items = append(result.Items, &item)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}

	if result.TotalCount > 0 {
		result.TotalPages = int((result.TotalCount + int64(limit) - 1) / int64(limit))
	}
	result.HasPrev = page > 1
	result.HasNext = page < result.TotalPages

	suppliers, sErr := r.ListDistinctSuppliers(ctx)
	if sErr != nil {
		return nil, sErr
	}
	result.AvailableSuppliers = suppliers

	return result, nil
}
