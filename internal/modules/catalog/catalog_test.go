package catalog_test

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

type mockCatalogRepo struct {
	products map[int64]*catalog.Product
	variants map[int64]*catalog.ProductVariant
	nextID   int64
}

func newMockCatalogRepo() *mockCatalogRepo {
	return &mockCatalogRepo{
		products: map[int64]*catalog.Product{},
		variants: map[int64]*catalog.ProductVariant{},
		nextID:   1,
	}
}

func (m *mockCatalogRepo) CreateProduct(_ context.Context, p *catalog.Product) error {
	p.ID = m.nextID
	m.nextID++
	m.products[p.ID] = p
	return nil
}

func (m *mockCatalogRepo) GetProductByID(_ context.Context, id int64) (*catalog.Product, error) {
	p, ok := m.products[id]
	if !ok {
		return nil, apperr.NotFound("product")
	}
	return p, nil
}

func (m *mockCatalogRepo) UpdateProduct(_ context.Context, p *catalog.Product) error {
	m.products[p.ID] = p
	return nil
}

func (m *mockCatalogRepo) DeleteProduct(_ context.Context, id int64) error {
	delete(m.products, id)
	return nil
}

func (m *mockCatalogRepo) SearchProducts(_ context.Context, params catalog.SearchParams) ([]*catalog.Product, error) {
	var list []*catalog.Product
	for _, p := range m.products {
		list = append(list, p)
	}
	return list, nil
}

func (m *mockCatalogRepo) CreateVariant(_ context.Context, v *catalog.ProductVariant) error {
	v.ID = m.nextID
	m.nextID++
	m.variants[v.ID] = v
	return nil
}

func (m *mockCatalogRepo) GetVariantByID(_ context.Context, id int64) (*catalog.ProductVariant, error) {
	v, ok := m.variants[id]
	if !ok {
		return nil, apperr.NotFound("product_variant")
	}
	return v, nil
}

func (m *mockCatalogRepo) ListVariantsByProduct(_ context.Context, productID int64) ([]*catalog.ProductVariant, error) {
	var list []*catalog.ProductVariant
	for _, v := range m.variants {
		if v.ProductID == productID {
			list = append(list, v)
		}
	}
	return list, nil
}

func (m *mockCatalogRepo) UpdateVariant(_ context.Context, v *catalog.ProductVariant) error {
	m.variants[v.ID] = v
	return nil
}

func (m *mockCatalogRepo) DeleteVariant(_ context.Context, id int64) error {
	delete(m.variants, id)
	return nil
}

func (m *mockCatalogRepo) CreateCategory(_ context.Context, c *catalog.Category) error { return nil }
func (m *mockCatalogRepo) GetCategoryByID(_ context.Context, id int64) (*catalog.Category, error) {
	return nil, apperr.NotFound("category")
}
func (m *mockCatalogRepo) UpdateCategory(_ context.Context, c *catalog.Category) error { return nil }
func (m *mockCatalogRepo) ListCategories(_ context.Context) ([]*catalog.Category, error) {
	return []*catalog.Category{}, nil
}
func (m *mockCatalogRepo) CreateBrand(_ context.Context, b *catalog.Brand) error { return nil }
func (m *mockCatalogRepo) GetBrandByID(_ context.Context, id int64) (*catalog.Brand, error) {
	return nil, apperr.NotFound("brand")
}
func (m *mockCatalogRepo) UpdateBrand(_ context.Context, b *catalog.Brand) error { return nil }
func (m *mockCatalogRepo) ListBrands(_ context.Context) ([]*catalog.Brand, error) {
	return []*catalog.Brand{}, nil
}

func (m *mockCatalogRepo) SetCustomerPricing(_ context.Context, cm *catalog.CustomerProductMapping) error {
	return nil
}
func (m *mockCatalogRepo) GetCustomerPricing(_ context.Context, vendorOrgID, customerOrgID, productID int64) (*catalog.CustomerProductMapping, error) {
	return nil, apperr.NotFound("customer_pricing")
}
func (m *mockCatalogRepo) CreateProductAlert(_ context.Context, a *catalog.ProductAlert) error {
	return nil
}
func (m *mockCatalogRepo) ListProductAlertsByUser(_ context.Context, userID int64) ([]*catalog.ProductAlert, error) {
	return nil, nil
}

func TestProductEffectivePrice(t *testing.T) {
	p := &catalog.Product{
		Price:    money.MustParse("100.00"),
		Discount: money.MustParse("15.50"),
	}

	effective := p.EffectivePrice()
	expected := money.MustParse("84.50")
	if effective != expected {
		t.Errorf("EffectivePrice = %v; want %v", effective, expected)
	}
}

func TestCatalogServiceCreateAndVariants(t *testing.T) {
	ctx := database.WithTenant(context.Background(), 42)
	repo := newMockCatalogRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := catalog.NewService(repo, logger)

	// 1. Create Product
	p, err := svc.CreateProduct(ctx, &catalog.Product{
		Name:           i18n.New("بانادول اكسترا", "Panadol Extra"),
		Price:          money.MustParse("45.00"),
		DosageForm:     "Tablet",
		ScientificName: "Paracetamol + Caffeine",
	})
	if err != nil {
		t.Fatalf("CreateProduct failed: %v", err)
	}

	if p.ID == 0 || p.OrganizationID != 42 {
		t.Errorf("Product creation metadata incorrect: id=%d, org=%d", p.ID, p.OrganizationID)
	}

	// 2. Create Variant
	v, err := svc.CreateVariant(ctx, &catalog.ProductVariant{
		ProductID: p.ID,
		Name:      i18n.New("شريط 12 قرص", "Strip of 12 tablets"),
		Price:     money.MustParse("22.50"),
		CostPrice: money.MustParse("18.00"),
	})
	if err != nil {
		t.Fatalf("CreateVariant failed: %v", err)
	}

	// 3. Get Product with Variants
	retrievedProduct, variants, err := svc.GetProduct(ctx, p.ID)
	if err != nil {
		t.Fatalf("GetProduct failed: %v", err)
	}
	if retrievedProduct.ID != p.ID {
		t.Errorf("retrieved product ID = %d; want %d", retrievedProduct.ID, p.ID)
	}
	if len(variants) != 1 || variants[0].ID != v.ID {
		t.Errorf("expected 1 variant with ID %d, got %d variants", v.ID, len(variants))
	}
}
