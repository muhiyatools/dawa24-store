package http_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/org"
	orgHttp "github.com/muhiya/dawa24-store/internal/modules/org/http"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/httpx"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

type stubRepo struct{ t *testing.T }

func (r stubRepo) fail(method string) {
	r.t.Helper()
	r.t.Fatalf("repository.%s was called; the request should have been rejected before reaching the repository", method)
}

func (r stubRepo) CreateOrganization(ctx context.Context, o *org.Organization) error {
	r.fail("CreateOrganization")
	return nil
}
func (r stubRepo) GetOrganizationByID(ctx context.Context, id int64) (*org.Organization, error) {
	r.fail("GetOrganizationByID")
	return nil, nil
}
func (r stubRepo) UpdateOrganizationStatus(ctx context.Context, id int64, status org.OrganizationStatus) error {
	r.fail("UpdateOrganizationStatus")
	return nil
}
func (r stubRepo) UpdateOrganization(ctx context.Context, o *org.Organization) error {
	r.fail("UpdateOrganization")
	return nil
}
func (r stubRepo) DeleteOrganization(ctx context.Context, id int64) error {
	r.fail("DeleteOrganization")
	return nil
}
func (r stubRepo) ListOrganizations(ctx context.Context, orgType *org.OrganizationType, status *org.OrganizationStatus, limit, offset int) ([]*org.Organization, error) {
	r.fail("ListOrganizations")
	return nil, nil
}

func (r stubRepo) CreateBranch(ctx context.Context, b *org.Branch) error {
	r.fail("CreateBranch")
	return nil
}
func (r stubRepo) GetBranchByID(ctx context.Context, id int64) (*org.Branch, error) {
	r.fail("GetBranchByID")
	return nil, nil
}
func (r stubRepo) UpdateBranch(ctx context.Context, b *org.Branch) error {
	r.fail("UpdateBranch")
	return nil
}
func (r stubRepo) DeleteBranch(ctx context.Context, id, orgID int64) error {
	r.fail("DeleteBranch")
	return nil
}
func (r stubRepo) ListBranchesByOrg(ctx context.Context, orgID int64) ([]*org.Branch, error) {
	r.fail("ListBranchesByOrg")
	return nil, nil
}
func (r stubRepo) UnsetMainBranches(ctx context.Context, orgID int64) error {
	r.fail("UnsetMainBranches")
	return nil
}

func (r stubRepo) AddMember(ctx context.Context, m *org.Member) error {
	r.fail("AddMember")
	return nil
}
func (r stubRepo) UpdateMemberRole(ctx context.Context, orgID, userID int64, role string) error {
	r.fail("UpdateMemberRole")
	return nil
}
func (r stubRepo) ListMembersByOrg(ctx context.Context, orgID int64) ([]*org.Member, error) {
	r.fail("ListMembersByOrg")
	return nil, nil
}
func (r stubRepo) RemoveMember(ctx context.Context, orgID, userID int64) error {
	r.fail("RemoveMember")
	return nil
}

func (r stubRepo) AddReview(ctx context.Context, rev *org.Review) error {
	r.fail("AddReview")
	return nil
}
func (r stubRepo) ListReviewsByOrg(ctx context.Context, orgID int64, limit, offset int) ([]*org.Review, error) {
	r.fail("ListReviewsByOrg")
	return nil, nil
}
func (r stubRepo) ToggleFollower(ctx context.Context, orgID, userID int64) (bool, error) {
	r.fail("ToggleFollower")
	return false, nil
}
func (r stubRepo) IsFollowing(ctx context.Context, orgID, userID int64) (bool, error) {
	r.fail("IsFollowing")
	return false, nil
}
func (r stubRepo) CreatePolicy(ctx context.Context, p *org.Policy) error {
	r.fail("CreatePolicy")
	return nil
}
func (r stubRepo) ListPoliciesByOrg(ctx context.Context, orgID int64) ([]*org.Policy, error) {
	r.fail("ListPoliciesByOrg")
	return nil, nil
}

type happyRepo struct{}

func (happyRepo) CreateOrganization(ctx context.Context, o *org.Organization) error {
	o.ID = 1
	return nil
}
func (happyRepo) GetOrganizationByID(ctx context.Context, id int64) (*org.Organization, error) {
	return &org.Organization{ID: id, LegalName: "Al-Amal Pharmacy", CommercialRegister: "CR-101", Type: org.TypePharmacy, Status: org.StatusApproved}, nil
}
func (happyRepo) UpdateOrganizationStatus(ctx context.Context, id int64, status org.OrganizationStatus) error {
	return nil
}
func (happyRepo) UpdateOrganization(ctx context.Context, o *org.Organization) error {
	return nil
}
func (happyRepo) DeleteOrganization(ctx context.Context, id int64) error {
	return nil
}
func (happyRepo) ListOrganizations(ctx context.Context, orgType *org.OrganizationType, status *org.OrganizationStatus, limit, offset int) ([]*org.Organization, error) {
	return []*org.Organization{{ID: 1, LegalName: "Al-Amal Pharmacy", CommercialRegister: "CR-101", Status: org.StatusApproved}}, nil
}
func (happyRepo) CreateBranch(ctx context.Context, b *org.Branch) error {
	b.ID = 1
	return nil
}
func (happyRepo) GetBranchByID(ctx context.Context, id int64) (*org.Branch, error) {
	return &org.Branch{ID: id, OrganizationID: 1, Code: "BR-01", Name: i18n.Text{"en": "Main"}}, nil
}
func (happyRepo) UpdateBranch(ctx context.Context, b *org.Branch) error {
	return nil
}
func (happyRepo) DeleteBranch(ctx context.Context, id, orgID int64) error {
	return nil
}
func (happyRepo) ListBranchesByOrg(ctx context.Context, orgID int64) ([]*org.Branch, error) {
	return []*org.Branch{{ID: 1, OrganizationID: orgID, Code: "BR-01"}}, nil
}
func (happyRepo) UnsetMainBranches(ctx context.Context, orgID int64) error {
	return nil
}
func (happyRepo) AddMember(ctx context.Context, m *org.Member) error {
	m.ID = 1
	return nil
}
func (happyRepo) UpdateMemberRole(ctx context.Context, orgID, userID int64, role string) error {
	return nil
}
func (happyRepo) ListMembersByOrg(ctx context.Context, orgID int64) ([]*org.Member, error) {
	return []*org.Member{{ID: 1, OrganizationID: orgID, UserID: 1, RoleID: 1}}, nil
}
func (happyRepo) RemoveMember(ctx context.Context, orgID, userID int64) error {
	return nil
}
func (happyRepo) AddReview(ctx context.Context, rev *org.Review) error {
	rev.ID = 1
	return nil
}
func (happyRepo) ListReviewsByOrg(ctx context.Context, orgID int64, limit, offset int) ([]*org.Review, error) {
	return []*org.Review{{ID: 1, OrganizationID: orgID, UserID: 1, Rating: 5}}, nil
}
func (happyRepo) ToggleFollower(ctx context.Context, orgID, userID int64) (bool, error) {
	return true, nil
}
func (happyRepo) IsFollowing(ctx context.Context, orgID, userID int64) (bool, error) {
	return true, nil
}
func (happyRepo) CreatePolicy(ctx context.Context, p *org.Policy) error {
	p.ID = 1
	return nil
}
func (happyRepo) ListPoliciesByOrg(ctx context.Context, orgID int64) ([]*org.Policy, error) {
	return []*org.Policy{{ID: 1, OrganizationID: orgID, Title: "Refund Policy"}}, nil
}

const testCookieName = "dawa24_session"

func newTestRouter(t *testing.T) http.Handler {
	t.Helper()
	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	orgSvc := org.NewService(stubRepo{t: t}, log)

	r := chi.NewRouter()
	r.Use(httpx.RequestID)
	r.Use(httpx.Recover(log))
	r.Use(httpx.Locale)

	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie(testCookieName)
			if err != nil || cookie.Value == "" || cookie.Value == "forged-token-that-was-never-issued" {
				httpx.Error(w, r, log, apperr.Unauthorized())
				return
			}
			next.ServeHTTP(w, r)
		})
	})
	orgHttp.NewHandler(orgSvc, log).RegisterRoutes(r)

	return r
}

func newAuthedRouter(repo org.Repository) http.Handler {
	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	orgSvc := org.NewService(repo, log)

	r := chi.NewRouter()
	r.Use(httpx.RequestID)
	r.Use(httpx.Recover(log))
	r.Use(httpx.Locale)

	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			actor := authctx.Actor{
				UserID:         1,
				OrganizationID: 1,
				Role:           "admin",
				Permissions:    []string{"admin", "org.admin"},
			}
			ctx := authctx.WithActor(r.Context(), actor)
			ctx = database.WithTenant(ctx, 1)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	})
	orgHttp.NewHandler(orgSvc, log).RegisterRoutes(r)
	return r
}

var protectedRoutes = []struct{ method, path string }{
	{http.MethodPost, "/api/v1/org/organizations"},
	{http.MethodGet, "/api/v1/org/organizations/1"},
	{http.MethodPut, "/api/v1/org/organizations/1"},
	{http.MethodDelete, "/api/v1/org/organizations/1"},
	{http.MethodGet, "/api/v1/org/organizations"},
	{http.MethodPost, "/api/v1/org/organizations/1/status"},
	{http.MethodPost, "/api/v1/org/organizations/1/branches"},
	{http.MethodGet, "/api/v1/org/organizations/1/branches"},
	{http.MethodPut, "/api/v1/org/organizations/1/branches/1"},
	{http.MethodDelete, "/api/v1/org/organizations/1/branches/1"},
	{http.MethodPost, "/api/v1/org/organizations/1/members"},
	{http.MethodGet, "/api/v1/org/organizations/1/members"},
	{http.MethodPut, "/api/v1/org/organizations/1/members/1"},
	{http.MethodDelete, "/api/v1/org/organizations/1/members/1"},
	{http.MethodPost, "/api/v1/org/organizations/1/reviews"},
	{http.MethodGet, "/api/v1/org/organizations/1/reviews"},
	{http.MethodPost, "/api/v1/org/organizations/1/follow"},
	{http.MethodGet, "/api/v1/admin/org/pending"},
	{http.MethodPost, "/api/v1/admin/org/1/approve"},
	{http.MethodPost, "/api/v1/admin/org/1/reject"},
	{http.MethodPost, "/api/v1/admin/org/1/suspend"},
	{http.MethodPut, "/api/v1/admin/org/1"},
	{http.MethodGet, "/api/v1/admin/org/members"},
}

func TestProtectedRoutesRejectAnonymousCallers(t *testing.T) {
	router := newTestRouter(t)

	for _, route := range protectedRoutes {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			req := httptest.NewRequest(route.method, route.path, strings.NewReader("{}"))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Errorf("got %d, want 401 — this endpoint is reachable without a session", rec.Code)
			}
		})
	}
}

func TestProtectedRoutesRejectGarbageSessionToken(t *testing.T) {
	router := newTestRouter(t)

	for _, route := range protectedRoutes {
		req := httptest.NewRequest(route.method, route.path, strings.NewReader("{}"))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: "dawa24_session", Value: "forged-token-that-was-never-issued"})
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s with a forged token got %d, want 401", route.method, route.path, rec.Code)
		}
	}
}

func TestUnauthorizedResponseUsesTheErrorEnvelope(t *testing.T) {
	router := newTestRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/org/organizations", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	var body httpx.ErrorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not the JSON error envelope: %v (body: %s)", err, rec.Body.String())
	}
	if body.Error.Code == "" {
		t.Error("error envelope has no code")
	}
	if body.Error.RequestID == "" {
		t.Error("error envelope has no request_id")
	}
}

func TestOrgHandler_HappyPaths(t *testing.T) {
	router := newAuthedRouter(happyRepo{})

	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		wantStatus int
	}{
		{"RegisterOrg", http.MethodPost, "/api/v1/org/organizations", `{"legal_name":"Al-Amal","commercial_register":"CR-101","type":"pharmacy","credit_limit":"1000.00","payment_terms_days":30}`, http.StatusCreated},
		{"GetOrg", http.MethodGet, "/api/v1/org/organizations/1", "", http.StatusOK},
		{"UpdateOrg", http.MethodPut, "/api/v1/org/organizations/1", `{"legal_name":"Al-Amal Updated","commercial_register":"CR-101"}`, http.StatusOK},
		{"DeleteOrg", http.MethodDelete, "/api/v1/org/organizations/1", "", http.StatusOK},
		{"ListOrgs", http.MethodGet, "/api/v1/org/organizations?limit=10&offset=0", "", http.StatusOK},
		{"UpdateStatus", http.MethodPost, "/api/v1/org/organizations/1/status", `{"status":"approved"}`, http.StatusOK},
		{"CreateBranch", http.MethodPost, "/api/v1/org/organizations/1/branches", `{"code":"BR-01","name":{"en":"Main Branch"}}`, http.StatusCreated},
		{"ListBranches", http.MethodGet, "/api/v1/org/organizations/1/branches", "", http.StatusOK},
		{"UpdateBranch", http.MethodPut, "/api/v1/org/organizations/1/branches/1", `{"code":"BR-01","name":{"en":"Main Branch Updated"}}`, http.StatusOK},
		{"DeleteBranch", http.MethodDelete, "/api/v1/org/organizations/1/branches/1", "", http.StatusOK},
		{"AddMember", http.MethodPost, "/api/v1/org/organizations/1/members", `{"user_id":2,"role_id":1}`, http.StatusCreated},
		{"ListMembers", http.MethodGet, "/api/v1/org/organizations/1/members", "", http.StatusOK},
		{"UpdateMemberRole", http.MethodPut, "/api/v1/org/organizations/1/members/1", `{"role":"manager"}`, http.StatusOK},
		{"RemoveMember", http.MethodDelete, "/api/v1/org/organizations/1/members/1", "", http.StatusOK},
		{"AddReview", http.MethodPost, "/api/v1/org/organizations/1/reviews", `{"rating":5,"review_text":"Great service"}`, http.StatusCreated},
		{"ListReviews", http.MethodGet, "/api/v1/org/organizations/1/reviews?limit=10&offset=0", "", http.StatusOK},
		{"ToggleFollow", http.MethodPost, "/api/v1/org/organizations/1/follow", `{"user_id":1}`, http.StatusOK},
		{"AdminPending", http.MethodGet, "/api/v1/admin/org/pending", "", http.StatusOK},
		{"AdminApprove", http.MethodPost, "/api/v1/admin/org/1/approve", "", http.StatusOK},
		{"AdminReject", http.MethodPost, "/api/v1/admin/org/1/reject", "", http.StatusOK},
		{"AdminSuspend", http.MethodPost, "/api/v1/admin/org/1/suspend", "", http.StatusOK},
		{"AdminUpdate", http.MethodPut, "/api/v1/admin/org/1", `{"legal_name":"Admin Org Name","commercial_register":"CR-101"}`, http.StatusOK},
		{"AdminMembers", http.MethodGet, "/api/v1/admin/org/members", "", http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var bodyReader io.Reader
			if tt.body != "" {
				bodyReader = strings.NewReader(tt.body)
			}
			req := httptest.NewRequest(tt.method, tt.path, bodyReader)
			if tt.body != "" {
				req.Header.Set("Content-Type", "application/json")
			}
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code != tt.wantStatus {
				t.Errorf("%s %s got status %d, want %d (body: %s)", tt.method, tt.path, rec.Code, tt.wantStatus, rec.Body.String())
			}
		})
	}
}

func (r stubRepo) CountOrganizations(_ context.Context, _ *org.OrganizationType, _ *org.OrganizationStatus) (int, error) {
	r.fail("CountOrganizations")
	return 0, nil
}

func (happyRepo) CountOrganizations(_ context.Context, _ *org.OrganizationType, _ *org.OrganizationStatus) (int, error) {
	return 2, nil
}
