package http

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/platform/httpx"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// Handler handles commerce HTTP endpoints.
type Handler struct {
	service *commerce.Service
	log     *slog.Logger
}

// NewHandler creates a commerce HTTP handler.
func NewHandler(service *commerce.Service, log *slog.Logger) *Handler {
	return &Handler{service: service, log: log}
}

// RegisterRoutes registers commerce endpoints on a Chi router.
func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Post("/api/v1/commerce/checkout", h.Checkout)
	r.Get("/api/v1/commerce/orders/{id}", h.GetOrder)
	r.Get("/api/v1/commerce/orders", h.ListCustomerOrders)
	r.Post("/api/v1/commerce/orders/{id}/status", h.TransitionStatus)
	r.Post("/api/v1/commerce/orders/{id}/cancel", h.CancelOrder)
	r.Get("/api/v1/commerce/vendor/shipments", h.ListVendorShipments)

	r.Get("/api/v1/commerce/cart", h.GetCart)
	r.Post("/api/v1/commerce/cart/items", h.AddCartItem)
	r.Delete("/api/v1/commerce/cart/items/{variantId}", h.RemoveCartItem)
	r.Delete("/api/v1/commerce/cart", h.ClearCart)

	r.Get("/api/v1/commerce/wishlist", h.GetWishlist)
	r.Post("/api/v1/commerce/wishlist", h.AddToWishlist)
	r.Delete("/api/v1/commerce/wishlist/{productId}", h.RemoveFromWishlist)

	r.Post("/api/v1/commerce/quotes", h.CreateQuoteRequest)
	r.Post("/api/v1/commerce/quotes/{id}/respond", h.RespondQuote)
	r.Get("/api/v1/commerce/quotes", h.ListQuotes)
}

// Checkout handles order placement.
func (h *Handler) Checkout(w http.ResponseWriter, r *http.Request) {
	var input commerce.CheckoutInput
	if err := httpx.DecodeJSON(w, r, &input); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	order, err := h.service.Checkout(r.Context(), input)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	httpx.JSON(w, http.StatusCreated, order)
}

// GetOrder returns order details.
func (h *Handler) GetOrder(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		httpx.Error(w, r, h.log, apperr.Validation("id.invalid", "Invalid order ID", nil))
		return
	}

	order, err := h.service.GetOrder(r.Context(), id)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	httpx.JSON(w, http.StatusOK, order)
}

// ListCustomerOrders returns orders for a customer.
func (h *Handler) ListCustomerOrders(w http.ResponseWriter, r *http.Request) {
	customerIDStr := r.URL.Query().Get("customer_id")
	customerID, err := strconv.ParseInt(customerIDStr, 10, 64)
	if err != nil || customerID <= 0 {
		httpx.Error(w, r, h.log, apperr.Validation("customer_id.invalid", "Valid customer_id query parameter required", nil))
		return
	}

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	orders, err := h.service.ListCustomerOrders(r.Context(), customerID, limit, offset)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	httpx.JSON(w, http.StatusOK, map[string]any{
		"orders": orders,
		"count":  len(orders),
	})
}

// TransitionStatusRequest represents a status change payload.
type TransitionStatusRequest struct {
	Status          commerce.OrderStatus `json:"status"`
	ChangedByUserID *int64               `json:"changed_by_user_id,omitempty"`
	Notes           string               `json:"notes,omitempty"`
}

// TransitionStatus transitions an order status.
func (h *Handler) TransitionStatus(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		httpx.Error(w, r, h.log, apperr.Validation("id.invalid", "Invalid order ID", nil))
		return
	}

	var req TransitionStatusRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	if err := h.service.TransitionOrderStatus(r.Context(), id, req.Status, req.ChangedByUserID, req.Notes); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	httpx.JSON(w, http.StatusOK, map[string]string{
		"status": "success",
	})
}

// ListVendorShipments returns shipments partitioned for a vendor.
func (h *Handler) ListVendorShipments(w http.ResponseWriter, r *http.Request) {
	vendorIDStr := r.URL.Query().Get("vendor_id")
	vendorID, err := strconv.ParseInt(vendorIDStr, 10, 64)
	if err != nil || vendorID <= 0 {
		httpx.Error(w, r, h.log, apperr.Validation("vendor_id.invalid", "Valid vendor_id query parameter required", nil))
		return
	}

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	shipments, err := h.service.ListVendorShipments(r.Context(), vendorID, limit, offset)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	httpx.JSON(w, http.StatusOK, map[string]any{
		"shipments": shipments,
		"count":     len(shipments),
	})
}

// AddToWishlist adds a product to customer wishlist.
func (h *Handler) AddToWishlist(w http.ResponseWriter, r *http.Request) {
	var input struct {
		UserID    int64 `json:"user_id"`
		ProductID int64 `json:"product_id"`
	}
	if err := httpx.DecodeJSON(w, r, &input); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	if err := h.service.AddToWishlist(r.Context(), input.UserID, input.ProductID); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, map[string]string{"status": "added"})
}

// RemoveFromWishlist removes a product from customer wishlist.
func (h *Handler) RemoveFromWishlist(w http.ResponseWriter, r *http.Request) {
	productIDStr := chi.URLParam(r, "productId")
	productID, err := strconv.ParseInt(productIDStr, 10, 64)
	if err != nil {
		httpx.Error(w, r, h.log, apperr.Validation("product_id.invalid", "Invalid product ID", nil))
		return
	}
	userIDStr := r.URL.Query().Get("user_id")
	userID, _ := strconv.ParseInt(userIDStr, 10, 64)
	if err := h.service.RemoveFromWishlist(r.Context(), userID, productID); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "removed"})
}

// GetWishlist returns all wishlist items for a user.
func (h *Handler) GetWishlist(w http.ResponseWriter, r *http.Request) {
	userIDStr := r.URL.Query().Get("user_id")
	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil || userID <= 0 {
		httpx.Error(w, r, h.log, apperr.Validation("user_id.invalid", "Valid user_id required", nil))
		return
	}
	items, err := h.service.GetWishlist(r.Context(), userID)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"wishlist": items, "count": len(items)})
}

// CancelOrder handles customer/admin order cancellation.
func (h *Handler) CancelOrder(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		httpx.Error(w, r, h.log, apperr.Validation("id.invalid", "Invalid order ID", nil))
		return
	}

	var body struct {
		UserID *int64 `json:"user_id,omitempty"`
		Reason string `json:"reason,omitempty"`
	}
	_ = httpx.DecodeJSON(w, r, &body)

	if err := h.service.CancelOrder(r.Context(), id, body.UserID, body.Reason); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "cancelled"})
}
