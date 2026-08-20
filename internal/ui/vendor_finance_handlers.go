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

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.VendorPaymentsPage(payments, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render vendor payments", "error", err)
	}
}

// VendorEarningsOrderPage renders orders revenue and commissions report for the vendor.
func (h *UIHandler) VendorEarningsOrderPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/earnings/order", http.StatusSeeOther)
		return
	}

	var revenue money.Amount
	if h.commSvc != nil {
		revenue, _ = h.commSvc.MonthSalesByVendor(ctx, actor.OrganizationID)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.VendorEarningsOrderPage(revenue, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render vendor earnings order", "error", err)
	}
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

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.VendorEarningsOffersPage(money.Zero, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render vendor earnings offers", "error", err)
	}
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

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.VendorOfferOrdersPage(shipments, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render vendor offer orders", "error", err)
	}
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

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.VendorOfferOrderDetailPage(shipID, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render vendor offer order detail", "error", err)
	}
}
