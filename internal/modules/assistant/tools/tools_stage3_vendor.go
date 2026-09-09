package tools

import (
	"github.com/muhiya/dawa24-store/internal/modules/assistant"
	"github.com/muhiya/dawa24-store/internal/modules/assistant/handles"
)

func vendorStage3Tools(r *Registry) []Tool {
	return []Tool{
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionVariantDetails, key: "variant_details",
			description: "One catalogue variant with price, discount, status, and stock summary.",
			scopes:      vendorScope, permissions: []string{"vendor.product.view"}, handleKind: handles.KindVariant, handleField: "variant", detail: true,
		}, projectionDetailSchema("variant")),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionStockByWarehouse, key: "stock_by_warehouse",
			description: "Stock quantities grouped by warehouse and product.",
			scopes:      vendorScope, permissions: []string{"vendor.inventory.view"}, handleKind: handles.KindWarehouse, handleField: "warehouse",
		}, projectionListSchema(map[string]any{"search": strProp("Product name, SKU, or warehouse name.")})),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionWarehouseTransfers, key: "warehouse_transfers",
			description: "Warehouse-to-warehouse stock transfers and their status.",
			scopes:      vendorScope, permissions: []string{"vendor.inventory.view"}, handleKind: handles.KindTransfer, handleField: "transfer",
		}, projectionListSchema(nil)),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionQuotaReport, key: "quota_report",
			description: "Branch quotas, usage, and remaining allowance on vendor products.",
			scopes:      vendorScope, permissions: []string{"vendor.quota.view"},
		}, projectionListSchema(nil)),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionCoverageReport, key: "coverage_report",
			description: "The vendor delivery coverage areas, branches, and weekly windows.",
			scopes:      vendorScope, permissions: []string{"vendor.pharmacy_coverage.view", "vendor.coverage.view"},
		}, projectionListSchema(map[string]any{"search": strProp("City, governorate, or branch name.")})),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionOrdersByStatus, key: "orders_by_status",
			description: "Incoming vendor shipments grouped by order status.",
			scopes:      vendorScope, permissions: []string{"vendor.order.view"},
		}, projectionListSchema(nil)),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionRevenueByPeriod, key: "revenue_by_period",
			description: "Exact vendor revenue grouped by day, week, or month.",
			scopes:      vendorScope, permissions: []string{"vendor.earnings.view"},
		}, projectionListSchema(nil)),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionRevenueByProduct, key: "revenue_by_product",
			description: "Exact vendor revenue grouped by product over a period.",
			scopes:      vendorScope, permissions: []string{"vendor.earnings.view"}, handleKind: handles.KindProduct, handleField: "product",
		}, projectionListSchema(map[string]any{"search": strProp("Product name or SKU.")})),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionCustomers, key: "customers_list",
			description: "Buying organisations and their order and revenue totals.",
			scopes:      vendorScope, permissions: []string{"vendor.order.view"}, handleKind: handles.KindOrgUnit, handleField: "organization",
		}, projectionListSchema(map[string]any{"search": strProp("Buying organisation name.")})),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionImportRuns, key: "import_runs_list",
			description: "Previous catalogue import runs and their outcomes.",
			scopes:      vendorScope, permissions: []string{"vendor.ingest.view"}, handleKind: handles.KindImportRun, handleField: "run",
		}, projectionListSchema(nil)),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionImportRunDetails, key: "import_run_details",
			description: "Rows, errors, and counters for one catalogue import run.",
			scopes:      vendorScope, permissions: []string{"vendor.ingest.view"}, handleKind: handles.KindImportRun, handleField: "run", detail: true,
		}, projectionDetailSchema("run")),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionOffersPerformance, key: "offers_performance",
			description: "Offer impressions, clicks, and conversions over a period.",
			scopes:      vendorScope, permissions: []string{"vendor.offer.view"}, handleKind: handles.KindOffer, handleField: "offer",
		}, projectionListSchema(nil)),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionSponsorshipStatus, key: "sponsorship_status",
			description: "Sponsorship purchases, credits used, and active sponsored items.",
			scopes:      vendorScope, permissions: []string{"vendor.offer_package.view"},
		}, projectionListSchema(nil)),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionTeam, key: "team_list",
			description: "The vendor organisation members, roles, and account status.",
			scopes:      vendorScope, permissions: []string{"vendor.team.view"}, handleKind: handles.KindUser, handleField: "member",
		}, projectionListSchema(map[string]any{"search": strProp("Member name or email.")})),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionReviews, key: "reviews_list",
			description: "Customer reviews, ratings, and response state.",
			scopes:      vendorScope, permissions: []string{"vendor.review.view"}, handleKind: handles.KindReview, handleField: "review",
		}, projectionListSchema(nil)),
	}
}
