package compare

import (
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// The platform-wide market intelligence aggregation used to live here. It is
// now strategic_service.go, computed over the whole market rather than over the
// hundred rows ListMarketDiscounts would return, and with every recommendation
// derived from the data rather than printed from a constant list. See
// market_dataset.go for what was wrong and why.

// ClassifyMarketComparison classifies a supplier row against market baseline into one of the 5 filter modes (Plan V5 §2.5.2).
func ClassifyMarketComparison(supplierNet, marketNet money.Amount, supplierDiscount, marketDiscount float64, hasMarketOffer bool) MarketComparisonFilter {
	if !hasMarketOffer {
		return MarketFilterExclusives
	}
	if supplierNet.Minor() < marketNet.Minor() || supplierDiscount > marketDiscount {
		return MarketFilterHigherDiscount
	}
	if supplierNet.Minor() > marketNet.Minor() || supplierDiscount < marketDiscount {
		return MarketFilterLowerDiscount
	}
	return MarketFilterEqualToMarket
}
