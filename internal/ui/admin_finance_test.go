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

func TestAdminFinanceRoutes(t *testing.T) {
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
			name:       "Anonymous GET /admin/orders/offers redirects to login",
			path:       "/admin/orders/offers",
			method:     "GET",
			actor:      nil,
			wantStatus: http.StatusSeeOther,
		},
		{
			name:   "Super admin GET /admin/orders/offers returns 200",
			path:   "/admin/orders/offers",
			method: "GET",
			actor: &authctx.Actor{
				UserID:  1,
				IsStaff: true,
				Role:    "super_admin",
			},
			wantStatus: http.StatusOK,
		},
		{
			name:   "Super admin GET /admin/earnings/order returns 200",
			path:   "/admin/earnings/order",
			method: "GET",
			actor: &authctx.Actor{
				UserID:  1,
				IsStaff: true,
				Role:    "super_admin",
			},
			wantStatus: http.StatusOK,
		},
		{
			name:   "Super admin GET /admin/earnings/offers returns 200",
			path:   "/admin/earnings/offers",
			method: "GET",
			actor: &authctx.Actor{
				UserID:  1,
				IsStaff: true,
				Role:    "super_admin",
			},
			wantStatus: http.StatusOK,
		},
		{
			name:   "Super admin GET /admin/invoices returns 200",
			path:   "/admin/invoices",
			method: "GET",
			actor: &authctx.Actor{
				UserID:  1,
				IsStaff: true,
				Role:    "super_admin",
			},
			wantStatus: http.StatusOK,
		},
		{
			name:   "Super admin GET /admin/payments returns 200",
			path:   "/admin/payments",
			method: "GET",
			actor: &authctx.Actor{
				UserID:  1,
				IsStaff: true,
				Role:    "super_admin",
			},
			wantStatus: http.StatusOK,
		},
		{
			name:   "Super admin GET /admin/wallets returns 200",
			path:   "/admin/wallets",
			method: "GET",
			actor: &authctx.Actor{
				UserID:  1,
				IsStaff: true,
				Role:    "super_admin",
			},
			wantStatus: http.StatusOK,
		},
		{
			name:   "Super admin GET /admin/plans-info returns 200",
			path:   "/admin/plans-info",
			method: "GET",
			actor: &authctx.Actor{
				UserID:  1,
				IsStaff: true,
				Role:    "super_admin",
			},
			wantStatus: http.StatusOK,
		},
		{
			name:   "Super admin GET /admin/plan-types returns 200",
			path:   "/admin/plan-types",
			method: "GET",
			actor: &authctx.Actor{
				UserID:  1,
				IsStaff: true,
				Role:    "super_admin",
			},
			wantStatus: http.StatusOK,
		},
		{
			name:   "Super admin GET /admin/plan-features returns 200",
			path:   "/admin/plan-features",
			method: "GET",
			actor: &authctx.Actor{
				UserID:  1,
				IsStaff: true,
				Role:    "super_admin",
			},
			wantStatus: http.StatusOK,
		},
		{
			name:   "Super admin GET /admin/plans/subscriptions returns 200",
			path:   "/admin/plans/subscriptions",
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
