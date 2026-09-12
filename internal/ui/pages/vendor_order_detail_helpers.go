package pages

import (
	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

type VendorOrderDetailData struct {
	Shipment   *commerce.OrderShipment
	History    []*commerce.OrderStatusHistory
	CanAssign  bool
	Couriers   []DeliveryCourierOption
	NoticeType string
	NoticeMsg  string
	Lang       string
	Dir        string
}

func computeVendorShipmentFinancials(sh *commerce.OrderShipment) (totalCost money.Amount, totalProfit money.Amount, marginPct float64) {
	if sh == nil {
		return money.Zero, money.Zero, 0
	}
	var costMinor int64
	for _, l := range sh.Lines {
		if l != nil && l.CostPrice != nil {
			costMinor += l.CostPrice.Minor() * int64(l.Quantity)
		}
	}
	totalCost = money.FromMinor(costMinor)
	profitMinor := sh.TotalAmount.Minor() - totalCost.Minor()
	if profitMinor < 0 {
		profitMinor = 0
	}
	totalProfit = money.FromMinor(profitMinor)
	if sh.TotalAmount.Minor() > 0 {
		marginPct = (float64(profitMinor) / float64(sh.TotalAmount.Minor())) * 100
	}
	return totalCost, totalProfit, marginPct
}
