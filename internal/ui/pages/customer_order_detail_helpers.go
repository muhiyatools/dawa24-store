package pages

import (
	"strings"

	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

func reviewScoreOf(rev *org.Review) int {
	if rev == nil {
		return 5
	}
	return rev.Rating
}

func isStepDone(status commerce.OrderStatus, step int) bool {
	switch step {
	case 1:
		return true
	case 2:
		return status == commerce.StatusConfirmed || status == commerce.StatusProcessing || status == commerce.StatusShipped || status == commerce.StatusInTransit || status == commerce.StatusOutForDelivery || status == commerce.StatusDelivered || status == commerce.StatusCompleted
	case 3:
		return status == commerce.StatusShipped || status == commerce.StatusInTransit || status == commerce.StatusOutForDelivery || status == commerce.StatusDelivered || status == commerce.StatusCompleted
	case 4:
		return status == commerce.StatusDelivered || status == commerce.StatusCompleted
	default:
		return false
	}
}

func isStepActive(status commerce.OrderStatus, step int) bool {
	switch step {
	case 1:
		return status == commerce.StatusPending
	case 2:
		return status == commerce.StatusConfirmed || status == commerce.StatusProcessing
	case 3:
		return status == commerce.StatusShipped || status == commerce.StatusInTransit || status == commerce.StatusOutForDelivery
	case 4:
		return status == commerce.StatusDelivered || status == commerce.StatusCompleted
	default:
		return false
	}
}

func orderProgressClass(status commerce.OrderStatus) string {
	switch status {
	case commerce.StatusPending:
		return "w-10"
	case commerce.StatusConfirmed, commerce.StatusProcessing:
		return "w-40"
	case commerce.StatusShipped, commerce.StatusInTransit, commerce.StatusOutForDelivery:
		return "w-75"
	case commerce.StatusDelivered, commerce.StatusCompleted:
		return "w-100"
	default:
		return "w-10"
	}
}

func getAllOrderLines(order *commerce.Order) []*commerce.OrderLine {
	if order == nil {
		return nil
	}
	if len(order.Lines) > 0 {
		return order.Lines
	}
	var res []*commerce.OrderLine
	for _, sh := range order.Shipments {
		if sh != nil {
			res = append(res, sh.Lines...)
		}
	}
	return res
}

func unitDiscountMinor(l *commerce.OrderLine) int64 {
	if l == nil || l.Quantity <= 0 || l.DiscountAmount.IsZero() {
		return 0
	}
	return l.DiscountAmount.Minor() / int64(l.Quantity)
}

func minAllowedQty(l *commerce.OrderLine) int {
	if l == nil || l.MinOrderQty <= 1 {
		return 1
	}
	return l.MinOrderQty
}

func maxAllowedQty(l *commerce.OrderLine) int {
	if l == nil {
		return 99999
	}
	maxVal := 99999
	if l.MaxQtyPerOrder > 0 {
		maxVal = l.MaxQtyPerOrder
	}
	if l.AvailableStock > 0 && l.AvailableStock < maxVal {
		maxVal = l.AvailableStock
	}
	return maxVal
}

func orderTaxRate(order *commerce.Order) float64 {
	if order == nil || order.TaxAmount.IsZero() {
		return 0.0
	}
	taxable, _ := order.Subtotal.Sub(order.DiscountAmount)
	if taxable.IsPositive() {
		return float64(order.TaxAmount.Minor()) / float64(taxable.Minor())
	}
	return 0.0
}

func linePublicPrice(l *commerce.OrderLine) money.Amount {
	if l == nil {
		return money.Zero
	}
	if l.ListPrice.IsPositive() {
		return l.ListPrice
	}
	if l.UnitPrice.IsPositive() {
		return l.UnitPrice
	}
	if l.OriginalPrice.IsPositive() {
		return l.OriginalPrice
	}
	return money.Zero
}

func lineSupplyPrice(l *commerce.OrderLine) money.Amount {
	if l == nil || l.Quantity <= 0 {
		return money.Zero
	}
	return money.FromMinor(l.TotalPrice.Minor() / int64(l.Quantity))
}

func lineDiscountPercent(l *commerce.OrderLine) float64 {
	if l == nil {
		return 0
	}
	if l.Quantity > 0 && l.UnitPrice.IsPositive() && l.DiscountAmount.IsPositive() {
		totalRetail := float64(l.UnitPrice.Minor() * int64(l.Quantity))
		if totalRetail > 0 {
			return (float64(l.DiscountAmount.Minor()) / totalRetail) * 100.0
		}
	}
	if l.CostDiscountPercentage > 0 {
		return l.CostDiscountPercentage
	}
	pub := linePublicPrice(l)
	if pub.Minor() > l.UnitPrice.Minor() {
		return (float64(pub.Minor()-l.UnitPrice.Minor()) / float64(pub.Minor())) * 100.0
	}
	return 0
}

func isOrderLineAnOffer(l *commerce.OrderLine) bool {
	if l == nil {
		return false
	}
	if l.OfferProductID != nil && *l.OfferProductID > 0 {
		return true
	}
	ar := l.ProductName.Get("ar")
	en := l.ProductName.Get("en")
	return strings.Contains(ar, "عرض") || strings.Contains(ar, "باقة") || strings.Contains(en, "Offer") || strings.Contains(en, "Bundle")
}
