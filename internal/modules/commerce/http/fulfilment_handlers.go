package http

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/httpx"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

func commerceID(r *http.Request, name, entity string) (int64, error) {
	id, err := strconv.ParseInt(chi.URLParam(r, name), 10, 64)
	if err != nil || id <= 0 {
		return 0, apperr.Validation("id.invalid", "Invalid "+entity+" identifier.",
			map[string]string{name: "must be a positive integer"})
	}
	return id, nil
}

// SetCartQuantity changes the quantity of a cart line in place.
func (h *Handler) SetCartQuantity(w http.ResponseWriter, r *http.Request) {
	userID, err := authctx.UserID(r.Context())
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	variantID, err := commerceID(r, "variantId", "variant")
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	var body struct {
		Quantity int `json:"quantity"`
	}
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	cart, err := h.service.SetCartQuantity(r.Context(), userID, variantID, body.Quantity)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, cart)
}

// GetShipment returns one vendor shipment.
func (h *Handler) GetShipment(w http.ResponseWriter, r *http.Request) {
	id, err := commerceID(r, "id", "shipment")
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	shipment, err := h.service.GetShipment(r.Context(), id)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, shipment)
}

// TransitionShipmentStatus advances a vendor's shipment through fulfilment.
func (h *Handler) TransitionShipmentStatus(w http.ResponseWriter, r *http.Request) {
	id, err := commerceID(r, "id", "shipment")
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	var body struct {
		Status string `json:"status"`
		Notes  string `json:"notes"`
	}
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	var actor *int64
	if uid, err := authctx.UserID(r.Context()); err == nil {
		actor = &uid
	}

	shipment, err := h.service.TransitionShipmentStatus(
		r.Context(), id, commerce.OrderStatus(body.Status), actor, body.Notes)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, shipment)
}

// GetOrderHistory returns the status audit trail for an order.
func (h *Handler) GetOrderHistory(w http.ResponseWriter, r *http.Request) {
	id, err := commerceID(r, "id", "order")
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	history, err := h.service.GetOrderHistory(r.Context(), id)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"history": history})
}

// RateOrder records a customer rating for a delivered order.
func (h *Handler) RateOrder(w http.ResponseWriter, r *http.Request) {
	userID, err := authctx.UserID(r.Context())
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	id, err := commerceID(r, "id", "order")
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	var body struct {
		Rating        *float64 `json:"rating,omitempty"`
		RepRating     int      `json:"rep_rating,omitempty"`
		SpeedRating   int      `json:"speed_rating,omitempty"`
		QualityRating int      `json:"quality_rating,omitempty"`
		Review        string   `json:"review"`
	}
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	var errRate error
	if body.RepRating > 0 && body.SpeedRating > 0 && body.QualityRating > 0 {
		_, errRate = h.service.RateOrderWithCriteria(r.Context(), id, userID, body.RepRating, body.SpeedRating, body.QualityRating, body.Review)
	} else if body.Rating != nil {
		errRate = h.service.RateOrder(r.Context(), id, userID, *body.Rating, body.Review)
	} else {
		httpx.Error(w, r, h.log, apperr.Validation("rating.required", "Either rating or 3 review criteria ratings must be provided", nil))
		return
	}

	if errRate != nil {
		httpx.Error(w, r, h.log, errRate)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
