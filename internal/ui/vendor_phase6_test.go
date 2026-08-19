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

func TestVendorPhase6Routes(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := ui.NewUIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, logger)

	r := chi.NewRouter()
	handler.RegisterVendorRoutes(r)

	tests := []struct {
		name       string
		path       string
		method     string
		actor      *authctx.Actor
		wantStatus int
	}{
		{
			name:       "Anonymous GET /vendor/warehouses redirects to login",
			path:       "/vendor/warehouses",
			method:     "GET",
			actor:      nil,
			wantStatus: http.StatusSeeOther,
		},
		{
			name:   "Vendor GET /vendor/warehouses returns 200",
			path:   "/vendor/warehouses",
			method: "GET",
			actor: &authctx.Actor{
				UserID:         10,
				OrganizationID: 2,
				OrgType:        "supplier",
			},
			wantStatus: http.StatusOK,
		},
		{
			name:   "Vendor GET /vendor/catalog/select returns 200",
			path:   "/vendor/catalog/select",
			method: "GET",
			actor: &authctx.Actor{
				UserID:         10,
				OrganizationID: 2,
				OrgType:        "supplier",
			},
			wantStatus: http.StatusOK,
		},
		{
			name:   "Vendor GET /vendor/saving-products returns 200",
			path:   "/vendor/saving-products",
			method: "GET",
			actor: &authctx.Actor{
				UserID:         10,
				OrganizationID: 2,
				OrgType:        "supplier",
			},
			wantStatus: http.StatusOK,
		},
		{
			name:   "Misspelled /vendor/saveing-products 301 redirects",
			path:   "/vendor/saveing-products",
			method: "GET",
			actor: &authctx.Actor{
				UserID:         10,
				OrganizationID: 2,
				OrgType:        "supplier",
			},
			wantStatus: http.StatusMovedPermanently,
		},
		{
			name:   "Vendor GET /vendor/payments returns 200",
			path:   "/vendor/payments",
			method: "GET",
			actor: &authctx.Actor{
				UserID:         10,
				OrganizationID: 2,
				OrgType:        "supplier",
			},
			wantStatus: http.StatusOK,
		},
		{
			name:   "Vendor GET /vendor/earnings/order returns 200",
			path:   "/vendor/earnings/order",
			method: "GET",
			actor: &authctx.Actor{
				UserID:         10,
				OrganizationID: 2,
				OrgType:        "supplier",
			},
			wantStatus: http.StatusOK,
		},
		{
			name:   "Vendor GET /vendor/earnings/offers returns 200",
			path:   "/vendor/earnings/offers",
			method: "GET",
			actor: &authctx.Actor{
				UserID:         10,
				OrganizationID: 2,
				OrgType:        "supplier",
			},
			wantStatus: http.StatusOK,
		},
		{
			name:   "Vendor GET /vendor/activities returns 200",
			path:   "/vendor/activities",
			method: "GET",
			actor: &authctx.Actor{
				UserID:         10,
				OrganizationID: 2,
				OrgType:        "supplier",
			},
			wantStatus: http.StatusOK,
		},
		{
			name:   "Vendor GET /vendor/policies returns 200",
			path:   "/vendor/policies",
			method: "GET",
			actor: &authctx.Actor{
				UserID:         10,
				OrganizationID: 2,
				OrgType:        "supplier",
			},
			wantStatus: http.StatusOK,
		},
		{
			name:   "Vendor GET /vendor/social-media returns 200",
			path:   "/vendor/social-media",
			method: "GET",
			actor: &authctx.Actor{
				UserID:         10,
				OrganizationID: 2,
				OrgType:        "supplier",
			},
			wantStatus: http.StatusOK,
		},
		{
			name:   "Vendor GET /vendor/team/import returns 200",
			path:   "/vendor/team/import",
			method: "GET",
			actor: &authctx.Actor{
				UserID:         10,
				OrganizationID: 2,
				OrgType:        "supplier",
			},
			wantStatus: http.StatusOK,
		},
		{
			name:   "Vendor GET /vendor/institutional-work returns 200",
			path:   "/vendor/institutional-work",
			method: "GET",
			actor: &authctx.Actor{
				UserID:         10,
				OrganizationID: 2,
				OrgType:        "supplier",
			},
			wantStatus: http.StatusOK,
		},
		{
			name:   "Vendor GET /vendor/pharmacy-coverage returns 200",
			path:   "/vendor/pharmacy-coverage",
			method: "GET",
			actor: &authctx.Actor{
				UserID:         10,
				OrganizationID: 2,
				OrgType:        "supplier",
			},
			wantStatus: http.StatusOK,
		},
		{
			name:   "Vendor GET /vendor/orders/offers returns 200",
			path:   "/vendor/orders/offers",
			method: "GET",
			actor: &authctx.Actor{
				UserID:         10,
				OrganizationID: 2,
				OrgType:        "supplier",
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
