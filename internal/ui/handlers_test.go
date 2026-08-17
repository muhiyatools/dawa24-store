package ui_test

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/ui"
)

func setupTestRouter() http.Handler {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := ui.NewUIHandler(
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, logger,
	)

	r := chi.NewRouter()
	handler.RegisterPageRoutes(r)
	return r
}

func TestPublicAndAuthPageRoutes(t *testing.T) {
	router := setupTestRouter()

	routes := []struct {
		method string
		path   string
	}{
		{"GET", "/"},
		{"GET", "/privacy"},
		{"GET", "/terms"},
		{"GET", "/auth/login"},
		{"GET", "/auth/register"},
		{"GET", "/auth/forgot"},
		{"GET", "/auth/reset?token=test-reset-tok"},
		{"GET", "/onboarding"},
		{"GET", "/admin/dashboard"},
		{"GET", "/admin/settings"},
		{"GET", "/vendor/products/new"},
	}

	for _, route := range routes {
		t.Run(route.path, func(t *testing.T) {
			req := httptest.NewRequest(route.method, route.path, nil)
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Errorf("%s %s returned status %d, want 200", route.method, route.path, rec.Code)
			}
		})
	}
}

func TestHTMXPartialHeaderHandling(t *testing.T) {
	router := setupTestRouter()

	// Anonymous request to cart renders empty state partial
	req := httptest.NewRequest("GET", "/cart", nil)
	req.Header.Set("HX-Request", "true")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("GET /cart with HX-Request returned status %d, want 200", rec.Code)
	}
}

func TestAuthenticatedUIRoutesWithActor(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := ui.NewUIHandler(
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, logger,
	)

	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			actor := authctx.Actor{
				UserID:         100,
				OrganizationID: 200,
				Role:           "customer",
			}
			next.ServeHTTP(w, req.WithContext(authctx.WithActor(req.Context(), actor)))
		})
	})
	handler.RegisterPageRoutes(r)

	req := httptest.NewRequest("GET", "/cart", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("GET /cart with actor returned status %d, want 200", rec.Code)
	}
}

func TestFormActionRoutes(t *testing.T) {
	router := setupTestRouter()

	actionRoutes := []struct {
		method string
		path   string
	}{
		{"POST", "/auth/logout"},
		{"GET", "/auth/logout"},
		{"POST", "/admin/settings"},
		{"POST", "/cart/add"},
		{"POST", "/cart/remove"},
		{"POST", "/checkout"},
		{"POST", "/notifications/123/read"},
		{"POST", "/vendor/products"},
		{"POST", "/vendor/orders/456/status"},
	}

	for _, route := range actionRoutes {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			req := httptest.NewRequest(route.method, route.path, nil)
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			// Actions perform redirects (303 See Other)
			if rec.Code != http.StatusSeeOther {
				t.Errorf("%s %s returned status %d, want %d", route.method, route.path, rec.Code, http.StatusSeeOther)
			}
		})
	}
}
