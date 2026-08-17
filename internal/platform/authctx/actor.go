// Package authctx carries the authenticated caller through the request context.
//
// It exists so that a module handler can ask "who is calling?" without importing
// the identity module — modules must not depend on each other (see ADR 0002 and
// the depguard rules), and platform packages are the shared ground they are all
// allowed to reach.
//
// The rule this package makes enforceable: the acting user is whoever
// authentication proved them to be, never whoever the request said they were.
// Handlers previously read `?user_id=` from the query string, which let any
// caller act as any user simply by changing a number.
package authctx

import (
	"context"

	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

type ctxKey int

const ctxKeyActor ctxKey = iota

// Actor is the authenticated caller.
type Actor struct {
	UserID         int64
	OrganizationID int64
	Role           string
	Permissions    []string
	Email          string
	Name           string
}

// DisplayName returns a user-friendly name to display in the navbar.
func (a Actor) DisplayName() string {
	if a.Name != "" {
		return a.Name
	}
	if a.Email != "" {
		for i, r := range a.Email {
			if r == '@' {
				return a.Email[:i]
			}
		}
		return a.Email
	}
	if a.Role == "pharmacy" {
		return "صيدلي معتمد"
	}
	if a.Role == "supplier" || a.Role == "vendor" {
		return "مورّد أدوية"
	}
	if a.Role == "admin" || a.Role == "super_admin" {
		return "مدير المنصة"
	}
	return "صيدلي معتمد"
}

// Can reports whether the actor holds a permission.
func (a Actor) Can(permission string) bool {
	for _, p := range a.Permissions {
		if p == permission {
			return true
		}
	}
	return false
}

// WithActor binds the authenticated caller to the context. Only authentication
// middleware should call this.
func WithActor(ctx context.Context, a Actor) context.Context {
	return context.WithValue(ctx, ctxKeyActor, a)
}

// From returns the authenticated caller, if any.
func From(ctx context.Context) (Actor, bool) {
	a, ok := ctx.Value(ctxKeyActor).(Actor)
	return a, ok && a.UserID > 0
}

// UserID returns the authenticated user id, or an Unauthorized error.
//
// Handlers use this instead of parsing an identifier from the request. It
// cannot be spoofed, because nothing outside authentication middleware can put
// an actor into the context.
func UserID(ctx context.Context) (int64, error) {
	a, ok := From(ctx)
	if !ok {
		return 0, apperr.Unauthorized()
	}
	return a.UserID, nil
}

// SameUserOrForbidden guards access to another user's data.
//
// Pass the id the request is trying to reach. It is permitted only when it is
// the caller's own id, or when the caller holds the override permission — which
// is how support staff legitimately read someone else's record.
func SameUserOrForbidden(ctx context.Context, targetUserID int64, overridePermission string) error {
	a, ok := From(ctx)
	if !ok {
		return apperr.Unauthorized()
	}
	if a.UserID == targetUserID {
		return nil
	}
	if overridePermission != "" && a.Can(overridePermission) {
		return nil
	}
	return apperr.Forbidden("actor.not_owner",
		"You do not have access to another user's data.")
}

// SameOrgOrForbidden guards access to another tenant's data.
//
// Pass the organization id taken from the route. Authentication establishes who
// is calling; it says nothing about which tenant they may act on. A handler that
// reads {id} from the URL and passes it straight to a service lets any logged-in
// user address any organization, which is how PUT /org/organizations/{id} came
// to accept another tenant's id.
//
// This matters more here than it looks: the application connects to PostgreSQL
// as a superuser, so row-level security is inert and this check is the only
// thing enforcing the boundary.
func SameOrgOrForbidden(ctx context.Context, targetOrgID int64, overridePermission string) error {
	a, ok := From(ctx)
	if !ok {
		return apperr.Unauthorized()
	}
	if a.OrganizationID == targetOrgID {
		return nil
	}
	// Platform staff legitimately act across tenants; they hold the permission.
	if overridePermission != "" && a.Can(overridePermission) {
		return nil
	}
	return apperr.Forbidden("actor.wrong_tenant",
		"You do not have access to another organization's data.")
}
