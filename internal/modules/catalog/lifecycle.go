package catalog

import (
	"context"

	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// Product listing, variant edits and taxonomy deletion.

// ListProducts returns the active organization's own catalogue.
func (s *Service) ListProducts(ctx context.Context, status string, limit, offset int) ([]*Product, error) {
	if _, ok := database.TenantFrom(ctx); !ok {
		return nil, database.ErrNoTenant
	}
	return s.repo.ListProducts(ctx, status, limit, offset)
}

// GetVariant returns one product variant.
func (s *Service) GetVariant(ctx context.Context, id int64) (*ProductVariant, error) {
	return s.repo.GetVariantByID(ctx, id)
}

// UpdateVariant persists changes to a variant.
func (s *Service) UpdateVariant(ctx context.Context, id int64, input *ProductVariant) (*ProductVariant, error) {
	orgID, ok := database.TenantFrom(ctx)
	if !ok {
		return nil, database.ErrNoTenant
	}

	existing, err := s.repo.GetVariantByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if input.Name.IsEmpty() {
		return nil, apperr.Validation("variant.name_required",
			"Variant name is required in at least one language.", nil)
	}
	if input.Price.IsNegative() {
		return nil, apperr.Validation("variant.price_negative",
			"Price cannot be negative.", nil)
	}
	// A discount larger than the price would produce a negative line total at
	// checkout, which money.Allocate would then distribute across shipments.
	if input.Discount.Minor() > input.Price.Minor() {
		return nil, apperr.Validation("variant.discount_exceeds_price",
			"Discount cannot exceed the price.", nil)
	}

	// Identity fields are not taken from the request body: organization and
	// parent product are what the row already says, so a payload cannot move a
	// variant to another tenant or another product.
	existing.Name = input.Name
	existing.SKU = input.SKU
	existing.Barcode = input.Barcode
	existing.Price = input.Price
	existing.CostPrice = input.CostPrice
	existing.Discount = input.Discount
	existing.Unit = input.Unit
	existing.Image = input.Image
	existing.Status = input.Status
	existing.IsFeatured = input.IsFeatured
	existing.OrganizationID = orgID

	if err := s.repo.UpdateVariant(ctx, existing); err != nil {
		return nil, err
	}
	return existing, nil
}

// DeleteVariant soft-deletes a variant.
func (s *Service) DeleteVariant(ctx context.Context, id int64) error {
	if _, ok := database.TenantFrom(ctx); !ok {
		return database.ErrNoTenant
	}
	if _, err := s.repo.GetVariantByID(ctx, id); err != nil {
		return err
	}
	return s.repo.DeleteVariant(ctx, id)
}

// DeleteCategory removes a category that no product references.
//
// Categories and brands are platform taxonomy shared across tenants, so a
// deletion here is visible to every vendor. Refusing while products still point
// at the row prevents one vendor's cleanup from stranding another's catalogue.
func (s *Service) DeleteCategory(ctx context.Context, id int64) error {
	if _, err := s.repo.GetCategoryByID(ctx, id); err != nil {
		return err
	}

	count, err := s.repo.CountProductsInCategory(ctx, id)
	if err != nil {
		return err
	}
	if count > 0 {
		return apperr.Conflict("category.in_use",
			"This category is still used by products. Move them to another category first.")
	}
	return s.repo.DeleteCategory(ctx, id)
}

// DeleteBrand removes a brand that no product references.
func (s *Service) DeleteBrand(ctx context.Context, id int64) error {
	if _, err := s.repo.GetBrandByID(ctx, id); err != nil {
		return err
	}

	count, err := s.repo.CountProductsInBrand(ctx, id)
	if err != nil {
		return err
	}
	if count > 0 {
		return apperr.Conflict("brand.in_use",
			"This brand is still used by products. Reassign them first.")
	}
	return s.repo.DeleteBrand(ctx, id)
}

// SetProductsStatus activates or deactivates many products at once.
//
// Returns how many rows changed. That number can be lower than the number of
// ids supplied: row-level security filters out anything owned by another
// tenant, so the caller learns the count without ever seeing those products.
func (s *Service) SetProductsStatus(ctx context.Context, ids []int64, status ProductStatus) (int64, error) {
	if _, ok := database.TenantFrom(ctx); !ok {
		return 0, database.ErrNoTenant
	}
	if len(ids) == 0 {
		return 0, apperr.Validation("products.none_selected",
			"Select at least one product.", nil)
	}
	// A bulk endpoint with no ceiling is a way to lock a large slice of the
	// table in one statement.
	const maxBulk = 500
	if len(ids) > maxBulk {
		return 0, apperr.Validation("products.too_many",
			"Too many products in one request.", map[string]string{"max": "500"})
	}
	switch status {
	case StatusActive, StatusInactive:
	default:
		return 0, apperr.Validation("products.invalid_status",
			"Status must be active or inactive.", nil)
	}
	return s.repo.SetProductsStatus(ctx, ids, status)
}

// ListBrandsByCategory returns the manufacturers that operate in one category.
//
// The product form offers a category first and then only that category's
// brands. Nothing constrained the pair before, so a product could sit in
// "مستحضرات تجميل" with a brand that only makes medical supplies.
func (s *Service) ListBrandsByCategory(ctx context.Context, categoryID int64) ([]*Brand, error) {
	if categoryID <= 0 {
		return nil, apperr.Validation("catalog.category_required",
			"Category is required.", map[string]string{"category_id": "التصنيف مطلوب"})
	}
	return s.repo.ListBrandsByCategory(ctx, categoryID)
}

// BrandInCategory reports whether a (category, brand) pair is allowed. The
// product form filters client-side for convenience; this is the rule.
func (s *Service) BrandInCategory(ctx context.Context, categoryID, brandID int64) (bool, error) {
	if categoryID <= 0 || brandID <= 0 {
		return false, nil
	}
	return s.repo.BrandInCategory(ctx, categoryID, brandID)
}

// SetBrandCategories replaces the categories a manufacturer operates in.
func (s *Service) SetBrandCategories(ctx context.Context, brandID int64, categoryIDs []int64) error {
	if brandID <= 0 {
		return apperr.Validation("catalog.brand_required",
			"Brand is required.", map[string]string{"brand_id": "الشركة المصنعة مطلوبة"})
	}
	return s.repo.SetBrandCategories(ctx, brandID, categoryIDs)
}
