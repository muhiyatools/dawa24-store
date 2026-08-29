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
	OrgID          int64  // Alias for OrganizationID
	OrgType        string // "customer" | "vendor" | "" for a user with no organization or staff-only
	OrgStatus      string // pending | approved | rejected | suspended
	BranchID       *int64 // non-nil when the member is bound to one branch
	Role           string // platform role
	Permissions    []string
	IsStaff        bool
	Email          string
	Name           string
}

// IsPlatformAdmin reports whether the actor has super_admin or admin role.
func (a Actor) IsPlatformAdmin() bool {
	return a.Role == "super_admin" || a.Role == "admin"
}

// IsCustomer reports whether the actor belongs to a customer (صيدلية) tenant.
// An organization member with no resolved type is treated as a customer so a
// pending organization cannot reach vendor surfaces by omission.
func (a Actor) IsCustomer() bool {
	return !a.IsStaff && a.OrgType != "vendor" && a.OrgType != "supplier"
}

// IsVendor reports whether the actor belongs to a vendor (مورّد) tenant.
func (a Actor) IsVendor() bool {
	return !a.IsStaff && (a.OrgType == "vendor" || a.OrgType == "supplier")
}

// IsJobSeeker reports whether the actor has the job_seeker platform role.
func (a Actor) IsJobSeeker() bool {
	return a.Role == "job_seeker"
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
	if a.IsStaff {
		return "مدير المنصة"
	}
	if a.Role == "job_seeker" {
		return "باحث عن عمل"
	}
	if a.OrgType == "vendor" {
		return "مورّد أدوية"
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

// BranchOption names one of the customer's own branches in the shell selector.
type BranchOption struct {
	ID   int64
	Name string // localized display name
}

// BuyingBranch is the branch the customer is buying for: one of their own
// branches, chosen in the customer shell and persisted per browser. The
// option list always comes from the database; only the selection rides the
// cookie, and handlers resolve coordinates from the branch record.
type BuyingBranch struct {
	Branches []BranchOption
	Active   *int64
}

type ctxKeyBuyingBranch struct{}

// WithBuyingBranch binds the customer's branch selection to the context.
func WithBuyingBranch(ctx context.Context, b BuyingBranch) context.Context {
	return context.WithValue(ctx, ctxKeyBuyingBranch{}, b)
}

// BuyingBranchFrom returns the branch selection bound for this request.
func BuyingBranchFrom(ctx context.Context) (BuyingBranch, bool) {
	b, ok := ctx.Value(ctxKeyBuyingBranch{}).(BuyingBranch)
	return b, ok
}

// From returns the authenticated caller, if any.
func From(ctx context.Context) (Actor, bool) {
	a, ok := ctx.Value(ctxKeyActor).(Actor)
	if ok && a.OrgID == 0 && a.OrganizationID > 0 {
		a.OrgID = a.OrganizationID
	}
	if ok && a.OrganizationID == 0 && a.OrgID > 0 {
		a.OrganizationID = a.OrgID
	}
	return a, ok && a.UserID > 0
}

// FromContext returns the authenticated caller from context, or an empty Actor.
func FromContext(ctx context.Context) Actor {
	a, _ := From(ctx)
	return a
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
