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
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/httpx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
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

type happyRepo struct{}

func (happyRepo) GetSetting(ctx context.Context, key string) (*platformadmin.SystemSetting, error) {
	return &platformadmin.SystemSetting{Key: key, Value: map[string]any{"mode": "dark"}}, nil
}
func (happyRepo) SetSetting(ctx context.Context, s *platformadmin.SystemSetting) error {
	return nil
}
func (happyRepo) ListPublicSettings(ctx context.Context) ([]*platformadmin.SystemSetting, error) {
	return []*platformadmin.SystemSetting{{Key: "site_name", Value: map[string]any{"name": "Dawa24"}}}, nil
}
func (happyRepo) ListCountries(ctx context.Context) ([]*platformadmin.Country, error) {
	return []*platformadmin.Country{{ID: 1, Name: i18n.Text{"en": "Egypt"}, Code: "EG"}}, nil
}
func (happyRepo) ListCities(ctx context.Context, countryID int64) ([]*platformadmin.City, error) {
	return []*platformadmin.City{{ID: 1, CountryID: countryID, Name: i18n.Text{"en": "Cairo"}}}, nil
}
func (happyRepo) ListCurrencies(ctx context.Context) ([]*platformadmin.Currency, error) {
	return []*platformadmin.Currency{{ID: 1, Code: "EGP", Name: i18n.Text{"en": "Egyptian Pound"}}}, nil
}
func (happyRepo) ListLanguages(ctx context.Context) ([]*platformadmin.Language, error) {
	return []*platformadmin.Language{{ID: 1, Code: "ar", Name: "Arabic"}}, nil
}
func (happyRepo) CreateContactMessage(ctx context.Context, m *platformadmin.ContactMessage) error {
	m.ID = 1
	return nil
}
func (happyRepo) ListContactMessages(ctx context.Context, status string, limit, offset int) ([]*platformadmin.ContactMessage, error) {
	return []*platformadmin.ContactMessage{{ID: 1, Name: "User", Email: "u@example.com", Message: "Help"}}, nil
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

func newAuthedRouter(repo platformadmin.Repository) http.Handler {
	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	paSvc := platformadmin.NewService(repo, log)

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
				Permissions: []string{"admin", "platform.admin"},
			}
			ctx := identityHttp.WithSession(r.Context(), sess)
			actor := authctx.Actor{
				UserID:         1,
				OrganizationID: 1,
				Role:           "admin",
				Permissions:    []string{"admin", "platform.admin"},
			}
			ctx = authctx.WithActor(ctx, actor)
			ctx = database.WithTenant(ctx, 1)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	})
	platformadminHttp.NewHandler(paSvc, log).RegisterRoutes(r)
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
	req := httptest.NewRequest(http.MethodGet, "/api/v1/platform/countries", nil)
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

func TestPlatformAdminHandler_HappyPaths(t *testing.T) {
	router := newAuthedRouter(happyRepo{})

	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		wantStatus int
	}{
		{"ListPublicSettings", http.MethodGet, "/api/v1/platform/settings/public", "", http.StatusOK},
		{"GetSetting", http.MethodGet, "/api/v1/platform/settings/theme", "", http.StatusOK},
		{"SetSetting", http.MethodPut, "/api/v1/platform/settings/theme", `{"value":{"mode":"dark"}}`, http.StatusOK},
		{"ListCountries", http.MethodGet, "/api/v1/platform/countries", "", http.StatusOK},
		{"ListCities", http.MethodGet, "/api/v1/platform/countries/1/cities", "", http.StatusOK},
		{"ListCurrencies", http.MethodGet, "/api/v1/platform/currencies", "", http.StatusOK},
		{"ListLanguages", http.MethodGet, "/api/v1/platform/languages", "", http.StatusOK},
		{"SubmitContact", http.MethodPost, "/api/v1/platform/contact", `{"name":"User","email":"u@example.com","message":"Hello"}`, http.StatusCreated},
		{"ListContactMessages", http.MethodGet, "/api/v1/platform/contact?limit=10&offset=0", "", http.StatusOK},
		{"AdminTranslations", http.MethodGet, "/api/v1/admin/platform/translations", "", http.StatusOK},
		{"AdminUpdateTranslation", http.MethodPut, "/api/v1/admin/platform/translations/key", `{"value":"val"}`, http.StatusOK},
		{"AdminAuditLog", http.MethodGet, "/api/v1/admin/platform/audit-log", "", http.StatusOK},
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
