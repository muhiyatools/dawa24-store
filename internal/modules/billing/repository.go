package billing

import (
	"context"

	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// Repository defines the storage contract for wallet transactions, payments, plans, and entitlements.
type Repository interface {
	GetOrCreateWallet(ctx context.Context, userID int64, currency string) (*Wallet, error)
	GetWallet(ctx context.Context, id int64) (*Wallet, error)
	RecordTransaction(ctx context.Context, walletID int64, txType TransactionType, delta money.Amount, refType string, refID *int64, desc string) (*WalletTransaction, error)
	ListTransactions(ctx context.Context, walletID int64, limit, offset int) ([]*WalletTransaction, error)

	CreatePayment(ctx context.Context, p *Payment) error
	GetPaymentByID(ctx context.Context, id int64) (*Payment, error)

	ListPlans(ctx context.Context) ([]*Plan, error)
	GetPlanByID(ctx context.Context, id int64) (*Plan, error)
	GetPlanBySlug(ctx context.Context, slug string) (*Plan, error)
	GetDefaultPlan(ctx context.Context) (*Plan, error)
	CreatePlan(ctx context.Context, p *Plan) error
	UpdatePlan(ctx context.Context, p *Plan) error
	CreateSubscription(ctx context.Context, sub *Subscription) error
	GetActiveSubscription(ctx context.Context, userID int64) (*Subscription, error)
	GetActiveSubscriptionByOrg(ctx context.Context, orgID int64) (*Subscription, error)
	CheckEntitlement(ctx context.Context, userID int64, featureKey string) (bool, string, error)

	CreateInvoice(ctx context.Context, inv *Invoice) error
	GetInvoiceByID(ctx context.Context, id int64) (*Invoice, error)
	GetInvoiceByOrderID(ctx context.Context, orderID int64) (*Invoice, error)
	UpdateInvoiceStatus(ctx context.Context, id int64, status InvoiceStatus) error
	ListInvoicesByOrg(ctx context.Context, orgID int64, limit, offset int) ([]*Invoice, error)

	AddPaymentMethod(ctx context.Context, pm *UserPaymentMethod) error
	ListPaymentMethods(ctx context.Context, userID int64) ([]*UserPaymentMethod, error)
	DeletePaymentMethod(ctx context.Context, userID, id int64) error

	ListPlatformPaymentMethods(ctx context.Context, onlyActive bool) ([]*PlatformPaymentMethod, error)
	GetPlatformPaymentMethod(ctx context.Context, id string) (*PlatformPaymentMethod, error)
	SavePlatformPaymentMethod(ctx context.Context, pm *PlatformPaymentMethod) error
	TogglePlatformPaymentMethod(ctx context.Context, id string, active bool) error
	DeletePlatformPaymentMethod(ctx context.Context, id string) error

	ListPaymentsByOrg(ctx context.Context, orgID int64, limit, offset int) ([]*Payment, error)

	AdminListSubscriptions(ctx context.Context, limit, offset int) ([]*Subscription, error)
	AdminAdjustWallet(ctx context.Context, walletID int64, amount money.Amount, reason string, actorID int64) error
	AdminPerformWalletAdjustment(ctx context.Context, walletID int64, amount money.Amount, txType TransactionType, reason string, actorID int64) error
	AdminListPayments(ctx context.Context, limit, offset int) ([]*Payment, error)
	EnsureAllOrgWallets(ctx context.Context) error
	AdminListDetailedWallets(ctx context.Context, filter WalletFilter) ([]*AdminWalletView, int, error)
	AdminListDetailedTransactions(ctx context.Context, filter TransactionFilter) ([]*AdminWalletTransactionView, int, error)
	AdminListDetailedInvoices(ctx context.Context, filter InvoiceFilter) ([]*AdminInvoiceView, int, error)
	AdminListDetailedPayments(ctx context.Context, filter PaymentFilter) ([]*AdminPaymentView, int, error)
}
