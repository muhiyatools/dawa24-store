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
	ClearCart(ctx context.Context, cartID int64) error

	CreateOrder(ctx context.Context, order *Order, shipments []*OrderShipment, lines []*OrderLine) error
	GetOrderByID(ctx context.Context, id int64) (*Order, error)
	GetOrderByNumber(ctx context.Context, number string) (*Order, error)
	UpdateOrderStatus(ctx context.Context, orderID int64, toStatus OrderStatus, history OrderStatusHistory) error
	ListOrdersByCustomer(ctx context.Context, customerID int64, limit, offset int) ([]*Order, error)
	ListShipmentsByVendor(ctx context.Context, vendorOrgID int64, limit, offset int) ([]*OrderShipment, error)
	GetShipmentByID(ctx context.Context, id int64) (*OrderShipment, error)
	// UpdateShipmentStatus is a compare-and-swap on the expected prior status,
	// so two vendor staff acting at once cannot both advance the same shipment.
	UpdateShipmentStatus(ctx context.Context, id int64, from, to OrderStatus, history OrderStatusHistory) error
	ListOrderHistory(ctx context.Context, orderID int64) ([]*OrderStatusHistory, error)
	RateOrder(ctx context.Context, orderID int64, customerID int64, rating int, review string) error

	AddToWishlist(ctx context.Context, userID int64, productID int64) error
	RemoveFromWishlist(ctx context.Context, userID int64, productID int64) error
	ListWishlist(ctx context.Context, userID int64) ([]*WishlistItem, error)

	CreateQuoteRequest(ctx context.Context, q *QuoteRequest) error
	GetQuoteRequestByID(ctx context.Context, id int64) (*QuoteRequest, error)
	UpdateQuoteStatus(ctx context.Context, id int64, status QuoteStatus, quotePrice money.Amount, supplierNotes string) error
	ListQuoteRequestsByOrg(ctx context.Context, orgID int64, isVendor bool, limit, offset int) ([]*QuoteRequest, error)
}
