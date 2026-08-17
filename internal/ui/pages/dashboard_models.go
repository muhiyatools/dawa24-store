package pages

import (
	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/modules/inventory"
	"github.com/muhiya/dawa24-store/internal/modules/promo"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// VendorDashboardData is the supplier dashboard view model.
type VendorDashboardData struct {
	ActiveProducts   int
	PendingShipments int
	MonthSales       money.Amount
	WalletBalance    money.Amount
	HasWallet        bool
	LowStockCount    int
	Shipments        []*commerce.OrderShipment
	LowStock         []*inventory.Stock
	Offers           []*promo.Offer
	UnreadQuotes     int
}

// PharmacyDashboardData is the pharmacy dashboard view model.
type PharmacyDashboardData struct {
	OpenOrders    int
	MonthSpend    money.Amount
	WalletBalance money.Amount
	HasWallet     bool
	Favorites     int
	ActiveOffers  int
	Orders        []*commerce.Order
	Offers        []*promo.Offer
}
