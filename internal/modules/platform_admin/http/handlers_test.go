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
	platformadmin "github.com/muhiya/dawa24-store/internal/modules/platform_admin"
	platformadminHttp "github.com/muhiya/dawa24-store/internal/modules/platform_admin/http"
	"github.com/muhiya/dawa24-store/internal/platform/httpx"
)

type stubRepo struct{ t *testing.T }

func (r stubRepo) fail(method string) {
	r.t.Helper()
	r.t.Fatalf("repository.%s was called; the request should have been rejected before reaching the repository", method)
}

func (r stubRepo) GetSetting(context.Context, string) (*platformadmin.SystemSetting, error) {
	r.fail("GetSetting")
	return nil, nil
}
func (r stubRepo) SetSetting(context.Context, *platformadmin.SystemSetting) error {
	r.fail("SetSetting")
	return nil
}
func (r stubRepo) ListPublicSettings(context.Context) ([]*platformadmin.SystemSetting, error) {
	r.fail("ListPublicSettings")
	return nil, nil
}
func (r stubRepo) ListCountries(context.Context) ([]*platformadmin.Country, error) {
	r.fail("ListCountries")
	return nil, nil
}
func (r stubRepo) ListCities(context.Context, int64) ([]*platformadmin.City, error) {
	r.fail("ListCities")
	return nil, nil
}
func (r stubRepo) ListCurrencies(context.Context) ([]*platformadmin.Currency, error) {
	r.fail("ListCurrencies")
	return nil, nil
}
func (r stubRepo) ListLanguages(context.Context) ([]*platformadmin.Language, error) {
	r.fail("ListLanguages")
	return nil, nil
}
func (r stubRepo) CreateContactMessage(context.Context, *platformadmin.ContactMessage) error {
	r.fail("CreateContactMessage")
	return nil
}
func (r stubRepo) ListContactMessages(context.Context, string, int, int) ([]*platformadmin.ContactMessage, error) {
	r.fail("ListContactMessages")
	return nil, nil
}

const testCookieName = "dawa24_session"

func newTestRouter(t *testing.T) http.Handler {
	t.Helper()

	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	idSvc := identity.NewService(nil, nil, log)
	paSvc := platformadmin.NewService(stubRepo{t: t}, log)

	r := chi.NewRouter()
	r.Use(httpx.RequestID)
	r.Use(httpx.Recover(log))
	r.Use(httpx.Locale)

	r.Group(func(protected chi.Router) {
		protected.Use(identityHttp.RequireAuth(idSvc, testCookieName, log))
		platformadminHttp.NewHandler(paSvc, log).RegisterRoutes(protected)
	})

	return r
}

var protectedRoutes = []struct{ method, path string }{
	{http.MethodGet, "/api/v1/platform/settings/public"},
	{http.MethodGet, "/api/v1/platform/settings/theme"},
	{http.MethodPut, "/api/v1/platform/settings/theme"},
	{http.MethodGet, "/api/v1/platform/countries"},
	{http.MethodGet, "/api/v1/platform/countries/1/cities"},
	{http.MethodGet, "/api/v1/platform/currencies"},
	{http.MethodGet, "/api/v1/platform/languages"},
	{http.MethodPost, "/api/v1/platform/contact"},
	{http.MethodGet, "/api/v1/platform/contact"},
	{http.MethodGet, "/api/v1/admin/platform/translations"},
	{http.MethodPut, "/api/v1/admin/platform/translations/key"},
	{http.MethodGet, "/api/v1/admin/platform/audit-log"},
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

	req := httptest.NewRequest(http.MethodGet, "/api/v1/platform/settings/public", nil)
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
