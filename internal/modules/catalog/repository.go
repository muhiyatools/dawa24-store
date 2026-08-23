package catalog

import (
	"context"

	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// SearchParams holds filters for searching products.
type SearchParams struct {
	Query          string
	OrganizationID *int64
	CategoryID     *int64
	BrandID        *int64
	MinPrice       *money.Amount
	MaxPrice       *money.Amount
	Sort           string // empty, price_asc, price_desc, newest, name
	Limit          int
	Offset         int
	AllowedWorkIDs []int64
	FilterMode     int // 0 = Simple (dashboard/catalog), 1 = WithConnections (purchase requests)
}

// VariantSearchParams holds filters for searching product variants.
type VariantSearchParams struct {
	Query      string
	CategoryID *int64
	Status     string
	Limit      int
	Offset     int
}

// Repository defines the persistence interface for the catalog bounded context.
type Repository interface {
	CreateProduct(ctx context.Context, p *Product) error
	GetProductByID(ctx context.Context, id int64) (*Product, error)
	UpdateProduct(ctx context.Context, p *Product) error
	DeleteProduct(ctx context.Context, id int64) error
	SearchProducts(ctx context.Context, params SearchParams) ([]*Product, error)
	// ListProducts is the vendor's own catalogue, scoped by row-level security
	// to the active organization rather than by a caller-supplied id.
	ListProducts(ctx context.Context, status string, limit, offset int) ([]*Product, error)
	SetProductsStatus(ctx context.Context, ids []int64, status ProductStatus) (int64, error)

	CreateVariant(ctx context.Context, v *ProductVariant) error
	GetVariantByID(ctx context.Context, id int64) (*ProductVariant, error)
	ListVariantsByProduct(ctx context.Context, productID int64) ([]*ProductVariant, error)
	ListVariantsByOrganization(ctx context.Context, orgID int64, params VariantSearchParams) ([]*ProductVariant, int, error)
	UpdateVariant(ctx context.Context, v *ProductVariant) error
	DeleteVariant(ctx context.Context, id int64) error

	CreateCategory(ctx context.Context, c *Category) error
	GetCategoryByID(ctx context.Context, id int64) (*Category, error)
	UpdateCategory(ctx context.Context, c *Category) error
	DeleteCategory(ctx context.Context, id int64) error
	ListCategories(ctx context.Context) ([]*Category, error)
	// CountProductsInCategory backs the refusal to delete a taxonomy row that
	// products still reference; deleting one would leave them uncategorised
	// with no way to find them in the vendor UI.
	CountProductsByOrg(ctx context.Context, orgID int64, status string) (int, error)
	CountProductsInCategory(ctx context.Context, categoryID int64) (int, error)

	CreateBrand(ctx context.Context, b *Brand) error
	GetBrandByID(ctx context.Context, id int64) (*Brand, error)
	UpdateBrand(ctx context.Context, b *Brand) error
	DeleteBrand(ctx context.Context, id int64) error
	// Category -> brand relationship (PLAN_V7 Phase 4). The product form offers
	// a category first, then only that category's manufacturers.
	ListBrandsByCategory(ctx context.Context, categoryID int64) ([]*Brand, error)
	BrandInCategory(ctx context.Context, categoryID, brandID int64) (bool, error)
	SetBrandCategories(ctx context.Context, brandID int64, categoryIDs []int64) error

	ListBrands(ctx context.Context) ([]*Brand, error)
	CountProductsInBrand(ctx context.Context, brandID int64) (int, error)

	SetCustomerPricing(ctx context.Context, m *CustomerProductMapping) error
	GetCustomerPricing(ctx context.Context, vendorOrgID, customerOrgID, productID int64) (*CustomerProductMapping, error)

	CreateProductAlert(ctx context.Context, a *ProductAlert) error
	ListProductAlertsByUser(ctx context.Context, userID int64) ([]*ProductAlert, error)

	GetFirstFinderQuestion(ctx context.Context) (*FinderQuestion, error)
	GetFinderQuestionByID(ctx context.Context, id int64) (*FinderQuestion, error)
	ListFinderOptions(ctx context.Context, questionID int64) ([]*FinderOption, error)
	GetFinderResultByID(ctx context.Context, id int64) (*FinderResult, error)
	ListFinderQuestions(ctx context.Context) ([]*FinderQuestion, error)

	CreateFinderQuestion(ctx context.Context, q *FinderQuestion) error
	CreateFinderOption(ctx context.Context, o *FinderOption) error
	CreateFinderResult(ctx context.Context, r *FinderResult) error
	ListFinderResults(ctx context.Context) ([]*FinderResult, error)

	// Denormalized read model (catalog.product_index)
	UpsertProductIndex(ctx context.Context, item *ProductIndexItem) error
	DeleteProductIndex(ctx context.Context, uniqueRowID string) error
	DeleteProductIndexByProduct(ctx context.Context, productID int64) error
	SearchProductIndex(ctx context.Context, params SearchParams) ([]*ProductIndexItem, error)
	RebuildProductIndex(ctx context.Context) (int64, error)

	// Saving Products (catalog.saving_products)
	CreateSavingProduct(ctx context.Context, sp *SavingProduct) error
	UpdateSavingProduct(ctx context.Context, sp *SavingProduct) error
	ListSavingProductsByOrg(ctx context.Context, orgID int64, limit, offset int) ([]*SavingProduct, error)
	ListSavingProductsEnriched(ctx context.Context, orgID int64, search string, filter string, limit, offset int) ([]*SavingProductEnriched, *SavingProductStats, error)
	GetSavingProductByID(ctx context.Context, id int64) (*SavingProduct, error)
	DeleteSavingProduct(ctx context.Context, id, orgID int64) error
	GetProductProviders(ctx context.Context, productID int64) ([]*ProductProviderInfo, error)
	BatchUpsertSavingProducts(ctx context.Context, orgID int64, userID *int64, items []*SavingProduct) (added, updated int, err error)
	ListAllSavingProductsAdmin(ctx context.Context, userID *int64, orgID *int64, search string, filter string, limit, offset int) ([]*SavingProductAdminView, *SavingProductAdminStats, error)
}
