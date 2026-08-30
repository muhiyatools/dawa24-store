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
	offers                        map[int64]*Offer
	packages                      map[int64]*OfferPackage
	sponsorships                  map[int64]*OfferSponsorship
	ads                           map[int64]*Ad
	sections                      map[int64]*HighlightSection
	nextID                        int64
	CreateSponsorshipPurchaseFunc func(context.Context, *SponsorshipPurchase) error
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

func (m *mockPromoRepo) ListOffersForProduct(_ context.Context, productID int64) ([]*OfferProductWithOffer, error) {
	var list []*OfferProductWithOffer
	for _, o := range m.offers {
		if !o.IsActive {
			continue
		}
		for _, pid := range o.ProductIDs {
			if pid == productID {
				list = append(list, &OfferProductWithOffer{
					Offer: o,
					Product: &OfferProduct{
						ProductID: pid,
						CustomQty: 1,
					},
				})
			}
		}
	}
	return list, nil
}

func (m *mockPromoRepo) ListOffersForProducts(_ context.Context, productIDs []int64) ([]*OfferProductWithOffer, error) {
	wanted := make(map[int64]struct{}, len(productIDs))
	for _, id := range productIDs {
		wanted[id] = struct{}{}
	}
	var list []*OfferProductWithOffer
	for _, o := range m.offers {
		if !o.IsActive {
			continue
		}
		for _, pid := range o.ProductIDs {
			if _, ok := wanted[pid]; ok {
				list = append(list, &OfferProductWithOffer{
					Offer:   o,
					Product: &OfferProduct{ProductID: pid, CustomQty: 1},
				})
			}
		}
	}
	return list, nil
}

func (m *mockPromoRepo) ListOffersVisibleTo(_ context.Context, latitude, longitude float64, dayOfWeek, limit, offset int, allowedWorkIDs []int64) ([]*VisibleOffer, error) {
	var list []*VisibleOffer
	for _, o := range m.offers {
		if o.IsActive {
			list = append(list, &VisibleOffer{Offer: o, VendorBranchID: o.OrganizationID})
		}
	}
	return list, nil
}

func (m *mockPromoRepo) ListOffers(_ context.Context, _, _ int) ([]*Offer, error) {
	var list []*Offer
	for _, o := range m.offers {
		list = append(list, o)
	}
	return list, nil
}

func (m *mockPromoRepo) SetOfferActive(_ context.Context, _ int64, _ bool) error {
	return nil
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
func (m *mockPromoRepo) UpdatePackage(_ context.Context, _ *OfferPackage) error { return nil }
func (m *mockPromoRepo) GetPackageByID(_ context.Context, id int64) (*OfferPackage, error) {
	if p, ok := m.packages[id]; ok {
		return p, nil
	}
	return nil, apperr.NotFound("package")
}
func (m *mockPromoRepo) AdminListPackages(_ context.Context) ([]*OfferPackage, error) {
	return nil, nil
}
func (m *mockPromoRepo) TogglePackageActive(_ context.Context, _ int64, _ bool) error {
	return nil
}
func (m *mockPromoRepo) CreateSponsorshipPurchase(ctx context.Context, p *SponsorshipPurchase) error {
	if m.CreateSponsorshipPurchaseFunc != nil {
		return m.CreateSponsorshipPurchaseFunc(ctx, p)
	}
	return nil
}
func (m *mockPromoRepo) GetSponsorshipPurchaseByID(_ context.Context, _ int64) (*SponsorshipPurchase, error) {
	return nil, nil
}
func (m *mockPromoRepo) ListSponsorshipPurchasesByOrg(_ context.Context, _ int64) ([]*SponsorshipPurchase, error) {
	return nil, nil
}
func (m *mockPromoRepo) ListActiveSponsorshipPurchasesByOrg(_ context.Context, _ int64) ([]*SponsorshipPurchase, error) {
	return nil, nil
}
func (m *mockPromoRepo) IncrementSponsorshipPurchaseCreditsUsed(_ context.Context, _ int64, _ int) error {
	return nil
}
func (m *mockPromoRepo) ExpireSponsorshipPurchases(_ context.Context) (int64, error) {
	return 0, nil
}
func (m *mockPromoRepo) CreateSponsorshipRequest(_ context.Context, _ *SponsorshipRequest) error {
	return nil
}
func (m *mockPromoRepo) GetSponsorshipRequestByID(_ context.Context, _ int64) (*SponsorshipRequest, error) {
	return nil, nil
}
func (m *mockPromoRepo) ListSponsorshipRequestsByOrg(_ context.Context, _ int64, _, _ int) ([]*SponsorshipRequest, error) {
	return nil, nil
}
func (m *mockPromoRepo) ListAllSponsorshipRequests(_ context.Context, _, _ int) ([]*SponsorshipRequest, error) {
	return nil, nil
}
func (m *mockPromoRepo) ListPendingSponsorshipRequests(_ context.Context, _, _ int) ([]*SponsorshipRequest, error) {
	return nil, nil
}
func (m *mockPromoRepo) UpdateSponsorshipRequestAdminStatus(_ context.Context, _ int64, _ AdminStatus, _ string, _ int64) error {
	return nil
}
func (m *mockPromoRepo) ActivateSponsorshipRequest(_ context.Context, _ int64, _ int64) (*SponsorshipRequest, error) {
	return nil, nil
}
func (m *mockPromoRepo) CancelSponsorshipRequest(_ context.Context, _, _ int64) error { return nil }
func (m *mockPromoRepo) ExpireSponsorshipRequests(_ context.Context) (int64, error) {
	return 0, nil
}
func (m *mockPromoRepo) RankedSponsorshipsForProducts(_ context.Context, _ []int64) ([]*RankedSponsorship, error) {
	return nil, nil
}
func (m *mockPromoRepo) RankedSponsorshipsForOffers(_ context.Context, _ []int64) ([]*RankedSponsorship, error) {
	return nil, nil
}
func (m *mockPromoRepo) IsSponsored(_ context.Context, _ SponsorshipItemType, _ int64) (*RankedSponsorship, error) {
	return nil, nil
}
func (m *mockPromoRepo) CreateAd(_ context.Context, _ *Ad) error { return nil }
func (m *mockPromoRepo) UpdateAd(_ context.Context, _ *Ad) error { return nil }
func (m *mockPromoRepo) GetAdByID(_ context.Context, _ int64) (*Ad, error) {
	return nil, nil
}
func (m *mockPromoRepo) ListAdsByOrg(_ context.Context, _ int64, _, _ int) ([]*Ad, error) {
	return nil, nil
}
func (m *mockPromoRepo) ListAllAds(_ context.Context, _, _ int) ([]*Ad, error) {
	return nil, nil
}
func (m *mockPromoRepo) UpdateAdAdminStatus(_ context.Context, _ int64, _ AdminStatus, _ string, _ int64) error {
	return nil
}
func (m *mockPromoRepo) RecordAdImpression(_ context.Context, _ int64, _ *int64, _, _ string) error {
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

func (m *mockPromoRepo) ListHighlightSectionsByOrg(_ context.Context, orgID int64) ([]*HighlightSection, error) {
	var list []*HighlightSection
	for _, s := range m.sections {
		if s.OwnerType == "organization" && s.OrganizationID != nil && *s.OrganizationID == orgID {
			list = append(list, s)
		}
	}
	return list, nil
}

func (m *mockPromoRepo) UpdateHighlightSection(_ context.Context, s *HighlightSection) error {
	m.sections[s.ID] = s
	return nil
}

func (m *mockPromoRepo) DeleteHighlightSection(_ context.Context, id, orgID int64) error {
	delete(m.sections, id)
	return nil
}

func (m *mockPromoRepo) GetHighlightSectionByID(_ context.Context, id int64) (*HighlightSection, error) {
	return m.sections[id], nil
}

func (m *mockPromoRepo) AddHighlightItem(_ context.Context, item *HighlightSectionItem) error {
	item.ID = m.nextID
	m.nextID++
	return nil
}

func (m *mockPromoRepo) ListHighlightItems(_ context.Context, _ int64) ([]*HighlightSectionItem, error) {
	return nil, nil
}

func (m *mockPromoRepo) ExpirePromotions(_ context.Context) (int64, error) {
	return 0, nil
}

func (m *mockPromoRepo) CreateSpecialOffer(_ context.Context, o *SpecialOffer) error {
	o.ID = 1
	return nil
}
func (m *mockPromoRepo) GetSpecialOfferByID(_ context.Context, id int64) (*SpecialOffer, error) {
	return &SpecialOffer{ID: id, Title: i18n.New("عرض تجريبي", "Demo Special Offer")}, nil
}
func (m *mockPromoRepo) ListSpecialOffersByOrg(_ context.Context, _ int64) ([]*SpecialOffer, error) {
	return []*SpecialOffer{{ID: 1, Title: i18n.New("عرض تجريبي", "Demo Special Offer")}}, nil
}
func (m *mockPromoRepo) ListAllSpecialOffers(_ context.Context, _, _ int) ([]*SpecialOffer, error) {
	return []*SpecialOffer{{ID: 1, Title: i18n.New("عرض تجريبي", "Demo Special Offer")}}, nil
}
func (m *mockPromoRepo) UpdateSpecialOfferAdminStatus(_ context.Context, _ int64, _, _ string, _ int64) error {
	return nil
}
func (m *mockPromoRepo) ToggleSpecialOfferStatus(_ context.Context, _ int64, _ bool) error {
	return nil
}
func (m *mockPromoRepo) DeleteSpecialOffer(_ context.Context, _, _ int64) error {
	return nil
}
func (m *mockPromoRepo) AddSpecialOfferLocation(_ context.Context, loc *SpecialOfferLocation) error {
	loc.ID = 1
	return nil
}
func (m *mockPromoRepo) ListSpecialOfferLocations(_ context.Context, _ int64) ([]*SpecialOfferLocation, error) {
	return []*SpecialOfferLocation{{ID: 1, Radius: 1000}}, nil
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
		Title:          i18n.New("خصم الصيف", "Summer Discount"),
		DiscountType:   DiscountPercentage,
		DiscountValue:  discVal,
		MinOrderAmount: minOrder,
		StartsAt:       now,
		ExpiresAt:      now.Add(7 * 24 * time.Hour),
		IsActive:       true,
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

	// 6b. Organization-owned highlight sections (066)
	orgSec, err := svc.CreateOrganizationHighlightSection(ctx, 7, i18n.New("الأكثر مبيعاً", "Best sellers"), "best")
	if err != nil || orgSec.ID <= 0 {
		t.Fatalf("CreateOrganizationHighlightSection failed: %v", err)
	}
	if orgSec.OwnerType != "organization" || orgSec.OrganizationID == nil || *orgSec.OrganizationID != 7 {
		t.Errorf("org section owner mismatch: %+v", orgSec)
	}
	if _, err := svc.CreateOrganizationHighlightSection(ctx, 7, i18n.Text{}, ""); err == nil {
		t.Error("empty-title org section should be rejected")
	}
	orgSecs, err := svc.ListHighlightSectionsByOrg(ctx, 7)
	if err != nil || len(orgSecs) != 1 {
		t.Fatalf("ListHighlightSectionsByOrg failed: %v", err)
	}
	pid := int64(42)
	if err := svc.AddHighlightItem(ctx, orgSec.ID, &pid, nil); err != nil {
		t.Fatalf("AddHighlightItem failed: %v", err)
	}
	items, err := svc.ListHighlightItems(ctx, orgSec.ID)
	if err != nil || len(items) != 0 {
		t.Fatalf("ListHighlightItems failed: %v", err)
	}

	_, err = svc.ExpirePromotions(ctx)
	if err != nil {
		t.Fatalf("ExpirePromotions failed: %v", err)
	}
}

func TestPurchaseSponsorshipPackage_WalletDebit(t *testing.T) {
	repo := newMockPromoRepo()
	pkg := &OfferPackage{
		ID:           1,
		Name:         i18n.New("الباقة الذهبية", "Gold Package"),
		Price:        money.FromMinor(150000),
		Credits:      10,
		DurationDays: 30,
		IsActive:     true,
	}
	repo.packages[1] = pkg

	var savedPurchase *SponsorshipPurchase
	repo.CreateSponsorshipPurchaseFunc = func(_ context.Context, p *SponsorshipPurchase) error {
		savedPurchase = p
		p.ID = 100
		return nil
	}

	svc := NewService(repo, slog.New(slog.NewTextHandler(io.Discard, nil)))

	var debitedOrgID int64
	var debitedAmount money.Amount
	fakeTxID := int64(777)
	svc.SetWalletDebiter(func(ctx context.Context, orgID int64, amount money.Amount, description string) (*int64, error) {
		debitedOrgID = orgID
		debitedAmount = amount
		return &fakeTxID, nil
	})

	ctx := database.WithTenant(context.Background(), 55)
	purchase, err := svc.PurchaseSponsorshipPackage(ctx, 1, true, "monthly")
	if err != nil {
		t.Fatalf("PurchaseSponsorshipPackage failed: %v", err)
	}

	if debitedOrgID != 55 {
		t.Errorf("debitedOrgID = %d, want 55", debitedOrgID)
	}
	if debitedAmount.Minor() != 150000 {
		t.Errorf("debitedAmount = %d, want 150000", debitedAmount.Minor())
	}
	if savedPurchase == nil {
		t.Fatal("expected purchase to be saved")
	}
	if savedPurchase.PaymentID != nil {
		t.Errorf("savedPurchase.PaymentID = %v, want nil (must not violate foreign key)", *savedPurchase.PaymentID)
	}
	if savedPurchase.SourceID == nil || *savedPurchase.SourceID != 777 {
		t.Errorf("savedPurchase.SourceID = %v, want 777", savedPurchase.SourceID)
	}
	if savedPurchase.SourceSystem != "wallet_checkout" {
		t.Errorf("savedPurchase.SourceSystem = %q, want 'wallet_checkout'", savedPurchase.SourceSystem)
	}
	if purchase.CreditsRemaining != 10 {
		t.Errorf("purchase.CreditsRemaining = %d, want 10", purchase.CreditsRemaining)
	}
}
