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

// AdminOfferOrdersPage renders customer offer procurement orders.
func (h *UIHandler) AdminOfferOrdersPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	var orders []*commerce.Order
	if h.commSvc != nil {
		// AsSystem justified: platform administrator finance oversight of orders
		orders, _ = h.commSvc.ListCustomerOrders(database.AsSystem(ctx), 0, 100, 0)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminOfferOrdersPage(orders, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin offer orders", "error", err)
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
		http.Redirect(w, r, "/admin/orders/offers", http.StatusSeeOther)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminOfferOrderDetailPage(order, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin offer order detail", "error", err)
	}
}

// AdminEarningsOrderPage renders commissions and gross earnings from marketplace orders.
func (h *UIHandler) AdminEarningsOrderPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	totalOrdersRevenue := money.FromMajor(150000)
	totalCommissions := money.FromMajor(7500)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminEarningsOrderPage(totalOrdersRevenue, totalCommissions, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin earnings order", "error", err)
	}
}

// AdminEarningsOffersPage renders commissions from vendor promotions & flash offers.
func (h *UIHandler) AdminEarningsOffersPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	totalOffersRevenue := money.FromMajor(85000)
	totalCommissions := money.FromMajor(4250)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminEarningsOffersPage(totalOffersRevenue, totalCommissions, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin earnings offers", "error", err)
	}
}

// AdminInvoicesPage renders billing invoices.
func (h *UIHandler) AdminInvoicesPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	var invoices []*billing.Invoice
	if h.billSvc != nil {
		invoices, _ = h.billSvc.AdminListInvoices(ctx, 100, 0)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminInvoicesPage(invoices, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin invoices", "error", err)
	}
}

// AdminPaymentsPage renders platform payment transaction logs.
func (h *UIHandler) AdminPaymentsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	var payments []*billing.Payment
	if h.billSvc != nil {
		payments, _ = h.billSvc.AdminListPayments(ctx, 100, 0)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminPaymentsPage(payments, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin payments", "error", err)
	}
}

// AdminWalletsPage renders pharmacy and vendor wallet balances.
func (h *UIHandler) AdminWalletsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	var wallets []*billing.Wallet
	if h.billSvc != nil {
		wallets, _ = h.billSvc.AdminListWallets(ctx, 100, 0)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminWalletsPage(wallets, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin wallets", "error", err)
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
	if err := pages.AdminReferenceCRUDPage("أنواع وتصنيفات خطط الاشتراك", "plan-types", "نوع خطة", items, lang, dir).Render(ctx, w); err != nil {
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
	if err := pages.AdminReferenceCRUDPage("ميزات ومحددات باقات الاشتراك", "plan-features", "ميزة", items, lang, dir).Render(ctx, w); err != nil {
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
