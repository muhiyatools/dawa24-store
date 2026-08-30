package ui

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/billing"
	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// VendorPaymentsPage renders the payments received by the vendor.
func (h *UIHandler) VendorPaymentsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/payments", http.StatusSeeOther)
		return
	}

	var payments []*billing.Payment
	if h.billSvc != nil {
		payments, _ = h.billSvc.ListPayments(ctx, actor.OrganizationID, 50, 0)
	}

	h.renderPage(ctx, w, "render vendor payments", pages.VendorPaymentsPage(payments, lang, dir))
}

// VendorEarningsOrderPage renders orders revenue and comprehensive net profit report for the vendor.
func (h *UIHandler) VendorEarningsOrderPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/earnings/order", http.StatusSeeOther)
		return
	}

	period := r.URL.Query().Get("period")
	if period == "" {
		period = "month"
	}

	var summary *commerce.VendorFinancialSummary
	if h.commSvc != nil {
		summary, _ = h.commSvc.GetVendorFinancialSummary(ctx, actor.OrganizationID, period)
	}
	if summary == nil {
		summary = &commerce.VendorFinancialSummary{Period: period}
	}

	h.renderPage(ctx, w, "render vendor earnings order", pages.VendorEarningsOrderPage(summary, lang, dir))
}

// VendorEarningsOffersPage renders offers revenue and commissions for the vendor.
func (h *UIHandler) VendorEarningsOffersPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/earnings/offers", http.StatusSeeOther)
		return
	}

	h.renderPage(ctx, w, "render vendor earnings offers", pages.VendorEarningsOffersPage(money.Zero, lang, dir))
}

// VendorOfferOrdersPage renders offer-based orders fulfilled by the vendor.
func (h *UIHandler) VendorOfferOrdersPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/orders/offers", http.StatusSeeOther)
		return
	}

	var shipments []*commerce.OrderShipment
	if h.commSvc != nil {
		shipments, _ = h.commSvc.ListVendorShipments(ctx, actor.OrganizationID, 100, 0)
	}

	h.renderPage(ctx, w, "render vendor offer orders", pages.VendorOfferOrdersPage(shipments, lang, dir))
}

// VendorOfferOrderDetailPage renders single offer order details.
func (h *UIHandler) VendorOfferOrderDetailPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	idStr := chi.URLParam(r, "id")
	shipID, _ := strconv.ParseInt(idStr, 10, 64)

	h.renderPage(ctx, w, "render vendor offer order detail", pages.VendorOfferOrderDetailPage(shipID, lang, dir))
}
