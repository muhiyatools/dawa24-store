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
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	commerceHttp "github.com/muhiya/dawa24-store/internal/modules/commerce/http"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/httpx"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

type stubRepo struct{ t *testing.T }

func (r stubRepo) fail(method string) {
	r.t.Helper()
	r.t.Fatalf("repository.%s was called; the request should have been rejected before reaching the repository", method)
}

func (r stubRepo) GetOrCreateCart(ctx context.Context, userID int64) (*commerce.Cart, error) {
	r.fail("GetOrCreateCart")
	return nil, nil
}
func (r stubRepo) GetCartWithItems(ctx context.Context, cartID int64) (*commerce.Cart, error) {
	r.fail("GetCartWithItems")
	return nil, nil
}
func (r stubRepo) AddToCartItem(ctx context.Context, cartID int64, item *commerce.CartItem) error {
	r.fail("AddToCartItem")
	return nil
}
func (r stubRepo) SetCartItemQuantity(ctx context.Context, cartID int64, variantID int64, quantity int) error {
	r.fail("SetCartItemQuantity")
	return nil
}
func (r stubRepo) RemoveCartItem(ctx context.Context, cartID int64, variantID int64) error {
	r.fail("RemoveCartItem")
	return nil
}
func (r stubRepo) ClearCart(ctx context.Context, cartID int64) error {
	r.fail("ClearCart")
	return nil
}

func (r stubRepo) CreateOrder(ctx context.Context, order *commerce.Order, shipments []*commerce.OrderShipment, lines []*commerce.OrderLine) error {
	r.fail("CreateOrder")
	return nil
}
func (r stubRepo) GetOrderByID(ctx context.Context, id int64) (*commerce.Order, error) {
	r.fail("GetOrderByID")
	return nil, nil
}
func (r stubRepo) GetOrderByNumber(ctx context.Context, number string) (*commerce.Order, error) {
	r.fail("GetOrderByNumber")
	return nil, nil
}
func (r stubRepo) UpdateOrderStatus(ctx context.Context, orderID int64, toStatus commerce.OrderStatus, history commerce.OrderStatusHistory) error {
	r.fail("UpdateOrderStatus")
	return nil
}
func (r stubRepo) UpdateCustomerPendingOrder(ctx context.Context, order *commerce.Order, lines []commerce.OrderLineEditItem, changedByUserID int64) (*commerce.Order, error) {
	r.fail("UpdateCustomerPendingOrder")
	return nil, nil
}
func (r stubRepo) ListOrdersByCustomer(ctx context.Context, customerID int64, limit, offset int) ([]*commerce.Order, error) {
	r.fail("ListOrdersByCustomer")
	return nil, nil
}
func (r stubRepo) ListShipmentsByVendor(ctx context.Context, vendorOrgID int64, limit, offset int) ([]*commerce.OrderShipment, error) {
	r.fail("ListShipmentsByVendor")
	return nil, nil
}
func (r stubRepo) GetShipmentByID(ctx context.Context, id int64) (*commerce.OrderShipment, error) {
	r.fail("GetShipmentByID")
	return nil, nil
}
func (r stubRepo) UpdateShipmentStatus(ctx context.Context, id int64, from, to commerce.OrderStatus, history commerce.OrderStatusHistory) error {
	r.fail("UpdateShipmentStatus")
	return nil
}
func (r stubRepo) GetShipmentForDeliveryByTracking(ctx context.Context, tracking string) (*commerce.OrderShipment, error) {
	r.fail("GetShipmentForDeliveryByTracking")
	return nil, nil
}
func (r stubRepo) VerifyAndCompleteDelivery(ctx context.Context, shipmentID int64, deliveryCode string, notes string, collectedAmountMinor int64) (*commerce.OrderShipment, error) {
	r.fail("VerifyAndCompleteDelivery")
	return nil, nil
}
func (r stubRepo) ListOrderHistory(ctx context.Context, orderID int64) ([]*commerce.OrderStatusHistory, error) {
	r.fail("ListOrderHistory")
	return nil, nil
}
func (r stubRepo) RateOrder(ctx context.Context, orderID int64, customerID int64, rating float64, review string) error {
	r.fail("RateOrder")
	return nil
}

func (r stubRepo) AddToWishlist(ctx context.Context, userID int64, productID int64) error {
	r.fail("AddToWishlist")
	return nil
}
func (r stubRepo) RemoveFromWishlist(ctx context.Context, userID int64, productID int64) error {
	r.fail("RemoveFromWishlist")
	return nil
}
func (r stubRepo) ListWishlist(ctx context.Context, userID int64) ([]*commerce.WishlistItem, error) {
	r.fail("ListWishlist")
	return nil, nil
}

func (r stubRepo) CreateQuoteRequest(ctx context.Context, qr *commerce.QuoteRequest) error {
	r.fail("CreateQuoteRequest")
	return nil
}
func (r stubRepo) GetQuoteRequestByID(ctx context.Context, id int64) (*commerce.QuoteRequest, error) {
	r.fail("GetQuoteRequestByID")
	return nil, nil
}
func (r stubRepo) UpdateQuoteStatus(ctx context.Context, id int64, status commerce.QuoteStatus, quotePrice money.Amount, supplierNotes string) error {
	r.fail("UpdateQuoteStatus")
	return nil
}
func (r stubRepo) ListQuoteRequestsByOrg(ctx context.Context, orgID int64, asSupplier bool, limit, offset int) ([]*commerce.QuoteRequest, error) {
	r.fail("ListQuoteRequestsByOrg")
	return nil, nil
}

func (r stubRepo) AdminSearchOrders(ctx context.Context, query string, limit, offset int) ([]*commerce.Order, error) {
	r.fail("AdminSearchOrders")
	return nil, nil
}
func (r stubRepo) CreatePurchaseRequest(ctx context.Context, pr *commerce.PurchaseRequest, lines []*commerce.PurchaseRequestLine) error {
	r.fail("CreatePurchaseRequest")
	return nil
}
func (r stubRepo) GetPurchaseRequestByID(ctx context.Context, id int64) (*commerce.PurchaseRequest, error) {
	r.fail("GetPurchaseRequestByID")
	return nil, nil
}
func (r stubRepo) GetPurchaseRequestByNumber(ctx context.Context, number string) (*commerce.PurchaseRequest, error) {
	r.fail("GetPurchaseRequestByNumber")
	return nil, nil
}
func (r stubRepo) ListPurchaseRequestsByCustomer(ctx context.Context, customerID int64, orgID *int64, status string, limit, offset int) ([]*commerce.PurchaseRequest, error) {
	r.fail("ListPurchaseRequestsByCustomer")
	return nil, nil
}
func (r stubRepo) ListPurchaseRequestsByVendor(ctx context.Context, vendorOrgID int64, status string, limit, offset int) ([]*commerce.PurchaseRequest, error) {
	r.fail("ListPurchaseRequestsByVendor")
	return nil, nil
}
func (r stubRepo) CountPurchaseRequestsByCustomer(ctx context.Context, customerID int64, orgID *int64) (map[string]int, error) {
	r.fail("CountPurchaseRequestsByCustomer")
	return nil, nil
}
func (r stubRepo) UpdatePurchaseRequestStatus(ctx context.Context, id int64, status commerce.PurchaseRequestStatus, vendorNotes string, responderID *int64) error {
	r.fail("UpdatePurchaseRequestStatus")
	return nil
}
func (r stubRepo) UpdatePurchaseRequestLineOffer(ctx context.Context, lineID int64, price money.Amount, discount float64, status string) error {
	r.fail("UpdatePurchaseRequestLineOffer")
	return nil
}
func (r stubRepo) AcceptNegotiation(ctx context.Context, orderID int64, actorID int64) error {
	r.fail("AcceptNegotiation")
	return nil
}
func (r stubRepo) RejectNegotiation(ctx context.Context, orderID int64, reason string, actorID int64) error {
	r.fail("RejectNegotiation")
	return nil
}
func (r stubRepo) GetVendorFinancialSummary(ctx context.Context, vendorOrgID int64, period string) (*commerce.VendorFinancialSummary, error) {
	r.fail("GetVendorFinancialSummary")
	return nil, nil
}

type happyRepo struct{}

func (happyRepo) GetOrCreateCart(ctx context.Context, userID int64) (*commerce.Cart, error) {
	return &commerce.Cart{ID: 1, UserID: userID}, nil
}
func (happyRepo) GetCartWithItems(ctx context.Context, cartID int64) (*commerce.Cart, error) {
	return &commerce.Cart{ID: cartID, UserID: 1, Items: []*commerce.CartItem{{ID: 1, CartID: cartID, ProductID: 1, ProductVariantID: 1, Quantity: 2, UnitPrice: money.MustParse("50.00")}}}, nil
}
func (happyRepo) AddToCartItem(ctx context.Context, cartID int64, item *commerce.CartItem) error {
	return nil
}
func (happyRepo) SetCartItemQuantity(ctx context.Context, cartID int64, variantID int64, quantity int) error {
	return nil
}
func (happyRepo) RemoveCartItem(ctx context.Context, cartID int64, variantID int64) error {
	return nil
}
func (happyRepo) ClearCart(ctx context.Context, cartID int64) error {
	return nil
}
func (happyRepo) CreateOrder(ctx context.Context, order *commerce.Order, shipments []*commerce.OrderShipment, lines []*commerce.OrderLine) error {
	order.ID = 1
	order.OrderNumber = "ORD-2026-0001"
	return nil
}
func (happyRepo) GetOrderByID(ctx context.Context, id int64) (*commerce.Order, error) {
	return &commerce.Order{ID: id, CustomerID: 1, OrderNumber: "ORD-1", Status: commerce.StatusDelivered, TotalAmount: money.MustParse("100.00")}, nil
}
func (happyRepo) GetOrderByNumber(ctx context.Context, number string) (*commerce.Order, error) {
	return &commerce.Order{ID: 1, CustomerID: 1, OrderNumber: number, Status: commerce.StatusDelivered}, nil
}
func (happyRepo) UpdateOrderStatus(ctx context.Context, orderID int64, toStatus commerce.OrderStatus, history commerce.OrderStatusHistory) error {
	return nil
}
func (happyRepo) ListOrdersByCustomer(ctx context.Context, customerID int64, limit, offset int) ([]*commerce.Order, error) {
	return []*commerce.Order{{ID: 1, CustomerID: customerID, OrderNumber: "ORD-1"}}, nil
}
func (happyRepo) ListShipmentsByVendor(ctx context.Context, vendorOrgID int64, limit, offset int) ([]*commerce.OrderShipment, error) {
	return []*commerce.OrderShipment{{ID: 1, OrganizationID: vendorOrgID, ShipmentNumber: "SH-1"}}, nil
}
func (happyRepo) GetShipmentByID(ctx context.Context, id int64) (*commerce.OrderShipment, error) {
	return &commerce.OrderShipment{ID: id, OrganizationID: 1, Status: commerce.StatusPending}, nil
}
func (happyRepo) UpdateShipmentStatus(ctx context.Context, id int64, from, to commerce.OrderStatus, history commerce.OrderStatusHistory) error {
	return nil
}
func (happyRepo) GetShipmentForDeliveryByTracking(ctx context.Context, tracking string) (*commerce.OrderShipment, error) {
	return &commerce.OrderShipment{ID: 1, ShipmentNumber: "SH-1", TrackingNumber: tracking, Status: commerce.StatusShipped, DeliveryCode: "123456"}, nil
}
func (happyRepo) VerifyAndCompleteDelivery(ctx context.Context, shipmentID int64, deliveryCode string, notes string, collectedAmountMinor int64) (*commerce.OrderShipment, error) {
	return &commerce.OrderShipment{ID: shipmentID, Status: commerce.StatusDelivered}, nil
}
func (happyRepo) ListOrderHistory(ctx context.Context, orderID int64) ([]*commerce.OrderStatusHistory, error) {
	return []*commerce.OrderStatusHistory{{ID: 1, OrderID: orderID, ToStatus: string(commerce.StatusPending)}}, nil
}
func (happyRepo) RateOrder(ctx context.Context, orderID int64, customerID int64, rating float64, review string) error {
	return nil
}
func (happyRepo) AddToWishlist(ctx context.Context, userID int64, productID int64) error {
	return nil
}
func (happyRepo) RemoveFromWishlist(ctx context.Context, userID int64, productID int64) error {
	return nil
}
func (happyRepo) ListWishlist(ctx context.Context, userID int64) ([]*commerce.WishlistItem, error) {
	return []*commerce.WishlistItem{{ID: 1, UserID: userID, ProductID: 1}}, nil
}
func (happyRepo) CreateQuoteRequest(ctx context.Context, qr *commerce.QuoteRequest) error {
	qr.ID = 1
	return nil
}
func (happyRepo) GetQuoteRequestByID(ctx context.Context, id int64) (*commerce.QuoteRequest, error) {
	return &commerce.QuoteRequest{ID: id, OrganizationID: 1, CustomerOrgID: 2, Status: commerce.QuotePending}, nil
}
func (happyRepo) UpdateQuoteStatus(ctx context.Context, id int64, status commerce.QuoteStatus, quotePrice money.Amount, supplierNotes string) error {
	return nil
}
func (happyRepo) ListQuoteRequestsByOrg(ctx context.Context, orgID int64, asSupplier bool, limit, offset int) ([]*commerce.QuoteRequest, error) {
	return []*commerce.QuoteRequest{{ID: 1, OrganizationID: orgID, Status: commerce.QuotePending}}, nil
}
func (happyRepo) AdminSearchOrders(ctx context.Context, query string, limit, offset int) ([]*commerce.Order, error) {
	return []*commerce.Order{{ID: 1, CustomerID: 1, OrderNumber: "ORD-1"}}, nil
}
func (happyRepo) CreatePurchaseRequest(ctx context.Context, pr *commerce.PurchaseRequest, lines []*commerce.PurchaseRequestLine) error {
	pr.ID = 1
	pr.RequestNumber = "PR-2026-0001"
	return nil
}
func (happyRepo) GetPurchaseRequestByID(ctx context.Context, id int64) (*commerce.PurchaseRequest, error) {
	return &commerce.PurchaseRequest{ID: id, RequestNumber: "PR-1", Status: commerce.PurchaseRequestPending}, nil
}
func (happyRepo) GetPurchaseRequestByNumber(ctx context.Context, number string) (*commerce.PurchaseRequest, error) {
	return &commerce.PurchaseRequest{ID: 1, RequestNumber: number, Status: commerce.PurchaseRequestPending}, nil
}
func (happyRepo) ListPurchaseRequestsByCustomer(ctx context.Context, customerID int64, orgID *int64, status string, limit, offset int) ([]*commerce.PurchaseRequest, error) {
	return []*commerce.PurchaseRequest{{ID: 1, RequestNumber: "PR-1", Status: commerce.PurchaseRequestPending}}, nil
}
func (happyRepo) ListPurchaseRequestsByVendor(ctx context.Context, vendorOrgID int64, status string, limit, offset int) ([]*commerce.PurchaseRequest, error) {
	return []*commerce.PurchaseRequest{{ID: 1, RequestNumber: "PR-1", Status: commerce.PurchaseRequestPending}}, nil
}
func (happyRepo) CountPurchaseRequestsByCustomer(ctx context.Context, customerID int64, orgID *int64) (map[string]int, error) {
	return map[string]int{"all": 1, "pending": 1}, nil
}
func (happyRepo) UpdatePurchaseRequestStatus(ctx context.Context, id int64, status commerce.PurchaseRequestStatus, vendorNotes string, responderID *int64) error {
	return nil
}
func (happyRepo) UpdatePurchaseRequestLineOffer(ctx context.Context, lineID int64, price money.Amount, discount float64, status string) error {
	return nil
}
func (happyRepo) AcceptNegotiation(ctx context.Context, orderID int64, actorID int64) error {
	return nil
}
func (happyRepo) RejectNegotiation(ctx context.Context, orderID int64, reason string, actorID int64) error {
	return nil
}
func (happyRepo) UpdateCustomerPendingOrder(ctx context.Context, order *commerce.Order, lines []commerce.OrderLineEditItem, changedByUserID int64) (*commerce.Order, error) {
	return order, nil
}
func (happyRepo) GetVendorFinancialSummary(ctx context.Context, vendorOrgID int64, period string) (*commerce.VendorFinancialSummary, error) {
	return &commerce.VendorFinancialSummary{Period: period}, nil
}

func newTestRouter(t *testing.T) http.Handler {
	t.Helper()
	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	svc := commerce.NewService(stubRepo{t: t}, log)
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
	commerceHttp.NewHandler(svc, log).RegisterRoutes(r)
	return r
}

func newAuthedRouter(repo commerce.Repository) http.Handler {
	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	svc := commerce.NewService(repo, log)
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
				Permissions:    []string{"admin", "super_admin", "commerce.admin"},
			}
			ctx := authctx.WithActor(r.Context(), actor)
			ctx = database.WithTenant(ctx, 1)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	})
	commerceHttp.NewHandler(svc, log).RegisterRoutes(r)
	return r
}

var protectedRoutes = []struct{ method, path string }{
	{http.MethodPost, "/api/v1/commerce/checkout"},
	{http.MethodPatch, "/api/v1/commerce/cart/items/1"},
	{http.MethodGet, "/api/v1/commerce/orders/1/history"},
	{http.MethodPost, "/api/v1/commerce/orders/1/rate"},
	{http.MethodGet, "/api/v1/commerce/shipments/1"},
	{http.MethodPost, "/api/v1/commerce/shipments/1/status"},
	{http.MethodGet, "/api/v1/commerce/orders/1"},
	{http.MethodGet, "/api/v1/commerce/orders"},
	{http.MethodPost, "/api/v1/commerce/orders/1/status"},
	{http.MethodPost, "/api/v1/commerce/orders/1/cancel"},
	{http.MethodGet, "/api/v1/commerce/vendor/shipments"},
	{http.MethodGet, "/api/v1/commerce/cart"},
	{http.MethodPost, "/api/v1/commerce/cart/items"},
	{http.MethodDelete, "/api/v1/commerce/cart/items/1"},
	{http.MethodDelete, "/api/v1/commerce/cart"},
	{http.MethodGet, "/api/v1/commerce/wishlist"},
	{http.MethodPost, "/api/v1/commerce/wishlist"},
	{http.MethodDelete, "/api/v1/commerce/wishlist/1"},
	{http.MethodPost, "/api/v1/commerce/quotes"},
	{http.MethodPost, "/api/v1/commerce/quotes/1/respond"},
	{http.MethodGet, "/api/v1/commerce/quotes"},
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
	req := httptest.NewRequest(http.MethodGet, "/api/v1/commerce/cart", nil)
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

func TestCommerceHandler_HappyPaths(t *testing.T) {
	router := newAuthedRouter(happyRepo{})

	validUntil := time.Now().AddDate(0, 0, 7).Format(time.RFC3339)
	_ = validUntil

	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		wantStatus int
	}{
		{"GetCart", http.MethodGet, "/api/v1/commerce/cart", "", http.StatusOK},
		{"AddCartItem", http.MethodPost, "/api/v1/commerce/cart/items", `{"product_id":1,"product_variant_id":1,"quantity":2,"unit_price":"50.00"}`, http.StatusOK},
		{"SetCartQuantity", http.MethodPatch, "/api/v1/commerce/cart/items/1", `{"quantity":5}`, http.StatusOK},
		{"RemoveCartItem", http.MethodDelete, "/api/v1/commerce/cart/items/1", "", http.StatusOK},
		{"ClearCart", http.MethodDelete, "/api/v1/commerce/cart", "", http.StatusOK},
		{"Checkout", http.MethodPost, "/api/v1/commerce/checkout", `{"customer_id":1,"payment_method":"fawry","items":[{"vendor_org_id":1,"product_name":{"en":"Panadol"},"unit_price":"50.00","quantity":2}]}`, http.StatusCreated},
		{"GetOrder", http.MethodGet, "/api/v1/commerce/orders/1", "", http.StatusOK},
		{"ListOrders", http.MethodGet, "/api/v1/commerce/orders?limit=10&offset=0", "", http.StatusOK},
		{"GetOrderHistory", http.MethodGet, "/api/v1/commerce/orders/1/history", "", http.StatusOK},
		{"RateOrder", http.MethodPost, "/api/v1/commerce/orders/1/rate", `{"rating":5,"review":"excellent"}`, http.StatusNoContent},
		{"CancelOrder", http.MethodPost, "/api/v1/commerce/orders/1/cancel", `{"reason":"mistake"}`, http.StatusOK},
		{"GetShipment", http.MethodGet, "/api/v1/commerce/shipments/1", "", http.StatusOK},
		{"ListVendorShipments", http.MethodGet, "/api/v1/commerce/vendor/shipments?vendor_id=1", "", http.StatusOK},
		{"GetWishlist", http.MethodGet, "/api/v1/commerce/wishlist", "", http.StatusOK},
		{"AddToWishlist", http.MethodPost, "/api/v1/commerce/wishlist", `{"product_id":1}`, http.StatusCreated},
		{"RemoveFromWishlist", http.MethodDelete, "/api/v1/commerce/wishlist/1", "", http.StatusOK},
		{"CreateQuote", http.MethodPost, "/api/v1/commerce/quotes", `{"organization_id":1,"customer_org_id":2,"product_name":"Panadol","requested_quantity":100,"target_unit_price":"45.00"}`, http.StatusCreated},
		{"RespondQuote", http.MethodPost, "/api/v1/commerce/quotes/1/respond", `{"quote_price":"47.00","supplier_notes":"best offer"}`, http.StatusOK},
		{"ListQuotes", http.MethodGet, "/api/v1/commerce/quotes", "", http.StatusOK},
		{"AdminSearchOrders", http.MethodGet, "/api/v1/admin/commerce/orders", "", http.StatusOK},
		{"AdminGetOrder", http.MethodGet, "/api/v1/admin/commerce/orders/1", "", http.StatusOK},
		{"AdminForceStatus", http.MethodPost, "/api/v1/admin/commerce/orders/1/status", `{"status":"confirmed","note":"admin verified"}`, http.StatusOK},
		{"AdminRefund", http.MethodPost, "/api/v1/admin/commerce/orders/1/refund", `{"reason":"customer returned item"}`, http.StatusOK},
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

func (r stubRepo) CountOrders(_ context.Context) (int, error) {
	r.fail("CountOrders")
	return 0, nil
}

func (happyRepo) CountOrders(_ context.Context) (int, error) { return 5, nil }

func (r stubRepo) MonthSalesByVendor(_ context.Context, _ int64) (money.Amount, error) {
	r.fail("MonthSalesByVendor")
	return money.Zero, nil
}

func (happyRepo) MonthSalesByVendor(_ context.Context, _ int64) (money.Amount, error) {
	return money.MustParse("1250.00"), nil
}

func (r stubRepo) MonthSpendByCustomer(_ context.Context, _ int64) (money.Amount, error) {
	r.fail("MonthSpendByCustomer")
	return money.Zero, nil
}

func (happyRepo) MonthSpendByCustomer(_ context.Context, _ int64) (money.Amount, error) {
	return money.MustParse("800.00"), nil
}

func (r stubRepo) SetShipmentTracking(context.Context, int64, string, string) error {
	r.fail("SetShipmentTracking")
	return nil
}

func (happyRepo) SetShipmentTracking(context.Context, int64, string, string) error {
	return nil
}

func (r stubRepo) CountVendorShipmentsByStatus(_ context.Context, _ int64, _ []string) (int, error) {
	r.fail("CountVendorShipmentsByStatus")
	return 0, nil
}

func (happyRepo) CountVendorShipmentsByStatus(_ context.Context, _ int64, _ []string) (int, error) {
	return 2, nil
}
