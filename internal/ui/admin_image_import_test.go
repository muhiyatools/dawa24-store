package ui

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

type mockCatalogImageRepo struct {
	products map[string]*catalog.Product
}

func newMockCatalogImageRepo() *mockCatalogImageRepo {
	return &mockCatalogImageRepo{
		products: make(map[string]*catalog.Product),
	}
}

func (m *mockCatalogImageRepo) CreateProduct(_ context.Context, p *catalog.Product) error {
	m.products[p.SKU] = p
	return nil
}
func (m *mockCatalogImageRepo) BulkUpsertProducts(_ context.Context, _ []*catalog.Product, _ catalog.BulkWriteOptions) (catalog.BulkWriteResult, error) { return catalog.BulkWriteResult{}, nil }
func (m *mockCatalogImageRepo) GetProductByID(_ context.Context, _ int64) (*catalog.Product, error) { return nil, nil }
func (m *mockCatalogImageRepo) UpdateProduct(_ context.Context, p *catalog.Product) error { m.products[p.SKU] = p; return nil }
func (m *mockCatalogImageRepo) DeleteProduct(_ context.Context, _ int64) error { return nil }
func (m *mockCatalogImageRepo) SearchProducts(_ context.Context, _ catalog.SearchParams) ([]*catalog.Product, error) { return nil, nil }
func (m *mockCatalogImageRepo) CountProducts(_ context.Context, _ catalog.SearchParams) (int, error) { return 0, nil }
func (m *mockCatalogImageRepo) ListProducts(_ context.Context, _ string, _, _ int) ([]*catalog.Product, error) { return nil, nil }
func (m *mockCatalogImageRepo) SetProductsStatus(_ context.Context, _ []int64, _ catalog.ProductStatus) (int64, error) { return 0, nil }
func (m *mockCatalogImageRepo) CreateVariant(_ context.Context, _ *catalog.ProductVariant) error { return nil }
func (m *mockCatalogImageRepo) GetVariantByID(_ context.Context, _ int64) (*catalog.ProductVariant, error) { return nil, nil }
func (m *mockCatalogImageRepo) GetVariantBySKUOrBarcode(_ context.Context, _ int64, _, _ string) (*catalog.ProductVariant, error) { return nil, nil }
func (m *mockCatalogImageRepo) GetVariantByProductAndOrg(_ context.Context, _, _ int64) (*catalog.ProductVariant, error) { return nil, nil }
func (m *mockCatalogImageRepo) ListVariantsByProduct(_ context.Context, _ int64) ([]*catalog.ProductVariant, error) { return nil, nil }
func (m *mockCatalogImageRepo) ListVariantsByProducts(_ context.Context, _ []int64) ([]*catalog.ProductVariant, error) { return nil, nil }
func (m *mockCatalogImageRepo) ListVariantsByOrganization(_ context.Context, _ int64, _ catalog.VariantSearchParams) ([]*catalog.ProductVariant, int, error) { return nil, 0, nil }
func (m *mockCatalogImageRepo) ListAllVariants(_ context.Context, _ catalog.VariantSearchParams) ([]*catalog.ProductVariant, int, error) { return nil, 0, nil }
func (m *mockCatalogImageRepo) UpdateVariant(_ context.Context, _ *catalog.ProductVariant) error { return nil }
func (m *mockCatalogImageRepo) DeleteVariant(_ context.Context, _ int64) error { return nil }
func (m *mockCatalogImageRepo) DeleteAllVariantsByOrg(_ context.Context, _ int64) (int64, error) { return 0, nil }
func (m *mockCatalogImageRepo) DeleteAllProducts(_ context.Context) (int64, error) { return 0, nil }
func (m *mockCatalogImageRepo) CreateCategory(_ context.Context, _ *catalog.Category) error { return nil }
func (m *mockCatalogImageRepo) GetCategoryByID(_ context.Context, _ int64) (*catalog.Category, error) { return nil, nil }
func (m *mockCatalogImageRepo) UpdateCategory(_ context.Context, _ *catalog.Category) error { return nil }
func (m *mockCatalogImageRepo) DeleteCategory(_ context.Context, _ int64) error { return nil }
func (m *mockCatalogImageRepo) ListCategories(_ context.Context) ([]*catalog.Category, error) { return nil, nil }
func (m *mockCatalogImageRepo) CountProductsByOrg(_ context.Context, _ int64, _ string) (int, error) { return 0, nil }
func (m *mockCatalogImageRepo) CountProductsInCategory(_ context.Context, _ int64) (int, error) { return 0, nil }
func (m *mockCatalogImageRepo) CreateBrand(_ context.Context, _ *catalog.Brand) error { return nil }
func (m *mockCatalogImageRepo) GetBrandByID(_ context.Context, _ int64) (*catalog.Brand, error) { return nil, nil }
func (m *mockCatalogImageRepo) UpdateBrand(_ context.Context, _ *catalog.Brand) error { return nil }
func (m *mockCatalogImageRepo) DeleteBrand(_ context.Context, _ int64) error { return nil }
func (m *mockCatalogImageRepo) ListBrandsByCategory(_ context.Context, _ int64) ([]*catalog.Brand, error) { return nil, nil }
func (m *mockCatalogImageRepo) BrandInCategory(_ context.Context, _, _ int64) (bool, error) { return false, nil }
func (m *mockCatalogImageRepo) SetBrandCategories(_ context.Context, _ int64, _ []int64) error { return nil }
func (m *mockCatalogImageRepo) ListBrands(_ context.Context) ([]*catalog.Brand, error) { return nil, nil }
func (m *mockCatalogImageRepo) CountProductsInBrand(_ context.Context, _ int64) (int, error) { return 0, nil }
func (m *mockCatalogImageRepo) SetCustomerPricing(_ context.Context, _ *catalog.CustomerProductMapping) error { return nil }
func (m *mockCatalogImageRepo) GetCustomerPricing(_ context.Context, _, _, _ int64) (*catalog.CustomerProductMapping, error) { return nil, nil }
func (m *mockCatalogImageRepo) CreateProductAlert(_ context.Context, _ *catalog.ProductAlert) error { return nil }
func (m *mockCatalogImageRepo) ListProductAlertsByUser(_ context.Context, _ int64) ([]*catalog.ProductAlert, error) { return nil, nil }
func (m *mockCatalogImageRepo) UpsertProductIndex(_ context.Context, _ *catalog.ProductIndexItem) error { return nil }
func (m *mockCatalogImageRepo) DeleteProductIndex(_ context.Context, _ string) error { return nil }
func (m *mockCatalogImageRepo) DeleteProductIndexByProduct(_ context.Context, _ int64) error { return nil }
func (m *mockCatalogImageRepo) SearchProductIndex(_ context.Context, _ catalog.SearchParams) ([]*catalog.ProductIndexItem, error) { return nil, nil }
func (m *mockCatalogImageRepo) RebuildProductIndex(_ context.Context) (int64, error) { return 0, nil }
func (m *mockCatalogImageRepo) CreateSavingProduct(_ context.Context, _ *catalog.SavingProduct) error { return nil }
func (m *mockCatalogImageRepo) UpdateSavingProduct(_ context.Context, _ *catalog.SavingProduct) error { return nil }
func (m *mockCatalogImageRepo) ListSavingProductsByOrg(_ context.Context, _ int64, _, _ int) ([]*catalog.SavingProduct, error) { return nil, nil }
func (m *mockCatalogImageRepo) ListSavingProductsEnriched(_ context.Context, _ int64, _, _ string, _, _ int) ([]*catalog.SavingProductEnriched, *catalog.SavingProductStats, error) { return nil, nil, nil }
func (m *mockCatalogImageRepo) GetSavingProductByID(_ context.Context, _ int64) (*catalog.SavingProduct, error) { return nil, nil }
func (m *mockCatalogImageRepo) DeleteSavingProduct(_ context.Context, _, _ int64) error { return nil }
func (m *mockCatalogImageRepo) DeleteAllSavingProducts(_ context.Context, _ int64) error { return nil }
func (m *mockCatalogImageRepo) GetProductProviders(_ context.Context, _ int64) ([]*catalog.ProductProviderInfo, error) { return nil, nil }
func (m *mockCatalogImageRepo) BatchUpsertSavingProducts(_ context.Context, _ int64, _ *int64, _ []*catalog.SavingProduct) (int, int, error) { return 0, 0, nil }
func (m *mockCatalogImageRepo) ListAllSavingProductsAdmin(_ context.Context, _ *int64, _ *int64, _, _ string, _, _ int) ([]*catalog.SavingProductAdminView, *catalog.SavingProductAdminStats, error) { return nil, nil, nil }
func (m *mockCatalogImageRepo) ListAllMasterProductsForMatching(_ context.Context) ([]*catalog.CatalogMatchSource, error) { return nil, nil }

func (m *mockCatalogImageRepo) GetProductBySKU(_ context.Context, sku string) (*catalog.Product, error) {
	if p, ok := m.products[sku]; ok {
		return p, nil
	}
	return nil, apperr.NotFound("product")
}

func (m *mockCatalogImageRepo) UpdateProductImageBySKU(_ context.Context, sku string, imagePath, imageLink string) (*catalog.Product, error) {
	if p, ok := m.products[sku]; ok {
		p.Image = imagePath
		p.ImageLink = imageLink
		return p, nil
	}
	return nil, apperr.NotFound("product")
}

func TestDetectImageImportColumns(t *testing.T) {
	tests := []struct {
		name       string
		headers    []string
		wantSKU    int
		wantURLCol int
	}{
		{
			name:       "Arabic headers standard",
			headers:    []string{"كود الصنف", "رابط صورة المنتج", "اسم الصنف"},
			wantSKU:    0,
			wantURLCol: 1,
		},
		{
			name:       "English headers reversed",
			headers:    []string{"Product Title", "Image URL", "SKU Code"},
			wantSKU:    2,
			wantURLCol: 1,
		},
		{
			name:       "Barcode and Link",
			headers:    []string{"الباركود", "رابط الصورة"},
			wantSKU:    0,
			wantURLCol: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotSKU, gotURL := detectImageImportColumns(tt.headers)
			if gotSKU != tt.wantSKU || gotURL != tt.wantURLCol {
				t.Errorf("detectImageImportColumns(%v) = (%d, %d); want (%d, %d)",
					tt.headers, gotSKU, gotURL, tt.wantSKU, tt.wantURLCol)
			}
		})
	}
}

func TestAdminProductImagesSamples(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	handler := &UIHandler{log: logger}

	// Test CSV Sample
	reqCSV := httptest.NewRequest("GET", "/admin/products/images/import/sample.csv", nil)
	wCSV := httptest.NewRecorder()
	handler.AdminProductImagesSampleCSV(wCSV, reqCSV)
	if wCSV.Code != http.StatusOK {
		t.Errorf("AdminProductImagesSampleCSV status = %d; want 200", wCSV.Code)
	}
	if !bytes.Contains(wCSV.Body.Bytes(), []byte("كود الصنف (SKU)")) {
		t.Errorf("CSV sample missing SKU header")
	}

	// Test XLSX Sample
	reqXLSX := httptest.NewRequest("GET", "/admin/products/images/import/sample.xlsx", nil)
	wXLSX := httptest.NewRecorder()
	handler.AdminProductImagesSampleXLSX(wXLSX, reqXLSX)
	if wXLSX.Code != http.StatusOK {
		t.Errorf("AdminProductImagesSampleXLSX status = %d; want 200", wXLSX.Code)
	}
	if wXLSX.Header().Get("Content-Type") != "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet" {
		t.Errorf("XLSX sample content type mismatch: %s", wXLSX.Header().Get("Content-Type"))
	}
}

func TestAdminProductImagesUploadAndMappingFlow(t *testing.T) {
	repo := newMockCatalogImageRepo()
	repo.products["PAN-500"] = &catalog.Product{
		ID:    101,
		SKU:   "PAN-500",
		Name:  i18n.Text{"ar": "بنادول إكسترا 500 مجم", "en": "Panadol Extra 500mg"},
		Price: money.MustParse("50.00"),
	}

	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	catSvc := catalog.NewService(repo, logger)
	handler := &UIHandler{log: logger, catSvc: catSvc}

	// Create a sample test image PNG in memory
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	for x := 0; x < 10; x++ {
		for y := 0; y < 10; y++ {
			img.Set(x, y, color.RGBA{R: 255, G: 0, B: 0, A: 255})
		}
	}
	var imgBuf bytes.Buffer
	_ = png.Encode(&imgBuf, img)

	// Local HTTP server serving test image
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(imgBuf.Bytes())
	}))
	defer testServer.Close()

	// 1. Upload CSV with valid product SKU and server image URL
	csvContent := fmt.Sprintf("كود الصنف,رابط الصورة\nPAN-500,%s/test.png\nUNKNOWN-999,%s/unknown.png\n", testServer.URL, testServer.URL)
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, _ := writer.CreateFormFile("file", "test_images.csv")
	_, _ = part.Write([]byte(csvContent))
	_ = writer.Close()

	req := httptest.NewRequest("POST", "/admin/products/images/import/upload", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	ctx := authctx.WithActor(req.Context(), authctx.Actor{
		UserID:         1,
		OrganizationID: 1,
		Role:           "superadmin",
	})
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	handler.AdminProductImagesUploadSubmit(w, req)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("AdminProductImagesUploadSubmit status = %d; want 303", w.Code)
	}

	redirectURL := w.Header().Get("Location")
	if redirectURL == "" {
		t.Fatalf("AdminProductImagesUploadSubmit missing Location redirect")
	}

	u, _ := url.Parse(redirectURL)
	sessionID := u.Path[len("/admin/products/images/import/"):]

	sess, ok := globalAdminImageImportSessionStore.GetSession(sessionID)
	if !ok {
		t.Fatalf("session %s not found in store", sessionID)
	}

	if sess.TotalRows != 2 {
		t.Errorf("TotalRows = %d; want 2", sess.TotalRows)
	}

	// 2. Submit column mapping & run import pipeline synchronously for test
	form := url.Values{}
	form.Set("sku_col", "0")
	form.Set("url_col", "1")
	reqMap := httptest.NewRequest("POST", "/admin/products/images/import/"+sessionID+"/mapping", strings.NewReader(form.Encode()))
	reqMap.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", sessionID)
	reqMap = reqMap.WithContext(context.WithValue(reqMap.Context(), chi.RouteCtxKey, rctx))
	wMap := httptest.NewRecorder()

	handler.AdminProductImagesMappingSubmit(wMap, reqMap)
	if wMap.Code != http.StatusSeeOther {
		t.Errorf("AdminProductImagesMappingSubmit status = %d; want 303", wMap.Code)
	}

	// Wait briefly for background process
	for i := 0; i < 20; i++ {
		cur, _ := globalAdminImageImportSessionStore.GetSession(sessionID)
		if cur != nil && (cur.Phase == AdminImagePhaseCompleted || cur.Phase == AdminImagePhaseFailed) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	cur, _ := globalAdminImageImportSessionStore.GetSession(sessionID)
	if cur == nil {
		t.Fatalf("session %s gone", sessionID)
	}

	if cur.Phase != AdminImagePhaseCompleted {
		t.Errorf("Phase = %v; want completed", cur.Phase)
	}
	if cur.NotFoundRows != 1 {
		t.Errorf("NotFoundRows = %d; want 1 (for UNKNOWN-999)", cur.NotFoundRows)
	}

	// Cleanup test uploads
	_ = os.RemoveAll(UploadBaseDir)
}
