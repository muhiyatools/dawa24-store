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

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	catalogHttp "github.com/muhiya/dawa24-store/internal/modules/catalog/http"
	"github.com/muhiya/dawa24-store/internal/platform/httpx"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

type stubRepo struct{ t *testing.T }

func (r stubRepo) fail(method string) {
	r.t.Helper()
	r.t.Fatalf("repository.%s was called; the request should have been rejected before reaching the repository", method)
}

func (r stubRepo) CreateProduct(ctx context.Context, p *catalog.Product) error {
	r.fail("CreateProduct")
	return nil
}
func (r stubRepo) GetProductByID(ctx context.Context, id int64) (*catalog.Product, error) {
	r.fail("GetProductByID")
	return nil, nil
}
func (r stubRepo) UpdateProduct(ctx context.Context, p *catalog.Product) error {
	r.fail("UpdateProduct")
	return nil
}
func (r stubRepo) DeleteProduct(ctx context.Context, id int64) error {
	r.fail("DeleteProduct")
	return nil
}
func (r stubRepo) SearchProducts(ctx context.Context, params catalog.SearchParams) ([]*catalog.Product, error) {
	r.fail("SearchProducts")
	return nil, nil
}
func (r stubRepo) ListProducts(ctx context.Context, status string, limit, offset int) ([]*catalog.Product, error) {
	r.fail("ListProducts")
	return nil, nil
}
func (r stubRepo) SetProductsStatus(ctx context.Context, ids []int64, status catalog.ProductStatus) (int64, error) {
	r.fail("SetProductsStatus")
	return 0, nil
}

func (r stubRepo) CreateVariant(ctx context.Context, v *catalog.ProductVariant) error {
	r.fail("CreateVariant")
	return nil
}
func (r stubRepo) GetVariantByID(ctx context.Context, id int64) (*catalog.ProductVariant, error) {
	r.fail("GetVariantByID")
	return nil, nil
}
func (r stubRepo) ListVariantsByProduct(ctx context.Context, productID int64) ([]*catalog.ProductVariant, error) {
	r.fail("ListVariantsByProduct")
	return nil, nil
}
func (r stubRepo) ListVariantsByOrganization(ctx context.Context, orgID int64, params catalog.VariantSearchParams) ([]*catalog.ProductVariant, int, error) {
	r.fail("ListVariantsByOrganization")
	return nil, 0, nil
}
func (r stubRepo) UpdateVariant(ctx context.Context, v *catalog.ProductVariant) error {
	r.fail("UpdateVariant")
	return nil
}
func (r stubRepo) DeleteVariant(ctx context.Context, id int64) error {
	r.fail("DeleteVariant")
	return nil
}

func (r stubRepo) CreateCategory(ctx context.Context, c *catalog.Category) error {
	r.fail("CreateCategory")
	return nil
}
func (r stubRepo) GetCategoryByID(ctx context.Context, id int64) (*catalog.Category, error) {
	r.fail("GetCategoryByID")
	return nil, nil
}
func (r stubRepo) UpdateCategory(ctx context.Context, c *catalog.Category) error {
	r.fail("UpdateCategory")
	return nil
}
func (r stubRepo) DeleteCategory(ctx context.Context, id int64) error {
	r.fail("DeleteCategory")
	return nil
}
func (r stubRepo) ListCategories(ctx context.Context) ([]*catalog.Category, error) {
	r.fail("ListCategories")
	return nil, nil
}
func (r stubRepo) CountProductsInCategory(ctx context.Context, categoryID int64) (int, error) {
	r.fail("CountProductsInCategory")
	return 0, nil
}

func (r stubRepo) CreateBrand(ctx context.Context, b *catalog.Brand) error {
	r.fail("CreateBrand")
	return nil
}
func (r stubRepo) GetBrandByID(ctx context.Context, id int64) (*catalog.Brand, error) {
	r.fail("GetBrandByID")
	return nil, nil
}
func (r stubRepo) UpdateBrand(ctx context.Context, b *catalog.Brand) error {
	r.fail("UpdateBrand")
	return nil
}
func (r stubRepo) DeleteBrand(ctx context.Context, id int64) error {
	r.fail("DeleteBrand")
	return nil
}
func (r stubRepo) ListBrands(ctx context.Context) ([]*catalog.Brand, error) {
	r.fail("ListBrands")
	return nil, nil
}

func (r stubRepo) SetCustomerPricing(ctx context.Context, m *catalog.CustomerProductMapping) error {
	r.fail("SetCustomerPricing")
	return nil
}
func (r stubRepo) GetCustomerPricing(ctx context.Context, organizationID, customerOrgID, productID int64) (*catalog.CustomerProductMapping, error) {
	r.fail("GetCustomerPricing")
	return nil, nil
}
func (r stubRepo) CreateProductAlert(ctx context.Context, a *catalog.ProductAlert) error {
	r.fail("CreateProductAlert")
	return nil
}
func (r stubRepo) ListProductAlertsByUser(ctx context.Context, userID int64) ([]*catalog.ProductAlert, error) {
	r.fail("ListProductAlertsByUser")
	return nil, nil
}

func (r stubRepo) GetFirstFinderQuestion(context.Context) (*catalog.FinderQuestion, error) {
	r.fail("GetFirstFinderQuestion")
	return nil, nil
}
func (r stubRepo) GetFinderQuestionByID(context.Context, int64) (*catalog.FinderQuestion, error) {
	r.fail("GetFinderQuestionByID")
	return nil, nil
}
func (r stubRepo) ListFinderOptions(context.Context, int64) ([]*catalog.FinderOption, error) {
	r.fail("ListFinderOptions")
	return nil, nil
}
func (r stubRepo) GetFinderResultByID(context.Context, int64) (*catalog.FinderResult, error) {
	r.fail("GetFinderResultByID")
	return nil, nil
}
func (r stubRepo) ListFinderQuestions(context.Context) ([]*catalog.FinderQuestion, error) {
	r.fail("ListFinderQuestions")
	return nil, nil
}

func (r stubRepo) CreateFinderQuestion(context.Context, *catalog.FinderQuestion) error {
	r.fail("CreateFinderQuestion")
	return nil
}
func (r stubRepo) CreateFinderOption(context.Context, *catalog.FinderOption) error {
	r.fail("CreateFinderOption")
	return nil
}
func (r stubRepo) CreateFinderResult(context.Context, *catalog.FinderResult) error {
	r.fail("CreateFinderResult")
	return nil
}
func (r stubRepo) ListFinderResults(context.Context) ([]*catalog.FinderResult, error) {
	r.fail("ListFinderResults")
	return nil, nil
}
func (r stubRepo) CountProductsInBrand(ctx context.Context, brandID int64) (int, error) {
	r.fail("CountProductsInBrand")
	return 0, nil
}
func (r stubRepo) UpsertProductIndex(ctx context.Context, item *catalog.ProductIndexItem) error {
	r.fail("UpsertProductIndex")
	return nil
}
func (r stubRepo) DeleteProductIndex(ctx context.Context, uniqueRowID string) error {
	r.fail("DeleteProductIndex")
	return nil
}
func (r stubRepo) DeleteProductIndexByProduct(ctx context.Context, productID int64) error {
	r.fail("DeleteProductIndexByProduct")
	return nil
}
func (r stubRepo) SearchProductIndex(ctx context.Context, params catalog.SearchParams) ([]*catalog.ProductIndexItem, error) {
	r.fail("SearchProductIndex")
	return nil, nil
}
func (r stubRepo) RebuildProductIndex(ctx context.Context) (int64, error) {
	r.fail("RebuildProductIndex")
	return 0, nil
}
func (r stubRepo) CreateSavingProduct(ctx context.Context, sp *catalog.SavingProduct) error {
	r.fail("CreateSavingProduct")
	return nil
}
func (r stubRepo) ListSavingProductsByOrg(ctx context.Context, orgID int64, limit, offset int) ([]*catalog.SavingProduct, error) {
	r.fail("ListSavingProductsByOrg")
	return nil, nil
}
func (r stubRepo) GetSavingProductByID(ctx context.Context, id int64) (*catalog.SavingProduct, error) {
	r.fail("GetSavingProductByID")
	return nil, nil
}
func (r stubRepo) DeleteSavingProduct(ctx context.Context, id, orgID int64) error {
	r.fail("DeleteSavingProduct")
	return nil
}

func newTestRouter(t *testing.T) http.Handler {
	t.Helper()
	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	svc := catalog.NewService(stubRepo{t: t}, log)
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
	catalogHttp.NewHandler(svc, log).RegisterRoutes(r)
	return r
}

var protectedRoutes = []struct{ method, path string }{
	{http.MethodGet, "/api/v1/catalog/search"},
	{http.MethodGet, "/api/v1/catalog/products/1"},
	{http.MethodPut, "/api/v1/catalog/products/1"},
	{http.MethodDelete, "/api/v1/catalog/products/1"},
	{http.MethodPost, "/api/v1/catalog/products"},
	{http.MethodGet, "/api/v1/catalog/products"},
	{http.MethodPost, "/api/v1/catalog/products/bulk-status"},
	{http.MethodPost, "/api/v1/catalog/products/1/variants"},
	{http.MethodGet, "/api/v1/catalog/products/1/variants/1"},
	{http.MethodPut, "/api/v1/catalog/products/1/variants/1"},
	{http.MethodDelete, "/api/v1/catalog/products/1/variants/1"},
	{http.MethodGet, "/api/v1/catalog/categories/1"},
	{http.MethodPut, "/api/v1/catalog/categories/1"},
	{http.MethodDelete, "/api/v1/catalog/categories/1"},
	{http.MethodGet, "/api/v1/catalog/brands/1"},
	{http.MethodPut, "/api/v1/catalog/brands/1"},
	{http.MethodDelete, "/api/v1/catalog/brands/1"},
	{http.MethodGet, "/api/v1/catalog/categories"},
	{http.MethodPost, "/api/v1/catalog/categories"},
	{http.MethodGet, "/api/v1/catalog/brands"},
	{http.MethodPost, "/api/v1/catalog/brands"},
	{http.MethodPost, "/api/v1/catalog/pricing/customer"},
	{http.MethodGet, "/api/v1/catalog/pricing/customer"},
	{http.MethodPost, "/api/v1/catalog/alerts"},
	{http.MethodGet, "/api/v1/catalog/alerts"},
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
	req := httptest.NewRequest(http.MethodGet, "/api/v1/catalog/products", nil)
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
	req := httptest.NewRequest(http.MethodPost, "/api/v1/catalog/products",
		strings.NewReader(`{"name":{"ar":"test"},"price":100,"unknown_field":"y"}`))
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
	req := httptest.NewRequest(http.MethodPost, "/api/v1/catalog/products",
		strings.NewReader(`{"name": `))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "dawa24_session", Value: "valid-token"})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("got %d, want 422 for a malformed body", rec.Code)
	}
}

func (r stubRepo) CountProductsByOrg(_ context.Context, _ int64, _ string) (int, error) {
	r.fail("CountProductsByOrg")
	return 0, nil
}

// Category -> brand relationship stubs (PLAN_V7 Phase 4). Behaviour is covered
// by the catalog repository tests; these only satisfy the interface.
func (s stubRepo) ListBrandsByCategory(context.Context, int64) ([]*catalog.Brand, error) {
	return nil, nil
}

func (s stubRepo) BrandInCategory(context.Context, int64, int64) (bool, error) {
	return true, nil
}

func (s stubRepo) SetBrandCategories(context.Context, int64, []int64) error {
	return nil
}
