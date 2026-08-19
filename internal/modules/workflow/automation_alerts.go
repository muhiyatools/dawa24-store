package workflow

import (
	"fmt"
)

// EvaluateAlerts inspects matched lines against user-defined alert thresholds.
func EvaluateAlerts(entries []MatchedProductEntry) []AutomationAlert {
	var alerts []AutomationAlert

	for _, entry := range entries {
		line := entry.RequestedLine
		offer := entry.BestOffer

		if offer == nil {
			alerts = append(alerts, AutomationAlert{
				RowIndex:    line.RowIndex,
				ProductName: line.ProductName,
				AlertType:   "unmatched_product",
				Message:     fmt.Sprintf("الصنف '%s' غير متوفر حالياً لدى الموردين المتوافقين.", line.ProductName),
			})
			continue
		}

		// 1. Price alert
		if line.TargetPrice != nil && line.TargetPrice.IsPositive() {
			if offer.FinalPrice.Minor() > line.TargetPrice.Minor() {
				alerts = append(alerts, AutomationAlert{
					RowIndex:    line.RowIndex,
					ProductName: line.ProductName,
					AlertType:   "price_exceeded",
					Message:     fmt.Sprintf("سعر الصنف (%s ج.م) أعلى من السعر المستهدف (%s ج.م).", offer.FinalPrice.String(), line.TargetPrice.String()),
				})
			}
		}

		// 2. Discount alert
		if line.TargetDiscount > 0 {
			if offer.Discount < line.TargetDiscount {
				alerts = append(alerts, AutomationAlert{
					RowIndex:    line.RowIndex,
					ProductName: line.ProductName,
					AlertType:   "discount_low",
					Message:     fmt.Sprintf("نسبة الخصم المتاحة (%.1f%%) أقل من الخصم المستهدف (%.1f%%).", offer.Discount, line.TargetDiscount),
				})
			}
		}

		// 3. Stock quantity alert
		if line.Quantity > offer.StockQuantity {
			alerts = append(alerts, AutomationAlert{
				RowIndex:    line.RowIndex,
				ProductName: line.ProductName,
				AlertType:   "stock_shortfall",
				Message:     fmt.Sprintf("الكمية المطلوبة (%d) أكبر من الرصيد المتوفر لدى المورد (%d).", line.Quantity, offer.StockQuantity),
			})
		}
	}

	return alerts
}
