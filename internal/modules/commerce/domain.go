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

// OrderStatus defines the canonical lifecycle states, matching Laravel's
// main_orders/adv_orders enum exactly (Rebuild V2 §3.3).
type OrderStatus string

const (
	StatusPending        OrderStatus = "pending"
	StatusProcessing     OrderStatus = "processing"
	StatusConfirmed      OrderStatus = "confirmed"
	StatusOnHold         OrderStatus = "on_hold"
	StatusShipped        OrderStatus = "shipped"
	StatusInTransit      OrderStatus = "in_transit"
	StatusOutForDelivery OrderStatus = "out_for_delivery"
	StatusDelivered      OrderStatus = "delivered"
	StatusCompleted      OrderStatus = "completed"
	StatusCancelled      OrderStatus = "cancelled"
	StatusFailed         OrderStatus = "failed"
	StatusReturned       OrderStatus = "returned"
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
	OrganizationID   int64        `json:"organization_id"`
	ProductName      i18n.Text    `json:"product_name,omitempty"`
	SupplierName     i18n.Text    `json:"supplier_name,omitempty"`
	MinOrderPrice    money.Amount `json:"min_order_price,omitempty"`
	OfferID          *int64       `json:"offer_id,omitempty"` // offer the item was added under (064)
	Quantity         int          `json:"quantity"`
	AvailableStock   int          `json:"available_stock,omitempty"`
	UnitPrice        money.Amount `json:"unit_price"`
	IsCovered        bool         `json:"is_covered,omitempty"`
	CoverageReason   string       `json:"coverage_reason,omitempty"`
	CreatedAt        time.Time    `json:"created_at"`
	UpdatedAt        time.Time    `json:"updated_at"`
}

// Order represents a master customer order placed against one offer
// (main_orders parity, Rebuild V2 §3.3).
type Order struct {
	ID                int64            `json:"id"`
	PublicID          string           `json:"public_id"`
	OrderNumber       string           `json:"order_number"`
	CustomerID        int64            `json:"customer_id"`
	OrganizationID    *int64           `json:"organization_id,omitempty"`
	OfferID           int64            `json:"offer_id"`                   // the offer this order belongs to (063)
	BranchID          *int64           `json:"branch_id,omitempty"`        // customer branch buying for
	VendorBranchID    *int64           `json:"vendor_branch_id,omitempty"` // fulfilling vendor branch
	UserAddressID     *int64           `json:"user_address_id,omitempty"`
	Status            OrderStatus      `json:"status"`
	Subtotal          money.Amount     `json:"subtotal"`
	DiscountAmount    money.Amount     `json:"discount_amount"`
	TotalDiscount     money.Amount     `json:"total_discount"` // offer discounts over lines (063)
	ShippingFee       money.Amount     `json:"shipping_fee"`
	TaxAmount         money.Amount     `json:"tax_amount"`
	TotalAmount       money.Amount     `json:"total_amount"`
	FinalPrice        money.Amount     `json:"final_price"` // paid after discount (063)
	PaymentMethod     string           `json:"payment_method"`
	PaymentStatus     PaymentStatus    `json:"payment_status"`
	Notes             string           `json:"notes,omitempty"`
	IsNegotiation     bool             `json:"is_negotiation"`
	NegotiationStatus string           `json:"negotiation_status"` // "none", "pending", "accepted", "rejected"
	NegotiationNotes  string           `json:"negotiation_notes,omitempty"`
	Rating            *float64         `json:"rating,omitempty"`
	Review            *string          `json:"review,omitempty"`
	RatedAt           *time.Time       `json:"rated_at,omitempty"`
	DeliveredAt       *time.Time       `json:"delivered_at,omitempty"`
	Shipments         []*OrderShipment `json:"shipments,omitempty"`
	Lines             []*OrderLine     `json:"lines,omitempty"`
	CreatedAt         time.Time        `json:"created_at"`
	UpdatedAt         time.Time        `json:"updated_at"`
	DeletedAt         *time.Time       `json:"deleted_at,omitempty"`

	// Enriched metadata for views
	CustomerOrgName       i18n.Text `json:"customer_org_name,omitempty"`
	CustomerBranchName    i18n.Text `json:"customer_branch_name,omitempty"`
	CustomerBranchAddress string    `json:"customer_branch_address,omitempty"`
	CustomerBranchPhone   string    `json:"customer_branch_phone,omitempty"`
	CustomerManagerName   string    `json:"customer_manager_name,omitempty"`
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

	// Enriched metadata for views
	OrderNumber           string        `json:"order_number,omitempty"`
	VendorName            i18n.Text     `json:"vendor_name,omitempty"`
	CustomerOrgName       i18n.Text     `json:"customer_org_name,omitempty"`
	CustomerBranchName    i18n.Text     `json:"customer_branch_name,omitempty"`
	CustomerBranchAddress string        `json:"customer_branch_address,omitempty"`
	CustomerBranchPhone   string        `json:"customer_branch_phone,omitempty"`
	CustomerManagerName   string        `json:"customer_manager_name,omitempty"`
	PaymentMethod         string        `json:"payment_method,omitempty"`
	PaymentStatus         PaymentStatus `json:"payment_status,omitempty"`
	Notes                 string        `json:"notes,omitempty"`
}

// OrderLine is an immutable snapshot of a product variant purchased in an order.
type OrderLine struct {
	ID                int64        `json:"id"`
	OrderID           int64        `json:"order_id"`
	ShipmentID        int64        `json:"shipment_id"`
	OrganizationID    int64        `json:"organization_id"`
	ProductID         *int64       `json:"product_id,omitempty"`
	ProductVariantID  *int64       `json:"product_variant_id,omitempty"`
	ProductName       i18n.Text    `json:"product_name"`
	VariantName       i18n.Text    `json:"variant_name,omitempty"`
	SKU               string       `json:"sku,omitempty"`
	OfferProductID    *int64       `json:"offer_product_id,omitempty"` // offer_product sold under (063)
	UnitPrice         money.Amount `json:"unit_price"`
	Quantity          int          `json:"quantity"`
	DiscountAmount    money.Amount `json:"discount_amount"`
	TotalPrice        money.Amount `json:"total_price"`
	ListPrice         money.Amount `json:"list_price,omitempty"`        // pre-discount strike price (063)
	OriginalPrice     money.Amount `json:"original_price,omitempty"`    // legacy price snapshot (063)
	OriginalDiscount  money.Amount `json:"original_discount,omitempty"` // legacy discount snapshot (063)
	IsNegotiated      bool         `json:"is_negotiated"`
	ProposedUnitPrice money.Amount `json:"proposed_unit_price,omitempty"`
	Rating            *float64     `json:"rating,omitempty"` // per-line rating (Laravel adv_orders.rating parity)
	CreatedAt         time.Time    `json:"created_at"`
}

// CalculateAverageRating computes the exact 2-decimal scalar average of review criteria (audit §3.3).
func CalculateAverageRating(ratings ...int) float64 {
	if len(ratings) == 0 {
		return 0
	}
	sum := 0
	for _, r := range ratings {
		if r < 1 {
			r = 1
		} else if r > 5 {
			r = 5
		}
		sum += r
	}
	cents := (sum*100 + len(ratings)/2) / len(ratings)
	return float64(cents) / 100.0
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

// OrderLineEditItem represents an item modification when a customer edits a pending order.
type OrderLineEditItem struct {
	ID             int64        `json:"id"` // 0 if new line
	ProductName    string       `json:"product_name"`
	Quantity       int          `json:"quantity"`
	UnitPrice      money.Amount `json:"unit_price"`
	DiscountAmount money.Amount `json:"discount_amount"`
	IsDeleted      bool         `json:"is_deleted"`
}

// UpdateCustomerOrderInput represents the payload submitted by a pharmacist to edit their pending order.
type UpdateCustomerOrderInput struct {
	OrderID int64               `json:"order_id"`
	Lines   []OrderLineEditItem `json:"lines"`
	Notes   string              `json:"notes,omitempty"`
}

// IsValidStatusTransition validates order state machine transitions. Compare-
// and-swap callers pass the status they read and the status they want; anything
// outside the DAG below is refused. Terminal states: cancelled, returned,
// failed, refunded, completed (only refundable afterwards).
func IsValidStatusTransition(from, to OrderStatus) bool {
	if from == to {
		return true
	}
	switch from {
	case StatusPending:
		return to == StatusProcessing || to == StatusConfirmed || to == StatusShipped || to == StatusDelivered || to == StatusCancelled || to == StatusOnHold
	case StatusProcessing:
		return to == StatusConfirmed || to == StatusShipped || to == StatusDelivered || to == StatusOnHold || to == StatusCancelled || to == StatusFailed
	case StatusConfirmed:
		return to == StatusShipped || to == StatusInTransit || to == StatusOutForDelivery || to == StatusDelivered || to == StatusOnHold || to == StatusCancelled || to == StatusFailed
	case StatusOnHold:
		return to == StatusProcessing || to == StatusConfirmed || to == StatusShipped || to == StatusCancelled || to == StatusFailed
	case StatusShipped:
		return to == StatusInTransit || to == StatusOutForDelivery || to == StatusDelivered || to == StatusCompleted || to == StatusReturned || to == StatusFailed
	case StatusInTransit:
		return to == StatusOutForDelivery || to == StatusDelivered || to == StatusCompleted || to == StatusReturned || to == StatusFailed
	case StatusOutForDelivery:
		return to == StatusDelivered || to == StatusCompleted || to == StatusReturned || to == StatusFailed
	case StatusDelivered:
		return to == StatusCompleted || to == StatusReturned || to == StatusRefunded
	case StatusCompleted:
		return to == StatusRefunded
	case StatusCancelled, StatusFailed, StatusReturned, StatusRefunded:
		return false // Terminal states
	default:
		return false
	}
}

// GenerateOrderNumber formats a clean, simplified, natural order identifier without prefixes.
// It produces short, human-friendly natural numbers (e.g. "2608270001" or "10542") that are easy to read and unique.
func GenerateOrderNumber(t time.Time, id int64) string {
	if id > 0 && id < 100000000 {
		return fmt.Sprintf("%d", id)
	}
	seq := id % 10000
	if seq < 0 {
		seq = -seq
	}
	return fmt.Sprintf("%s%04d", t.Format("060102"), seq)
}

// GenerateShipmentNumber formats a shipment partition identifier based on the order number.
func GenerateShipmentNumber(orderNumber string, seq int) string {
	return fmt.Sprintf("%s-%d", orderNumber, seq)
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

// QuoteStatus represents the state of a buyer quote request.
type QuoteStatus string

const (
	QuotePending  QuoteStatus = "pending"
	QuoteQuoted   QuoteStatus = "quoted"
	QuoteAccepted QuoteStatus = "accepted"
	QuoteRejected QuoteStatus = "rejected"
	QuoteExpired  QuoteStatus = "expired"
)

// QuoteRequest represents a buyer-initiated inquiry for bulk special pricing.
type QuoteRequest struct {
	ID                int64        `json:"id"`
	PublicID          string       `json:"public_id"`
	OrganizationID    int64        `json:"organization_id"`
	CustomerOrgID     int64        `json:"customer_org_id"`
	ProductID         *int64       `json:"product_id,omitempty"`
	ProductName       string       `json:"product_name"`
	RequestedQuantity int          `json:"requested_quantity"`
	TargetUnitPrice   money.Amount `json:"target_unit_price"`
	QuoteUnitPrice    money.Amount `json:"quote_unit_price"`
	Status            QuoteStatus  `json:"status"`
	BuyerNotes        string       `json:"buyer_notes,omitempty"`
	SupplierNotes     string       `json:"supplier_notes,omitempty"`
	ValidUntil        *time.Time   `json:"valid_until,omitempty"`
	CreatedAt         time.Time    `json:"created_at"`
	UpdatedAt         time.Time    `json:"updated_at"`
}

// PurchaseRequestStatus defines the state of a multi-line procurement purchase request (Plan V5 Phase 3 Task 3.1).
type PurchaseRequestStatus string

const (
	PurchaseRequestPending    PurchaseRequestStatus = "pending"
	PurchaseRequestApproved   PurchaseRequestStatus = "approved"
	PurchaseRequestProcessing PurchaseRequestStatus = "processing"
	PurchaseRequestCompleted  PurchaseRequestStatus = "completed"
	PurchaseRequestCancelled  PurchaseRequestStatus = "cancelled"
)

// PurchaseRequest represents a customer-initiated formal multi-line procurement purchase request.
type PurchaseRequest struct {
	ID             int64                  `json:"id"`
	PublicID       string                 `json:"public_id"`
	RequestNumber  string                 `json:"request_number"`
	CustomerID     int64                  `json:"customer_id"`
	OrganizationID *int64                 `json:"organization_id,omitempty"`
	BranchID       *int64                 `json:"branch_id,omitempty"`
	VendorOrgID    int64                  `json:"vendor_org_id"`
	VendorBranchID *int64                 `json:"vendor_branch_id,omitempty"`
	Status         PurchaseRequestStatus  `json:"status"`
	TotalItems     int                    `json:"total_items"`
	EstimatedTotal money.Amount           `json:"estimated_total"`
	BuyerNotes     string                 `json:"buyer_notes,omitempty"`
	VendorNotes    string                 `json:"vendor_notes,omitempty"`
	CreatedAt      time.Time              `json:"created_at"`
	UpdatedAt      time.Time              `json:"updated_at"`
	RespondedAt    *time.Time             `json:"responded_at,omitempty"`
	RespondedBy    *int64                 `json:"responded_by,omitempty"`
	Lines          []*PurchaseRequestLine `json:"lines,omitempty"`
	VendorName     string                 `json:"vendor_name,omitempty"`
	CustomerName   string                 `json:"customer_name,omitempty"`
}

// PurchaseRequestLine represents a specific item within a purchase request.
type PurchaseRequestLine struct {
	ID              int64        `json:"id"`
	RequestID       int64        `json:"request_id"`
	ProductID       *int64       `json:"product_id,omitempty"`
	ProductName     string       `json:"product_name"`
	ProductSKU      string       `json:"product_sku,omitempty"`
	Quantity        int          `json:"quantity"`
	TargetPrice     money.Amount `json:"target_price"`
	TargetDiscount  float64      `json:"target_discount"`
	OfferedPrice    money.Amount `json:"offered_price"`
	OfferedDiscount float64      `json:"offered_discount"`
	Status          string       `json:"status"` // pending, approved, rejected, modified
	Notes           string       `json:"notes,omitempty"`
	CreatedAt       time.Time    `json:"created_at"`
	UpdatedAt       time.Time    `json:"updated_at"`
}

// GeneratePurchaseRequestNumber formats a unique purchase request identifier (Rule R7 / Plan V5 §3.1).
func GeneratePurchaseRequestNumber(t time.Time, id int64) string {
	return fmt.Sprintf("PR-%s-%06d", t.Format("20060102"), id%1000000)
}
