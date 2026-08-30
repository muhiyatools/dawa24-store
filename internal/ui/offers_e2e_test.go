package ui_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/modules/promo"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/ui"
)

type mockPromoRepo struct {
	offers []*promo.Offer
	spec   *promo.SpecialOffer
}

func (m *mockPromoRepo) CreateOffer(ctx context.Context, o *promo.Offer) error {
	o.ID = 101
	m.offers = append(m.offers, o)
	return nil
}
func (m *mockPromoRepo) GetOfferByID(ctx context.Context, id int64) (*promo.Offer, error) {
	if len(m.offers) > 0 {
		return m.offers[0], nil
	}
	return nil, nil
}
func (m *mockPromoRepo) ListActiveOffers(ctx context.Context, limit, offset int) ([]*promo.Offer, error) {
	return m.offers, nil
}
func (m *mockPromoRepo) ListOffersForProduct(ctx context.Context, productID int64) ([]*promo.OfferProductWithOffer, error) {
	return nil, nil
}
func (m *mockPromoRepo) ListOffersForProducts(ctx context.Context, productIDs []int64) ([]*promo.OfferProductWithOffer, error) {
	return nil, nil
}
func (m *mockPromoRepo) ListOffersVisibleTo(ctx context.Context, latitude, longitude float64, dayOfWeek, limit, offset int, allowedWorks []int64) ([]*promo.VisibleOffer, error) {
	return nil, nil
}
func (m *mockPromoRepo) ListOffers(ctx context.Context, limit, offset int) ([]*promo.Offer, error) {
	return m.offers, nil
}
func (m *mockPromoRepo) SetOfferActive(ctx context.Context, id int64, active bool) error {
	return nil
}
func (m *mockPromoRepo) IncrementOfferEngagement(ctx context.Context, offerID int64, isClick bool) error {
	return nil
}
func (m *mockPromoRepo) CreatePackage(ctx context.Context, p *promo.OfferPackage) error {
	return nil
}
func (m *mockPromoRepo) ListPackages(ctx context.Context) ([]*promo.OfferPackage, error) {
	return nil, nil
}
func (m *mockPromoRepo) CreateSponsorship(ctx context.Context, s *promo.OfferSponsorship) error {
	return nil
}
func (m *mockPromoRepo) ListActiveAds(ctx context.Context, position string) ([]*promo.Ad, error) {
	return nil, nil
}
func (m *mockPromoRepo) RecordAdClick(ctx context.Context, adID int64, userID *int64, ip, ua string) error {
	return nil
}
func (m *mockPromoRepo) CreateHighlightSection(ctx context.Context, h *promo.HighlightSection) error {
	return nil
}
func (m *mockPromoRepo) UpdateHighlightSection(ctx context.Context, h *promo.HighlightSection) error {
	return nil
}
func (m *mockPromoRepo) DeleteHighlightSection(ctx context.Context, id, orgID int64) error {
	return nil
}
func (m *mockPromoRepo) GetHighlightSectionByID(ctx context.Context, id int64) (*promo.HighlightSection, error) {
	return nil, nil
}
func (m *mockPromoRepo) ListHighlightSections(ctx context.Context) ([]*promo.HighlightSection, error) {
	return nil, nil
}
func (m *mockPromoRepo) ListHighlightSectionsByOrg(ctx context.Context, orgID int64) ([]*promo.HighlightSection, error) {
	return nil, nil
}
func (m *mockPromoRepo) AddHighlightItem(ctx context.Context, item *promo.HighlightSectionItem) error {
	return nil
}
func (m *mockPromoRepo) ListHighlightItems(ctx context.Context, sectionID int64) ([]*promo.HighlightSectionItem, error) {
	return nil, nil
}
func (m *mockPromoRepo) ExpirePromotions(ctx context.Context) (int64, error) {
	return 0, nil
}
func (m *mockPromoRepo) CreateSpecialOffer(ctx context.Context, o *promo.SpecialOffer) error {
	o.ID = 101
	m.spec = o
	return nil
}
func (m *mockPromoRepo) GetSpecialOfferByID(ctx context.Context, id int64) (*promo.SpecialOffer, error) {
	return m.spec, nil
}
func (m *mockPromoRepo) ListSpecialOffersByOrg(ctx context.Context, orgID int64) ([]*promo.SpecialOffer, error) {
	if m.spec != nil {
		return []*promo.SpecialOffer{m.spec}, nil
	}
	return nil, nil
}
func (m *mockPromoRepo) DeleteSpecialOffer(ctx context.Context, id, orgID int64) error {
	return nil
}
func (m *mockPromoRepo) AddSpecialOfferLocation(ctx context.Context, loc *promo.SpecialOfferLocation) error {
	return nil
}
func (m *mockPromoRepo) ListSpecialOfferLocations(ctx context.Context, offerID int64) ([]*promo.SpecialOfferLocation, error) {
	if m.spec != nil {
		return m.spec.Locations, nil
	}
	return nil, nil
}
func (m *mockPromoRepo) ListAllSpecialOffers(ctx context.Context, limit, offset int) ([]*promo.SpecialOffer, error) {
	if m.spec != nil {
		return []*promo.SpecialOffer{m.spec}, nil
	}
	return nil, nil
}
func (m *mockPromoRepo) UpdateSpecialOfferAdminStatus(ctx context.Context, id int64, adminStatus, notes string, approvedBy int64) error {
	return nil
}
func (m *mockPromoRepo) ToggleSpecialOfferStatus(ctx context.Context, id int64, isActive bool) error {
	return nil
}

// --- New sponsorship and ads methods (stubs) ---
func (m *mockPromoRepo) UpdatePackage(ctx context.Context, p *promo.OfferPackage) error { return nil }
func (m *mockPromoRepo) GetPackageByID(ctx context.Context, id int64) (*promo.OfferPackage, error) {
	return nil, nil
}
func (m *mockPromoRepo) AdminListPackages(ctx context.Context) ([]*promo.OfferPackage, error) {
	return nil, nil
}
func (m *mockPromoRepo) TogglePackageActive(ctx context.Context, id int64, active bool) error {
	return nil
}

func (m *mockPromoRepo) CreateSponsorshipPurchase(ctx context.Context, p *promo.SponsorshipPurchase) error {
	return nil
}
func (m *mockPromoRepo) GetSponsorshipPurchaseByID(ctx context.Context, id int64) (*promo.SponsorshipPurchase, error) {
	return nil, nil
}
func (m *mockPromoRepo) ListSponsorshipPurchasesByOrg(ctx context.Context, orgID int64) ([]*promo.SponsorshipPurchase, error) {
	return nil, nil
}
func (m *mockPromoRepo) ListActiveSponsorshipPurchasesByOrg(ctx context.Context, orgID int64) ([]*promo.SponsorshipPurchase, error) {
	return nil, nil
}
func (m *mockPromoRepo) IncrementSponsorshipPurchaseCreditsUsed(ctx context.Context, purchaseID int64, credits int) error {
	return nil
}
func (m *mockPromoRepo) ExpireSponsorshipPurchases(ctx context.Context) (int64, error) { return 0, nil }

func (m *mockPromoRepo) CreateSponsorshipRequest(ctx context.Context, r *promo.SponsorshipRequest) error {
	return nil
}
func (m *mockPromoRepo) GetSponsorshipRequestByID(ctx context.Context, id int64) (*promo.SponsorshipRequest, error) {
	return nil, nil
}
func (m *mockPromoRepo) ListSponsorshipRequestsByOrg(ctx context.Context, orgID int64, limit, offset int) ([]*promo.SponsorshipRequest, error) {
	return nil, nil
}
func (m *mockPromoRepo) ListAllSponsorshipRequests(ctx context.Context, limit, offset int) ([]*promo.SponsorshipRequest, error) {
	return nil, nil
}
func (m *mockPromoRepo) ListPendingSponsorshipRequests(ctx context.Context, limit, offset int) ([]*promo.SponsorshipRequest, error) {
	return nil, nil
}
func (m *mockPromoRepo) UpdateSponsorshipRequestAdminStatus(ctx context.Context, id int64, status promo.AdminStatus, notes string, reviewerID int64) error {
	return nil
}
func (m *mockPromoRepo) ActivateSponsorshipRequest(ctx context.Context, id int64, reviewerID int64) (*promo.SponsorshipRequest, error) {
	return nil, nil
}
func (m *mockPromoRepo) CancelSponsorshipRequest(ctx context.Context, id, orgID int64) error {
	return nil
}
func (m *mockPromoRepo) ExpireSponsorshipRequests(ctx context.Context) (int64, error) { return 0, nil }

func (m *mockPromoRepo) RankedSponsorshipsForProducts(ctx context.Context, productIDs []int64) ([]*promo.RankedSponsorship, error) {
	return nil, nil
}
func (m *mockPromoRepo) RankedSponsorshipsForOffers(ctx context.Context, offerIDs []int64) ([]*promo.RankedSponsorship, error) {
	return nil, nil
}
func (m *mockPromoRepo) IsSponsored(ctx context.Context, itemType promo.SponsorshipItemType, itemID int64) (*promo.RankedSponsorship, error) {
	return nil, nil
}

func (m *mockPromoRepo) CreateAd(ctx context.Context, a *promo.Ad) error            { return nil }
func (m *mockPromoRepo) UpdateAd(ctx context.Context, a *promo.Ad) error            { return nil }
func (m *mockPromoRepo) GetAdByID(ctx context.Context, id int64) (*promo.Ad, error) { return nil, nil }
func (m *mockPromoRepo) ListAdsByOrg(ctx context.Context, orgID int64, limit, offset int) ([]*promo.Ad, error) {
	return nil, nil
}
func (m *mockPromoRepo) ListAllAds(ctx context.Context, limit, offset int) ([]*promo.Ad, error) {
	return nil, nil
}
func (m *mockPromoRepo) UpdateAdAdminStatus(ctx context.Context, id int64, status promo.AdminStatus, notes string, reviewerID int64) error {
	return nil
}
func (m *mockPromoRepo) RecordAdImpression(ctx context.Context, adID int64, userID *int64, ip, ua string) error {
	return nil
}

func TestOffersWorkflowAndRendering(t *testing.T) {
	now := time.Now().UTC()
	expiry := now.Add(30 * 24 * time.Hour)

	mockRepo := &mockPromoRepo{
		offers: []*promo.Offer{
			{
				ID:             101,
				OrganizationID: 50,
				Title:          i18n.New("عرض الصيف الحصري للأدوية", "Summer Medicine Offer"),
				Description:    i18n.New("خصم خاص 20% لكافة الصيدليات", "Special 20% discount"),
				DiscountType:   promo.DiscountPercentage,
				DiscountValue:  money.FromMinor(2000), // 20%
				StartsAt:       now,
				ExpiresAt:      expiry,
				IsActive:       true,
				AdminStatus:    "approved",
			},
		},
		spec: &promo.SpecialOffer{
			ID:                 101,
			OrganizationID:     50,
			OrganizationName:   "شركة الأمل للمستلزمات والأدوية",
			Title:              i18n.New("عرض الصيف الحصري للأدوية", "Summer Medicine Offer"),
			Description:        i18n.New("خصم خاص 20% لكافة الصيدليات", "Special 20% discount"),
			DiscountPercentage: 20.0,
			TotalPrice:         money.FromMinor(45000),
			StartDate:          &now,
			EndDate:            &expiry,
			Status:             "active",
			AdminStatus:        "approved",
			Products: []*promo.SpecialOfferProduct{
				{
					ID:                 1,
					OfferID:            101,
					VariantID:          26643,
					VariantName:        "يوريكودروب 80مجم 30 قرص",
					OriginalPrice:      money.FromMinor(3500),
					CustomPrice:        money.FromMinor(2800),
					DiscountPercentage: 20.0,
					Quantity:           10,
				},
			},
			Locations: []*promo.SpecialOfferLocation{
				{
					ID:        1,
					OfferID:   101,
					CityName:  "القاهرة",
					AddressAr: "مدينة نصر والتجمع الخامس",
				},
			},
		},
	}

	promoSvc := promo.NewService(mockRepo, slog.Default())
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := ui.NewUIHandler(nil, nil, nil, nil, nil, nil, nil, promoSvc, nil, nil, nil, nil, nil, nil, logger)

	r := chi.NewRouter()
	handler.RegisterPublicRoutes(r)
	r.Post("/cart/add-offer", handler.AddOfferToCartSubmit)

	// 1. Test GET /offers (Public Offers Listing)
	req := httptest.NewRequest(http.MethodGet, "/offers", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 for /offers, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !containsStr(body, "عرض الصيف الحصري للأدوية") {
		t.Fatalf("expected /offers to contain offer title")
	}
	if !containsStr(body, "شركة الأمل للمستلزمات والأدوية") {
		t.Fatalf("expected /offers to contain supplier name")
	}

	// 2. Test GET /offers/101 (Offer Detail Page)
	reqDetail := httptest.NewRequest(http.MethodGet, "/offers/101", nil)
	recDetail := httptest.NewRecorder()
	r.ServeHTTP(recDetail, reqDetail)

	if recDetail.Code != http.StatusOK {
		t.Fatalf("expected status 200 for /offers/101, got %d", recDetail.Code)
	}
	bodyDetail := recDetail.Body.String()
	if !containsStr(bodyDetail, "يوريكودروب 80مجم 30 قرص") {
		t.Fatalf("expected /offers/101 to contain medicine item name")
	}
	if !containsStr(bodyDetail, "28.00") {
		t.Fatalf("expected /offers/101 to display discounted custom price")
	}

	// 3. Test GET /offers/101 as Logged-in Customer Pharmacy
	reqAuth := httptest.NewRequest(http.MethodGet, "/offers/101", nil)
	reqAuth = reqAuth.WithContext(authctx.WithActor(reqAuth.Context(), authctx.Actor{
		UserID:         5,
		OrganizationID: 12,
		OrgType:        string(org.TypeCustomer),
		Role:           "customer",
	}))
	recAuth := httptest.NewRecorder()
	r.ServeHTTP(recAuth, reqAuth)

	if recAuth.Code != http.StatusOK {
		t.Fatalf("expected status 200 for /offers/101 as customer, got %d", recAuth.Code)
	}
	bodyAuth := recAuth.Body.String()
	if !containsStr(bodyAuth, "action=\"/cart/add-offer\"") {
		t.Fatalf("expected /offers/101 for customer to render /cart/add-offer form")
	}

	// 4. Test POST /cart/add-offer
	formOffer := url.Values{}
	formOffer.Set("offer_id", "101")
	formOffer.Set("quantity", "1")
	reqPostOffer := httptest.NewRequest(http.MethodPost, "/cart/add-offer", strings.NewReader(formOffer.Encode()))
	reqPostOffer.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqPostOffer = reqPostOffer.WithContext(authctx.WithActor(reqPostOffer.Context(), authctx.Actor{
		UserID:         5,
		OrganizationID: 12,
		OrgType:        string(org.TypeCustomer),
		Role:           "customer",
		Permissions:    []string{"pharmacy.cart.use"},
	}))
	recPostOffer := httptest.NewRecorder()
	r.ServeHTTP(recPostOffer, reqPostOffer)
	if recPostOffer.Code != http.StatusSeeOther && recPostOffer.Code != http.StatusOK {
		t.Fatalf("expected status 303 or 200 for POST /cart/add-offer, got %d", recPostOffer.Code)
	}
}

func containsStr(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && (s == substr || len(s) >= len(substr) && stringContains(s, substr))
}

func stringContains(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
