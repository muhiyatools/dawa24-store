package http

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/billing"
	"github.com/muhiya/dawa24-store/internal/platform/httpx"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// Handler exposes billing and wallet endpoints.
type Handler struct {
	service *billing.Service
	log     *slog.Logger
}

// NewHandler creates a billing HTTP handler.
func NewHandler(service *billing.Service, log *slog.Logger) *Handler {
	return &Handler{service: service, log: log}
}

// RegisterRoutes registers billing routes on a Chi router.
func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Get("/api/v1/billing/wallet", h.GetWallet)
	r.Post("/api/v1/billing/wallet/deposit", h.Deposit)
	r.Post("/api/v1/billing/wallet/withdraw", h.Withdraw)
	r.Get("/api/v1/billing/plans", h.ListPlans)
	r.Post("/api/v1/billing/subscriptions", h.Subscribe)
	r.Get("/api/v1/billing/entitlements/{key}", h.CheckEntitlement)
}

// GetWallet retrieves wallet details.
func (h *Handler) GetWallet(w http.ResponseWriter, r *http.Request) {
	userIDStr := r.URL.Query().Get("user_id")
	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil || userID <= 0 {
		httpx.Error(w, r, h.log, apperr.Validation("user_id.invalid", "Valid user_id required", nil))
		return
	}

	wallet, err := h.service.GetWallet(r.Context(), userID, "EGP")
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	httpx.JSON(w, http.StatusOK, wallet)
}

type WalletTransactionRequest struct {
	UserID      int64        `json:"user_id"`
	Currency    string       `json:"currency"`
	Amount      money.Amount `json:"amount"`
	Description string       `json:"description"`
}

// Deposit credits the user wallet.
func (h *Handler) Deposit(w http.ResponseWriter, r *http.Request) {
	var req WalletTransactionRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	tx, err := h.service.Deposit(r.Context(), req.UserID, req.Currency, req.Amount, "manual", nil, req.Description)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	httpx.JSON(w, http.StatusOK, tx)
}

// Withdraw debits the user wallet.
func (h *Handler) Withdraw(w http.ResponseWriter, r *http.Request) {
	var req WalletTransactionRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	tx, err := h.service.Withdraw(r.Context(), req.UserID, req.Currency, req.Amount, "manual", nil, req.Description)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	httpx.JSON(w, http.StatusOK, tx)
}

// ListPlans lists subscription plans.
func (h *Handler) ListPlans(w http.ResponseWriter, r *http.Request) {
	plans, err := h.service.ListPlans(r.Context())
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	httpx.JSON(w, http.StatusOK, map[string]any{
		"plans": plans,
	})
}

type SubscribeRequest struct {
	UserID   int64  `json:"user_id"`
	PlanSlug string `json:"plan_slug"`
}

// Subscribe activates a subscription.
func (h *Handler) Subscribe(w http.ResponseWriter, r *http.Request) {
	var req SubscribeRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	sub, err := h.service.Subscribe(r.Context(), req.UserID, nil, req.PlanSlug, "api_direct", nil)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	httpx.JSON(w, http.StatusCreated, sub)
}

// CheckEntitlement checks feature key access.
func (h *Handler) CheckEntitlement(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	userIDStr := r.URL.Query().Get("user_id")
	userID, _ := strconv.ParseInt(userIDStr, 10, 64)

	allowed, val, err := h.service.CheckEntitlement(r.Context(), userID, key)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	httpx.JSON(w, http.StatusOK, map[string]any{
		"feature": key,
		"allowed": allowed,
		"value":   val,
	})
}
