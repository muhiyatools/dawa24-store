package http

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/billing"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
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

	r.Post("/api/v1/billing/invoices", h.CreateInvoice)
	r.Get("/api/v1/billing/invoices/{id}", h.GetInvoice)
	r.Get("/api/v1/billing/invoices", h.ListInvoices)
	r.Post("/api/v1/billing/invoices/{id}/pay", h.PayInvoice)

	r.Post("/api/v1/billing/payment-methods", h.AddPaymentMethod)
	r.Get("/api/v1/billing/payment-methods", h.ListPaymentMethods)
	r.Delete("/api/v1/billing/payment-methods/{id}", h.DeletePaymentMethod)

	h.RegisterAdminRoutes(r)
}

// GetWallet retrieves wallet details.
func (h *Handler) GetWallet(w http.ResponseWriter, r *http.Request) {
	// The acting user comes from the authenticated session, never from the
	// request. Reading it from the query string let any caller act as any
	// user by changing a number.
	userID, err := authctx.UserID(r.Context())
	if err != nil {
		httpx.Error(w, r, h.log, err)
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
	// The acting user comes from the authenticated session, never from the
	// request. Reading it from the query string let any caller act as any
	// user by changing a number.
	userID, err := authctx.UserID(r.Context())
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

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

// CreateInvoice generates an invoice.
func (h *Handler) CreateInvoice(w http.ResponseWriter, r *http.Request) {
	var inv billing.Invoice
	if err := httpx.DecodeJSON(w, r, &inv); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	created, err := h.service.CreateInvoice(r.Context(), &inv)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	httpx.JSON(w, http.StatusCreated, created)
}

// GetInvoice returns an invoice by ID.
func (h *Handler) GetInvoice(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		httpx.Error(w, r, h.log, apperr.Validation("id.invalid", "Invalid invoice ID", nil))
		return
	}

	inv, err := h.service.GetInvoice(r.Context(), id)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	httpx.JSON(w, http.StatusOK, inv)
}

// ListInvoices lists invoices for an organization.
func (h *Handler) ListInvoices(w http.ResponseWriter, r *http.Request) {
	// The organization is the caller's active tenant, resolved by middleware
	// after verifying membership. Reading it from ?org_id= let any caller name
	// any organization and read its invoices.
	orgID, ok := database.TenantFrom(r.Context())
	if !ok {
		httpx.Error(w, r, h.log, database.ErrNoTenant)
		return
	}

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	list, err := h.service.ListInvoices(r.Context(), orgID, limit, offset)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	httpx.JSON(w, http.StatusOK, map[string]any{"invoices": list, "count": len(list)})
}

// PayInvoice records invoice payment.
func (h *Handler) PayInvoice(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		httpx.Error(w, r, h.log, apperr.Validation("id.invalid", "Invalid invoice ID", nil))
		return
	}

	if err := h.service.MarkInvoicePaid(r.Context(), id); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	httpx.JSON(w, http.StatusOK, map[string]string{"status": "paid"})
}

// AddPaymentMethod adds a user payment method.
func (h *Handler) AddPaymentMethod(w http.ResponseWriter, r *http.Request) {
	var pm billing.UserPaymentMethod
	if err := httpx.DecodeJSON(w, r, &pm); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	if err := h.service.AddPaymentMethod(r.Context(), &pm); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	httpx.JSON(w, http.StatusCreated, pm)
}

// ListPaymentMethods returns saved payment methods.
func (h *Handler) ListPaymentMethods(w http.ResponseWriter, r *http.Request) {
	// The acting user comes from the authenticated session, never from the
	// request. Reading it from the query string let any caller act as any
	// user by changing a number.
	userID, err := authctx.UserID(r.Context())
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	list, err := h.service.ListPaymentMethods(r.Context(), userID)
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	httpx.JSON(w, http.StatusOK, map[string]any{"payment_methods": list, "count": len(list)})
}

// DeletePaymentMethod deletes a saved payment method belonging to the caller.
func (h *Handler) DeletePaymentMethod(w http.ResponseWriter, r *http.Request) {
	userID, err := authctx.UserID(r.Context())
	if err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		httpx.Error(w, r, h.log, apperr.Validation("payment_method.invalid_id", "Invalid payment method ID.", nil))
		return
	}

	if err := h.service.DeletePaymentMethod(r.Context(), userID, id); err != nil {
		httpx.Error(w, r, h.log, err)
		return
	}

	httpx.JSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

