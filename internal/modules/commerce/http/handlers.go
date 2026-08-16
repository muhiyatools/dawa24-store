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
	r.Get("/api/v1/commerce/vendor/shipments", h.ListVendorShipments)
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
