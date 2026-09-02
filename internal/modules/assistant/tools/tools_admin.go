package tools

import (
	"context"
	"encoding/json"

	"github.com/muhiya/dawa24-store/internal/modules/assistant"
	"github.com/muhiya/dawa24-store/internal/modules/assistant/handles"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/rbac"
)

// Platform staff tools.
//
// These are the only tools whose queries cross tenant boundaries, and they are
// deliberately few and deliberately coarse: counts, registration records, and
// AI consumption. There is no tool here that reads one tenant's order book,
// prices or documents — a staff member who needs that opens the screen, where
// the access is attributable to a page view rather than to a paraphrase.
//
// Each is gated on the specific admin permission the equivalent screen needs,
// held by THIS admin. "Is staff" is not a permission and does not admit
// anything here; the platform assistant gate (platform.assistant.use) admits
// the assistant, and each tool then asks for its own key on top.

var adminScope = []rbac.Scope{rbac.ScopeAdmin}

func adminTools(r *Registry) []Tool {
	return []Tool{
		{
			Name:        "organizations_search",
			Description: "بحث في المنشآت: الاسم والنوع وحالة الاعتماد وتاريخ التسجيل.",
			Params: objectSchema(pageProps(map[string]any{
				"search": strProp("اسم المنشأة أو جزء منه."),
				"status": enumProp("حالة الاعتماد.", "pending", "approved", "rejected", "suspended"),
			})),
			Scopes:      adminScope,
			Permissions: []string{"org.organization.view"},
			Handler:     r.organizationsSearch,
		},
		{
			Name:        "platform_overview",
			Description: "مؤشرات المنصة: عدد المنشآت والمستخدمين والطلبات وحجم التداول.",
			Params:      objectSchema(dateProps(nil)),
			Scopes:      adminScope,
			Permissions: []string{"platform.dashboard.view"},
			Handler:     r.platformOverview,
		},
		{
			Name:        "ai_usage_summary",
			Description: "استهلاك الذكاء الاصطناعي حسب المنشأة والخاصية.",
			Params: objectSchema(dateProps(map[string]any{
				"limit": intProp("عدد الصفوف المطلوبة، بحد أقصى 25.", 1, assistant.PageLimit),
			})),
			Scopes:      adminScope,
			Permissions: []string{"platform.setting.view", "platform.dashboard.view"},
			Handler:     r.aiUsageSummary,
		},
	}
}

func (r *Registry) organizationsSearch(ctx context.Context, actor authctx.Actor, raw json.RawMessage) (Result, error) {
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
	status, err := oneOf("status", args.Status, "pending", "approved", "rejected", "suspended")
	if err != nil {
		return Result{}, err
	}
	off, err := clampOffset(args.Offset)
	if err != nil {
		return Result{}, err
	}

	res, err := r.reader.Organizations(ctx, actor, assistant.ProductQuery{
		Search: term,
		Status: status,
		Offset: off,
		Limit:  clampLimit(args.Limit),
	})
	if err != nil {
		return Result{}, err
	}
	for i := range res.Rows {
		res.Rows[i].Handle = r.issue(actor, handles.KindOrgUnit, res.Rows[i].ID)
	}
	return page(res, "organizations"), nil
}

func (r *Registry) platformOverview(ctx context.Context, actor authctx.Actor, raw json.RawMessage) (Result, error) {
	var args dateRangeArgs
	if err := decode(raw, &args); err != nil {
		return Result{}, err
	}
	rng, err := args.parse(30)
	if err != nil {
		return Result{}, err
	}
	summary, err := r.reader.PlatformOverview(ctx, actor, rng)
	if err != nil {
		return Result{}, err
	}
	if summary == nil {
		return Result{Note: "لا تتوفر مؤشرات لهذه الفترة."}, nil
	}
	return Result{Data: summary, Rows: 1}, nil
}

func (r *Registry) aiUsageSummary(ctx context.Context, actor authctx.Actor, raw json.RawMessage) (Result, error) {
	var args struct {
		dateRangeArgs
		Limit int `json:"limit,omitempty"`
	}
	if err := decode(raw, &args); err != nil {
		return Result{}, err
	}
	rng, err := args.parse(30)
	if err != nil {
		return Result{}, err
	}
	res, err := r.reader.AIUsage(ctx, actor, rng, clampLimit(args.Limit))
	if err != nil {
		return Result{}, err
	}
	return page(res, "usage"), nil
}
