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
	"github.com/muhiya/dawa24-store/internal/platform/httpx"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
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

func requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("dawa24_session")
		authHeader := r.Header.Get("Authorization")
		if (err != nil || cookie.Value == "forged-token-that-was-never-issued") && (authHeader == "" || authHeader == "Bearer forged-token") {
			httpx.Error(w, r, slog.Default(), apperr.Unauthorized())
			return
		}
		next.ServeHTTP(w, r)
	})
}

func newTestRouter(t *testing.T) http.Handler {
	t.Helper()
	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	svc := org.NewService(stubRepo{t: t}, log)
	r := chi.NewRouter()
	r.Use(httpx.RequestID)
	r.Use(httpx.Recover(log))
	r.Use(httpx.Locale)
	r.Use(requireAuth) // apply mock auth middleware for testing
	orgHttp.NewHandler(svc, log).RegisterRoutes(r)
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

func TestBearerTokenIsAlsoValidated(t *testing.T) {
	router := newTestRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/org/organizations", nil)
	req.Header.Set("Authorization", "Bearer forged-token")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("got %d, want 401 for a forged bearer token", rec.Code)
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
		t.Error("error envelope has no code; clients cannot branch on it")
	}
	if body.Error.RequestID == "" {
		t.Error("error envelope has no request_id")
	}
}

// Ensure proper setup for malformed JSON checking by omitting requireAuth so it hits the handler.
func newTestRouterNoAuth(t *testing.T) http.Handler {
	t.Helper()
	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	svc := org.NewService(stubRepo{t: t}, log)
	r := chi.NewRouter()
	r.Use(httpx.RequestID)
	r.Use(httpx.Recover(log))
	r.Use(httpx.Locale)
	orgHttp.NewHandler(svc, log).RegisterRoutes(r)
	return r
}

func TestHandlerRejectsUnknownJSONFields(t *testing.T) {
	router := newTestRouterNoAuth(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/org/organizations", strings.NewReader(`{"legal_name":"A","unknown_field":"B"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("got %d, want 422 for an unknown JSON field", rec.Code)
	}
}

func TestHandlerRejectsMalformedBody(t *testing.T) {
	router := newTestRouterNoAuth(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/org/organizations", strings.NewReader(`{"legal_name": `))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("got %d, want 422 for a malformed body", rec.Code)
	}
}
