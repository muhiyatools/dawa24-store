package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/assistant"
	"github.com/muhiya/dawa24-store/internal/modules/assistant/actions"
	"github.com/muhiya/dawa24-store/internal/modules/assistant/handles"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/gateway"
	"github.com/muhiya/dawa24-store/internal/platform/rbac"
)

// The two tools that face the action layer.
//
// propose_action is the only way the model reaches a change, and it cannot
// make one: it stores a proposal the user must confirm. Its schema is built per
// caller, listing only the actions that caller's grants admit, so the model is
// never shown an action it could not propose.
//
// find_offers is the buying read the proposals depend on: which supplier
// listings this branch can order right now, with a ref for cart_add. It is
// answered by the catalogue's own query and availability rule, not by a
// separate approximation.

// OfferFinder answers find_offers. Implemented by the platform UI layer.
type OfferFinder interface {
	FindOffers(ctx context.Context, actor authctx.Actor, q assistant.OfferQuery) (*assistant.OfferResult, error)
}

// SetActions wires the action flow and the offer finder.
func (r *Registry) SetActions(flow *actions.Flow, offers OfferFinder) {
	r.flow, r.offers = flow, offers
}

var buyingScopes = []rbac.Scope{rbac.ScopePharmacy, rbac.ScopeVendor}

func actionTools(r *Registry) []Tool {
	return []Tool{
		{
			Name: "propose_action",
			// The description and the action enum are replaced per caller in
			// Schemas; this is the fallback shape.
			Description: "Prepare an action for the user to confirm.",
			Params: objectSchema(map[string]any{
				"action": strProp("Action name."),
				"args":   map[string]any{"type": "object", "description": "Arguments for the action."},
			}, "action"),
			Scopes:      allScopes,
			Permissions: []string{assistant.ActPharmacy, assistant.ActVendor, assistant.ActAdmin},
			Timeout:     20 * time.Second,
			Handler:     r.proposeAction,
		},
		{
			Name: "find_offers",
			Description: "Supplier listings a buying branch can order right now, with price, discount, minimum quantity and whether it is orderable " +
				"(coverage, stock, quota and approval checked exactly as the cart checks them). Returns a ref per listing for cart_add.",
			Params: objectSchema(map[string]any{
				"search":          strProp("Product name, active ingredient or code."),
				"branch":          strProp("Branch ref from the session context or the branches dataset. Omit for the user's buying branch."),
				"only_discounted": map[string]any{"type": "boolean", "description": "Only listings with a discount."},
				"limit":           intProp("Listings to return, max 40.", 1, 40),
			}, "search"),
			Scopes:      buyingScopes,
			Permissions: []string{rbac.BuyCatalogView.Pharmacy, rbac.BuyCatalogView.Vendor},
			Timeout:     20 * time.Second,
			Handler:     r.findOffers,
		},
	}
}

// actionSpec renders propose_action for one caller, or reports that the caller
// has no actions at all, in which case the tool is not offered.
func (r *Registry) actionSpec(actor authctx.Actor) (gateway.ToolSpec, bool) {
	defs := r.flow.Available(actor)
	if len(defs) == 0 {
		return gateway.ToolSpec{}, false
	}
	sort.Slice(defs, func(i, j int) bool { return defs[i].Name < defs[j].Name })
	names := make([]string, len(defs))
	var b strings.Builder
	b.WriteString("Prepare a change for the user to confirm. Nothing happens until they press confirm on the card; ")
	b.WriteString("never tell them it is done. Look records up first so refs and quantities are right. Available actions:\n")
	for i, d := range defs {
		names[i] = d.Name
		fmt.Fprintf(&b, "- %s — %s\n", actions.Signature(d), d.Description)
	}
	return gateway.ToolSpec{
		Name:        "propose_action",
		Description: b.String(),
		Parameters: objectSchema(map[string]any{
			"action": enumProp("The action to prepare.", names...),
			"args":   map[string]any{"type": "object", "description": "Arguments exactly as in the action's signature."},
		}, "action"),
	}, true
}

func (r *Registry) proposeAction(ctx context.Context, actor authctx.Actor, raw json.RawMessage) (Result, error) {
	if r.flow == nil {
		return Result{Note: "تنفيذ الإجراءات غير متاح حالياً."}, nil
	}
	var args struct {
		Action string          `json:"action"`
		Args   json.RawMessage `json:"args"`
	}
	if err := decode(raw, &args); err != nil {
		return Result{}, err
	}
	turn := actions.TurnFrom(ctx)

	pending, err := r.flow.Propose(ctx, actor, actions.ProposeRequest{
		Action:         strings.TrimSpace(args.Action),
		Args:           args.Args,
		ConversationID: turn.ConversationID,
		Channel:        turn.Channel,
		Resolve: func(kind handles.Kind, token string) (int64, error) {
			return r.resolveHandle(actor, kind, token)
		},
	})
	var invalid *actions.ErrInvalid
	switch {
	case err == nil:
	case errors.Is(err, actions.ErrNotAllowed):
		return Result{Note: "هذا الإجراء غير متاح لهذا المستخدم."}, nil
	case errors.Is(err, actions.ErrBadRef):
		return Result{}, errBadHandle
	case errors.As(err, &invalid):
		return Result{}, badArgs("%s", invalid.Message)
	default:
		if refusal, ok := actions.AsRefusal(err); ok {
			return Result{Note: refusal.Message}, nil
		}
		return Result{}, err
	}

	data := map[string]any{
		"status":  "awaiting_confirmation",
		"title":   pending.Preview.Title,
		"summary": pending.Preview.Summary,
		"note":    "A confirmation card is shown to the user. It has NOT happened yet; tell them what will happen once they confirm.",
	}
	if len(pending.Preview.Warnings) > 0 {
		data["warnings"] = pending.Preview.Warnings
	}
	return Result{Data: data, Rows: 1, Entities: []assistant.Entity{assistant.ProposalEntity(pending)}}, nil
}

func (r *Registry) findOffers(ctx context.Context, actor authctx.Actor, raw json.RawMessage) (Result, error) {
	if r.offers == nil {
		return Result{Note: "البحث في العروض المتاحة للشراء غير متاح حالياً."}, nil
	}
	var args struct {
		Search         string `json:"search"`
		Branch         string `json:"branch"`
		OnlyDiscounted bool   `json:"only_discounted"`
		Limit          int    `json:"limit"`
	}
	if err := decode(raw, &args); err != nil {
		return Result{}, err
	}
	search, err := trimSearch(args.Search)
	if err != nil {
		return Result{}, err
	}
	if search == "" {
		return Result{}, badArgs("حدّد اسم الصنف المطلوب.")
	}
	q := assistant.OfferQuery{Search: search, OnlyDiscounted: args.OnlyDiscounted, Limit: args.Limit}
	if q.Limit <= 0 || q.Limit > 40 {
		q.Limit = 20
	}
	if args.Branch != "" {
		if q.BranchID, err = r.resolveHandle(actor, handles.KindBranch, args.Branch); err != nil {
			return Result{}, err
		}
	}
	res, err := r.offers.FindOffers(ctx, actor, q)
	if err != nil {
		if refusal, ok := actions.AsRefusal(err); ok {
			return Result{Note: refusal.Message}, nil
		}
		return Result{}, err
	}
	if res == nil || len(res.Offers) == 0 {
		return Result{Note: "لا توجد عروض مطابقة يمكن طلبها لهذا الفرع الآن."}, nil
	}
	var entities []assistant.Entity
	for i := range res.Offers {
		o := &res.Offers[i]
		o.Ref = r.issue(actor, handles.KindVariant, o.VariantID)
		if e := assistant.RecordEntity(assistant.EntityProduct, o.ProductID, o.Product); e.ID > 0 {
			entities = append(entities, e)
		}
	}
	return Result{Data: res, Rows: len(res.Offers), Entities: entities}, nil
}
