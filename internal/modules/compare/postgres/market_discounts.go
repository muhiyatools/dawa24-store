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

// خصومات السوق العامة — the temporary warehouses, and nothing else.
//
// This screen shows the price lists moderators and administrators upload as
// **temporary warehouses**: rows of compare.file_rows belonging to a
// compare.files row with is_temp_warehouse = TRUE. That is the board's whole
// purpose — a market-wide view of what the trade is quoting — and it is what
// the people who maintain it put into it.
//
// Two things are deliberately excluded and both are load-bearing:
//
//   - **Ordinary Compare Tool uploads.** A supplier's own price list is theirs.
//     is_temp_warehouse = TRUE is the only gate, it lives in the FROM clause
//     rather than in a WHERE the caller can influence, and no request parameter
//     reaches it. TestMarketDiscountsShowsOnlyTemporaryWarehouses pins that.
//   - **Archived and deleted files.** A warehouse someone withdrew stops being
//     part of the market the moment they withdraw it.
//
// It is worth being explicit about what this board is not: it is price
// intelligence, not a catalogue. A row here is a line from a spreadsheet, not a
// variant a pharmacy can add to a basket, and it is only orderable when the
// matching pipeline has bound it to a catalogue product. The card says so
// rather than offering a button that goes nowhere.

// marketWarehouseFrom is the source, shared by the listing and the supplier
// filter so the two can never disagree about what "the market" contains.
const marketWarehouseFrom = `
	FROM compare.file_rows r
	JOIN compare.files f ON f.id = r.file_id
	WHERE f.is_temp_warehouse = TRUE
	  AND f.deleted_at IS NULL
	  AND f.archived_at IS NULL
	  AND f.status = 'ready'
	  AND r.price > 0
	  AND COALESCE(TRIM(r.raw_name), '') <> ''`

// marketWarehouseSupplierSQL is the uploading warehouse's name. A temporary
// warehouse has no organization_id — it is a moderator's upload — so there is
// no org.organizations row to read a trade name from.
const marketWarehouseSupplierSQL = `COALESCE(NULLIF(TRIM(f.supplier_name), ''), f.original_filename)`

// marketWarehouseDiscountSQL is the discount a card may show.
//
// It is clamped to 0 outside 0..100, and that is not defensive tidying: 6,182
// of the 46,862 live rows carry exactly 100.00, which is a column that was
// mapped to the wrong place rather than a supplier giving stock away. Those
// rows are what produced the "46,000 lines at 100% خصم / 0.00 ج.م" this board
// was once abandoned over.
//
// Every use goes through this — the column, the sort and the range filter — so
// the board cannot rank by a number it refuses to print. Sorting by the raw
// column put all 6,182 of them on page one of "الأعلى خصماً", each showing 0%.
const marketWarehouseDiscountSQL = `
	CASE WHEN COALESCE(r.discount, 0) > 0 AND COALESCE(r.discount, 0) < 100
	     THEN r.discount ELSE 0 END`

// marketWarehouseNetSQL is the price after the sheet's discount.
//
// price_after_discount is trusted when the upload carried one and recomputed
// from the percentage otherwise, because the two disagree on files whose
// columns were mapped before the mapping was corrected.
const marketWarehouseNetSQL = `
	CASE
	  WHEN COALESCE(r.price_after_discount, 0) > 0 THEN r.price_after_discount
	  WHEN COALESCE(r.discount, 0) > 0 AND COALESCE(r.discount, 0) < 100
	    THEN ROUND(r.price * (100.0 - r.discount) / 100.0, 2)
	  ELSE r.price
	END`

// ListDistinctSuppliers names the temporary warehouses currently on the board.
func (r *Repository) ListDistinctSuppliers(ctx context.Context) ([]string, error) {
	var suppliers []string
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		sql := `
			SELECT DISTINCT TRIM(` + marketWarehouseSupplierSQL + `) AS supplier_name
			` + marketWarehouseFrom + `
			  AND TRIM(COALESCE(` + marketWarehouseSupplierSQL + `, '')) <> ''
			ORDER BY 1 ASC;`
		rows, err := tx.Query(txCtx, sql)
		if err != nil {
			return fmt.Errorf("compare postgres: list market suppliers: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var sup string
			if err := rows.Scan(&sup); err != nil {
				return err
			}
			if strings.TrimSpace(sup) != "" {
				suppliers = append(suppliers, sup)
			}
		}
		return rows.Err()
	})
	return suppliers, err
}

// ListMarketDiscounts returns one page of temporary-warehouse rows.
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
				matchedID  *int64
				totalCount int64
			)
			if err := rows.Scan(
				&item.ID, &item.FileID, &item.SupplierName, &item.ProductName,
				&item.OriginalPrice, &item.DiscountPercent, &item.PriceAfterDiscount,
				&matchedID, &item.UploadedAt, &totalCount,
			); err != nil {
				return err
			}
			result.TotalCount = totalCount

			item.MatchedProductID = matchedID
			// Orderable only once the matching pipeline has bound this line to a
			// catalogue product. Until then it is a quoted price, and the card
			// says so instead of offering a button that leads nowhere.
			item.InCatalog = matchedID != nil && *matchedID > 0
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
