package assistant

import (
	"context"
	"time"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// The read model the assistant answers from.
//
// Three properties make this safe to hand a language model, and all three are
// structural rather than conventional:
//
//  1. Every method takes the live authctx.Actor as its FIRST argument and
//     derives its scope from that. There is no organisation parameter to get
//     wrong, and nothing the model produces can influence who the caller is.
//
//  2. Every query runs inside db.InReadTx, which sets the Postgres GUC that
//     row-level security reads. A query that forgot its own WHERE clause
//     returns zero rows rather than a competitor's order book — the database
//     refuses, not the code.
//
//  3. Every list is paginated with a server-imposed ceiling. A model asking for
//     "all orders" gets a page and a cursor, so one careless question cannot
//     pull a year of trading into a prompt.
//
// The interface is deliberately read-only. There is no write method here, on
// any type, and adding one would be visible in review as exactly what it is.

// PageLimit is the largest number of rows any tool returns in one call.
//
// Twenty-five, and not more, because these rows are going into a prompt: a
// bigger page costs tokens on every subsequent turn of the conversation and
// buys an answer nobody reads. Anything larger is a job for the dashboard's own
// export.
const PageLimit = 25

// Page is one window over a result set.
type Page[T any] struct {
	Rows       []T  `json:"rows"`
	HasMore    bool `json:"has_more"`
	NextOffset int  `json:"next_offset,omitempty"`
	Total      int  `json:"total,omitempty"`
}

// DateRange bounds a query in time. A zero bound means unbounded.
type DateRange struct {
	From time.Time
	To   time.Time
}

// ---------------------------------------------------------------------------
// Shared row shapes
// ---------------------------------------------------------------------------

// Bucket is one group in an aggregate: a status, a month, a counterparty.
type Bucket struct {
	Key   string       `json:"key"`
	Label string       `json:"label"`
	Count int          `json:"count"`
	Total money.Amount `json:"total"`
}

// Aggregate is the answer to "how much, over what period, split how".
type Aggregate struct {
	Count   int          `json:"count"`
	Total   money.Amount `json:"total"`
	Average money.Amount `json:"average"`
	From    *time.Time   `json:"from,omitempty"`
	To      *time.Time   `json:"to,omitempty"`
	Buckets []Bucket     `json:"buckets,omitempty"`
}

// BranchRow is one branch of the caller's own organisation.
type BranchRow struct {
	ID     int64  `json:"-"`
	Handle string `json:"branch"`
	Name   string `json:"name"`
	Phone  string `json:"phone,omitempty"`
	City   string `json:"city,omitempty"`
	IsMain bool   `json:"is_main"`
	Status string `json:"status"`
}

// WalletTxRow is one movement on the wallet.
type WalletTxRow struct {
	Type        string       `json:"type"`
	Amount      money.Amount `json:"amount"`
	BalanceAter money.Amount `json:"balance_after"`
	Description string       `json:"description,omitempty"`
	At          time.Time    `json:"at"`
}

// WalletSummary is the balance and the recent movements behind it.
type WalletSummary struct {
	Currency string        `json:"currency"`
	Balance  money.Amount  `json:"balance"`
	Recent   []WalletTxRow `json:"recent"`
}

// SubscriptionSummary is the plan the organisation is on.
type SubscriptionSummary struct {
	PlanName      string     `json:"plan"`
	Status        string     `json:"status"`
	StartsAt      time.Time  `json:"starts_at"`
	ExpiresAt     time.Time  `json:"expires_at"`
	DaysRemaining int        `json:"days_remaining"`
	PriceMonth    string     `json:"price_month,omitempty"`
	RenewedAt     *time.Time `json:"renewed_at,omitempty"`
}

// ---------------------------------------------------------------------------
// Pharmacy (buyer) rows
// ---------------------------------------------------------------------------

// PurchaseOrderRow is one order this pharmacy placed.
type PurchaseOrderRow struct {
	ID            int64        `json:"-"`
	Handle        string       `json:"order"`
	Number        string       `json:"number"`
	Status        string       `json:"status"`
	PaymentStatus string       `json:"payment_status"`
	Subtotal      money.Amount `json:"subtotal"`
	Discount      money.Amount `json:"discount"`
	Shipping      money.Amount `json:"shipping"`
	Total         money.Amount `json:"total"`
	LineCount     int          `json:"line_count"`
	Suppliers     []string     `json:"suppliers,omitempty"`
	PlacedAt      time.Time    `json:"placed_at"`
}

// OrderLineRow is one item inside an order.
type OrderLineRow struct {
	ProductName string       `json:"product"`
	SKU         string       `json:"sku,omitempty"`
	Supplier    string       `json:"supplier,omitempty"`
	UnitPrice   money.Amount `json:"unit_price"`
	Quantity    int          `json:"quantity"`
	Discount    money.Amount `json:"discount"`
	Total       money.Amount `json:"total"`
}

// ShipmentRow is one supplier's portion of an order.
type ShipmentRow struct {
	Number       string       `json:"number"`
	Counterparty string       `json:"counterparty"`
	Status       string       `json:"status"`
	Total        money.Amount `json:"total"`
	Carrier      string       `json:"carrier,omitempty"`
	Tracking     string       `json:"tracking,omitempty"`
	ShippedAt    *time.Time   `json:"shipped_at,omitempty"`
	DeliveredAt  *time.Time   `json:"delivered_at,omitempty"`
}

// OrderDetail is one order with everything under it.
type OrderDetail struct {
	Order     PurchaseOrderRow `json:"order"`
	Lines     []OrderLineRow   `json:"lines"`
	Shipments []ShipmentRow    `json:"shipments"`
	Notes     string           `json:"notes,omitempty"`
}

// ProductSpendRow is what one product cost over a period.
type ProductSpendRow struct {
	ProductName string       `json:"product"`
	Supplier    string       `json:"supplier,omitempty"`
	Quantity    int          `json:"quantity"`
	Total       money.Amount `json:"total"`
	Orders      int          `json:"orders"`
}

// MarketProductRow is one buyable item as this pharmacy may see it.
type MarketProductRow struct {
	ID         int64        `json:"-"`
	Handle     string       `json:"product"`
	Name       string       `json:"name"`
	Supplier   string       `json:"supplier"`
	Price      money.Amount `json:"price"`
	Discount   money.Amount `json:"discount"`
	FinalPrice money.Amount `json:"final_price"`
	Unit       string       `json:"unit,omitempty"`
	Company    string       `json:"company,omitempty"`
	Scientific string       `json:"scientific_name,omitempty"`
}

// ---------------------------------------------------------------------------
// Vendor (seller) rows
// ---------------------------------------------------------------------------

// SupplyOrderRow is one shipment this vendor owes a pharmacy.
type SupplyOrderRow struct {
	ID          int64        `json:"-"`
	Handle      string       `json:"shipment"`
	Number      string       `json:"number"`
	OrderNumber string       `json:"order_number"`
	Buyer       string       `json:"buyer"`
	Status      string       `json:"status"`
	Total       money.Amount `json:"total"`
	LineCount   int          `json:"line_count"`
	PlacedAt    time.Time    `json:"placed_at"`
}

// SupplyOrderDetail is one shipment with its lines.
type SupplyOrderDetail struct {
	Shipment SupplyOrderRow `json:"shipment"`
	Lines    []OrderLineRow `json:"lines"`
	Branch   string         `json:"branch,omitempty"`
}

// VendorProductRow is one item in this vendor's own catalogue.
type VendorProductRow struct {
	ID         int64        `json:"-"`
	Handle     string       `json:"product"`
	Name       string       `json:"name"`
	SKU        string       `json:"sku,omitempty"`
	Price      money.Amount `json:"price"`
	Discount   money.Amount `json:"discount"`
	FinalPrice money.Amount `json:"final_price"`
	Status     string       `json:"status"`
	SoldTimes  int64        `json:"sold_times"`
	Stock      *int         `json:"stock,omitempty"`
}

// SoldProductRow is one product's sales over a period.
type SoldProductRow struct {
	ProductName string       `json:"product"`
	Quantity    int          `json:"quantity"`
	Revenue     money.Amount `json:"revenue"`
	Orders      int          `json:"orders"`
}

// LowStockRow is one stock line at or below its reorder threshold.
type LowStockRow struct {
	ProductName  string `json:"product"`
	VariantName  string `json:"variant,omitempty"`
	Warehouse    string `json:"warehouse"`
	Quantity     int    `json:"quantity"`
	MinThreshold int    `json:"min_threshold"`
}

// OfferRow is one promotional offer this vendor published.
type OfferRow struct {
	ID            int64        `json:"-"`
	Handle        string       `json:"offer"`
	Title         string       `json:"title"`
	DiscountType  string       `json:"discount_type"`
	DiscountValue string       `json:"discount_value"`
	StartsAt      time.Time    `json:"starts_at"`
	ExpiresAt     time.Time    `json:"expires_at"`
	Active        bool         `json:"active"`
	Views         int64        `json:"views"`
	Clicks        int64        `json:"clicks"`
	ProductCount  int          `json:"product_count"`
	MinOrderValue money.Amount `json:"min_order_value"`
}

// ---------------------------------------------------------------------------
// Admin rows
// ---------------------------------------------------------------------------

// OrganizationRow is one registered company.
type OrganizationRow struct {
	ID        int64     `json:"-"`
	Handle    string    `json:"organization"`
	Name      string    `json:"name"`
	Type      string    `json:"type"`
	Status    string    `json:"status"`
	City      string    `json:"city,omitempty"`
	CreatedAt time.Time `json:"registered_at"`
}

// PlatformSummary is the operator's headline numbers.
type PlatformSummary struct {
	Organizations   int          `json:"organizations"`
	Pharmacies      int          `json:"pharmacies"`
	Vendors         int          `json:"vendors"`
	PendingApproval int          `json:"pending_approval"`
	Users           int          `json:"users"`
	Orders          int          `json:"orders"`
	GMV             money.Amount `json:"gmv"`
	From            *time.Time   `json:"from,omitempty"`
	To              *time.Time   `json:"to,omitempty"`
}

// AIUsageRow is one organisation's AI consumption over a period.
type AIUsageRow struct {
	Organization string `json:"organization"`
	Feature      string `json:"feature,omitempty"`
	Calls        int    `json:"calls"`
	InputTokens  int64  `json:"input_tokens"`
	OutputTokens int64  `json:"output_tokens"`
	CostUSD      string `json:"cost_usd"`
}

// ---------------------------------------------------------------------------
// Queries
// ---------------------------------------------------------------------------

// OrderQuery filters an order or shipment listing.
type OrderQuery struct {
	Status        string
	PaymentStatus string
	Search        string
	Range         DateRange
	Offset        int
	Limit         int
}

// GroupBy names how an aggregate is split.
type GroupBy string

const (
	GroupNone         GroupBy = ""
	GroupByStatus     GroupBy = "status"
	GroupByMonth      GroupBy = "month"
	GroupByCounterpar GroupBy = "counterparty"
)

// AggregateQuery bounds and splits a total.
type AggregateQuery struct {
	Range   DateRange
	Group   GroupBy
	Status  string
	Limit   int
	Ranking string // "revenue" | "quantity", where a ranking applies
}

// ProductQuery filters a catalogue search.
type ProductQuery struct {
	Search string
	Status string
	Offset int
	Limit  int
}

// Reader is the assistant's whole view of the business. Read-only, actor-scoped,
// paginated. Implemented by assistant/postgres.
type Reader interface {
	// Shared across dashboards.
	Branches(ctx context.Context, actor authctx.Actor) ([]BranchRow, error)
	Wallet(ctx context.Context, actor authctx.Actor) (*WalletSummary, error)
	Subscription(ctx context.Context, actor authctx.Actor) (*SubscriptionSummary, error)

	// Pharmacy: what we bought.
	PurchaseOrders(ctx context.Context, actor authctx.Actor, q OrderQuery) (Page[PurchaseOrderRow], error)
	PurchaseOrderDetail(ctx context.Context, actor authctx.Actor, orderID int64) (*OrderDetail, error)
	PurchaseSummary(ctx context.Context, actor authctx.Actor, q AggregateQuery) (*Aggregate, error)
	PurchasedProducts(ctx context.Context, actor authctx.Actor, q AggregateQuery) (Page[ProductSpendRow], error)
	MarketProducts(ctx context.Context, actor authctx.Actor, q ProductQuery) (Page[MarketProductRow], error)

	// Vendor: what we sold.
	SupplyOrders(ctx context.Context, actor authctx.Actor, q OrderQuery) (Page[SupplyOrderRow], error)
	SupplyOrderDetail(ctx context.Context, actor authctx.Actor, shipmentID int64) (*SupplyOrderDetail, error)
	SalesSummary(ctx context.Context, actor authctx.Actor, q AggregateQuery) (*Aggregate, error)
	SoldProducts(ctx context.Context, actor authctx.Actor, q AggregateQuery) (Page[SoldProductRow], error)
	VendorProducts(ctx context.Context, actor authctx.Actor, q ProductQuery) (Page[VendorProductRow], error)
	LowStock(ctx context.Context, actor authctx.Actor, limit int) (Page[LowStockRow], error)
	Offers(ctx context.Context, actor authctx.Actor, activeOnly bool, limit int) (Page[OfferRow], error)

	// Admin: the platform, read-only and permission-gated.
	Organizations(ctx context.Context, actor authctx.Actor, q ProductQuery) (Page[OrganizationRow], error)
	PlatformOverview(ctx context.Context, actor authctx.Actor, r DateRange) (*PlatformSummary, error)
	AIUsage(ctx context.Context, actor authctx.Actor, r DateRange, limit int) (Page[AIUsageRow], error)
}
