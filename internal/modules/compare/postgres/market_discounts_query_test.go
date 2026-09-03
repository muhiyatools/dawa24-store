package postgres

import (
	"strings"
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/compare"
)

// The availability gate is the whole security property of this screen: an
// out-of-stock variant must be unreachable, not merely unshown. These assert it
// at the query level, where no request parameter can undo it.
func TestMarketDiscountsQueryAlwaysGatesOnStock(t *testing.T) {
	f := func(min, max float64) *float64 { return &min }

	cases := []struct {
		name   string
		filter compare.MarketDiscountsFilter
	}{
		{"no filters", compare.MarketDiscountsFilter{}},
		{"search", compare.MarketDiscountsFilter{Query: "اماريل"}},
		{"supplier", compare.MarketDiscountsFilter{Supplier: "مخزن المتحدة"}},
		{"price range", compare.MarketDiscountsFilter{MinPrice: f(10, 0), MaxPrice: f(90, 0)}},
		{"discount range", compare.MarketDiscountsFilter{MinDiscount: f(5, 0), MaxDiscount: f(50, 0)}},
		{"sorted cheapest", compare.MarketDiscountsFilter{SortBy: "price_asc"}},
		{"deep page", compare.MarketDiscountsFilter{Page: 400, Limit: 96}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sql, _, _, _ := buildMarketDiscountsQuery(tc.filter)

			// The CTE keeps only variants whose warehouse quantities sum above
			// zero, and it is INNER JOINed, so nothing outside it can appear.
			if !strings.Contains(sql, "HAVING SUM(s.quantity) > 0") {
				t.Error("the query does not restrict to variants with stock")
			}
			if !strings.Contains(sql, "JOIN variant_stock st ON st.product_variant_id = v.id") {
				t.Error("the stock CTE is not inner-joined; out-of-stock variants could appear")
			}
			if strings.Contains(sql, "LEFT JOIN variant_stock") {
				t.Error("the stock join is outer; that readmits variants with no stock")
			}

			// Only approved vendors' live variants.
			for _, must := range []string{
				"v.deleted_at IS NULL",
				"v.status = 'active'",
				"o.status = 'approved'",
				"p.deleted_at IS NULL",
			} {
				if !strings.Contains(sql, must) {
					t.Errorf("the query is missing the %q predicate", must)
				}
			}

			// The old source must be gone entirely.
			for _, banned := range []string{"compare.file_rows", "compare.files", "is_temp_warehouse"} {
				if strings.Contains(sql, banned) {
					t.Errorf("the query still reads %s; this screen is catalogue stock now", banned)
				}
			}
		})
	}
}

// A page size arriving from the query string cannot become an unbounded scan.
func TestMarketDiscountsQueryClampsPaging(t *testing.T) {
	for _, tc := range []struct {
		in   int
		want int
	}{{0, 24}, {-5, 24}, {7, 24}, {24, 24}, {48, 48}, {96, 96}, {5000, 24}} {
		_, args, _, limit := buildMarketDiscountsQuery(compare.MarketDiscountsFilter{Limit: tc.in})
		if limit != tc.want {
			t.Errorf("limit %d became %d, want %d", tc.in, limit, tc.want)
		}
		if got := args[len(args)-2]; got != tc.want {
			t.Errorf("limit %d bound %v as the SQL LIMIT, want %d", tc.in, got, tc.want)
		}
	}

	_, args, page, limit := buildMarketDiscountsQuery(compare.MarketDiscountsFilter{Page: 0, Limit: 48})
	if page != 1 {
		t.Errorf("page 0 became %d, want 1", page)
	}
	if off := args[len(args)-1]; off != 0 {
		t.Errorf("page 1 offset is %v, want 0", off)
	}
	_ = limit
}

// Every filter value reaches Postgres as a bound parameter.
func TestMarketDiscountsQueryBindsFilters(t *testing.T) {
	min, max := 12.5, 80.0
	sql, args, _, _ := buildMarketDiscountsQuery(compare.MarketDiscountsFilter{
		Query: "'; DROP TABLE catalog.products; --", Supplier: "مخزن", MinPrice: &min, MaxPrice: &max,
	})
	if strings.Contains(sql, "DROP TABLE") {
		t.Fatal("a filter value was interpolated into the SQL text")
	}
	if len(args) != 6 { // query, supplier, min, max, limit, offset
		t.Fatalf("expected 6 bound args, got %d: %v", len(args), args)
	}
}
