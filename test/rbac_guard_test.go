package test

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/muhiya/dawa24-store/internal/platform/rbac"
)

// These tests walk the source the way admin_guard_test.go does. They exist
// because the failure they prevent is invisible: an ungated route behaves
// exactly like a gated one until somebody who should not reach it does, and a
// gate naming a permission nobody can hold behaves exactly like a missing
// feature. Neither shows up in manual testing by an account that holds
// everything — which is how the whole vendor and pharmacy surface came to be
// ungated without anyone noticing.

var (
	reGroup      = regexp.MustCompile(`(?s)r\.Group\(func\(g chi\.Router\) \{(.*?)\n\t\}\)`)
	reGateKeys   = regexp.MustCompile(`Require(?:Tenant)?PagePermission\(([^)]*)\)`)
	reQuoted     = regexp.MustCompile(`"([^"]+)"`)
	reRouteInSrc = regexp.MustCompile(`\bg\.(Get|Post|Put|Delete|Patch)\("([^"]+)"`)
	reBareRoute  = regexp.MustCompile(`\br\.(Get|Post|Put|Delete|Patch)\("(/(?:admin|vendor|customer)/[^"]*)"`)
)

// routeFiles are the registrars whose routes must all be gated on a permission.
var routeFiles = []string{
	"internal/ui/admin_routes_catalog.go",
	"internal/ui/admin_routes_commerce.go",
	"internal/ui/admin_routes_identity.go",
	"internal/ui/admin_routes_org.go",
	"internal/ui/admin_routes_platform.go",
	"internal/ui/vendor_routes.go",
	"internal/ui/vendor_catalog_routes.go",
	"internal/ui/customer_routes.go",
}

// TestEveryRouteGateNamesADeclaredPermission.
//
// A gate on a key the catalogue does not declare cannot be satisfied by any
// role, so the page is unreachable by everyone except the owner — and it fails
// silently, as a 404 that reads like the feature was never built. That is
// exactly what six admin sidebar links did before this catalogue existed.
func TestEveryRouteGateNamesADeclaredPermission(t *testing.T) {
	const root = ".."
	catalog := rbac.Default()

	for _, rel := range routeFiles {
		src, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatalf("reading %s: %v", rel, err)
		}
		for _, m := range reGateKeys.FindAllStringSubmatch(string(src), -1) {
			for _, k := range reQuoted.FindAllStringSubmatch(m[1], -1) {
				if !catalog.Known(k[1]) {
					t.Errorf("%s gates a route on %q, which internal/platform/rbac does not declare; "+
						"no role can hold it, so the route is unreachable", rel, k[1])
				}
			}
		}
	}
}

// TestTenantGatesUseTenantScopedPermissions.
//
// A vendor route gated on an admin permission would be reachable only by
// someone holding a platform grant — which no company member has — and a
// vendor route gated on a pharmacy permission would be unreachable outright.
// Both are silent, and both are one careless copy-paste away.
func TestTenantGatesUseTenantScopedPermissions(t *testing.T) {
	const root = ".."
	catalog := rbac.Default()

	for rel, want := range map[string]rbac.Scope{
		"internal/ui/vendor_routes.go":         rbac.ScopeVendor,
		"internal/ui/vendor_catalog_routes.go": rbac.ScopeVendor,
		"internal/ui/customer_routes.go":       rbac.ScopePharmacy,
	} {
		src, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatalf("reading %s: %v", rel, err)
		}
		for _, m := range regexp.MustCompile(`RequireTenantPagePermission\(([^)]*)\)`).
			FindAllStringSubmatch(string(src), -1) {
			for _, k := range reQuoted.FindAllStringSubmatch(m[1], -1) {
				p, ok := catalog.Lookup(k[1])
				if !ok {
					continue // covered by the test above
				}
				if !p.InScope(want) {
					t.Errorf("%s gates a route on %q, which is not grantable in the %s dashboard",
						rel, k[1], want)
				}
			}
		}
	}

	// The admin registrars must not reach for a tenant permission either: a
	// staff account never holds one, so the page would be dead.
	for _, rel := range routeFiles {
		if strings.Contains(rel, "vendor_") || strings.Contains(rel, "customer_") {
			continue
		}
		src, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatalf("reading %s: %v", rel, err)
		}
		for _, m := range reGateKeys.FindAllStringSubmatch(string(src), -1) {
			for _, k := range reQuoted.FindAllStringSubmatch(m[1], -1) {
				if p, ok := catalog.Lookup(k[1]); ok && !p.InScope(rbac.ScopeAdmin) {
					t.Errorf("%s gates an admin route on %q, which is not an admin permission", rel, k[1])
				}
			}
		}
	}
}

// TestNoDashboardRouteIsRegisteredOutsideAGate.
//
// Every route in these files must sit inside an r.Group whose Use(...) applies
// a permission gate. A route registered on the bare router is reachable by any
// member of the audience — which is what the entire vendor and pharmacy
// surface used to be.
func TestNoDashboardRouteIsRegisteredOutsideAGate(t *testing.T) {
	const root = ".."

	// Routes that are deliberately ungated, with the reason. Anything else
	// registered outside a group fails this test.
	allowed := map[string]string{
		"/vendor/password":          "changing your own password is an account action, not a company one",
		"/customer/password":        "changing your own password is an account action, not a company one",
		"/customer/set-branch":      "choosing which of your own branches you buy for is not a privilege",
		"/customer/branches/active": "choosing which of your own branches you buy for is not a privilege",
		"/admin/dashboard":          "reachable by any authenticated staff member; RequireStaff is the gate",
	}

	for _, rel := range routeFiles {
		src, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatalf("reading %s: %v", rel, err)
		}
		body := string(src)

		// Remove every gated group, then look at what is left.
		outside := reGroup.ReplaceAllString(body, "")
		for _, m := range reBareRoute.FindAllStringSubmatch(outside, -1) {
			if _, ok := allowed[m[2]]; ok {
				continue
			}
			t.Errorf("%s registers %s %s outside any permission group; "+
				"wrap it in r.Group(func(g chi.Router){ g.Use(authctx.Require…PagePermission(\"…\")) … }) "+
				"or add it to the allowed list above with a reason",
				rel, m[1], m[2])
		}

		// And every group that does contain routes must apply a gate.
		for _, g := range reGroup.FindAllStringSubmatch(body, -1) {
			if !reRouteInSrc.MatchString(g[1]) {
				continue
			}
			if !strings.Contains(g[1], "PagePermission(") {
				routes := reRouteInSrc.FindAllStringSubmatch(g[1], -1)
				paths := make([]string, 0, len(routes))
				for _, r := range routes {
					paths = append(paths, r[2])
				}
				t.Errorf("%s has a route group with no permission gate, covering: %s",
					rel, strings.Join(paths, ", "))
			}
		}
	}
}

// TestEverySidebarPermissionGatesARoute.
//
// The other direction: a sidebar item whose permission gates no route is a
// link the caller can see and open regardless of their grants. The check is
// deliberately loose — it asks only that the permission appears in some gate —
// because a section can span several route groups.
func TestEverySidebarPermissionGatesARoute(t *testing.T) {
	const root = ".."

	gated := map[string]bool{}
	for _, rel := range routeFiles {
		src, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatalf("reading %s: %v", rel, err)
		}
		for _, m := range reGateKeys.FindAllStringSubmatch(string(src), -1) {
			for _, k := range reQuoted.FindAllStringSubmatch(m[1], -1) {
				gated[k[1]] = true
			}
		}
	}

	// Items whose destination is a shared or public route, gated elsewhere.
	elsewhere := map[string]bool{
		"vendor.invoice.view":          true, // /invoices is a shared route
		"vendor.compare.use":           true, // /compare/tool is shared with pharmacies
		"vendor.market_discounts.view": true, // /market-discounts is a public page
		"pharmacy.offer.view":          true, // /offers is a public storefront route
		"pharmacy.supplier.view":       true, // /suppliers is a public directory
		"platform.dashboard.view":      true, // /admin/dashboard: any staff member
	}

	var missing []string
	for _, scope := range rbac.Scopes() {
		for _, section := range rbac.Nav(scope) {
			for _, item := range section.Items {
				if gated[item.Perm] || elsewhere[item.Perm] {
					continue
				}
				missing = append(missing, string(scope)+" "+item.Key+" → "+item.Perm)
			}
		}
	}
	sort.Strings(missing)
	for _, m := range missing {
		t.Errorf("sidebar item %s names a permission that gates no route; "+
			"the link would be hidden while the page stayed open to anyone", m)
	}
}
