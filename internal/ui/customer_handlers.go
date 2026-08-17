package ui

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/modules/identity"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/money"
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

	var minPrice, maxPrice *money.Amount
	if v := r.URL.Query().Get("min_price"); v != "" {
		if a, err := money.Parse(v); err == nil {
			minPrice = &a
		}
	}
	if v := r.URL.Query().Get("max_price"); v != "" {
		if a, err := money.Parse(v); err == nil {
			maxPrice = &a
		}
	}

	products, err := h.catSvc.Search(ctx, catalog.SearchParams{
		Query:    query,
		Sort:     r.URL.Query().Get("sort"),
		MinPrice: minPrice,
		MaxPrice: maxPrice,
		Limit:    h.pageLimit(r),
		Offset:   h.pageOffset(r),
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

func (h *UIHandler) AddToCartSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID, err := authctx.UserID(ctx)
	if err != nil {
		http.Redirect(w, r, "/auth/login?redirect=/cart", http.StatusSeeOther)
		return
	}

	if h.commSvc == nil {
		http.Redirect(w, r, "/cart", http.StatusSeeOther)
		return
	}

	variantID, _ := strconv.ParseInt(r.PostFormValue("variant_id"), 10, 64)
	productID, _ := strconv.ParseInt(r.PostFormValue("product_id"), 10, 64)
	qty, _ := strconv.Atoi(r.PostFormValue("quantity"))
	if qty <= 0 {
		qty = 1
	}

	item := &commerce.CartItem{
		ProductID:        productID,
		ProductVariantID: variantID,
		Quantity:         qty,
	}

	_, _ = h.commSvc.AddToCart(ctx, userID, item)
	http.Redirect(w, r, "/cart", http.StatusSeeOther)
}

func (h *UIHandler) RemoveFromCartSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID, err := authctx.UserID(ctx)
	if err != nil {
		http.Redirect(w, r, "/auth/login?redirect=/cart", http.StatusSeeOther)
		return
	}

	if h.commSvc == nil {
		http.Redirect(w, r, "/cart", http.StatusSeeOther)
		return
	}

	variantID, _ := strconv.ParseInt(r.PostFormValue("variant_id"), 10, 64)
	_, _ = h.commSvc.RemoveFromCart(ctx, userID, variantID)
	http.Redirect(w, r, "/cart", http.StatusSeeOther)
}

func (h *UIHandler) CheckoutSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID, err := authctx.UserID(ctx)
	if err != nil {
		http.Redirect(w, r, "/auth/login?redirect=/checkout", http.StatusSeeOther)
		return
	}

	if h.commSvc == nil {
		http.Redirect(w, r, "/orders", http.StatusSeeOther)
		return
	}

	cart, err := h.commSvc.GetCart(ctx, userID)
	if err != nil || cart == nil || len(cart.Items) == 0 {
		http.Redirect(w, r, "/cart", http.StatusSeeOther)
		return
	}

	var items []commerce.CheckoutLineItem
	for _, it := range cart.Items {
		pID := it.ProductID
		vID := it.ProductVariantID
		items = append(items, commerce.CheckoutLineItem{
			VendorOrgID:      1,
			ProductID:        &pID,
			ProductVariantID: &vID,
			Quantity:         it.Quantity,
			UnitPrice:        it.UnitPrice,
		})
	}

	paymentMethod := r.PostFormValue("payment_method")
	if paymentMethod == "" {
		paymentMethod = "cod"
	}

	order, err := h.commSvc.Checkout(ctx, commerce.CheckoutInput{
		CustomerID:    userID,
		PaymentMethod: paymentMethod,
		Notes:         r.PostFormValue("notes"),
		Items:         items,
	})
	if err != nil {
		h.renderError(w, r, err)
		return
	}

	_ = h.commSvc.ClearCart(ctx, userID)
	http.Redirect(w, r, "/orders/"+strconv.FormatInt(order.ID, 10), http.StatusSeeOther)
}

func (h *UIHandler) MarkNotificationReadSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID, _ := authctx.UserID(ctx)
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if h.notifSvc != nil && id > 0 {
		_ = h.notifSvc.MarkRead(ctx, id, userID)
	}
	http.Redirect(w, r, "/notifications", http.StatusSeeOther)
}
