package catalog

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"testing"

	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

type mockCatalogRepo struct {
	products        map[int64]*Product
	variants        map[int64]*ProductVariant
	categories      map[int64]*Category
	brands          map[int64]*Brand
	customerPricing map[string]*CustomerProductMapping
	alerts          map[int64]*ProductAlert
	nextID          int64
}

func newMockCatalogRepo() *mockCatalogRepo {
	return &mockCatalogRepo{
		products:        map[int64]*Product{},
		variants:        map[int64]*ProductVariant{},
		categories:      map[int64]*Category{},
		brands:          map[int64]*Brand{},
		customerPricing: map[string]*CustomerProductMapping{},
		alerts:          map[int64]*ProductAlert{},
		nextID:          1,
	}
}

func (m *mockCatalogRepo) CreateProduct(_ context.Context, p *Product) error {
	p.ID = m.nextID
	m.nextID++
	m.products[p.ID] = p
	return nil
}

func (m *mockCatalogRepo) GetProductByID(_ context.Context, id int64) (*Product, error) {
	p, ok := m.products[id]
	if !ok {
		return nil, apperr.NotFound("product")
	}
	return p, nil
}

func (m *mockCatalogRepo) UpdateProduct(_ context.Context, p *Product) error {
	m.products[p.ID] = p
	return nil
}

func (m *mockCatalogRepo) DeleteProduct(_ context.Context, id int64) error {
	delete(m.products, id)
	return nil
}

func (m *mockCatalogRepo) SearchProducts(_ context.Context, params SearchParams) ([]*Product, error) {
	var list []*Product
	for _, p := range m.products {
		list = append(list, p)
	}
	return list, nil
}

func (m *mockCatalogRepo) CreateVariant(_ context.Context, v *ProductVariant) error {
	v.ID = m.nextID
	m.nextID++
	m.variants[v.ID] = v
	return nil
}

func (m *mockCatalogRepo) GetVariantByID(_ context.Context, id int64) (*ProductVariant, error) {
	v, ok := m.variants[id]
	if !ok {
		return nil, apperr.NotFound("product_variant")
	}
	return v, nil
}

func (m *mockCatalogRepo) ListVariantsByProduct(_ context.Context, productID int64) ([]*ProductVariant, error) {
	var list []*ProductVariant
	for _, v := range m.variants {
		if v.ProductID == productID {
			list = append(list, v)
		}
	}
	return list, nil
}

func (m *mockCatalogRepo) UpdateVariant(_ context.Context, v *ProductVariant) error {
	m.variants[v.ID] = v
	return nil
}

func (m *mockCatalogRepo) DeleteVariant(_ context.Context, id int64) error {
	delete(m.variants, id)
	return nil
}

func (m *mockCatalogRepo) CreateCategory(_ context.Context, c *Category) error {
	c.ID = m.nextID
	m.nextID++
	m.categories[c.ID] = c
	return nil
}

func (m *mockCatalogRepo) GetCategoryByID(_ context.Context, id int64) (*Category, error) {
	c, ok := m.categories[id]
	if !ok {
		return nil, apperr.NotFound("category")
	}
	return c, nil
}

func (m *mockCatalogRepo) UpdateCategory(_ context.Context, c *Category) error {
	m.categories[c.ID] = c
	return nil
}

func (m *mockCatalogRepo) ListCategories(_ context.Context) ([]*Category, error) {
	var list []*Category
	for _, c := range m.categories {
		list = append(list, c)
	}
	return list, nil
}

func (m *mockCatalogRepo) CreateBrand(_ context.Context, b *Brand) error {
	b.ID = m.nextID
	m.nextID++
	m.brands[b.ID] = b
	return nil
}

func (m *mockCatalogRepo) GetBrandByID(_ context.Context, id int64) (*Brand, error) {
	b, ok := m.brands[id]
	if !ok {
		return nil, apperr.NotFound("brand")
	}
	return b, nil
}

func (m *mockCatalogRepo) UpdateBrand(_ context.Context, b *Brand) error {
	m.brands[b.ID] = b
	return nil
}

func (m *mockCatalogRepo) ListBrands(_ context.Context) ([]*Brand, error) {
	var list []*Brand
	for _, b := range m.brands {
		list = append(list, b)
	}
	return list, nil
}

func (m *mockCatalogRepo) SetCustomerPricing(_ context.Context, cm *CustomerProductMapping) error {
	key := fmt.Sprintf("%d:%d:%d", cm.OrganizationID, deref64(cm.CustomerOrgID), cm.ProductID)
	m.customerPricing[key] = cm
	return nil
}

func (m *mockCatalogRepo) GetCustomerPricing(_ context.Context, vendorOrgID, customerOrgID, productID int64) (*CustomerProductMapping, error) {
	key := fmt.Sprintf("%d:%d:%d", vendorOrgID, customerOrgID, productID)
	cm, ok := m.customerPricing[key]
	if !ok {
		return nil, apperr.NotFound("customer_pricing")
	}
	return cm, nil
}

func deref64(p *int64) int64 {
	if p == nil {
		return 0
	}
	return *p
}

func (m *mockCatalogRepo) CreateProductAlert(_ context.Context, a *ProductAlert) error {
	a.ID = m.nextID
	m.nextID++
	m.alerts[a.ID] = a
	return nil
}

func (m *mockCatalogRepo) ListProductAlertsByUser(_ context.Context, userID int64) ([]*ProductAlert, error) {
	var list []*ProductAlert
	for _, a := range m.alerts {
		if a.UserID == userID {
			list = append(list, a)
		}
	}
	return list, nil
}

func (m *mockCatalogRepo) GetFirstFinderQuestion(_ context.Context) (*FinderQuestion, error) {
	return nil, nil
}

func (m *mockCatalogRepo) GetFinderQuestionByID(_ context.Context, _ int64) (*FinderQuestion, error) {
	return nil, nil
}

func (m *mockCatalogRepo) ListFinderOptions(_ context.Context, _ int64) ([]*FinderOption, error) {
	return nil, nil
}

func (m *mockCatalogRepo) GetFinderResultByID(_ context.Context, _ int64) (*FinderResult, error) {
	return nil, nil
}

func (m *mockCatalogRepo) ListFinderQuestions(_ context.Context) ([]*FinderQuestion, error) {
	return nil, nil
}

func (m *mockCatalogRepo) CreateFinderQuestion(_ context.Context, q *FinderQuestion) error {
	q.ID = 1
	return nil
}

func (m *mockCatalogRepo) CreateFinderOption(_ context.Context, o *FinderOption) error {
	o.ID = 1
	return nil
}

func (m *mockCatalogRepo) CreateFinderResult(_ context.Context, r *FinderResult) error {
	r.ID = 1
	return nil
}

func (m *mockCatalogRepo) ListFinderResults(_ context.Context) ([]*FinderResult, error) {
	return nil, nil
}

func (m *mockCatalogRepo) UpsertProductIndex(_ context.Context, item *ProductIndexItem) error {
	return nil
}

func (m *mockCatalogRepo) DeleteProductIndex(_ context.Context, _ string) error {
	return nil
}

func (m *mockCatalogRepo) DeleteProductIndexByProduct(_ context.Context, _ int64) error {
	return nil
}

func (m *mockCatalogRepo) SearchProductIndex(_ context.Context, _ SearchParams) ([]*ProductIndexItem, error) {
	return nil, nil // returns empty to trigger deterministic fallback in tests
}

func (m *mockCatalogRepo) RebuildProductIndex(_ context.Context) (int64, error) {
	return int64(len(m.products)), nil
}

func TestProductEffectivePrice(t *testing.T) {
	p := &Product{
		Price:    money.MustParse("100.00"),
		Discount: money.MustParse("15.50"),
	}

	effective := p.EffectivePrice()
	expected := money.MustParse("84.50")
	if effective != expected {
		t.Errorf("EffectivePrice = %v; want %v", effective, expected)
	}
}

// T1: Unique row ID composition matches Laravel format exactly
func TestComposeUniqueRowID(t *testing.T) {
	cases := []struct {
		productID int64
		variantID *int64
		branchID  *int64
		expected  string
	}{
		{10, nil, nil, "p_10"},
		{10, ptr(int64(20)), nil, "p_10_v_20"},
		{10, ptr(int64(20)), ptr(int64(30)), "p_10_v_20_b_30"},
		{10, nil, ptr(int64(30)), "p_10_b_30"},
	}

	for _, c := range cases {
		got := ComposeUniqueRowID(c.productID, c.variantID, c.branchID)
		if got != c.expected {
			t.Errorf("ComposeUniqueRowID(%d, %v, %v) = %q, want %q", c.productID, c.variantID, c.branchID, got, c.expected)
		}
	}
}

// T11: Deterministic fallback — when the read index is empty, FastSearch returns products from master table
func TestFastSearchDeterministicFallback(t *testing.T) {
	ctx := context.Background()
	repo := newMockCatalogRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := NewService(repo, logger)

	// Add product to master catalogue
	p := &Product{
		OrganizationID: 1,
		Name:           i18n.New("بانادول", "Panadol"),
		Price:          money.MustParse("50.00"),
		Status:         StatusActive,
	}
	_ = repo.CreateProduct(ctx, p)

	// FastSearch with empty read index
	results, err := svc.FastSearch(ctx, SearchParams{Query: "بانادول"})
	if err != nil {
		t.Fatalf("FastSearch failed: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected fallback to return 1 item, got %d", len(results))
	}
	expectedID := p.ID
	expectedRowID := fmt.Sprintf("p_%d", p.ID)
	if results[0].ProductID != expectedID || results[0].UniqueRowID != expectedRowID {
		t.Errorf("expected product %d (%s), got %+v", expectedID, expectedRowID, results[0])
	}
}

func ptr[T any](v T) *T {
	return &v
}

func TestCatalogServiceCreateAndVariants(t *testing.T) {
	ctx := database.WithTenant(context.Background(), 42)
	repo := newMockCatalogRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := NewService(repo, logger)

	// 1. Create Product
	p, err := svc.CreateProduct(ctx, &Product{
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
	v, err := svc.CreateVariant(ctx, &ProductVariant{
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
