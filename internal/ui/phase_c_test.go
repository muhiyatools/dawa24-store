package ui_test

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/ui"
)

// TestPhaseC_VendorContentAndPolicies verifies Task C.5: Vendor policies and social media forms.
func TestPhaseC_VendorContentAndPolicies(t *testing.T) {
	handler := ui.NewUIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	r := newRealUIHandlerRouter(handler)
	vendorActor := authctx.Actor{UserID: 10, OrganizationID: 2, Role: "vendor", Permissions: []string{"org.admin"}}

	// GET /vendor/policies renders
	rec := doGET(t, r, "/vendor/policies", vendorActor)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "سياسات التوريد والدفع والاسترجاع")

	// GET /vendor/social-media renders
	rec = doGET(t, r, "/vendor/social-media", vendorActor)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "قنوات التواصل الاجتماعي")

	// POST /vendor/policies submits and obeys Law 3 (returns error notice when service is unavailable)
	rec = doPOST(t, r, "/vendor/policies", url.Values{
		"return_policy":   []string{"سياسة الاسترجاع"},
		"shipping_policy": []string{"سياسة الشحن والتوصيل"},
	}, vendorActor)
	assert.Equal(t, http.StatusSeeOther, rec.Code)
	assert.Contains(t, rec.Header().Get("Location"), "/vendor/policies")
	assert.Contains(t, rec.Header().Get("Location"), "notice=error")

	// POST /vendor/social-media submits and obeys Law 3 (returns error notice when service is unavailable)
	rec = doPOST(t, r, "/vendor/social-media", url.Values{
		"facebook":  []string{"https://facebook.com/vendor"},
		"instagram": []string{"https://instagram.com/vendor"},
	}, vendorActor)
	assert.Equal(t, http.StatusSeeOther, rec.Code)
	assert.Contains(t, rec.Header().Get("Location"), "/vendor/social-media")
	assert.Contains(t, rec.Header().Get("Location"), "notice=error")
}

// TestPhaseC_VendorSavingProducts verifies Task C.11: Vendor saving products screens.
func TestPhaseC_VendorSavingProducts(t *testing.T) {
	handler := ui.NewUIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	r := newRealUIHandlerRouter(handler)
	vendorActor := authctx.Actor{UserID: 10, OrganizationID: 2, Role: "vendor"}

	// GET /vendor/saving-products renders
	rec := doGET(t, r, "/vendor/saving-products", vendorActor)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "منتجات التوفير (Saving Products)")

	// GET /vendor/saving-products/import renders
	rec = doGET(t, r, "/vendor/saving-products/import", vendorActor)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "استيراد ملف منتجات التوفير")
}

// TestPhaseC_VendorInstitutionalWork verifies Task C.12: Institutional work and pharmacy coverage screens.
func TestPhaseC_VendorInstitutionalWork(t *testing.T) {
	handler := ui.NewUIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	r := newRealUIHandlerRouter(handler)
	vendorActor := authctx.Actor{UserID: 10, OrganizationID: 2, Role: "vendor"}

	// GET /vendor/institutional-work renders
	rec := doGET(t, r, "/vendor/institutional-work", vendorActor)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "الأعمال والاتفاقيات المؤسسية للمنشأة")

	// GET /vendor/pharmacy-coverage renders
	rec = doGET(t, r, "/vendor/pharmacy-coverage", vendorActor)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "الصيدليات المشمولة في نطاق التغطية الأسبوعية")
}

// TestPhaseC_AdminReferenceData verifies Task C.6: Admin countries, social media, highlight sections, API integrations.
func TestPhaseC_AdminReferenceData(t *testing.T) {
	handler := ui.NewUIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	r := newRealUIHandlerRouter(handler)
	adminActor := authctx.Actor{UserID: 1, IsStaff: true, Role: "super_admin"}

	// Countries
	rec := doGET(t, r, "/admin/countries", adminActor)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "دليل الدول والمناطق")

	// Highlight Sections
	rec = doGET(t, r, "/admin/highlight-sections", adminActor)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "الأقسام المميزة والعروض البارزة")

	// API Integrations (Redirects to Developers)
	rec = doGET(t, r, "/admin/api-integrations", adminActor)
	assert.Equal(t, http.StatusMovedPermanently, rec.Code)
}

// TestPhaseC_AdminFinanceScreens verifies Task C.3: Admin Invoices, Payments, Wallets, Plans.
func TestPhaseC_AdminFinanceScreens(t *testing.T) {
	handler := ui.NewUIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	r := newRealUIHandlerRouter(handler)
	adminActor := authctx.Actor{UserID: 1, IsStaff: true, Role: "super_admin"}

	// Unified Finance Hub
	rec := doGET(t, r, "/admin/finance", adminActor)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "المركز المالي والمحافظ والتحصيل")

	// Unified Plans Hub
	rec = doGET(t, r, "/admin/plans", adminActor)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "باقات واشتراكات المنظومة")

	// Legacy routes redirect to unified hubs
	rec = doGET(t, r, "/admin/invoices", adminActor)
	assert.Equal(t, http.StatusMovedPermanently, rec.Code)

	rec = doGET(t, r, "/admin/payments", adminActor)
	assert.Equal(t, http.StatusMovedPermanently, rec.Code)

	rec = doGET(t, r, "/admin/wallets", adminActor)
	assert.Equal(t, http.StatusMovedPermanently, rec.Code)

	rec = doGET(t, r, "/admin/plans-info", adminActor)
	assert.Equal(t, http.StatusMovedPermanently, rec.Code)
}
