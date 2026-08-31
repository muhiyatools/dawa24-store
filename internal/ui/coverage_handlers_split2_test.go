package ui_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/workflow"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
)

func TestVendorCoverageRoutes(t *testing.T) {
	vendorActor := &authctx.Actor{
		UserID:         1,
		OrganizationID: 10,
		OrgType:        "vendor",
		OrgStatus:      "approved",
		Role:           "vendor",
		Permissions:    []string{"vendor.*"},
	}

	customerActor := &authctx.Actor{
		UserID:         2,
		OrganizationID: 20,
		OrgType:        "customer",
		OrgStatus:      "approved",
		Role:           "customer",
		Permissions:    []string{"pharmacy.*"},
	}

	wfRepo := newMockCoverageRepo()
	orgRepo := mockOrgCoverageRepo{}

	vendorRouter := newTestCoverageRouter(vendorActor, wfRepo, orgRepo)
	customerRouter := newTestCoverageRouter(customerActor, wfRepo, orgRepo)

	// T4: Customer attempting to reach /vendor/coverage gets 404
	t.Run("Customer GET /vendor/coverage returns 404", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/vendor/coverage", nil)
		rec := httptest.NewRecorder()
		customerRouter.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Errorf("want 404, got %d", rec.Code)
		}
	})

	// T6: Vendor GET /vendor/coverage returns 200 and renders
	t.Run("Vendor GET /vendor/coverage returns 200", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/vendor/coverage", nil)
		rec := httptest.NewRecorder()
		vendorRouter.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("want 200, got %d", rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "Main Warehouse") {
			t.Errorf("expected branch name in body")
		}
	})

	// T6: Vendor POST /vendor/coverage creates coverage and redirects with success
	t.Run("Vendor POST /vendor/coverage success", func(t *testing.T) {
		form := url.Values{
			"branch_id":       {"100"},
			"day_of_week":     {"2"},
			"distance_meters": {"30000"},
			"coverage_from":   {"08:00"},
			"coverage_to":     {"16:00"},
			"latitude":        {"30.05"},
			"longitude":       {"31.25"},
			"is_active":       {"true"},
		}
		req := httptest.NewRequest("POST", "/vendor/coverage", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()
		vendorRouter.ServeHTTP(rec, req)

		if rec.Code != http.StatusSeeOther {
			t.Errorf("want 303 redirect, got %d", rec.Code)
		}
		loc := rec.Header().Get("Location")
		if !strings.Contains(loc, "notice=success") {
			t.Errorf("expected notice=success redirect, got %s", loc)
		}
	})

	// T3: Vendor POST /vendor/coverage with other org's branch is rejected
	t.Run("Vendor POST /vendor/coverage cross-tenant branch rejected", func(t *testing.T) {
		form := url.Values{
			"branch_id":       {"200"}, // belongs to org 99, not 10
			"day_of_week":     {"3"},
			"distance_meters": {"25000"},
		}
		req := httptest.NewRequest("POST", "/vendor/coverage", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()
		vendorRouter.ServeHTTP(rec, req)

		if rec.Code != http.StatusSeeOther {
			t.Errorf("want 303 redirect, got %d", rec.Code)
		}
		loc := rec.Header().Get("Location")
		if !strings.Contains(loc, "notice=error") {
			t.Errorf("expected notice=error redirect, got %s", loc)
		}
	})

	// T6: Vendor POST /vendor/coverage/{id}/toggle toggles active state
	t.Run("Vendor POST /vendor/coverage/1/toggle", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/vendor/coverage/1/toggle", nil)
		rec := httptest.NewRecorder()
		vendorRouter.ServeHTTP(rec, req)

		if rec.Code != http.StatusSeeOther {
			t.Errorf("want 303 redirect, got %d", rec.Code)
		}
		if wfRepo.coverages[1].IsActive != false {
			t.Errorf("expected coverage 1 to be toggled to false")
		}
	})

	// T6: Vendor POST /vendor/coverage/{id} updates coverage
	t.Run("Vendor POST /vendor/coverage/1 update", func(t *testing.T) {
		// Re-create coverage 1 first
		wfRepo.coverages[1] = &workflow.WeeklyCoverage{
			ID:             1,
			OrganizationID: 10,
			BranchID:       100,
			DayOfWeek:      1,
			DistanceMeters: 25000,
			IsActive:       true,
		}

		form := url.Values{
			"branch_id":       {"100"},
			"day_of_week":     {"4"},
			"distance_meters": {"50000"},
			"coverage_from":   {"10:00"},
			"coverage_to":     {"18:00"},
			"address":         {"Updated Warehouse Route"},
			"is_active":       {"true"},
		}
		req := httptest.NewRequest("POST", "/vendor/coverage/1", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()
		vendorRouter.ServeHTTP(rec, req)

		if rec.Code != http.StatusSeeOther {
			t.Errorf("want 303 redirect, got %d", rec.Code)
		}
		if wfRepo.coverages[1].DayOfWeek != 4 {
			t.Errorf("want DayOfWeek 4, got %d", wfRepo.coverages[1].DayOfWeek)
		}
		if wfRepo.coverages[1].DistanceMeters != 50000 {
			t.Errorf("want DistanceMeters 50000, got %d", wfRepo.coverages[1].DistanceMeters)
		}
	})

	// T6: Vendor POST /vendor/coverage with apply_to_all_days
	t.Run("Vendor POST /vendor/coverage with apply_to_all_days creates 7 days", func(t *testing.T) {
		initialCount := len(wfRepo.coverages)
		form := url.Values{
			"branch_id":         {"100"},
			"apply_to_all_days": {"true"},
			"distance_meters":   {"20000"},
			"coverage_from":     {"09:00"},
			"coverage_to":       {"17:00"},
			"is_active":         {"true"},
		}
		req := httptest.NewRequest("POST", "/vendor/coverage", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()
		vendorRouter.ServeHTTP(rec, req)

		if rec.Code != http.StatusSeeOther {
			t.Errorf("want 303 redirect, got %d", rec.Code)
		}
		if len(wfRepo.coverages) != initialCount+7 {
			t.Errorf("want %d coverages, got %d", initialCount+7, len(wfRepo.coverages))
		}
	})

	// T6: Vendor POST /vendor/coverage/{id}/delete deletes coverage
	t.Run("Vendor POST /vendor/coverage/1/delete", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/vendor/coverage/1/delete", nil)
		rec := httptest.NewRecorder()
		vendorRouter.ServeHTTP(rec, req)

		if rec.Code != http.StatusSeeOther {
			t.Errorf("want 303 redirect, got %d", rec.Code)
		}
		if _, exists := wfRepo.coverages[1]; exists {
			t.Errorf("expected coverage 1 to be deleted")
		}
	})

	// T7: Vendor POST /vendor/delivery-bands/create saves band with meters
	t.Run("Vendor POST /vendor/delivery-bands/create with meters", func(t *testing.T) {
		form := url.Values{
			"from_meters":  {"0"},
			"to_meters":    {"5000"},
			"delivery_fee": {"35.5"},
		}
		req := httptest.NewRequest("POST", "/vendor/delivery-bands/create", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()
		vendorRouter.ServeHTTP(rec, req)

		if rec.Code != http.StatusSeeOther {
			t.Errorf("want 303 redirect, got %d", rec.Code)
		}
	})

	// T8: Vendor POST /vendor/delivery-bands/{id}/delete removes band
	t.Run("Vendor POST /vendor/delivery-bands/1/delete", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/vendor/delivery-bands/1/delete", nil)
		rec := httptest.NewRecorder()
		vendorRouter.ServeHTTP(rec, req)

		if rec.Code != http.StatusSeeOther {
			t.Errorf("want 303 redirect, got %d", rec.Code)
		}
	})
}
