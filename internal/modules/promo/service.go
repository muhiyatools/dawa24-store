package promo

import (
	"context"
	"log/slog"
	"time"

	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// Service coordinates vendor promotions, sponsorships, and advertising campaigns.
type Service struct {
	repo Repository
	log  *slog.Logger
}

// NewService creates a new promo service.
func NewService(repo Repository, log *slog.Logger) *Service {
	return &Service{
		repo: repo,
		log:  log,
	}
}

// CreateOffer registers a new promotional discount campaign under the tenant.
func (s *Service) CreateOffer(ctx context.Context, o *Offer) (*Offer, error) {
	orgID, ok := database.TenantFrom(ctx)
	if !ok {
		return nil, database.ErrNoTenant
	}
	o.OrganizationID = orgID

	if err := o.Validate(); err != nil {
		return nil, err
	}
	o.IsActive = true

	if err := s.repo.CreateOffer(ctx, o); err != nil {
		return nil, err
	}

	s.log.InfoContext(ctx, "offer created", "offer_id", o.ID, "org_id", o.OrganizationID)
	return o, nil
}

// GetOffer retrieves an offer by ID.
func (s *Service) GetOffer(ctx context.Context, id int64) (*Offer, error) {
	return s.repo.GetOfferByID(ctx, id)
}

// ListActiveOffers lists all running promotions across the marketplace.
func (s *Service) ListActiveOffers(ctx context.Context, limit, offset int) ([]*Offer, error) {
	return s.repo.ListActiveOffers(ctx, limit, offset)
}

// RecordOfferView tracks an impression for an offer.
func (s *Service) RecordOfferView(ctx context.Context, offerID int64) error {
	return s.repo.IncrementOfferEngagement(ctx, offerID, false)
}

// RecordOfferClick tracks a click event on an offer.
func (s *Service) RecordOfferClick(ctx context.Context, offerID int64) error {
	return s.repo.IncrementOfferEngagement(ctx, offerID, true)
}

// ListPackages returns all available sponsorship packages.
func (s *Service) ListPackages(ctx context.Context) ([]*OfferPackage, error) {
	return s.repo.ListPackages(ctx)
}

// CreatePackage registers a new sponsorship package plan.
func (s *Service) CreatePackage(ctx context.Context, p *OfferPackage) (*OfferPackage, error) {
	if p.Name.IsEmpty() {
		return nil, apperr.Validation("package.name_required", "Package name is required.", nil)
	}
	p.IsActive = true
	if err := s.repo.CreatePackage(ctx, p); err != nil {
		return nil, err
	}
	return p, nil
}

// SponsorOffer sponsors an offer using a package tier.
func (s *Service) SponsorOffer(ctx context.Context, offerID int64, packageID int64, durationDays int) (*OfferSponsorship, error) {
	orgID, ok := database.TenantFrom(ctx)
	if !ok {
		return nil, database.ErrNoTenant
	}

	now := time.Now().UTC()
	if durationDays <= 0 {
		durationDays = 30
	}

	sponsorship := &OfferSponsorship{
		OrganizationID: orgID,
		OfferID:        offerID,
		PackageID:      packageID,
		StartsAt:       now,
		ExpiresAt:      now.Add(time.Duration(durationDays) * 24 * time.Hour),
		Status:         "active",
	}

	if err := s.repo.CreateSponsorship(ctx, sponsorship); err != nil {
		return nil, err
	}

	s.log.InfoContext(ctx, "offer sponsored", "offer_id", offerID, "package_id", packageID)
	return sponsorship, nil
}

// ListActiveAds returns display banners for a designated position.
func (s *Service) ListActiveAds(ctx context.Context, position string) ([]*Ad, error) {
	return s.repo.ListActiveAds(ctx, position)
}

// RecordAdClick logs ad click analytics.
func (s *Service) RecordAdClick(ctx context.Context, adID int64, userID *int64, ip, ua string) error {
	if adID <= 0 {
		return apperr.Validation("ad_id.invalid", "Invalid ad ID", nil)
	}
	return s.repo.RecordAdClick(ctx, adID, userID, ip, ua)
}

// CreateHighlightSection creates a homepage curated highlight section.
func (s *Service) CreateHighlightSection(ctx context.Context, h *HighlightSection) (*HighlightSection, error) {
	if h.Title.IsEmpty() || h.Slug == "" {
		return nil, apperr.Validation("section.invalid", "Title and slug are required.", nil)
	}
	if err := s.repo.CreateHighlightSection(ctx, h); err != nil {
		return nil, err
	}
	return h, nil
}

// ListHighlightSections returns all active curated sections.
func (s *Service) ListHighlightSections(ctx context.Context) ([]*HighlightSection, error) {
	return s.repo.ListHighlightSections(ctx)
}

// ExpirePromotions marks expired offers and sponsorships as inactive.
func (s *Service) ExpirePromotions(ctx context.Context) (int64, error) {
	return s.repo.ExpirePromotions(ctx)
}
