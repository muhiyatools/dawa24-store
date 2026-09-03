// Command uipreview serves real pages with synthetic data so their layout can
// be inspected in a browser without a database. Development tool; not built
// into the server image.
package main

import (
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/compare"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

func main() {
	r := chi.NewRouter()
	staticDir, _ := filepath.Abs(filepath.Join("internal", "ui", "static"))
	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.Dir(staticDir))))

	actor := authctx.Actor{UserID: 1, OrganizationID: 2, OrgType: "vendor", OrgStatus: "approved", Role: "vendor_owner"}
	actor.Grants([]string{"vendor.*"})

	r.Get("/market-discounts", func(w http.ResponseWriter, req *http.Request) {
		ctx := authctx.WithActor(req.Context(), actor)
		items := make([]*compare.MarketDiscountRow, 0, 8)
		for i := 0; i < 8; i++ {
			pid := int64(100 + i)
			items = append(items, &compare.MarketDiscountRow{
				ID: int64(i + 1), VariantID: int64(i + 1), AvailableStock: int64(12 + i*7),
				SupplierName:       "مخزن الأعراج الطبي",
				ProductName:        "تريتوالس 1100/40 مجم 14 كيسولة",
				SKU:                "SKU-9984",
				OriginalPrice:      money.FromMinor(35000),
				DiscountPercent:    float64(10 + i*5),
				PriceAfterDiscount: money.FromMinor(29000),
				MatchedProductID:   &pid,
				InCatalog:          i%2 == 0,
				CreatedAt:          time.Now(),
			})
		}
		res := &compare.MarketDiscountsResult{
			Items: items, TotalCount: 46862, Page: 1, Limit: 24, TotalPages: 1953,
			AvailableSuppliers: []string{"مخزن الأعراج الطبي", "شركة النيل", "مخزن المتحدة"},
		}
		view := req.URL.Query().Get("view")
		if view == "" {
			view = "grid"
		}
		_ = pages.MarketDiscountsPage("ar", "rtl", actor, res, compare.MarketDiscountsFilter{Limit: 24, Page: 1}, view).Render(ctx, w)
	})

	r.Get("/compare/market-benchmark", func(w http.ResponseWriter, req *http.Request) {
		ctx := authctx.WithActor(req.Context(), actor)
		rows := make([]*compare.BenchmarkRow, 0, 6)
		names := []string{"تريتوالس 1100/40 مجم 14 كيسولة", "بروجست 200 مل", "روزاليا سيرم 120مل", "ليفا اف 30 قرص", "ابلون كيراتين كريم 120 جرام", "ثيوتاسيد 300مجم"}
		classes := []string{compare.BenchHigher, compare.BenchBetter, compare.BenchEqual, compare.BenchExclusive, compare.BenchHigher, compare.BenchBetter}
		for i := 0; i < 6; i++ {
			rows = append(rows, &compare.BenchmarkRow{
				RowID: int64(i + 1), ProductName: names[i], SKU: fmt.Sprintf("SKU-%04d", 1200+i),
				InCatalog: i%3 != 1, YourPrice: money.FromMinor(int64(35000 + i*1500)),
				YourDiscount: float64(10 + i*3), YourNet: money.FromMinor(int64(29000 + i*1200)),
				BetterOffers: i % 4, EqualOffers: (i + 1) % 3, WorseOffers: (i * 2) % 5,
				BestSupplier: "مخزن النيل الطبي", BestNet: money.FromMinor(int64(26500 + i*900)),
				BestDiscount: float64(18 + i), Classification: classes[i],
			})
		}
		data := pages.MarketBenchmarkPageData{
			Result: &compare.BenchmarkResult{
				Rows: rows, Total: 128, Shown: 6, SupplierName: "تحديث نيوميرم السبت-4",
				HigherCount: 41, EqualCount: 22, BetterCount: 55, ExclusiveCount: 10,
				AvgYourDiscount: 18.4, AvgMarketDiscount: 21.2,
				PotentialBuyerSaving: money.FromMinor(184300),
			},
			Files:  []*compare.CompareFile{{ID: 1, SupplierName: "تحديث نيوميرم السبت-4", RowCount: 128}},
			FileID: 1, ActiveTab: "all",
		}
		_ = pages.CompareMarketBenchmarkPage("ar", "rtl", data).Render(ctx, w)
	})

	log.Println("uipreview on http://localhost:8099")
	log.Fatal(http.ListenAndServe("localhost:8099", r))
}
