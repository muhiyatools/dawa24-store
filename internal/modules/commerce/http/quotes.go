package http

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/platform/httpx"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// CreateQuoteRequest creates buyer price inquiry.
func (h *Handler) CreateQuoteRequest(w http.ResponseWriter, r *http.Request) {
	var q commerce.QuoteRequest
	if err := httpx.DecodeJSON(w, r, &q); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	created, err := h.service.CreateQuoteRequest(r.Context(), &q)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	httpx.JSON(w, http.StatusCreated, created)
}

// RespondQuote allows vendor to respond with a quote.
func (h *Handler) RespondQuote(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		httpx.Error(w, r, h.log, apperr.Validation("id.invalid", "Invalid quote ID", nil))
		return
	}

	var body struct {
		Status        string       `json:"status"`
		QuotePrice    money.Amount `json:"quote_price"`
		SupplierNotes string       `json:"supplier_notes"`
	}
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	if err := h.service.RespondToQuote(r.Context(), id, commerce.QuoteStatus(body.Status), body.QuotePrice, body.SupplierNotes); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	httpx.JSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// ListQuotes lists quotes for vendor or buyer.
func (h *Handler) ListQuotes(w http.ResponseWriter, r *http.Request) {
	orgIDStr := r.URL.Query().Get("org_id")
	orgID, err := strconv.ParseInt(orgIDStr, 10, 64)
	if err != nil || orgID <= 0 {
		httpx.Error(w, r, h.log, apperr.Validation("org_id.invalid", "Valid org_id is required", nil))
		return
	}

	isVendor := r.URL.Query().Get("role") == "vendor"
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	list, err := h.service.ListQuoteRequests(r.Context(), orgID, isVendor, limit, offset)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	httpx.JSON(w, http.StatusOK, map[string]any{"quotes": list, "count": len(list)})
}
