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

	"github.com/muhiya/dawa24-store/internal/modules/identity"
	identityHttp "github.com/muhiya/dawa24-store/internal/modules/identity/http"
	"github.com/muhiya/dawa24-store/internal/modules/workflow"
	workflowHttp "github.com/muhiya/dawa24-store/internal/modules/workflow/http"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/httpx"
)

type stubRepo struct{ t *testing.T }

func (r stubRepo) fail(method string) {
	r.t.Helper()
	r.t.Fatalf("repository.%s was called; the request should have been rejected before reaching the repository", method)
}

func (r stubRepo) CreatePriorityRequest(context.Context, *workflow.PurchasePriorityRequest) error {
	r.fail("CreatePriorityRequest")
	return nil
}
func (r stubRepo) GetPriorityRequestByID(context.Context, int64) (*workflow.PurchasePriorityRequest, error) {
	r.fail("GetPriorityRequestByID")
	return nil, nil
}
func (r stubRepo) SaveWeeklyCoverage(context.Context, *workflow.WeeklyCoverage) error {
	r.fail("SaveWeeklyCoverage")
	return nil
}
func (r stubRepo) ListWeeklyCoverage(context.Context, int64) ([]*workflow.WeeklyCoverage, error) {
	r.fail("ListWeeklyCoverage")
	return nil, nil
}
func (r stubRepo) CreateIssue(context.Context, *workflow.ReportIssue) error {
	r.fail("CreateIssue")
	return nil
}
func (r stubRepo) GetIssueByID(context.Context, int64) (*workflow.ReportIssue, error) {
	r.fail("GetIssueByID")
	return nil, nil
}
func (r stubRepo) ListIssues(context.Context, int, int) ([]*workflow.ReportIssue, error) {
	r.fail("ListIssues")
	return nil, nil
}

type happyRepo struct{}

func (happyRepo) CreatePriorityRequest(ctx context.Context, r *workflow.PurchasePriorityRequest) error {
	r.ID = 1
	r.Status = "pending"
	return nil
}
func (happyRepo) GetPriorityRequestByID(ctx context.Context, id int64) (*workflow.PurchasePriorityRequest, error) {
	return &workflow.PurchasePriorityRequest{ID: id, UserID: 1, Status: "pending"}, nil
}
func (happyRepo) SaveWeeklyCoverage(ctx context.Context, c *workflow.WeeklyCoverage) error {
	c.ID = 1
	return nil
}
func (happyRepo) ListWeeklyCoverage(ctx context.Context, branchID int64) ([]*workflow.WeeklyCoverage, error) {
	return []*workflow.WeeklyCoverage{{ID: 1, BranchID: branchID, DayOfWeek: 1, DistanceMeters: 5000, IsActive: true}}, nil
}
func (happyRepo) CreateIssue(ctx context.Context, i *workflow.ReportIssue) error {
	i.ID = 1
	return nil
}
func (happyRepo) GetIssueByID(ctx context.Context, id int64) (*workflow.ReportIssue, error) {
	return &workflow.ReportIssue{ID: id, ReportedBy: 1, IssueType: "quality", Description: "Broken seal"}, nil
}
func (happyRepo) ListIssues(ctx context.Context, limit, offset int) ([]*workflow.ReportIssue, error) {
	return []*workflow.ReportIssue{{ID: 1, ReportedBy: 1, IssueType: "quality", Description: "Broken seal"}}, nil
}

const testCookieName = "dawa24_session"

func newTestRouter(t *testing.T) http.Handler {
	t.Helper()

	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	idSvc := identity.NewService(nil, nil, log)
	wfSvc := workflow.NewService(stubRepo{t: t}, log)

	r := chi.NewRouter()
	r.Use(httpx.RequestID)
	r.Use(httpx.Recover(log))
	r.Use(httpx.Locale)

	r.Group(func(protected chi.Router) {
		protected.Use(identityHttp.RequireAuth(idSvc, testCookieName, log))
		workflowHttp.NewHandler(wfSvc, log).RegisterRoutes(protected)
	})

	return r
}

func newAuthedRouter(repo workflow.Repository) http.Handler {
	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	wfSvc := workflow.NewService(repo, log)

	r := chi.NewRouter()
	r.Use(httpx.RequestID)
	r.Use(httpx.Recover(log))
	r.Use(httpx.Locale)

	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sess := &identity.Session{
				UserID:      1,
				ActiveOrgID: 1,
				Role:        "admin",
				Permissions: []string{"admin", "workflow.admin"},
			}
			ctx := identityHttp.WithSession(r.Context(), sess)
			actor := authctx.Actor{
				UserID:         1,
				OrganizationID: 1,
				Role:           "admin",
				Permissions:    []string{"admin", "workflow.admin"},
			}
			ctx = authctx.WithActor(ctx, actor)
			ctx = database.WithTenant(ctx, 1)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	})
	workflowHttp.NewHandler(wfSvc, log).RegisterRoutes(r)
	return r
}

var protectedRoutes = []struct{ method, path string }{
	{http.MethodPost, "/api/v1/workflow/priority-requests"},
	{http.MethodPost, "/api/v1/workflow/branches/1/coverage"},
	{http.MethodGet, "/api/v1/workflow/branches/1/coverage"},
	{http.MethodPost, "/api/v1/workflow/issues"},
	{http.MethodGet, "/api/v1/workflow/issues"},
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
	req := httptest.NewRequest(http.MethodGet, "/api/v1/workflow/issues", nil)
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

func TestWorkflowHandler_HappyPaths(t *testing.T) {
	router := newAuthedRouter(happyRepo{})

	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		wantStatus int
	}{
		{"CreatePriorityRequest", http.MethodPost, "/api/v1/workflow/priority-requests", `{"user_id":1,"priority_lowest_price":true,"budget_constraint":"1000.00"}`, http.StatusCreated},
		{"SetWeeklyCoverage", http.MethodPost, "/api/v1/workflow/branches/1/coverage", `{"branch_id":1,"day_of_week":1,"distance_meters":5000,"is_active":true}`, http.StatusOK},
		{"GetBranchCoverage", http.MethodGet, "/api/v1/workflow/branches/1/coverage", "", http.StatusOK},
		{"ReportIssue", http.MethodPost, "/api/v1/workflow/issues", `{"reported_by":1,"issue_type":"quality","description":"Broken seal","priority":"high"}`, http.StatusCreated},
		{"ListIssues", http.MethodGet, "/api/v1/workflow/issues?limit=10&offset=0", "", http.StatusOK},
		{"AdminIssues", http.MethodGet, "/api/v1/admin/workflow/issues", "", http.StatusOK},
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
