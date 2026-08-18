package catalog

import (
	"context"
	"log/slog"

	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// Service coordinates product catalog business logic.
type Service struct {
	repo Repository
	log  *slog.Logger
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
		} else {
			p.OrganizationID = 1 // Default platform master catalog org
		}
	}

	if err := p.Validate(); err != nil {
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
	return s.repo.UpdateProduct(ctx, p)
}

// DeleteProduct soft-deletes a product.
func (s *Service) DeleteProduct(ctx context.Context, id int64) error {
	return s.repo.DeleteProduct(ctx, id)
}

// Search runs full Arabic/English search across the product catalogue.
func (s *Service) Search(ctx context.Context, params SearchParams) ([]*Product, error) {
	return s.repo.SearchProducts(ctx, params)
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

// ListVariantsByProduct retrieves all variants for a parent product.
func (s *Service) ListVariantsByProduct(ctx context.Context, productID int64) ([]*ProductVariant, error) {
	return s.repo.ListVariantsByProduct(ctx, productID)
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

// GetFirstFinderQuestion returns the questionnaire entry question.
func (s *Service) GetFirstFinderQuestion(ctx context.Context) (*FinderQuestion, error) {
	return s.repo.GetFirstFinderQuestion(ctx)
}

// GetFinderQuestion returns one question.
func (s *Service) GetFinderQuestion(ctx context.Context, id int64) (*FinderQuestion, error) {
	return s.repo.GetFinderQuestionByID(ctx, id)
}

// ListFinderOptions returns a question's answer choices.
func (s *Service) ListFinderOptions(ctx context.Context, questionID int64) ([]*FinderOption, error) {
	return s.repo.ListFinderOptions(ctx, questionID)
}

// GetFinderResult returns the terminal recommendation.
func (s *Service) GetFinderResult(ctx context.Context, id int64) (*FinderResult, error) {
	return s.repo.GetFinderResultByID(ctx, id)
}

// ListFinderQuestions returns all questions (for the admin tree builder).
func (s *Service) ListFinderQuestions(ctx context.Context) ([]*FinderQuestion, error) {
	return s.repo.ListFinderQuestions(ctx)
}

// CreateFinderQuestion adds a question to the finder questionnaire.
func (s *Service) CreateFinderQuestion(ctx context.Context, q *FinderQuestion) error {
	return s.repo.CreateFinderQuestion(ctx, q)
}

// CreateFinderOption adds an answer choice to a question.
func (s *Service) CreateFinderOption(ctx context.Context, o *FinderOption) error {
	return s.repo.CreateFinderOption(ctx, o)
}

// CreateFinderResult adds a terminal recommendation.
func (s *Service) CreateFinderResult(ctx context.Context, r *FinderResult) error {
	return s.repo.CreateFinderResult(ctx, r)
}

// ListFinderResults returns all terminal recommendations.
func (s *Service) ListFinderResults(ctx context.Context) ([]*FinderResult, error) {
	return s.repo.ListFinderResults(ctx)
}

// CountProductsByOrg returns an organization's product total for a status.
// Pass an empty status for every product.
func (s *Service) CountProductsByOrg(ctx context.Context, orgID int64, status string) (int, error) {
	return s.repo.CountProductsByOrg(ctx, orgID, status)
}
