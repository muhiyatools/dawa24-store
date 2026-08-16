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
	GetPlanBySlug(ctx context.Context, slug string) (*Plan, error)
	CreateSubscription(ctx context.Context, sub *Subscription) error
	GetActiveSubscription(ctx context.Context, userID int64) (*Subscription, error)
	CheckEntitlement(ctx context.Context, userID int64, featureKey string) (bool, string, error)
}
