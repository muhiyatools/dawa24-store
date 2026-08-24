package catalog

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// Service coordinates product catalog business logic.
type Service struct {
	repo     Repository
	log      *slog.Logger
	instGate InstitutionalGate
	// imports and enricher back the reviewed catalogue import. Both are
	// optional: without a store the wizard is unavailable, and without an
	// enricher the AI switch is simply not offered.
	imports  ImportSessionStore
	enricher Enricher
}

// SetInstitutionalGate installs the institutional work filter gate.
func (s *Service) SetInstitutionalGate(gate InstitutionalGate) {
	s.instGate = gate
}

// NewService creates a new catalog service.
func NewService(repo Repository, log *slog.Logger) *Service {
	return &Service{
		repo: repo,
		log:  log,
	}
}

// CreateProduct creates a new product under the tenant bound to the request context.
func (s *Service) CreateProduct(ctx context.Context, p *Product) (*Product, error) {
	if p.OrganizationID == 0 {
		orgID, ok := database.TenantFrom(ctx)
		if ok && orgID > 0 {
			p.OrganizationID = orgID
		}
	}

	if err := p.Validate(); err != nil {
		return nil, err
	}
	if err := s.assertBrandInCategory(ctx, p); err != nil {
		return nil, err
	}

	if p.Status == "" {
		p.Status = StatusActive
	}

	if err := s.repo.CreateProduct(ctx, p); err != nil {
		return nil, err
	}

	s.log.InfoContext(ctx, "product created", "product_id", p.ID, "org_id", p.OrganizationID)
	return p, nil
}

// maxImportBatch bounds one upload. Ten thousand rows is roughly the largest
// real distributor catalogue and already takes several seconds inside a single
// transaction; beyond that the request would outlive its own timeout and hold
// write locks on catalog.products the whole time.
const maxImportBatch = 20000

// BulkImportProducts writes a parsed batch into the master catalogue.
//
// Rows are validated here, before the write, so a bad value is reported against
// its product rather than aborting a transaction halfway through. Products that
// fail validation are dropped from the batch and named in the returned issues;
// the write itself stays all-or-nothing.
func (s *Service) BulkImportProducts(ctx context.Context, prods []*Product) (BulkWriteResult, []RowIssue, error) {
	empty := BulkWriteResult{Matches: map[int]MatchReason{}}
	if len(prods) == 0 {
		return empty, nil, nil
	}
	if len(prods) > maxImportBatch {
		return empty, nil, apperr.Validation("catalog.import_too_large",
			fmt.Sprintf("الملف يحتوي على %d صنف، والحد الأقصى المسموح به في عملية الاستيراد الواحدة هو %d صنف. يرجى تقسيم الملف.",
				len(prods), maxImportBatch), nil)
	}

	valid := make([]*Product, 0, len(prods))
	var issues []RowIssue

	for i, p := range prods {
		if p == nil {
			continue
		}
		if err := p.Validate(); err != nil {
			issues = append(issues, RowIssue{
				Row:      i + 1,
				Value:    p.Name.Get(i18n.AR),
				Message:  s.validationMessage(err),
				Severity: SeverityError,
			})
			continue
		}
		valid = append(valid, p)
	}

	if len(valid) == 0 {
		return empty, issues, apperr.Validation("catalog.import_no_valid_rows",
			"لا يوجد أي صنف صالح للاستيراد بعد التحقق من البيانات.", nil)
	}

	res, err := s.repo.BulkUpsertProducts(ctx, valid)
	if err != nil {
		s.log.ErrorContext(ctx, "bulk import products failed",
			"count", len(valid), "failures", len(res.Failures), "error", err)
		return res, issues, err
	}

	s.log.InfoContext(ctx, "bulk import products completed",
		"inserted", res.Inserted, "updated", res.Updated,
		"brands_created", res.BrandsCreated, "submitted", len(valid))
	return res, issues, nil
}

// validationMessage renders a domain validation failure for the import report,
// preferring the Arabic message the rule carries.
func (s *Service) validationMessage(err error) string {
	var appErr *apperr.Error
	if errors.As(err, &appErr) && appErr.Msg != "" {
		return appErr.Msg
	}
	return err.Error()
}

// GetProduct retrieves a product and its associated active variants.
func (s *Service) GetProduct(ctx context.Context, id int64) (*Product, []*ProductVariant, error) {
	p, err := s.repo.GetProductByID(ctx, id)
	if err != nil {
		return nil, nil, err
	}

	variants, err := s.repo.ListVariantsByProduct(ctx, id)
	if err != nil {
		return nil, nil, err
	}

	return p, variants, nil
}

// UpdateProduct updates product details.
func (s *Service) UpdateProduct(ctx context.Context, p *Product) error {
	if err := p.Validate(); err != nil {
		return err
	}
	if err := s.assertBrandInCategory(ctx, p); err != nil {
		return err
	}
	return s.repo.UpdateProduct(ctx, p)
}

// assertBrandInCategory refuses a product whose manufacturer does not operate
// in its category. The product form filters the brand list client-side for
// convenience; this is the rule, so a crafted or stale form cannot save a
// cosmetics product under a medical-supplies-only manufacturer.
//
// A product with no category or no brand is left alone: both columns are
// nullable and plenty of legacy rows carry only one.
func (s *Service) assertBrandInCategory(ctx context.Context, p *Product) error {
	if p.CategoryID == nil || p.BrandID == nil || *p.CategoryID <= 0 || *p.BrandID <= 0 {
		return nil
	}
	ok, err := s.repo.BrandInCategory(ctx, *p.CategoryID, *p.BrandID)
	if err != nil {
		return err
	}
	if !ok {
		return apperr.Validation("catalog.brand_category_mismatch",
			"The selected manufacturer does not operate in the selected category.",
			map[string]string{"brand_id": "الشركة المصنعة المختارة لا تعمل ضمن التصنيف المحدد"})
	}
	return nil
}

// DeleteProduct soft-deletes a product.
func (s *Service) DeleteProduct(ctx context.Context, id int64) error {
	return s.repo.DeleteProduct(ctx, id)
}

// Search runs full Arabic/English search across the product catalogue.
func (s *Service) Search(ctx context.Context, params SearchParams) ([]*Product, error) {
	if s.instGate != nil && len(params.AllowedWorkIDs) == 0 {
		if uid, err := authctx.UserID(ctx); err == nil && uid > 0 {
			works, err := s.instGate.AllowedWorkIDs(ctx, uid, params.FilterMode)
			if err == nil {
				params.AllowedWorkIDs = works
			}
		}
	}
	return s.repo.SearchProducts(ctx, params)
}

// Count returns the total count of products matching search filters.
func (s *Service) Count(ctx context.Context, params SearchParams) (int, error) {
	if s.instGate != nil && len(params.AllowedWorkIDs) == 0 {
		if uid, err := authctx.UserID(ctx); err == nil && uid > 0 {
			works, err := s.instGate.AllowedWorkIDs(ctx, uid, params.FilterMode)
			if err == nil {
				params.AllowedWorkIDs = works
			}
		}
	}
	return s.repo.CountProducts(ctx, params)
}

// SearchWithTotal searches products and returns total matching count for pagination.
func (s *Service) SearchWithTotal(ctx context.Context, params SearchParams) ([]*Product, int, error) {
	if s.instGate != nil && len(params.AllowedWorkIDs) == 0 {
		if uid, err := authctx.UserID(ctx); err == nil && uid > 0 {
			works, err := s.instGate.AllowedWorkIDs(ctx, uid, params.FilterMode)
			if err == nil {
				params.AllowedWorkIDs = works
			}
		}
	}
	prods, err := s.repo.SearchProducts(ctx, params)
	if err != nil {
		return nil, 0, err
	}
	total, err := s.repo.CountProducts(ctx, params)
	if err != nil {
		return prods, len(prods), nil
	}
	return prods, total, nil
}

// FastSearch searches the denormalized read model (catalog.product_index) with deterministic fallback (Rule R3).
// If the indexed table returns 0 results or errors, it falls back to direct catalog.products table search.
func (s *Service) FastSearch(ctx context.Context, params SearchParams) ([]*ProductIndexItem, error) {
	if s.instGate != nil && len(params.AllowedWorkIDs) == 0 {
		if uid, err := authctx.UserID(ctx); err == nil && uid > 0 {
			works, err := s.instGate.AllowedWorkIDs(ctx, uid, params.FilterMode)
			if err == nil {
				params.AllowedWorkIDs = works
			}
		}
	}

	items, err := s.repo.SearchProductIndex(ctx, params)
	if err == nil && len(items) > 0 {
		return items, nil
	}

	// Deterministic fallback (Rule R3): Fallback to catalog.products
	products, fbErr := s.repo.SearchProducts(ctx, params)
	if fbErr != nil {
		if err != nil {
			return nil, err
		}
		return nil, fbErr
	}

	fallbackItems := make([]*ProductIndexItem, 0, len(products))
	for _, p := range products {
		nameAr := p.Name.Get("ar")
		nameEn := p.Name.Get("en")
		hasDisc := p.Discount.IsPositive()
		finalPrice := p.EffectivePrice()
		var discPct float64
		if p.Price.IsPositive() && hasDisc {
			discPct = float64(p.Discount.Minor()) / float64(p.Price.Minor()) * 100.0
		}

		fallbackItems = append(fallbackItems, &ProductIndexItem{
			UniqueRowID:          ComposeUniqueRowID(p.ID, nil, p.BranchID),
			ProductID:            p.ID,
			SKU:                  p.SKU,
			NameAR:               nameAr,
			NameEN:               nameEn,
			ScientificName:       p.ScientificName,
			Price:                p.Price,
			Discount:             p.Discount,
			StockQuantity:        0,
			CategoryID:           p.CategoryID,
			BrandID:              p.BrandID,
			HasDiscount:          hasDisc,
			DiscountPercentage:   discPct,
			PriceAfterDiscount:   finalPrice,
			OrganizationID:       p.OrganizationID,
			BranchID:             p.BranchID,
			Status:               string(p.Status),
			ProductType:          "parent",
			InstitutionalWorkIDs: p.InstitutionalWorkIDs,
			CreatedAt:            p.CreatedAt,
			UpdatedAt:            p.UpdatedAt,
		})
	}
	return fallbackItems, nil
}

// RebuildProductIndex runs a full sweep rebuild of the denormalized read model.
func (s *Service) RebuildProductIndex(ctx context.Context) (int64, error) {
	return s.repo.RebuildProductIndex(ctx)
}

// CreateVariant creates a product variant.
func (s *Service) CreateVariant(ctx context.Context, v *ProductVariant) (*ProductVariant, error) {
	orgID, ok := database.TenantFrom(ctx)
	if !ok {
		return nil, database.ErrNoTenant
	}
	v.OrganizationID = orgID

	if v.ProductID <= 0 {
		return nil, apperr.Validation("variant.product_required", "Parent product ID is required.", nil)
	}
	if v.Name.IsEmpty() {
		return nil, apperr.Validation("variant.name_required", "Variant name is required.", nil)
	}
	if v.Status == "" {
		v.Status = StatusActive
	}

	if err := s.repo.CreateVariant(ctx, v); err != nil {
		return nil, err
	}

	s.log.InfoContext(ctx, "variant created", "variant_id", v.ID, "product_id", v.ProductID)
	return v, nil
}

// GetVariantBySKUOrBarcode retrieves a variant by SKU or Barcode within an organization.
func (s *Service) GetVariantBySKUOrBarcode(ctx context.Context, orgID int64, sku, barcode string) (*ProductVariant, error) {
	return s.repo.GetVariantBySKUOrBarcode(ctx, orgID, sku, barcode)
}

// GetVariantByProductAndOrg retrieves a variant by parent product ID and organization ID.
func (s *Service) GetVariantByProductAndOrg(ctx context.Context, orgID int64, productID int64) (*ProductVariant, error) {
	return s.repo.GetVariantByProductAndOrg(ctx, orgID, productID)
}

// ListVariantsByProduct retrieves all variants for a parent product.
func (s *Service) ListVariantsByProduct(ctx context.Context, productID int64) ([]*ProductVariant, error) {
	return s.repo.ListVariantsByProduct(ctx, productID)
}

// ListVariantsByOrganization retrieves all variants sold by a supplier with pagination and search.
func (s *Service) ListVariantsByOrganization(ctx context.Context, orgID int64, params VariantSearchParams) ([]*ProductVariant, int, error) {
	return s.repo.ListVariantsByOrganization(ctx, orgID, params)
}

// ListAllVariants retrieves all variants across all vendors with pagination and search (for admin panel).
func (s *Service) ListAllVariants(ctx context.Context, params VariantSearchParams) ([]*ProductVariant, int, error) {
	return s.repo.ListAllVariants(ctx, params)
}

// ListCategories returns all categories.
func (s *Service) ListCategories(ctx context.Context) ([]*Category, error) {
	return s.repo.ListCategories(ctx)
}

// CreateCategory adds a category.
func (s *Service) CreateCategory(ctx context.Context, c *Category) (*Category, error) {
	if c.Name.IsEmpty() {
		return nil, apperr.Validation("category.name_required", "Category name is required.", nil)
	}
	if err := s.repo.CreateCategory(ctx, c); err != nil {
		return nil, err
	}
	return c, nil
}

// GetCategory retrieves a category.
func (s *Service) GetCategory(ctx context.Context, id int64) (*Category, error) {
	return s.repo.GetCategoryByID(ctx, id)
}

// UpdateCategory modifies category data.
func (s *Service) UpdateCategory(ctx context.Context, c *Category) error {
	return s.repo.UpdateCategory(ctx, c)
}

// CountProductsInCategory returns the number of products linked to a category.
func (s *Service) CountProductsInCategory(ctx context.Context, categoryID int64) (int, error) {
	return s.repo.CountProductsInCategory(ctx, categoryID)
}

// ListBrands returns all brands.
func (s *Service) ListBrands(ctx context.Context) ([]*Brand, error) {
	return s.repo.ListBrands(ctx)
}

// CreateBrand adds a brand.
func (s *Service) CreateBrand(ctx context.Context, b *Brand) (*Brand, error) {
	if b.Name.IsEmpty() {
		return nil, apperr.Validation("brand.name_required", "Brand name is required.", nil)
	}
	if err := s.repo.CreateBrand(ctx, b); err != nil {
		return nil, err
	}
	return b, nil
}

// GetBrand retrieves a brand.
func (s *Service) GetBrand(ctx context.Context, id int64) (*Brand, error) {
	return s.repo.GetBrandByID(ctx, id)
}

// UpdateBrand modifies brand data.
func (s *Service) UpdateBrand(ctx context.Context, b *Brand) error {
	return s.repo.UpdateBrand(ctx, b)
}

// CountProductsInBrand returns the number of active products linked to a brand.
func (s *Service) CountProductsInBrand(ctx context.Context, brandID int64) (int, error) {
	return s.repo.CountProductsInBrand(ctx, brandID)
}

// SetCustomerPricing stores customer-specific custom price or discount terms.
func (s *Service) SetCustomerPricing(ctx context.Context, m *CustomerProductMapping) error {
	if m.OrganizationID <= 0 || m.CustomerOrgID == nil || *m.CustomerOrgID <= 0 || m.ProductID <= 0 {
		return apperr.Validation("mapping.invalid", "Organization, customer, and product IDs are required.", nil)
	}
	return s.repo.SetCustomerPricing(ctx, m)
}

// GetCustomerPricing looks up custom pricing for a customer.
func (s *Service) GetCustomerPricing(ctx context.Context, vendorOrgID, customerOrgID, productID int64) (*CustomerProductMapping, error) {
	return s.repo.GetCustomerPricing(ctx, vendorOrgID, customerOrgID, productID)
}

// CreateProductAlert registers a price/stock alert.
func (s *Service) CreateProductAlert(ctx context.Context, a *ProductAlert) (*ProductAlert, error) {
	if a.UserID <= 0 || a.ProductID <= 0 || a.AlertType == "" {
		return nil, apperr.Validation("alert.invalid", "User ID, product ID, and alert type are required.", nil)
	}
	if err := s.repo.CreateProductAlert(ctx, a); err != nil {
		return nil, err
	}
	return a, nil
}

// ListProductAlerts returns active alerts for a user.
func (s *Service) ListProductAlerts(ctx context.Context, userID int64) ([]*ProductAlert, error) {
	return s.repo.ListProductAlertsByUser(ctx, userID)
}

// CountProductsByOrg returns an organization's product total for a status.
// Pass an empty status for every product.
func (s *Service) CountProductsByOrg(ctx context.Context, orgID int64, status string) (int, error) {
	return s.repo.CountProductsByOrg(ctx, orgID, status)
}

// CreateSavingProduct registers a saving target product.
func (s *Service) CreateSavingProduct(ctx context.Context, sp *SavingProduct) error {
	if sp.NameProduct == "" {
		return apperr.Validation("name_product.required", "اسم المنتج مطلوب.", nil)
	}
	return s.repo.CreateSavingProduct(ctx, sp)
}

// UpdateSavingProduct updates an existing saving product.
func (s *Service) UpdateSavingProduct(ctx context.Context, sp *SavingProduct) error {
	if sp.NameProduct == "" {
		return apperr.Validation("name_product.required", "اسم المنتج مطلوب.", nil)
	}
	return s.repo.UpdateSavingProduct(ctx, sp)
}

// ListSavingProducts returns saving products for an organization.
func (s *Service) ListSavingProducts(ctx context.Context, orgID int64, limit, offset int) ([]*SavingProduct, error) {
	return s.repo.ListSavingProductsByOrg(ctx, orgID, limit, offset)
}

// ListSavingProductsEnriched returns enriched saving products with linked details and counts.
func (s *Service) ListSavingProductsEnriched(ctx context.Context, orgID int64, search, filter string, limit, offset int) ([]*SavingProductEnriched, *SavingProductStats, error) {
	return s.repo.ListSavingProductsEnriched(ctx, orgID, search, filter, limit, offset)
}

// GetSavingProduct returns a single saving product by ID.
func (s *Service) GetSavingProduct(ctx context.Context, id int64) (*SavingProduct, error) {
	return s.repo.GetSavingProductByID(ctx, id)
}

// DeleteSavingProduct removes a saving product record.
func (s *Service) DeleteSavingProduct(ctx context.Context, id, orgID int64) error {
	return s.repo.DeleteSavingProduct(ctx, id, orgID)
}

// GetProductProviders returns all suppliers and variants selling a master catalog product.
func (s *Service) GetProductProviders(ctx context.Context, productID int64) ([]*ProductProviderInfo, error) {
	return s.repo.GetProductProviders(ctx, productID)
}

// BatchUpsertSavingProducts inserts or updates saving products in bulk for an organization.
func (s *Service) BatchUpsertSavingProducts(ctx context.Context, orgID int64, userID *int64, items []*SavingProduct) (added, updated int, err error) {
	return s.repo.BatchUpsertSavingProducts(ctx, orgID, userID, items)
}

// ListAllSavingProductsAdmin returns all saving products across all users and organizations for platform administration.
func (s *Service) ListAllSavingProductsAdmin(ctx context.Context, userID *int64, orgID *int64, search string, filter string, limit, offset int) ([]*SavingProductAdminView, *SavingProductAdminStats, error) {
	return s.repo.ListAllSavingProductsAdmin(ctx, userID, orgID, search, filter, limit, offset)
}
