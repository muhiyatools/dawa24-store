package promo

import (
	"context"
)

// Repository defines the persistence interface for promotional offers and ads.
type Repository interface {
	CreateOffer(ctx context.Context, o *Offer) error
	GetOfferByID(ctx context.Context, id int64) (*Offer, error)
	ListActiveOffers(ctx context.Context, limit, offset int) ([]*Offer, error)
	ListOffersForProduct(ctx context.Context, productID int64) ([]*OfferProductWithOffer, error)
	// ListOffersForProducts returns the approved running offers selling any of
	// the given products in one query; callers group by op.product_id.
	ListOffersForProducts(ctx context.Context, productIDs []int64) ([]*OfferProductWithOffer, error)
	ListOffers(ctx context.Context, limit, offset int) ([]*Offer, error)
	ListOffersVisibleTo(ctx context.Context, latitude, longitude float64, dayOfWeek, limit, offset int, allowedWorkIDs []int64) ([]*VisibleOffer, error)
	SetOfferActive(ctx context.Context, id int64, active bool) error
	IncrementOfferEngagement(ctx context.Context, offerID int64, isClick bool) error

	CreatePackage(ctx context.Context, p *OfferPackage) error
	UpdatePackage(ctx context.Context, p *OfferPackage) error
	GetPackageByID(ctx context.Context, id int64) (*OfferPackage, error)
	ListPackages(ctx context.Context) ([]*OfferPackage, error)
	AdminListPackages(ctx context.Context) ([]*OfferPackage, error)
	TogglePackageActive(ctx context.Context, id int64, active bool) error

	// Sponsorship purchases — vendor acquires a package and receives credits.
	CreateSponsorshipPurchase(ctx context.Context, p *SponsorshipPurchase) error
	GetSponsorshipPurchaseByID(ctx context.Context, id int64) (*SponsorshipPurchase, error)
	ListSponsorshipPurchasesByOrg(ctx context.Context, orgID int64) ([]*SponsorshipPurchase, error)
	ListActiveSponsorshipPurchasesByOrg(ctx context.Context, orgID int64) ([]*SponsorshipPurchase, error)
	IncrementSponsorshipPurchaseCreditsUsed(ctx context.Context, purchaseID int64, credits int) error
	ExpireSponsorshipPurchases(ctx context.Context) (int64, error)

	// Sponsorship requests — the approval workflow.
	CreateSponsorshipRequest(ctx context.Context, r *SponsorshipRequest) error
	GetSponsorshipRequestByID(ctx context.Context, id int64) (*SponsorshipRequest, error)
	ListSponsorshipRequestsByOrg(ctx context.Context, orgID int64, limit, offset int) ([]*SponsorshipRequest, error)
	ListSponsorshipRequestsByOrgWithTotal(ctx context.Context, orgID int64, limit, offset int) ([]*SponsorshipRequest, int, error)
	ListAllSponsorshipRequests(ctx context.Context, limit, offset int) ([]*SponsorshipRequest, error)
	ListAllSponsorshipRequestsWithTotal(ctx context.Context, limit, offset int) ([]*SponsorshipRequest, int, error)
	ListPendingSponsorshipRequests(ctx context.Context, limit, offset int) ([]*SponsorshipRequest, error)
	UpdateSponsorshipRequestAdminStatus(ctx context.Context, id int64, status AdminStatus, notes string, reviewerID int64) error
	ActivateSponsorshipRequest(ctx context.Context, id int64, reviewerID int64) (*SponsorshipRequest, error)
	CancelSponsorshipRequest(ctx context.Context, id, orgID int64) error
	ExpireSponsorshipRequests(ctx context.Context) (int64, error)

	// Sponsored ranking — returns the active, approved sponsorships ranked by
	// package tier level for the given item IDs.
	RankedSponsorshipsForProducts(ctx context.Context, productIDs []int64) ([]*RankedSponsorship, error)
	RankedSponsorshipsForOffers(ctx context.Context, offerIDs []int64) ([]*RankedSponsorship, error)
	IsSponsored(ctx context.Context, itemType SponsorshipItemType, itemID int64) (*RankedSponsorship, error)

	CreateSponsorship(ctx context.Context, s *OfferSponsorship) error

	// Advertisements — vendor creates, admin approves, tracking records.
	CreateAd(ctx context.Context, a *Ad) error
	UpdateAd(ctx context.Context, a *Ad) error
	GetAdByID(ctx context.Context, id int64) (*Ad, error)
	ListAdsByOrg(ctx context.Context, orgID int64, limit, offset int) ([]*Ad, error)
	ListAdsByOrgWithTotal(ctx context.Context, orgID int64, limit, offset int) ([]*Ad, int, error)
	ListAllAds(ctx context.Context, limit, offset int) ([]*Ad, error)
	ListAllAdsWithTotal(ctx context.Context, limit, offset int) ([]*Ad, int, error)
	ListActiveAds(ctx context.Context, position string) ([]*Ad, error)
	UpdateAdAdminStatus(ctx context.Context, id int64, status AdminStatus, notes string, reviewerID int64) error
	RecordAdClick(ctx context.Context, adID int64, userID *int64, ip, ua string) error
	RecordAdImpression(ctx context.Context, adID int64, userID *int64, ip, ua string) error

	CreateHighlightSection(ctx context.Context, h *HighlightSection) error
	UpdateHighlightSection(ctx context.Context, h *HighlightSection) error
	DeleteHighlightSection(ctx context.Context, id, orgID int64) error
	GetHighlightSectionByID(ctx context.Context, id int64) (*HighlightSection, error)
	ListHighlightSections(ctx context.Context) ([]*HighlightSection, error)
	ListHighlightSectionsByOrg(ctx context.Context, orgID int64) ([]*HighlightSection, error)
	AddHighlightItem(ctx context.Context, item *HighlightSectionItem) error
	ListHighlightItems(ctx context.Context, sectionID int64) ([]*HighlightSectionItem, error)
	ExpirePromotions(ctx context.Context) (int64, error)

	// Laravel-parity Special Offers & Location Covers
	CreateSpecialOffer(ctx context.Context, o *SpecialOffer) error
	GetSpecialOfferByID(ctx context.Context, id int64) (*SpecialOffer, error)
	ListSpecialOffersByOrg(ctx context.Context, orgID int64) ([]*SpecialOffer, error)
	ListAllSpecialOffers(ctx context.Context, limit, offset int) ([]*SpecialOffer, error)
	ListAllSpecialOffersWithTotal(ctx context.Context, statusFilter string, limit, offset int) ([]*SpecialOffer, int, error)
	UpdateSpecialOfferAdminStatus(ctx context.Context, id int64, adminStatus, notes string, approvedBy int64) error
	ToggleSpecialOfferStatus(ctx context.Context, id int64, isActive bool) error
	DeleteSpecialOffer(ctx context.Context, id, orgID int64) error
	AddSpecialOfferLocation(ctx context.Context, loc *SpecialOfferLocation) error
	ListSpecialOfferLocations(ctx context.Context, offerID int64) ([]*SpecialOfferLocation, error)
}
