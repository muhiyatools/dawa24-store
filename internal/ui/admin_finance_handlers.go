package ui

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/billing"
	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// AdminFinancePage renders the unified financial management hub.
func (h *UIHandler) AdminFinancePage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	tab := r.URL.Query().Get("tab")
	if tab == "" {
		tab = "invoices"
	}

	var invoices []*billing.Invoice
	var payments []*billing.Payment
	var wallets []*billing.Wallet

	if h.billSvc != nil {
		invoices, _ = h.billSvc.AdminListInvoices(ctx, 100, 0)
		payments, _ = h.billSvc.AdminListPayments(ctx, 100, 0)
		wallets, _ = h.billSvc.AdminListWallets(ctx, 100, 0)
	}

	var totalRevenueMinor int64
	var totalPaidMinor int64
	for _, p := range payments {
		if p.Status == "paid" || p.Status == "completed" || p.Status == "success" {
			totalPaidMinor += p.Amount.Minor()
		}
	}
	for _, inv := range invoices {
		if inv.Status != "cancelled" {
			totalRevenueMinor += inv.TotalAmount.Minor()
		}
	}
	if totalRevenueMinor == 0 && totalPaidMinor > 0 {
		totalRevenueMinor = totalPaidMinor
	}

	totalPaid := money.FromMinor(totalPaidMinor)
	totalRevenue := money.FromMinor(totalRevenueMinor)

	// Platform commission: 5% of gross volume
	commission := money.FromMinor(totalRevenueMinor * 5 / 100)

	var totalHeldMinor int64
	for _, w := range wallets {
		totalHeldMinor += w.Balance.Minor()
	}
	totalHeld := money.FromMinor(totalHeldMinor)

	data := pages.AdminFinanceData{
		ActiveTab:       tab,
		Invoices:        invoices,
		Payments:        payments,
		Wallets:         wallets,
		TotalRevenue:    totalRevenue,
		TotalCommission: commission,
		TotalPaid:       totalPaid,
		TotalHeld:       totalHeld,
		Query:           r.URL.Query().Get("q"),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminFinance(data, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin finance hub", "error", err)
	}
}

// AdminOfferOrderDetailPage renders single offer order details.
func (h *UIHandler) AdminOfferOrderDetailPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	idStr := chi.URLParam(r, "id")
	orderID, _ := strconv.ParseInt(idStr, 10, 64)

	var order *commerce.Order
	if h.commSvc != nil && orderID > 0 {
		order, _ = h.commSvc.GetOrder(database.AsSystem(ctx), orderID)
	}

	if order == nil {
		http.Redirect(w, r, "/admin/orders?tab=offers", http.StatusSeeOther)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminOfferOrderDetailPage(order, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin offer order detail", "error", err)
	}
}

// AdminPlansInfoPage renders subscription plan tiers directory.
func (h *UIHandler) AdminPlansInfoPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	var plans []*billing.Plan
	if h.billSvc != nil {
		plans, _ = h.billSvc.ListPlans(ctx)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminPlansInfoPage(plans, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin plans info", "error", err)
	}
}

// AdminPlanTypesPage renders subscription plan types CRUD.
func (h *UIHandler) AdminPlanTypesPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	var items []pages.ReferenceItem
	if h.billSvc != nil {
		plans, _ := h.billSvc.ListPlans(ctx)
		for _, p := range plans {
			items = append(items, pages.ReferenceItem{
				ID:          p.ID,
				Name:        p.Name.Get(i18n.Lang(lang)),
				Description: p.Slug,
				Status:      "active",
				Extra:       p.Description.Get(i18n.Lang(lang)),
			})
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminReferenceCRUDPage("أنواع وتصنيفات خطط الاشتراك", "plan-types", "نوع خطة", items, "plans", lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin plan types", "error", err)
	}
}

// AdminPlanFeaturesPage renders feature matrix for subscription plans.
func (h *UIHandler) AdminPlanFeaturesPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	var items []pages.ReferenceItem
	if h.billSvc != nil {
		plans, _ := h.billSvc.ListPlans(ctx)
		for _, p := range plans {
			for k, v := range p.Features {
				items = append(items, pages.ReferenceItem{
					ID:          p.ID,
					Name:        k,
					Description: p.Name.Get(i18n.Lang(lang)),
					Status:      "active",
					Extra:       v,
				})
			}
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminReferenceCRUDPage("ميزات ومحددات باقات الاشتراك", "plan-features", "ميزة", items, "plans", lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin plan features", "error", err)
	}
}

// AdminPlansSubscriptionsPage renders active subscriptions and subscriber histories.
func (h *UIHandler) AdminPlansSubscriptionsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	var subs []*billing.Subscription
	if h.billSvc != nil {
		subs, _ = h.billSvc.AdminListSubscriptions(ctx, 100, 0)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminPlansSubscriptionsPage(subs, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin plan subscriptions", "error", err)
	}
}
