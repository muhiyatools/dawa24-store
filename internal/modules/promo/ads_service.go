package promo

import (
	"context"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"time"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// CreateAd creates a new advertisement for the vendor's organization.
// The ad starts in pending admin_status and must be approved before display.
func (s *Service) CreateAd(ctx context.Context, a *Ad) (*Ad, error) {
	orgID, ok := database.TenantFrom(ctx)
	if !ok {
		return nil, database.ErrNoTenant
	}
	a.OrganizationID = &orgID
	a.AdminStatus = AdminPending
	a.IsActive = false
	if a.Title == "" && a.TitleAr != "" {
		a.Title = a.TitleAr
	}
	if a.Title == "" && a.TitleEn != "" {
		a.Title = a.TitleEn
	}
	if a.Title == "" {
		return nil, apperr.Validation("ad.title_required", i18n.TDefault("w4_mod.w4str_242_242"), nil)
	}
	if a.MediaURL == "" {
		return nil, apperr.Validation("ad.media_required", i18n.TDefault("w4_mod.w4str_243_243"), nil)
	}
	if a.DurationDays <= 0 {
		a.DurationDays = 30
	}
	if a.ExpiresAt.IsZero() {
		a.ExpiresAt = time.Now().UTC().Add(time.Duration(a.DurationDays) * 24 * time.Hour)
	}
	if a.StartsAt.IsZero() {
		a.StartsAt = time.Now().UTC()
	}
	if err := s.repo.CreateAd(ctx, a); err != nil {
		return nil, err
	}
	s.log.InfoContext(ctx, "ad created", "ad_id", a.ID, "org_id", orgID, "position", a.Position)
	return a, nil
}

// UpdateAd updates an existing ad owned by the vendor's organization.
func (s *Service) UpdateAd(ctx context.Context, a *Ad) error {
	orgID, ok := database.TenantFrom(ctx)
	if !ok {
		return database.ErrNoTenant
	}
	existing, err := s.repo.GetAdByID(ctx, a.ID)
	if err != nil {
		return err
	}
	if existing.OrganizationID == nil || *existing.OrganizationID != orgID {
		return apperr.NotFound("ad")
	}
	if a.Title == "" && a.TitleAr != "" {
		a.Title = a.TitleAr
	}
	if a.Title == "" && a.TitleEn != "" {
		a.Title = a.TitleEn
	}
	if a.Title == "" {
		return apperr.Validation("ad.title_required", i18n.TDefault("w4_mod.w4str_242_242"), nil)
	}
	return s.repo.UpdateAd(ctx, a)
}

// GetAd retrieves an ad by ID.
func (s *Service) GetAd(ctx context.Context, id int64) (*Ad, error) {
	return s.repo.GetAdByID(ctx, id)
}

// ListAdsByOrg returns ads for the active tenant.
func (s *Service) ListAdsByOrg(ctx context.Context, limit, offset int) ([]*Ad, error) {
	orgID, ok := database.TenantFrom(ctx)
	if !ok {
		return nil, database.ErrNoTenant
	}
	ads, err := s.repo.ListAdsByOrg(ctx, orgID, limit, offset)
	if err != nil {
		return nil, err
	}
	for _, a := range ads {
		if a != nil {
			a.CTR = ComputeCTR(a.Impressions, a.Clicks)
		}
	}
	return ads, nil
}

// AdminListAds returns all ads for admin moderation.
func (s *Service) AdminListAds(ctx context.Context, limit, offset int) ([]*Ad, error) {
	ads, err := s.repo.ListAllAds(database.AsSystem(ctx), limit, offset)
	if err != nil {
		return nil, err
	}
	for _, a := range ads {
		if a != nil {
			a.CTR = ComputeCTR(a.Impressions, a.Clicks)
		}
	}
	return ads, nil
}

// AdminApproveAd approves an ad for display.
func (s *Service) AdminApproveAd(ctx context.Context, id int64, notes string) error {
	reviewerID, _ := authctx.UserID(ctx)
	return s.repo.UpdateAdAdminStatus(database.AsSystem(ctx), id, AdminApproved, notes, reviewerID)
}

// AdminRejectAd rejects an ad, keeping it invisible.
func (s *Service) AdminRejectAd(ctx context.Context, id int64, notes string) error {
	reviewerID, _ := authctx.UserID(ctx)
	return s.repo.UpdateAdAdminStatus(database.AsSystem(ctx), id, AdminRejected, notes, reviewerID)
}

// RecordAdImpression logs an ad view.
func (s *Service) RecordAdImpression(ctx context.Context, adID int64, userID *int64, ip, ua string) error {
	if adID <= 0 {
		return apperr.Validation("ad_id.invalid", "Invalid ad ID", nil)
	}
	return s.repo.RecordAdImpression(database.AsSystem(ctx), adID, userID, ip, ua)
}

// AdminListPackages returns all sponsorship packages for admin management.
func (s *Service) AdminListPackages(ctx context.Context) ([]*OfferPackage, error) {
	return s.repo.AdminListPackages(database.AsSystem(ctx))
}

// AdminCreatePackage creates a sponsorship package (admin).
func (s *Service) AdminCreatePackage(ctx context.Context, p *OfferPackage) (*OfferPackage, error) {
	if p.Name.IsEmpty() {
		return nil, apperr.Validation("package.name_required", "Package name is required.", nil)
	}
	p.IsActive = true
	if err := s.repo.CreatePackage(database.AsSystem(ctx), p); err != nil {
		return nil, err
	}
	return p, nil
}

// AdminUpdatePackage updates a sponsorship package (admin).
func (s *Service) AdminUpdatePackage(ctx context.Context, p *OfferPackage) (*OfferPackage, error) {
	if p.ID <= 0 || p.Name.IsEmpty() {
		return nil, apperr.Validation("package.invalid", "Package ID and name are required.", nil)
	}
	if err := s.repo.UpdatePackage(database.AsSystem(ctx), p); err != nil {
		return nil, err
	}
	return p, nil
}

// AdminTogglePackageActive toggles a package's active state (admin).
func (s *Service) AdminTogglePackageActive(ctx context.Context, id int64, active bool) error {
	return s.repo.TogglePackageActive(database.AsSystem(ctx), id, active)
}

// AdminGetPackageByID retrieves a single package by ID for admin.
func (s *Service) AdminGetPackageByID(ctx context.Context, id int64) (*OfferPackage, error) {
	return s.repo.GetPackageByID(database.AsSystem(ctx), id)
}
