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

func TestAdminDeletesListsAndTrashRoutes(t *testing.T) {
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
			name:       "Anonymous GET /admin/deletes-lists redirects to login",
			path:       "/admin/deletes-lists",
			method:     "GET",
			actor:      nil,
			wantStatus: http.StatusSeeOther,
		},
		{
			name:   "Super admin GET /admin/deletes-lists returns 200",
			path:   "/admin/deletes-lists",
			method: "GET",
			actor: &authctx.Actor{
				UserID:  1,
				IsStaff: true,
				Role:    "super_admin",
			},
			wantStatus: http.StatusOK,
		},
		{
			name:   "Super admin GET /admin/deletes-lists/products returns 200",
			path:   "/admin/deletes-lists/products",
			method: "GET",
			actor: &authctx.Actor{
				UserID:  1,
				IsStaff: true,
				Role:    "super_admin",
			},
			wantStatus: http.StatusOK,
		},
		{
			name:   "Super admin GET /admin/trash-list returns 200",
			path:   "/admin/trash-list",
			method: "GET",
			actor: &authctx.Actor{
				UserID:  1,
				IsStaff: true,
				Role:    "super_admin",
			},
			wantStatus: http.StatusOK,
		},
		{
			name:   "Super admin GET /admin/trash-list/products returns 200",
			path:   "/admin/trash-list/products",
			method: "GET",
			actor: &authctx.Actor{
				UserID:  1,
				IsStaff: true,
				Role:    "super_admin",
			},
			wantStatus: http.StatusOK,
		},
		{
			name:   "Super admin POST /admin/trash-list/products/12/restore redirects",
			path:   "/admin/trash-list/products/12/restore",
			method: "POST",
			actor: &authctx.Actor{
				UserID:  1,
				IsStaff: true,
				Role:    "super_admin",
			},
			wantStatus: http.StatusSeeOther,
		},
		{
			name:   "Super admin POST /admin/trash-list/products/12/purge redirects",
			path:   "/admin/trash-list/products/12/purge",
			method: "POST",
			actor: &authctx.Actor{
				UserID:  1,
				IsStaff: true,
				Role:    "super_admin",
			},
			wantStatus: http.StatusSeeOther,
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
