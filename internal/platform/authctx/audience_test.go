package authctx_test

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
)

func TestRequirePagePermission(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	gate := authctx.RequirePagePermission("catalog.product.view", logger)
	developerGate := authctx.RequirePagePermission("platform.developer.sql", logger)

	okHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	tests := []struct {
		name       string
		gate       func(http.Handler) http.Handler
		actor      *authctx.Actor
		wantStatus int
	}{
		{
			name:       "Anonymous request redirects to login",
			gate:       gate,
			actor:      nil,
			wantStatus: http.StatusSeeOther,
		},
		{
			name: "Non-staff user (customer) gets 404",
			gate: gate,
			actor: &authctx.Actor{
				UserID:  1,
				IsStaff: false,
				Role:    "customer",
			},
			wantStatus: http.StatusNotFound,
		},
		{
			name: "Staff user lacking permission gets 404 (T5a/T5b)",
			gate: developerGate,
			actor: &authctx.Actor{
				UserID:      2,
				IsStaff:     true,
				Role:        "support",
				Permissions: []string{"catalog.product.view"},
			},
			wantStatus: http.StatusNotFound,
		},
		{
			name: "Staff user with required permission gets 200 (T5c)",
			gate: gate,
			actor: &authctx.Actor{
				UserID:      3,
				IsStaff:     true,
				Role:        "support",
				Permissions: []string{"catalog.product.view"},
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "Super admin role bypasses permission check",
			gate: developerGate,
			actor: &authctx.Actor{
				UserID:  4,
				IsStaff: true,
				Role:    "super_admin",
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "Developer role bypasses developer gate",
			gate: developerGate,
			actor: &authctx.Actor{
				UserID:  5,
				IsStaff: true,
				Role:    "developer",
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "Wildcard permission bypasses check",
			gate: developerGate,
			actor: &authctx.Actor{
				UserID:      6,
				IsStaff:     true,
				Role:        "admin",
				Permissions: []string{"*"},
			},
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/admin/test", nil)
			if tt.actor != nil {
				req = req.WithContext(authctx.WithActor(req.Context(), *tt.actor))
			}
			rec := httptest.NewRecorder()
			handler := tt.gate(okHandler)
			handler.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("got status %d, want %d", rec.Code, tt.wantStatus)
			}
		})
	}
}
