package ui

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/modules/identity"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

func (h *UIHandler) CustomerCatalogPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	query := r.URL.Query().Get("q")
	lang, dir := h.localeAndDir(r)

	if h.catSvc == nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = pages.CustomerCatalog(nil, nil, query, lang, dir, h.isHTMX(r)).Render(ctx, w)
		return
	}

	products, err := h.catSvc.Search(ctx, catalog.SearchParams{
		Query:  query,
		Limit:  h.pageLimit(r),
		Offset: h.pageOffset(r),
	})
	if err != nil {
		h.renderError(w, r, err)
		return
	}

	categories, _ := h.catSvc.ListCategories(ctx)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.CustomerCatalog(products, categories, query, lang, dir, h.isHTMX(r)).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render catalog page", "error", err)
	}
}

func (h *UIHandler) CustomerProductDetailPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		h.renderError(w, r, err)
		return
	}

	if h.catSvc == nil {
		h.renderError(w, r, http.ErrNotSupported)
		return
	}

	product, variants, err := h.catSvc.GetProduct(ctx, id)
	if err != nil {
		h.renderError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.CustomerProductDetail(product, variants, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render product detail page", "error", err)
	}
}

func (h *UIHandler) CustomerCartPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	userID, err := authctx.UserID(ctx)
	if err != nil || h.commSvc == nil {
		// If unauthenticated or commSvc not wired, show empty cart
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = pages.CustomerCart(nil, lang, dir, h.isHTMX(r)).Render(ctx, w)
		return
	}

	cart, err := h.commSvc.GetCart(ctx, userID)
	if err != nil {
		h.renderError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.CustomerCart(cart, lang, dir, h.isHTMX(r)).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render cart page", "error", err)
	}
}

func (h *UIHandler) CustomerCheckoutPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	userID, err := authctx.UserID(ctx)
	if err != nil {
		http.Redirect(w, r, "/auth/login?redirect=/checkout", http.StatusSeeOther)
		return
	}

	if h.commSvc == nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = pages.CustomerCheckout(nil, nil, lang, dir).Render(ctx, w)
		return
	}

	cart, err := h.commSvc.GetCart(ctx, userID)
	if err != nil {
		h.renderError(w, r, err)
		return
	}

	var addrs []*identity.UserAddress
	if h.idSvc != nil {
		addrs, _ = h.idSvc.ListAddresses(ctx, userID)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.CustomerCheckout(cart, addrs, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render checkout page", "error", err)
	}
}

func (h *UIHandler) CustomerOrdersPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	userID, err := authctx.UserID(ctx)
	if err != nil {
		http.Redirect(w, r, "/auth/login?redirect=/orders", http.StatusSeeOther)
		return
	}

	if h.commSvc == nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = pages.CustomerOrders(nil, lang, dir, h.isHTMX(r)).Render(ctx, w)
		return
	}

	orders, err := h.commSvc.ListCustomerOrders(ctx, userID, h.pageLimit(r), h.pageOffset(r))
	if err != nil {
		h.renderError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.CustomerOrders(orders, lang, dir, h.isHTMX(r)).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render customer orders page", "error", err)
	}
}

func (h *UIHandler) CustomerOrderDetailPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		h.renderError(w, r, err)
		return
	}

	if h.commSvc == nil {
		h.renderError(w, r, http.ErrNotSupported)
		return
	}

	order, err := h.commSvc.GetOrder(ctx, id)
	if err != nil {
		h.renderError(w, r, err)
		return
	}

	var history []*commerce.OrderStatusHistory

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.CustomerOrderDetail(order, history, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render customer order detail page", "error", err)
	}
}

func (h *UIHandler) NotificationsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	userID, err := authctx.UserID(ctx)
	if err != nil {
		http.Redirect(w, r, "/auth/login?redirect=/notifications", http.StatusSeeOther)
		return
	}

	if h.notifSvc == nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = pages.Notifications(nil, 0, lang, dir, h.isHTMX(r)).Render(ctx, w)
		return
	}

	logs, err := h.notifSvc.ListUserNotifications(ctx, userID, h.pageLimit(r), h.pageOffset(r))
	if err != nil {
		h.renderError(w, r, err)
		return
	}

	unread, _ := h.notifSvc.GetUnreadCount(ctx, userID)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.Notifications(logs, unread, lang, dir, h.isHTMX(r)).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render notifications page", "error", err)
	}
}
