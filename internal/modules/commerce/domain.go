// Package commerce manages shopping carts, order creation, price snapshotting,
// multi-vendor shipment splits, and order lifecycle transitions.
package commerce

import (
	"fmt"
	"time"

	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// OrderStatus defines the canonical lifecycle states for customer orders and vendor shipments.
type OrderStatus string

const (
	StatusPending        OrderStatus = "pending"
	StatusConfirmed      OrderStatus = "confirmed"
	StatusProcessing     OrderStatus = "processing"
	StatusReadyForPickup OrderStatus = "ready_for_pickup"
	StatusShipped        OrderStatus = "shipped"
	StatusDelivered      OrderStatus = "delivered"
	StatusCancelled      OrderStatus = "cancelled"
	StatusRefunded       OrderStatus = "refunded"
)

// PaymentStatus tracks payment authorization and settlement states.
type PaymentStatus string

const (
	PaymentUnpaid            PaymentStatus = "unpaid"
	PaymentAuthorized        PaymentStatus = "authorized"
	PaymentPaid              PaymentStatus = "paid"
	PaymentPartiallyRefunded PaymentStatus = "partially_refunded"
	PaymentRefunded          PaymentStatus = "refunded"
	PaymentFailed            PaymentStatus = "failed"
)

// Cart represents an active shopping cart for a user.
type Cart struct {
	ID             int64       `json:"id"`
	PublicID       string      `json:"public_id"`
	UserID         int64       `json:"user_id"`
	OrganizationID *int64      `json:"organization_id,omitempty"`
	Items          []*CartItem `json:"items,omitempty"`
	CreatedAt      time.Time   `json:"created_at"`
	UpdatedAt      time.Time   `json:"updated_at"`
}

// CartItem represents an item within a shopping cart.
type CartItem struct {
	ID               int64        `json:"id"`
	CartID           int64        `json:"cart_id"`
	ProductID        int64        `json:"product_id"`
	ProductVariantID int64        `json:"product_variant_id"`
	Quantity         int          `json:"quantity"`
	UnitPrice        money.Amount `json:"unit_price"`
	CreatedAt        time.Time    `json:"created_at"`
	UpdatedAt        time.Time    `json:"updated_at"`
}

// Order represents a master customer order spanning one or more vendor shipments.
type Order struct {
	ID             int64            `json:"id"`
	PublicID       string           `json:"public_id"`
	OrderNumber    string           `json:"order_number"`
	CustomerID     int64            `json:"customer_id"`
	OrganizationID *int64           `json:"organization_id,omitempty"`
	Status         OrderStatus      `json:"status"`
	Subtotal       money.Amount     `json:"subtotal"`
	DiscountAmount money.Amount     `json:"discount_amount"`
	ShippingFee    money.Amount     `json:"shipping_fee"`
	TaxAmount      money.Amount     `json:"tax_amount"`
	TotalAmount    money.Amount     `json:"total_amount"`
	PaymentMethod  string           `json:"payment_method"`
	PaymentStatus  PaymentStatus    `json:"payment_status"`
	Notes          string           `json:"notes,omitempty"`
	Shipments      []*OrderShipment `json:"shipments,omitempty"`
	CreatedAt      time.Time        `json:"created_at"`
	UpdatedAt      time.Time        `json:"updated_at"`
	DeletedAt      *time.Time       `json:"deleted_at,omitempty"`
}

// OrderShipment represents a vendor-specific shipment split from a master order.
type OrderShipment struct {
	ID             int64        `json:"id"`
	PublicID       string       `json:"public_id"`
	OrderID        int64        `json:"order_id"`
	OrganizationID int64        `json:"organization_id"`
	BranchID       *int64       `json:"branch_id,omitempty"`
	ShipmentNumber string       `json:"shipment_number"`
	Status         OrderStatus  `json:"status"`
	Subtotal       money.Amount `json:"subtotal"`
	ShippingFee    money.Amount `json:"shipping_fee"`
	TotalAmount    money.Amount `json:"total_amount"`
	TrackingNumber string       `json:"tracking_number,omitempty"`
	CarrierName    string       `json:"carrier_name,omitempty"`
	Lines          []*OrderLine `json:"lines,omitempty"`
	ShippedAt      *time.Time   `json:"shipped_at,omitempty"`
	DeliveredAt    *time.Time   `json:"delivered_at,omitempty"`
	CreatedAt      time.Time    `json:"created_at"`
	UpdatedAt      time.Time    `json:"updated_at"`
}

// OrderLine is an immutable snapshot of a product variant purchased in an order.
type OrderLine struct {
	ID               int64        `json:"id"`
	OrderID          int64        `json:"order_id"`
	ShipmentID       int64        `json:"shipment_id"`
	OrganizationID   int64        `json:"organization_id"`
	ProductID        *int64       `json:"product_id,omitempty"`
	ProductVariantID *int64       `json:"product_variant_id,omitempty"`
	ProductName      i18n.Text    `json:"product_name"`
	VariantName      i18n.Text    `json:"variant_name,omitempty"`
	SKU              string       `json:"sku,omitempty"`
	UnitPrice        money.Amount `json:"unit_price"`
	Quantity         int          `json:"quantity"`
	DiscountAmount   money.Amount `json:"discount_amount"`
	TotalPrice       money.Amount `json:"total_price"`
	CreatedAt        time.Time    `json:"created_at"`
}

// OrderStatusHistory logs every transition of order/shipment status.
type OrderStatusHistory struct {
	ID              int64     `json:"id"`
	OrderID         int64     `json:"order_id"`
	ShipmentID      *int64    `json:"shipment_id,omitempty"`
	FromStatus      *string   `json:"from_status,omitempty"`
	ToStatus        string    `json:"to_status"`
	Notes           string    `json:"notes,omitempty"`
	ChangedByUserID *int64    `json:"changed_by_user_id,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
}

// IsValidStatusTransition validates order state machine transitions.
func IsValidStatusTransition(from, to OrderStatus) bool {
	if from == to {
		return true
	}
	switch from {
	case StatusPending:
		return to == StatusConfirmed || to == StatusCancelled
	case StatusConfirmed:
		return to == StatusProcessing || to == StatusCancelled
	case StatusProcessing:
		return to == StatusReadyForPickup || to == StatusShipped || to == StatusCancelled
	case StatusReadyForPickup:
		return to == StatusShipped || to == StatusDelivered || to == StatusCancelled
	case StatusShipped:
		return to == StatusDelivered || to == StatusRefunded
	case StatusDelivered:
		return to == StatusRefunded
	case StatusCancelled, StatusRefunded:
		return false // Terminal states
	default:
		return false
	}
}

// GenerateOrderNumber formats a standardized order identifier.
func GenerateOrderNumber(t time.Time, id int64) string {
	return fmt.Sprintf("ORD-%s-%06d", t.Format("20060102"), id%1000000)
}

// GenerateShipmentNumber formats a shipment partition identifier.
func GenerateShipmentNumber(orderNumber string, seq int) string {
	return fmt.Sprintf("%s-S%d", orderNumber, seq)
}

// ValidateLine ensures an order line has positive quantity and consistent totals.
func (l *OrderLine) ValidateLine() error {
	if l.Quantity <= 0 {
		return apperr.Validation("line.quantity_invalid", "Quantity must be greater than zero.", nil)
	}
	if l.UnitPrice.IsNegative() {
		return apperr.Validation("line.price_invalid", "Unit price cannot be negative.", nil)
	}
	return nil
}

// WishlistItem represents an item in a user's wishlist / favorites.
type WishlistItem struct {
	ID        int64     `json:"id"`
	PublicID  string    `json:"public_id"`
	UserID    int64     `json:"user_id"`
	ProductID int64     `json:"product_id"`
	CreatedAt time.Time `json:"created_at"`
}
