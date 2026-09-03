package ui_test

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// Routes that are deliberately registered without a permission gate, with the
// reason. Everything else in the dashboard route files must sit inside a
// chi group whose Use() names a Require*Permission guard — otherwise the page
// is reachable by typing its URL, whatever the sidebar shows.
//
// Adding an entry here is a decision, not a formality: say why the route needs
// no permission.
var routesWithoutPermissionGate = map[string]string{
	// Redirects to a page that is itself gated.
	"GET /vendor/session":            "301 to /vendor/sessions, which is gated",
	"GET /vendor/notifications":      "301 to /notifications, which is gated",
	"GET /customer/session":          "301 to /customer/sessions, which is gated",
	"GET /customer/notifications":    "301 to /notifications, which is gated",

	// Account actions, not company actions: a member locked out of every page
	// must still be able to secure their own credentials and pick the branch
	// they are buying for.
	"POST /vendor/password":           "changing your own password is an account action",
	"POST /customer/password":         "changing your own password is an account action",
	"POST /customer/set-branch":       "choosing your own buying branch is an account action",
	"POST /customer/branches/active":  "choosing your own active branch is an account action",
}

var (
	guardRe = regexp.MustCompile(`\bRequire[A-Za-z0-9_]*\(`)
	routeRe = regexp.MustCompile(`\b[a-zA-Z_][a-zA-Z0-9_]*\.(Get|Post|Put|Delete|Patch|Head)\(\s*"([^"]*)"`)
)

// TestEveryDashboardRouteSitsBehindAPermission walks the admin, vendor and
// pharmacy route tables and fails on any registration that is not inside a
// permission-guarded group.
//
// This is the direct-URL guarantee. A sidebar that hides a link proves nothing;
// only a middleware on the route does. The check is textual because that is
// what it is protecting: the shape of the route file, at the moment somebody
// adds a line to it.
func TestEveryDashboardRouteSitsBehindAPermission(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("..", "..", "internal", "ui", "*routes*.go"))
	if err != nil {
		t.Fatalf("glob route files: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("no route files found — has internal/ui moved?")
	}

	var ungated []string
	checked := 0
	for _, f := range files {
		base := filepath.Base(f)
		// public_routes.go is the signed-out surface by definition; its
		// handlers carry their own checks and audience_separation_test.go
		// covers them.
		if base == "public_routes.go" {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		var stack []map[string]bool
		for _, line := range strings.Split(string(src), "\n") {
			if guardRe.MatchString(line) && len(stack) > 0 {
				stack[len(stack)-1]["guard"] = true
			}
			if m := routeRe.FindStringSubmatch(line); m != nil && !strings.Contains(line, ".Use(") {
				checked++
				guarded := false
				for _, frame := range stack {
					if frame["guard"] {
						guarded = true
						break
					}
				}
				key := strings.ToUpper(m[1]) + " " + m[2]
				if !guarded {
					if _, allowed := routesWithoutPermissionGate[key]; !allowed {
						ungated = append(ungated, key+"   ("+base+")")
					}
				}
			}
			opens := strings.Count(line, "{") - strings.Count(line, "}")
			for i := 0; i < opens; i++ {
				stack = append(stack, map[string]bool{})
			}
			for i := 0; i > opens && len(stack) > 0; i-- {
				stack = stack[:len(stack)-1]
			}
		}
	}

	if checked == 0 {
		t.Fatal("parsed no routes at all — the route-file shape changed and this gate is not checking anything")
	}
	if len(ungated) > 0 {
		sort.Strings(ungated)
		t.Fatalf("%d dashboard route(s) are reachable without a permission check.\n"+
			"Put each inside a chi group whose Use() names a Require*Permission guard,\n"+
			"or add it to routesWithoutPermissionGate with the reason:\n  %s",
			len(ungated), strings.Join(ungated, "\n  "))
	}
	t.Logf("checked %d dashboard routes; all behind a permission gate", checked)
}
