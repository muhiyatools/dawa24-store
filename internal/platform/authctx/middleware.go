package authctx

import (
	"log/slog"
	"net/http"

	"github.com/muhiya/dawa24-store/internal/platform/httpx"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// RequirePermission ensures the authenticated actor possesses the required permission.
func RequirePermission(permissionKey string, log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			actor, ok := From(r.Context())
			if !ok {
				httpx.Error(w, r, log, apperr.Unauthorized())
				return
			}

			// Super admins bypass granular permission checks
			if actor.Role == "super_admin" || actor.Role == "developer" {
				next.ServeHTTP(w, r)
				return
			}

			if !actor.Can(permissionKey) {
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

// RequireRole ensures the authenticated actor possesses one of the allowed roles.
func RequireRole(allowedRoles []string, log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			actor, ok := From(r.Context())
			if !ok {
				httpx.Error(w, r, log, apperr.Unauthorized())
				return
			}

			if actor.Role == "super_admin" || actor.Role == "developer" {
				next.ServeHTTP(w, r)
				return
			}

			hasRole := false
			for _, r := range allowedRoles {
				if actor.Role == r {
					hasRole = true
					break
				}
			}

			if !hasRole {
				httpx.Error(w, r, log, apperr.Forbidden(
					"auth.forbidden",
					"Your role is not authorized for this resource.",
				))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
