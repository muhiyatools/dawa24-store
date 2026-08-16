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
	"github.com/muhiya/dawa24-store/internal/modules/ingest"
	ingestHttp "github.com/muhiya/dawa24-store/internal/modules/ingest/http"
	"github.com/muhiya/dawa24-store/internal/platform/httpx"
)

type stubRepo struct{ t *testing.T }

func (r stubRepo) fail(method string) {
	r.t.Helper()
	r.t.Fatalf("repository.%s was called; the request should have been rejected before reaching the repository", method)
}

func (r stubRepo) CreateFileUpload(context.Context, *ingest.FileUpload) error {
	r.fail("CreateFileUpload")
	return nil
}
func (r stubRepo) GetFileUploadByID(context.Context, int64) (*ingest.FileUpload, error) {
	r.fail("GetFileUploadByID")
	return nil, nil
}
func (r stubRepo) CreateImportSession(context.Context, *ingest.ImportSession) error {
	r.fail("CreateImportSession")
	return nil
}
func (r stubRepo) GetImportSessionByID(context.Context, int64) (*ingest.ImportSession, error) {
	r.fail("GetImportSessionByID")
	return nil, nil
}
func (r stubRepo) ListImportSessions(context.Context, int64, int, int) ([]*ingest.ImportSession, error) {
	r.fail("ListImportSessions")
	return nil, nil
}
func (r stubRepo) UpdateImportSessionProgress(context.Context, int64, int, int, ingest.SessionStatus, string) error {
	r.fail("UpdateImportSessionProgress")
	return nil
}
func (r stubRepo) UpdateColumnMapping(context.Context, int64, map[string]string) error {
	r.fail("UpdateColumnMapping")
	return nil
}
func (r stubRepo) UpdateSessionStatus(context.Context, int64, ingest.SessionStatus) error {
	r.fail("UpdateSessionStatus")
	return nil
}
func (r stubRepo) InsertImportRows(context.Context, []*ingest.ImportRow) error {
	r.fail("InsertImportRows")
	return nil
}
func (r stubRepo) ListImportRows(context.Context, int64, string, int, int) ([]*ingest.ImportRow, error) {
	r.fail("ListImportRows")
	return nil, nil
}
func (r stubRepo) GetImportRowByID(context.Context, int64) (*ingest.ImportRow, error) {
	r.fail("GetImportRowByID")
	return nil, nil
}
func (r stubRepo) UpdateImportRowMatch(context.Context, int64, *int64, float64, string) error {
	r.fail("UpdateImportRowMatch")
	return nil
}

const testCookieName = "dawa24_session"

func newTestRouter(t *testing.T) http.Handler {
	t.Helper()

	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	idSvc := identity.NewService(nil, nil, log)
	ingestSvc := ingest.NewService(stubRepo{t: t}, log)

	r := chi.NewRouter()
	r.Use(httpx.RequestID)
	r.Use(httpx.Recover(log))
	r.Use(httpx.Locale)

	r.Group(func(protected chi.Router) {
		protected.Use(identityHttp.RequireAuth(idSvc, testCookieName, log))
		ingestHttp.NewHandler(ingestSvc, log).RegisterRoutes(protected)
	})

	return r
}

var protectedRoutes = []struct{ method, path string }{
	{http.MethodPost, "/api/v1/ingest/uploads"},
	{http.MethodPost, "/api/v1/ingest/sessions"},
	{http.MethodGet, "/api/v1/ingest/sessions"},
	{http.MethodGet, "/api/v1/ingest/sessions/1"},
	{http.MethodGet, "/api/v1/ingest/sessions/1/rows"},
	{http.MethodPost, "/api/v1/ingest/sessions/1/mapping"},
	{http.MethodPost, "/api/v1/ingest/sessions/1/commit"},
	{http.MethodPost, "/api/v1/ingest/sessions/1/cancel"},
	{http.MethodPut, "/api/v1/ingest/sessions/1/rows/1"},
	{http.MethodGet, "/api/v1/ingest/sessions/1/events"},
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
		req.AddCookie(&http.Cookie{Name: testCookieName, Value: "forged-token-that-was-never-issued"})
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s with a forged token got %d, want 401", route.method, route.path, rec.Code)
		}
	}
}

func TestUnauthorizedResponseUsesTheErrorEnvelope(t *testing.T) {
	router := newTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/ingest/sessions", nil)
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
