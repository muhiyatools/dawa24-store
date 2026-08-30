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

func newTestRouter(actor *authctx.Actor) http.Handler {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := ui.NewUIHandler(
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, logger,
	)

	// Mirrors cmd/server/routes.go, except session-based RequireAuth is
	// replaced by a stub that carries an optional test actor.
	stubAuth := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if _, ok := authctx.From(req.Context()); ok {
				next.ServeHTTP(w, req)
				return
			}
			if actor != nil {
				next.ServeHTTP(w, req.WithContext(authctx.WithActor(req.Context(), *actor)))
				return
			}
			next.ServeHTTP(w, req)
		})
	}

	r := chi.NewRouter()
	handler.RegisterPublicRoutes(r)
	r.Group(func(uiRouter chi.Router) {
		uiRouter.Use(stubAuth)
		uiRouter.Use(authctx.RequireCustomer(logger))
		uiRouter.Use(authctx.RequireApproved(logger))
		handler.RegisterCustomerRoutes(uiRouter)
	})
	r.Group(func(uiRouter chi.Router) {
		uiRouter.Use(stubAuth)
		uiRouter.Use(authctx.RequireVendor(logger))
		uiRouter.Use(authctx.RequireApproved(logger))
		handler.RegisterVendorRoutes(uiRouter)
	})
	r.Group(func(uiRouter chi.Router) {
		uiRouter.Use(stubAuth)
		uiRouter.Use(authctx.RequireStaff(logger))
		handler.RegisterAdminRoutes(uiRouter)
	})
	r.Group(func(uiRouter chi.Router) {
		uiRouter.Use(stubAuth)
		handler.RegisterSharedRoutes(uiRouter)
	})
	return r
}

func setupTestRouter() http.Handler {
	return newTestRouter(&authctx.Actor{
		UserID:      1,
		IsStaff:     true,
		Role:        "super_admin",
		OrgStatus:   "approved",
		Permissions: []string{"*"},
	})
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
		{"GET", "/catalog"},
		{"GET", "/jobs"},
		{"GET", "/suppliers"},
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

	// Anonymous request to catalog partial renders successfully
	req := httptest.NewRequest("GET", "/catalog", nil)
	req.Header.Set("HX-Request", "true")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("GET /catalog with HX-Request returned status %d, want 200", rec.Code)
	}
}

func TestAuthenticatedUIRoutesWithActor(t *testing.T) {
	actor := authctx.Actor{
		UserID:         100,
		OrganizationID: 200,
		Role:           "user",
		OrgType:        "customer",
		OrgStatus:      "approved",
		// The pharmacy dashboard is permission-gated now. This double stands
		// in for a company owner, who holds their whole dashboard.
		Permissions: []string{"pharmacy.*"},
	}
	router := newTestRouter(&actor)

	req := httptest.NewRequest("GET", "/cart", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("GET /cart with actor returned status %d, want 200", rec.Code)
	}
}

func TestFormActionRoutes(t *testing.T) {
	staffActor := &authctx.Actor{
		UserID:      1,
		IsStaff:     true,
		Role:        "super_admin",
		OrgStatus:   "approved",
		Permissions: []string{"*"},
	}
	customerActor := &authctx.Actor{
		UserID:         10,
		OrganizationID: 100,
		OrgType:        "customer",
		OrgStatus:      "approved",
		Permissions:    []string{"pharmacy.*", "*"},
	}
	vendorActor := &authctx.Actor{
		UserID:         20,
		OrganizationID: 200,
		OrgType:        "vendor",
		OrgStatus:      "approved",
		Permissions:    []string{"vendor.*", "*"},
	}

	actionRoutes := []struct {
		method string
		path   string
		actor  *authctx.Actor
	}{
		{"POST", "/auth/logout", staffActor},
		{"GET", "/auth/logout", staffActor},
		{"POST", "/admin/settings", staffActor},
		{"POST", "/cart/add", customerActor},
		{"POST", "/cart/remove", customerActor},
		{"POST", "/checkout", customerActor},
		{"POST", "/notifications/123/read", staffActor},
		{"POST", "/vendor/variants/new", vendorActor},
		{"POST", "/vendor/orders/456/status", vendorActor},
		{"POST", "/admin/products/new", staffActor},
	}

	for _, route := range actionRoutes {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			router := newTestRouter(route.actor)
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

func TestAdminProductSampleDownloads(t *testing.T) {
	actor := authctx.Actor{
		UserID:      1,
		Role:        "super_admin",
		IsStaff:     true,
		OrgStatus:   "approved",
		Permissions: []string{"*"},
	}
	router := newTestRouter(&actor)

	t.Run("GET /admin/products/sample.csv", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/admin/products/sample.csv", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("want 200, got %d", rec.Code)
		}
		if ct := rec.Header().Get("Content-Type"); ct != "text/csv; charset=utf-8" {
			t.Fatalf("want text/csv, got %q", ct)
		}
		if len(rec.Body.Bytes()) < 100 {
			t.Fatalf("CSV content too small: %d bytes", len(rec.Body.Bytes()))
		}
	})

	t.Run("GET /admin/products/sample.xlsx", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/admin/products/sample.xlsx", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("want 200, got %d", rec.Code)
		}
		if ct := rec.Header().Get("Content-Type"); ct != "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet" {
			t.Fatalf("want spreadsheetml.sheet, got %q", ct)
		}
		if len(rec.Body.Bytes()) < 100 {
			t.Fatalf("XLSX content too small: %d bytes", len(rec.Body.Bytes()))
		}
	})
}

func TestVendorIngestAndRolesRoutes(t *testing.T) {
	actor := &authctx.Actor{
		UserID:         1,
		OrganizationID: 1,
		OrgType:        "vendor",
		OrgStatus:      "approved",
		Role:           "vendor",
		IsOwner:        true,
		Permissions:    []string{"*"},
	}
	router := newTestRouter(actor)

	t.Run("GET /vendor/roles", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/vendor/roles", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("want 200, got %d", rec.Code)
		}
	})

	t.Run("GET /vendor/ingest/sample.csv", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/vendor/ingest/sample.csv", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("want 200, got %d", rec.Code)
		}
		if ct := rec.Header().Get("Content-Type"); ct != "text/csv; charset=utf-8" {
			t.Fatalf("want text/csv, got %q", ct)
		}
		if len(rec.Body.Bytes()) < 50 {
			t.Fatalf("CSV template too small")
		}
	})

	t.Run("GET /vendor/ingest/sample.xlsx", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/vendor/ingest/sample.xlsx", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("want 200, got %d", rec.Code)
		}
		if ct := rec.Header().Get("Content-Type"); ct != "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet" {
			t.Fatalf("want excel content type, got %q", ct)
		}
		if len(rec.Body.Bytes()) < 100 {
			t.Fatalf("Excel template too small")
		}
	})

	t.Run("GET /vendor/ingest/inventory.csv", func(t *testing.T) {
		// The vendor's live inventory dump. The import's own results export now
		// lives under a run id, because it exports one run rather than the
		// catalogue.
		req := httptest.NewRequest("GET", "/vendor/ingest/inventory.csv", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("want 200, got %d", rec.Code)
		}
		if ct := rec.Header().Get("Content-Type"); ct != "text/csv; charset=utf-8" {
			t.Fatalf("want text/csv, got %q", ct)
		}
	})
}
