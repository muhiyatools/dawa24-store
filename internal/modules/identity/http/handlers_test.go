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
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/config"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/httpx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

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
func (r stubRepo) AddFavorite(context.Context, int64, int64) error {
	r.fail("AddFavorite")
	return nil
}
func (r stubRepo) RemoveFavorite(context.Context, int64, int64) error {
	r.fail("RemoveFavorite")
	return nil
}
func (r stubRepo) ListFavorites(context.Context, int64) ([]int64, error) {
	r.fail("ListFavorites")
	return nil, nil
}
func (r stubRepo) AdminListUsers(context.Context, string, string) ([]*identity.User, error) {
	r.fail("AdminListUsers")
	return nil, nil
}
func (r stubRepo) AdminUpdateUserStatus(context.Context, int64, string, int64) error {
	r.fail("AdminUpdateUserStatus")
	return nil
}
func (r stubRepo) AdminResetMFA(context.Context, int64, int64) error {
	r.fail("AdminResetMFA")
	return nil
}
func (r stubRepo) AdminAssignRole(context.Context, int64, string, int64) error {
	r.fail("AdminAssignRole")
	return nil
}

type happyRepo struct{}

func (happyRepo) CreateUser(ctx context.Context, u *identity.User) error {
	u.ID = 1
	return nil
}
func (happyRepo) GetUserByID(ctx context.Context, id int64) (*identity.User, error) {
	return &identity.User{
		ID:       id,
		Email:    "user@example.com",
		Name:     i18n.Text{"en": "User"},
		Status:   identity.StatusActive,
		Role:     "customer",
		Language: "en",
	}, nil
}
func (happyRepo) GetUserByEmail(ctx context.Context, email string) (*identity.User, error) {
	return &identity.User{
		ID:           1,
		Email:        email,
		Name:         i18n.Text{"en": "User"},
		Status:       identity.StatusActive,
		Role:         "customer",
		PasswordHash: "$2a$10$abcdefghijklmnopqrstuu",
	}, nil
}
func (happyRepo) UpdateUser(ctx context.Context, u *identity.User) error { return nil }
func (happyRepo) GetSecurity(ctx context.Context, id int64) (*identity.UserSecurity, error) {
	return &identity.UserSecurity{UserID: id}, nil
}
func (happyRepo) UpsertSecurity(ctx context.Context, s *identity.UserSecurity) error { return nil }
func (happyRepo) GetMFA(ctx context.Context, id int64) (*identity.UserMFA, error) {
	return &identity.UserMFA{UserID: id, Enabled: false}, nil
}
func (happyRepo) UpsertMFA(ctx context.Context, m *identity.UserMFA) error { return nil }
func (happyRepo) GetPermissionsForUser(ctx context.Context, userID, orgID int64) ([]string, error) {
	return []string{"customer"}, nil
}
func (happyRepo) GetRolesForUser(ctx context.Context, userID int64) ([]string, error) {
	return []string{"customer"}, nil
}
func (happyRepo) UserBelongsToOrg(ctx context.Context, userID, orgID int64) (bool, error) {
	return true, nil
}
func (happyRepo) CreateAddress(ctx context.Context, a *identity.UserAddress) error {
	a.ID = 1
	return nil
}
func (happyRepo) GetAddressByID(ctx context.Context, id, userID int64) (*identity.UserAddress, error) {
	return &identity.UserAddress{ID: id, UserID: userID, Title: "Home", Recipient: "User", Phone: "01000000000", Address: "123 Main St", CityID: 1}, nil
}
func (happyRepo) ListAddresses(ctx context.Context, userID int64) ([]*identity.UserAddress, error) {
	return []*identity.UserAddress{{ID: 1, UserID: userID, Title: "Home", Recipient: "User", Phone: "01000000000", Address: "123 Main St", CityID: 1}}, nil
}
func (happyRepo) UpdateAddress(ctx context.Context, a *identity.UserAddress) error { return nil }
func (happyRepo) DeleteAddress(ctx context.Context, id, userID int64) error        { return nil }
func (happyRepo) AddFavorite(ctx context.Context, userID, productID int64) error   { return nil }
func (happyRepo) RemoveFavorite(ctx context.Context, userID, productID int64) error {
	return nil
}
func (happyRepo) ListFavorites(ctx context.Context, userID int64) ([]int64, error) {
	return []int64{1, 2}, nil
}
func (happyRepo) AdminListUsers(ctx context.Context, role, status string) ([]*identity.User, error) {
	return []*identity.User{{ID: 1, Email: "user@example.com"}}, nil
}
func (happyRepo) AdminUpdateUserStatus(ctx context.Context, userID int64, status string, actorID int64) error {
	return nil
}
func (happyRepo) AdminResetMFA(ctx context.Context, userID int64, actorID int64) error {
	return nil
}
func (happyRepo) AdminAssignRole(ctx context.Context, userID int64, role string, actorID int64) error {
	return nil
}

const testCookieName = "dawa24_session"

func newTestRouter(t *testing.T) http.Handler {
	t.Helper()

	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	svc := identity.NewService(stubRepo{t: t}, nil, log)
	handler := identityHttp.NewHandler(svc, config.Session{
		CookieName: testCookieName,
		TTL:        30 * 24 * time.Hour,
		SecureOnly: false,
	}, log)

	r := chi.NewRouter()
	r.Use(httpx.RequestID)
	r.Use(httpx.Recover(log))
	r.Use(httpx.Locale)
	handler.RegisterRoutes(r)
	return r
}

func newAuthedRouter(repo identity.Repository) http.Handler {
	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	svc := identity.NewService(repo, nil, log)
	handler := identityHttp.NewHandler(svc, config.Session{
		CookieName: testCookieName,
		TTL:        30 * 24 * time.Hour,
		SecureOnly: false,
	}, log)

	r := chi.NewRouter()
	r.Use(httpx.RequestID)
	r.Use(httpx.Recover(log))
	r.Use(httpx.Locale)

	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sess := &identity.Session{
				UserID:      1,
				ActiveOrgID: 1,
				Role:        "super_admin",
				Permissions: []string{"admin", "super_admin", "identity.admin"},
			}
			ctx := identityHttp.WithSession(r.Context(), sess)
			actor := authctx.Actor{
				UserID:         1,
				OrganizationID: 1,
				Role:           "super_admin",
				Permissions:    []string{"admin", "super_admin", "identity.admin"},
			}
			ctx = authctx.WithActor(ctx, actor)
			ctx = database.WithTenant(ctx, 1)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	})

	r.Post("/api/v1/auth/register", handler.Register)
	r.Post("/api/v1/auth/login", handler.Login)
	r.Post("/api/v1/auth/logout", handler.Logout)
	r.Get("/api/v1/auth/me", handler.Me)
	r.Get("/api/v1/me", handler.GetMe)
	r.Put("/api/v1/me", handler.UpdateMe)
	r.Get("/api/v1/me/addresses", handler.ListAddresses)
	r.Post("/api/v1/me/addresses", handler.CreateAddress)
	r.Put("/api/v1/me/addresses/{id}", handler.UpdateAddress)
	r.Delete("/api/v1/me/addresses/{id}", handler.DeleteAddress)
	r.Get("/api/v1/me/favorites", handler.ListFavorites)
	r.Post("/api/v1/me/favorites", handler.AddFavorite)
	r.Delete("/api/v1/me/favorites/{productId}", handler.RemoveFavorite)
	handler.RegisterAdminRoutes(r)
	return r
}

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

	for _, route := range protectedRoutes {
		req := httptest.NewRequest(route.method, route.path, strings.NewReader("{}"))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{
			Name:  testCookieName,
			Value: "forged-token-that-was-never-issued",
		})
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s with a forged token got %d, want 401", route.method, route.path, rec.Code)
		}
	}
}

func TestUnauthorizedResponseUsesTheErrorEnvelope(t *testing.T) {
	router := newTestRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
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

func TestIdentityHandler_HappyPaths(t *testing.T) {
	router := newAuthedRouter(happyRepo{})

	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		wantStatus int
	}{
		{"Register", http.MethodPost, "/api/v1/auth/register", `{"email":"newuser@example.com","password":"password123","name_ar":"مستخدم","name_en":"User"}`, http.StatusCreated},
		{"Logout", http.MethodPost, "/api/v1/auth/logout", "", http.StatusNoContent},
		{"Me", http.MethodGet, "/api/v1/auth/me", "", http.StatusOK},
		{"GetMe", http.MethodGet, "/api/v1/me", "", http.StatusOK},
		{"UpdateMe", http.MethodPut, "/api/v1/me", `{"name_ar":"مستخدم","name_en":"Updated User","phone":"01000000000","timezone":"Africa/Cairo","language":"en"}`, http.StatusOK},
		{"ListAddresses", http.MethodGet, "/api/v1/me/addresses", "", http.StatusOK},
		{"CreateAddress", http.MethodPost, "/api/v1/me/addresses", `{"title":"Office","recipient":"User","phone":"01000000000","address":"456 Nile St","city_id":1}`, http.StatusCreated},
		{"UpdateAddress", http.MethodPut, "/api/v1/me/addresses/1", `{"title":"Office 2","recipient":"User","phone":"01000000000","address":"456 Nile St","city_id":1}`, http.StatusOK},
		{"DeleteAddress", http.MethodDelete, "/api/v1/me/addresses/1", "", http.StatusOK},
		{"ListFavorites", http.MethodGet, "/api/v1/me/favorites", "", http.StatusOK},
		{"AddFavorite", http.MethodPost, "/api/v1/me/favorites", `{"product_id":1}`, http.StatusOK},
		{"RemoveFavorite", http.MethodDelete, "/api/v1/me/favorites/1", "", http.StatusOK},
		{"AdminUsers", http.MethodGet, "/api/v1/admin/identity/users", "", http.StatusOK},
		{"AdminGetUser", http.MethodGet, "/api/v1/admin/identity/users/1", "", http.StatusOK},
		{"AdminSuspend", http.MethodPost, "/api/v1/admin/identity/users/1/suspend", "", http.StatusOK},
		{"AdminReactivate", http.MethodPost, "/api/v1/admin/identity/users/1/reactivate", "", http.StatusOK},
		{"AdminResetMFA", http.MethodPost, "/api/v1/admin/identity/users/1/reset-mfa", "", http.StatusOK},
		{"AdminAssignRole", http.MethodPut, "/api/v1/admin/identity/users/1/role", `{"role":"vendor"}`, http.StatusOK},
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

func (r stubRepo) AdminCountUsers(_ context.Context) (int, error) {
	r.fail("AdminCountUsers")
	return 0, nil
}

func (happyRepo) AdminCountUsers(_ context.Context) (int, error) { return 3, nil }

func (r stubRepo) DefaultOrgForUser(_ context.Context, _ int64) (int64, error) {
	r.fail("DefaultOrgForUser")
	return 0, nil
}

func (happyRepo) DefaultOrgForUser(_ context.Context, _ int64) (int64, error) { return 1, nil }
