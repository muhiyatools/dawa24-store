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

type VendorShipmentFinancialSummary struct {
	TotalPublicPrice money.Amount // إجمالي سعر الجمهور
	TotalDiscount    money.Amount // الخصم الممنوح
	ShippingFee      money.Amount // تكاليف الشحن
	NetTotal         money.Amount // السعر الصافي
	TotalCost        money.Amount // التكلفة
	TotalProfit      money.Amount // الربح التقديري
	MarginPct        float64
}

func computeVendorShipmentFinancialSummary(sh *commerce.OrderShipment) VendorShipmentFinancialSummary {
	if sh == nil {
		return VendorShipmentFinancialSummary{}
	}
	var (
		grossMinor    int64
		netItemsMinor int64
		costMinor     int64
	)
	for _, l := range sh.Lines {
		if l == nil {
			continue
		}
		qty := int64(l.Quantity)
		grossMinor += l.UnitPrice.Minor() * qty
		netItemsMinor += l.TotalPrice.Minor()
		if l.CostPrice != nil {
			costMinor += l.CostPrice.Minor() * qty
		}
	}
	discountMinor := grossMinor - netItemsMinor
	if discountMinor < 0 {
		discountMinor = 0
	}
	shippingMinor := sh.ShippingFee.Minor()
	netTotalMinor := sh.TotalAmount.Minor()
	if netTotalMinor == 0 {
		netTotalMinor = netItemsMinor + shippingMinor
	}
	profitMinor := netTotalMinor - costMinor
	if profitMinor < 0 {
		profitMinor = 0
	}
	var marginPct float64
	if netTotalMinor > 0 {
		marginPct = (float64(profitMinor) / float64(netTotalMinor)) * 100
	}
	return VendorShipmentFinancialSummary{
		TotalPublicPrice: money.FromMinor(grossMinor),
		TotalDiscount:    money.FromMinor(discountMinor),
		ShippingFee:      sh.ShippingFee,
		NetTotal:         money.FromMinor(netTotalMinor),
		TotalCost:        money.FromMinor(costMinor),
		TotalProfit:      money.FromMinor(profitMinor),
		MarginPct:        marginPct,
	}
}

func computeVendorShipmentFinancials(sh *commerce.OrderShipment) (totalCost money.Amount, totalProfit money.Amount, marginPct float64) {
	s := computeVendorShipmentFinancialSummary(sh)
	return s.TotalCost, s.TotalProfit, s.MarginPct
}
