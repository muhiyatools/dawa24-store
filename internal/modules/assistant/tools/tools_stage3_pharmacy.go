package tools

import (
	"github.com/muhiya/dawa24-store/internal/modules/assistant"
	"github.com/muhiya/dawa24-store/internal/modules/assistant/handles"
)

func pharmacyStage3Tools(r *Registry) []Tool {
	return []Tool{
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionReorderSuggestions, key: "reorder_suggestions",
			description: "Products bought regularly that may be due for replenishment.",
			scopes:      pharmacyScope, permissions: []string{permOrderView}, handleKind: handles.KindProduct, handleField: "product",
		}, projectionListSchema(map[string]any{"search": strProp("Product name or SKU.")})),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionCatalogSearch, key: "catalog_search",
			description: "Buyer-aware catalogue search for products currently available to order.",
			scopes:      pharmacyScope, permissions: []string{"pharmacy.purchase_request.view", permOrderView}, handleKind: handles.KindProduct, handleField: "product",
		}, projectionListSchema(map[string]any{"search": strProp("Product name, active ingredient, SKU, or barcode.")})),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionOfferDetails, key: "offer_details",
			description: "Details, products, price rules, and validity of one offer.",
			scopes:      pharmacyScope, permissions: []string{"pharmacy.offer.view"}, handleKind: handles.KindOffer, handleField: "offer", detail: true,
		}, projectionDetailSchema("offer")),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionCartSummary, key: "cart_summary",
			description: "The current shopping cart, item count, and exact total.",
			scopes:      pharmacyScope, permissions: []string{"pharmacy.cart.use"},
		}, projectionListSchema(nil)),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionPurchaseRequests, key: "purchase_requests_list",
			description: "Open purchase requests and supplier quotes.",
			scopes:      pharmacyScope, permissions: []string{"pharmacy.purchase_request.view"}, handleKind: handles.KindRequest, handleField: "request",
		}, projectionListSchema(nil)),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionInvoices, key: "invoices_list",
			description: "Invoices, due dates, totals, and payment status.",
			scopes:      pharmacyScope, permissions: []string{permOrderView}, handleKind: handles.KindInvoice, handleField: "invoice",
		}, projectionListSchema(nil)),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionInvoiceDetails, key: "invoice_details",
			description: "Lines, amounts, and status of one invoice.",
			scopes:      pharmacyScope, permissions: []string{permOrderView}, handleKind: handles.KindInvoice, handleField: "invoice", detail: true,
		}, projectionDetailSchema("invoice")),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionPayments, key: "payments_list",
			description: "Payments made by the organisation and their settlement status.",
			scopes:      pharmacyScope, permissions: []string{"pharmacy.wallet.view"}, handleKind: handles.KindPayment, handleField: "payment",
		}, projectionListSchema(nil)),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionSavingProducts, key: "saving_products_list",
			description: "Saving-product suggestions and their prices and quantities.",
			scopes:      pharmacyScope, permissions: []string{"pharmacy.saving_product.view"}, handleKind: handles.KindProduct, handleField: "product",
		}, projectionListSchema(nil)),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionSmartOrderRuns, key: "smart_order_runs_list",
			description: "Previous smart-order runs and their outcome counts.",
			scopes:      pharmacyScope, permissions: []string{"pharmacy.smart_order.view"}, handleKind: handles.KindSmartRun, handleField: "run",
		}, projectionListSchema(nil)),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionSmartOrderDetails, key: "smart_order_run_details",
			description: "The result and line decisions of one smart-order run.",
			scopes:      pharmacyScope, permissions: []string{"pharmacy.smart_order.view"}, handleKind: handles.KindSmartRun, handleField: "run", detail: true,
		}, projectionDetailSchema("run")),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionDecisionMemory, key: "decision_memory_search",
			description: "Saved product-matching decisions used by the organisation.",
			scopes:      pharmacyScope, permissions: []string{"pharmacy.decision_memory.view"},
		}, projectionListSchema(map[string]any{"search": strProp("Normalised product name or decision key.")})),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionBranchQuota, key: "branch_quota_status",
			description: "Quota consumption and remaining allowance for branch products.",
			scopes:      pharmacyScope, permissions: []string{"pharmacy.purchase_request.view", "pharmacy.smart_order.view"},
		}, projectionListSchema(map[string]any{
			"branch":  strProp("Branch reference from branches_list."),
			"product": strProp("Product reference from a catalogue result."),
		})),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionSupplierProfile, key: "supplier_profile",
			description: "A supplier profile, rating, contact details, and coverage summary.",
			scopes:      pharmacyScope, permissions: []string{"pharmacy.supplier.view"}, handleKind: handles.KindOrgUnit, handleField: "organization",
		}, projectionListSchema(map[string]any{"search": strProp("Supplier name or organisation number.")})),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionFavourites, key: "favourites_list",
			description: "Products and suppliers saved as favourites.",
			scopes:      pharmacyScope, permissions: []string{"pharmacy.favorite.view"}, handleKind: handles.KindProduct, handleField: "product",
		}, projectionListSchema(nil)),
		projectionTool(r, stage3Spec{
			kind: assistant.ProjectionNotifications, key: "notifications_list",
			description: "Recent unread and read notifications for this account.",
			scopes:      pharmacyScope, permissions: []string{"pharmacy.dashboard.view"},
		}, projectionListSchema(map[string]any{"status": enumProp("Notification state.", "read", "unread")})),
	}
}
