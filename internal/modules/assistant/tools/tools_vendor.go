package tools

import (
	"context"
	"encoding/json"

	"github.com/muhiya/dawa24-store/internal/modules/assistant"
	"github.com/muhiya/dawa24-store/internal/modules/assistant/handles"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/rbac"
)

// The supplier's side of the same trade.
//
// A vendor reads commerce.order_shipments, not commerce.orders: row-level
// security scopes the former to the selling organisation and the latter to the
// buying one. That split is what makes "my incoming orders" and "my purchases"
// two different questions with the same underlying rows, and it is enforced by
// the database rather than by these queries remembering to say so.

var vendorScope = []rbac.Scope{rbac.ScopeVendor}

const permVendorOrder = "vendor.order.view"

func vendorTools(r *Registry) []Tool {
	return []Tool{
		{
			Name: "supply_orders_list",
			Description: "أوامر التوريد الواردة إلى هذا المورّد: رقم الشحنة والصيدلية الطالبة " +
				"والحالة والقيمة والتاريخ. للفلترة بالحالة أو الفترة. لا تستخدمها لحساب " +
				"الإجماليات — استخدم sales_summary.",
			Params: objectSchema(pageProps(dateProps(map[string]any{
				"status": enumProp("حالة الشحنة.",
					"pending", "confirmed", "processing", "ready_for_pickup",
					"shipped", "delivered", "cancelled", "returned"),
				"search": strProp("بحث في رقم الشحنة أو رقم الطلب."),
			}))),
			Scopes:      vendorScope,
			Permissions: []string{permVendorOrder},
			Handler:     r.supplyOrdersList,
		},
		{
			Name: "supply_order_details",
			Description: "تفاصيل شحنة واحدة: أصنافها وكمياتها وأسعارها، والفرع المسؤول عنها. " +
				"يتطلب مرجع الشحنة من نتيجة supply_orders_list.",
			Params: objectSchema(map[string]any{
				"shipment": strProp("مرجع الشحنة كما ورد في حقل shipment من نتيجة supply_orders_list."),
			}, "shipment"),
			Scopes:      vendorScope,
			Permissions: []string{permVendorOrder},
			Handler:     r.supplyOrderDetails,
		},
		{
			Name: "sales_summary",
			Description: "إجمالي مبيعات المورّد خلال فترة: عدد الأوامر، الإيراد، متوسط قيمة " +
				"الأمر، مع تقسيم اختياري حسب الحالة أو الشهر أو العميل. هذه هي الأداة الصحيحة " +
				"لكل سؤال عن حجم المبيعات أو المقارنة بين فترتين.",
			Params: objectSchema(dateProps(map[string]any{
				"group_by": enumProp("طريقة تقسيم الإجمالي.", "status", "month", "counterparty"),
				"status":   strProp("قصر الحساب على حالة شحنة واحدة."),
			})),
			Scopes:      vendorScope,
			Permissions: []string{permVendorOrder},
			Handler:     r.salesSummary,
		},
		{
			Name:        "top_sold_products",
			Description: "أكثر أصناف هذا المورّد مبيعاً خلال فترة، مرتبة حسب الإيراد أو الكمية.",
			Params: objectSchema(dateProps(map[string]any{
				"rank_by": enumProp("معيار الترتيب. الافتراضي الإيراد.", "revenue", "quantity"),
				"limit":   intProp("عدد الأصناف المطلوبة، بحد أقصى 25.", 1, assistant.PageLimit),
			})),
			Scopes:      vendorScope,
			Permissions: []string{permVendorOrder},
			Handler:     r.topSold,
		},
		{
			Name: "my_products",
			Description: "كتالوج أصناف هذا المورّد: الاسم والكود والسعر والخصم والسعر النهائي " +
				"وحالة النشر وعدد مرات البيع. للبحث عن صنف بعينه أو مراجعة التسعير.",
			Params: objectSchema(pageProps(map[string]any{
				"search": strProp("اسم الصنف أو الكود."),
				"status": enumProp("حالة الصنف.", "active", "inactive", "pending", "rejected"),
			})),
			Scopes:      vendorScope,
			Permissions: []string{"vendor.product.view"},
			Handler:     r.vendorProducts,
		},
		{
			Name: "low_stock",
			Description: "الأصناف التي وصل رصيدها في المخازن إلى الحد الأدنى أو أقل. " +
				"استخدمها لأسئلة «إيه اللي قرب يخلص» و«محتاج أورد إيه».",
			Params: objectSchema(map[string]any{
				"limit": intProp("عدد الأصناف المطلوبة، بحد أقصى 25.", 1, assistant.PageLimit),
			}),
			Scopes:      vendorScope,
			Permissions: []string{"vendor.inventory.view"},
			Handler:     r.lowStock,
		},
		{
			Name: "my_offers",
			Description: "عروض هذا المورّد: العنوان ونوع الخصم وقيمته وفترة السريان وعدد " +
				"المشاهدات والنقرات وعدد الأصناف المشمولة. لقياس أداء العروض.",
			Params: objectSchema(map[string]any{
				"active_only": boolProp("اقتصر على العروض السارية الآن."),
				"limit":       intProp("عدد العروض المطلوبة، بحد أقصى 25.", 1, assistant.PageLimit),
			}),
			Scopes:      vendorScope,
			Permissions: []string{"vendor.offer.view"},
			Handler:     r.vendorOffers,
		},
	}
}

func (r *Registry) supplyOrdersList(ctx context.Context, actor authctx.Actor, raw json.RawMessage) (Result, error) {
	var args struct {
		dateRangeArgs
		Status string `json:"status,omitempty"`
		Search string `json:"search,omitempty"`
		Limit  int    `json:"limit,omitempty"`
		Offset int    `json:"offset,omitempty"`
	}
	if err := decode(raw, &args); err != nil {
		return Result{}, err
	}
	q, err := r.buildOrderQuery(args.dateRangeArgs, args.Status, "", args.Search, args.Limit, args.Offset)
	if err != nil {
		return Result{}, err
	}

	res, err := r.reader.SupplyOrders(ctx, actor, q)
	if err != nil {
		return Result{}, err
	}
	for i := range res.Rows {
		res.Rows[i].Handle = r.issue(actor, handles.KindShipment, res.Rows[i].ID)
	}
	return page(res, "shipments"), nil
}

func (r *Registry) supplyOrderDetails(ctx context.Context, actor authctx.Actor, raw json.RawMessage) (Result, error) {
	var args struct {
		Shipment string `json:"shipment"`
	}
	if err := decode(raw, &args); err != nil {
		return Result{}, err
	}
	id, err := r.resolveHandle(actor, handles.KindShipment, args.Shipment)
	if err != nil {
		return Result{}, err
	}

	detail, err := r.reader.SupplyOrderDetail(ctx, actor, id)
	if err != nil {
		return Result{}, err
	}
	if detail == nil {
		return Result{Note: "الشحنة غير متاحة الآن."}, nil
	}
	detail.Shipment.Handle = r.issue(actor, handles.KindShipment, detail.Shipment.ID)
	return Result{Data: detail, Rows: len(detail.Lines)}, nil
}

func (r *Registry) salesSummary(ctx context.Context, actor authctx.Actor, raw json.RawMessage) (Result, error) {
	q, err := r.buildAggregateQuery(raw)
	if err != nil {
		return Result{}, err
	}
	agg, err := r.reader.SalesSummary(ctx, actor, q)
	if err != nil {
		return Result{}, err
	}
	if agg == nil || agg.Count == 0 {
		return Result{Note: "لا توجد أوامر توريد في هذه الفترة."}, nil
	}
	return Result{Data: agg, Rows: agg.Count}, nil
}

func (r *Registry) topSold(ctx context.Context, actor authctx.Actor, raw json.RawMessage) (Result, error) {
	q, err := r.buildRankingQuery(raw)
	if err != nil {
		return Result{}, err
	}
	res, err := r.reader.SoldProducts(ctx, actor, q)
	if err != nil {
		return Result{}, err
	}
	return page(res, "products"), nil
}

func (r *Registry) vendorProducts(ctx context.Context, actor authctx.Actor, raw json.RawMessage) (Result, error) {
	var args struct {
		Search string `json:"search,omitempty"`
		Status string `json:"status,omitempty"`
		Limit  int    `json:"limit,omitempty"`
		Offset int    `json:"offset,omitempty"`
	}
	if err := decode(raw, &args); err != nil {
		return Result{}, err
	}
	term, err := trimSearch(args.Search)
	if err != nil {
		return Result{}, err
	}
	status, err := oneOf("status", args.Status, "active", "inactive", "pending", "rejected")
	if err != nil {
		return Result{}, err
	}
	off, err := clampOffset(args.Offset)
	if err != nil {
		return Result{}, err
	}

	res, err := r.reader.VendorProducts(ctx, actor, assistant.ProductQuery{
		Search: term,
		Status: status,
		Offset: off,
		Limit:  clampLimit(args.Limit),
	})
	if err != nil {
		return Result{}, err
	}
	for i := range res.Rows {
		res.Rows[i].Handle = r.issue(actor, handles.KindProduct, res.Rows[i].ID)
	}
	return page(res, "products"), nil
}

func (r *Registry) lowStock(ctx context.Context, actor authctx.Actor, raw json.RawMessage) (Result, error) {
	var args struct {
		Limit int `json:"limit,omitempty"`
	}
	if err := decode(raw, &args); err != nil {
		return Result{}, err
	}
	res, err := r.reader.LowStock(ctx, actor, clampLimit(args.Limit))
	if err != nil {
		return Result{}, err
	}
	if len(res.Rows) == 0 {
		return Result{Note: "لا توجد أصناف تحت الحد الأدنى للمخزون."}, nil
	}
	return page(res, "items"), nil
}

func (r *Registry) vendorOffers(ctx context.Context, actor authctx.Actor, raw json.RawMessage) (Result, error) {
	var args struct {
		ActiveOnly bool `json:"active_only,omitempty"`
		Limit      int  `json:"limit,omitempty"`
	}
	if err := decode(raw, &args); err != nil {
		return Result{}, err
	}
	res, err := r.reader.Offers(ctx, actor, args.ActiveOnly, clampLimit(args.Limit))
	if err != nil {
		return Result{}, err
	}
	for i := range res.Rows {
		res.Rows[i].Handle = r.issue(actor, handles.KindOffer, res.Rows[i].ID)
	}
	return page(res, "offers"), nil
}
