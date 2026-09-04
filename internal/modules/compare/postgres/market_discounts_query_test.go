package postgres

import (
	"strings"
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/compare"
)

// خصومات السوق العامة shows the platform's temporary warehouses, and nothing
// else.
//
// This test replaces one that pinned the opposite policy — the board was read
// from catalogue stock for a while — and the reason it is worth pinning either
// way is the same: a supplier's own Compare Tool upload is theirs, and this
// board is read by every other supplier. The gate is `is_temp_warehouse = TRUE`
// inside a constant FROM clause, so no filter, sort or page parameter can
// relax it. If someone ever assembles that clause from a variable, this fails.
func TestMarketDiscountsShowsOnlyTemporaryWarehouses(t *testing.T) {
	cases := []struct {
		name   string
		filter compare.MarketDiscountsFilter
	}{
		{"no filters", compare.MarketDiscountsFilter{}},
		{"search", compare.MarketDiscountsFilter{Query: "بانادول"}},
		{"supplier", compare.MarketDiscountsFilter{Supplier: "مخزن المتحدة"}},
		{"price range", compare.MarketDiscountsFilter{MinPrice: f(10), MaxPrice: f(90)}},
		{"discount range", compare.MarketDiscountsFilter{MinDiscount: f(5), MaxDiscount: f(50)}},
		{"sort oldest", compare.MarketDiscountsFilter{SortBy: "oldest"}},
		{"sort price asc", compare.MarketDiscountsFilter{SortBy: "price_asc"}},
		{"sort price desc", compare.MarketDiscountsFilter{SortBy: "price_desc"}},
		{"sort discount", compare.MarketDiscountsFilter{SortBy: "discount_desc"}},
		{"deep page", compare.MarketDiscountsFilter{Page: 97, Limit: 96}},
		{"everything at once", compare.MarketDiscountsFilter{
			Query: "x", Supplier: "y", MinPrice: f(1), MaxPrice: f(2),
			MinDiscount: f(3), MaxDiscount: f(4), SortBy: "price_asc", Page: 5, Limit: 48,
		}},
	}

	required := []string{
		"f.is_temp_warehouse = TRUE",
		"f.deleted_at IS NULL",
		"f.archived_at IS NULL",
		"f.status = 'ready'",
		"r.price > 0",
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sql, _, _, _ := buildMarketDiscountsQuery(tc.filter)

			for _, clause := range required {
				if !strings.Contains(sql, clause) {
					t.Errorf("the query is missing %q; the board would show more than the temporary warehouses", clause)
				}
			}

			// The board must not read the catalogue: a variant is a thing to
			// buy, and this screen quotes prices.
			for _, forbidden := range []string{"catalog.product_variants", "inventory.stocks"} {
				if strings.Contains(sql, forbidden) {
					t.Errorf("the query reads %s; this board is temporary-warehouse price lists", forbidden)
				}
			}
		})
	}
}

// A filter must never be able to reach the gate.
//
// Every user-supplied value goes in as a bind parameter, so a supplier name of
// "x' OR f.is_temp_warehouse = FALSE --" is a string to compare against, not
// SQL to run.
func TestMarketDiscountsFiltersAreBound(t *testing.T) {
	hostile := "x' OR f.is_temp_warehouse = FALSE --"
	sql, args, _, _ := buildMarketDiscountsQuery(compare.MarketDiscountsFilter{
		Query: hostile, Supplier: hostile,
	})

	if strings.Contains(sql, hostile) {
		t.Fatal("a filter value was interpolated into the SQL instead of bound")
	}
	found := 0
	for _, a := range args {
		if s, ok := a.(string); ok && strings.Contains(s, hostile) {
			found++
		}
	}
	if found != 2 {
		t.Errorf("expected both hostile values to arrive as bind parameters, found %d", found)
	}
	if !strings.Contains(sql, "f.is_temp_warehouse = TRUE") {
		t.Error("the gate is gone")
	}
}

// Paging is clamped to the sizes the screen offers.
func TestMarketDiscountsQueryClampsPaging(t *testing.T) {
	for _, tc := range []struct {
		in        compare.MarketDiscountsFilter
		wantLimit int
		wantPage  int
	}{
		{compare.MarketDiscountsFilter{}, 24, 1},
		{compare.MarketDiscountsFilter{Limit: 48, Page: 3}, 48, 3},
		{compare.MarketDiscountsFilter{Limit: 96, Page: 2}, 96, 2},
		{compare.MarketDiscountsFilter{Limit: 5000, Page: -4}, 24, 1},
		{compare.MarketDiscountsFilter{Limit: 25}, 24, 1},
	} {
		_, _, page, limit := buildMarketDiscountsQuery(tc.in)
		if limit != tc.wantLimit || page != tc.wantPage {
			t.Errorf("limit %d page %d => limit %d page %d, want %d/%d",
				tc.in.Limit, tc.in.Page, limit, page, tc.wantLimit, tc.wantPage)
		}
	}
}

// The listing and the supplier filter must read the same rows, or the dropdown
// offers a warehouse the board cannot show.
func TestMarketSupplierFilterSharesTheSource(t *testing.T) {
	sql, _, _, _ := buildMarketDiscountsQuery(compare.MarketDiscountsFilter{})
	if !strings.Contains(sql, strings.TrimSpace(marketWarehouseFrom[:40])) {
		t.Error("the listing does not use the shared source clause")
	}
	if !strings.Contains(marketWarehouseFrom, "f.is_temp_warehouse = TRUE") {
		t.Error("the shared source clause does not carry the gate")
	}
}

func f(v float64) *float64 { return &v }
