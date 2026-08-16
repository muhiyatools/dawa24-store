package promo

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

type mockPromoRepo struct {
	offers       map[int64]*Offer
	packages     map[int64]*OfferPackage
	sponsorships map[int64]*OfferSponsorship
	ads          map[int64]*Ad
	sections     map[int64]*HighlightSection
	nextID       int64
}

func newMockPromoRepo() *mockPromoRepo {
	return &mockPromoRepo{
		offers:       map[int64]*Offer{},
		packages:     map[int64]*OfferPackage{},
		sponsorships: map[int64]*OfferSponsorship{},
		ads:          map[int64]*Ad{},
		sections:     map[int64]*HighlightSection{},
		nextID:       1,
	}
}

func (m *mockPromoRepo) CreateOffer(_ context.Context, o *Offer) error {
	o.ID = m.nextID
	m.nextID++
	m.offers[o.ID] = o
	return nil
}

func (m *mockPromoRepo) GetOfferByID(_ context.Context, id int64) (*Offer, error) {
	o, ok := m.offers[id]
	if !ok {
		return nil, apperr.NotFound("offer")
	}
	return o, nil
}

func (m *mockPromoRepo) ListActiveOffers(_ context.Context, limit, offset int) ([]*Offer, error) {
	var list []*Offer
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

func (m *mockPromoRepo) CreatePackage(_ context.Context, p *OfferPackage) error {
	p.ID = m.nextID
	m.nextID++
	m.packages[p.ID] = p
	return nil
}

func (m *mockPromoRepo) ListPackages(_ context.Context) ([]*OfferPackage, error) {
	var list []*OfferPackage
	for _, p := range m.packages {
		list = append(list, p)
	}
	return list, nil
}

func (m *mockPromoRepo) CreateSponsorship(_ context.Context, s *OfferSponsorship) error {
	s.ID = m.nextID
	m.nextID++
	m.sponsorships[s.ID] = s
	return nil
}

func (m *mockPromoRepo) ListActiveAds(_ context.Context, position string) ([]*Ad, error) {
	var list []*Ad
	for _, a := range m.ads {
		if a.IsActive && (position == "" || a.Position == position) {
			list = append(list, a)
		}
	}
	return list, nil
}

func (m *mockPromoRepo) RecordAdClick(_ context.Context, adID int64, userID *int64, ip, ua string) error {
	if a, ok := m.ads[adID]; ok {
		a.Clicks++
	}
	return nil
}

func (m *mockPromoRepo) CreateHighlightSection(_ context.Context, h *HighlightSection) error {
	h.ID = m.nextID
	m.nextID++
	m.sections[h.ID] = h
	return nil
}

func (m *mockPromoRepo) ListHighlightSections(_ context.Context) ([]*HighlightSection, error) {
	var list []*HighlightSection
	for _, s := range m.sections {
		list = append(list, s)
	}
	return list, nil
}

func (m *mockPromoRepo) ExpirePromotions(_ context.Context) (int64, error) {
	return 0, nil
}

func TestPromoServiceLifecycle(t *testing.T) {
	ctx := database.WithTenant(context.Background(), 42)
	repo := newMockPromoRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := NewService(repo, logger)

	// 1. Create Offer
	discVal, _ := money.Parse("10.00")
	minOrder, _ := money.Parse("100.00")
	now := time.Now().UTC()

	o := &Offer{
		Title:         i18n.New("خصم الصيف", "Summer Discount"),
		DiscountType:  DiscountPercentage,
		DiscountValue: discVal,
		MinOrderValue: minOrder,
		StartsAt:      now,
		ExpiresAt:     now.Add(7 * 24 * time.Hour),
		IsActive:      true,
	}

	createdOffer, err := svc.CreateOffer(ctx, o)
	if err != nil {
		t.Fatalf("CreateOffer failed: %v", err)
	}
	if createdOffer.ID <= 0 || createdOffer.OrganizationID != 42 {
		t.Errorf("Offer metadata wrong: ID=%d Org=%d", createdOffer.ID, createdOffer.OrganizationID)
	}

	// 2. Get Offer & Engagement
	gotOffer, err := svc.GetOffer(ctx, createdOffer.ID)
	if err != nil || gotOffer.ID != createdOffer.ID {
		t.Fatalf("GetOffer failed: %v", err)
	}

	if err := svc.RecordOfferView(ctx, createdOffer.ID); err != nil {
		t.Fatalf("RecordOfferView failed: %v", err)
	}
	if err := svc.RecordOfferClick(ctx, createdOffer.ID); err != nil {
		t.Fatalf("RecordOfferClick failed: %v", err)
	}

	// 3. Calculate Discount
	subtotal, _ := money.Parse("200.00")
	disc, err := createdOffer.CalculateDiscount(subtotal)
	if err != nil {
		t.Fatalf("CalculateDiscount failed: %v", err)
	}
	if disc.IsZero() {
		t.Errorf("expected positive discount, got %v", disc)
	}

	// 4. Packages & Sponsorship
	pkgPrice, _ := money.Parse("50.00")
	pkg := &OfferPackage{
		Name:         i18n.New("باقة برونزية", "Bronze Tier"),
		Price:        pkgPrice,
		DurationDays: 14,
		MaxOffers:    3,
		IsActive:     true,
	}
	createdPkg, err := svc.CreatePackage(ctx, pkg)
	if err != nil {
		t.Fatalf("CreatePackage failed: %v", err)
	}

	pkgs, err := svc.ListPackages(ctx)
	if err != nil || len(pkgs) != 1 {
		t.Fatalf("ListPackages failed: %v", err)
	}

	spons, err := svc.SponsorOffer(ctx, createdOffer.ID, createdPkg.ID, 30)
	if err != nil {
		t.Fatalf("SponsorOffer failed: %v", err)
	}
	if spons.OfferID != createdOffer.ID {
		t.Errorf("got spons offer id %d, want %d", spons.OfferID, createdOffer.ID)
	}

	// 5. Active Offers and Ads
	activeOffers, err := svc.ListActiveOffers(ctx, 10, 0)
	if err != nil || len(activeOffers) != 1 {
		t.Fatalf("ListActiveOffers failed: %v", err)
	}

	repo.ads[1] = &Ad{
		ID:       1,
		Title:    "Top Banner",
		Position: "homepage_top",
		IsActive: true,
	}
	ads, err := svc.ListActiveAds(ctx, "homepage_top")
	if err != nil || len(ads) != 1 {
		t.Fatalf("ListActiveAds failed: %v", err)
	}
	if err := svc.RecordAdClick(ctx, 1, nil, "127.0.0.1", "agent"); err != nil {
		t.Fatalf("RecordAdClick failed: %v", err)
	}

	// 6. Highlight sections & Expire
	h := &HighlightSection{
		Title: i18n.New("عروض مميزة", "Featured Deals"),
		Slug:  "featured-deals",
	}
	createdSec, err := svc.CreateHighlightSection(ctx, h)
	if err != nil || createdSec.ID <= 0 {
		t.Fatalf("CreateHighlightSection failed: %v", err)
	}
	secs, err := svc.ListHighlightSections(ctx)
	if err != nil || len(secs) != 1 {
		t.Fatalf("ListHighlightSections failed: %v", err)
	}

	_, err = svc.ExpirePromotions(ctx)
	if err != nil {
		t.Fatalf("ExpirePromotions failed: %v", err)
	}
}
