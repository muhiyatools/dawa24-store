package ui

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
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
func (m *mockCatalogImageRepo) BulkUpsertProducts(_ context.Context, _ []*catalog.Product, _ catalog.BulkWriteOptions) (catalog.BulkWriteResult, error) {
	return catalog.BulkWriteResult{}, nil
}
func (m *mockCatalogImageRepo) GetProductByID(_ context.Context, _ int64) (*catalog.Product, error) {
	return nil, nil
}
func (m *mockCatalogImageRepo) UpdateProduct(_ context.Context, p *catalog.Product) error {
	m.products[p.SKU] = p
	return nil
}
func (m *mockCatalogImageRepo) DeleteProduct(_ context.Context, _ int64) error { return nil }
func (m *mockCatalogImageRepo) SearchProducts(_ context.Context, _ catalog.SearchParams) ([]*catalog.Product, error) {
	return nil, nil
}
func (m *mockCatalogImageRepo) CountProducts(_ context.Context, _ catalog.SearchParams) (int, error) {
	return 0, nil
}
func (m *mockCatalogImageRepo) ListProducts(_ context.Context, _ string, _, _ int) ([]*catalog.Product, error) {
	return nil, nil
}
func (m *mockCatalogImageRepo) SetProductsStatus(_ context.Context, _ []int64, _ catalog.ProductStatus) (int64, error) {
	return 0, nil
}
func (m *mockCatalogImageRepo) CreateVariant(_ context.Context, _ *catalog.ProductVariant) error {
	return nil
}
func (m *mockCatalogImageRepo) GetVariantByID(_ context.Context, _ int64) (*catalog.ProductVariant, error) {
	return nil, nil
}
func (m *mockCatalogImageRepo) GetVariantBySKUOrBarcode(_ context.Context, _ int64, _, _ string) (*catalog.ProductVariant, error) {
	return nil, nil
}
func (m *mockCatalogImageRepo) GetVariantByProductAndOrg(_ context.Context, _, _ int64) (*catalog.ProductVariant, error) {
	return nil, nil
}
func (m *mockCatalogImageRepo) ListVariantsByProduct(_ context.Context, _ int64) ([]*catalog.ProductVariant, error) {
	return nil, nil
}
func (m *mockCatalogImageRepo) ListVariantsByProducts(_ context.Context, _ []int64) ([]*catalog.ProductVariant, error) {
	return nil, nil
}
func (m *mockCatalogImageRepo) ListVariantsByOrganization(_ context.Context, _ int64, _ catalog.VariantSearchParams) ([]*catalog.ProductVariant, int, error) {
	return nil, 0, nil
}
func (m *mockCatalogImageRepo) ListAllVariants(_ context.Context, _ catalog.VariantSearchParams) ([]*catalog.ProductVariant, int, error) {
	return nil, 0, nil
}
func (m *mockCatalogImageRepo) UpdateVariant(_ context.Context, _ *catalog.ProductVariant) error {
	return nil
}
func (m *mockCatalogImageRepo) DeleteVariant(_ context.Context, _ int64) error { return nil }
func (m *mockCatalogImageRepo) DeleteAllVariantsByOrg(_ context.Context, _ int64) (int64, error) {
	return 0, nil
}
func (m *mockCatalogImageRepo) DeleteAllProducts(_ context.Context) (int64, error) { return 0, nil }
func (m *mockCatalogImageRepo) CreateCategory(_ context.Context, _ *catalog.Category) error {
	return nil
}
func (m *mockCatalogImageRepo) GetCategoryByID(_ context.Context, _ int64) (*catalog.Category, error) {
	return nil, nil
}
func (m *mockCatalogImageRepo) UpdateCategory(_ context.Context, _ *catalog.Category) error {
	return nil
}
func (m *mockCatalogImageRepo) DeleteCategory(_ context.Context, _ int64) error { return nil }
func (m *mockCatalogImageRepo) ListCategories(_ context.Context) ([]*catalog.Category, error) {
	return nil, nil
}
func (m *mockCatalogImageRepo) CountProductsByOrg(_ context.Context, _ int64, _ string) (int, error) {
	return 0, nil
}
func (m *mockCatalogImageRepo) CountProductsInCategory(_ context.Context, _ int64) (int, error) {
	return 0, nil
}
func (m *mockCatalogImageRepo) CreateBrand(_ context.Context, _ *catalog.Brand) error { return nil }
func (m *mockCatalogImageRepo) GetBrandByID(_ context.Context, _ int64) (*catalog.Brand, error) {
	return nil, nil
}
func (m *mockCatalogImageRepo) UpdateBrand(_ context.Context, _ *catalog.Brand) error { return nil }
func (m *mockCatalogImageRepo) DeleteBrand(_ context.Context, _ int64) error          { return nil }
func (m *mockCatalogImageRepo) ListBrandsByCategory(_ context.Context, _ int64) ([]*catalog.Brand, error) {
	return nil, nil
}
func (m *mockCatalogImageRepo) BrandInCategory(_ context.Context, _, _ int64) (bool, error) {
	return false, nil
}
func (m *mockCatalogImageRepo) SetBrandCategories(_ context.Context, _ int64, _ []int64) error {
	return nil
}
func (m *mockCatalogImageRepo) ListBrands(_ context.Context) ([]*catalog.Brand, error) {
	return nil, nil
}
func (m *mockCatalogImageRepo) CountProductsInBrand(_ context.Context, _ int64) (int, error) {
	return 0, nil
}
func (m *mockCatalogImageRepo) SetCustomerPricing(_ context.Context, _ *catalog.CustomerProductMapping) error {
	return nil
}
func (m *mockCatalogImageRepo) GetCustomerPricing(_ context.Context, _, _, _ int64) (*catalog.CustomerProductMapping, error) {
	return nil, nil
}
func (m *mockCatalogImageRepo) CreateProductAlert(_ context.Context, _ *catalog.ProductAlert) error {
	return nil
}
func (m *mockCatalogImageRepo) ListProductAlertsByUser(_ context.Context, _ int64) ([]*catalog.ProductAlert, error) {
	return nil, nil
}
func (m *mockCatalogImageRepo) UpsertProductIndex(_ context.Context, _ *catalog.ProductIndexItem) error {
	return nil
}
func (m *mockCatalogImageRepo) DeleteProductIndex(_ context.Context, _ string) error { return nil }
func (m *mockCatalogImageRepo) DeleteProductIndexByProduct(_ context.Context, _ int64) error {
	return nil
}
func (m *mockCatalogImageRepo) SearchProductIndex(_ context.Context, _ catalog.SearchParams) ([]*catalog.ProductIndexItem, error) {
	return nil, nil
}
func (m *mockCatalogImageRepo) RebuildProductIndex(_ context.Context) (int64, error) { return 0, nil }
func (m *mockCatalogImageRepo) CreateSavingProduct(_ context.Context, _ *catalog.SavingProduct) error {
	return nil
}
func (m *mockCatalogImageRepo) UpdateSavingProduct(_ context.Context, _ *catalog.SavingProduct) error {
	return nil
}
func (m *mockCatalogImageRepo) ListSavingProductsByOrg(_ context.Context, _ int64, _, _ int) ([]*catalog.SavingProduct, error) {
	return nil, nil
}
func (m *mockCatalogImageRepo) ListSavingProductsEnriched(_ context.Context, _ int64, _, _ string, _, _ int) ([]*catalog.SavingProductEnriched, *catalog.SavingProductStats, error) {
	return nil, nil, nil
}
func (m *mockCatalogImageRepo) GetSavingProductByID(_ context.Context, _ int64) (*catalog.SavingProduct, error) {
	return nil, nil
}
func (m *mockCatalogImageRepo) DeleteSavingProduct(_ context.Context, _, _ int64) error  { return nil }
func (m *mockCatalogImageRepo) DeleteAllSavingProducts(_ context.Context, _ int64) error { return nil }
func (m *mockCatalogImageRepo) GetProductProviders(_ context.Context, _ int64) ([]*catalog.ProductProviderInfo, error) {
	return nil, nil
}
func (m *mockCatalogImageRepo) BatchUpsertSavingProducts(_ context.Context, _ int64, _ *int64, _ []*catalog.SavingProduct) (int, int, error) {
	return 0, 0, nil
}
func (m *mockCatalogImageRepo) ListAllSavingProductsAdmin(_ context.Context, _ *int64, _ *int64, _, _ string, _, _ int) ([]*catalog.SavingProductAdminView, *catalog.SavingProductAdminStats, error) {
	return nil, nil, nil
}
func (m *mockCatalogImageRepo) ListAllMasterProductsForMatching(_ context.Context) ([]*catalog.CatalogMatchSource, error) {
	return nil, nil
}

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

func (m *mockCatalogImageRepo) ListMatchDecisions(_ context.Context, _ string, _, _ int) ([]*catalog.MatchDecisionView, int, error) {
	return nil, 0, nil
}
func (m *mockCatalogImageRepo) DeleteMatchDecision(_ context.Context, _ int64) error {
	return nil
}
func (m *mockCatalogImageRepo) ClearMatchDecisions(_ context.Context) error {
	return nil
}
func (m *mockCatalogImageRepo) ListMatchDecisionsForOrg(_ context.Context, _ int64, _ string, _, _ int) ([]*catalog.MatchDecisionView, int, error) {
	return nil, 0, nil
}
func (m *mockCatalogImageRepo) DeleteMatchDecisionForOrg(_ context.Context, _, _ int64) error {
	return nil
}
func (m *mockCatalogImageRepo) ClearMatchDecisionsForOrg(_ context.Context, _ int64) error {
	return nil
}
func (m *mockCatalogImageRepo) SaveManualDecision(_ context.Context, _, _ int64, _ string, _ int64, _ string) error {
	return nil
}
func (m *mockCatalogImageRepo) IsDecisionMemoryEnabled(_ context.Context) bool {
	return true
}
func (m *mockCatalogImageRepo) SetDecisionMemoryEnabled(_ context.Context, _ bool) error {
	return nil
}
func (m *mockCatalogImageRepo) ListCustomerMappings(_ context.Context, _ int64, _ string, _, _ int) ([]*catalog.CustomerMappingView, int, error) {
	return nil, 0, nil
}
func (m *mockCatalogImageRepo) DeleteCustomerMapping(_ context.Context, _, _ int64) error {
	return nil
}
func (m *mockCatalogImageRepo) ClearCustomerMappings(_ context.Context, _ int64) error {
	return nil
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
