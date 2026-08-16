package billing_test

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/billing"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

type mockBillingRepo struct {
	wallets       map[int64]*billing.Wallet
	transactions  map[int64][]*billing.WalletTransaction
	plans         map[string]*billing.Plan
	subscriptions map[int64]*billing.Subscription
	nextID        int64
}

func newMockBillingRepo() *mockBillingRepo {
	return &mockBillingRepo{
		wallets:       map[int64]*billing.Wallet{},
		transactions:  map[int64][]*billing.WalletTransaction{},
		plans:         map[string]*billing.Plan{},
		subscriptions: map[int64]*billing.Subscription{},
		nextID:        1,
	}
}

func (m *mockBillingRepo) GetOrCreateWallet(_ context.Context, userID int64, currency string) (*billing.Wallet, error) {
	for _, w := range m.wallets {
		if w.UserID == userID && w.Currency == currency {
			return w, nil
		}
	}
	w := &billing.Wallet{
		ID:       m.nextID,
		UserID:   userID,
		Currency: currency,
		Balance:  money.Zero,
	}
	m.nextID++
	m.wallets[w.ID] = w
	return w, nil
}

func (m *mockBillingRepo) GetWallet(_ context.Context, id int64) (*billing.Wallet, error) {
	w, ok := m.wallets[id]
	if !ok {
		return nil, apperr.NotFound("wallet")
	}
	return w, nil
}

func (m *mockBillingRepo) RecordTransaction(
	_ context.Context,
	walletID int64,
	txType billing.TransactionType,
	delta money.Amount,
	refType string,
	refID *int64,
	desc string,
) (*billing.WalletTransaction, error) {
	w, ok := m.wallets[walletID]
	if !ok {
		return nil, apperr.NotFound("wallet")
	}

	newBal, err := w.Balance.Add(delta)
	if err != nil {
		return nil, apperr.Internal(err)
	}
	if newBal.IsNegative() {
		return nil, apperr.Validation("wallet.insufficient_funds", "Insufficient funds", nil)
	}
	w.Balance = newBal

	tx := &billing.WalletTransaction{
		ID:           m.nextID,
		WalletID:     walletID,
		Type:         txType,
		Amount:       delta,
		BalanceAfter: newBal,
		Description:  desc,
		CreatedAt:    time.Now(),
	}
	m.nextID++
	m.transactions[walletID] = append(m.transactions[walletID], tx)
	return tx, nil
}

func (m *mockBillingRepo) ListTransactions(_ context.Context, walletID int64, limit, offset int) ([]*billing.WalletTransaction, error) {
	return m.transactions[walletID], nil
}

func (m *mockBillingRepo) CreatePayment(_ context.Context, p *billing.Payment) error { return nil }
func (m *mockBillingRepo) GetPaymentByID(_ context.Context, id int64) (*billing.Payment, error) {
	return nil, apperr.NotFound("payment")
}

func (m *mockBillingRepo) ListPlans(_ context.Context) ([]*billing.Plan, error) {
	var list []*billing.Plan
	for _, p := range m.plans {
		list = append(list, p)
	}
	return list, nil
}

func (m *mockBillingRepo) GetPlanBySlug(_ context.Context, slug string) (*billing.Plan, error) {
	p, ok := m.plans[slug]
	if !ok {
		return nil, apperr.NotFound("plan")
	}
	return p, nil
}

func (m *mockBillingRepo) CreateSubscription(_ context.Context, sub *billing.Subscription) error {
	sub.ID = m.nextID
	m.nextID++
	m.subscriptions[sub.UserID] = sub
	return nil
}

func (m *mockBillingRepo) GetActiveSubscription(_ context.Context, userID int64) (*billing.Subscription, error) {
	sub, ok := m.subscriptions[userID]
	if !ok {
		return nil, apperr.NotFound("subscription")
	}
	return sub, nil
}

func (m *mockBillingRepo) CheckEntitlement(_ context.Context, userID int64, featureKey string) (bool, string, error) {
	sub, ok := m.subscriptions[userID]
	if !ok || sub.Status != billing.SubActive {
		return false, "", nil
	}
	plan, ok := m.plans[sub.SourceSystem]
	if !ok || plan.Features == nil {
		return false, "", nil
	}
	val, ok := plan.Features[featureKey]
	return ok, val, nil
}

func (m *mockBillingRepo) CreateInvoice(_ context.Context, inv *billing.Invoice) error {
	inv.ID = m.nextID
	m.nextID++
	return nil
}

func (m *mockBillingRepo) GetInvoiceByID(_ context.Context, id int64) (*billing.Invoice, error) {
	return nil, apperr.NotFound("invoice")
}

func (m *mockBillingRepo) UpdateInvoiceStatus(_ context.Context, id int64, status billing.InvoiceStatus) error {
	return nil
}

func (m *mockBillingRepo) ListInvoicesByOrg(_ context.Context, orgID int64, limit, offset int) ([]*billing.Invoice, error) {
	return nil, nil
}

func (m *mockBillingRepo) AddPaymentMethod(_ context.Context, pm *billing.UserPaymentMethod) error {
	pm.ID = m.nextID
	m.nextID++
	return nil
}

func (m *mockBillingRepo) ListPaymentMethods(_ context.Context, userID int64) ([]*billing.UserPaymentMethod, error) {
	return nil, nil
}

func TestWalletDepositAndWithdraw(t *testing.T) {
	ctx := context.Background()
	repo := newMockBillingRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := billing.NewService(repo, logger)

	// 1. Initial Deposit 500.00 EGP
	tx1, err := svc.Deposit(ctx, 42, "EGP", money.MustParse("500.00"), "test", nil, "Initial balance")
	if err != nil {
		t.Fatalf("Deposit failed: %v", err)
	}
	if tx1.BalanceAfter != money.MustParse("500.00") {
		t.Errorf("BalanceAfter = %v; want 500.00", tx1.BalanceAfter)
	}

	// 2. Withdraw 200.00 EGP
	tx2, err := svc.Withdraw(ctx, 42, "EGP", money.MustParse("200.00"), "test", nil, "Payment")
	if err != nil {
		t.Fatalf("Withdraw failed: %v", err)
	}
	if tx2.BalanceAfter != money.MustParse("300.00") {
		t.Errorf("BalanceAfter = %v; want 300.00", tx2.BalanceAfter)
	}

	// 3. Attempt to withdraw 400.00 EGP (exceeds balance 300.00 EGP) -> Should Fail
	_, err = svc.Withdraw(ctx, 42, "EGP", money.MustParse("400.00"), "test", nil, "Overdraft")
	if err == nil || apperr.KindOf(err) != apperr.KindValidation {
		t.Errorf("expected validation error on overdraft, got: %v", err)
	}

	// 4. Test Invoice Creation
	inv, err := svc.CreateInvoice(ctx, &billing.Invoice{
		OrganizationID: 8801,
		Lines: []billing.InvoiceLine{
			{Description: "Panadol Extra 500mg (100 boxes)", Quantity: 100, UnitPrice: money.MustParse("50.00"), TotalPrice: money.MustParse("5000.00")},
		},
		TaxAmount: money.MustParse("700.00"),
	})
	if err != nil {
		t.Fatalf("CreateInvoice failed: %v", err)
	}
	if inv.TotalAmount != money.MustParse("5700.00") {
		t.Errorf("TotalAmount = %v; want 5700.00", inv.TotalAmount)
	}
}

func (m *mockBillingRepo) AdminAdjustWallet(ctx context.Context, walletID int64, amount money.Amount, reason string, actorID int64) error {
	return nil
}

func (m *mockBillingRepo) AdminListPayments(ctx context.Context, limit, offset int) ([]*billing.Payment, error) {
	return nil, nil
}

func (m *mockBillingRepo) AdminListSubscriptions(ctx context.Context, limit, offset int) ([]*billing.Subscription, error) {
	return nil, nil
}

func (m *mockBillingRepo) DeletePaymentMethod(ctx context.Context, id int64) error { return nil }
