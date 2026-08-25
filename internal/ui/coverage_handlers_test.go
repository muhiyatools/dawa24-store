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
	"github.com/muhiya/dawa24-store/internal/modules/workflow"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/ui"
)

type mockCoverageRepo struct {
	coverages map[int64]*workflow.WeeklyCoverage
	nextID    int64
}

func newMockCoverageRepo() *mockCoverageRepo {
	return &mockCoverageRepo{
		coverages: map[int64]*workflow.WeeklyCoverage{
			1: {
				ID:             1,
				OrganizationID: 10,
				BranchID:       100,
				DayOfWeek:      1,
				DistanceMeters: 25000,
				IsActive:       true,
			},
		},
		nextID: 2,
	}
}

func (m *mockCoverageRepo) CreatePriorityRequest(_ context.Context, _ *workflow.PurchasePriorityRequest) error {
	return nil
}
func (m *mockCoverageRepo) GetPriorityRequestByID(_ context.Context, _ int64) (*workflow.PurchasePriorityRequest, error) {
	return nil, nil
}
func (m *mockCoverageRepo) SaveWeeklyCoverage(_ context.Context, c *workflow.WeeklyCoverage) error {
	c.ID = m.nextID
	m.nextID++
	m.coverages[c.ID] = c
	return nil
}
func (m *mockCoverageRepo) UpdateWeeklyCoverage(_ context.Context, c *workflow.WeeklyCoverage) error {
	if _, ok := m.coverages[c.ID]; !ok {
		return apperr.NotFound("weekly_coverage")
	}
	m.coverages[c.ID] = c
	return nil
}
func (m *mockCoverageRepo) DeleteWeeklyCoverage(_ context.Context, id int64) error {
	if _, ok := m.coverages[id]; !ok {
		return apperr.NotFound("weekly_coverage")
	}
	delete(m.coverages, id)
	return nil
}
func (m *mockCoverageRepo) ToggleWeeklyCoverage(_ context.Context, id int64, isActive bool) error {
	c, ok := m.coverages[id]
	if !ok {
		return apperr.NotFound("weekly_coverage")
	}
	c.IsActive = isActive
	return nil
}
func (m *mockCoverageRepo) GetWeeklyCoverageByID(_ context.Context, id int64) (*workflow.WeeklyCoverage, error) {
	c, ok := m.coverages[id]
	if !ok {
		return nil, apperr.NotFound("weekly_coverage")
	}
	return c, nil
}
func (m *mockCoverageRepo) ListWeeklyCoverage(_ context.Context, branchID int64) ([]*workflow.WeeklyCoverage, error) {
	var list []*workflow.WeeklyCoverage
	for _, c := range m.coverages {
		if c.BranchID == branchID {
			list = append(list, c)
		}
	}
	return list, nil
}
func (m *mockCoverageRepo) ListCoverageForOrganization(_ context.Context, orgID int64) ([]*workflow.CoverageView, error) {
	var list []*workflow.CoverageView
	for _, c := range m.coverages {
		if c.OrganizationID == orgID {
			list = append(list, &workflow.CoverageView{
				WeeklyCoverage: *c,
				BranchName:     "Main Warehouse",
				CityName:       "Cairo",
			})
		}
	}
	return list, nil
}
func (m *mockCoverageRepo) CreateIssue(_ context.Context, _ *workflow.ReportIssue) error {
	return nil
}
func (m *mockCoverageRepo) GetIssueByID(_ context.Context, _ int64) (*workflow.ReportIssue, error) {
	return nil, nil
}
func (m *mockCoverageRepo) ListIssues(_ context.Context, _, _ int) ([]*workflow.ReportIssue, error) {
	return nil, nil
}
func (m *mockCoverageRepo) CreateRequest(_ context.Context, _ *workflow.Request) error { return nil }
func (m *mockCoverageRepo) GetRequestByID(_ context.Context, _ int64) (*workflow.Request, error) {
	return nil, nil
}
func (m *mockCoverageRepo) ListRequestsByOrg(_ context.Context, _ int64, _ string, _, _ int) ([]*workflow.Request, error) {
	return nil, nil
}
func (m *mockCoverageRepo) UpdateRequestStatus(_ context.Context, _ int64, _ workflow.RequestStatus) error {
	return nil
}
func (m *mockCoverageRepo) ListPriorityRequestsByUser(_ context.Context, _ int64, _, _ int) ([]*workflow.PurchasePriorityRequest, error) {
	return nil, nil
}
func (m *mockCoverageRepo) UpdatePriorityRequestStatus(_ context.Context, _ int64, _ string, _ string, _ *int64, _ map[string]any) error {
	return nil
}
func (m *mockCoverageRepo) GetCandidateProducts(_ context.Context, _ int64, _ []int64, _ []int64, _ *money.Amount, _ int) ([]workflow.CandidateProduct, error) {
	return nil, nil
}

type mockOrgCoverageRepo struct {
	org.Repository
}

func (mockOrgCoverageRepo) GetBranchByID(_ context.Context, id int64) (*org.Branch, error) {
	if id == 100 {
		return &org.Branch{ID: 100, OrganizationID: 10, Name: i18n.New("Main Warehouse", "Main Warehouse")}, nil
	}
	if id == 200 {
		return &org.Branch{ID: 200, OrganizationID: 99, Name: i18n.New("Other Org Warehouse", "Other Org Warehouse")}, nil
	}
	return nil, apperr.NotFound("branch")
}
func (mockOrgCoverageRepo) ListBranchesByOrg(_ context.Context, orgID int64) ([]*org.Branch, error) {
	if orgID == 10 {
		return []*org.Branch{{ID: 100, OrganizationID: 10, Name: i18n.New("Main Warehouse", "Main Warehouse")}}, nil
	}
	return nil, nil
}
func (mockOrgCoverageRepo) GetDeliveryBands(_ context.Context, orgID int64) ([]*org.DeliveryBand, error) {
	return []*org.DeliveryBand{
		{ID: 1, OrganizationID: orgID, FromMeters: 0, ToMeters: 10000, Fee: money.FromMinor(3000)},
	}, nil
}

func newTestCoverageRouter(actor *authctx.Actor, wfRepo workflow.Repository, orgRepo org.Repository) http.Handler {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	wfSvc := workflow.NewService(wfRepo, logger)
	orgSvc := org.NewService(orgRepo, logger)

	handler := ui.NewUIHandler(
		nil, orgSvc, nil, nil, nil, nil, nil, nil, nil, nil, nil, wfSvc, nil, nil, logger,
	)

	stubAuth := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if actor != nil {
				ctx := authctx.WithActor(req.Context(), *actor)
				if actor.OrganizationID > 0 {
					ctx = database.WithTenant(ctx, actor.OrganizationID)
				}
				next.ServeHTTP(w, req.WithContext(ctx))
				return
			}
			next.ServeHTTP(w, req)
		})
	}

	r := chi.NewRouter()
	r.Group(func(uiRouter chi.Router) {
		uiRouter.Use(stubAuth)
		uiRouter.Use(authctx.RequireCustomer(logger))
		uiRouter.Use(authctx.RequireApproved(logger))
		handler.RegisterCustomerRoutes(uiRouter)
	})
	r.Group(func(uiRouter chi.Router) {
		uiRouter.Use(stubAuth)
		uiRouter.Use(authctx.RequireVendor(logger))
		uiRouter.Use(authctx.RequireApproved(logger))
		handler.RegisterVendorRoutes(uiRouter)
	})
	return r
}

func TestVendorCoverageRoutes(t *testing.T) {
	vendorActor := &authctx.Actor{
		UserID:         1,
		OrganizationID: 10,
		OrgType:        "vendor",
		OrgStatus:      "approved",
		Role:           "vendor",
		Permissions:    []string{"org.manage"},
	}

	customerActor := &authctx.Actor{
		UserID:         2,
		OrganizationID: 20,
		OrgType:        "customer",
		OrgStatus:      "approved",
		Role:           "customer",
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
}
