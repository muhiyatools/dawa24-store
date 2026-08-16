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

	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	commerceHttp "github.com/muhiya/dawa24-store/internal/modules/commerce/http"
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
func (r stubRepo) ListOrderHistory(ctx context.Context, orderID int64) ([]*commerce.OrderStatusHistory, error) {
	r.fail("ListOrderHistory")
	return nil, nil
}
func (r stubRepo) RateOrder(ctx context.Context, orderID int64, customerID int64, rating int, review string) error {
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

func (r stubRepo) CreateQuoteRequest(ctx context.Context, q *commerce.QuoteRequest) error {
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
func (r stubRepo) ListQuoteRequestsByOrg(ctx context.Context, orgID int64, isVendor bool, limit, offset int) ([]*commerce.QuoteRequest, error) {
	r.fail("ListQuoteRequestsByOrg")
	return nil, nil
}

func newTestRouter(t *testing.T) http.Handler {
	t.Helper()

	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	svc := commerce.NewService(stubRepo{t: t}, log)

	r := chi.NewRouter()
	r.Use(httpx.RequestID)
	r.Use(httpx.Recover(log))
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie("dawa24_session")
			if r.Header.Get("Authorization") == "" && err != nil {
				httpx.Error(w, r, log, apperr.Unauthorized())
				return
			}
			if r.Header.Get("Authorization") == "Bearer forged-token" || (err == nil && cookie.Value == "forged-token-that-was-never-issued") {
				httpx.Error(w, r, log, apperr.Unauthorized())
				return
			}
			next.ServeHTTP(w, r)
		})
	})

	commerceHttp.NewHandler(svc, log).RegisterRoutes(r)

	return r
}

var allRoutes = []struct{ method, path string }{
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

// Tests cover authorization surface of the commerce handlers
// These endpoints require an authenticated user context

func TestRoutesRejectAnonymousCallers(t *testing.T) {
	router := newTestRouter(t)

	for _, route := range allRoutes {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			req := httptest.NewRequest(route.method, route.path, strings.NewReader("{}"))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Errorf("got %d, want 401 — this endpoint should reject anonymous callers", rec.Code)
			}
		})
	}
}

func TestProtectedRoutesRejectGarbageSessionToken(t *testing.T) {
	router := newTestRouter(t)

	for _, route := range allRoutes {
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

func TestBearerTokenIsAlsoValidated(t *testing.T) {
	router := newTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/commerce/cart", nil)
	req.Header.Set("Authorization", "Bearer forged-token")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("got %d, want 401 for a forged bearer token", rec.Code)
	}
}

func TestRejectsUnknownJSONFields(t *testing.T) {
	router := newTestRouter(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/commerce/checkout",
		strings.NewReader(`{"customer_id":1,"totally_unknown":"y"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer valid-token")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("got %d, want 422 for an unknown JSON field", rec.Code)
	}
}

func TestRejectsMalformedBody(t *testing.T) {
	router := newTestRouter(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/commerce/checkout",
		strings.NewReader(`{"customer_id": `))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer valid-token")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("got %d, want 422 for a malformed body", rec.Code)
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
		t.Error("error envelope has no code; clients cannot branch on it")
	}
	if body.Error.RequestID == "" {
		t.Error("error envelope has no request_id")
	}
}
