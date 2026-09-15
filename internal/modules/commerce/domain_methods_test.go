package commerce_test

import (
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

func TestOrderLine_CostAndProfitCalculations(t *testing.T) {
	t.Run("Line with explicit CostPrice and CostDiscountPercentage", func(t *testing.T) {
		costPrice := money.FromMajor(50)
		line := &commerce.OrderLine{
			Quantity:               3,
			UnitPrice:              money.FromMajor(80),
			TotalPrice:             money.FromMajor(240), // 3 * 80
			ListPrice:              money.FromMajor(100), // Retail strike price 100
			CostPrice:              &costPrice,           // Unit cost 50
			CostDiscountPercentage: 10.0,                  // 10% cost discount -> 45 unit cost
		}

		if !line.HasCostPrice() {
			t.Fatal("expected HasCostPrice to be true")
		}

		// Unit discounted purchase cost: 50 - (50 * 0.10) = 45 EGP = 4500 minor
		unitCost := line.EffectivePurchaseCost()
		if unitCost.Minor() != 4500 {
			t.Fatalf("expected unit cost 4500, got %d", unitCost.Minor())
		}

		// Total purchase cost: 45 * 3 = 135 EGP = 13500 minor
		totPurchCost := line.TotalPurchaseCost()
		if totPurchCost.Minor() != 13500 {
			t.Fatalf("expected total purchase cost 13500, got %d", totPurchCost.Minor())
		}

		// TotalCost: must strictly be purchase cost (13500 minor).
		// Selling discount (60 EGP conceded against list price 300) must NOT be added!
		totCost := line.TotalCost()
		if totCost.Minor() != 13500 {
			t.Fatalf("expected total cost 13500, got %d (selling discount must not be added)", totCost.Minor())
		}

		// Net profit: TotalPrice (240) - TotalCost (135) = 105 EGP = 10500 minor
		netProfit := line.TotalNetProfit()
		if netProfit.Minor() != 10500 {
			t.Fatalf("expected net profit 10500, got %d", netProfit.Minor())
		}
	})

	t.Run("Line without CostPrice results in 0 cost and 0 net profit", func(t *testing.T) {
		line := &commerce.OrderLine{
			Quantity:   2,
			UnitPrice:  money.FromMajor(80),
			TotalPrice: money.FromMajor(160),
			ListPrice:  money.FromMajor(100),
			CostPrice:  nil, // No cost recorded
		}

		if line.HasCostPrice() {
			t.Fatal("expected HasCostPrice to be false")
		}

		if line.EffectivePurchaseCost().Minor() != 0 {
			t.Fatalf("expected 0 effective purchase cost, got %d", line.EffectivePurchaseCost().Minor())
		}

		if line.TotalPurchaseCost().Minor() != 0 {
			t.Fatalf("expected 0 total purchase cost, got %d", line.TotalPurchaseCost().Minor())
		}

		if line.TotalCost().Minor() != 0 {
			t.Fatalf("expected 0 total cost, got %d", line.TotalCost().Minor())
		}

		if line.TotalNetProfit().Minor() != 0 {
			t.Fatalf("expected 0 net profit, got %d", line.TotalNetProfit().Minor())
		}
	})
}
