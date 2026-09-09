package tools

import (
	"github.com/muhiya/dawa24-store/internal/modules/assistant"
	"github.com/muhiya/dawa24-store/internal/modules/assistant/handles"
)

func adminStage3Tools(r *Registry) []Tool {
	return []Tool{
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionOrganizations, key: "organizations_list",
			description: "Platform organisations with filters for type and lifecycle status.",
			scopes:      adminScope, permissions: []string{"org.organization.view"}, handleKind: handles.KindOrgUnit, handleField: "organization",
		}, projectionListSchema(map[string]any{"search": strProp("Organisation name or number.")})),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionOrganizationDetails, key: "organization_details",
			description: "One organisation with branches, warehouses, and account status.",
			scopes:      adminScope, permissions: []string{"org.organization.view"}, handleKind: handles.KindOrgUnit, handleField: "organization", detail: true,
		}, projectionDetailSchema("organization")),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionApprovals, key: "approvals_pending",
			description: "Pending organisation and document approval requests.",
			scopes:      adminScope, permissions: []string{"org.approval.view"},
		}, projectionListSchema(map[string]any{"search": strProp("Organisation name.")})),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionDeletionRequests, key: "deletion_requests",
			description: "Account deletion requests awaiting platform review.",
			scopes:      adminScope, permissions: []string{"org.organization.view"},
		}, projectionListSchema(nil)),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionUsers, key: "users_search",
			description: "Users filtered by name, email, or account status.",
			scopes:      adminScope, permissions: []string{"identity.user.view"}, handleKind: handles.KindUser, handleField: "member",
		}, projectionListSchema(map[string]any{"search": strProp("Name, email, or phone.")})),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionUserDetails, key: "user_details",
			description: "One user, organisation memberships, roles, and last activity.",
			scopes:      adminScope, permissions: []string{"identity.user.view"}, handleKind: handles.KindUser, handleField: "member", detail: true,
		}, projectionDetailSchema("member")),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionErrorLogs, key: "error_logs_search",
			description: "Application error logs filtered by level, path, or message.",
			scopes:      adminScope, permissions: []string{"platform.error_log.view"},
		}, projectionListSchema(map[string]any{"search": strProp("Error message, route, or exception class.")})),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionAuditLog, key: "audit_log_search",
			description: "Platform audit activity filtered by action or entity.",
			scopes:      adminScope, permissions: []string{"platform.activity_log.view"},
		}, projectionListSchema(map[string]any{"search": strProp("Action, entity type, or entity reference.")})),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionFinance, key: "finance_overview",
			description: "Platform wallet, invoice, payment, withdrawal, and revenue totals.",
			scopes:      adminScope, permissions: []string{"billing.finance.view", "billing.wallet.read", "billing.invoice.view"},
		}, projectionListSchema(nil)),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionWalletTransactions, key: "wallet_transactions_search",
			description: "Wallet movements across the platform, with organisation and type filters.",
			scopes:      adminScope, permissions: []string{"billing.wallet.read"},
		}, projectionListSchema(map[string]any{"search": strProp("Organisation name, transaction type, or reference.")})),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionSubscriptions, key: "subscriptions_report",
			description: "Subscription counts, plans, statuses, and upcoming expiries.",
			scopes:      adminScope, permissions: []string{"billing.subscription_plan.view"},
		}, projectionListSchema(nil)),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionVisitors, key: "visitors_report",
			description: "Visitor totals and most visited cities and devices over a period.",
			scopes:      adminScope, permissions: []string{"platform.analytics.view"},
		}, projectionListSchema(nil)),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionHealth, key: "platform_health",
			description: "Current error, queue, and background-job health indicators.",
			scopes:      adminScope, permissions: []string{"platform.dashboard.view"},
		}, projectionListSchema(nil)),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionMatchDecisions, key: "match_decisions_search",
			description: "Saved product matching decisions and confidence history.",
			scopes:      adminScope, permissions: []string{"catalog.match_decision.view"},
		}, projectionListSchema(map[string]any{"search": strProp("Normalised product name or decision key.")})),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionInstitutionalGraph, key: "institutional_graph",
			description: "Institutional-work connections between buyer and supplier branches.",
			scopes:      adminScope, permissions: []string{"org.institutional_work.view"},
		}, projectionListSchema(map[string]any{"search": strProp("Work or organisation name.")})),
	}
}
