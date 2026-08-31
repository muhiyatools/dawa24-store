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

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/muhiya/dawa24-store/internal/modules/promo"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/ui"
)

func TestAdminAdvProducts_PageRenders(t *testing.T) {
	mockRepo := newMockAdminPromoRepo()
	promoSvc := promo.NewService(mockRepo, slog.Default())
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := ui.NewUIHandler(nil, nil, nil, nil, nil, nil, nil, promoSvc, nil, nil, nil, nil, nil, nil, logger)

	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			actor := authctx.Actor{
				UserID:      1,
				IsStaff:     true,
				Role:        "super_admin",
				Permissions: []string{"*"},
			}
			ctx := authctx.WithActor(req.Context(), actor)
			next.ServeHTTP(w, req.WithContext(ctx))
		})
	})
	r.Get("/admin/adv-products", handler.AdminAdvProductsPage)
	r.Post("/admin/adv-products/{id}/approve", handler.AdminAdvProductApproveSubmit)
	r.Post("/admin/adv-products/{id}/reject", handler.AdminAdvProductRejectSubmit)
	r.Post("/admin/adv-products/new", handler.AdminAdvProductCreateSubmit)

	t.Run("GET /admin/adv-products renders hub with correct title and tables", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/admin/adv-products", nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		require.Equal(t, http.StatusOK, rec.Code)
		body, _ := io.ReadAll(rec.Body)
		bodyStr := string(body)

		assert.Contains(t, bodyStr, "رعاية المنتجات")
		assert.Contains(t, bodyStr, "Product Sponsorships")
		assert.Contains(t, bodyStr, "إجمالي المنتجات المروجة")
		assert.Contains(t, bodyStr, "الرعايات النشطة حالياً")
		assert.Contains(t, bodyStr, "طلبات بانتظار الموافقة")
		assert.Contains(t, bodyStr, "سجل رعاية وتثبيت المنتجات")
	})

	t.Run("POST /admin/adv-products/101/approve approves sponsorship request", func(t *testing.T) {
		form := url.Values{}
		form.Set("notes", "معتمد وموافق عليه")

		req := httptest.NewRequest(http.MethodPost, "/admin/adv-products/101/approve", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusSeeOther, rec.Code)
		assert.Contains(t, rec.Header().Get("Location"), "/admin/adv-products")

		// Verify status in mock
		sr, err := mockRepo.GetSponsorshipRequestByID(context.Background(), 101)
		require.NoError(t, err)
		require.NotNil(t, sr)
		assert.Equal(t, promo.AdminApproved, sr.AdminStatus)
	})

	t.Run("POST /admin/adv-products/101/reject rejects sponsorship request", func(t *testing.T) {
		// Reset status to pending
		mockRepo.requests[0].AdminStatus = promo.AdminPending

		form := url.Values{}
		form.Set("notes", "البيانات غير مكتملة")

		req := httptest.NewRequest(http.MethodPost, "/admin/adv-products/101/reject", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusSeeOther, rec.Code)
		assert.Contains(t, rec.Header().Get("Location"), "/admin/adv-products")

		// Verify status in mock
		sr, err := mockRepo.GetSponsorshipRequestByID(context.Background(), 101)
		require.NoError(t, err)
		require.NotNil(t, sr)
		assert.Equal(t, promo.AdminRejected, sr.AdminStatus)
	})
}
