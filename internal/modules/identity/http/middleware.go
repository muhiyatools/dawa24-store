package http

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/muhiya/dawa24-store/internal/modules/identity"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/httpx"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

type ctxKey int

const (
	ctxKeySession ctxKey = iota
)

// WithSession stores the active session in context.
func WithSession(ctx context.Context, sess *identity.Session) context.Context {
	return context.WithValue(ctx, ctxKeySession, sess)
}

// SessionFrom retrieves the active session from context.
func SessionFrom(ctx context.Context) (*identity.Session, bool) {
	sess, ok := ctx.Value(ctxKeySession).(*identity.Session)
	return sess, ok && sess != nil
}

// RequireAuth authenticates incoming requests via Authorization Bearer token or session cookie.
func RequireAuth(service *identity.Service, cookieName string, log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := extractToken(r, cookieName)
			if token == "" {
				httpx.Error(w, r, log, apperr.Unauthorized())
				return
			}

			sess, err := service.ValidateSession(r.Context(), token)
			if err != nil {
				httpx.Error(w, r, log, apperr.Unauthorized())
				return
			}

			ctx := WithSession(r.Context(), sess)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequirePermission ensures the authenticated user possesses the required permission.
func RequirePermission(permissionKey string, log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sess, ok := SessionFrom(r.Context())
			if !ok {
				httpx.Error(w, r, log, apperr.Unauthorized())
				return
			}

			// Super admins bypass granular permission checks
			if sess.Role == "super_admin" || sess.Role == "developer" {
				next.ServeHTTP(w, r)
				return
			}

			hasPerm := false
			for _, p := range sess.Permissions {
				if p == permissionKey {
					hasPerm = true
					break
				}
			}

			if !hasPerm {
				httpx.Error(w, r, log, apperr.Forbidden(
					"auth.forbidden",
					"You do not have permission to perform this action.",
				))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// ResolveTenant resolves the active tenant organization and binds it to database context.
func ResolveTenant(service *identity.Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			sess, hasSession := SessionFrom(ctx)

			var orgID int64
			// Check X-Dawa-Org-ID header first
			if headerOrg := strings.TrimSpace(r.Header.Get("X-Dawa-Org-ID")); headerOrg != "" {
				if parsed, err := strconv.ParseInt(headerOrg, 10, 64); err == nil && parsed > 0 {
					orgID = parsed
				}
			}

			// Fallback to session active org
			if orgID == 0 && hasSession {
				orgID = sess.ActiveOrgID
			}

			if orgID > 0 {
				ctx = database.WithTenant(ctx, orgID)
			}

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func extractToken(r *http.Request, cookieName string) string {
	// 1. Authorization: Bearer <token>
	if authHeader := r.Header.Get("Authorization"); authHeader != "" {
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) == 2 && strings.EqualFold(parts[0], "bearer") {
			return strings.TrimSpace(parts[1])
		}
	}

	// 2. Cookie
	if cookie, err := r.Cookie(cookieName); err == nil && cookie != nil {
		return strings.TrimSpace(cookie.Value)
	}

	return ""
}
