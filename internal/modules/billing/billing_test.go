package billing

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

type mockBillingRepo struct {
	wallets       map[int64]*Wallet
	transactions  map[int64][]*WalletTransaction
	plans         map[string]*Plan
	subscriptions map[int64]*Subscription
	invoices      map[int64]*Invoice
	methods       map[int64]*UserPaymentMethod
	nextID        int64
}

func newMockBillingRepo() *mockBillingRepo {
	return &mockBillingRepo{
		wallets:       map[int64]*Wallet{},
		transactions:  map[int64][]*WalletTransaction{},
		plans:         map[string]*Plan{},
		subscriptions: map[int64]*Subscription{},
		invoices:      map[int64]*Invoice{},
		methods:       map[int64]*UserPaymentMethod{},
		nextID:        1,
	}
}

func (m *mockBillingRepo) GetOrCreateWallet(_ context.Context, userID int64, currency string) (*Wallet, error) {
	for _, w := range m.wallets {
		if w.UserID == userID && w.Currency == currency {
			return w, nil
		}
	}
	w := &Wallet{
		ID:       m.nextID,
		UserID:   userID,
		Currency: currency,
		Balance:  money.Zero,
	}
	m.nextID++
	m.wallets[w.ID] = w
	return w, nil
}

func (m *mockBillingRepo) GetWallet(_ context.Context, id int64) (*Wallet, error) {
	w, ok := m.wallets[id]
	if !ok {
		return nil, apperr.NotFound("wallet")
	}
	return w, nil
}

func (m *mockBillingRepo) RecordTransaction(
	_ context.Context,
	walletID int64,
	txType TransactionType,
	delta money.Amount,
	refType string,
	refID *int64,
	desc string,
) (*WalletTransaction, error) {
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

	tx := &WalletTransaction{
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

func (m *mockBillingRepo) ListTransactions(_ context.Context, walletID int64, limit, offset int) ([]*WalletTransaction, error) {
	return m.transactions[walletID], nil
}

func (m *mockBillingRepo) CreatePayment(_ context.Context, p *Payment) error { return nil }
func (m *mockBillingRepo) GetPaymentByID(_ context.Context, id int64) (*Payment, error) {
	return nil, apperr.NotFound("payment")
}

func (m *mockBillingRepo) ListPlans(_ context.Context) ([]*Plan, error) {
	var list []*Plan
	for _, p := range m.plans {
		list = append(list, p)
	}
	return list, nil
}

func (m *mockBillingRepo) CreatePlan(_ context.Context, p *Plan) error {
	p.ID = 1
	return nil
}

func (m *mockBillingRepo) GetPlanByID(_ context.Context, id int64) (*Plan, error) {
	for _, p := range m.plans {
		if p.ID == id {
			return p, nil
		}
	}
	return nil, apperr.NotFound("plan")
}

func (m *mockBillingRepo) GetDefaultPlan(_ context.Context) (*Plan, error) {
	for _, p := range m.plans {
		if p.IsDefault {
			return p, nil
		}
	}
	for _, p := range m.plans {
		return p, nil
	}
	return nil, apperr.NotFound("plan")
}

func (m *mockBillingRepo) UpdatePlan(_ context.Context, p *Plan) error {
	m.plans[p.Slug] = p
	return nil
}

func (m *mockBillingRepo) GetPlanBySlug(_ context.Context, slug string) (*Plan, error) {
	p, ok := m.plans[slug]
	if !ok {
		return nil, apperr.NotFound("plan")
	}
	return p, nil
}

func (m *mockBillingRepo) CreateSubscription(_ context.Context, sub *Subscription) error {
	sub.ID = m.nextID
	m.nextID++
	m.subscriptions[sub.UserID] = sub
	return nil
}

func (m *mockBillingRepo) GetActiveSubscription(_ context.Context, userID int64) (*Subscription, error) {
	sub, ok := m.subscriptions[userID]
	if !ok {
		return nil, apperr.NotFound("subscription")
	}
	return sub, nil
}

func (m *mockBillingRepo) GetActiveSubscriptionByOrg(_ context.Context, orgID int64) (*Subscription, error) {
	for _, sub := range m.subscriptions {
		if sub.OrganizationID != nil && *sub.OrganizationID == orgID && sub.Status == SubActive {
			return sub, nil
		}
	}
	return nil, apperr.NotFound("subscription")
}

func (m *mockBillingRepo) CheckEntitlement(_ context.Context, userID int64, featureKey string) (bool, string, error) {
	sub, ok := m.subscriptions[userID]
	if !ok || sub.Status != SubActive {
		return false, "", nil
	}
	for _, plan := range m.plans {
		if plan.ID == sub.PlanID || plan.Slug == "pro" {
			if plan.Features == nil {
				return false, "", nil
			}
			val, ok := plan.Features[featureKey]
			return ok, val, nil
		}
	}
	return false, "", nil
}

func (m *mockBillingRepo) CreateInvoice(_ context.Context, inv *Invoice) error {
	inv.ID = m.nextID
	m.nextID++
	m.invoices[inv.ID] = inv
	return nil
}

func (m *mockBillingRepo) GetInvoiceByID(_ context.Context, id int64) (*Invoice, error) {
	inv, ok := m.invoices[id]
	if !ok {
		return nil, apperr.NotFound("invoice")
	}
	return inv, nil
}

func (m *mockBillingRepo) GetInvoiceByOrderID(_ context.Context, orderID int64) (*Invoice, error) {
	for _, inv := range m.invoices {
		if inv.OrderID != nil && *inv.OrderID == orderID {
			return inv, nil
		}
	}
	return nil, apperr.NotFound("invoice")
}

func (m *mockBillingRepo) UpdateInvoiceStatus(_ context.Context, id int64, status InvoiceStatus) error {
	if inv, ok := m.invoices[id]; ok {
		inv.Status = status
	}
	return nil
}

func (m *mockBillingRepo) ListInvoicesByOrg(_ context.Context, orgID int64, limit, offset int) ([]*Invoice, error) {
	var list []*Invoice
	for _, inv := range m.invoices {
		if inv.OrganizationID == orgID {
			list = append(list, inv)
		}
	}
	return list, nil
}

func (m *mockBillingRepo) AddPaymentMethod(_ context.Context, pm *UserPaymentMethod) error {
	pm.ID = m.nextID
	m.nextID++
	m.methods[pm.ID] = pm
	return nil
}

func (m *mockBillingRepo) ListPaymentMethods(_ context.Context, userID int64) ([]*UserPaymentMethod, error) {
	var list []*UserPaymentMethod
	for _, pm := range m.methods {
		if pm.UserID == userID {
			list = append(list, pm)
		}
	}
	return list, nil
}

func (m *mockBillingRepo) DeletePaymentMethod(_ context.Context, _, id int64) error {
	delete(m.methods, id)
	return nil
}

func (m *mockBillingRepo) ListPlatformPaymentMethods(_ context.Context, onlyActive bool) ([]*PlatformPaymentMethod, error) {
	return nil, nil
}

func (m *mockBillingRepo) GetPlatformPaymentMethod(_ context.Context, id string) (*PlatformPaymentMethod, error) {
	return nil, nil
}

func (m *mockBillingRepo) SavePlatformPaymentMethod(_ context.Context, pm *PlatformPaymentMethod) error {
	return nil
}

func (m *mockBillingRepo) TogglePlatformPaymentMethod(_ context.Context, id string, active bool) error {
	return nil
}

func (m *mockBillingRepo) DeletePlatformPaymentMethod(_ context.Context, id string) error {
	return nil
}

func (m *mockBillingRepo) AdminAdjustWallet(_ context.Context, walletID int64, amount money.Amount, reason string, actorID int64) error {
	return nil
}

func (m *mockBillingRepo) AdminListPayments(_ context.Context, limit, offset int) ([]*Payment, error) {
	return nil, nil
}

func (m *mockBillingRepo) AdminListSubscriptions(_ context.Context, limit, offset int) ([]*Subscription, error) {
	return nil, nil
}

func (m *mockBillingRepo) ListPaymentsByOrg(_ context.Context, orgID int64, limit, offset int) ([]*Payment, error) {
	return nil, nil
}

func (m *mockBillingRepo) AdminListInvoices(_ context.Context, limit, offset int) ([]*Invoice, error) {
	return nil, nil
}

func (m *mockBillingRepo) AdminListWallets(_ context.Context, limit, offset int) ([]*Wallet, error) {
	return nil, nil
}

func (m *mockBillingRepo) EnsureAllOrgWallets(_ context.Context) error {
	return nil
}

func (m *mockBillingRepo) AdminListDetailedWallets(_ context.Context, _ WalletFilter) ([]*AdminWalletView, int, error) {
	return nil, 0, nil
}

func (m *mockBillingRepo) AdminListDetailedTransactions(_ context.Context, _ TransactionFilter) ([]*AdminWalletTransactionView, int, error) {
	return nil, 0, nil
}

func (m *mockBillingRepo) AdminListDetailedInvoices(_ context.Context, _ InvoiceFilter) ([]*AdminInvoiceView, int, error) {
	return nil, 0, nil
}

func (m *mockBillingRepo) AdminListDetailedPayments(_ context.Context, _ PaymentFilter) ([]*AdminPaymentView, int, error) {
	return nil, 0, nil
}

func (m *mockBillingRepo) AdminPerformWalletAdjustment(_ context.Context, _ int64, _ money.Amount, _ TransactionType, _ string, _ int64) error {
	return nil
}

func TestWalletDepositAndWithdraw(t *testing.T) {
	ctx := context.Background()
	repo := newMockBillingRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := NewService(repo, logger)

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
	inv, err := svc.CreateInvoice(ctx, &Invoice{
		OrganizationID: 8801,
		Lines: []InvoiceLine{
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

	gotInv, err := svc.GetInvoice(ctx, inv.ID)
	if err != nil {
		t.Fatalf("GetInvoice failed: %v", err)
	}
	if gotInv.ID != inv.ID {
		t.Errorf("got invoice id %d, want %d", gotInv.ID, inv.ID)
	}

	if err := svc.MarkInvoicePaid(ctx, inv.ID); err != nil {
		t.Fatalf("MarkInvoicePaid failed: %v", err)
	}

	// 5. Test Subscription and Entitlements
	repo.plans["pro"] = &Plan{
		Slug:     "pro",
		Features: map[string]string{"max_products": "1000", "ai_matching": "true"},
	}
	sub, err := svc.Subscribe(ctx, 42, nil, "pro", "month", nil)
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}
	if sub.UserID != 42 {
		t.Errorf("got user id %d, want 42", sub.UserID)
	}

	allowed, val, err := svc.CheckEntitlement(ctx, 42, "ai_matching")
	if err != nil || !allowed || val != "true" {
		t.Errorf("CheckEntitlement failed: allowed=%v, val=%s, err=%v", allowed, val, err)
	}

	// 6. Payment Methods
	pm := &UserPaymentMethod{
		UserID:            42,
		Provider:          "Paymob",
		AccountIdentifier: "tok_123",
		IsDefault:         true,
	}
	if err := svc.AddPaymentMethod(ctx, pm); err != nil {
		t.Fatalf("AddPaymentMethod failed: %v", err)
	}
	methods, err := svc.ListPaymentMethods(ctx, 42)
	if err != nil || len(methods) != 1 {
		t.Fatalf("ListPaymentMethods failed: %v", err)
	}

	// 7. Admin methods
	adjMoney, _ := money.Parse("100.00")
	if err := svc.AdminAdjustWallet(ctx, 1, adjMoney, "Bonus credit", 1); err != nil {
		t.Fatalf("AdminAdjustWallet failed: %v", err)
	}
	_, _ = svc.AdminListPayments(ctx, 10, 0)
	_, _ = svc.AdminListSubscriptions(ctx, 10, 0)
}

func TestCreatePlanValidation(t *testing.T) {
	repo := newMockBillingRepo()
	svc := NewService(repo, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx := context.Background()

	if _, err := svc.CreatePlan(ctx, &Plan{}); err == nil {
		t.Fatal("expected error for empty slug/name")
	}

	p, err := svc.CreatePlan(ctx, &Plan{Slug: "basic", Name: i18n.Text{"ar": "أساسية"}, PriceMonth: money.MustParse("100.00")})
	if err != nil {
		t.Fatalf("CreatePlan failed: %v", err)
	}
	if p.ID == 0 {
		t.Fatal("expected plan id to be set")
	}
}
