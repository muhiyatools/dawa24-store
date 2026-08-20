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
