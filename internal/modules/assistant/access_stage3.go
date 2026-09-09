package assistant

import (
	"context"
	"encoding/json"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
)

// ProjectionKind identifies one fixed, read-only assistant projection. The
// name is selected by server code; it is never accepted as SQL or a table name
// from the model.
type ProjectionKind string

const (
	ProjectionReorderSuggestions  ProjectionKind = "reorder_suggestions"
	ProjectionCatalogSearch       ProjectionKind = "catalog_search"
	ProjectionOfferDetails        ProjectionKind = "offer_details"
	ProjectionCartSummary         ProjectionKind = "cart_summary"
	ProjectionPurchaseRequests    ProjectionKind = "purchase_requests_list"
	ProjectionInvoices            ProjectionKind = "invoices_list"
	ProjectionInvoiceDetails      ProjectionKind = "invoice_details"
	ProjectionPayments            ProjectionKind = "payments_list"
	ProjectionSavingProducts      ProjectionKind = "saving_products_list"
	ProjectionSmartOrderRuns      ProjectionKind = "smart_order_runs_list"
	ProjectionSmartOrderDetails   ProjectionKind = "smart_order_run_details"
	ProjectionDecisionMemory      ProjectionKind = "decision_memory_search"
	ProjectionBranchQuota         ProjectionKind = "branch_quota_status"
	ProjectionSupplierProfile     ProjectionKind = "supplier_profile"
	ProjectionFavourites          ProjectionKind = "favourites_list"
	ProjectionNotifications       ProjectionKind = "notifications_list"
	ProjectionVariantDetails      ProjectionKind = "variant_details"
	ProjectionStockByWarehouse    ProjectionKind = "stock_by_warehouse"
	ProjectionWarehouseTransfers  ProjectionKind = "warehouse_transfers"
	ProjectionQuotaReport         ProjectionKind = "quota_report"
	ProjectionCoverageReport      ProjectionKind = "coverage_report"
	ProjectionOrdersByStatus      ProjectionKind = "orders_by_status"
	ProjectionRevenueByPeriod     ProjectionKind = "revenue_by_period"
	ProjectionRevenueByProduct    ProjectionKind = "revenue_by_product"
	ProjectionCustomers           ProjectionKind = "customers_list"
	ProjectionImportRuns          ProjectionKind = "import_runs_list"
	ProjectionImportRunDetails    ProjectionKind = "import_run_details"
	ProjectionOffersPerformance   ProjectionKind = "offers_performance"
	ProjectionSponsorshipStatus   ProjectionKind = "sponsorship_status"
	ProjectionTeam                ProjectionKind = "team_list"
	ProjectionReviews             ProjectionKind = "reviews_list"
	ProjectionOrganizations       ProjectionKind = "organizations_list"
	ProjectionOrganizationDetails ProjectionKind = "organization_details"
	ProjectionApprovals           ProjectionKind = "approvals_pending"
	ProjectionDeletionRequests    ProjectionKind = "deletion_requests"
	ProjectionUsers               ProjectionKind = "users_search"
	ProjectionUserDetails         ProjectionKind = "user_details"
	ProjectionErrorLogs           ProjectionKind = "error_logs_search"
	ProjectionAuditLog            ProjectionKind = "audit_log_search"
	ProjectionFinance             ProjectionKind = "finance_overview"
	ProjectionWalletTransactions  ProjectionKind = "wallet_transactions_search"
	ProjectionSubscriptions       ProjectionKind = "subscriptions_report"
	ProjectionVisitors            ProjectionKind = "visitors_report"
	ProjectionHealth              ProjectionKind = "platform_health"
	ProjectionMatchDecisions      ProjectionKind = "match_decisions_search"
	ProjectionInstitutionalGraph  ProjectionKind = "institutional_graph"
)

// ProjectionQuery is the bounded argument set shared by Stage-3 reads.
// Detail IDs are resolved from actor-bound handles before this reaches a
// repository.
type ProjectionQuery struct {
	Kind      ProjectionKind
	Range     DateRange
	Search    string
	Status    string
	Limit     int
	Offset    int
	ID        int64
	BranchID  int64
	ProductID int64
}

// ProjectionRow is a JSON-safe row returned by a fixed projection query.
// Database IDs stay server-side; Handle is the only identifier a model sees.
type ProjectionRow struct {
	ID     int64
	Handle string
	Values map[string]any
}

func (r ProjectionRow) MarshalJSON() ([]byte, error) {
	values := make(map[string]any, len(r.Values)+1)
	for key, value := range r.Values {
		values[key] = value
	}
	if r.Handle != "" {
		values["handle"] = r.Handle
	}
	return json.Marshal(values)
}

// ProjectionReader is optional so existing in-memory readers and security
// tests continue to exercise the original tool set without fake SQL data.
type ProjectionReader interface {
	ReadProjection(context.Context, authctx.Actor, ProjectionQuery) (Page[ProjectionRow], error)
}
