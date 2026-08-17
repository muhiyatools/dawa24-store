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
	CountProductsInCategory(ctx context.Context, categoryID int64) (int, error)

	CreateBrand(ctx context.Context, b *Brand) error
	GetBrandByID(ctx context.Context, id int64) (*Brand, error)
	UpdateBrand(ctx context.Context, b *Brand) error
	DeleteBrand(ctx context.Context, id int64) error
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
}
