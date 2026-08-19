package ui

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/modules/promo"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// CustomerCPanelPage renders account and organization operational dashboard summary.
func (h *UIHandler) CustomerCPanelPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/customer/cpanel", http.StatusSeeOther)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.CustomerCPanelPage(actor.OrganizationID, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render customer cpanel", "error", err)
	}
}

// CustomerSavingProductsPage renders the customer's live price-delta tracking list.
func (h *UIHandler) CustomerSavingProductsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/customer/saving-products", http.StatusSeeOther)
		return
	}

	search := r.URL.Query().Get("q")

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.CustomerSavingProductsPage(search, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render customer saving products", "error", err)
	}
}

// CustomerSavingProductsImportPage renders bulk spreadsheet upload for customer saving products.
func (h *UIHandler) CustomerSavingProductsImportPage(w http.ResponseWriter, r *http.Request) {
	lang, dir := h.localeAndDir(r)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = pages.CustomerSavingProductsImportPage(lang, dir).Render(r.Context(), w)
}

// CustomerSavingProductDetailPage renders single saving product delta details.
func (h *UIHandler) CustomerSavingProductDetailPage(w http.ResponseWriter, r *http.Request) {
	lang, dir := h.localeAndDir(r)
	idStr := chi.URLParam(r, "id")
	id, _ := strconv.ParseInt(idStr, 10, 64)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = pages.CustomerSavingProductDetailPage(id, lang, dir).Render(r.Context(), w)
}

// CustomerSavingProductsAlias redirects misspelled route /customer/saveing-products.
func (h *UIHandler) CustomerSavingProductsAlias(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/customer/saving-products", http.StatusMovedPermanently)
}

// CustomerOfferOrdersPage renders offer-based orders placed by the pharmacy.
func (h *UIHandler) CustomerOfferOrdersPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/orders/offers", http.StatusSeeOther)
		return
	}

	var orders []*commerce.Order
	if h.commSvc != nil {
		orders, _ = h.commSvc.ListCustomerOrders(ctx, actor.OrganizationID, 100, 0)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.CustomerOfferOrdersPage(orders, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render customer offer orders", "error", err)
	}
}

// CustomerOfferOrderDetailPage renders single offer order details.
func (h *UIHandler) CustomerOfferOrderDetailPage(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	http.Redirect(w, r, fmt.Sprintf("/orders/%s", idStr), http.StatusSeeOther)
}

// CustomerOfferCheckoutPage renders dedicated checkout flow for a specific offer.
func (h *UIHandler) CustomerOfferCheckoutPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/checkout", http.StatusSeeOther)
		return
	}

	idStr := chi.URLParam(r, "id")
	offerID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || offerID <= 0 {
		http.Redirect(w, r, "/offers", http.StatusSeeOther)
		return
	}

	var offer *promo.Offer
	if h.promoSvc != nil {
		offer, _ = h.promoSvc.GetOffer(database.AsSystem(ctx), offerID)
	}

	if offer == nil {
		http.Redirect(w, r, "/offers", http.StatusSeeOther)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.CustomerOfferCheckoutPage(offer, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render offer checkout", "error", err)
	}
}

// CustomerAddOrderPage renders manual / quick order entry form.
func (h *UIHandler) CustomerAddOrderPage(w http.ResponseWriter, r *http.Request) {
	lang, dir := h.localeAndDir(r)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = pages.CustomerAddOrderPage(lang, dir).Render(r.Context(), w)
}

// CustomerProductsMainAlias redirects /customer/products/main/{id} to /catalog/{id}.
func (h *UIHandler) CustomerProductsMainAlias(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	http.Redirect(w, r, fmt.Sprintf("/catalog/%s", idStr), http.StatusMovedPermanently)
}

// GuestOrderTrackingPage renders unauthenticated public order tracking screen.
func (h *UIHandler) GuestOrderTrackingPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	orderNumber := r.URL.Query().Get("order_number")
	var order *commerce.Order
	if orderNumber != "" && h.commSvc != nil {
		order, _ = h.commSvc.GetOrderByNumber(database.AsSystem(ctx), orderNumber)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.GuestOrderTrackingPage(orderNumber, order, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render guest tracking", "error", err)
	}
}
