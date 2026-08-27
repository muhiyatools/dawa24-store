package ui_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/ui"
)

func TestInvoicePrintAndVendorInvoicesPages(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := ui.NewUIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, logger)

	r := chi.NewRouter()
	handler.RegisterSharedRoutes(r)
	handler.RegisterCustomerRoutes(r)
	handler.RegisterVendorRoutes(r)
	handler.RegisterAdminRoutes(r)

	vendorActor := &authctx.Actor{
		UserID:         10,
		OrganizationID: 2,
		OrgType:        "vendor",
		Role:           "vendor_admin",
	}

	customerActor := &authctx.Actor{
		UserID:         20,
		OrganizationID: 3,
		OrgType:        "customer",
		Role:           "customer_owner",
	}

	t.Run("Customer GET /invoices redirects to /orders", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/invoices", nil)
		ctx := authctx.WithActor(context.Background(), *customerActor)
		req = req.WithContext(ctx)
		rec := httptest.NewRecorder()

		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusSeeOther {
			t.Errorf("expected 303 SeeOther redirect for customer on /invoices, got %d", rec.Code)
		}
		loc := rec.Header().Get("Location")
		if !strings.HasPrefix(loc, "/orders") {
			t.Errorf("expected redirect location to start with /orders, got %s", loc)
		}
	})

	t.Run("Vendor GET /invoices returns 200 OK", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/invoices", nil)
		ctx := authctx.WithActor(context.Background(), *vendorActor)
		req = req.WithContext(ctx)
		rec := httptest.NewRecorder()

		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200 OK for vendor on /invoices, got %d", rec.Code)
		}
		body := rec.Body.String()
		if !strings.Contains(body, "الفواتير الضريبية والإلكترونية") {
			t.Errorf("expected page to contain invoice title, got: %s", body)
		}
	})

	t.Run("Vendor GET /vendor/invoices returns 200 OK", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/vendor/invoices", nil)
		ctx := authctx.WithActor(context.Background(), *vendorActor)
		req = req.WithContext(ctx)
		rec := httptest.NewRecorder()

		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200 OK on /vendor/invoices, got %d", rec.Code)
		}
	})

	t.Run("Admin GET /admin/finance?tab=invoices returns 200 OK", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/admin/finance?tab=invoices", nil)
		ctx := authctx.WithActor(context.Background(), authctx.Actor{
			UserID:  1,
			IsStaff: true,
			Role:    "super_admin",
		})
		req = req.WithContext(ctx)
		rec := httptest.NewRecorder()

		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200 OK on /admin/finance?tab=invoices, got %d", rec.Code)
		}
		body := rec.Body.String()
		if !strings.Contains(body, "invoices") {
			t.Errorf("expected invoices tab to be rendered")
		}
	})
}
