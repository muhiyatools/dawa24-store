package ui_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/muhiya/dawa24-store/internal/modules/promo"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/ui"
)

// mockPromoRepo is an in-memory test double for promo.Service admin operations.
type mockPromoRepo struct {
	packages []*promo.OfferPackage
	requests []*promo.SponsorshipRequest
	ads      []*promo.Ad
}

func newMockPromoRepo() *mockPromoRepo {
	return &mockPromoRepo{
		packages: []*promo.OfferPackage{
			{
				ID:           1,
				PublicID:     "pkg_gold",
				Name:         i18n.New("الباقة الذهبية", "Gold Package"),
				Description:  i18n.New("تثبيت 10 أصناف في الصدارة", "Pin 10 items in top results"),
				Price:        money.FromMajor(1500),
				DurationDays: 30,
				Credits:      10,
				MaxOffers:    20,
				TierLevel:    3,
				SortOrder:    1,
				BadgeColor:   "#f59e0b",
				IsFeatured:   true,
				IsActive:     true,
				CreatedAt:    time.Now(),
				UpdatedAt:    time.Now(),
			},
		},
		requests: []*promo.SponsorshipRequest{
			{
				ID:             101,
				OrganizationID: 42,
				PackageID:      1,
				ItemType:       "product",
				ItemID:         901,
				CreditsUsed:    1,
				AdminStatus:    "pending",
				CreatedAt:      time.Now(),
			},
		},
		ads: []*promo.Ad{
			{
				ID:          501,
				Title:       i18n.New("بنر الأدوية الترويجية", "Promo Medicines Banner"),
				Position:    "home_top",
				AdminStatus: "pending",
				Impressions: 1200,
				Clicks:      85,
				CTR:         7.08,
				CreatedAt:   time.Now(),
			},
		},
	}
}

func (m *mockPromoRepo) CreatePackage(ctx context.Context, p *promo.OfferPackage) error {
	p.ID = int64(len(m.packages) + 1)
	p.PublicID = "pkg_auto"
	p.CreatedAt = time.Now()
	p.UpdatedAt = time.Now()
	m.packages = append(m.packages, p)
	return nil
}

func (m *mockPromoRepo) UpdatePackage(ctx context.Context, p *promo.OfferPackage) error {
	for i, existing := range m.packages {
		if existing.ID == p.ID {
			m.packages[i] = p
			return nil
		}
	}
	return nil
}

func (m *mockPromoRepo) GetPackageByID(ctx context.Context, id int64) (*promo.OfferPackage, error) {
	for _, p := range m.packages {
		if p.ID == id {
			return p, nil
		}
	}
	return nil, nil
}

func (m *mockPromoRepo) AdminListPackages(ctx context.Context) ([]*promo.OfferPackage, error) {
	return m.packages, nil
}

func (m *mockPromoRepo) ListPackages(ctx context.Context) ([]*promo.OfferPackage, error) {
	var list []*promo.OfferPackage
	for _, p := range m.packages {
		if p.IsActive {
			list = append(list, p)
		}
	}
	return list, nil
}

func (m *mockPromoRepo) TogglePackageActive(ctx context.Context, id int64, active bool) error {
	for _, p := range m.packages {
		if p.ID == id {
			p.IsActive = active
			return nil
		}
	}
	return nil
}

func (m *mockPromoRepo) ListAllSponsorshipRequests(ctx context.Context, limit, offset int) ([]*promo.SponsorshipRequest, error) {
	return m.requests, nil
}

func (m *mockPromoRepo) ActivateSponsorshipRequest(ctx context.Context, requestID int64, reviewerID *int64) (*promo.SponsorshipRequest, error) {
	for _, r := range m.requests {
		if r.ID == requestID {
			r.AdminStatus = "approved"
			return r, nil
		}
	}
	return nil, nil
}

func (m *mockPromoRepo) UpdateSponsorshipRequestAdminStatus(ctx context.Context, requestID int64, status promo.AdminReviewStatus, notes string, reviewerID *int64) error {
	for _, r := range m.requests {
		if r.ID == requestID {
			r.AdminStatus = string(status)
			return nil
		}
	}
	return nil
}

func (m *mockPromoRepo) AdminListAds(ctx context.Context, limit, offset int) ([]*promo.Ad, error) {
	return m.ads, nil
}

func (m *mockPromoRepo) UpdateAdAdminStatus(ctx context.Context, id int64, status promo.AdminReviewStatus, notes string, reviewerID *int64) error {
	for _, a := range m.ads {
		if a.ID == id {
			a.AdminStatus = string(status)
			return nil
		}
	}
	return nil
}

func (m *mockPromoRepo) RecordAdImpression(ctx context.Context, adID int64, userID *int64, ip, ua string) error {
	return nil
}

func (m *mockPromoRepo) GetAdByID(ctx context.Context, id int64) (*promo.Ad, error) {
	for _, a := range m.ads {
		if a.ID == id {
			return a, nil
		}
	}
	return nil, nil
}

func (m *mockPromoRepo) ListActiveAds(ctx context.Context, position string, limit int) ([]*promo.Ad, error) {
	return m.ads, nil
}

func (m *mockPromoRepo) RecordAdClick(ctx context.Context, adID int64, userID *int64, ip, ua string) error {
	return nil
}

func (m *mockPromoRepo) PurchaseSponsorship(ctx context.Context, orgID, pkgID int64, paymentMethod string, paymentRef *string) (*promo.SponsorshipPurchase, error) {
	return nil, nil
}

func (m *mockPromoRepo) CreateSponsorshipRequest(ctx context.Context, req *promo.SponsorshipRequest) error {
	return nil
}

func (m *mockPromoRepo) ListSponsorshipPurchases(ctx context.Context, orgID int64) ([]*promo.SponsorshipPurchase, error) {
	return nil, nil
}

func (m *mockPromoRepo) ListActiveSponsorshipPurchases(ctx context.Context, orgID int64) ([]*promo.SponsorshipPurchase, error) {
	return nil, nil
}

func (m *mockPromoRepo) ListSponsorshipRequests(ctx context.Context, orgID int64) ([]*promo.SponsorshipRequest, error) {
	return nil, nil
}

func (m *mockPromoRepo) CancelSponsorshipRequest(ctx context.Context, requestID, orgID int64) error {
	return nil
}

func (m *mockPromoRepo) IsItemSponsored(ctx context.Context, itemType string, itemID int64) (bool, *promo.OfferPackage, error) {
	return false, nil, nil
}

func (m *mockPromoRepo) PinSponsoredOfferToTop(ctx context.Context, offerID int64) error {
	return nil
}

func (m *mockPromoRepo) CreateAd(ctx context.Context, ad *promo.Ad) error {
	return nil
}

func (m *mockPromoRepo) ListAdsByOrg(ctx context.Context, orgID int64) ([]*promo.Ad, error) {
	return nil, nil
}

func (m *mockPromoRepo) TrackAdImpression(ctx context.Context, id int64) error {
	return nil
}

func (m *mockPromoRepo) TrackAdClick(ctx context.Context, id int64) error {
	return nil
}

func setupTestPromoHandler(repo promo.Repository) (*ui.UIHandler, *chi.Mux) {
	promoSvc := promo.NewService(repo)
	handler := ui.New(ui.Config{
		PromoSvc: promoSvc,
	})

	r := chi.NewRouter()
	r.Get("/admin/offers-packages", handler.AdminOffersPackagesHubPage)
	r.Post("/admin/offers-packages/new", handler.AdminOfferPackageCreateSubmit)
	r.Post("/admin/offers-packages/{id}/edit", handler.AdminOfferPackageEditSubmit)
	r.Post("/admin/offers-packages/{id}/toggle", handler.AdminOfferPackageToggleSubmit)
	r.Post("/admin/offers-packages/sponsorships/{id}/approve", handler.AdminSponsorshipRequestApproveSubmit)
	r.Post("/admin/offers-packages/sponsorships/{id}/reject", handler.AdminSponsorshipRequestRejectSubmit)
	r.Post("/admin/ads/{id}/approve", handler.AdminAdApproveSubmit)
	r.Post("/admin/ads/{id}/reject", handler.AdminAdRejectSubmit)
	r.Get("/admin/ad-plan", handler.AdminAdPlansPage)
	r.Get("/admin/offers-packages/packages", handler.AdminOfferPackagesListPage)

	return handler, r
}

func TestAdminOffersPackages_HubPageRenders(t *testing.T) {
	repo := newMockPromoRepo()
	_, r := setupTestPromoHandler(repo)

	req := httptest.NewRequest(http.MethodGet, "/admin/offers-packages?tab=packages", nil)
	ctx := authctx.WithUserID(req.Context(), 1)
	ctx = authctx.WithRole(ctx, authctx.RoleAdmin)
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, "منظومة باقات الرعايات والعروض الترويجية")
	assert.Contains(t, body, "الباقة الذهبية")
	assert.Contains(t, body, "باقات الرعايات المتاحة")
	assert.Contains(t, body, "مراجعة طلبات الرعاية")
	assert.Contains(t, body, "مراجعة الإعلانات والبنرات")
}

func TestAdminOffersPackages_CreateNewPackage(t *testing.T) {
	repo := newMockPromoRepo()
	_, r := setupTestPromoHandler(repo)

	form := url.Values{}
	form.Set("name_ar", "الباقة الماسية المميزة")
	form.Set("name_en", "Diamond VIP Package")
	form.Set("desc_ar", "تثبيت 25 صنف في الصدارة")
	form.Set("desc_en", "Pin 25 items at top")
	form.Set("price", "3500.00")
	form.Set("duration_days", "60")
	form.Set("credits", "25")
	form.Set("tier_level", "5")
	form.Set("max_offers", "50")
	form.Set("badge_color", "#8b5cf6")
	form.Set("is_featured", "true")
	form.Set("is_active", "true")

	req := httptest.NewRequest(http.MethodPost, "/admin/offers-packages/new", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	ctx := authctx.WithUserID(req.Context(), 1)
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusSeeOther, rec.Code)
	loc := rec.Header().Get("Location")
	assert.Contains(t, loc, "/admin/offers-packages?tab=packages")
	assert.Contains(t, loc, "notice=success")

	require.Len(t, repo.packages, 2)
	created := repo.packages[1]
	assert.Equal(t, "الباقة الماسية المميزة", created.Name.Get(i18n.AR))
	assert.Equal(t, "Diamond VIP Package", created.Name.Get(i18n.EN))
	assert.Equal(t, 5, created.TierLevel)
	assert.Equal(t, 25, created.Credits)
	assert.True(t, created.IsFeatured)
	assert.True(t, created.IsActive)
}

func TestAdminOffersPackages_EditPackage(t *testing.T) {
	repo := newMockPromoRepo()
	_, r := setupTestPromoHandler(repo)

	form := url.Values{}
	form.Set("name_ar", "الباقة الذهبية المعدلة")
	form.Set("name_en", "Updated Gold Package")
	form.Set("desc_ar", "وصف محدث")
	form.Set("desc_en", "Updated desc")
	form.Set("price", "1800.00")
	form.Set("duration_days", "45")
	form.Set("credits", "15")
	form.Set("tier_level", "4")
	form.Set("max_offers", "30")
	form.Set("badge_color", "#0284c7")
	form.Set("is_featured", "false")
	form.Set("is_active", "true")

	req := httptest.NewRequest(http.MethodPost, "/admin/offers-packages/1/edit", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	ctx := authctx.WithUserID(req.Context(), 1)
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusSeeOther, rec.Code)
	loc := rec.Header().Get("Location")
	assert.Contains(t, loc, "/admin/offers-packages?tab=packages")
	assert.Contains(t, loc, "notice=success")

	updated := repo.packages[0]
	assert.Equal(t, "الباقة الذهبية المعدلة", updated.Name.Get(i18n.AR))
	assert.Equal(t, 15, updated.Credits)
	assert.Equal(t, 4, updated.TierLevel)
}

func TestAdminOffersPackages_ToggleActive(t *testing.T) {
	repo := newMockPromoRepo()
	_, r := setupTestPromoHandler(repo)

	form := url.Values{}
	form.Set("active", "false")

	req := httptest.NewRequest(http.MethodPost, "/admin/offers-packages/1/toggle", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	ctx := authctx.WithUserID(req.Context(), 1)
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusSeeOther, rec.Code)
	assert.False(t, repo.packages[0].IsActive)
}

func TestAdminOffersPackages_LegacyAdPlanRedirect(t *testing.T) {
	repo := newMockPromoRepo()
	_, r := setupTestPromoHandler(repo)

	req := httptest.NewRequest(http.MethodGet, "/admin/ad-plan", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusMovedPermanently, rec.Code)
	assert.Equal(t, "/admin/offers-packages?tab=packages", rec.Header().Get("Location"))
}

func TestAdminOffersPackages_SponsorshipApproveAndReject(t *testing.T) {
	repo := newMockPromoRepo()
	_, r := setupTestPromoHandler(repo)

	// Approve
	req := httptest.NewRequest(http.MethodPost, "/admin/offers-packages/sponsorships/101/approve", nil)
	ctx := authctx.WithUserID(req.Context(), 1)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusSeeOther, rec.Code)
	assert.Contains(t, rec.Header().Get("Location"), "/admin/offers-packages?tab=requests")
	assert.Equal(t, "approved", repo.requests[0].AdminStatus)

	// Reject
	reqReject := httptest.NewRequest(http.MethodPost, "/admin/offers-packages/sponsorships/101/reject", strings.NewReader("notes=does+not+comply"))
	reqReject.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqReject = reqReject.WithContext(ctx)
	recReject := httptest.NewRecorder()
	r.ServeHTTP(recReject, reqReject)

	assert.Equal(t, http.StatusSeeOther, recReject.Code)
	assert.Equal(t, "rejected", repo.requests[0].AdminStatus)
}
