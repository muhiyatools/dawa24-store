package ui_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/ui"
)

func TestAdminCatalogAndInventoryRoutes(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := ui.NewUIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, logger)

	r := chi.NewRouter()
	handler.RegisterAdminRoutes(r)

	tests := []struct {
		name       string
		path       string
		method     string
		actor      *authctx.Actor
		wantStatus int
	}{
		{
			name:       "Anonymous GET /admin/stocks redirects to login",
			path:       "/admin/stocks",
			method:     "GET",
			actor:      nil,
			wantStatus: http.StatusSeeOther,
		},
		{
			name:   "Super admin GET /admin/stocks returns 200",
			path:   "/admin/stocks",
			method: "GET",
			actor: &authctx.Actor{
				UserID:  1,
				IsStaff: true,
				Role:    "super_admin",
			},
			wantStatus: http.StatusOK,
		},
		{
			name:   "Super admin GET /admin/warehouses returns 200",
			path:   "/admin/warehouses",
			method: "GET",
			actor: &authctx.Actor{
				UserID:  1,
				IsStaff: true,
				Role:    "super_admin",
			},
			wantStatus: http.StatusOK,
		},
		{
			name:   "Super admin GET /admin/user/temparte-warehouses returns 200",
			path:   "/admin/user/temparte-warehouses",
			method: "GET",
			actor: &authctx.Actor{
				UserID:  1,
				IsStaff: true,
				Role:    "super_admin",
			},
			wantStatus: http.StatusOK,
		},
		{
			name:   "Super admin GET /admin/saving-products returns 200",
			path:   "/admin/saving-products",
			method: "GET",
			actor: &authctx.Actor{
				UserID:  1,
				IsStaff: true,
				Role:    "super_admin",
			},
			wantStatus: http.StatusOK,
		},
		{
			name:   "Misspelled /admin/saveing-products 301 redirects to /admin/saving-products",
			path:   "/admin/saveing-products",
			method: "GET",
			actor: &authctx.Actor{
				UserID:  1,
				IsStaff: true,
				Role:    "super_admin",
			},
			wantStatus: http.StatusMovedPermanently,
		},
		{
			name:   "Super admin GET /admin/adv-products returns 200",
			path:   "/admin/adv-products",
			method: "GET",
			actor: &authctx.Actor{
				UserID:  1,
				IsStaff: true,
				Role:    "super_admin",
			},
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			if tt.actor != nil {
				ctx = authctx.WithActor(ctx, *tt.actor)
			}

			req, _ := http.NewRequestWithContext(ctx, tt.method, tt.path, nil)
			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)

			if rr.Code != tt.wantStatus {
				t.Errorf("path %s expected status %d, got %d", tt.path, tt.wantStatus, rr.Code)
			}
		})
	}
}
