package postgres

import (
	"fmt"
	"strings"

	"github.com/muhiya/dawa24-store/internal/modules/compare"
)

// buildMarketDiscountsQuery assembles the listing SQL, its arguments, and the
// page and page size actually used.
//
// It is a separate, pure function so the invariants that matter can be asserted
// without a database: that the availability CTE is always joined, that no
// filter can widen the result beyond in-stock variants of approved vendors, and
// that the page never reads a compare-tool spreadsheet again.
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
			`(p.name->>'ar' ILIKE $%d OR p.name->>'en' ILIKE $%d
			  OR v.name->>'ar' ILIKE $%d OR COALESCE(v.sku, '') ILIKE $%d
			  OR COALESCE(v.barcode, '') ILIKE $%d OR %s ILIKE $%d)`,
			argIdx, argIdx, argIdx, argIdx, argIdx, marketSupplierNameSQL, argIdx))
		args = append(args, "%"+q+"%")
		argIdx++
	}

	if sup := strings.TrimSpace(filter.Supplier); sup != "" {
		where = append(where, fmt.Sprintf("TRIM(%s) = $%d", marketSupplierNameSQL, argIdx))
		args = append(args, sup)
		argIdx++
	}

	if filter.MinPrice != nil {
		where = append(where, fmt.Sprintf("(%s) >= $%d", marketNetPriceSQL, argIdx))
		args = append(args, *filter.MinPrice)
		argIdx++
	}
	if filter.MaxPrice != nil {
		where = append(where, fmt.Sprintf("(%s) <= $%d", marketNetPriceSQL, argIdx))
		args = append(args, *filter.MaxPrice)
		argIdx++
	}
	if filter.MinDiscount != nil {
		where = append(where, fmt.Sprintf("v.discount >= $%d", argIdx))
		args = append(args, *filter.MinDiscount)
		argIdx++
	}
	if filter.MaxDiscount != nil {
		where = append(where, fmt.Sprintf("v.discount <= $%d", argIdx))
		args = append(args, *filter.MaxDiscount)
		argIdx++
	}

	extra := ""
	if len(where) > 0 {
		extra = "\n\t\t  AND " + strings.Join(where, "\n\t\t  AND ")
	}

	orderBy := "v.created_at DESC, v.id DESC"
	switch filter.SortBy {
	case "oldest":
		orderBy = "v.created_at ASC, v.id ASC"
	case "price_asc":
		orderBy = fmt.Sprintf("(%s) ASC, v.discount DESC", marketNetPriceSQL)
	case "price_desc":
		orderBy = fmt.Sprintf("(%s) DESC, v.discount DESC", marketNetPriceSQL)
	case "discount_desc", "":
		orderBy = fmt.Sprintf("v.discount DESC, (%s) ASC", marketNetPriceSQL)
	}

	sql = marketStockCTE + fmt.Sprintf(`
		SELECT
			v.id,
			v.product_id,
			%s AS supplier_name,
			COALESCE(NULLIF(TRIM(p.name->>'ar'), ''), NULLIF(TRIM(v.name->>'ar'), ''), '') AS product_name,
			COALESCE(v.sku, '') AS sku,
			v.price,
			CASE WHEN v.discount > 0 AND v.discount < 100 THEN v.discount ELSE 0 END AS discount_percent,
			%s AS price_after_discount,
			st.qty AS available_stock,
			v.created_at,
			COUNT(*) OVER() AS total_count
		%s%s
		ORDER BY %s
		LIMIT $%d OFFSET $%d;`,
		marketSupplierNameSQL, marketNetPriceSQL, marketBaseFrom, extra, orderBy, argIdx, argIdx+1)

	args = append(args, limit, offset)
	return sql, args, page, limit
}
