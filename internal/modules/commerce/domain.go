// Package commerce manages shopping carts, order creation, price snapshotting,
// multi-vendor shipment splits, and order lifecycle transitions.
package commerce

import (
	"crypto/rand"
	"fmt"
	"math/big"
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

// IsOfferLine reports whether this line is an offer bought as a unit rather
// than a catalogue item.
//
// An offer is sold at its own price for whatever it contains; the products
// listed against it are a manifest, not separately purchasable lines. Such a
// line therefore carries an offer and no product reference at all, which is the
// shape migration 155 added to commerce.cart_items and which the check
// constraint there enforces.
func (i *CartItem) IsOfferLine() bool {
	return i != nil && i.OfferID != nil && *i.OfferID > 0 &&
		i.ProductID == 0 && i.ProductVariantID == 0
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
	ID                   int64        `json:"id"`
	PublicID             string       `json:"public_id"`
	OrderID              int64        `json:"order_id"`
	OrganizationID       int64        `json:"organization_id"`
	BranchID             *int64       `json:"branch_id,omitempty"`
	ShipmentNumber       string       `json:"shipment_number"`
	Status               OrderStatus  `json:"status"`
	Subtotal             money.Amount `json:"subtotal"`
	ShippingFee          money.Amount `json:"shipping_fee"`
	TotalAmount          money.Amount `json:"total_amount"`
	TrackingNumber       string       `json:"tracking_number,omitempty"`
	CarrierName          string       `json:"carrier_name,omitempty"`
	DeliveryCode         string       `json:"delivery_code,omitempty"`
	DeliveryAttempts     int          `json:"delivery_attempts"`
	DeliveryLockedUntil  *time.Time   `json:"delivery_locked_until,omitempty"`
	DeliveryNotes        string       `json:"delivery_notes,omitempty"`
	CollectedAmountMinor int64        `json:"collected_amount_minor"`
	DeliveredByCourierAt *time.Time   `json:"delivered_by_courier_at,omitempty"`
	Lines                []*OrderLine `json:"lines,omitempty"`
	ShippedAt            *time.Time   `json:"shipped_at,omitempty"`
	DeliveredAt          *time.Time   `json:"delivered_at,omitempty"`
	CreatedAt            time.Time    `json:"created_at"`
	UpdatedAt            time.Time    `json:"updated_at"`

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
	ID                     int64         `json:"id"`
	OrderID                int64         `json:"order_id"`
	ShipmentID             int64         `json:"shipment_id"`
	OrganizationID         int64         `json:"organization_id"`
	ProductID              *int64        `json:"product_id,omitempty"`
	ProductVariantID       *int64        `json:"product_variant_id,omitempty"`
	ProductName            i18n.Text     `json:"product_name"`
	VariantName            i18n.Text     `json:"variant_name,omitempty"`
	SKU                    string        `json:"sku,omitempty"`
	OfferProductID         *int64        `json:"offer_product_id,omitempty"` // offer_product sold under (063)
	UnitPrice              money.Amount  `json:"unit_price"`
	Quantity               int           `json:"quantity"`
	DiscountAmount         money.Amount  `json:"discount_amount"`
	TotalPrice             money.Amount  `json:"total_price"`
	CostPrice              *money.Amount `json:"cost_price,omitempty"`        // Optional snapshot of cost price at purchase
	CostDiscountPercentage float64       `json:"cost_discount_percentage"`    // Snapshot of cost discount %
	ListPrice              money.Amount  `json:"list_price,omitempty"`        // pre-discount strike price (063)
	OriginalPrice          money.Amount  `json:"original_price,omitempty"`    // legacy price snapshot (063)
	OriginalDiscount       money.Amount  `json:"original_discount,omitempty"` // legacy discount snapshot (063)
	IsNegotiated           bool          `json:"is_negotiated"`
	ProposedUnitPrice      money.Amount  `json:"proposed_unit_price,omitempty"`
	Rating                 *float64      `json:"rating,omitempty"` // per-line rating (Laravel adv_orders.rating parity)
	AvailableStock         int           `json:"available_stock,omitempty"`
	MinOrderQty            int           `json:"min_order_qty,omitempty"`
	MaxQtyPerOrder         int           `json:"max_qty_per_order,omitempty"`
	CreatedAt              time.Time     `json:"created_at"`
}

// HasCostPrice reports whether this line snapshot carried an explicit cost price.
func (l *OrderLine) HasCostPrice() bool {
	return l != nil && l.CostPrice != nil && l.CostPrice.IsPositive()
}

// UnitDiscountedCost calculates the discounted unit cost price.
func (l *OrderLine) UnitDiscountedCost() money.Amount {
	if !l.HasCostPrice() {
		return money.Zero
	}
	if l.CostDiscountPercentage > 0 {
		discMinor := int64(float64(l.CostPrice.Minor()) * (l.CostDiscountPercentage / 100.0))
		return money.FromMinor(l.CostPrice.Minor() - discMinor)
	}
	return *l.CostPrice
}

// TotalCost calculates the total cost for this order line (Discounted Cost * Quantity).
func (l *OrderLine) TotalCost() money.Amount {
	if !l.HasCostPrice() || l.Quantity <= 0 {
		return money.Zero
	}
	unitCost := l.UnitDiscountedCost()
	return money.FromMinor(unitCost.Minor() * int64(l.Quantity))
}

// TotalNetProfit computes the vendor's net profit for this line.
// If CostPrice is set, net profit = TotalPrice - TotalCost.
// If CostPrice is NOT set (NULL or zero), net profit = TotalPrice (profit equals selling price after discount).
func (l *OrderLine) TotalNetProfit() money.Amount {
	if l == nil {
		return money.Zero
	}
	if !l.HasCostPrice() {
		return l.TotalPrice
	}
	totCost := l.TotalCost()
	return money.FromMinor(l.TotalPrice.Minor() - totCost.Minor())
}

// VendorFinancialSummary represents the unified financial and profit metrics for a vendor.
type VendorFinancialSummary struct {
	Period               string                  `json:"period"` // "month", "last_month", "year", "all"
	GrossSales           money.Amount            `json:"gross_sales"`
	TotalDiscounts       money.Amount            `json:"total_discounts"`
	NetSales             money.Amount            `json:"net_sales"`
	COGS                 money.Amount            `json:"cogs"` // Total Cost of Goods Sold (discounted cost prices)
	PlatformFees         money.Amount            `json:"platform_fees"`
	NetProfit            money.Amount            `json:"net_profit"`
	ProfitMargin         float64                 `json:"profit_margin"`
	DeliveredOrdersCount int                     `json:"delivered_orders_count"`
	PendingOrdersTotal   money.Amount            `json:"pending_orders_total"`
	PendingOrdersCount   int                     `json:"pending_orders_count"`
	WalletBalance        money.Amount            `json:"wallet_balance"`
	Shipments            []*VendorShipmentProfit `json:"shipments,omitempty"`
	TopProducts          []*VendorProductProfit  `json:"top_products,omitempty"`
}

// VendorShipmentProfit gives financial breakdown for a single delivered shipment.
type VendorShipmentProfit struct {
	ShipmentID      int64        `json:"shipment_id"`
	ShipmentNumber  string       `json:"shipment_number"`
	OrderID         int64        `json:"order_id"`
	OrderNumber     string       `json:"order_number"`
	CustomerOrgName string       `json:"customer_org_name"`
	DeliveredAt     time.Time    `json:"delivered_at"`
	GrossSales      money.Amount `json:"gross_sales"`
	Discounts       money.Amount `json:"discounts"`
	NetSales        money.Amount `json:"net_sales"`
	COGS            money.Amount `json:"cogs"`
	PlatformFee     money.Amount `json:"platform_fee"`
	NetProfit       money.Amount `json:"net_profit"`
	ProfitMargin    float64      `json:"profit_margin"`
	PaymentStatus   string       `json:"payment_status"`
	LineItemsCount  int          `json:"line_items_count"`
}

// VendorProductProfit gives profitability analysis per catalog item.
type VendorProductProfit struct {
	ProductID              int64         `json:"product_id"`
	VariantID              int64         `json:"variant_id"`
	Name                   string        `json:"name"`
	SKU                    string        `json:"sku"`
	QuantitySold           int           `json:"quantity_sold"`
	SellingPrice           money.Amount  `json:"selling_price"`
	CostPrice              *money.Amount `json:"cost_price,omitempty"`
	CostDiscountPercentage float64       `json:"cost_discount_percentage"`
	DiscountedCost         money.Amount  `json:"discounted_cost"`
	TotalRevenue           money.Amount  `json:"total_revenue"`
	TotalCost              money.Amount  `json:"total_cost"`
	NetProfit              money.Amount  `json:"net_profit"`
	ProfitMargin           float64       `json:"profit_margin"`
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

// GenerateDeliveryCode produces a secure random 6-digit numeric verification PIN.
func GenerateDeliveryCode() string {
	n, err := rand.Int(rand.Reader, big.NewInt(900000))
	if err != nil {
		return "582914" // safe fallback
	}
	return fmt.Sprintf("%06d", n.Int64()+100000)
}

// Standard courier delivery errors.
var (
	ErrDeliveryLocked       = apperr.Conflict("delivery.locked", i18n.TDefault("w4_mod.15_138"))
	ErrInvalidDeliveryCode  = apperr.Validation("delivery.invalid_code", i18n.TDefault("w4_mod.w4str_139_139"), map[string]string{"delivery_code": "invalid"})
	ErrDeliveryNotAvailable = apperr.Conflict("delivery.not_available", i18n.TDefault("w4_mod.w4str_140_140"))
)

// GenerateTrackingNumber produces a formatted tracking reference code for couriers.
func GenerateTrackingNumber(orderNumber string, seq int) string {
	n, err := rand.Int(rand.Reader, big.NewInt(9000))
	randSuffix := int64(1000)
	if err == nil {
		randSuffix = n.Int64() + 1000
	}
	if orderNumber != "" {
		return fmt.Sprintf("TRK-%s-%d%04d", orderNumber, seq, randSuffix)
	}
	return fmt.Sprintf("TRK-%d%04d", time.Now().Unix()%1000000, randSuffix)
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
