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
	"github.com/muhiya/dawa24-store/internal/modules/notifications"
	notificationsHttp "github.com/muhiya/dawa24-store/internal/modules/notifications/http"
	"github.com/muhiya/dawa24-store/internal/platform/httpx"
)

type stubRepo struct{ t *testing.T }

func (r stubRepo) fail(method string) {
	r.t.Helper()
	r.t.Fatalf("repository.%s was called; the request should have been rejected before reaching the repository", method)
}

func (r stubRepo) CreateLog(context.Context, *notifications.NotificationLog) error {
	r.fail("CreateLog")
	return nil
}
func (r stubRepo) GetTemplateBySlug(context.Context, string) (*notifications.Template, error) {
	r.fail("GetTemplateBySlug")
	return nil, nil
}
func (r stubRepo) ListUserNotifications(context.Context, int64, int, int) ([]*notifications.NotificationLog, error) {
	r.fail("ListUserNotifications")
	return nil, nil
}
func (r stubRepo) MarkAsRead(context.Context, int64, int64) error { r.fail("MarkAsRead"); return nil }
func (r stubRepo) MarkAllAsRead(context.Context, int64) (int64, error) {
	r.fail("MarkAllAsRead")
	return 0, nil
}
func (r stubRepo) ListUnread(context.Context, int64, int, int) ([]*notifications.NotificationLog, error) {
	r.fail("ListUnread")
	return nil, nil
}
func (r stubRepo) GetUnreadCount(context.Context, int64) (int, error) {
	r.fail("GetUnreadCount")
	return 0, nil
}

const testCookieName = "dawa24_session"

func newTestRouter(t *testing.T) http.Handler {
	t.Helper()

	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	idSvc := identity.NewService(nil, nil, log)
	notifSvc := notifications.NewService(stubRepo{t: t}, log)

	r := chi.NewRouter()
	r.Use(httpx.RequestID)
	r.Use(httpx.Recover(log))
	r.Use(httpx.Locale)

	r.Group(func(protected chi.Router) {
		protected.Use(identityHttp.RequireAuth(idSvc, testCookieName, log))
		notificationsHttp.NewHandler(notifSvc, log).RegisterRoutes(protected)
	})

	return r
}

var protectedRoutes = []struct{ method, path string }{
	{http.MethodGet, "/api/v1/notifications"},
	{http.MethodPost, "/api/v1/notifications/1/read"},
	{http.MethodGet, "/api/v1/notifications/unread-count"},
	{http.MethodGet, "/api/v1/notifications/unread"},
	{http.MethodPost, "/api/v1/notifications/read-all"},
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

	req := httptest.NewRequest(http.MethodGet, "/api/v1/notifications", nil)
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
