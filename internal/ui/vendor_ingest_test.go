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
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/ui"
)

func TestVendorIngestWizardResumability(t *testing.T) {
	ctx := context.Background()
	actor := authctx.Actor{
		UserID:         100,
		OrganizationID: 10,
	}
	ctx = authctx.WithActor(ctx, actor)
	ctx = database.WithTenant(ctx, 10)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := ui.NewUIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, logger)

	r := chi.NewRouter()
	handler.RegisterVendorRoutes(r)

	// Step 1: GET /vendor/ingest without session should render 200 OK
	req1, _ := http.NewRequestWithContext(ctx, "GET", "/vendor/ingest", nil)
	rr1 := httptest.NewRecorder()
	r.ServeHTTP(rr1, req1)
	if rr1.Code != http.StatusOK {
		t.Errorf("expected 200 OK for /vendor/ingest, got %d", rr1.Code)
	}

	// Step 2: GET /vendor/ingest/{sessionID}
	req2, _ := http.NewRequestWithContext(ctx, "GET", "/vendor/ingest/999", nil)
	rr2 := httptest.NewRecorder()
	r.ServeHTTP(rr2, req2)
	// Non-existing session redirects back to /vendor/ingest
	if rr2.Code != http.StatusSeeOther {
		t.Errorf("expected redirect for non-existent session, got %d", rr2.Code)
	}
}
