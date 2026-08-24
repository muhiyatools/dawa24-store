// Package billing manages digital wallets, append-only transaction ledgers,
// payment records, subscription plans, and feature entitlements.
package billing

import (
	"time"

	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// TransactionType identifies categories of wallet balance changes.
type TransactionType string

const (
	TxDeposit     TransactionType = "deposit"
	TxWithdrawal  TransactionType = "withdrawal"
	TxPurchase    TransactionType = "purchase"
	TxRefund      TransactionType = "refund"
	TxBonus       TransactionType = "bonus"
	TxPenalty     TransactionType = "penalty"
	TxTransferIn  TransactionType = "transfer_in"
	TxTransferOut TransactionType = "transfer_out"
	TxAdjustment  TransactionType = "adjustment"
)

// SubscriptionStatus tracks subscription validity.
type SubscriptionStatus string

const (
	SubActive    SubscriptionStatus = "active"
	SubTrialing  SubscriptionStatus = "trialing"
	SubPastDue   SubscriptionStatus = "past_due"
	SubCancelled SubscriptionStatus = "cancelled"
	SubExpired   SubscriptionStatus = "expired"
)

// Wallet represents a tenant or user digital balance account.
type Wallet struct {
	ID             int64        `json:"id"`
	PublicID       string       `json:"public_id"`
	UserID         int64        `json:"user_id"`
	OrganizationID *int64       `json:"organization_id,omitempty"`
	Currency       string       `json:"currency"`
	Balance        money.Amount `json:"balance"` // Computed from ledger
	CreatedAt      time.Time    `json:"created_at"`
	UpdatedAt      time.Time    `json:"updated_at"`
}

// WalletTransaction is an immutable append-only ledger row.
type WalletTransaction struct {
	ID            int64           `json:"id"`
	WalletID      int64           `json:"wallet_id"`
	Type          TransactionType `json:"type"`
	Amount        money.Amount    `json:"amount"` // Signed: positive = credit, negative = debit
	BalanceAfter  money.Amount    `json:"balance_after"`
	ReferenceType string          `json:"reference_type,omitempty"`
	ReferenceID   *int64          `json:"reference_id,omitempty"`
	Description   string          `json:"description,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
}

// Payment records a monetary transaction via a payment method.
type Payment struct {
	ID                   int64        `json:"id"`
	PublicID             string       `json:"public_id"`
	PaymentIntegrationID *int64       `json:"payment_integration_id,omitempty"`
	OrderID              *int64       `json:"order_id,omitempty"`
	UserID               int64        `json:"user_id"`
	OrganizationID       *int64       `json:"organization_id,omitempty"`
	Amount               money.Amount `json:"amount"`
	Method               string       `json:"method"`
	Status               string       `json:"status"`
	TransactionID        string       `json:"transaction_id,omitempty"`
	ReferenceNumber      string       `json:"reference_number,omitempty"`
	PaidAt               *time.Time   `json:"paid_at,omitempty"`
	CreatedAt            time.Time    `json:"created_at"`
	UpdatedAt            time.Time    `json:"updated_at"`
}

// Plan represents a subscription tier with billing parameters, limits, and AI Gateway plan linkage.
type Plan struct {
	ID               int64             `json:"id"`
	PublicID         string            `json:"public_id"`
	Slug             string            `json:"slug"`
	Name             i18n.Text         `json:"name"`
	Description      i18n.Text         `json:"description,omitempty"`
	PriceMonth       money.Amount      `json:"price_month"`
	PriceYear        money.Amount      `json:"price_year"`
	DurationDays     int               `json:"duration_days"`
	MaxUsers         *int              `json:"max_users,omitempty"`
	MaxLoginSessions int               `json:"max_login_sessions"`
	MaxDevices       int               `json:"max_devices"`
	AIPlanID         string            `json:"ai_plan_id"`
	IsDefault        bool              `json:"is_default"`
	IsActive         bool              `json:"is_active"`
	Features         map[string]string `json:"features,omitempty"`
	CreatedAt        time.Time         `json:"created_at"`
	UpdatedAt        time.Time         `json:"updated_at"`
}

// Subscription represents an active plan subscription (unifying legacy D7 systems).
type Subscription struct {
	ID             int64              `json:"id"`
	PublicID       string             `json:"public_id"`
	UserID         int64              `json:"user_id"`
	OrganizationID *int64             `json:"organization_id,omitempty"`
	PlanID         int64              `json:"plan_id"`
	Status         SubscriptionStatus `json:"status"`
	StartsAt       time.Time          `json:"starts_at"`
	ExpiresAt      time.Time          `json:"expires_at"`
	SourceSystem   string             `json:"source_system"`
	SourceID       *int64             `json:"source_id,omitempty"`
	CreatedAt      time.Time          `json:"created_at"`
	UpdatedAt      time.Time          `json:"updated_at"`
}

// IsActiveAt checks whether the subscription is active at a given moment in time.
func (s *Subscription) IsActiveAt(now time.Time) bool {
	if s == nil || s.Status != SubActive {
		return false
	}
	return s.StartsAt.Before(now) && s.ExpiresAt.After(now)
}

// Validate ensures non-negative amounts for deposit and credit operations.
func ValidateCreditAmount(a money.Amount) error {
	if a.IsZero() || a.IsNegative() {
		return apperr.Validation("amount.invalid", "Amount must be strictly positive.", nil)
	}
	return nil
}

// InvoiceStatus tracks invoice lifecycle states.
type InvoiceStatus string

const (
	InvoiceDraft     InvoiceStatus = "draft"
	InvoiceIssued    InvoiceStatus = "issued"
	InvoicePaid      InvoiceStatus = "paid"
	InvoiceOverdue   InvoiceStatus = "overdue"
	InvoiceCancelled InvoiceStatus = "cancelled"
)

// Invoice represents a B2B commercial tax invoice.
type Invoice struct {
	ID             int64         `json:"id"`
	PublicID       string        `json:"public_id"`
	OrganizationID int64         `json:"organization_id"`
	CustomerOrgID  *int64        `json:"customer_org_id,omitempty"`
	OrderID        *int64        `json:"order_id,omitempty"`
	InvoiceNumber  string        `json:"invoice_number"`
	IssueDate      time.Time     `json:"issue_date"`
	DueDate        time.Time     `json:"due_date"`
	Subtotal       money.Amount  `json:"subtotal"`
	TaxAmount      money.Amount  `json:"tax_amount"`
	DiscountAmount money.Amount  `json:"discount_amount"`
	TotalAmount    money.Amount  `json:"total_amount"`
	Status         InvoiceStatus `json:"status"`
	PaymentMethod  string        `json:"payment_method"`
	Notes          string        `json:"notes,omitempty"`
	Lines          []InvoiceLine `json:"lines,omitempty"`
	CreatedAt      time.Time     `json:"created_at"`
	UpdatedAt      time.Time     `json:"updated_at"`
}

// InvoiceLine is a line item on an invoice.
type InvoiceLine struct {
	ID          int64        `json:"id"`
	InvoiceID   int64        `json:"invoice_id"`
	ProductID   *int64       `json:"product_id,omitempty"`
	Description string       `json:"description"`
	Quantity    int          `json:"quantity"`
	UnitPrice   money.Amount `json:"unit_price"`
	TotalPrice  money.Amount `json:"total_price"`
}

// UserPaymentMethod represents a stored payment method identifier.
type UserPaymentMethod struct {
	ID                int64     `json:"id"`
	PublicID          string    `json:"public_id"`
	UserID            int64     `json:"user_id"`
	Provider          string    `json:"provider"`
	AccountIdentifier string    `json:"account_identifier"`
	IsDefault         bool      `json:"is_default"`
	CreatedAt         time.Time `json:"created_at"`
}

// PlatformPaymentMethod represents a payment channel configured for the platform.
type PlatformPaymentMethod struct {
	ID                string    `json:"id"`                  // e.g. "bank_transfer", "instapay", "card", "vodafone_cash"
	Name              i18n.Text `json:"name"`                // Arabic & English name
	ProviderType      string    `json:"provider_type"`       // "bank", "instapay", "wallet", "card", "cash"
	Description       i18n.Text `json:"description"`         // Arabic & English guidelines
	AccountName       string    `json:"account_name"`        // Corporate account holder
	BankName          string    `json:"bank_name"`           // Official bank name
	AccountNumber     string    `json:"account_number"`      // Bank account number
	IBAN              string    `json:"iban"`                // Official IBAN
	SwiftCode         string    `json:"swift_code"`          // SWIFT / BIC code
	BranchName        string    `json:"branch_name"`         // Bank branch
	InstaPayHandle    string    `json:"instapay_handle"`     // IPA address e.g. dawa24@instapay
	PhoneNumber       string    `json:"phone_number"`        // Phone number for wallets / InstaPay
	IsActive          bool      `json:"is_active"`           // Globally active or disabled
	IsDepositEnabled  bool      `json:"is_deposit_enabled"`  // Allowed for wallet deposit
	IsCheckoutEnabled bool      `json:"is_checkout_enabled"` // Allowed for checkout orders
	DisplayOrder      int       `json:"display_order"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}
