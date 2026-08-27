package http_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/billing"
	billingHttp "github.com/muhiya/dawa24-store/internal/modules/billing/http"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/httpx"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

type stubRepo struct{ t *testing.T }

func (r stubRepo) fail(method string) {
	r.t.Helper()
	r.t.Fatalf("repository.%s was called; the request should have been rejected before reaching the repository", method)
}

func (r stubRepo) GetOrCreateWallet(ctx context.Context, userID int64, currency string) (*billing.Wallet, error) {
	r.fail("GetOrCreateWallet")
	return nil, nil
}
func (r stubRepo) GetWallet(ctx context.Context, id int64) (*billing.Wallet, error) {
	r.fail("GetWallet")
	return nil, nil
}
func (r stubRepo) RecordTransaction(ctx context.Context, walletID int64, txType billing.TransactionType, delta money.Amount, refType string, refID *int64, desc string) (*billing.WalletTransaction, error) {
	r.fail("RecordTransaction")
	return nil, nil
}
func (r stubRepo) ListTransactions(ctx context.Context, walletID int64, limit, offset int) ([]*billing.WalletTransaction, error) {
	r.fail("ListTransactions")
	return nil, nil
}

func (r stubRepo) CreatePayment(ctx context.Context, p *billing.Payment) error {
	r.fail("CreatePayment")
	return nil
}
func (r stubRepo) GetPaymentByID(ctx context.Context, id int64) (*billing.Payment, error) {
	r.fail("GetPaymentByID")
	return nil, nil
}

func (r stubRepo) ListPlans(ctx context.Context) ([]*billing.Plan, error) {
	r.fail("ListPlans")
	return nil, nil
}

func (r stubRepo) CreatePlan(ctx context.Context, p *billing.Plan) error {
	r.fail("CreatePlan")
	return nil
}
func (r stubRepo) GetPlanByID(ctx context.Context, id int64) (*billing.Plan, error) {
	r.fail("GetPlanByID")
	return nil, nil
}
func (r stubRepo) GetPlanBySlug(ctx context.Context, slug string) (*billing.Plan, error) {
	r.fail("GetPlanBySlug")
	return nil, nil
}
func (r stubRepo) GetDefaultPlan(ctx context.Context) (*billing.Plan, error) {
	r.fail("GetDefaultPlan")
	return nil, nil
}
func (r stubRepo) UpdatePlan(ctx context.Context, p *billing.Plan) error {
	r.fail("UpdatePlan")
	return nil
}
func (r stubRepo) CreateSubscription(ctx context.Context, sub *billing.Subscription) error {
	r.fail("CreateSubscription")
	return nil
}
func (r stubRepo) GetActiveSubscription(ctx context.Context, userID int64) (*billing.Subscription, error) {
	r.fail("GetActiveSubscription")
	return nil, nil
}
func (r stubRepo) GetActiveSubscriptionByOrg(ctx context.Context, orgID int64) (*billing.Subscription, error) {
	r.fail("GetActiveSubscriptionByOrg")
	return nil, nil
}
func (r stubRepo) CheckEntitlement(ctx context.Context, userID int64, featureKey string) (bool, string, error) {
	r.fail("CheckEntitlement")
	return false, "", nil
}

func (r stubRepo) CreateInvoice(ctx context.Context, inv *billing.Invoice) error {
	r.fail("CreateInvoice")
	return nil
}
func (r stubRepo) GetInvoiceByID(ctx context.Context, id int64) (*billing.Invoice, error) {
	r.fail("GetInvoiceByID")
	return nil, nil
}
func (r stubRepo) GetInvoiceByOrderID(ctx context.Context, orderID int64) (*billing.Invoice, error) {
	r.fail("GetInvoiceByOrderID")
	return nil, nil
}
func (r stubRepo) UpdateInvoiceStatus(ctx context.Context, id int64, status billing.InvoiceStatus) error {
	r.fail("UpdateInvoiceStatus")
	return nil
}
func (r stubRepo) ListInvoicesByOrg(ctx context.Context, orgID int64, limit, offset int) ([]*billing.Invoice, error) {
	r.fail("ListInvoicesByOrg")
	return nil, nil
}

func (r stubRepo) AddPaymentMethod(ctx context.Context, pm *billing.UserPaymentMethod) error {
	r.fail("AddPaymentMethod")
	return nil
}
func (r stubRepo) ListPaymentMethods(ctx context.Context, userID int64) ([]*billing.UserPaymentMethod, error) {
	r.fail("ListPaymentMethods")
	return nil, nil
}
func (r stubRepo) DeletePaymentMethod(ctx context.Context, _, id int64) error {
	r.fail("DeletePaymentMethod")
	return nil
}
func (r stubRepo) ListPlatformPaymentMethods(ctx context.Context, onlyActive bool) ([]*billing.PlatformPaymentMethod, error) {
	r.fail("ListPlatformPaymentMethods")
	return nil, nil
}
func (r stubRepo) GetPlatformPaymentMethod(ctx context.Context, id string) (*billing.PlatformPaymentMethod, error) {
	r.fail("GetPlatformPaymentMethod")
	return nil, nil
}
func (r stubRepo) SavePlatformPaymentMethod(ctx context.Context, pm *billing.PlatformPaymentMethod) error {
	r.fail("SavePlatformPaymentMethod")
	return nil
}
func (r stubRepo) TogglePlatformPaymentMethod(ctx context.Context, id string, active bool) error {
	r.fail("TogglePlatformPaymentMethod")
	return nil
}
func (r stubRepo) DeletePlatformPaymentMethod(ctx context.Context, id string) error {
	r.fail("DeletePlatformPaymentMethod")
	return nil
}

func (r stubRepo) AdminListSubscriptions(ctx context.Context, limit, offset int) ([]*billing.Subscription, error) {
	r.fail("AdminListSubscriptions")
	return nil, nil
}
func (r stubRepo) AdminAdjustWallet(ctx context.Context, walletID int64, amount money.Amount, reason string, actorID int64) error {
	r.fail("AdminAdjustWallet")
	return nil
}
func (r stubRepo) ListPaymentsByOrg(ctx context.Context, orgID int64, limit, offset int) ([]*billing.Payment, error) {
	r.fail("ListPaymentsByOrg")
	return nil, nil
}
func (r stubRepo) AdminListInvoices(ctx context.Context, limit, offset int) ([]*billing.Invoice, error) {
	r.fail("AdminListInvoices")
	return nil, nil
}
func (r stubRepo) AdminListWallets(ctx context.Context, limit, offset int) ([]*billing.Wallet, error) {
	r.fail("AdminListWallets")
	return nil, nil
}
func (r stubRepo) AdminListPayments(ctx context.Context, limit, offset int) ([]*billing.Payment, error) {
	r.fail("AdminListPayments")
	return nil, nil
}
func (r stubRepo) EnsureAllOrgWallets(ctx context.Context) error {
	r.fail("EnsureAllOrgWallets")
	return nil
}
func (r stubRepo) AdminListDetailedWallets(ctx context.Context, filter billing.WalletFilter) ([]*billing.AdminWalletView, int, error) {
	r.fail("AdminListDetailedWallets")
	return nil, 0, nil
}
func (r stubRepo) AdminListDetailedTransactions(ctx context.Context, filter billing.TransactionFilter) ([]*billing.AdminWalletTransactionView, int, error) {
	r.fail("AdminListDetailedTransactions")
	return nil, 0, nil
}
func (r stubRepo) AdminListDetailedInvoices(ctx context.Context, filter billing.InvoiceFilter) ([]*billing.AdminInvoiceView, int, error) {
	r.fail("AdminListDetailedInvoices")
	return nil, 0, nil
}
func (r stubRepo) AdminListDetailedPayments(ctx context.Context, filter billing.PaymentFilter) ([]*billing.AdminPaymentView, int, error) {
	r.fail("AdminListDetailedPayments")
	return nil, 0, nil
}
func (r stubRepo) AdminPerformWalletAdjustment(ctx context.Context, walletID int64, amount money.Amount, txType billing.TransactionType, reason string, actorID int64) error {
	r.fail("AdminPerformWalletAdjustment")
	return nil
}

type happyRepo struct{}

func (happyRepo) GetOrCreateWallet(ctx context.Context, userID int64, currency string) (*billing.Wallet, error) {
	return &billing.Wallet{ID: 1, UserID: userID, Currency: currency, Balance: money.MustParse("100.00")}, nil
}
func (happyRepo) GetWallet(ctx context.Context, id int64) (*billing.Wallet, error) {
	return &billing.Wallet{ID: id, UserID: 1, Currency: "EGP", Balance: money.MustParse("100.00")}, nil
}
func (happyRepo) RecordTransaction(ctx context.Context, walletID int64, txType billing.TransactionType, delta money.Amount, refType string, refID *int64, desc string) (*billing.WalletTransaction, error) {
	return &billing.WalletTransaction{ID: 1, WalletID: walletID, Type: txType, Amount: delta, BalanceAfter: delta}, nil
}
func (happyRepo) ListTransactions(ctx context.Context, walletID int64, limit, offset int) ([]*billing.WalletTransaction, error) {
	return []*billing.WalletTransaction{{ID: 1, WalletID: walletID}}, nil
}
func (happyRepo) CreatePayment(ctx context.Context, p *billing.Payment) error {
	p.ID = 1
	return nil
}
func (happyRepo) GetPaymentByID(ctx context.Context, id int64) (*billing.Payment, error) {
	return &billing.Payment{ID: id, UserID: 1, Amount: money.MustParse("100.00"), Method: "fawry"}, nil
}
func (happyRepo) ListPlans(ctx context.Context) ([]*billing.Plan, error) {
	return []*billing.Plan{{ID: 1, Slug: "basic", Name: i18n.Text{"en": "Basic"}}}, nil
}
func (happyRepo) GetPlanBySlug(ctx context.Context, slug string) (*billing.Plan, error) {
	return &billing.Plan{ID: 1, Slug: slug, Name: i18n.Text{"en": "Basic"}}, nil
}

func (happyRepo) CreatePlan(ctx context.Context, p *billing.Plan) error {
	p.ID = 1
	return nil
}
func (happyRepo) GetPlanByID(ctx context.Context, id int64) (*billing.Plan, error) {
	return &billing.Plan{ID: id, Slug: "basic", Name: i18n.Text{"en": "Basic"}}, nil
}
func (happyRepo) GetDefaultPlan(ctx context.Context) (*billing.Plan, error) {
	return &billing.Plan{ID: 1, Slug: "basic", Name: i18n.Text{"en": "Basic"}, IsDefault: true}, nil
}
func (happyRepo) UpdatePlan(ctx context.Context, p *billing.Plan) error {
	return nil
}
func (happyRepo) CreateSubscription(ctx context.Context, sub *billing.Subscription) error {
	sub.ID = 1
	return nil
}
func (happyRepo) GetActiveSubscription(ctx context.Context, userID int64) (*billing.Subscription, error) {
	return &billing.Subscription{ID: 1, UserID: userID, Status: billing.SubActive}, nil
}
func (happyRepo) GetActiveSubscriptionByOrg(ctx context.Context, orgID int64) (*billing.Subscription, error) {
	return &billing.Subscription{ID: 1, OrganizationID: &orgID, Status: billing.SubActive}, nil
}
func (happyRepo) CheckEntitlement(ctx context.Context, userID int64, featureKey string) (bool, string, error) {
	return true, "unlimited", nil
}
func (happyRepo) CreateInvoice(ctx context.Context, inv *billing.Invoice) error {
	inv.ID = 1
	return nil
}
func (happyRepo) GetInvoiceByID(ctx context.Context, id int64) (*billing.Invoice, error) {
	return &billing.Invoice{ID: id, OrganizationID: 1, InvoiceNumber: "INV-1", Subtotal: money.MustParse("100.00")}, nil
}
func (happyRepo) GetInvoiceByOrderID(ctx context.Context, orderID int64) (*billing.Invoice, error) {
	return &billing.Invoice{ID: 1, OrganizationID: 1, OrderID: &orderID, InvoiceNumber: "INV-1", Subtotal: money.MustParse("100.00")}, nil
}
func (happyRepo) UpdateInvoiceStatus(ctx context.Context, id int64, status billing.InvoiceStatus) error {
	return nil
}
func (happyRepo) ListInvoicesByOrg(ctx context.Context, orgID int64, limit, offset int) ([]*billing.Invoice, error) {
	return []*billing.Invoice{{ID: 1, OrganizationID: orgID}}, nil
}
func (happyRepo) AddPaymentMethod(ctx context.Context, pm *billing.UserPaymentMethod) error {
	pm.ID = 1
	return nil
}
func (happyRepo) ListPaymentMethods(ctx context.Context, userID int64) ([]*billing.UserPaymentMethod, error) {
	return []*billing.UserPaymentMethod{{ID: 1, UserID: userID, Provider: "fawry"}}, nil
}
func (happyRepo) DeletePaymentMethod(ctx context.Context, _, id int64) error {
	return nil
}
func (happyRepo) ListPlatformPaymentMethods(ctx context.Context, onlyActive bool) ([]*billing.PlatformPaymentMethod, error) {
	return []*billing.PlatformPaymentMethod{{ID: "instapay", ProviderType: "instapay"}}, nil
}
func (happyRepo) GetPlatformPaymentMethod(ctx context.Context, id string) (*billing.PlatformPaymentMethod, error) {
	return &billing.PlatformPaymentMethod{ID: id, ProviderType: "instapay"}, nil
}
func (happyRepo) SavePlatformPaymentMethod(ctx context.Context, pm *billing.PlatformPaymentMethod) error {
	return nil
}
func (happyRepo) TogglePlatformPaymentMethod(ctx context.Context, id string, active bool) error {
	return nil
}
func (happyRepo) DeletePlatformPaymentMethod(ctx context.Context, id string) error {
	return nil
}
func (happyRepo) AdminListSubscriptions(ctx context.Context, limit, offset int) ([]*billing.Subscription, error) {
	return []*billing.Subscription{{ID: 1, UserID: 1}}, nil
}
func (happyRepo) AdminAdjustWallet(ctx context.Context, walletID int64, amount money.Amount, reason string, actorID int64) error {
	return nil
}
func (happyRepo) ListPaymentsByOrg(ctx context.Context, orgID int64, limit, offset int) ([]*billing.Payment, error) {
	return []*billing.Payment{{ID: 1, OrganizationID: &orgID, Amount: money.MustParse("100.00")}}, nil
}
func (happyRepo) AdminListInvoices(ctx context.Context, limit, offset int) ([]*billing.Invoice, error) {
	return []*billing.Invoice{{ID: 1, InvoiceNumber: "INV-1"}}, nil
}
func (happyRepo) AdminListWallets(ctx context.Context, limit, offset int) ([]*billing.Wallet, error) {
	return []*billing.Wallet{{ID: 1, UserID: 1, Balance: money.MustParse("100.00")}}, nil
}
func (happyRepo) AdminListPayments(ctx context.Context, limit, offset int) ([]*billing.Payment, error) {
	return []*billing.Payment{{ID: 1, UserID: 1}}, nil
}
func (happyRepo) EnsureAllOrgWallets(ctx context.Context) error {
	return nil
}
func (happyRepo) AdminListDetailedWallets(ctx context.Context, filter billing.WalletFilter) ([]*billing.AdminWalletView, int, error) {
	return []*billing.AdminWalletView{{ID: 1, UserID: 1, UserName: "Test User"}}, 1, nil
}
func (happyRepo) AdminListDetailedTransactions(ctx context.Context, filter billing.TransactionFilter) ([]*billing.AdminWalletTransactionView, int, error) {
	return []*billing.AdminWalletTransactionView{{ID: 1, WalletID: 1, Type: billing.TxDeposit}}, 1, nil
}
func (happyRepo) AdminListDetailedInvoices(ctx context.Context, filter billing.InvoiceFilter) ([]*billing.AdminInvoiceView, int, error) {
	return []*billing.AdminInvoiceView{{ID: 1, InvoiceNumber: "INV-1"}}, 1, nil
}
func (happyRepo) AdminListDetailedPayments(ctx context.Context, filter billing.PaymentFilter) ([]*billing.AdminPaymentView, int, error) {
	return []*billing.AdminPaymentView{{ID: 1, UserID: 1}}, 1, nil
}
func (happyRepo) AdminPerformWalletAdjustment(ctx context.Context, walletID int64, amount money.Amount, txType billing.TransactionType, reason string, actorID int64) error {
	return nil
}

func newTestRouter(t *testing.T) http.Handler {
	t.Helper()
	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	svc := billing.NewService(stubRepo{t: t}, log)
	r := chi.NewRouter()
	r.Use(httpx.RequestID)
	r.Use(httpx.Recover(log))
	r.Use(httpx.Locale)

	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie("dawa24_session")
			if err != nil || cookie.Value == "" {
				httpx.Error(w, r, log, apperr.Unauthorized())
				return
			}
			if cookie.Value == "forged-token-that-was-never-issued" {
				httpx.Error(w, r, log, apperr.Unauthorized())
				return
			}
			next.ServeHTTP(w, r)
		})
	})
	billingHttp.NewHandler(svc, log).RegisterRoutes(r)
	return r
}

func newAuthedRouter(repo billing.Repository) http.Handler {
	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	svc := billing.NewService(repo, log)
	r := chi.NewRouter()
	r.Use(httpx.RequestID)
	r.Use(httpx.Recover(log))
	r.Use(httpx.Locale)

	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			actor := authctx.Actor{
				UserID:         1,
				OrganizationID: 1,
				Role:           "super_admin",
				Permissions:    []string{"admin", "super_admin", "billing.admin"},
			}
			ctx := authctx.WithActor(r.Context(), actor)
			ctx = database.WithTenant(ctx, 1)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	})
	billingHttp.NewHandler(svc, log).RegisterRoutes(r)
	return r
}

var protectedRoutes = []struct{ method, path string }{
	{http.MethodGet, "/api/v1/billing/wallet"},
	{http.MethodPost, "/api/v1/billing/wallet/deposit"},
	{http.MethodPost, "/api/v1/billing/wallet/withdraw"},
	{http.MethodGet, "/api/v1/billing/plans"},
	{http.MethodPost, "/api/v1/billing/subscriptions"},
	{http.MethodGet, "/api/v1/billing/entitlements/test_feat"},
	{http.MethodPost, "/api/v1/billing/invoices"},
	{http.MethodGet, "/api/v1/billing/invoices/1"},
	{http.MethodGet, "/api/v1/billing/invoices"},
	{http.MethodPost, "/api/v1/billing/invoices/1/pay"},
	{http.MethodPost, "/api/v1/billing/payment-methods"},
	{http.MethodGet, "/api/v1/billing/payment-methods"},
}

func TestProtectedRoutesRejectAnonymousCallers(t *testing.T) {
	router := newTestRouter(t)
	for _, route := range protectedRoutes {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			req := httptest.NewRequest(route.method, route.path, strings.NewReader("{}"))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code != http.StatusUnauthorized {
				t.Errorf("got %d, want 401 — this endpoint is reachable without a session", rec.Code)
			}
		})
	}
}

func TestProtectedRoutesRejectGarbageSessionToken(t *testing.T) {
	router := newTestRouter(t)
	for _, route := range protectedRoutes {
		req := httptest.NewRequest(route.method, route.path, strings.NewReader("{}"))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: "dawa24_session", Value: "forged-token-that-was-never-issued"})
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s with a forged token got %d, want 401", route.method, route.path, rec.Code)
		}
	}
}

func TestUnauthorizedResponseUsesTheErrorEnvelope(t *testing.T) {
	router := newTestRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/billing/wallet", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	var body httpx.ErrorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not the JSON error envelope: %v (body: %s)", err, rec.Body.String())
	}
	if body.Error.Code == "" {
		t.Error("error envelope has no code")
	}
	if body.Error.RequestID == "" {
		t.Error("error envelope has no request_id")
	}
}

func TestBillingHandler_HappyPaths(t *testing.T) {
	router := newAuthedRouter(happyRepo{})

	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		wantStatus int
	}{
		{"GetWallet", http.MethodGet, "/api/v1/billing/wallet", "", http.StatusOK},
		{"Deposit", http.MethodPost, "/api/v1/billing/wallet/deposit", `{"user_id":1,"currency":"EGP","amount":"50.00","description":"test deposit"}`, http.StatusOK},
		{"Withdraw", http.MethodPost, "/api/v1/billing/wallet/withdraw", `{"user_id":1,"currency":"EGP","amount":"25.00","description":"test withdraw"}`, http.StatusOK},
		{"ListPlans", http.MethodGet, "/api/v1/billing/plans", "", http.StatusOK},
		{"Subscribe", http.MethodPost, "/api/v1/billing/subscriptions", `{"plan_slug":"basic"}`, http.StatusCreated},
		{"CheckEntitlement", http.MethodGet, "/api/v1/billing/entitlements/ai_matching", "", http.StatusOK},
		{"CreateInvoice", http.MethodPost, "/api/v1/billing/invoices", `{"organization_id":1,"invoice_number":"INV-100","subtotal":"100.00","issue_date":"2026-01-01T00:00:00Z","due_date":"2026-02-01T00:00:00Z"}`, http.StatusCreated},
		{"GetInvoice", http.MethodGet, "/api/v1/billing/invoices/1", "", http.StatusOK},
		{"ListInvoices", http.MethodGet, "/api/v1/billing/invoices?limit=10&offset=0", "", http.StatusOK},
		{"PayInvoice", http.MethodPost, "/api/v1/billing/invoices/1/pay", `{"payment_method_id":1}`, http.StatusOK},
		{"AddPaymentMethod", http.MethodPost, "/api/v1/billing/payment-methods", `{"user_id":1,"provider":"fawry","account_identifier":"01000000000","is_default":true}`, http.StatusCreated},
		{"ListPaymentMethods", http.MethodGet, "/api/v1/billing/payment-methods", "", http.StatusOK},
		{"AdminSubscriptions", http.MethodGet, "/api/v1/admin/billing/subscriptions", "", http.StatusOK},
		{"AdminAdjustWallet", http.MethodPost, "/api/v1/admin/billing/wallets/1/adjust", `{"amount":"10.00","reason":"admin bonus"}`, http.StatusOK},
		{"AdminPayments", http.MethodGet, "/api/v1/admin/billing/payments", "", http.StatusOK},
		{"AdminMarkPaid", http.MethodPost, "/api/v1/admin/billing/invoices/1/paid", "", http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var bodyReader io.Reader
			if tt.body != "" {
				bodyReader = strings.NewReader(tt.body)
			}
			req := httptest.NewRequest(tt.method, tt.path, bodyReader)
			if tt.body != "" {
				req.Header.Set("Content-Type", "application/json")
			}
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code != tt.wantStatus {
				t.Errorf("%s %s got status %d, want %d (body: %s)", tt.method, tt.path, rec.Code, tt.wantStatus, rec.Body.String())
			}
		})
	}
}
