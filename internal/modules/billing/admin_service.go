package billing

import (
	"context"

	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// AdminListSubscriptions provides cross-tenant subscription listing.
func (s *Service) AdminListSubscriptions(ctx context.Context, limit, offset int) ([]*Subscription, error) {
	return s.repo.AdminListSubscriptions(ctx, limit, offset)
}

// AdminAdjustWallet adjusts a wallet's balance.
func (s *Service) AdminAdjustWallet(ctx context.Context, walletID int64, amount money.Amount, reason string, actorID int64) error {
	return s.repo.AdminAdjustWallet(ctx, walletID, amount, reason, actorID)
}

// AdminListPayments provides cross-tenant payment listing.
func (s *Service) AdminListPayments(ctx context.Context, limit, offset int) ([]*Payment, error) {
	return s.repo.AdminListPayments(ctx, limit, offset)
}

// AdminListInvoices provides cross-tenant invoice listing.
func (s *Service) AdminListInvoices(ctx context.Context, limit, offset int) ([]*Invoice, error) {
	return s.repo.AdminListInvoices(ctx, limit, offset)
}

// AdminListWallets provides cross-tenant wallet listing with computed balances.
func (s *Service) AdminListWallets(ctx context.Context, limit, offset int) ([]*Wallet, error) {
	return s.repo.AdminListWallets(ctx, limit, offset)
}

// EnsureAllOrgWallets guarantees all registered organizations have wallet rows.
func (s *Service) EnsureAllOrgWallets(ctx context.Context) error {
	return s.repo.EnsureAllOrgWallets(ctx)
}

// AdminListDetailedWallets returns all wallets enriched with user and organization metadata.
func (s *Service) AdminListDetailedWallets(ctx context.Context, filter WalletFilter) ([]*AdminWalletView, int, error) {
	return s.repo.AdminListDetailedWallets(ctx, filter)
}

// AdminListDetailedTransactions returns wallet transactions with filters and metadata.
func (s *Service) AdminListDetailedTransactions(ctx context.Context, filter TransactionFilter) ([]*AdminWalletTransactionView, int, error) {
	return s.repo.AdminListDetailedTransactions(ctx, filter)
}

// AdminListDetailedInvoices returns invoices enriched with legal names and order details.
func (s *Service) AdminListDetailedInvoices(ctx context.Context, filter InvoiceFilter) ([]*AdminInvoiceView, int, error) {
	return s.repo.AdminListDetailedInvoices(ctx, filter)
}

// AdminListDetailedPayments returns payments enriched with order and tenant details.
func (s *Service) AdminListDetailedPayments(ctx context.Context, filter PaymentFilter) ([]*AdminPaymentView, int, error) {
	return s.repo.AdminListDetailedPayments(ctx, filter)
}

// AdminPerformWalletAdjustment executes a deposit, withdrawal, or balance adjustment.
func (s *Service) AdminPerformWalletAdjustment(ctx context.Context, walletID int64, amount money.Amount, txType TransactionType, reason string, actorID int64) error {
	return s.repo.AdminPerformWalletAdjustment(ctx, walletID, amount, txType, reason, actorID)
}
