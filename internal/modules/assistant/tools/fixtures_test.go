package tools_test

import (
	"context"
	"github.com/muhiya/dawa24-store/internal/modules/assistant"
	"github.com/muhiya/dawa24-store/internal/modules/assistant/handles"
	"github.com/muhiya/dawa24-store/internal/modules/assistant/tools"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/gateway"
	"github.com/muhiya/dawa24-store/internal/platform/rbac"
	"testing"
)

// auditLog captures decisions so a refusal can be checked for being recorded.
type auditLog struct {
	entries []assistant.ToolAudit
}

func (a *auditLog) RecordToolCall(_ context.Context, e assistant.ToolAudit) {
	a.entries = append(a.entries, e)
}

func (a *auditLog) last() assistant.ToolAudit {
	if len(a.entries) == 0 {
		return assistant.ToolAudit{}
	}
	return a.entries[len(a.entries)-1]
}

// ---------------------------------------------------------------------------
// Fixtures
// ---------------------------------------------------------------------------

type fixture struct {
	reg    *tools.Registry
	reader *spyReader
	audit  *auditLog
	signer *handles.Signer
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	reader := &spyReader{}
	audit := &auditLog{}
	signer := handles.NewSigner(testSecret)
	return &fixture{
		reg:    tools.NewRegistry(reader, signer, audit, nil),
		reader: reader,
		audit:  audit,
		signer: signer,
	}
}

// actor builds a caller with an explicit permission set, which is what a role
// resolved from the database produces.
func actor(scope rbac.Scope, orgID, userID int64, perms ...string) authctx.Actor {
	orgType := "customer"
	switch scope {
	case rbac.ScopeVendor:
		orgType = "vendor"
	case rbac.ScopeAdmin:
		orgType = ""
	}
	a := authctx.Actor{
		UserID:         userID,
		OrgID:          orgID,
		OrganizationID: orgID,
		OrgType:        orgType,
		OrgStatus:      "approved",
		Scope:          scope,
		IsStaff:        scope == rbac.ScopeAdmin,
	}
	a.Grants(perms)
	return a
}

// pharmacist is a fully-granted pharmacy user.
func pharmacist(orgID, userID int64) authctx.Actor {
	return actor(rbac.ScopePharmacy, orgID, userID,
		assistant.GatePharmacy, "pharmacy.order.view", "pharmacy.branch.view",
		"pharmacy.wallet.view", "pharmacy.subscription.view", "pharmacy.offer.view")
}

func vendor(orgID, userID int64) authctx.Actor {
	return actor(rbac.ScopeVendor, orgID, userID,
		assistant.GateVendor, "vendor.order.view", "vendor.product.view",
		"vendor.branch.view", "vendor.inventory.view", "vendor.offer.view")
}

func call(name, args string) gateway.ToolCall {
	return gateway.ToolCall{ID: "call_1", Name: name, Arguments: args}
}
