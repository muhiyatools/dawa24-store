package catalog_test

import (
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

func TestProductVariant_CostAndProfitCalculations(t *testing.T) {
	t.Run("Variant with cost and discounts", func(t *testing.T) {
		cost := money.FromMajor(80)
		v := &catalog.ProductVariant{
			Price:                  money.FromMajor(100), // Retail price
			Discount:               money.FromMinor(1000), // 10.00% selling discount -> 90 selling price
			CostPrice:              &cost,
			CostDiscountPercentage: 20.0, // 20% cost discount -> 80 - 16 = 64
		}

		if !v.HasCostPrice() {
			t.Fatal("expected HasCostPrice to be true")
		}

		effSell := v.EffectiveSellingPrice()
		if effSell.Minor() != 9000 {
			t.Fatalf("expected effective selling price 9000 (90 EGP), got %d", effSell.Minor())
		}

		discCost := v.DiscountedCost()
		if discCost.Minor() != 6400 {
			t.Fatalf("expected discounted cost 6400 (64 EGP), got %d", discCost.Minor())
		}

		// UnitTotalCost must strictly equal DiscountedCost (64), NOT adding selling discount (10)
		totCost := v.UnitTotalCost()
		if totCost.Minor() != 6400 {
			t.Fatalf("expected unit total cost 6400, got %d", totCost.Minor())
		}

		// Net profit: 90 - 64 = 26
		netProfit := v.UnitNetProfit()
		if netProfit.Minor() != 2600 {
			t.Fatalf("expected net profit 2600 (26 EGP), got %d", netProfit.Minor())
		}

		// Margin %: (26 / 90) * 100 = 28.888...%
		margin := v.ProfitMarginPercent()
		expectedMargin := (26.0 / 90.0) * 100.0
		if margin < expectedMargin-0.01 || margin > expectedMargin+0.01 {
			t.Fatalf("expected margin ~%.2f%%, got %.2f%%", expectedMargin, margin)
		}
	})

	t.Run("Variant without cost price returns 0 cost and 0.0% margin", func(t *testing.T) {
		v := &catalog.ProductVariant{
			Price:    money.FromMajor(100),
			Discount: money.FromMinor(1000), // 10%
		}

		if v.HasCostPrice() {
			t.Fatal("expected HasCostPrice to be false")
		}

		if v.DiscountedCost().Minor() != 0 {
			t.Fatalf("expected 0 discounted cost when no cost price, got %d", v.DiscountedCost().Minor())
		}

		if v.UnitTotalCost().Minor() != 0 {
			t.Fatalf("expected 0 total cost when no cost price, got %d", v.UnitTotalCost().Minor())
		}

		if v.UnitNetProfit().Minor() != 0 {
			t.Fatalf("expected 0 net profit when no cost price, got %d", v.UnitNetProfit().Minor())
		}

		margin := v.ProfitMarginPercent()
		if margin != 0.0 {
			t.Fatalf("expected exactly 0.0%% margin when no cost price, got %.2f%%", margin)
		}
	})
}
