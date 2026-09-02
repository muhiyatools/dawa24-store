package tools

import (
	"context"
	"encoding/json"

	"github.com/muhiya/dawa24-store/internal/modules/assistant"
	"github.com/muhiya/dawa24-store/internal/modules/assistant/handles"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/rbac"
)

// Tools both trading dashboards share.
//
// They carry the permission keys of BOTH dashboards because Permissions is an
// any-of check and the scope gate runs first. A pharmacy user is admitted by
// pharmacy.branch.view and a vendor by vendor.branch.view; neither can be
// admitted by the other's key, because the permission resolver strips
// out-of-scope grants before an actor is ever built.

var tradingScopes = []rbac.Scope{rbac.ScopePharmacy, rbac.ScopeVendor}

func sharedTools(r *Registry) []Tool {
	return []Tool{
		{
			Name: "branches_list",
			Description: "قائمة فروع منشأة المستخدم المسجلة على المنصة، باسم الفرع والهاتف " +
				"والمدينة وحالة التفعيل. استخدمها عندما يسأل المستخدم عن فروعه أو عندما تحتاج " +
				"أن تنسب بيانات إلى فرع.",
			Params:      objectSchema(nil),
			Scopes:      tradingScopes,
			Permissions: []string{"pharmacy.branch.view", "vendor.branch.view"},
			Handler:     r.branchesList,
		},
		{
			Name: "wallet_summary",
			Description: "رصيد محفظة المنشأة الحالي وآخر المعاملات المالية عليها (إيداع، سحب، " +
				"شراء، استرداد). استخدمها لأسئلة الرصيد والمصروفات من المحفظة.",
			Params:      objectSchema(nil),
			Scopes:      tradingScopes,
			Permissions: []string{"pharmacy.wallet.view", "vendor.wallet.view"},
			Handler:     r.walletSummary,
		},
		{
			Name: "subscription_status",
			Description: "حالة اشتراك المنشأة: اسم الباقة، الحالة، تاريخ البداية والانتهاء، " +
				"وعدد الأيام المتبقية. استخدمها لأسئلة التجديد والباقة.",
			Params:      objectSchema(nil),
			Scopes:      tradingScopes,
			Permissions: []string{"pharmacy.subscription.view", "vendor.subscription.view"},
			Handler:     r.subscriptionStatus,
		},
	}
}

func (r *Registry) branchesList(ctx context.Context, actor authctx.Actor, raw json.RawMessage) (Result, error) {
	var args struct{}
	if err := decode(raw, &args); err != nil {
		return Result{}, err
	}
	rows, err := r.reader.Branches(ctx, actor)
	if err != nil {
		return Result{}, err
	}
	for i := range rows {
		rows[i].Handle = r.issue(actor, handles.KindBranch, rows[i].ID)
	}
	if len(rows) == 0 {
		return Result{Note: "لا توجد فروع مسجلة لهذه المنشأة."}, nil
	}
	return Result{Data: map[string]any{"branches": rows}, Rows: len(rows)}, nil
}

func (r *Registry) walletSummary(ctx context.Context, actor authctx.Actor, raw json.RawMessage) (Result, error) {
	var args struct{}
	if err := decode(raw, &args); err != nil {
		return Result{}, err
	}
	summary, err := r.reader.Wallet(ctx, actor)
	if err != nil {
		return Result{}, err
	}
	if summary == nil {
		return Result{Note: "لا توجد محفظة مفعّلة لهذا الحساب."}, nil
	}
	return Result{Data: summary, Rows: len(summary.Recent)}, nil
}

func (r *Registry) subscriptionStatus(ctx context.Context, actor authctx.Actor, raw json.RawMessage) (Result, error) {
	var args struct{}
	if err := decode(raw, &args); err != nil {
		return Result{}, err
	}
	sub, err := r.reader.Subscription(ctx, actor)
	if err != nil {
		return Result{}, err
	}
	if sub == nil {
		return Result{Note: "لا يوجد اشتراك نشط لهذه المنشأة."}, nil
	}
	return Result{Data: sub, Rows: 1}, nil
}

// page is the shared shape every listing tool returns, so the model learns one
// pagination convention instead of a different one per tool.
func page[T any](p assistant.Page[T], key string) Result {
	if len(p.Rows) == 0 {
		return Result{Note: "لا توجد نتائج مطابقة."}
	}
	data := map[string]any{key: p.Rows, "count": len(p.Rows)}
	if p.Total > 0 {
		data["total_matching"] = p.Total
	}
	if p.HasMore {
		data["has_more"] = true
		data["next_offset"] = p.NextOffset
	}
	return Result{Data: data, Rows: len(p.Rows)}
}
