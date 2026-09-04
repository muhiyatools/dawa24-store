package postgres

import (
	"fmt"
	"strings"

	"github.com/muhiya/dawa24-store/internal/modules/compare"
)

// Building one page of خصومات السوق العامة.
//
// The source clause is a constant spliced in whole (marketWarehouseFrom), never
// assembled from anything the caller sent. Filters can only ever append to it,
// so is_temp_warehouse = TRUE cannot be relaxed by a request parameter — which
// is the property that keeps a supplier's own Compare Tool upload off a board
// every other supplier reads.
func buildMarketDiscountsQuery(filter compare.MarketDiscountsFilter) (sql string, args []any, page, limit int) {
	limit = filter.Limit
	if limit != 24 && limit != 48 && limit != 96 {
		limit = 24
	}
	page = filter.Page
	if page <= 0 {
		page = 1
	}
	offset := (page - 1) * limit

	argIdx := 1
	var where []string

	if q := strings.TrimSpace(filter.Query); q != "" {
		where = append(where, fmt.Sprintf(
			`(r.raw_name ILIKE $%d
			  OR platform.normalize_arabic(r.raw_name) ILIKE platform.normalize_arabic($%d)
			  OR COALESCE(r.normalized_name, '') ILIKE $%d
			  OR %s ILIKE $%d)`,
			argIdx, argIdx, argIdx, marketWarehouseSupplierSQL, argIdx))
		args = append(args, "%"+q+"%")
		argIdx++
	}

	if sup := strings.TrimSpace(filter.Supplier); sup != "" {
		where = append(where, fmt.Sprintf("TRIM(%s) = $%d", marketWarehouseSupplierSQL, argIdx))
		args = append(args, sup)
		argIdx++
	}

	if filter.MinPrice != nil {
		where = append(where, fmt.Sprintf("(%s) >= $%d", marketWarehouseNetSQL, argIdx))
		args = append(args, *filter.MinPrice)
		argIdx++
	}
	if filter.MaxPrice != nil {
		where = append(where, fmt.Sprintf("(%s) <= $%d", marketWarehouseNetSQL, argIdx))
		args = append(args, *filter.MaxPrice)
		argIdx++
	}
	if filter.MinDiscount != nil {
		where = append(where, fmt.Sprintf("(%s) >= $%d", marketWarehouseDiscountSQL, argIdx))
		args = append(args, *filter.MinDiscount)
		argIdx++
	}
	if filter.MaxDiscount != nil {
		where = append(where, fmt.Sprintf("(%s) <= $%d", marketWarehouseDiscountSQL, argIdx))
		args = append(args, *filter.MaxDiscount)
		argIdx++
	}

	extra := ""
	if len(where) > 0 {
		extra = "\n\t\t  AND " + strings.Join(where, "\n\t\t  AND ")
	}

	// The date shown on a card is the upload date, so "newest" means the most
	// recently uploaded warehouse rather than the row's own insert order.
	orderBy := "f.created_at DESC, r.id DESC"
	switch filter.SortBy {
	case "oldest":
		orderBy = "f.created_at ASC, r.id ASC"
	case "price_asc":
		orderBy = fmt.Sprintf("(%s) ASC, (%s) DESC", marketWarehouseNetSQL, marketWarehouseDiscountSQL)
	case "price_desc":
		orderBy = fmt.Sprintf("(%s) DESC, (%s) DESC", marketWarehouseNetSQL, marketWarehouseDiscountSQL)
	case "discount_desc", "":
		// The clamped expression, not the raw column: ranking by a number the
		// card refuses to print is how 6,182 rows showing "0%" reached the top
		// of "الأعلى خصماً".
		orderBy = fmt.Sprintf("(%s) DESC, (%s) ASC", marketWarehouseDiscountSQL, marketWarehouseNetSQL)
	}

	sql = fmt.Sprintf(`
		SELECT
			r.id,
			r.file_id,
			%s AS supplier_name,
			TRIM(r.raw_name) AS product_name,
			r.price,
			%s AS discount_percent,
			%s AS price_after_discount,
			r.matched_product_id,
			f.created_at AS uploaded_at,
			COUNT(*) OVER() AS total_count
		%s%s
		ORDER BY %s
		LIMIT $%d OFFSET $%d;`,
		marketWarehouseSupplierSQL, marketWarehouseDiscountSQL, marketWarehouseNetSQL,
		marketWarehouseFrom, extra, orderBy, argIdx, argIdx+1)

	args = append(args, limit, offset)
	return sql, args, page, limit
}
