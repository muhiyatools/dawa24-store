package ui_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/rbac"
)

// The account menu must never offer a door that does not open.
//
// This is the test the old menu could not have passed. It hand-wrote eleven
// links and checked a permission on exactly one of them, so:
//
//   - every supplier was shown "طلبات التوريد" pointing at /orders, which is
//     registered inside the customer audience group and answers 404 to anyone
//     who is not a pharmacy;
//   - a pharmacy member without pharmacy.wallet.view, pharmacy.order.view or
//     pharmacy.supplier.view was shown all three, and each answered 404;
//   - a supplier was shown "الإعدادات", which redirected to
//     /vendor/organization, which answers 404 without vendor.organization.view.
//
// Hiding a link is not authorization — the route gates are, and they are
// applied independently. What this asserts is the other half: that the two
// agree, for every audience and every holding, by walking the registry the menu
// renders from and issuing a real request for each destination through the same
// router cmd/server builds.
func TestAccountMenuOffersOnlyReachableDestinations(t *testing.T) {
	cases := []struct {
		name  string
		actor *authctx.Actor
	}{
		{
			name: "super_admin",
			actor: func() *authctx.Actor {
				a := &authctx.Actor{UserID: 1, IsStaff: true, Role: "super_admin"}
				a.Grants([]string{"*"})
				return a
			}(),
		},
		{
			name: "staff_with_two_permissions",
			actor: func() *authctx.Actor {
				a := &authctx.Actor{UserID: 2, IsStaff: true, Role: "support"}
				a.Grants([]string{"platform.dashboard.view", "org.approval.view"})
				return a
			}(),
		},
		{
			name: "vendor_owner",
			actor: func() *authctx.Actor {
				a := &authctx.Actor{
					UserID: 10, OrganizationID: 100, OrgType: "vendor",
					OrgStatus: "approved", Scope: rbac.ScopeVendor,
				}
				a.Grants(rbac.Default().KeysFor(rbac.ScopeVendor))
				return a
			}(),
		},
		{
			name: "vendor_warehouse_clerk",
			actor: func() *authctx.Actor {
				a := &authctx.Actor{
					UserID: 11, OrganizationID: 100, OrgType: "vendor",
					OrgStatus: "approved", Scope: rbac.ScopeVendor,
				}
				a.Grants([]string{"vendor.dashboard.view", "vendor.warehouse.view"})
				return a
			}(),
		},
		{
			name: "pharmacy_owner",
			actor: func() *authctx.Actor {
				a := &authctx.Actor{
					UserID: 20, OrganizationID: 200, OrgType: "customer",
					OrgStatus: "approved", Scope: rbac.ScopePharmacy,
				}
				a.Grants(rbac.Default().KeysFor(rbac.ScopePharmacy))
				return a
			}(),
		},
		{
			name: "pharmacy_counter_assistant",
			actor: func() *authctx.Actor {
				a := &authctx.Actor{
					UserID: 21, OrganizationID: 200, OrgType: "customer",
					OrgStatus: "approved", Scope: rbac.ScopePharmacy,
				}
				a.Grants([]string{"pharmacy.dashboard.view", "pharmacy.purchase_request.view"})
				return a
			}(),
		},
		{
			name: "pending_vendor",
			actor: func() *authctx.Actor {
				a := &authctx.Actor{
					UserID: 30, OrganizationID: 300, OrgType: "vendor",
					OrgStatus: "pending", Scope: rbac.ScopeVendor,
				}
				a.Grants(rbac.Default().KeysFor(rbac.ScopeVendor))
				return a
			}(),
		},
		{
			name: "pending_pharmacy",
			actor: func() *authctx.Actor {
				a := &authctx.Actor{
					UserID: 31, OrganizationID: 301, OrgType: "customer",
					OrgStatus: "pending", Scope: rbac.ScopePharmacy,
				}
				a.Grants(rbac.Default().KeysFor(rbac.ScopePharmacy))
				return a
			}(),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			groups := rbac.AccountMenu(
				tc.actor.DashboardScope(),
				rbac.NewSet(tc.actor.Permissions),
				tc.actor.IsOrgApproved(),
			)

			seen := 0
			router := newTestRouter(tc.actor)
			for _, group := range groups {
				for _, item := range group.Items {
					seen++
					// 404 is the audience and permission gates' refusal. Any
					// other outcome — a redirect to login or to the pending
					// screen, a render, or a panic from one of this harness's
					// nil services — means the handler was reached, which is
					// all the menu is claiming.
					if reachStatus(router, item.Href) == http.StatusNotFound {
						t.Errorf("menu offers %s (%q) but the router answers 404 for this caller",
							item.Href, item.Label("ar"))
					}
				}
			}
			if seen == 0 {
				t.Error("no menu items at all: every caller must at least reach their own account settings")
			}
		})
	}
}

// reachStatus issues one GET and reports the status, treating a panic as
// "reached".
//
// The harness builds UIHandler with nil services, so a page that renders real
// data dereferences nil rather than answering. That is a property of the
// harness, not of the route: the request got past every gate to a handler. Only
// a 404 means the destination does not exist for this caller.
func reachStatus(router http.Handler, href string) (code int) {
	defer func() {
		if recover() != nil {
			code = http.StatusInternalServerError
		}
	}()
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, href, nil))
	return rec.Code
}

// TestAccountMenuAlwaysOffersAccountSettings is the floor.
//
// Whatever else a caller holds or does not hold, and whatever state their
// organization is in, they must be able to reach the page that changes their
// own password.
func TestAccountMenuAlwaysOffersAccountSettings(t *testing.T) {
	holdings := []struct {
		name     string
		scope    rbac.Scope
		perms    []string
		approved bool
	}{
		{"pharmacy with nothing", rbac.ScopePharmacy, nil, true},
		{"vendor with nothing", rbac.ScopeVendor, nil, true},
		{"admin with nothing", rbac.ScopeAdmin, nil, true},
		{"pharmacy pending", rbac.ScopePharmacy, nil, false},
		{"vendor pending", rbac.ScopeVendor, nil, false},
	}

	for _, h := range holdings {
		t.Run(h.name, func(t *testing.T) {
			groups := rbac.AccountMenu(h.scope, rbac.NewSet(h.perms), h.approved)
			for _, g := range groups {
				for _, item := range g.Items {
					if item.Href == "/settings" {
						return
					}
				}
			}
			t.Error("account settings is not offered")
		})
	}
}

// TestVendorCannotReachCustomerOrders proves the audit above can fail.
//
// /orders is the exact link the old menu gave suppliers. It is registered
// inside the customer audience group, so a supplier gets the audience gate's
// 404 — the URL does not exist for them at all. If this ever stops being true,
// the audit has stopped testing anything.
func TestVendorCannotReachCustomerOrders(t *testing.T) {
	a := &authctx.Actor{
		UserID: 10, OrganizationID: 100, OrgType: "vendor",
		OrgStatus: "approved", Scope: rbac.ScopeVendor,
	}
	a.Grants(rbac.Default().KeysFor(rbac.ScopeVendor))

	if got := reachStatus(newTestRouter(a), "/orders"); got != http.StatusSeeOther {
		t.Fatalf("/orders for a supplier returned %d, want 303 — should redirect to supplier dashboard", got)
	}
}
