package promo

import (
	"context"
)

// Repository defines the persistence interface for promotional offers and ads.
type Repository interface {
	CreateOffer(ctx context.Context, o *Offer) error
	GetOfferByID(ctx context.Context, id int64) (*Offer, error)
	ListActiveOffers(ctx context.Context, limit, offset int) ([]*Offer, error)
	ListOffers(ctx context.Context, limit, offset int) ([]*Offer, error)
	SetOfferActive(ctx context.Context, id int64, active bool) error
	IncrementOfferEngagement(ctx context.Context, offerID int64, isClick bool) error

	CreatePackage(ctx context.Context, p *OfferPackage) error
	ListPackages(ctx context.Context) ([]*OfferPackage, error)

	CreateSponsorship(ctx context.Context, s *OfferSponsorship) error
	ListActiveAds(ctx context.Context, position string) ([]*Ad, error)
	RecordAdClick(ctx context.Context, adID int64, userID *int64, ip, ua string) error

	CreateHighlightSection(ctx context.Context, h *HighlightSection) error
	ListHighlightSections(ctx context.Context) ([]*HighlightSection, error)
	ExpirePromotions(ctx context.Context) (int64, error)
}
