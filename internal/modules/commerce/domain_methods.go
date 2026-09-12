package commerce

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// IsOfferLine reports whether this line is an offer bought as a unit rather
// than a catalogue item.
//
// An offer is sold at its own price for whatever it contains; the products
// listed against it are a manifest, not separately purchasable lines. Such a
// line therefore carries an offer and no product reference at all, which is the
// shape migration 155 added to commerce.cart_items and which the check
// constraint there enforces.
func (i *CartItem) IsOfferLine() bool {
	return i != nil && i.OfferID != nil && *i.OfferID > 0 &&
		i.ProductID == 0 && i.ProductVariantID == 0
}

// BranchLocationURL returns the most accurate Google Maps navigation or directions link for the branch.
func (s *OrderShipment) BranchLocationURL() string {
	if s == nil {
		return ""
	}
	if s.CustomerBranchLatitude != nil && s.CustomerBranchLongitude != nil &&
		(*s.CustomerBranchLatitude != 0 || *s.CustomerBranchLongitude != 0) {
		return fmt.Sprintf("https://www.google.com/maps/dir/?api=1&destination=%.6f,%.6f", *s.CustomerBranchLatitude, *s.CustomerBranchLongitude)
	}
	if trimmed := strings.TrimSpace(s.CustomerBranchGoogleMapsURL); trimmed != "" {
		return trimmed
	}
	if trimmedAddr := strings.TrimSpace(s.CustomerBranchAddress); trimmedAddr != "" {
		return "https://www.google.com/maps/search/?api=1&query=" + url.QueryEscape(trimmedAddr)
	}
	return ""
}

// HasExactCoordinates reports whether the shipment recipient branch has valid GPS coordinates.
func (s *OrderShipment) HasExactCoordinates() bool {
	return s != nil && s.CustomerBranchLatitude != nil && s.CustomerBranchLongitude != nil &&
		(*s.CustomerBranchLatitude != 0 || *s.CustomerBranchLongitude != 0)
}

// TotalUnitsCount returns the total quantity sum of all order lines in the shipment.
func (s *OrderShipment) TotalUnitsCount() int {
	if s == nil {
		return 0
	}
	total := 0
	for _, l := range s.Lines {
		if l != nil {
			total += l.Quantity
		}
	}
	return total
}

// HasCostPrice reports whether this line snapshot carried an explicit cost price.
func (l *OrderLine) HasCostPrice() bool {
	return l != nil && l.CostPrice != nil && l.CostPrice.IsPositive()
}

// EffectivePublicPrice returns the official retail price (سعر الجمهور).
func (l *OrderLine) EffectivePublicPrice() money.Amount {
	if l == nil {
		return money.Zero
	}
	if l.ListPrice.IsPositive() {
		return l.ListPrice
	}
	if l.CostPrice != nil && l.CostPrice.IsPositive() {
		return *l.CostPrice
	}
	if l.OriginalPrice.IsPositive() {
		return l.OriginalPrice
	}
	return l.UnitPrice
}

// EffectiveSellingPrice returns the actual unit selling price after discount.
func (l *OrderLine) EffectiveSellingPrice() money.Amount {
	if l == nil || l.Quantity <= 0 {
		return money.Zero
	}
	return money.FromMinor(l.TotalPrice.Minor() / int64(l.Quantity))
}

// EffectiveSellingDiscountPercent returns the selling discount percentage relative to public price.
func (l *OrderLine) EffectiveSellingDiscountPercent() float64 {
	pub := l.EffectivePublicPrice()
	if !pub.IsPositive() {
		return 0
	}
	sell := l.EffectiveSellingPrice()
	if sell.Minor() >= pub.Minor() {
		return 0
	}
	return (float64(pub.Minor()-sell.Minor()) / float64(pub.Minor())) * 100.0
}

// EffectivePurchaseCost calculates unit purchase cost:
// purchaseCost = publicPrice * (1 - costDiscountPercentage / 100).
func (l *OrderLine) EffectivePurchaseCost() money.Amount {
	if l == nil {
		return money.Zero
	}
	pub := l.EffectivePublicPrice()
	if pub.IsPositive() && l.CostDiscountPercentage > 0 {
		discMinor := int64(float64(pub.Minor()) * (l.CostDiscountPercentage / 100.0))
		return money.FromMinor(pub.Minor() - discMinor)
	}
	if l.HasCostPrice() {
		return *l.CostPrice
	}
	return money.Zero
}

// UnitDiscountedCost calculates the discounted unit cost price.
func (l *OrderLine) UnitDiscountedCost() money.Amount {
	return l.EffectivePurchaseCost()
}

// TotalCost calculates the total cost for this order line (Discounted Cost * Quantity).
func (l *OrderLine) TotalCost() money.Amount {
	if l == nil || l.Quantity <= 0 {
		return money.Zero
	}
	unitCost := l.EffectivePurchaseCost()
	if !unitCost.IsPositive() {
		return money.Zero
	}
	return money.FromMinor(unitCost.Minor() * int64(l.Quantity))
}

// TotalNetProfit computes the vendor's net profit for this line.
// net profit = TotalPrice - TotalCost.
func (l *OrderLine) TotalNetProfit() money.Amount {
	if l == nil {
		return money.Zero
	}
	totCost := l.TotalCost()
	if !totCost.IsPositive() {
		return l.TotalPrice
	}
	return money.FromMinor(l.TotalPrice.Minor() - totCost.Minor())
}

// CalculateAverageRating computes the exact 2-decimal scalar average of review criteria (audit §3.3).
func CalculateAverageRating(ratings ...int) float64 {
	if len(ratings) == 0 {
		return 0
	}
	sum := 0
	for _, r := range ratings {
		if r < 1 {
			r = 1
		} else if r > 5 {
			r = 5
		}
		sum += r
	}
	cents := (sum*100 + len(ratings)/2) / len(ratings)
	return float64(cents) / 100.0
}
