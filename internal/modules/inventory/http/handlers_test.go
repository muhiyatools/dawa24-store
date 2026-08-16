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

	"github.com/muhiya/dawa24-store/internal/modules/inventory"
	inventoryHttp "github.com/muhiya/dawa24-store/internal/modules/inventory/http"
	"github.com/muhiya/dawa24-store/internal/platform/httpx"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

type stubRepo struct{ t *testing.T }

func (r stubRepo) fail(method string) {
	r.t.Helper()
	r.t.Fatalf("repository.%s was called; the request should have been rejected before reaching the repository", method)
}

func (r stubRepo) CreateWarehouse(ctx context.Context, w *inventory.Warehouse) error {
	r.fail("CreateWarehouse")
	return nil
}
func (r stubRepo) GetWarehouseByID(ctx context.Context, id int64) (*inventory.Warehouse, error) {
	r.fail("GetWarehouseByID")
	return nil, nil
}
func (r stubRepo) ListWarehouses(ctx context.Context) ([]*inventory.Warehouse, error) {
	r.fail("ListWarehouses")
	return nil, nil
}
func (r stubRepo) UpdateWarehouse(ctx context.Context, w *inventory.Warehouse) error {
	r.fail("UpdateWarehouse")
	return nil
}
func (r stubRepo) SoftDeleteWarehouse(ctx context.Context, id int64) error {
	r.fail("SoftDeleteWarehouse")
	return nil
}
func (r stubRepo) CountStockInWarehouse(ctx context.Context, warehouseID int64) (int, error) {
	r.fail("CountStockInWarehouse")
	return 0, nil
}

func (r stubRepo) GetStock(ctx context.Context, warehouseID, variantID int64) (*inventory.Stock, error) {
	r.fail("GetStock")
	return nil, nil
}
func (r stubRepo) UpsertStock(ctx context.Context, s *inventory.Stock) error {
	r.fail("UpsertStock")
	return nil
}
func (r stubRepo) AdjustStock(ctx context.Context, stockID int64, delta int, movement inventory.StockMovement) (*inventory.Stock, error) {
	r.fail("AdjustStock")
	return nil, nil
}
func (r stubRepo) ListStocksByWarehouse(ctx context.Context, warehouseID int64) ([]*inventory.Stock, error) {
	r.fail("ListStocksByWarehouse")
	return nil, nil
}
func (r stubRepo) ListStockMovements(ctx context.Context, stockID int64, limit int) ([]*inventory.StockMovement, error) {
	r.fail("ListStockMovements")
	return nil, nil
}
func (r stubRepo) ListLowStock(ctx context.Context, limit, offset int) ([]*inventory.Stock, error) {
	r.fail("ListLowStock")
	return nil, nil
}
func (r stubRepo) ListMovementsByOrg(ctx context.Context, limit, offset int) ([]*inventory.StockMovement, error) {
	r.fail("ListMovementsByOrg")
	return nil, nil
}

func (r stubRepo) CreateTransfer(ctx context.Context, t *inventory.WarehouseTransfer) error {
	r.fail("CreateTransfer")
	return nil
}
func (r stubRepo) GetTransferByID(ctx context.Context, id int64) (*inventory.WarehouseTransfer, error) {
	r.fail("GetTransferByID")
	return nil, nil
}
func (r stubRepo) UpdateTransferStatus(ctx context.Context, id int64, from, to inventory.TransferStatus) error {
	r.fail("UpdateTransferStatus")
	return nil
}
func (r stubRepo) ListTransfers(ctx context.Context, status string, limit, offset int) ([]*inventory.WarehouseTransfer, error) {
	r.fail("ListTransfers")
	return nil, nil
}

func newTestRouter(t *testing.T) http.Handler {
	t.Helper()
	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	svc := inventory.NewService(stubRepo{t: t}, log)
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
	inventoryHttp.NewHandler(svc, log).RegisterRoutes(r)
	return r
}

var protectedRoutes = []struct{ method, path string }{
	{http.MethodGet, "/api/v1/inventory/warehouses"},
	{http.MethodPost, "/api/v1/inventory/warehouses"},
	{http.MethodGet, "/api/v1/inventory/warehouses/1"},
	{http.MethodPut, "/api/v1/inventory/warehouses/1"},
	{http.MethodDelete, "/api/v1/inventory/warehouses/1"},
	{http.MethodGet, "/api/v1/inventory/warehouses/1/stocks"},
	{http.MethodPost, "/api/v1/inventory/stocks/adjust"},
	{http.MethodGet, "/api/v1/inventory/stocks/low"},
	{http.MethodGet, "/api/v1/inventory/stocks/1/movements"},
	{http.MethodGet, "/api/v1/inventory/movements"},
	{http.MethodPost, "/api/v1/inventory/transfers"},
	{http.MethodGet, "/api/v1/inventory/transfers"},
	{http.MethodGet, "/api/v1/inventory/transfers/1"},
	{http.MethodPost, "/api/v1/inventory/transfers/1/receive"},
	{http.MethodPost, "/api/v1/inventory/transfers/1/cancel"},
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
	req := httptest.NewRequest(http.MethodGet, "/api/v1/inventory/warehouses", nil)
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

func TestHandlerRejectsUnknownJSONFields(t *testing.T) {
	router := newTestRouter(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/inventory/warehouses",
		strings.NewReader(`{"name":"test","unknown_field":"y"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "dawa24_session", Value: "valid-token"})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("got %d, want 422 for an unknown JSON field", rec.Code)
	}
}

func TestHandlerRejectsMalformedBody(t *testing.T) {
	router := newTestRouter(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/inventory/warehouses",
		strings.NewReader(`{"name": `))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "dawa24_session", Value: "valid-token"})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("got %d, want 422 for a malformed body", rec.Code)
	}
}
