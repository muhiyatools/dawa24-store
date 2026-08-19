package promo

import (
	"context"
	"log/slog"
	"time"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// Service coordinates vendor promotions, sponsorships, and advertising campaigns.
type Service struct {
	repo Repository
	log  *slog.Logger

	reqDocs  RequiredDocsChecker
	instGate InstitutionalGate
}

// RequiredDocsChecker is injected from composition root (Rebuild V2 §4.2): it
// must return an error when the organization cannot trade (missing mandatory
// documents). The promo module cannot import the attachments module, so the
// checker is a plain function set by cmd/server.
type RequiredDocsChecker func(ctx context.Context, orgID int64, orgType string) error

// SetRequiredDocsChecker installs the §4.2 documents gate. When set, offer
// creation refuses vendors with missing mandatory documents.
func (s *Service) SetRequiredDocsChecker(fn RequiredDocsChecker) {
	s.reqDocs = fn
}

// SetInstitutionalGate installs the institutional work filter gate.
func (s *Service) SetInstitutionalGate(gate InstitutionalGate) {
	s.instGate = gate
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
	if s.reqDocs != nil {
		if err := s.reqDocs(ctx, orgID, "vendor"); err != nil {
			return nil, err
		}
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

// ListOffersForProduct lists the approved, running offers selling a product.
func (s *Service) ListOffersForProduct(ctx context.Context, productID int64) ([]*OfferProductWithOffer, error) {
	return s.repo.ListOffersForProduct(ctx, productID)
}

// ListOffersVisibleTo lists the offers a pharmacy branch can buy: vendor
// branches whose weekly coverage contains the pharmacy branch coordinates.
func (s *Service) ListOffersVisibleTo(ctx context.Context, latitude, longitude float64, dayOfWeek, limit, offset int) ([]*VisibleOffer, error) {
	var allowedWorks []int64
	if s.instGate != nil {
		if uid, err := authctx.UserID(ctx); err == nil && uid > 0 {
			works, err := s.instGate.AllowedWorkIDs(ctx, uid, 0) // Simple mode
			if err == nil {
				allowedWorks = works
			}
		}
	}
	return s.repo.ListOffersVisibleTo(ctx, latitude, longitude, dayOfWeek, limit, offset, allowedWorks)
}

// ListOffers returns all offers for admin moderation.
func (s *Service) ListOffers(ctx context.Context, limit, offset int) ([]*Offer, error) {
	return s.repo.ListOffers(ctx, limit, offset)
}

// SetOfferActive activates or deactivates an offer.
func (s *Service) SetOfferActive(ctx context.Context, id int64, active bool) error {
	return s.repo.SetOfferActive(ctx, id, active)
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
	if h.OwnerType == "" {
		h.OwnerType = "platform"
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

// CreateOrganizationHighlightSection adds an organization-owned merchandising
// row on its storefront (066 — org.highlight_sections merged into promo).
func (s *Service) CreateOrganizationHighlightSection(ctx context.Context, orgID int64, title i18n.Text, slug string) (*HighlightSection, error) {
	if title.IsEmpty() {
		return nil, apperr.Validation("highlight.title_required", "A highlight section title is required.", nil)
	}
	sec := &HighlightSection{
		OwnerType:      "organization",
		OrganizationID: &orgID,
		Title:          title,
		Slug:           slug,
		IsActive:       true,
	}
	if err := s.repo.CreateHighlightSection(ctx, sec); err != nil {
		return nil, err
	}
	return sec, nil
}

// ListHighlightSectionsByOrg returns an organization's merchandising rows.
func (s *Service) ListHighlightSectionsByOrg(ctx context.Context, orgID int64) ([]*HighlightSection, error) {
	return s.repo.ListHighlightSectionsByOrg(ctx, orgID)
}

// AddHighlightItem adds a product or offer to a highlight section.
func (s *Service) AddHighlightItem(ctx context.Context, sectionID int64, productID, offerID *int64) error {
	item := &HighlightSectionItem{SectionID: sectionID, ProductID: productID, OfferID: offerID}
	return s.repo.AddHighlightItem(ctx, item)
}

// ListHighlightItems returns a section's items.
func (s *Service) ListHighlightItems(ctx context.Context, sectionID int64) ([]*HighlightSectionItem, error) {
	return s.repo.ListHighlightItems(ctx, sectionID)
}

// ExpirePromotions runs the automated expiry sweeps.
func (s *Service) ExpirePromotions(ctx context.Context) (int64, error) {
	return s.repo.ExpirePromotions(ctx)
}

// CreateSpecialOffer creates a special offer under tenant organization.
func (s *Service) CreateSpecialOffer(ctx context.Context, o *SpecialOffer) (*SpecialOffer, error) {
	if o.OrganizationID <= 0 {
		orgID, ok := database.TenantFrom(ctx)
		if !ok {
			return nil, database.ErrNoTenant
		}
		o.OrganizationID = orgID
	}
	if err := s.repo.CreateSpecialOffer(ctx, o); err != nil {
		return nil, err
	}
	return o, nil
}

// GetSpecialOffer retrieves a special offer by ID with details.
func (s *Service) GetSpecialOffer(ctx context.Context, id int64) (*SpecialOffer, error) {
	return s.repo.GetSpecialOfferByID(ctx, id)
}

// ListSpecialOffersByOrg lists all special offers for an organization.
func (s *Service) ListSpecialOffersByOrg(ctx context.Context, orgID int64) ([]*SpecialOffer, error) {
	return s.repo.ListSpecialOffersByOrg(ctx, orgID)
}

// DeleteSpecialOffer deletes a special offer.
func (s *Service) DeleteSpecialOffer(ctx context.Context, id, orgID int64) error {
	return s.repo.DeleteSpecialOffer(ctx, id, orgID)
}

// AddSpecialOfferLocation adds a geographic coverage record.
func (s *Service) AddSpecialOfferLocation(ctx context.Context, loc *SpecialOfferLocation) error {
	return s.repo.AddSpecialOfferLocation(ctx, loc)
}

// ListSpecialOfferLocations lists coverage locations for an offer.
func (s *Service) ListSpecialOfferLocations(ctx context.Context, offerID int64) ([]*SpecialOfferLocation, error) {
	return s.repo.ListSpecialOfferLocations(ctx, offerID)
}
