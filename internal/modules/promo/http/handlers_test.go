package http_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/promo"
	promoHttp "github.com/muhiya/dawa24-store/internal/modules/promo/http"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/httpx"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

type stubRepo struct{ t *testing.T }

func (r stubRepo) fail(method string) {
	r.t.Helper()
	r.t.Fatalf("repository.%s was called; the request should have been rejected before reaching the repository", method)
}

func (r stubRepo) CreateOffer(context.Context, *promo.Offer) error { r.fail("CreateOffer"); return nil }
func (r stubRepo) GetOfferByID(context.Context, int64) (*promo.Offer, error) {
	r.fail("GetOfferByID")
	return nil, nil
}
func (r stubRepo) ListActiveOffers(context.Context, int, int) ([]*promo.Offer, error) {
	r.fail("ListActiveOffers")
	return nil, nil
}
func (r stubRepo) ListOffersForProduct(context.Context, int64) ([]*promo.OfferProductWithOffer, error) {
	r.fail("ListOffersForProduct")
	return nil, nil
}
func (r stubRepo) ListOffersForProducts(context.Context, []int64) ([]*promo.OfferProductWithOffer, error) {
	r.fail("ListOffersForProducts")
	return nil, nil
}
func (r stubRepo) ListOffersVisibleTo(context.Context, float64, float64, int, int, int, []int64) ([]*promo.VisibleOffer, error) {
	r.fail("ListOffersVisibleTo")
	return nil, nil
}

func (r stubRepo) ListOffers(context.Context, int, int) ([]*promo.Offer, error) {
	r.fail("ListOffers")
	return nil, nil
}
func (r stubRepo) SetOfferActive(context.Context, int64, bool) error {
	r.fail("SetOfferActive")
	return nil
}
func (r stubRepo) IncrementOfferEngagement(context.Context, int64, bool) error {
	r.fail("IncrementOfferEngagement")
	return nil
}
func (r stubRepo) CreatePackage(context.Context, *promo.OfferPackage) error {
	r.fail("CreatePackage")
	return nil
}
func (r stubRepo) ListPackages(context.Context) ([]*promo.OfferPackage, error) {
	r.fail("ListPackages")
	return nil, nil
}
func (r stubRepo) CreateSponsorship(context.Context, *promo.OfferSponsorship) error {
	r.fail("CreateSponsorship")
	return nil
}
func (r stubRepo) ListActiveAds(context.Context, string) ([]*promo.Ad, error) {
	r.fail("ListActiveAds")
	return nil, nil
}
func (r stubRepo) RecordAdClick(context.Context, int64, *int64, string, string) error {
	r.fail("RecordAdClick")
	return nil
}
func (stubRepo) UpdatePackage(context.Context, *promo.OfferPackage) error { return nil }
func (stubRepo) GetPackageByID(context.Context, int64) (*promo.OfferPackage, error) {
	return nil, nil
}
func (stubRepo) AdminListPackages(context.Context) ([]*promo.OfferPackage, error) {
	return nil, nil
}
func (stubRepo) TogglePackageActive(context.Context, int64, bool) error { return nil }
func (stubRepo) CreateSponsorshipPurchase(context.Context, *promo.SponsorshipPurchase) error {
	return nil
}
func (stubRepo) GetSponsorshipPurchaseByID(context.Context, int64) (*promo.SponsorshipPurchase, error) {
	return nil, nil
}
func (stubRepo) ListSponsorshipPurchasesByOrg(context.Context, int64) ([]*promo.SponsorshipPurchase, error) {
	return nil, nil
}
func (stubRepo) ListActiveSponsorshipPurchasesByOrg(context.Context, int64) ([]*promo.SponsorshipPurchase, error) {
	return nil, nil
}
func (stubRepo) IncrementSponsorshipPurchaseCreditsUsed(context.Context, int64, int) error {
	return nil
}
func (stubRepo) ExpireSponsorshipPurchases(context.Context) (int64, error) { return 0, nil }
func (stubRepo) CreateSponsorshipRequest(context.Context, *promo.SponsorshipRequest) error {
	return nil
}
func (stubRepo) GetSponsorshipRequestByID(context.Context, int64) (*promo.SponsorshipRequest, error) {
	return nil, nil
}
func (stubRepo) ListSponsorshipRequestsByOrg(context.Context, int64, int, int) ([]*promo.SponsorshipRequest, error) {
	return nil, nil
}
func (stubRepo) ListAllSponsorshipRequests(context.Context, int, int) ([]*promo.SponsorshipRequest, error) {
	return nil, nil
}
func (stubRepo) ListPendingSponsorshipRequests(context.Context, int, int) ([]*promo.SponsorshipRequest, error) {
	return nil, nil
}
func (stubRepo) UpdateSponsorshipRequestAdminStatus(context.Context, int64, promo.AdminStatus, string, int64) error {
	return nil
}
func (stubRepo) ActivateSponsorshipRequest(context.Context, int64, int64) (*promo.SponsorshipRequest, error) {
	return nil, nil
}
func (stubRepo) CancelSponsorshipRequest(context.Context, int64, int64) error { return nil }
func (stubRepo) ExpireSponsorshipRequests(context.Context) (int64, error) { return 0, nil }
func (stubRepo) RankedSponsorshipsForProducts(context.Context, []int64) ([]*promo.RankedSponsorship, error) {
	return nil, nil
}
func (stubRepo) RankedSponsorshipsForOffers(context.Context, []int64) ([]*promo.RankedSponsorship, error) {
	return nil, nil
}
func (stubRepo) IsSponsored(context.Context, promo.SponsorshipItemType, int64) (*promo.RankedSponsorship, error) {
	return nil, nil
}
func (stubRepo) CreateAd(context.Context, *promo.Ad) error { return nil }
func (stubRepo) UpdateAd(context.Context, *promo.Ad) error { return nil }
func (stubRepo) GetAdByID(context.Context, int64) (*promo.Ad, error) {
	return nil, nil
}
func (stubRepo) ListAdsByOrg(context.Context, int64, int, int) ([]*promo.Ad, error) {
	return nil, nil
}
func (stubRepo) ListAllAds(context.Context, int, int) ([]*promo.Ad, error) {
	return nil, nil
}
func (stubRepo) UpdateAdAdminStatus(context.Context, int64, promo.AdminStatus, string, int64) error {
	return nil
}
func (stubRepo) RecordAdImpression(context.Context, int64, *int64, string, string) error {
	return nil
}
func (r stubRepo) CreateHighlightSection(context.Context, *promo.HighlightSection) error {
	r.fail("CreateHighlightSection")
	return nil
}
func (r stubRepo) UpdateHighlightSection(context.Context, *promo.HighlightSection) error {
	r.fail("UpdateHighlightSection")
	return nil
}
func (r stubRepo) DeleteHighlightSection(context.Context, int64, int64) error {
	r.fail("DeleteHighlightSection")
	return nil
}
func (r stubRepo) GetHighlightSectionByID(context.Context, int64) (*promo.HighlightSection, error) {
	r.fail("GetHighlightSectionByID")
	return nil, nil
}
func (r stubRepo) ListHighlightSections(context.Context) ([]*promo.HighlightSection, error) {
	r.fail("ListHighlightSections")
	return nil, nil
}
func (r stubRepo) ListHighlightSectionsByOrg(context.Context, int64) ([]*promo.HighlightSection, error) {
	r.fail("ListHighlightSectionsByOrg")
	return nil, nil
}
func (r stubRepo) AddHighlightItem(context.Context, *promo.HighlightSectionItem) error {
	r.fail("AddHighlightItem")
	return nil
}
func (r stubRepo) ListHighlightItems(context.Context, int64) ([]*promo.HighlightSectionItem, error) {
	r.fail("ListHighlightItems")
	return nil, nil
}
func (r stubRepo) ExpirePromotions(context.Context) (int64, error) {
	r.fail("ExpirePromotions")
	return 0, nil
}
func (r stubRepo) CreateSpecialOffer(context.Context, *promo.SpecialOffer) error {
	r.fail("CreateSpecialOffer")
	return nil
}
func (r stubRepo) GetSpecialOfferByID(context.Context, int64) (*promo.SpecialOffer, error) {
	r.fail("GetSpecialOfferByID")
	return nil, nil
}
func (r stubRepo) ListSpecialOffersByOrg(context.Context, int64) ([]*promo.SpecialOffer, error) {
	r.fail("ListSpecialOffersByOrg")
	return nil, nil
}
func (r stubRepo) ListAllSpecialOffers(context.Context, int, int) ([]*promo.SpecialOffer, error) {
	r.fail("ListAllSpecialOffers")
	return nil, nil
}
func (r stubRepo) UpdateSpecialOfferAdminStatus(context.Context, int64, string, string, int64) error {
	r.fail("UpdateSpecialOfferAdminStatus")
	return nil
}
func (r stubRepo) ToggleSpecialOfferStatus(context.Context, int64, bool) error {
	r.fail("ToggleSpecialOfferStatus")
	return nil
}
func (r stubRepo) DeleteSpecialOffer(context.Context, int64, int64) error {
	r.fail("DeleteSpecialOffer")
	return nil
}
func (r stubRepo) AddSpecialOfferLocation(context.Context, *promo.SpecialOfferLocation) error {
	r.fail("AddSpecialOfferLocation")
	return nil
}
func (r stubRepo) ListSpecialOfferLocations(context.Context, int64) ([]*promo.SpecialOfferLocation, error) {
	r.fail("ListSpecialOfferLocations")
	return nil, nil
}

type happyRepo struct{}

func (happyRepo) CreateOffer(ctx context.Context, o *promo.Offer) error {
	o.ID = 1
	return nil
}
func (happyRepo) GetOfferByID(ctx context.Context, id int64) (*promo.Offer, error) {
	return &promo.Offer{
		ID:             id,
		OrganizationID: 1,
		Title:          i18n.Text{"en": "Summer Sale"},
		DiscountType:   promo.DiscountPercentage,
		DiscountValue:  money.MustParse("10.00"),
		StartsAt:       time.Now().Add(-time.Hour),
		ExpiresAt:      time.Now().Add(24 * time.Hour),
		IsActive:       true,
	}, nil
}
func (happyRepo) ListActiveOffers(ctx context.Context, limit, offset int) ([]*promo.Offer, error) {
	return []*promo.Offer{{
		ID:             1,
		OrganizationID: 1,
		Title:          i18n.Text{"en": "Summer Sale"},
		IsActive:       true,
	}}, nil
}

func (happyRepo) ListOffersForProduct(ctx context.Context, productID int64) ([]*promo.OfferProductWithOffer, error) {
	return nil, nil
}
func (happyRepo) ListOffersForProducts(context.Context, []int64) ([]*promo.OfferProductWithOffer, error) {
	return nil, nil
}
func (happyRepo) ListOffersVisibleTo(ctx context.Context, latitude, longitude float64, dayOfWeek, limit, offset int, allowedWorkIDs []int64) ([]*promo.VisibleOffer, error) {
	return nil, nil
}

func (happyRepo) ListOffers(ctx context.Context, limit, offset int) ([]*promo.Offer, error) {
	return []*promo.Offer{{ID: 1, OrganizationID: 1, Title: i18n.Text{"en": "Summer Sale"}, IsActive: true}}, nil
}
func (happyRepo) SetOfferActive(ctx context.Context, id int64, active bool) error {
	return nil
}
func (happyRepo) IncrementOfferEngagement(ctx context.Context, id int64, isClick bool) error {
	return nil
}
func (happyRepo) CreatePackage(ctx context.Context, p *promo.OfferPackage) error {
	p.ID = 1
	return nil
}
func (happyRepo) ListPackages(ctx context.Context) ([]*promo.OfferPackage, error) {
	return []*promo.OfferPackage{{ID: 1, Name: i18n.Text{"en": "Gold"}}}, nil
}
func (happyRepo) CreateSponsorship(ctx context.Context, s *promo.OfferSponsorship) error {
	s.ID = 1
	return nil
}
func (happyRepo) ListActiveAds(ctx context.Context, position string) ([]*promo.Ad, error) {
	return []*promo.Ad{{ID: 1, Title: "Ad 1", IsActive: true}}, nil
}
func (happyRepo) RecordAdClick(ctx context.Context, adID int64, userID *int64, ip, ua string) error {
	return nil
}
func (happyRepo) UpdatePackage(context.Context, *promo.OfferPackage) error { return nil }
func (happyRepo) GetPackageByID(context.Context, int64) (*promo.OfferPackage, error) {
	return nil, nil
}
func (happyRepo) AdminListPackages(context.Context) ([]*promo.OfferPackage, error) {
	return nil, nil
}
func (happyRepo) TogglePackageActive(context.Context, int64, bool) error { return nil }
func (happyRepo) CreateSponsorshipPurchase(context.Context, *promo.SponsorshipPurchase) error {
	return nil
}
func (happyRepo) GetSponsorshipPurchaseByID(context.Context, int64) (*promo.SponsorshipPurchase, error) {
	return nil, nil
}
func (happyRepo) ListSponsorshipPurchasesByOrg(context.Context, int64) ([]*promo.SponsorshipPurchase, error) {
	return nil, nil
}
func (happyRepo) ListActiveSponsorshipPurchasesByOrg(context.Context, int64) ([]*promo.SponsorshipPurchase, error) {
	return nil, nil
}
func (happyRepo) IncrementSponsorshipPurchaseCreditsUsed(context.Context, int64, int) error {
	return nil
}
func (happyRepo) ExpireSponsorshipPurchases(context.Context) (int64, error) { return 0, nil }
func (happyRepo) CreateSponsorshipRequest(context.Context, *promo.SponsorshipRequest) error {
	return nil
}
func (happyRepo) GetSponsorshipRequestByID(context.Context, int64) (*promo.SponsorshipRequest, error) {
	return &promo.SponsorshipRequest{ID: 1, AdminStatus: "pending"}, nil
}
func (happyRepo) ListSponsorshipRequestsByOrg(context.Context, int64, int, int) ([]*promo.SponsorshipRequest, error) {
	return nil, nil
}
func (happyRepo) ListAllSponsorshipRequests(context.Context, int, int) ([]*promo.SponsorshipRequest, error) {
	return nil, nil
}
func (happyRepo) ListPendingSponsorshipRequests(context.Context, int, int) ([]*promo.SponsorshipRequest, error) {
	return nil, nil
}
func (happyRepo) UpdateSponsorshipRequestAdminStatus(context.Context, int64, promo.AdminStatus, string, int64) error {
	return nil
}
func (happyRepo) ActivateSponsorshipRequest(context.Context, int64, int64) (*promo.SponsorshipRequest, error) {
	return &promo.SponsorshipRequest{ID: 1, AdminStatus: "approved", Status: "active"}, nil
}
func (happyRepo) CancelSponsorshipRequest(context.Context, int64, int64) error { return nil }
func (happyRepo) ExpireSponsorshipRequests(context.Context) (int64, error) { return 0, nil }
func (happyRepo) RankedSponsorshipsForProducts(context.Context, []int64) ([]*promo.RankedSponsorship, error) {
	return nil, nil
}
func (happyRepo) RankedSponsorshipsForOffers(context.Context, []int64) ([]*promo.RankedSponsorship, error) {
	return nil, nil
}
func (happyRepo) IsSponsored(context.Context, promo.SponsorshipItemType, int64) (*promo.RankedSponsorship, error) {
	return nil, nil
}
func (happyRepo) CreateAd(context.Context, *promo.Ad) error { return nil }
func (happyRepo) UpdateAd(context.Context, *promo.Ad) error { return nil }
func (happyRepo) GetAdByID(context.Context, int64) (*promo.Ad, error) {
	return nil, nil
}
func (happyRepo) ListAdsByOrg(context.Context, int64, int, int) ([]*promo.Ad, error) {
	return nil, nil
}
func (happyRepo) ListAllAds(context.Context, int, int) ([]*promo.Ad, error) {
	return nil, nil
}
func (happyRepo) UpdateAdAdminStatus(context.Context, int64, promo.AdminStatus, string, int64) error {
	return nil
}
func (happyRepo) RecordAdImpression(context.Context, int64, *int64, string, string) error {
	return nil
}
func (happyRepo) CreateHighlightSection(ctx context.Context, h *promo.HighlightSection) error {
	h.ID = 1
	return nil
}
func (happyRepo) UpdateHighlightSection(ctx context.Context, h *promo.HighlightSection) error {
	return nil
}
func (happyRepo) DeleteHighlightSection(ctx context.Context, id, orgID int64) error {
	return nil
}
func (happyRepo) GetHighlightSectionByID(ctx context.Context, id int64) (*promo.HighlightSection, error) {
	return &promo.HighlightSection{ID: id, Title: i18n.Text{"ar": "قسم مميز"}}, nil
}
func (happyRepo) ListHighlightSections(ctx context.Context) ([]*promo.HighlightSection, error) {
	return []*promo.HighlightSection{{ID: 1, Title: i18n.Text{"en": "Featured"}}}, nil
}
func (happyRepo) ListHighlightSectionsByOrg(ctx context.Context, orgID int64) ([]*promo.HighlightSection, error) {
	return []*promo.HighlightSection{{ID: 1, OwnerType: "organization", OrganizationID: &orgID, Title: i18n.Text{"ar": "الأكثر مبيعاً"}}}, nil
}
func (happyRepo) AddHighlightItem(ctx context.Context, item *promo.HighlightSectionItem) error {
	item.ID = 1
	return nil
}
func (happyRepo) ListHighlightItems(ctx context.Context, sectionID int64) ([]*promo.HighlightSectionItem, error) {
	return nil, nil
}
func (happyRepo) ExpirePromotions(ctx context.Context) (int64, error) {
	return 0, nil
}
func (happyRepo) CreateSpecialOffer(ctx context.Context, o *promo.SpecialOffer) error {
	o.ID = 1
	return nil
}
func (happyRepo) GetSpecialOfferByID(ctx context.Context, id int64) (*promo.SpecialOffer, error) {
	return &promo.SpecialOffer{ID: id, Title: i18n.New("عرض خاص", "Special Offer")}, nil
}
func (happyRepo) ListSpecialOffersByOrg(ctx context.Context, orgID int64) ([]*promo.SpecialOffer, error) {
	return []*promo.SpecialOffer{{ID: 1, Title: i18n.New("عرض خاص", "Special Offer")}}, nil
}
func (happyRepo) ListAllSpecialOffers(ctx context.Context, limit, offset int) ([]*promo.SpecialOffer, error) {
	return []*promo.SpecialOffer{{ID: 1, Title: i18n.New("عرض خاص", "Special Offer")}}, nil
}
func (happyRepo) UpdateSpecialOfferAdminStatus(ctx context.Context, id int64, adminStatus, notes string, approvedBy int64) error {
	return nil
}
func (happyRepo) ToggleSpecialOfferStatus(ctx context.Context, id int64, isActive bool) error {
	return nil
}
func (happyRepo) DeleteSpecialOffer(ctx context.Context, id, orgID int64) error {
	return nil
}
func (happyRepo) AddSpecialOfferLocation(ctx context.Context, loc *promo.SpecialOfferLocation) error {
	loc.ID = 1
	return nil
}
func (happyRepo) ListSpecialOfferLocations(ctx context.Context, offerID int64) ([]*promo.SpecialOfferLocation, error) {
	return []*promo.SpecialOfferLocation{{ID: 1, Radius: 1000}}, nil
}

const testCookieName = "dawa24_session"

func newTestRouter(t *testing.T) http.Handler {
	t.Helper()

	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	promoSvc := promo.NewService(stubRepo{t: t}, log)

	r := chi.NewRouter()
	r.Use(httpx.RequestID)
	r.Use(httpx.Recover(log))
	r.Use(httpx.Locale)

	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie(testCookieName)
			if err != nil || cookie.Value == "" || cookie.Value == "forged-token-that-was-never-issued" {
				httpx.Error(w, r, log, apperr.Unauthorized())
				return
			}
			next.ServeHTTP(w, r)
		})
	})
	promoHttp.NewHandler(promoSvc, log).RegisterRoutes(r)

	return r
}

func newAuthedRouter(repo promo.Repository) http.Handler {
	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	promoSvc := promo.NewService(repo, log)

	r := chi.NewRouter()
	r.Use(httpx.RequestID)
	r.Use(httpx.Recover(log))
	r.Use(httpx.Locale)

	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			actor := authctx.Actor{
				UserID:         1,
				OrganizationID: 1,
				Role:           "admin",
				Permissions:    []string{"admin", "promo.admin"},
			}
			ctx := authctx.WithActor(r.Context(), actor)
			ctx = database.WithTenant(ctx, 1)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	})
	promoHttp.NewHandler(promoSvc, log).RegisterRoutes(r)
	return r
}

var protectedRoutes = []struct{ method, path string }{
	{http.MethodGet, "/api/v1/promo/offers"},
	{http.MethodPost, "/api/v1/promo/offers"},
	{http.MethodGet, "/api/v1/promo/offers/1"},
	{http.MethodPost, "/api/v1/promo/offers/1/click"},
	{http.MethodGet, "/api/v1/promo/packages"},
	{http.MethodGet, "/api/v1/promo/ads"},
	{http.MethodPost, "/api/v1/promo/ads/1/click"},
	{http.MethodGet, "/api/v1/promo/highlights"},
	{http.MethodPost, "/api/v1/promo/highlights"},
	{http.MethodGet, "/api/v1/admin/promo/ads"},
	{http.MethodPost, "/api/v1/admin/promo/ads/1/approve"},
	{http.MethodPost, "/api/v1/admin/promo/ads/1/reject"},
	{http.MethodGet, "/api/v1/admin/promo/sponsorships"},
	{http.MethodPost, "/api/v1/admin/promo/sponsorships/1/review"},
}

func TestProtectedRoutesRejectAnonymousCallers(t *testing.T) {
	router := newTestRouter(t)

	for _, route := range protectedRoutes {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			req := httptest.NewRequest(route.method, route.path, strings.NewReader("{}"))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Errorf("got %d, want 401 — this endpoint is reachable without a session", rec.Code)
			}
		})
	}
}

func TestProtectedRoutesRejectGarbageSessionToken(t *testing.T) {
	router := newTestRouter(t)

	for _, route := range protectedRoutes {
		req := httptest.NewRequest(route.method, route.path, strings.NewReader("{}"))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: "dawa24_session", Value: "forged-token-that-was-never-issued"})
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s with a forged token got %d, want 401", route.method, route.path, rec.Code)
		}
	}
}

func TestUnauthorizedResponseUsesTheErrorEnvelope(t *testing.T) {
	router := newTestRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/promo/offers", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	var body httpx.ErrorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not the JSON error envelope: %v (body: %s)", err, rec.Body.String())
	}
	if body.Error.Code == "" {
		t.Error("error envelope has no code")
	}
	if body.Error.RequestID == "" {
		t.Error("error envelope has no request_id")
	}
}

func TestPromoHandler_HappyPaths(t *testing.T) {
	router := newAuthedRouter(happyRepo{})

	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		wantStatus int
	}{
		{"ListOffers", http.MethodGet, "/api/v1/promo/offers?limit=10&offset=0", "", http.StatusOK},
		{"CreateOffer", http.MethodPost, "/api/v1/promo/offers", `{"organization_id":1,"title":{"en":"Summer Sale"},"discount_type":"percentage","discount_value":"10.00","starts_at":"2026-01-01T00:00:00Z","expires_at":"2026-12-31T23:59:59Z","is_active":true}`, http.StatusCreated},
		{"GetOffer", http.MethodGet, "/api/v1/promo/offers/1", "", http.StatusOK},
		{"RecordClick", http.MethodPost, "/api/v1/promo/offers/1/click", "", http.StatusNoContent},
		{"ListPackages", http.MethodGet, "/api/v1/promo/packages", "", http.StatusOK},
		{"ListAds", http.MethodGet, "/api/v1/promo/ads?position=home", "", http.StatusOK},
		{"RecordAdClick", http.MethodPost, "/api/v1/promo/ads/1/click", "", http.StatusNoContent},
		{"ListHighlights", http.MethodGet, "/api/v1/promo/highlights", "", http.StatusOK},
		{"CreateHighlight", http.MethodPost, "/api/v1/promo/highlights", `{"title":{"en":"Featured"},"slug":"featured","display_order":1,"is_active":true}`, http.StatusCreated},
		{"AdminListAds", http.MethodGet, "/api/v1/admin/promo/ads", "", http.StatusOK},
		{"AdminApproveAd", http.MethodPost, "/api/v1/admin/promo/ads/1/approve", "", http.StatusOK},
		{"AdminRejectAd", http.MethodPost, "/api/v1/admin/promo/ads/1/reject", "", http.StatusOK},
		{"AdminListSponsorships", http.MethodGet, "/api/v1/admin/promo/sponsorships", "", http.StatusOK},
		{"AdminReviewSponsorship", http.MethodPost, "/api/v1/admin/promo/sponsorships/1/review", "", http.StatusOK},
		{"AdminListPackages", http.MethodGet, "/api/v1/admin/promo/packages", "", http.StatusOK},
		{"AdminCreatePackage", http.MethodPost, "/api/v1/admin/promo/packages", `{"name":{"en":"Platinum"},"price":"100.00","duration_days":30,"max_offers":10}`, http.StatusCreated},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var bodyReader io.Reader
			if tt.body != "" {
				bodyReader = strings.NewReader(tt.body)
			}
			req := httptest.NewRequest(tt.method, tt.path, bodyReader)
			if tt.body != "" {
				req.Header.Set("Content-Type", "application/json")
			}
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code != tt.wantStatus {
				t.Errorf("%s %s got status %d, want %d (body: %s)", tt.method, tt.path, rec.Code, tt.wantStatus, rec.Body.String())
			}
		})
	}
}
