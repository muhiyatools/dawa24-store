package billing

import (
	"context"
	"log/slog"
	"time"

	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// Service manages digital wallets, ledger transactions, and plan entitlements.
type Service struct {
	repo Repository
	log  *slog.Logger
}

// NewService creates a new billing service.
func NewService(repo Repository, log *slog.Logger) *Service {
	return &Service{
		repo: repo,
		log:  log,
	}
}

// GetWallet retrieves or initializes a user's wallet with current balance.
func (s *Service) GetWallet(ctx context.Context, userID int64, currency string) (*Wallet, error) {
	return s.repo.GetOrCreateWallet(ctx, userID, currency)
}

// Deposit credits funds to a wallet.
func (s *Service) Deposit(
	ctx context.Context,
	userID int64,
	currency string,
	amount money.Amount,
	refType string,
	refID *int64,
	desc string,
) (*WalletTransaction, error) {
	if err := ValidateCreditAmount(amount); err != nil {
		return nil, err
	}

	wallet, err := s.repo.GetOrCreateWallet(ctx, userID, currency)
	if err != nil {
		return nil, err
	}

	tx, err := s.repo.RecordTransaction(ctx, wallet.ID, TxDeposit, amount, refType, refID, desc)
	if err != nil {
		return nil, err
	}

	s.log.InfoContext(ctx, "wallet credited", "wallet_id", wallet.ID, "amount", amount.String(), "balance", tx.BalanceAfter.String())
	return tx, nil
}

// Withdraw debits funds from a wallet, rejecting overdrafts.
func (s *Service) Withdraw(
	ctx context.Context,
	userID int64,
	currency string,
	amount money.Amount,
	refType string,
	refID *int64,
	desc string,
) (*WalletTransaction, error) {
	if err := ValidateCreditAmount(amount); err != nil {
		return nil, err
	}

	wallet, err := s.repo.GetOrCreateWallet(ctx, userID, currency)
	if err != nil {
		return nil, err
	}

	negDelta, err := money.Zero.Sub(amount)
	if err != nil {
		return nil, apperr.Internal(err)
	}

	tx, err := s.repo.RecordTransaction(ctx, wallet.ID, TxWithdrawal, negDelta, refType, refID, desc)
	if err != nil {
		return nil, err
	}

	s.log.InfoContext(ctx, "wallet debited", "wallet_id", wallet.ID, "amount", amount.String(), "balance", tx.BalanceAfter.String())
	return tx, nil
}

// ListPlans returns all active subscription plans.
func (s *Service) ListPlans(ctx context.Context) ([]*Plan, error) {
	return s.repo.ListPlans(ctx)
}

// Subscribe grants a user a subscription to a plan tier.
func (s *Service) Subscribe(
	ctx context.Context,
	userID int64,
	orgID *int64,
	planSlug string,
	sourceSystem string,
	sourceID *int64,
) (*Subscription, error) {
	plan, err := s.repo.GetPlanBySlug(ctx, planSlug)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	duration := time.Duration(plan.DurationDays) * 24 * time.Hour
	if duration <= 0 {
		duration = 30 * 24 * time.Hour
	}

	sub := &Subscription{
		UserID:         userID,
		OrganizationID: orgID,
		PlanID:         plan.ID,
		Status:         SubActive,
		StartsAt:       now,
		ExpiresAt:      now.Add(duration),
		SourceSystem:   sourceSystem,
		SourceID:       sourceID,
	}

	if err := s.repo.CreateSubscription(ctx, sub); err != nil {
		return nil, err
	}

	s.log.InfoContext(ctx, "subscription activated", "user_id", userID, "plan", planSlug, "expires", sub.ExpiresAt)
	return sub, nil
}

// CheckEntitlement resolves whether a user has access to a specific feature key.
func (s *Service) CheckEntitlement(ctx context.Context, userID int64, featureKey string) (bool, string, error) {
	return s.repo.CheckEntitlement(ctx, userID, featureKey)
}
