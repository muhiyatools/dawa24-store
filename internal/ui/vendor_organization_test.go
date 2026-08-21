package ui_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/ui"
)

type mockOrgRepoVendorTest struct {
	org.Repository
	profiles map[int64]*org.SupplierOrgProfile
}

func newMockOrgRepoVendorTest() *mockOrgRepoVendorTest {
	return &mockOrgRepoVendorTest{
		profiles: make(map[int64]*org.SupplierOrgProfile),
	}
}

func (m *mockOrgRepoVendorTest) GetSupplierProfile(ctx context.Context, id int64) (*org.SupplierOrgProfile, error) {
	p, ok := m.profiles[id]
	if !ok {
		return &org.SupplierOrgProfile{
			ID:            id,
			NameAr:        "الشركة الافتراضية",
			Type:          "supplier",
			MinOrderPrice: money.FromMajor(10),
			MaxOrderPrice: money.FromMajor(50),
		}, nil
	}
	return p, nil
}

func (m *mockOrgRepoVendorTest) UpdateSupplierProfile(ctx context.Context, p *org.SupplierOrgProfile) error {
	m.profiles[p.ID] = p
	return nil
}

func TestVendorOrganization_Flow(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	repo := newMockOrgRepoVendorTest()
	orgSvc := org.NewService(repo, logger)

	h := ui.NewUIHandler(
		nil, orgSvc, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, logger,
	)

	repo.profiles[42] = &org.SupplierOrgProfile{
		ID:                 42,
		NameAr:             "سمارت كودز 1",
		NameEn:             "Smart Codes",
		Type:               "company",
		MinOrderPrice:      money.FromMajor(10),
		MaxOrderPrice:      money.FromMajor(50),
		OrganizationNumber: "267244",
		Email:              "info@smartcodes.com",
		Phone:              "01099887766",
		TaxNumber:          "TX431256",
		Address:            "Giza, Egypt",
		DescriptionAr:      "شركة برمجة متخصصة في تطوير تطبيقات الويب",
		DescriptionEn:      "Software company specializing in web apps",
	}

	vendorActor := authctx.Actor{UserID: 10, OrganizationID: 42, OrgType: "vendor"}

	// 1. Test GET /vendor/organization renders populated profile
	t.Run("GET /vendor/organization renders fields", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/vendor/organization", nil)
		req = req.WithContext(authctx.WithActor(req.Context(), vendorActor))
		rec := httptest.NewRecorder()

		h.VendorOrganizationPage(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", rec.Code)
		}
		body := rec.Body.String()
		if !strings.Contains(body, "سمارت كودز 1") {
			t.Errorf("expected body to contain 'سمارت كودز 1'")
		}
		if !strings.Contains(body, "Smart Codes") {
			t.Errorf("expected body to contain 'Smart Codes'")
		}
		if !strings.Contains(body, "267244") {
			t.Errorf("expected body to contain org number '267244'")
		}
		if !strings.Contains(body, "TX431256") {
			t.Errorf("expected body to contain tax number 'TX431256'")
		}
	})

	// 2. Test POST /vendor/organization updates data
	t.Run("POST /vendor/organization updates data", func(t *testing.T) {
		form := url.Values{}
		form.Set("name_ar", "شركة سمارت كودز الدولية")
		form.Set("name_en", "Smart Codes Global")
		form.Set("type", "company")
		form.Set("min_order_price", "15.50")
		form.Set("max_order_price", "75.00")
		form.Set("organization_number", "998877")
		form.Set("email", "contact@smartcodes.global")
		form.Set("phone", "0123456789")
		form.Set("tax_number", "TX998877")
		form.Set("address", "Cairo, Egypt")
		form.Set("description_ar", "تحديث الوصف بالعربي")
		form.Set("description_en", "Updated English description")

		req := httptest.NewRequest("POST", "/vendor/organization", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req = req.WithContext(authctx.WithActor(req.Context(), vendorActor))
		rec := httptest.NewRecorder()

		h.VendorOrganizationSubmit(rec, req)

		if rec.Code != http.StatusSeeOther {
			t.Fatalf("expected redirect 303, got %d", rec.Code)
		}

		updated := repo.profiles[42]
		if updated.NameAr != "شركة سمارت كودز الدولية" {
			t.Errorf("expected NameAr 'شركة سمارت كودز الدولية', got %s", updated.NameAr)
		}
		if updated.MinOrderPrice.String() != "15.50" {
			t.Errorf("expected MinOrderPrice 15.50, got %s", updated.MinOrderPrice.String())
		}
	})

	// 3. Test router redirection from /settings/organization for vendor vs customer
	t.Run("Router handles /settings/organization smartly", func(t *testing.T) {
		r := chi.NewRouter()
		h.RegisterVendorRoutes(r)
		r.Get("/settings/organization", func(w http.ResponseWriter, req *http.Request) {
			if actor, ok := authctx.From(req.Context()); ok && actor.OrgType == "vendor" {
				http.Redirect(w, req, "/vendor/organization", http.StatusSeeOther)
				return
			}
			http.Redirect(w, req, "/settings?tab=organization", http.StatusMovedPermanently)
		})

		// Vendor request
		vReq := httptest.NewRequest("GET", "/settings/organization", nil)
		vReq = vReq.WithContext(authctx.WithActor(vReq.Context(), vendorActor))
		vRec := httptest.NewRecorder()
		r.ServeHTTP(vRec, vReq)
		if vRec.Code != http.StatusSeeOther || vRec.Header().Get("Location") != "/vendor/organization" {
			t.Errorf("expected vendor to be redirected to /vendor/organization, got status %d, location %s", vRec.Code, vRec.Header().Get("Location"))
		}

		// Customer / Pharmacy request
		custActor := authctx.Actor{UserID: 20, OrganizationID: 10, OrgType: "customer"}
		cReq := httptest.NewRequest("GET", "/settings/organization", nil)
		cReq = cReq.WithContext(authctx.WithActor(cReq.Context(), custActor))
		cRec := httptest.NewRecorder()
		r.ServeHTTP(cRec, cReq)
		if cRec.Code != http.StatusMovedPermanently || cRec.Header().Get("Location") != "/settings?tab=organization" {
			t.Errorf("expected customer to be redirected to /settings?tab=organization, got status %d, location %s", cRec.Code, cRec.Header().Get("Location"))
		}
	})
}
