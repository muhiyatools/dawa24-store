package tools_test

import (
	"context"
	"encoding/json"
	"github.com/muhiya/dawa24-store/internal/modules/assistant"
	"github.com/muhiya/dawa24-store/internal/modules/assistant/handles"
	"github.com/muhiya/dawa24-store/internal/modules/assistant/tools"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Arguments
// ---------------------------------------------------------------------------

// No tool may accept an identity parameter. This walks the whole registry, so
// a tool added later inherits the rule rather than needing its own test.
func TestNoToolAcceptsAnIdentityArgument(t *testing.T) {
	f := newFixture(t)
	banned := []string{
		"org_id", "organization_id", "org", "organisation_id", "tenant", "tenant_id",
		"user_id", "user", "customer_id", "owner_id", "branch_id", "account_id",
		"is_admin", "is_staff", "role", "scope", "permission", "permissions",
	}

	for _, name := range f.reg.Names() {
		tool, ok := f.reg.Lookup(name)
		if !ok {
			t.Fatalf("declared tool %s cannot be looked up", name)
		}
		props, _ := tool.Params["properties"].(map[string]any)
		for field := range props {
			lower := strings.ToLower(field)
			for _, bad := range banned {
				if lower == bad {
					t.Errorf("tool %s accepts identity argument %q", name, field)
				}
			}
		}
		// additionalProperties:false is what turns a smuggled field into a
		// visible refusal instead of a silently dropped one.
		if extra, ok := tool.Params["additionalProperties"].(bool); !ok || extra {
			t.Errorf("tool %s does not forbid additional properties", name)
		}
	}
}

// A smuggled argument is refused rather than ignored.
func TestUnknownArgumentsAreRefused(t *testing.T) {
	f := newFixture(t)
	ph := pharmacist(1, 10)

	bad := []string{
		`{"organization_id": 99}`,
		`{"limit": 5, "org_id": 2}`,
		`{"status":"pending","__proto__":{"admin":true}}`,
		`{"limit": 5}{"limit": 9}`,
	}
	for _, args := range bad {
		out := f.reg.Dispatch(context.Background(), ph, 0, call("orders_list", args))
		if out.Decision == string(tools.DecisionAllowed) {
			t.Fatalf("accepted smuggled arguments: %s", args)
		}
	}
	if len(f.reader.calls) != 0 {
		t.Fatalf("invalid arguments reached the reader: %v", f.reader.calls)
	}
}

// Out-of-range and nonsense values are refused, not clamped into a confidently
// wrong answer.
func TestArgumentValidation(t *testing.T) {
	f := newFixture(t)
	ph := pharmacist(1, 10)

	cases := map[string]string{
		"negative offset":  `{"offset": -5}`,
		"absurd offset":    `{"offset": 500000}`,
		"unknown status":   `{"status": "everything"}`,
		"bad date":         `{"from": "last tuesday"}`,
		"reversed period":  `{"from": "2026-06-01", "to": "2026-01-01"}`,
		"oversized search": `{"search": "` + strings.Repeat("ا", 400) + `"}`,
	}
	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			out := f.reg.Dispatch(context.Background(), ph, 0, call("orders_list", args))
			if out.Decision == string(tools.DecisionAllowed) {
				t.Fatalf("accepted %s", name)
			}
		})
	}
}

// A model that asks for a thousand rows gets a page.
func TestPageSizeIsCapped(t *testing.T) {
	f := newFixture(t)
	ph := pharmacist(1, 10)

	out := f.reg.Dispatch(context.Background(), ph, 0, call("orders_list", `{"limit": 5000}`))
	// The schema caps it, so an out-of-range limit is a refusal rather than a
	// silently shrunk request; either way the reader never sees 5000.
	if out.Decision == string(tools.DecisionAllowed) {
		var payload struct {
			Data struct {
				Count int `json:"count"`
			} `json:"data"`
		}
		_ = json.Unmarshal([]byte(out.Content), &payload)
		if payload.Data.Count > assistant.PageLimit {
			t.Fatalf("returned %d rows, cap is %d", payload.Data.Count, assistant.PageLimit)
		}
	}
}

// ---------------------------------------------------------------------------
// Unknown tools and injection
// ---------------------------------------------------------------------------

// An invented tool name must be refused outright, never resolved to the
// nearest real one.
func TestUnknownToolsAreRefused(t *testing.T) {
	f := newFixture(t)
	ph := pharmacist(1, 10)

	for _, name := range []string{
		"", "sql", "execute_sql", "orders_list_all", "ORDERS_LIST",
		"admin_orders_list", "../orders_list", "orders_list ",
	} {
		out := f.reg.Dispatch(context.Background(), ph, 0, call(name, "{}"))
		if out.Decision == string(tools.DecisionAllowed) && strings.TrimSpace(name) != "orders_list" {
			t.Fatalf("invented tool %q was allowed", name)
		}
	}
}

// A refusal must not tell the model anything it can act on: no SQL, no column
// names, no tenant identifiers.
func TestRefusalsLeakNothing(t *testing.T) {
	f := newFixture(t)
	attacker := pharmacist(2, 20)
	stolen := f.signer.Issue(handles.KindOrder, 501, handles.Binding{OrgID: 1, UserID: 10})
	args, _ := json.Marshal(map[string]string{"order": stolen})

	out := f.reg.Dispatch(context.Background(), attacker, 0, call("order_details", string(args)))
	lower := strings.ToLower(out.Content)
	for _, leak := range []string{"select", "commerce.", "organization_id", "pgx", "sql", "501"} {
		if strings.Contains(lower, leak) {
			t.Fatalf("refusal leaked %q: %s", leak, out.Content)
		}
	}
}

// Every decision, allowed or not, is recorded.
func TestEveryDecisionIsAudited(t *testing.T) {
	f := newFixture(t)
	ph := pharmacist(1, 10)

	f.reg.Dispatch(context.Background(), ph, 77, call("branches_list", "{}"))
	f.reg.Dispatch(context.Background(), ph, 77, call("sales_summary", "{}"))
	f.reg.Dispatch(context.Background(), ph, 77, call("nope", "{}"))

	if len(f.audit.entries) != 3 {
		t.Fatalf("audited %d calls, want 3", len(f.audit.entries))
	}
	for _, e := range f.audit.entries {
		if e.TurnID != 77 || e.UserID != ph.UserID {
			t.Fatalf("audit entry not attributed: %+v", e)
		}
	}
}

// ---------------------------------------------------------------------------
// Read-only
// ---------------------------------------------------------------------------

// The assistant must offer no way to change anything. This reads the registry
// rather than the source, so a mutating tool added later fails here.
func TestNoToolNameSuggestsAWrite(t *testing.T) {
	f := newFixture(t)
	forbidden := []string{
		"create", "update", "delete", "remove", "cancel", "place", "submit",
		"pay", "refund", "confirm", "approve", "reject", "set_", "add_", "edit",
	}
	for _, name := range f.reg.Names() {
		lower := strings.ToLower(name)
		for _, verb := range forbidden {
			if strings.Contains(lower, verb) {
				t.Errorf("tool %q reads as a mutation; the assistant is read-only", name)
			}
		}
	}
}
