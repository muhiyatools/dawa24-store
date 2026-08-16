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

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/identity"
	identityHttp "github.com/muhiya/dawa24-store/internal/modules/identity/http"
	"github.com/muhiya/dawa24-store/internal/modules/promo"
	promoHttp "github.com/muhiya/dawa24-store/internal/modules/promo/http"
	"github.com/muhiya/dawa24-store/internal/platform/httpx"
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

func (r stubRepo) CreateHighlightSection(context.Context, *promo.HighlightSection) error {
	r.fail("CreateHighlightSection")
	return nil
}
func (r stubRepo) ListHighlightSections(context.Context) ([]*promo.HighlightSection, error) {
	r.fail("ListHighlightSections")
	return nil, nil
}
func (r stubRepo) ExpirePromotions(context.Context) (int64, error) {
	r.fail("ExpirePromotions")
	return 0, nil
}

const testCookieName = "dawa24_session"

func newTestRouter(t *testing.T) http.Handler {
	t.Helper()

	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	idSvc := identity.NewService(nil, nil, log)
	promoSvc := promo.NewService(stubRepo{t: t}, log)

	r := chi.NewRouter()
	r.Use(httpx.RequestID)
	r.Use(httpx.Recover(log))
	r.Use(httpx.Locale)

	r.Group(func(protected chi.Router) {
		protected.Use(identityHttp.RequireAuth(idSvc, testCookieName, log))
		promoHttp.NewHandler(promoSvc, log).RegisterRoutes(protected)
	})

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
		req.AddCookie(&http.Cookie{Name: testCookieName, Value: "forged-token-that-was-never-issued"})
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
		t.Error("error envelope has no code; clients cannot branch on it")
	}
	if body.Error.RequestID == "" {
		t.Error("error envelope has no request_id")
	}
}
