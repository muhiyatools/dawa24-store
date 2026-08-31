package http

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/identity"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/httpx"
	"github.com/muhiya/dawa24-store/internal/platform/rbac"
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
func RequireAuth(service *identity.Service, resolver *rbac.Resolver, cookieName string, log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := extractToken(r, cookieName)
			if token == "" {
				if r.Method == http.MethodGet && strings.Contains(r.Header.Get("Accept"), "text/html") {
					q := url.Values{}
					q.Set("redirect", r.URL.RequestURI())
					http.Redirect(w, r, "/auth/login?"+q.Encode(), http.StatusSeeOther)
					return
				}
				httpx.Error(w, r, log, apperr.Unauthorized())
				return
			}

			sess, err := service.ValidateSession(r.Context(), token)
			if err != nil {
				// Expired/invalid session: clear cookie
				http.SetCookie(w, &http.Cookie{
					Name:     cookieName,
					Value:    "",
					Path:     "/",
					MaxAge:   -1,
					Expires:  time.Unix(0, 0),
					HttpOnly: true,
					SameSite: http.SameSiteLaxMode,
				})

				if r.Method == http.MethodGet && strings.Contains(r.Header.Get("Accept"), "text/html") {
					q := url.Values{}
					q.Set("redirect", r.URL.RequestURI())
					var appErr *apperr.Error
					if errors.Is(err, identity.ErrSessionIdleTimeout) || (errors.As(err, &appErr) && appErr.Code == "session.idle_timeout") {
						q.Set("reason", "idle_timeout")
					} else if errors.Is(err, identity.ErrSessionEvictedConcurrentLimit) || (errors.As(err, &appErr) && appErr.Code == "session.evicted_concurrent_limit") {
						q.Set("error", "concurrent_limit")
					}
					http.Redirect(w, r, "/auth/login?"+q.Encode(), http.StatusSeeOther)
					return
				}
				httpx.Error(w, r, log, apperr.Unauthorized())
				return
			}

			// The session's active organization is the trusted default. A zero
			// org means the user has no organization membership (or is staff),
			// and no query then runs under a guessed tenant — the previous
			// hardcoded fallback to organization 1 handed a pharmacy account
			// another tenant's data via RLS scoping.
			activeOrgID := sess.ActiveOrgID

			ctx := WithSession(r.Context(), sess)
			if activeOrgID > 0 {
				ctx = database.WithTenant(ctx, activeOrgID)
			}
			// Publish the caller through the platform-level actor context so
			// other modules can identify them without importing this one, and
			// without trusting anything in the request.
			//
			// OrgType and OrgStatus come from the session, which login built
			// from org.members joined to org.organizations — never from the
			// request (Rebuild V2 §1.2).
			ctx = authctx.WithActor(ctx, actorFor(ctx, resolver, sess, activeOrgID, log))
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// actorFor builds the authenticated caller.
//
// Permissions come from the resolver, which reads them now, rather than from
// the copy the session was stamped with at login. That copy was the whole
// problem: sessions last 720 hours, so revoking a role had no effect on anyone
// already signed in until they happened to log out. A permission system that
// cannot revoke within the session lifetime is not enforcing anything.
//
// If the resolver is unavailable — it is not wired in tests, and the database
// can be down — the caller falls back to the session's copy. That is the
// conservative choice in the wrong direction only for revocation; the
// alternative, denying everything, turns a database blip into a total outage
// for signed-in users.
func actorFor(ctx context.Context, resolver *rbac.Resolver, sess *identity.Session, orgID int64, log *slog.Logger) authctx.Actor {
	actor := authctx.Actor{
		UserID:         sess.UserID,
		OrganizationID: orgID,
		OrgType:        sess.OrgType,
		OrgStatus:      sess.OrgStatus,
		Role:           sess.Role,
		IsStaff:        sess.IsStaff(),
		Email:          sess.Email,
	}
	actor.Grants(sess.Permissions)

	if resolver == nil {
		if scope, ok := rbac.TenantScopeFor(sess.OrgType); ok {
			actor.Scope = scope
		} else if actor.IsStaff {
			actor.Scope = rbac.ScopeAdmin
		}
		return actor
	}

	grant, err := resolver.Resolve(ctx, sess.UserID, orgID)
	if err != nil {
		log.ErrorContext(ctx, "could not resolve permissions; falling back to the session copy",
			"error", err, "user_id", sess.UserID, "organization_id", orgID)
		return actor
	}
	actor.IsStaff = grant.IsStaff
	actor.IsOwner = grant.IsPlatformOwner || grant.IsOrgOwner
	actor.Scope = grant.Scope
	if grant.PlatformRole != "" {
		actor.Role = grant.PlatformRole
	}
	if grant.OrgType != "" {
		actor.OrgType = grant.OrgType
	}
	if grant.OrgStatus != "" {
		actor.OrgStatus = grant.OrgStatus
	}
	actor.Grants(grant.Keys)
	return actor
}

// OptionalAuth authenticates incoming requests if a valid session exists, but allows anonymous requests.
func OptionalAuth(service *identity.Service, resolver *rbac.Resolver, cookieName string, log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := extractToken(r, cookieName)
			if token == "" {
				next.ServeHTTP(w, r)
				return
			}

			sess, err := service.ValidateSession(r.Context(), token)
			if err != nil {
				// Expired or invalid session: clear cookie so browser stops sending dead token
				http.SetCookie(w, &http.Cookie{
					Name:     cookieName,
					Value:    "",
					Path:     "/",
					MaxAge:   -1,
					Expires:  time.Unix(0, 0),
					HttpOnly: true,
					SameSite: http.SameSiteLaxMode,
				})
				next.ServeHTTP(w, r)
				return
			}

			// Same rule as RequireAuth: the session's active organization is
			// the only tenant default; there is no guessed fallback.
			activeOrgID := sess.ActiveOrgID

			ctx := WithSession(r.Context(), sess)
			if activeOrgID > 0 {
				ctx = database.WithTenant(ctx, activeOrgID)
			}
			ctx = authctx.WithActor(ctx, actorFor(ctx, resolver, sess, activeOrgID, log))
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// ResolveTenant resolves the active tenant organization and binds it to database context.
func ResolveTenant(service *identity.Service, log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			sess, hasSession := SessionFrom(ctx)

			// The session's active organization is the trusted default.
			var orgID int64
			if hasSession {
				orgID = sess.ActiveOrgID
			}

			// X-Dawa-Org-ID lets a user who belongs to several organizations
			// switch between them. It is a request-supplied value, so it is
			// honoured only after confirming membership. Trusting it directly
			// would hand any caller another tenant's data, because row-level
			// security scopes to whatever organization it is told.
			if headerOrg := strings.TrimSpace(r.Header.Get("X-Dawa-Org-ID")); headerOrg != "" {
				requested, err := strconv.ParseInt(headerOrg, 10, 64)
				switch {
				case err != nil || requested <= 0:
					httpx.Error(w, r, log, apperr.Validation("org.invalid",
						"X-Dawa-Org-ID must be a positive integer.", nil))
					return
				case !hasSession:
					httpx.Error(w, r, log, apperr.Unauthorized())
					return
				case requested != orgID:
					newType, newStatus, newPerms, err := service.GetOrgInfoForUser(ctx, sess.UserID, requested)
					if err != nil {
						httpx.Error(w, r, log, err)
						return
					}
					orgID = requested
					if actor, ok := authctx.From(ctx); ok {
						actor.OrganizationID = orgID
						actor.OrgType = newType
						actor.OrgStatus = newStatus
						actor.Permissions = newPerms
						ctx = authctx.WithActor(ctx, actor)
					}
				}
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
