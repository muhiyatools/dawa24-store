package billing

import (
	"context"
	"fmt"
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

// ListWalletTransactions returns the paginated ledger for a wallet.
func (s *Service) ListWalletTransactions(ctx context.Context, walletID int64, limit, offset int) ([]*WalletTransaction, error) {
	return s.repo.ListTransactions(ctx, walletID, limit, offset)
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

// CreatePlan adds a subscription tier (admin).
func (s *Service) CreatePlan(ctx context.Context, p *Plan) (*Plan, error) {
	if p.Slug == "" || p.Name.IsEmpty() {
		return nil, apperr.Validation("plan.slug_required", "A plan slug and name are required.", nil)
	}
	if err := s.repo.CreatePlan(ctx, p); err != nil {
		return nil, err
	}
	return p, nil
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

// CreateInvoice generates a new B2B invoice.
func (s *Service) CreateInvoice(ctx context.Context, inv *Invoice) (*Invoice, error) {
	if inv.OrganizationID <= 0 {
		return nil, apperr.Validation("invoice.org_required", "Organization ID is required.", nil)
	}
	if inv.InvoiceNumber == "" {
		inv.InvoiceNumber = fmt.Sprintf("INV-%d-%d", inv.OrganizationID, time.Now().Unix())
	}
	if inv.Status == "" {
		inv.Status = InvoiceDraft
	}

	var subtotal money.Amount
	for _, l := range inv.Lines {
		subtotal, _ = subtotal.Add(l.TotalPrice)
	}
	inv.Subtotal = subtotal
	total, _ := subtotal.Add(inv.TaxAmount)
	total, _ = total.Sub(inv.DiscountAmount)
	inv.TotalAmount = total

	if err := s.repo.CreateInvoice(ctx, inv); err != nil {
		return nil, err
	}

	s.log.InfoContext(ctx, "invoice created", "invoice_id", inv.ID, "number", inv.InvoiceNumber, "total", inv.TotalAmount.String())
	return inv, nil
}

// GetInvoice returns an invoice by ID.
func (s *Service) GetInvoice(ctx context.Context, id int64) (*Invoice, error) {
	return s.repo.GetInvoiceByID(ctx, id)
}

// ListInvoices lists invoices for an organization.
func (s *Service) ListInvoices(ctx context.Context, orgID int64, limit, offset int) ([]*Invoice, error) {
	return s.repo.ListInvoicesByOrg(ctx, orgID, limit, offset)
}

// MarkInvoicePaid updates invoice status to paid.
func (s *Service) MarkInvoicePaid(ctx context.Context, id int64) error {
	return s.repo.UpdateInvoiceStatus(ctx, id, InvoicePaid)
}

// AddPaymentMethod saves a user payment method.
func (s *Service) AddPaymentMethod(ctx context.Context, pm *UserPaymentMethod) error {
	if pm.UserID <= 0 || pm.Provider == "" || pm.AccountIdentifier == "" {
		return apperr.Validation("payment_method.invalid", "User ID, provider, and account identifier are required.", nil)
	}
	return s.repo.AddPaymentMethod(ctx, pm)
}

// ListPaymentMethods returns saved payment methods for a user.
func (s *Service) ListPaymentMethods(ctx context.Context, userID int64) ([]*UserPaymentMethod, error) {
	return s.repo.ListPaymentMethods(ctx, userID)
}

// DeletePaymentMethod removes a user payment method scoped to the owner.
func (s *Service) DeletePaymentMethod(ctx context.Context, userID, id int64) error {
	if userID <= 0 || id <= 0 {
		return apperr.Validation("payment_method.invalid_id", "Valid user ID and payment method ID are required.", nil)
	}
	return s.repo.DeletePaymentMethod(ctx, userID, id)
}
