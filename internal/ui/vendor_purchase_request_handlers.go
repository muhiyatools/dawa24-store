package ui

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// VendorPurchaseRequestsPage renders incoming purchase requests for a supplier (Plan V5 Phase 3 §3.1.5).
func (h *UIHandler) VendorPurchaseRequestsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/purchase-requests", http.StatusSeeOther)
		return
	}

	status := r.URL.Query().Get("status")
	if status == "" {
		status = "all"
	}

	var requests []*commerce.PurchaseRequest
	if h.commSvc != nil {
		reqs, err := h.commSvc.ListVendorPurchaseRequests(ctx, actor.OrganizationID, status, 50, 0)
		if err == nil {
			requests = reqs
		}
	}

	h.renderPage(ctx, w, "render vendor purchase requests page", pages.VendorPurchaseRequestsPage(lang, dir, requests, status))
}

// VendorPurchaseRequestDetailPage renders one incoming purchase request with line items.
func (h *UIHandler) VendorPurchaseRequestDetailPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/purchase-requests", http.StatusSeeOther)
		return
	}

	reqIDStr := chi.URLParam(r, "id")
	reqID, err := strconv.ParseInt(reqIDStr, 10, 64)
	if err != nil || reqID <= 0 {
		http.Redirect(w, r, "/vendor/purchase-requests", http.StatusSeeOther)
		return
	}

	var request *commerce.PurchaseRequest
	if h.commSvc != nil {
		req, err := h.commSvc.GetPurchaseRequest(ctx, reqID)
		if err == nil && req.VendorOrgID == actor.OrganizationID {
			request = req
		}
	}
	if request == nil {
		http.Redirect(w, r, "/vendor/purchase-requests", http.StatusSeeOther)
		return
	}

	h.renderPage(ctx, w, "render vendor purchase request detail page", pages.VendorPurchaseRequestDetailPage(lang, dir, request))
}

// VendorPurchaseRequestRespondSubmit allows vendor to approve, reject, or comment on a purchase request.
func (h *UIHandler) VendorPurchaseRequestRespondSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	reqIDStr := chi.URLParam(r, "id")
	reqID, err := strconv.ParseInt(reqIDStr, 10, 64)
	if err != nil || reqID <= 0 {
		h.redirectWithNotice(w, r, "/vendor/purchase-requests", "error", i18n.T(lang, "vendor.purchase_request.invalid_id"))
		return
	}

	status := commerce.PurchaseRequestStatus(r.FormValue("status"))
	vendorNotes := strings.TrimSpace(r.FormValue("vendor_notes"))

	responderID := actor.UserID

	if h.commSvc != nil {
		req, err := h.commSvc.GetPurchaseRequest(ctx, reqID)
		if err != nil || req == nil {
			h.redirectWithNotice(w, r, "/vendor/purchase-requests", "error", i18n.T(lang, "vendor.purchase_request.not_found"))
			return
		}
		if !actor.IsStaff && !actor.Can("commerce.admin") {
			if req.VendorOrgID != actor.OrganizationID {
				h.redirectWithNotice(w, r, "/vendor/purchase-requests", "error", i18n.T(lang, "vendor.orders.unauthorized_order_management"))
				return
			}
		}

		if err := h.commSvc.RespondPurchaseRequest(ctx, reqID, status, vendorNotes, &responderID); err != nil {
			h.redirectWithNotice(w, r, "/vendor/purchase-requests/"+reqIDStr, "error", h.safeMessage(err, lang))
			return
		}

		// Dispatch notification to pharmacy customer
		vendorName := h.resolveOrgName(ctx, actor.OrganizationID)
		var custOrgID int64
		if req.OrganizationID != nil {
			custOrgID = *req.OrganizationID
		}
		go h.notifyPurchaseRequestResponded(context.Background(), req.CustomerID, custOrgID, vendorName, reqID)
	}

	h.redirectWithNotice(w, r, "/vendor/purchase-requests/"+reqIDStr, "success", i18n.T(lang, "vendor.purchase_request.status_updated_success"))
}

// VendorPurchaseRequestLineRespondSubmit updates vendor price/discount counter-offer for a specific line item.
func (h *UIHandler) VendorPurchaseRequestLineRespondSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	lineIDStr := chi.URLParam(r, "id")
	lineID, err := strconv.ParseInt(lineIDStr, 10, 64)
	if err != nil || lineID <= 0 {
		h.redirectWithNotice(w, r, "/vendor/purchase-requests", "error", i18n.T(lang, "vendor.purchase_request.invalid_line_id"))
		return
	}

	var price money.Amount
	if pStr := r.FormValue("offered_price"); pStr != "" {
		if p, err := money.Parse(pStr); err == nil {
			price = p
		}
	}

	var discount float64
	if dStr := r.FormValue("offered_discount"); dStr != "" {
		if d, err := strconv.ParseFloat(dStr, 64); err == nil && d >= 0 {
			discount = d
		}
	}

	status := r.FormValue("status")
	if status == "" {
		status = "approved"
	}

	if h.commSvc != nil {
		if err := h.commSvc.UpdatePurchaseRequestLineOffer(ctx, lineID, price, discount, status); err != nil {
			h.redirectWithNotice(w, r, "/vendor/purchase-requests", "error", h.safeMessage(err, lang))
			return
		}
	}

	h.redirectWithNotice(w, r, "/vendor/purchase-requests", "success", i18n.T(lang, "vendor.purchase_request.line_offer_updated_success"))
}
