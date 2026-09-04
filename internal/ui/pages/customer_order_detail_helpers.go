package pages

import (
	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/modules/org"
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