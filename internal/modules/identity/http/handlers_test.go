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
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/identity"
	identityHttp "github.com/muhiya/dawa24-store/internal/modules/identity/http"
	"github.com/muhiya/dawa24-store/internal/platform/config"
	"github.com/muhiya/dawa24-store/internal/platform/httpx"
)

// These tests cover the authorization surface of the identity handlers.
//
// Row-level security protects rows once a query runs; it cannot protect an
// endpoint that never checks who is calling. That gap is exactly what handler
// tests close, and it is why every protected route below is asserted to reject
// an anonymous caller rather than merely "not crash".
//
// The service is constructed with a nil SessionStore on purpose:
// Service.ValidateSession returns Unauthorized when the store is nil, which
// exercises the real middleware path without needing Redis.

// stubRepo satisfies identity.Repository. Every method fails loudly, because no
// test here should reach the repository: if one does, the request got past an
// authorization check it should not have.
type stubRepo struct{ t *testing.T }

func (r stubRepo) fail(method string) {
	r.t.Helper()
	r.t.Fatalf("repository.%s was called; the request should have been rejected before reaching the repository", method)
}

func (r stubRepo) CreateUser(context.Context, *identity.User) error { r.fail("CreateUser"); return nil }
func (r stubRepo) GetUserByID(context.Context, int64) (*identity.User, error) {
	r.fail("GetUserByID")
	return nil, nil
}
func (r stubRepo) GetUserByEmail(context.Context, string) (*identity.User, error) {
	r.fail("GetUserByEmail")
	return nil, nil
}
func (r stubRepo) UpdateUser(context.Context, *identity.User) error { r.fail("UpdateUser"); return nil }
func (r stubRepo) GetSecurity(context.Context, int64) (*identity.UserSecurity, error) {
	r.fail("GetSecurity")
	return nil, nil
}
func (r stubRepo) UpsertSecurity(context.Context, *identity.UserSecurity) error {
	r.fail("UpsertSecurity")
	return nil
}
func (r stubRepo) GetMFA(context.Context, int64) (*identity.UserMFA, error) {
	r.fail("GetMFA")
	return nil, nil
}
func (r stubRepo) UpsertMFA(context.Context, *identity.UserMFA) error {
	r.fail("UpsertMFA")
	return nil
}
func (r stubRepo) GetPermissionsForUser(context.Context, int64, int64) ([]string, error) {
	r.fail("GetPermissionsForUser")
	return nil, nil
}
func (r stubRepo) GetRolesForUser(context.Context, int64) ([]string, error) {
	r.fail("GetRolesForUser")
	return nil, nil
}
func (r stubRepo) UserBelongsToOrg(context.Context, int64, int64) (bool, error) {
	r.fail("UserBelongsToOrg")
	return false, nil
}
func (r stubRepo) CreateAddress(context.Context, *identity.UserAddress) error {
	r.fail("CreateAddress")
	return nil
}
func (r stubRepo) GetAddressByID(context.Context, int64, int64) (*identity.UserAddress, error) {
	r.fail("GetAddressByID")
	return nil, nil
}
func (r stubRepo) ListAddresses(context.Context, int64) ([]*identity.UserAddress, error) {
	r.fail("ListAddresses")
	return nil, nil
}
func (r stubRepo) UpdateAddress(context.Context, *identity.UserAddress) error {
	r.fail("UpdateAddress")
	return nil
}
func (r stubRepo) DeleteAddress(context.Context, int64, int64) error {
	r.fail("DeleteAddress")
	return nil
}
func (r stubRepo) AddFavorite(context.Context, int64, int64) error { r.fail("AddFavorite"); return nil }
func (r stubRepo) RemoveFavorite(context.Context, int64, int64) error {
	r.fail("RemoveFavorite")
	return nil
}
func (r stubRepo) ListFavorites(context.Context, int64) ([]int64, error) {
	r.fail("ListFavorites")
	return nil, nil
}

const testCookieName = "dawa24_session"

func newTestRouter(t *testing.T) http.Handler {
	t.Helper()

	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	svc := identity.NewService(stubRepo{t: t}, nil, log)

	r := chi.NewRouter()
	r.Use(httpx.RequestID)
	r.Use(httpx.Recover(log))
	r.Use(httpx.Locale)

	identityHttp.NewHandler(svc, config.Session{
		CookieName: testCookieName,
		TTL:        24 * time.Hour,
		SecureOnly: true,
	}, log).RegisterRoutes(r)

	return r
}

// protectedRoutes is every route behind RequireAuth. Adding a protected route
// without adding it here means it is untested; that is the point of listing
// them explicitly rather than discovering them by reflection.
var protectedRoutes = []struct{ method, path string }{
	{http.MethodGet, "/api/v1/auth/me"},
	{http.MethodGet, "/api/v1/me"},
	{http.MethodPut, "/api/v1/me"},
	{http.MethodGet, "/api/v1/me/addresses"},
	{http.MethodPost, "/api/v1/me/addresses"},
	{http.MethodPut, "/api/v1/me/addresses/1"},
	{http.MethodDelete, "/api/v1/me/addresses/1"},
	{http.MethodGet, "/api/v1/me/favorites"},
	{http.MethodPost, "/api/v1/me/favorites"},
	{http.MethodDelete, "/api/v1/me/favorites/1"},
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

	// A token that is well-formed but not a real session. It must be rejected
	// by validation, not merely by absence.
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

func TestBearerTokenIsAlsoValidated(t *testing.T) {
	router := newTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	req.Header.Set("Authorization", "Bearer forged-token")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("got %d, want 401 for a forged bearer token", rec.Code)
	}
}

func TestUnauthorizedResponseUsesTheErrorEnvelope(t *testing.T) {
	router := newTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	var body httpx.ErrorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not the JSON error envelope: %v (body: %s)", err, rec.Body.String())
	}

	if body.Error.Code == "" {
		t.Error("error envelope has no code; clients cannot branch on it")
	}
	// The request id is what ties a user's support ticket to a log line.
	if body.Error.RequestID == "" {
		t.Error("error envelope has no request_id")
	}
}

func TestLogoutClearsTheSessionCookieSecurely(t *testing.T) {
	router := newTestRouter(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	req.AddCookie(&http.Cookie{Name: testCookieName, Value: "whatever"})
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("logout returned %d, want 204", rec.Code)
	}

	var cleared *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == testCookieName {
			cleared = c
		}
	}
	if cleared == nil {
		t.Fatal("logout did not set a clearing cookie; the browser keeps the old session")
	}
	if cleared.Value != "" || cleared.MaxAge >= 0 {
		t.Errorf("cookie not cleared: value=%q maxage=%d", cleared.Value, cleared.MaxAge)
	}
	// A session cookie readable by JavaScript is stealable by any XSS.
	if !cleared.HttpOnly {
		t.Error("session cookie is not HttpOnly")
	}
	if !cleared.Secure {
		t.Error("session cookie is not Secure despite SecureOnly config")
	}
	if cleared.SameSite != http.SameSiteLaxMode {
		t.Errorf("SameSite is %v, want Lax", cleared.SameSite)
	}
}

func TestLoginRejectsUnknownJSONFields(t *testing.T) {
	router := newTestRouter(t)

	// DecodeJSON uses DisallowUnknownFields. A typo'd field must fail loudly
	// rather than be silently ignored — otherwise a client "sets" a value that
	// never arrives and the bug surfaces much later.
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login",
		strings.NewReader(`{"email":"a@b.com","password":"x","totally_unknown":"y"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("got %d, want 422 for an unknown JSON field", rec.Code)
	}
}

func TestLoginRejectsMalformedBody(t *testing.T) {
	router := newTestRouter(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login",
		strings.NewReader(`{"email": `))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("got %d, want 422 for a malformed body", rec.Code)
	}
}

func TestArabicIsTheDefaultErrorLanguage(t *testing.T) {
	router := newTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	var body httpx.ErrorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("not an error envelope: %v", err)
	}

	// This platform is Arabic-first: with no Accept-Language and no cookie, the
	// user-facing message must come back in Arabic.
	if !strings.ContainsAny(body.Error.Message, "ابتثجحخدذرزسشصضطظعغفقكلمنهوي") {
		t.Errorf("default error message is not Arabic: %q", body.Error.Message)
	}
}

func TestEnglishRequestedExplicitlyIsHonoured(t *testing.T) {
	router := newTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/me?lang=en", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	var body httpx.ErrorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("not an error envelope: %v", err)
	}
	if strings.ContainsAny(body.Error.Message, "ابتثجحخدذرزسشصضطظعغفقكلمنهوي") {
		t.Errorf("lang=en still returned Arabic: %q", body.Error.Message)
	}
}
