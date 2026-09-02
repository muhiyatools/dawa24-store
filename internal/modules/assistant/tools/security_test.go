package tools_test

import (
	"context"
	"encoding/json"
	"github.com/muhiya/dawa24-store/internal/modules/assistant"
	"github.com/muhiya/dawa24-store/internal/modules/assistant/handles"
	"github.com/muhiya/dawa24-store/internal/modules/assistant/tools"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/rbac"
	"testing"
)

// The hostile-model suite.
//
// Every test here scripts a tool call the way a compromised or manipulated
// model would emit one — a foreign handle, a tool from another dashboard, an
// argument nobody declared — and asserts that dispatch refuses it before any
// query runs. The reader is instrumented, so "no query ran" is a fact the test
// can check rather than an inference.
//
// What this suite is really asserting is that authorization consults nothing
// the model produced except an opaque handle whose signature binds it to the
// caller. If that ever stops being true, one of these fails.

const testSecret = "assistant-tools-test-secret-value-32ch"

// ---------------------------------------------------------------------------
// Instrumented reader
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// The gate: the owner's per-role switch
// ---------------------------------------------------------------------------

// A user who holds every dashboard permission but not the assistant grant gets
// nothing. This is the switch a pharmacy owner flips per employee role.
func TestAssistantGateIsRequired(t *testing.T) {
	f := newFixture(t)
	ungated := actor(rbac.ScopePharmacy, 1, 10, "pharmacy.order.view")

	if got := f.reg.Schemas(ungated); len(got) != 0 {
		t.Fatalf("ungated user was shown %d tools", len(got))
	}

	out := f.reg.Dispatch(context.Background(), ungated, 0, call("orders_list", "{}"))
	if out.Decision != string(tools.DecisionGate) {
		t.Fatalf("decision = %q, want denied_gate", out.Decision)
	}
	if len(f.reader.calls) != 0 {
		t.Fatalf("a denied call still read data: %v", f.reader.calls)
	}
}

// Holding the gate alone must not widen access: it admits the assistant, and
// each tool still asks for the permission its screen asks for.
func TestGateAloneGrantsNoData(t *testing.T) {
	f := newFixture(t)
	gateOnly := actor(rbac.ScopePharmacy, 1, 10, assistant.GatePharmacy)

	out := f.reg.Dispatch(context.Background(), gateOnly, 0, call("orders_list", "{}"))
	if out.Decision != string(tools.DecisionPermission) {
		t.Fatalf("decision = %q, want denied_permission", out.Decision)
	}
	if len(f.reader.calls) != 0 {
		t.Fatalf("a denied call still read data: %v", f.reader.calls)
	}
	if f.audit.last().Decision != string(tools.DecisionPermission) {
		t.Fatal("refusal was not audited")
	}
}

// ---------------------------------------------------------------------------
// Cross-role
// ---------------------------------------------------------------------------

// A pharmacy user must not reach a vendor tool or an admin tool, however the
// model spells the request.
func TestCrossRoleToolsAreRefused(t *testing.T) {
	f := newFixture(t)
	ph := pharmacist(1, 10)

	for _, name := range []string{"sales_summary", "my_products", "low_stock",
		"organizations_search", "platform_overview", "ai_usage_summary"} {
		t.Run(name, func(t *testing.T) {
			before := len(f.reader.calls)
			out := f.reg.Dispatch(context.Background(), ph, 0, call(name, "{}"))
			if out.Decision == string(tools.DecisionAllowed) {
				t.Fatalf("pharmacy user was allowed %s", name)
			}
			if len(f.reader.calls) != before {
				t.Fatalf("refused %s still read data", name)
			}
		})
	}
}

func TestVendorCannotReachPharmacyTools(t *testing.T) {
	f := newFixture(t)
	v := vendor(2, 20)

	for _, name := range []string{"orders_list", "spend_summary", "market_search"} {
		t.Run(name, func(t *testing.T) {
			out := f.reg.Dispatch(context.Background(), v, 0, call(name, `{"search":"x"}`))
			if out.Decision == string(tools.DecisionAllowed) {
				t.Fatalf("vendor was allowed %s", name)
			}
		})
	}
	if len(f.reader.calls) != 0 {
		t.Fatalf("refused calls still read data: %v", f.reader.calls)
	}
}

// The schema list a model sees must contain nothing outside its dashboard.
func TestSchemasAreScoped(t *testing.T) {
	f := newFixture(t)

	cases := []struct {
		name    string
		actor   authctx.Actor
		allowed []string
		denied  []string
	}{
		{"pharmacy", pharmacist(1, 10),
			[]string{"orders_list", "spend_summary", "branches_list"},
			[]string{"sales_summary", "organizations_search", "low_stock"}},
		{"vendor", vendor(2, 20),
			[]string{"supply_orders_list", "sales_summary", "my_products"},
			[]string{"orders_list", "spend_summary", "organizations_search"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			seen := map[string]bool{}
			for _, s := range f.reg.Schemas(tc.actor) {
				seen[s.Name] = true
			}
			for _, want := range tc.allowed {
				if !seen[want] {
					t.Errorf("%s should be offered %s", tc.name, want)
				}
			}
			for _, deny := range tc.denied {
				if seen[deny] {
					t.Errorf("%s must not be offered %s", tc.name, deny)
				}
			}
		})
	}
}

// A partially-granted employee is offered only the tools matching what they
// hold, so the model cannot even name the others.
func TestSchemasFollowIndividualGrants(t *testing.T) {
	f := newFixture(t)
	limited := actor(rbac.ScopePharmacy, 1, 10, assistant.GatePharmacy, "pharmacy.branch.view")

	names := map[string]bool{}
	for _, s := range f.reg.Schemas(limited) {
		names[s.Name] = true
	}
	if !names["branches_list"] {
		t.Fatal("branch permission did not offer branches_list")
	}
	for _, forbidden := range []string{"orders_list", "spend_summary", "wallet_summary"} {
		if names[forbidden] {
			t.Fatalf("%s offered without its permission", forbidden)
		}
	}
}

// ---------------------------------------------------------------------------
// Handles: forgery, enumeration, cross-tenant replay
// ---------------------------------------------------------------------------

func TestForeignHandleIsRefused(t *testing.T) {
	f := newFixture(t)
	victim := pharmacist(1, 10)
	attacker := pharmacist(2, 20)

	// Issued legitimately to the victim while they were reading their orders.
	stolen := f.signer.Issue(handles.KindOrder, 501,
		handles.Binding{OrgID: victim.OrgID, UserID: victim.UserID})

	args, _ := json.Marshal(map[string]string{"order": stolen})
	out := f.reg.Dispatch(context.Background(), attacker, 0, call("order_details", string(args)))

	if out.Decision != string(tools.DecisionHandle) {
		t.Fatalf("decision = %q, want denied_handle", out.Decision)
	}
	if len(f.reader.calls) != 0 {
		t.Fatalf("a foreign handle still reached the reader: %v", f.reader.calls)
	}
}

// Raw ids must not work in place of a handle. This is the enumeration test: a
// model that guesses "order 501" gets nothing.
func TestRawIDsAreNotAccepted(t *testing.T) {
	f := newFixture(t)
	ph := pharmacist(1, 10)

	for _, guess := range []string{"1", "501", "999999", "hord_1", "", "null"} {
		args, _ := json.Marshal(map[string]string{"order": guess})
		out := f.reg.Dispatch(context.Background(), ph, 0, call("order_details", string(args)))
		if out.Decision == string(tools.DecisionAllowed) {
			t.Fatalf("raw id %q was accepted as a handle", guess)
		}
	}
	if len(f.reader.calls) != 0 {
		t.Fatalf("guessed ids reached the reader: %v", f.reader.calls)
	}
}

// The caller's own handle works — the refusals above are not simply "nothing
// works".
func TestOwnHandleResolves(t *testing.T) {
	f := newFixture(t)
	ph := pharmacist(1, 10)

	own := f.signer.Issue(handles.KindOrder, 501,
		handles.Binding{OrgID: ph.OrgID, UserID: ph.UserID})
	args, _ := json.Marshal(map[string]string{"order": own})

	out := f.reg.Dispatch(context.Background(), ph, 0, call("order_details", string(args)))
	if out.Decision != string(tools.DecisionAllowed) {
		t.Fatalf("decision = %q, want allowed: %s", out.Decision, out.Content)
	}
	if len(f.reader.calls) != 1 || f.reader.calls[0] != "PurchaseOrderDetail" {
		t.Fatalf("reads = %v", f.reader.calls)
	}
}
