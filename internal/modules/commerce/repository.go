package commerce

import (
	"context"
)

// Repository defines the storage contract for cart, order, and shipment operations.
type Repository interface {
	GetOrCreateCart(ctx context.Context, userID int64) (*Cart, error)
	GetCartWithItems(ctx context.Context, cartID int64) (*Cart, error)
	AddToCartItem(ctx context.Context, cartID int64, item *CartItem) error
	RemoveCartItem(ctx context.Context, cartID int64, variantID int64) error
	ClearCart(ctx context.Context, cartID int64) error

	CreateOrder(ctx context.Context, order *Order, shipments []*OrderShipment, lines []*OrderLine) error
	GetOrderByID(ctx context.Context, id int64) (*Order, error)
	GetOrderByNumber(ctx context.Context, number string) (*Order, error)
	UpdateOrderStatus(ctx context.Context, orderID int64, toStatus OrderStatus, history OrderStatusHistory) error
	ListOrdersByCustomer(ctx context.Context, customerID int64, limit, offset int) ([]*Order, error)
	ListShipmentsByVendor(ctx context.Context, vendorOrgID int64, limit, offset int) ([]*OrderShipment, error)

	AddToWishlist(ctx context.Context, userID int64, productID int64) error
	RemoveFromWishlist(ctx context.Context, userID int64, productID int64) error
	ListWishlist(ctx context.Context, userID int64) ([]*WishlistItem, error)
}
