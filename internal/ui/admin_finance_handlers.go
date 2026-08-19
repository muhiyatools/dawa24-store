package ui

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/platform/database"
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
	_ = pages.AdminOfferOrderDetailPage(order, lang, dir).Render(ctx, w)
}

// AdminEarningsOrderPage renders commissions and gross earnings from marketplace orders.
func (h *UIHandler) AdminEarningsOrderPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	totalOrdersRevenue := money.FromMajor(150000)
	totalCommissions := money.FromMajor(7500)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = pages.AdminEarningsOrderPage(totalOrdersRevenue, totalCommissions, lang, dir).Render(ctx, w)
}

// AdminEarningsOffersPage renders commissions from vendor promotions & flash offers.
func (h *UIHandler) AdminEarningsOffersPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	totalOffersRevenue := money.FromMajor(85000)
	totalCommissions := money.FromMajor(4250)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = pages.AdminEarningsOffersPage(totalOffersRevenue, totalCommissions, lang, dir).Render(ctx, w)
}

// AdminInvoicesPage renders billing invoices.
func (h *UIHandler) AdminInvoicesPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = pages.AdminInvoicesPage(lang, dir).Render(ctx, w)
}

// AdminPaymentsPage renders platform payment transaction logs.
func (h *UIHandler) AdminPaymentsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = pages.AdminPaymentsPage(lang, dir).Render(ctx, w)
}

// AdminWalletsPage renders pharmacy and vendor wallet balances.
func (h *UIHandler) AdminWalletsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = pages.AdminWalletsPage(lang, dir).Render(ctx, w)
}

// AdminPlansInfoPage renders subscription plan tiers directory.
func (h *UIHandler) AdminPlansInfoPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = pages.AdminPlansInfoPage(lang, dir).Render(ctx, w)
}

// AdminPlanTypesPage renders subscription plan types CRUD.
func (h *UIHandler) AdminPlanTypesPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	items := []pages.ReferenceItem{
		{ID: 1, Name: "باقات اشتراك الصيدليات", Description: "pharmacy_basic, pharmacy_pro", Status: "active", Extra: "نوع العميل: صيدلية"},
		{ID: 2, Name: "باقات اشتراك الموردين والمستودعات", Description: "vendor_standard, vendor_enterprise", Status: "active", Extra: "نوع العميل: مورد"},
		{ID: 3, Name: "باقات المستودعات المؤقتة", Description: "temp_warehouse_tier1, temp_warehouse_tier2", Status: "active", Extra: "نوع الخدمة: تخزين إضافي"},
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = pages.AdminReferenceCRUDPage("أنواع وتصنيفات خطط الاشتراك", "plan-types", "نوع خطة", items, lang, dir).Render(ctx, w)
}

// AdminPlanFeaturesPage renders feature matrix for subscription plans.
func (h *UIHandler) AdminPlanFeaturesPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	items := []pages.ReferenceItem{
		{ID: 1, Name: "محرك مقارنة الأسعار المتقدم", Description: "compare_engine_unlimited", Status: "active", Extra: "كود: CMP_ADV"},
		{ID: 2, Name: "الربط الآلي عبر API للمخزون", Description: "api_inventory_sync", Status: "active", Extra: "كود: API_SYNC"},
		{ID: 3, Name: "مساعد الذكاء الاصطناعي للمطابقة", Description: "ai_matching_assistant", Status: "active", Extra: "كود: AI_MATCH"},
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = pages.AdminReferenceCRUDPage("ميزات ومحددات باقات الاشتراك", "plan-features", "ميزة", items, lang, dir).Render(ctx, w)
}

// AdminPlansSubscriptionsPage renders active subscriptions and subscriber histories.
func (h *UIHandler) AdminPlansSubscriptionsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = pages.AdminPlansSubscriptionsPage(lang, dir).Render(ctx, w)
}
