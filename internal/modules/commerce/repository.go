package commerce

import (
	"context"

	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// Repository defines the storage contract for cart, order, and shipment operations.
type Repository interface {
	GetOrCreateCart(ctx context.Context, userID int64) (*Cart, error)
	GetCartWithItems(ctx context.Context, cartID int64) (*Cart, error)
	AddToCartItem(ctx context.Context, cartID int64, item *CartItem) error
	// SetCartItemQuantity changes quantity in place, preserving the unit price
	// captured when the item was added. Removing and re-adding would re-read
	// the current price, so a vendor price change between the two calls would
	// silently reprice the basket under the customer.
	SetCartItemQuantity(ctx context.Context, cartID int64, variantID int64, quantity int) error
	RemoveCartItem(ctx context.Context, cartID int64, variantID int64) error
	RemoveCartItemByID(ctx context.Context, cartID, itemID int64) error
	SetCartItemQuantityByID(ctx context.Context, cartID, itemID int64, qty int) error
	ClearCart(ctx context.Context, cartID int64) error

	CreateOrder(ctx context.Context, order *Order, shipments []*OrderShipment, lines []*OrderLine) error
	GetOrderByID(ctx context.Context, id int64) (*Order, error)
	GetOrderByNumber(ctx context.Context, number string) (*Order, error)
	UpdateOrderStatus(ctx context.Context, orderID int64, toStatus OrderStatus, history OrderStatusHistory) error
	UpdateCustomerPendingOrder(ctx context.Context, order *Order, lines []OrderLineEditItem, changedByUserID int64) (*Order, error)
	ListOrdersByCustomer(ctx context.Context, customerID int64, limit, offset int) ([]*Order, error)
	CountOrders(ctx context.Context) (int, error)
	CountVendorShipmentsByStatus(ctx context.Context, orgID int64, statuses []string) (int, error)
	// MonthSalesByVendor sums a vendor's sold order-line totals for the current
	// month, for the supplier dashboard's "sales this month" metric.
	MonthSalesByVendor(ctx context.Context, vendorOrgID int64) (money.Amount, error)
	// MonthSpendByCustomer sums what a buyer paid across order lines this month.
	MonthSpendByCustomer(ctx context.Context, customerID int64) (money.Amount, error)
	// GetVendorFinancialSummary computes the complete, unified financial and profit analytics for a vendor.
	GetVendorFinancialSummary(ctx context.Context, vendorOrgID int64, period string) (*VendorFinancialSummary, error)
	ListShipmentsByVendor(ctx context.Context, vendorOrgID int64, limit, offset int) ([]*OrderShipment, error)
	GetShipmentByID(ctx context.Context, id int64) (*OrderShipment, error)
	// UpdateShipmentStatus is a compare-and-swap on the expected prior status,
	// so two vendor staff acting at once cannot both advance the same shipment.
	UpdateShipmentStatus(ctx context.Context, id int64, from, to OrderStatus, history OrderStatusHistory) error
	SetShipmentTracking(ctx context.Context, id int64, carrier, tracking string) error
	GetShipmentForDeliveryByTracking(ctx context.Context, tracking string) (*OrderShipment, error)
	VerifyAndCompleteDelivery(ctx context.Context, shipmentID int64, deliveryCode string, notes string, collectedAmountMinor int64) (*OrderShipment, error)
	ListOrderHistory(ctx context.Context, orderID int64) ([]*OrderStatusHistory, error)
	RateOrder(ctx context.Context, orderID int64, customerID int64, rating float64, review string) error

	AddToWishlist(ctx context.Context, userID int64, productID int64) error
	RemoveFromWishlist(ctx context.Context, userID int64, productID int64) error
	ListWishlist(ctx context.Context, userID int64) ([]*WishlistItem, error)

	CreateQuoteRequest(ctx context.Context, q *QuoteRequest) error
	GetQuoteRequestByID(ctx context.Context, id int64) (*QuoteRequest, error)
	UpdateQuoteStatus(ctx context.Context, id int64, status QuoteStatus, quotePrice money.Amount, supplierNotes string) error
	ListQuoteRequestsByOrg(ctx context.Context, orgID int64, isVendor bool, limit, offset int) ([]*QuoteRequest, error)

	CreatePurchaseRequest(ctx context.Context, pr *PurchaseRequest, lines []*PurchaseRequestLine) error
	GetPurchaseRequestByID(ctx context.Context, id int64) (*PurchaseRequest, error)
	GetPurchaseRequestByNumber(ctx context.Context, number string) (*PurchaseRequest, error)
	ListPurchaseRequestsByCustomer(ctx context.Context, customerID int64, orgID *int64, status string, limit, offset int) ([]*PurchaseRequest, error)
	ListPurchaseRequestsByVendor(ctx context.Context, vendorOrgID int64, status string, limit, offset int) ([]*PurchaseRequest, error)
	CountPurchaseRequestsByCustomer(ctx context.Context, customerID int64, orgID *int64) (map[string]int, error)
	UpdatePurchaseRequestStatus(ctx context.Context, id int64, status PurchaseRequestStatus, vendorNotes string, responderID *int64) error
	UpdatePurchaseRequestLineOffer(ctx context.Context, lineID int64, price money.Amount, discount float64, status string) error

	AdminSearchOrders(ctx context.Context, query string, limit, offset int) ([]*Order, error)
	AcceptNegotiation(ctx context.Context, orderID int64, actorID int64) error
	RejectNegotiation(ctx context.Context, orderID int64, reason string, actorID int64) error
}
