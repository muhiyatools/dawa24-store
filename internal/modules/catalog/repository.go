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

	CreateVariant(ctx context.Context, v *ProductVariant) error
	GetVariantByID(ctx context.Context, id int64) (*ProductVariant, error)
	ListVariantsByProduct(ctx context.Context, productID int64) ([]*ProductVariant, error)
	UpdateVariant(ctx context.Context, v *ProductVariant) error
	DeleteVariant(ctx context.Context, id int64) error

	CreateCategory(ctx context.Context, c *Category) error
	GetCategoryByID(ctx context.Context, id int64) (*Category, error)
	UpdateCategory(ctx context.Context, c *Category) error
	ListCategories(ctx context.Context) ([]*Category, error)

	CreateBrand(ctx context.Context, b *Brand) error
	GetBrandByID(ctx context.Context, id int64) (*Brand, error)
	UpdateBrand(ctx context.Context, b *Brand) error
	ListBrands(ctx context.Context) ([]*Brand, error)

	SetCustomerPricing(ctx context.Context, m *CustomerProductMapping) error
	GetCustomerPricing(ctx context.Context, vendorOrgID, customerOrgID, productID int64) (*CustomerProductMapping, error)

	CreateProductAlert(ctx context.Context, a *ProductAlert) error
	ListProductAlertsByUser(ctx context.Context, userID int64) ([]*ProductAlert, error)
}
