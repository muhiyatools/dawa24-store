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
	ListOffers(ctx context.Context, limit, offset int) ([]*Offer, error)
	ListOffersVisibleTo(ctx context.Context, latitude, longitude float64, dayOfWeek, limit, offset int, allowedWorkIDs []int64) ([]*VisibleOffer, error)
	SetOfferActive(ctx context.Context, id int64, active bool) error
	IncrementOfferEngagement(ctx context.Context, offerID int64, isClick bool) error

	CreatePackage(ctx context.Context, p *OfferPackage) error
	ListPackages(ctx context.Context) ([]*OfferPackage, error)

	CreateSponsorship(ctx context.Context, s *OfferSponsorship) error
	ListActiveAds(ctx context.Context, position string) ([]*Ad, error)
	RecordAdClick(ctx context.Context, adID int64, userID *int64, ip, ua string) error

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
	DeleteSpecialOffer(ctx context.Context, id, orgID int64) error
	AddSpecialOfferLocation(ctx context.Context, loc *SpecialOfferLocation) error
	ListSpecialOfferLocations(ctx context.Context, offerID int64) ([]*SpecialOfferLocation, error)
}
