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
	orgID, ok := database.TenantFrom(ctx)
	if !ok {
		return nil, database.ErrNoTenant
	}
	p.OrganizationID = orgID

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

// ListCategories returns all categories.
func (s *Service) ListCategories(ctx context.Context) ([]*Category, error) {
	return s.repo.ListCategories(ctx)
}

// ListBrands returns all brands.
func (s *Service) ListBrands(ctx context.Context) ([]*Brand, error) {
	return s.repo.ListBrands(ctx)
}
