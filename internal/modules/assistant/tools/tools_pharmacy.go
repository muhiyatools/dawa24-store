package tools

import (
	"context"
	"encoding/json"

	"github.com/muhiya/dawa24-store/internal/modules/assistant"
	"github.com/muhiya/dawa24-store/internal/modules/assistant/handles"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/rbac"
)

// Pharmacy tools that encode more than a table read. Orders, lines, spend and
// rankings are answered by the dataset engine (tools_data.go).

var pharmacyScope = []rbac.Scope{rbac.ScopePharmacy}

const permOrderView = "pharmacy.order.view"

func pharmacyTools(r *Registry) []Tool {
	return []Tool{
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
