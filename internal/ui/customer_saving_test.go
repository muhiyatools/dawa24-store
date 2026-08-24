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

func TestCustomerPhase7Routes(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := ui.NewUIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, logger)

	r := chi.NewRouter()
	handler.RegisterCustomerRoutes(r)
	handler.RegisterPublicRoutes(r)

	// /customer/cpanel is a 301 to /customer/dashboard: it was a link hub to
	// three pages already in the sidebar (PLAN_V7 Task 3.2).
	tests := []struct {
		name       string
		path       string
		method     string
		actor      *authctx.Actor
		wantStatus int
	}{
		{
			name:   "Customer GET /customer/saving-products returns 200",
			path:   "/customer/saving-products",
			method: "GET",
			actor: &authctx.Actor{
				UserID:         1,
				OrganizationID: 5,
				OrgType:        "customer",
			},
			wantStatus: http.StatusOK,
		},
		{
			name:   "Misspelled /customer/saveing-products 301 redirects",
			path:   "/customer/saveing-products",
			method: "GET",
			actor: &authctx.Actor{
				UserID:         1,
				OrganizationID: 5,
				OrgType:        "customer",
			},
			wantStatus: http.StatusMovedPermanently,
		},
		{
			name:   "Customer GET /orders/offers 301 redirects to /orders",
			path:   "/orders/offers",
			method: "GET",
			actor: &authctx.Actor{
				UserID:         1,
				OrganizationID: 5,
				OrgType:        "customer",
			},
			wantStatus: http.StatusMovedPermanently,
		},
		{
			name:   "Customer GET /customer/add-order returns 200",
			path:   "/customer/add-order",
			method: "GET",
			actor: &authctx.Actor{
				UserID:         1,
				OrganizationID: 5,
				OrgType:        "customer",
			},
			wantStatus: http.StatusOK,
		},
		{
			name:   "Customer GET /customer/products/main/10 redirects to /catalog/10",
			path:   "/customer/products/main/10",
			method: "GET",
			actor: &authctx.Actor{
				UserID:         1,
				OrganizationID: 5,
				OrgType:        "customer",
			},
			wantStatus: http.StatusMovedPermanently,
		},
		{
			name:       "Anonymous GET /tracking returns 200",
			path:       "/tracking",
			method:     "GET",
			actor:      nil,
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
