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

	"github.com/muhiya/dawa24-store/internal/modules/billing"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/rbac"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/ui"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

func TestInvoicePrintAndVendorInvoicesPages(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := ui.NewUIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, logger)

	r := chi.NewRouter()
	handler.RegisterPreApprovalRoutes(r)
	handler.RegisterApprovedSharedRoutes(r)
	handler.RegisterCustomerSharedRoutes(r)
	handler.RegisterVendorSharedRoutes(r)
	handler.RegisterCustomerRoutes(r)
	handler.RegisterVendorRoutes(r)
	handler.RegisterAdminRoutes(r)

	vendorActor := &authctx.Actor{
		UserID:         10,
		OrganizationID: 2,
		OrgType:        "vendor",
		Role:           "vendor_admin",
		Scope:          rbac.ScopeVendor,
		Permissions:    []string{"vendor.payment.view"},
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
		if !strings.Contains(body, "فواتير وسندات التوريد") {
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
			UserID:      1,
			IsStaff:     true,
			Role:        "super_admin",
			Permissions: []string{"*"},
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

	t.Run("Printable invoice renders exact disclaimer and integer discount percentage", func(t *testing.T) {
		data := billing.PrintableInvoiceData{
			InvoiceNumber: "INV-2026-00001",
			Vendor: billing.PrintableOrgInfo{
				DisplayName: "شركة فارما للتوزيع",
				TaxNumber:   "100-200-300",
			},
			Customer: billing.PrintableOrgInfo{
				DisplayName: "صيدلية النور",
			},
			Lines: []*billing.PrintableInvoiceLine{
				{
					Index:           1,
					ItemName:        "Panadol Extra 500mg",
					Quantity:        10,
					UnitPrice:       money.FromMinor(10000), // 100 EGP
					DiscountPercent: 15.4,                   // should render as 15% without decimals
					NetUnitPrice:    money.FromMinor(8500),
					TotalPrice:      money.FromMinor(85000),
				},
			},
		}

		var sb strings.Builder
		err := pages.InvoicePrintablePage(data, "ar", "rtl").Render(context.Background(), &sb)
		if err != nil {
			t.Fatalf("failed to render invoice printable page: %v", err)
		}
		rendered := sb.String()

		expectedDisclaimer := "Dawa24 منصة وسيطة ولا تتحمل أي مسؤولية عن البيع أو التسليم أو الاستلام أو التحصيل أو السداد بين العميل والمورد، وتظل المسؤولية كاملة على أطراف المعاملة، كلٌ حسب التزامه القانوني والتعاقدي."
		if !strings.Contains(rendered, expectedDisclaimer) {
			t.Errorf("expected rendered invoice to contain required legal disclaimer")
		}

		if !strings.Contains(rendered, "15%") {
			t.Errorf("expected rendered invoice to format 15.4%% discount as 15%% (integer)")
		}

		if !strings.Contains(rendered, "دواء") || !strings.Contains(rendered, "24") {
			t.Errorf("expected rendered invoice to contain Dawa 24 branding")
		}
	})
}
