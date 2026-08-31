package authctx

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/muhiya/dawa24-store/internal/platform/httpx"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// Audience middleware enforces who may mount an HTML route group (Rebuild V2
// §1.3). The API side was already guarded; the HTML side had a single OptionalAuth
// middleware, which let a pharmacy account open every /admin/* and /vendor/*
// page and run database.AsSystem work against other tenants' data.
//
// Every authenticated page route must live inside one of the four groups these
// middlewares protect. A route without an audience must not mount (the guard
// test in test/route_audience_test.go enforces that).
//
// Response policy:
//   - Not signed in            → 302 /auth/login?redirect=<path>  (recoverable)
//   - Wrong account type       → 404  (a vendor must not learn /customer/* exists)
//   - Organization pending     → 302 /onboarding/pending
//   - Organization rejected    → 302 /onboarding/pending?state=rejected
//   - Organization suspended   → 302 /onboarding/pending?state=suspended
//   - Staff route, non-staff   → 404
func RequireCustomer(log *slog.Logger) func(http.Handler) http.Handler {
	return requireType(log, "customer")
}

// RequireVendor gates the /vendor/* group: only members of a vendor
// organization pass; everyone else gets a 404.
func RequireVendor(log *slog.Logger) func(http.Handler) http.Handler {
	return requireType(log, "vendor")
}

// RequireStaff gates /admin/*: only platform staff (super_admin, admin,
// support, developer) pass. A signed-in customer or vendor who spells an
// /admin/ path sees the same 404 as a stranger.
func RequireStaff(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			actor, ok := From(r.Context())
			if !ok {
				redirectToLogin(w, r)
				return
			}
			if !actor.IsStaff {
				notFound(w, r)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequirePagePermission gates an HTML admin page on a permission key. Pass
// several and holding any one of them is enough, for a page two roles reach
// for different reasons.
//
// It answers with the audience policy's 404 rather than a JSON 403: a support
// agent must not learn that /admin/developers or other restricted admin
// subtrees exist.
//
// There is no longer a role-name bypass here. It used to read
// `actor.Role == "super_admin" || actor.Role == "developer"`, which meant the
// developer role could open every admin page regardless of its grants, and
// meant a new staff role could not be given full access without editing this
// function. Both roles now hold their access as permissions like everyone
// else — super_admin holds the whole catalogue, which is the same outcome
// arrived at through the system rather than around it.
func RequirePagePermission(permissionKeys ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			actor, ok := From(r.Context())
			if !ok {
				redirectToLogin(w, r)
				return
			}
			if !actor.IsStaff {
				notFound(w, r)
				return
			}
			if actor.CanAny(permissionKeys...) {
				next.ServeHTTP(w, r)
				return
			}
			denied(r, "admin", actor, permissionKeys)
			notFound(w, r)
		})
	}
}

// RequireTenantPagePermission gates an HTML page inside a vendor or pharmacy
// dashboard. It is the same rule as the admin gate, held against the caller's
// own company: the permissions were resolved from their membership, so a role
// that reveals /vendor/wallet in one company says nothing about another.
//
// The vendor and pharmacy dashboards had no gate of any kind before this. Every
// sidebar link was rendered for every member and every route was reachable by
// URL, so a warehouse clerk could open the wallet, the team page and the role
// editor — the company owner had a roles screen that was static markup and no
// way to stop them.
func RequireTenantPagePermission(permissionKeys ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			actor, ok := From(r.Context())
			if !ok {
				redirectToLogin(w, r)
				return
			}
			// Staff do not hold tenant permissions and have no business on a
			// company dashboard; the audience gate already said so, and this
			// keeps the two answers consistent.
			if actor.IsStaff {
				notFound(w, r)
				return
			}
			if actor.OrganizationID <= 0 && actor.OrgID <= 0 {
				notFound(w, r)
				return
			}
			if actor.CanAny(permissionKeys...) {
				next.ServeHTTP(w, r)
				return
			}
			denied(r, "tenant", actor, permissionKeys)
			notFound(w, r)
		})
	}
}

// RequireAPIPermission gates a JSON API route on platform/staff permission keys.
// Pass several and holding any one of them is enough.
//
// Unlike RequirePagePermission which returns a 404 to hide URL space from
// browsing humans, this returns JSON 403 via httpx.Error + apperr.Forbidden.
// An API client is already authenticated and a misleading 404 would cost
// debuggability for no security gain.
func RequireAPIPermission(permissionKeys ...string) func(http.Handler) http.Handler {
	log := slog.Default()
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			actor, ok := From(r.Context())
			if !ok {
				httpx.Error(w, r, log, apperr.Unauthorized())
				return
			}
			if !actor.IsStaff {
				denied(r, "api_staff", actor, permissionKeys)
				httpx.Error(w, r, log, apperr.Forbidden("auth.forbidden", "Staff permission required"))
				return
			}
			if actor.CanAny(permissionKeys...) {
				next.ServeHTTP(w, r)
				return
			}
			denied(r, "api_staff", actor, permissionKeys)
			httpx.Error(w, r, log, apperr.Forbidden("auth.forbidden", "Permission denied"))
		})
	}
}

// RequireAPITenantPermission gates a JSON API route on tenant permission keys
// within the caller's organization.
//
// Unlike RequireTenantPagePermission which returns a 404 to hide URL space from
// browsing humans, this returns JSON 403 via httpx.Error + apperr.Forbidden.
// An API client is already authenticated and a misleading 404 would cost
// debuggability for no security gain.
func RequireAPITenantPermission(permissionKeys ...string) func(http.Handler) http.Handler {
	log := slog.Default()
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			actor, ok := From(r.Context())
			if !ok {
				httpx.Error(w, r, log, apperr.Unauthorized())
				return
			}
			if actor.IsStaff {
				denied(r, "api_tenant", actor, permissionKeys)
				httpx.Error(w, r, log, apperr.Forbidden("auth.forbidden", "Tenant permission required"))
				return
			}
			if actor.OrganizationID <= 0 && actor.OrgID <= 0 {
				denied(r, "api_tenant", actor, permissionKeys)
				httpx.Error(w, r, log, apperr.Forbidden("auth.forbidden", "Organization membership required"))
				return
			}
			if actor.CanAny(permissionKeys...) {
				next.ServeHTTP(w, r)
				return
			}
			denied(r, "api_tenant", actor, permissionKeys)
			httpx.Error(w, r, log, apperr.Forbidden("auth.forbidden", "Permission denied"))
		})
	}
}

// denied records a refusal. Every gate logs through here so that a permission
// problem in production is one grep, and so the log line always carries the
// caller, the path and what was required.
func denied(r *http.Request, surface string, actor Actor, required []string) {
	slog.WarnContext(r.Context(), "permission denied",
		"surface", surface,
		"path", r.URL.Path,
		"method", r.Method,
		"user_id", actor.UserID,
		"organization_id", actor.OrganizationID,
		"role", actor.Role,
		"scope", string(actor.Scope),
		"required", required)
}

// RequireApproved blocks members whose organization has not been approved.
// Pending organizations are told to wait; rejected and suspended ones are
// sent to the same screen with a state explaining what happened.
// On API routes (/api/*), returns JSON 403 rather than an HTML redirect.
func RequireApproved(log *slog.Logger) func(http.Handler) http.Handler {
	if log == nil {
		log = slog.Default()
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			actor, ok := From(r.Context())
			if strings.HasPrefix(r.URL.Path, "/api/") {
				if !ok {
					httpx.Error(w, r, log, apperr.Unauthorized())
					return
				}
				if actor.IsStaff {
					next.ServeHTTP(w, r)
					return
				}
				switch actor.OrgStatus {
				case "approved", "active", "verified":
					next.ServeHTTP(w, r)
					return
				case "pending", "under_review":
					httpx.Error(w, r, log, apperr.Forbidden("org.pending", "Organization is pending approval."))
					return
				case "rejected":
					httpx.Error(w, r, log, apperr.Forbidden("org.rejected", "Organization has been rejected."))
					return
				case "suspended":
					httpx.Error(w, r, log, apperr.Forbidden("org.suspended", "Organization has been suspended."))
					return
				default:
					httpx.Error(w, r, log, apperr.Forbidden("org.unapproved", "Organization approval required."))
					return
				}
			}

			if !ok {
				redirectToLogin(w, r)
				return
			}
			if actor.IsStaff {
				next.ServeHTTP(w, r)
				return
			}
			switch actor.OrgStatus {
			case "approved", "active", "verified":
				next.ServeHTTP(w, r)
				return
			case "pending", "under_review":
				http.Redirect(w, r, "/onboarding/pending", http.StatusFound)
				return
			case "rejected":
				http.Redirect(w, r, "/onboarding/pending?state=rejected", http.StatusFound)
				return
			case "suspended":
				http.Redirect(w, r, "/onboarding/pending?state=suspended", http.StatusFound)
				return
			default:
				if actor.OrganizationID <= 0 {
					http.Redirect(w, r, "/onboarding", http.StatusFound)
					return
				}
				http.Redirect(w, r, "/onboarding/pending", http.StatusFound)
				return
			}

		})
	}
}

// requireType builds the type-gate middleware. The 404 for a wrong audience is
// deliberate: the URL space of one account type does not exist for the other.
func requireType(log *slog.Logger, want string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			actor, ok := From(r.Context())
			if !ok {
				redirectToLogin(w, r)
				return
			}
			if actor.IsStaff {
				// Staff are not customers or vendors; /customer/* and
				// /vendor/* do not exist for them either.
				notFound(w, r)
				return
			}
			if want == "customer" {
				if actor.IsCustomer() {
					next.ServeHTTP(w, r)
					return
				}
			} else if want == "vendor" {
				if actor.IsVendor() {
					next.ServeHTTP(w, r)
					return
				}
			}
			log.WarnContext(r.Context(), "audience denied",
				"path", r.URL.Path, "user_id", actor.UserID,
				"org_type", actor.OrgType, "want", want)
			notFound(w, r)
		})
	}
}

// redirectToLogin sends the caller to sign in, preserving where they were
// heading so a successful login lands back on the page they wanted.
func redirectToLogin(w http.ResponseWriter, r *http.Request) {
	q := url.Values{}
	q.Set("redirect", r.URL.Path)
	http.Redirect(w, r, "/auth/login?"+q.Encode(), http.StatusSeeOther)
}

// notFound renders a minimal 404. It deliberately does not say whether the
// page exists under another audience.
func notFound(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusNotFound)
	fmt.Fprint(w, notFoundHTML)
}

const notFoundHTML = `<!DOCTYPE html>
<html lang="ar" dir="rtl">
<head>
<meta charset="UTF-8"/>
<meta name="viewport" content="width=device-width, initial-scale=1.0"/>
<title>404 — غير موجود</title>
</head>
<body style="font-family:system-ui,sans-serif;background:#f6f7f9;color:#24292f;display:flex;align-items:center;justify-content:center;min-height:100vh;margin:0;">
<div style="text-align:center;padding:2rem;">
<h1 style="font-size:3rem;margin:0;color:#1f6feb;">404</h1>
<p style="font-size:1.1rem;margin:0.5rem 0;">الصفحة غير موجودة</p>
<p style="color:#6e7781;font-size:0.9rem;margin:0;">Page not found</p>
<a href="/" style="display:inline-block;margin-top:1.25rem;color:#1f6feb;text-decoration:none;font-weight:600;">العودة للرئيسية</a>
</div>
</body>
</html>`
