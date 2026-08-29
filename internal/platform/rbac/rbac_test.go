package rbac_test

import (
	"strings"
	"testing"

	"github.com/muhiya/dawa24-store/internal/platform/rbac"
)

// TestCatalogBuilds is the guard on the declarations themselves: Default()
// panics on a duplicate key, an unknown group, a dangling implication or a
// scope mismatch, so simply reaching it proves the catalogue is coherent.
func TestCatalogBuilds(t *testing.T) {
	c := rbac.Default()
	if len(c.Permissions()) == 0 {
		t.Fatal("catalogue is empty")
	}
	for _, s := range rbac.Scopes() {
		if len(c.PermissionsFor(s)) == 0 {
			t.Errorf("scope %s declares no permissions", s)
		}
		if len(c.Matrix(s)) == 0 {
			t.Errorf("scope %s produces an empty role editor", s)
		}
	}
}

// TestEverySidebarItemHasAGrantablePermission is the rule the old sidebars
// broke. The admin shell gated six links on keys such as
// "notifications.notification.view" that identity.permissions never held, so
// no role could be granted them and the links were invisible to everyone but
// super_admin — which reads as "the feature is missing", not "you lack access".
func TestEverySidebarItemHasAGrantablePermission(t *testing.T) {
	c := rbac.Default()
	for _, scope := range rbac.Scopes() {
		inScope := map[string]bool{}
		for _, p := range c.PermissionsFor(scope) {
			inScope[p.Key] = true
		}
		for _, sec := range rbac.Nav(scope) {
			for _, item := range sec.Items {
				if item.Perm == "" {
					t.Errorf("%s: sidebar item %q declares no permission", scope, item.Key)
					continue
				}
				for _, key := range append([]string{item.Perm}, item.Also...) {
					if !c.Known(key) {
						t.Errorf("%s: sidebar item %q names undeclared permission %q", scope, item.Key, key)
						continue
					}
					if !inScope[key] {
						t.Errorf("%s: sidebar item %q names permission %q, which is not grantable in that scope",
							scope, item.Key, key)
					}
				}
			}
		}
	}
}

// TestNavKeysAreUniqueWithinAScope catches two links claiming one activeNav
// value, which would highlight both and make the active state meaningless.
func TestNavKeysAreUniqueWithinAScope(t *testing.T) {
	for _, scope := range rbac.Scopes() {
		seen := map[string]string{}
		for _, sec := range rbac.Nav(scope) {
			for _, item := range sec.Items {
				if prev, dup := seen[item.Key]; dup {
					t.Errorf("%s: nav key %q claimed by both %q and %q", scope, item.Key, prev, sec.Key)
				}
				seen[item.Key] = sec.Key
			}
		}
	}
}

// TestRestrictIsTheCompanyBoundary. A vendor owner who posts a role form
// naming platform permissions must get a role without them. Filtering the
// checkboxes shown in the editor is a convenience; this is the control.
func TestRestrictIsTheCompanyBoundary(t *testing.T) {
	c := rbac.Default()
	forged := []string{
		"vendor.order.view",       // legitimately theirs
		"platform.setting.update", // another dashboard entirely
		"identity.user.delete",    // platform staff only
		"pharmacy.order.create",   // a different tenant dashboard
		"catalog.product.delete",  // admin catalogue, not vendor items
		"totally.made.up",         // not declared at all
	}
	got := c.Restrict(forged, rbac.ScopeVendor)
	for _, k := range got {
		if !strings.HasPrefix(k, "vendor.") {
			t.Errorf("Restrict leaked %q into the vendor scope", k)
		}
	}
	if !contains(got, "vendor.order.view") {
		t.Error("Restrict dropped a permission the vendor legitimately holds")
	}
}

// TestActionsImplyTheirPage. A role that may edit a page it cannot open is a
// role whose owner will file a bug, so an action expands to its page.
func TestActionsImplyTheirPage(t *testing.T) {
	c := rbac.Default()
	for _, scope := range rbac.Scopes() {
		for _, p := range c.PermissionsFor(scope) {
			if p.Kind != rbac.KindAction || len(p.Implies) == 0 {
				continue
			}
			expanded := c.Expand([]string{p.Key})
			for _, want := range p.Implies {
				if !contains(expanded, want) {
					t.Errorf("%s: %q does not expand to its implied %q", scope, p.Key, want)
				}
			}
		}
	}
}

// TestOwnerHoldsEverythingInScopeAndNothingOutsideIt.
func TestOwnerHoldsEverythingInScopeAndNothingOutsideIt(t *testing.T) {
	c := rbac.Default()
	owner, ok := rbac.OrganizationRole("org_owner")
	if !ok {
		t.Fatal("org_owner is not declared")
	}
	for _, scope := range []rbac.Scope{rbac.ScopeVendor, rbac.ScopePharmacy} {
		grants := rbac.GrantsFor(owner, scope)
		if len(grants) != len(c.KeysFor(scope)) {
			t.Errorf("%s: owner holds %d of %d permissions", scope, len(grants), len(c.KeysFor(scope)))
		}
		set := rbac.NewSet(grants)
		other := rbac.ScopeAdmin
		for _, k := range c.KeysFor(other) {
			if p, _ := c.Lookup(k); p.InScope(scope) {
				continue // legitimately shared
			}
			if set.Has(k) {
				t.Errorf("%s owner holds out-of-scope permission %q", scope, k)
			}
		}
	}
}

// TestSetMatching covers the wildcard rules the middleware relies on.
func TestSetMatching(t *testing.T) {
	s := rbac.NewSet([]string{"vendor.order.view", "catalog.*"})
	cases := []struct {
		want string
		ok   bool
	}{
		{"vendor.order.view", true},
		{"vendor.order.update", false},
		{"catalog.product.view", true},
		{"catalog.brand.delete", true},
		{"catalogue.product.view", false}, // prefix must stop at the dot
		{"commerce.order.view", false},
	}
	for _, tc := range cases {
		if got := s.Has(tc.want); got != tc.ok {
			t.Errorf("Has(%q) = %v, want %v", tc.want, got, tc.ok)
		}
	}

	all := rbac.NewSet([]string{rbac.Wildcard})
	if !all.Has("anything.at.all") {
		t.Error("the wildcard grant does not satisfy an arbitrary permission")
	}
	empty := rbac.NewSet(nil)
	if empty.Has("vendor.order.view") {
		t.Error("an empty holding satisfied a requirement")
	}
	if !empty.Has("") {
		t.Error("an ungated requirement was denied")
	}
}

// TestSystemRoleGrantsAreDeclared. A starter role naming a permission that no
// longer exists would seed a company with dead grants.
func TestSystemRoleGrantsAreDeclared(t *testing.T) {
	c := rbac.Default()
	for _, r := range rbac.PlatformRoles() {
		for _, k := range r.Permissions {
			if !c.Known(k) {
				t.Errorf("platform role %q grants undeclared permission %q", r.Key, k)
			}
		}
	}
	for _, r := range rbac.OrganizationRoles() {
		for _, scope := range []rbac.Scope{rbac.ScopeVendor, rbac.ScopePharmacy} {
			for _, k := range rbac.GrantsFor(r, scope) {
				if !c.Known(k) {
					t.Errorf("org role %q in %s grants undeclared permission %q", r.Key, scope, k)
				}
			}
		}
	}
}

func contains(hay []string, needle string) bool {
	for _, h := range hay {
		if h == needle {
			return true
		}
	}
	return false
}
