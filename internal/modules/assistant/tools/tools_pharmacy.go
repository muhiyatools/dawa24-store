package tools

import (
	"context"
	"encoding/json"

	"github.com/muhiya/dawa24-store/internal/modules/assistant"
	"github.com/muhiya/dawa24-store/internal/modules/assistant/handles"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/rbac"
)

// What a pharmacy owner actually asks.
//
// The tool descriptions below are written for the model, in the vocabulary the
// customer uses on the screens: طلب الشراء, الشحنة, المورّد. They say what the
// tool answers AND when not to reach for it, because the most expensive mistake
// an agent makes is calling the listing tool five times to compute a total the
// summary tool would have returned once.

var pharmacyScope = []rbac.Scope{rbac.ScopePharmacy}

const permOrderView = "pharmacy.order.view"

func pharmacyTools(r *Registry) []Tool {
	return []Tool{
		{
			Name:        "orders_list",
			Description: "طلبات الشراء: الرقم والحالة والمبلغ والتاريخ والمورّد. للإجماليات استخدم spend_summary.",
			Params: objectSchema(pageProps(dateProps(map[string]any{
				"status": enumProp("حالة الطلب.",
					"pending", "confirmed", "processing", "ready_for_pickup",
					"shipped", "delivered", "cancelled", "refunded"),
				"payment_status": enumProp("حالة السداد.",
					"unpaid", "authorized", "paid", "partially_refunded", "refunded", "failed"),
				"search": strProp("بحث في رقم الطلب."),
			}))),
			Scopes:      pharmacyScope,
			Permissions: []string{permOrderView},
			Handler:     r.ordersList,
		},
		{
			Name:        "order_details",
			Description: "تفاصيل طلب واحد: أصنافه وشحناته. يحتاج مرجع الطلب من orders_list.",
			Params: objectSchema(map[string]any{
				"order": strProp("مرجع الطلب كما ورد في حقل order من نتيجة orders_list."),
			}, "order"),
			Scopes:      pharmacyScope,
			Permissions: []string{permOrderView},
			Handler:     r.orderDetails,
		},
		{
			Name:        "spend_summary",
			Description: "إجمالي المشتريات خلال فترة مع تقسيم اختياري. لكل سؤال «كم أنفقت» أو مقارنة فترتين.",
			Params: objectSchema(dateProps(map[string]any{
				"group_by": enumProp("طريقة تقسيم الإجمالي.", "status", "month", "counterparty"),
				"status":   strProp("قصر الحساب على حالة طلب واحدة."),
			})),
			Scopes:      pharmacyScope,
			Permissions: []string{permOrderView},
			Handler:     r.spendSummary,
		},
		{
			Name:        "top_purchased_products",
			Description: "أكثر الأصناف استهلاكاً للميزانية خلال فترة.",
			Params: objectSchema(dateProps(map[string]any{
				"rank_by": enumProp("معيار الترتيب. الافتراضي الإنفاق.", "revenue", "quantity"),
				"limit":   intProp("عدد الأصناف المطلوبة، بحد أقصى 25.", 1, assistant.PageLimit),
			})),
			Scopes:      pharmacyScope,
			Permissions: []string{permOrderView},
			Handler:     r.topPurchased,
		},
		{
			Name:        "market_search",
			Description: "بحث في أصناف الموردين: السعر والخصم والسعر النهائي واسم المورّد.",
			Params: objectSchema(pageProps(map[string]any{
				"search": strProp("اسم الصنف أو المادة الفعالة أو الكود."),
			}), "search"),
			Scopes:      pharmacyScope,
			Permissions: []string{"pharmacy.offer.view", "pharmacy.supplier.view", permOrderView},
			Handler:     r.marketSearch,
		},
	}
}

func (r *Registry) ordersList(ctx context.Context, actor authctx.Actor, raw json.RawMessage) (Result, error) {
	var args struct {
		dateRangeArgs
		Status        string `json:"status,omitempty"`
		PaymentStatus string `json:"payment_status,omitempty"`
		Search        string `json:"search,omitempty"`
		Limit         int    `json:"limit,omitempty"`
		Offset        int    `json:"offset,omitempty"`
	}
	if err := decode(raw, &args); err != nil {
		return Result{}, err
	}
	q, err := r.buildOrderQuery(args.dateRangeArgs, args.Status, args.PaymentStatus,
		args.Search, args.Limit, args.Offset)
	if err != nil {
		return Result{}, err
	}

	res, err := r.reader.PurchaseOrders(ctx, actor, q)
	if err != nil {
		return Result{}, err
	}
	for i := range res.Rows {
		res.Rows[i].Handle = r.issue(actor, handles.KindOrder, res.Rows[i].ID)
	}
	return page(res, "orders"), nil
}

// buildOrderQuery is shared with the vendor listing: the filters are identical
// even though the rows on either side of a trade are not.
func (r *Registry) buildOrderQuery(
	dr dateRangeArgs, status, paymentStatus, search string, limit, offset int,
) (assistant.OrderQuery, error) {
	var q assistant.OrderQuery

	rng, err := dr.parse(0) // a listing with no period means "most recent"
	if err != nil {
		return q, err
	}
	st, err := oneOf("status", status,
		"pending", "confirmed", "processing", "ready_for_pickup",
		"shipped", "delivered", "cancelled", "refunded", "returned")
	if err != nil {
		return q, err
	}
	pay, err := oneOf("payment_status", paymentStatus,
		"unpaid", "authorized", "paid", "partially_refunded", "refunded", "failed")
	if err != nil {
		return q, err
	}
	term, err := trimSearch(search)
	if err != nil {
		return q, err
	}
	off, err := clampOffset(offset)
	if err != nil {
		return q, err
	}

	return assistant.OrderQuery{
		Status:        st,
		PaymentStatus: pay,
		Search:        term,
		Range:         rng,
		Offset:        off,
		Limit:         clampLimit(limit),
	}, nil
}

func (r *Registry) orderDetails(ctx context.Context, actor authctx.Actor, raw json.RawMessage) (Result, error) {
	var args struct {
		Order string `json:"order"`
	}
	if err := decode(raw, &args); err != nil {
		return Result{}, err
	}
	id, err := r.resolveHandle(actor, handles.KindOrder, args.Order)
	if err != nil {
		return Result{}, err
	}

	detail, err := r.reader.PurchaseOrderDetail(ctx, actor, id)
	if err != nil {
		return Result{}, err
	}
	// A verified handle whose row does not come back means the row was deleted
	// or is no longer visible under row-level security. Either way the answer
	// is the same, and it is not "here is what it used to say".
	if detail == nil {
		return Result{Note: "الطلب غير متاح الآن."}, nil
	}
	detail.Order.Handle = r.issue(actor, handles.KindOrder, detail.Order.ID)
	return Result{Data: detail, Rows: len(detail.Lines)}, nil
}

func (r *Registry) spendSummary(ctx context.Context, actor authctx.Actor, raw json.RawMessage) (Result, error) {
	q, err := r.buildAggregateQuery(raw)
	if err != nil {
		return Result{}, err
	}
	agg, err := r.reader.PurchaseSummary(ctx, actor, q)
	if err != nil {
		return Result{}, err
	}
	if agg == nil || agg.Count == 0 {
		return Result{Note: "لا توجد طلبات في هذه الفترة."}, nil
	}
	return Result{Data: agg, Rows: agg.Count}, nil
}

// buildAggregateQuery is shared by both summary tools.
func (r *Registry) buildAggregateQuery(raw json.RawMessage) (assistant.AggregateQuery, error) {
	var args struct {
		dateRangeArgs
		GroupBy string `json:"group_by,omitempty"`
		Status  string `json:"status,omitempty"`
	}
	var q assistant.AggregateQuery
	if err := decode(raw, &args); err != nil {
		return q, err
	}
	// Ninety days: long enough to show a trend, short enough that "how much did
	// I spend" does not silently aggregate three years of trading.
	rng, err := args.parse(90)
	if err != nil {
		return q, err
	}
	group, err := oneOf("group_by", args.GroupBy, "status", "month", "counterparty")
	if err != nil {
		return q, err
	}
	status, err := oneOf("status", args.Status,
		"pending", "confirmed", "processing", "ready_for_pickup",
		"shipped", "delivered", "cancelled", "refunded", "returned")
	if err != nil {
		return q, err
	}
	return assistant.AggregateQuery{
		Range:  rng,
		Group:  assistant.GroupBy(group),
		Status: status,
		Limit:  assistant.PageLimit,
	}, nil
}

func (r *Registry) topPurchased(ctx context.Context, actor authctx.Actor, raw json.RawMessage) (Result, error) {
	q, err := r.buildRankingQuery(raw)
	if err != nil {
		return Result{}, err
	}
	res, err := r.reader.PurchasedProducts(ctx, actor, q)
	if err != nil {
		return Result{}, err
	}
	return page(res, "products"), nil
}

func (r *Registry) buildRankingQuery(raw json.RawMessage) (assistant.AggregateQuery, error) {
	var args struct {
		dateRangeArgs
		RankBy string `json:"rank_by,omitempty"`
		Limit  int    `json:"limit,omitempty"`
	}
	var q assistant.AggregateQuery
	if err := decode(raw, &args); err != nil {
		return q, err
	}
	rng, err := args.parse(90)
	if err != nil {
		return q, err
	}
	rank, err := oneOf("rank_by", args.RankBy, "revenue", "quantity")
	if err != nil {
		return q, err
	}
	if rank == "" {
		rank = "revenue"
	}
	return assistant.AggregateQuery{
		Range:   rng,
		Ranking: rank,
		Limit:   clampLimit(args.Limit),
	}, nil
}

func (r *Registry) marketSearch(ctx context.Context, actor authctx.Actor, raw json.RawMessage) (Result, error) {
	var args struct {
		Search string `json:"search"`
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
	if term == "" {
		return Result{}, badArgs("حدّد اسم الصنف المطلوب البحث عنه.")
	}
	off, err := clampOffset(args.Offset)
	if err != nil {
		return Result{}, err
	}

	res, err := r.reader.MarketProducts(ctx, actor, assistant.ProductQuery{
		Search: term,
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
