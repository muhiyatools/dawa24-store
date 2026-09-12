package pages_test

import (
	"context"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/ui/components"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

func TestVendorEarningsOrderPage_Pagination(t *testing.T) {
	shipments := []*commerce.VendorShipmentProfit{
		{
			ShipmentID:      101,
			ShipmentNumber:  "SHP-001",
			OrderID:         201,
			OrderNumber:     "ORD-201",
			CustomerOrgName: "صيدلية التحرير",
			DeliveredAt:     time.Now(),
			GrossSales:      money.FromMajor(1000),
			Discounts:       money.FromMajor(100),
			NetSales:        money.FromMajor(900),
			COGS:            money.FromMajor(700),
			PlatformFee:     money.FromMajor(0),
			NetProfit:       money.FromMajor(200),
			ProfitMargin:    22.2,
			PaymentStatus:   "paid",
		},
	}

	products := []*commerce.VendorProductProfit{
		{
			ProductID:              1,
			Name:                   "بانادول إكسترا",
			SKU:                    "PAN-01",
			QuantitySold:           50,
			SellingPrice:           money.FromMajor(50),
			DiscountedCost:         money.FromMajor(40),
			TotalRevenue:           money.FromMajor(2500),
			TotalCost:              money.FromMajor(2000),
			NetProfit:              money.FromMajor(500),
			ProfitMargin:           20.0,
			CostDiscountPercentage: 10,
		},
	}

	summary := &commerce.VendorFinancialSummary{
		Period:               "month",
		GrossSales:           money.FromMajor(1000),
		TotalDiscounts:       money.FromMajor(100),
		NetSales:             money.FromMajor(900),
		COGS:                 money.FromMajor(700),
		NetProfit:            money.FromMajor(200),
		ProfitMargin:         22.2,
		DeliveredOrdersCount: 1,
		Shipments:            shipments,
		TopProducts:          products,
	}

	qVals := url.Values{}
	qVals.Set("period", "month")

	ordersPagination := components.PaginationProps{
		CurrentPage: 1,
		PageSize:    15,
		TotalCount:  45, // 3 pages
		BaseURL:     "/vendor/earnings/order",
		QueryValues: qVals,
		PageParam:   "page_orders",
		SizeParam:   "limit_orders",
	}

	productsPagination := components.PaginationProps{
		CurrentPage: 2,
		PageSize:    10,
		TotalCount:  30, // 3 pages
		BaseURL:     "/vendor/earnings/order",
		QueryValues: qVals,
		PageParam:   "page_products",
		SizeParam:   "limit_products",
	}

	data := pages.VendorEarningsOrderPageData{
		Summary:            summary,
		PagedShipments:     shipments,
		OrdersPagination:   ordersPagination,
		PagedProducts:      products,
		ProductsPagination: productsPagination,
		Lang:               "ar",
		Dir:                "rtl",
	}

	var buf strings.Builder
	err := pages.VendorEarningsOrderPage(data).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("failed to render VendorEarningsOrderPage: %v", err)
	}

	html := buf.String()

	// 1. Verify shipments table renders row and pagination
	if !strings.Contains(html, "SHP-001") {
		t.Error("expected SHP-001 in orders table")
	}
	if !strings.Contains(html, "page_orders=") {
		t.Error("expected page_orders parameter in shipments table pagination links")
	}

	// 2. Verify products table renders row and pagination
	if !strings.Contains(html, "بانادول إكسترا") {
		t.Error("expected product name in products table")
	}
	if !strings.Contains(html, "page_products=") {
		t.Error("expected page_products parameter in products table pagination links")
	}
}
