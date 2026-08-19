package ui

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/workflow"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// CustomerPurchasePriorityPage renders the Purchase Priority Engine form and execution history (Plan V5 Phase 3 §3.2.4).
func (h *UIHandler) CustomerPurchasePriorityPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/customer/purchase-priority", http.StatusSeeOther)
		return
	}

	var history []*workflow.PurchasePriorityRequest
	if h.wfSvc != nil {
		reqs, err := h.wfSvc.ListPriorityEngines(ctx, actor.UserID, 20, 0)
		if err == nil {
			history = reqs
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.CustomerPurchasePriorityPage(lang, dir, history, nil).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render purchase priority page", "error", err)
	}
}

// CustomerPurchasePriorityRunSubmit executes the Purchase Priority Engine computation.
func (h *UIHandler) CustomerPurchasePriorityRunSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/customer/purchase-priority", http.StatusSeeOther)
		return
	}

	if err := r.ParseForm(); err != nil {
		h.redirectWithNotice(w, r, "/customer/purchase-priority", "error", "تعذر قراءة بيانات النموذج.")
		return
	}

	highestDiscount := r.FormValue("priority_highest_discount") == "true" || r.FormValue("priority_highest_discount") == "on" || r.FormValue("priority_highest_discount") == "1"
	lowestPrice := r.FormValue("priority_lowest_price") == "true" || r.FormValue("priority_lowest_price") == "on" || r.FormValue("priority_lowest_price") == "1"
	fastestDelivery := r.FormValue("priority_fastest_delivery") == "true" || r.FormValue("priority_fastest_delivery") == "on" || r.FormValue("priority_fastest_delivery") == "1"
	preferredSuppliers := r.FormValue("priority_preferred_suppliers_only") == "true" || r.FormValue("priority_preferred_suppliers_only") == "on" || r.FormValue("priority_preferred_suppliers_only") == "1"

	var budgetPtr *money.Amount
	if bStr := r.FormValue("budget_constraint"); bStr != "" {
		if b, err := money.Parse(bStr); err == nil && b.IsPositive() {
			budgetPtr = &b
		}
	}

	prefs := workflow.Priorities{
		PriorityHighestDiscount:        highestDiscount,
		PriorityLowestPrice:            lowestPrice,
		PriorityFastestDelivery:        fastestDelivery,
		PriorityPreferredSuppliersOnly: preferredSuppliers,
		BudgetConstraint:               budgetPtr,
	}

	var orgPtr *int64
	if actor.OrganizationID > 0 {
		orgPtr = &actor.OrganizationID
	}

	if h.wfSvc != nil {
		created, err := h.wfSvc.CreatePriorityEngine(ctx, actor.UserID, orgPtr, prefs)
		if err != nil {
			h.redirectWithNotice(w, r, "/customer/purchase-priority", "error", h.safeMessage(err, langOf(r)))
			return
		}

		// Process priority engine synchronously for immediate recommendations
		responderID := actor.UserID
		summary, err := h.wfSvc.ProcessPriorityEngine(ctx, created.ID, &responderID)
		if err != nil {
			h.log.WarnContext(ctx, "process priority engine background/async fallback", "id", created.ID, "error", err)
		}

		reqs, _ := h.wfSvc.ListPriorityEngines(ctx, actor.UserID, 20, 0)

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := pages.CustomerPurchasePriorityPage(lang, dir, reqs, summary).Render(ctx, w); err != nil {
			h.log.ErrorContext(ctx, "render purchase priority page after run", "error", err)
		}
		return
	}

	h.redirectWithNotice(w, r, "/customer/purchase-priority", "success", "تم تشغيل محرك أولويات الشراء بنجاح.")
}

// CustomerPurchasePriorityDetailPage displays results and recommendations for a previous priority engine run.
func (h *UIHandler) CustomerPurchasePriorityDetailPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/customer/purchase-priority", http.StatusSeeOther)
		return
	}

	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		http.Redirect(w, r, "/customer/purchase-priority", http.StatusSeeOther)
		return
	}

	var req *workflow.PurchasePriorityRequest
	if h.wfSvc != nil {
		rItem, err := h.wfSvc.GetPriorityRequest(ctx, id)
		if err == nil && rItem.UserID == actor.UserID {
			req = rItem
		}
	}
	if req == nil {
		http.Redirect(w, r, "/customer/purchase-priority", http.StatusSeeOther)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.CustomerPurchasePriorityDetailPage(lang, dir, req).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render priority detail page", "error", err)
	}
}
