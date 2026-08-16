package promo_test

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/promo"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

type mockPromoRepo struct {
	offers       map[int64]*promo.Offer
	packages     map[int64]*promo.OfferPackage
	sponsorships map[int64]*promo.OfferSponsorship
	ads          map[int64]*promo.Ad
	nextID       int64
}

func newMockPromoRepo() *mockPromoRepo {
	return &mockPromoRepo{
		offers:       map[int64]*promo.Offer{},
		packages:     map[int64]*promo.OfferPackage{},
		sponsorships: map[int64]*promo.OfferSponsorship{},
		ads:          map[int64]*promo.Ad{},
		nextID:       1,
	}
}

func (m *mockPromoRepo) CreateOffer(_ context.Context, o *promo.Offer) error {
	o.ID = m.nextID
	m.nextID++
	m.offers[o.ID] = o
	return nil
}

func (m *mockPromoRepo) GetOfferByID(_ context.Context, id int64) (*promo.Offer, error) {
	o, ok := m.offers[id]
	if !ok {
		return nil, apperr.NotFound("offer")
	}
	return o, nil
}

func (m *mockPromoRepo) ListActiveOffers(_ context.Context, limit, offset int) ([]*promo.Offer, error) {
	var list []*promo.Offer
	for _, o := range m.offers {
		if o.IsActive {
			list = append(list, o)
		}
	}
	return list, nil
}

func (m *mockPromoRepo) IncrementOfferEngagement(_ context.Context, offerID int64, isClick bool) error {
	o, ok := m.offers[offerID]
	if !ok {
		return apperr.NotFound("offer")
	}
	if isClick {
		o.ClicksCount++
	} else {
		o.ViewsCount++
	}
	return nil
}

func (m *mockPromoRepo) CreatePackage(_ context.Context, p *promo.OfferPackage) error {
	p.ID = m.nextID
	m.nextID++
	m.packages[p.ID] = p
	return nil
}

func (m *mockPromoRepo) ListPackages(_ context.Context) ([]*promo.OfferPackage, error) {
	var list []*promo.OfferPackage
	for _, p := range m.packages {
		list = append(list, p)
	}
	return list, nil
}

func (m *mockPromoRepo) CreateSponsorship(_ context.Context, s *promo.OfferSponsorship) error {
	s.ID = m.nextID
	m.nextID++
	m.sponsorships[s.ID] = s
	return nil
}

func (m *mockPromoRepo) ListActiveAds(_ context.Context, position string) ([]*promo.Ad, error) {
	var list []*promo.Ad
	for _, a := range m.ads {
		if a.IsActive && (position == "" || a.Position == position) {
			list = append(list, a)
		}
	}
	return list, nil
}

func (m *mockPromoRepo) RecordAdClick(_ context.Context, adID int64, userID *int64, ip, ua string) error {
	a, ok := m.ads[adID]
	if !ok {
		return apperr.NotFound("ad")
	}
	a.Clicks++
	return nil
}

func TestOfferDiscounts(t *testing.T) {
	now := time.Now()

	// 1. 15% Percentage Discount
	percentOffer := &promo.Offer{
		DiscountType:  promo.DiscountPercentage,
		DiscountValue: money.MustParse("15.00"), // 15% (1500 basis points)
		IsActive:      true,
		StartsAt:      now.Add(-time.Hour),
		ExpiresAt:     now.Add(time.Hour),
	}

	disc, err := percentOffer.CalculateDiscount(money.MustParse("200.00"))
	if err != nil {
		t.Fatalf("CalculateDiscount failed: %v", err)
	}
	expectedDisc := money.MustParse("30.00")
	if disc != expectedDisc {
		t.Errorf("Percentage discount = %v; want %v", disc, expectedDisc)
	}

	// 2. Minimum order value requirement
	minValOffer := &promo.Offer{
		DiscountType:  promo.DiscountFixed,
		DiscountValue: money.MustParse("50.00"),
		MinOrderValue: money.MustParse("500.00"),
		IsActive:      true,
	}

	// Below min order value
	disc, _ = minValOffer.CalculateDiscount(money.MustParse("300.00"))
	if !disc.IsZero() {
		t.Errorf("expected 0 discount below min order value, got %v", disc)
	}

	// Above min order value
	disc, _ = minValOffer.CalculateDiscount(money.MustParse("600.00"))
	if disc != money.MustParse("50.00") {
		t.Errorf("expected 50.00 discount, got %v", disc)
	}
}

func TestPromoServiceCreateAndEngage(t *testing.T) {
	ctx := database.WithTenant(context.Background(), 15)
	repo := newMockPromoRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := promo.NewService(repo, logger)

	now := time.Now()
	offer, err := svc.CreateOffer(ctx, &promo.Offer{
		Title:         i18n.New("خصم الصيف 10%", "Summer 10% Discount"),
		DiscountType:  promo.DiscountPercentage,
		DiscountValue: money.MustParse("10.00"),
		StartsAt:      now,
		ExpiresAt:     now.Add(7 * 24 * time.Hour),
	})
	if err != nil {
		t.Fatalf("CreateOffer failed: %v", err)
	}

	// Track 2 impressions and 1 click
	_ = svc.RecordOfferView(ctx, offer.ID)
	_ = svc.RecordOfferView(ctx, offer.ID)
	_ = svc.RecordOfferClick(ctx, offer.ID)

	retrieved, _ := svc.GetOffer(ctx, offer.ID)
	if retrieved.ViewsCount != 2 || retrieved.ClicksCount != 1 {
		t.Errorf("expected views=2, clicks=1; got views=%d, clicks=%d", retrieved.ViewsCount, retrieved.ClicksCount)
	}
}
