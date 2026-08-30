package ui

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/commerce"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// VendorOrdersPage renders supplier order fulfillments.
func (h *UIHandler) VendorOrdersPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/orders", http.StatusSeeOther)
		return
	}

	if h.commSvc == nil {
		h.renderPage(ctx, w, "render vendor orders fallback", pages.VendorOrders(pages.VendorOrdersData{}, lang, dir, h.isHTMX(r)))
		return
	}

	shipments, err := h.commSvc.ListVendorShipments(ctx, actor.OrganizationID, 100, 0)
	if err != nil {
		h.renderError(w, r, err)
		return
	}

	filterStatus := r.URL.Query().Get("status")
	var pendingCount, confirmedCount, shippedCount, deliveredCount int
	var filtered []*commerce.OrderShipment

	for _, s := range shipments {
		switch s.Status {
		case commerce.StatusPending:
			pendingCount++
		case commerce.StatusConfirmed:
			confirmedCount++
		case commerce.StatusShipped:
			shippedCount++
		case commerce.StatusDelivered:
			deliveredCount++
		}

		if filterStatus == "" || string(s.Status) == filterStatus {
			filtered = append(filtered, s)
		}
	}

	data := pages.VendorOrdersData{
		Shipments:      filtered,
		FilterStatus:   filterStatus,
		TotalCount:     len(shipments),
		PendingCount:   pendingCount,
		ConfirmedCount: confirmedCount,
		ShippedCount:   shippedCount,
		DeliveredCount: deliveredCount,
	}

	h.renderPage(ctx, w, "render vendor orders page", pages.VendorOrders(data, lang, dir, h.isHTMX(r)))
}

// VendorOrderStatusSubmit transitions shipment delivery states.
func (h *UIHandler) VendorOrderStatusSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/orders", http.StatusSeeOther)
		return
	}

	shipmentID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	toStatus := r.PostFormValue("status")
	notes := r.PostFormValue("notes")

	if actor.OrganizationID > 0 {
		ctx = database.WithTenant(ctx, actor.OrganizationID)
	}

	if h.commSvc != nil && shipmentID > 0 && toStatus != "" {
		_, err := h.commSvc.TransitionShipmentStatus(ctx, shipmentID, commerce.OrderStatus(toStatus), &actor.UserID, notes)
		if err != nil {
			h.log.ErrorContext(ctx, "vendor transition shipment status failed", "error", err, "shipment", shipmentID, "to", toStatus)
			h.redirectWithNotice(w, r, "/vendor/orders", "error", i18n.T(langOf(r), "vendor.orders.update_shipment_status_error_prefix")+h.safeMessage(err, langOf(r)))
			return
		}
		trackingVal := strings.TrimSpace(r.PostFormValue("tracking"))
		carrier := strings.TrimSpace(r.PostFormValue("carrier"))
		if toStatus == string(commerce.StatusShipped) && trackingVal == "" {
			trackingVal = commerce.GenerateTrackingNumber(fmt.Sprintf("%d", shipmentID), 1)
		}
		if carrier != "" || trackingVal != "" {
			_ = h.commSvc.SetShipmentTracking(ctx, shipmentID, carrier, trackingVal)
		}

		// Dispatch live notification to customer
		if shipment, sErr := h.commSvc.GetShipment(database.AsSystem(ctx), shipmentID); sErr == nil && shipment != nil {
			if order, oErr := h.commSvc.GetOrder(database.AsSystem(ctx), shipment.OrderID); oErr == nil && order != nil {
				vendorName := h.resolveOrgName(ctx, actor.OrganizationID)
				go h.notifyOrderStatusChanged(context.Background(), order, shipmentID, commerce.OrderStatus(toStatus), vendorName, notes)
			}
		}
	}

	h.redirectWithNotice(w, r, "/vendor/orders", "success", i18n.T(langOf(r), "vendor.orders.shipment_status_updated_success"))
}

// VendorNegotiationAcceptSubmit accepts a customer's proposed negotiated price and confirms the order.
func (h *UIHandler) VendorNegotiationAcceptSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/orders", http.StatusSeeOther)
		return
	}

	orderID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if h.commSvc != nil && orderID > 0 {
		order, err := h.commSvc.GetOrder(ctx, orderID)
		if err != nil || order == nil {
			h.redirectWithNotice(w, r, "/vendor/orders", "error", i18n.T(langOf(r), "vendor.orders.order_not_found"))
			return
		}
		if !actor.IsStaff && !actor.Can("commerce.admin") {
			isVendorOrder := false
			for _, sh := range order.Shipments {
				if sh.OrganizationID == actor.OrganizationID {
					isVendorOrder = true
					break
				}
			}
			if !isVendorOrder {
				h.redirectWithNotice(w, r, "/vendor/orders", "error", i18n.T(langOf(r), "vendor.orders.unauthorized_order_management"))
				return
			}
		}

		if err := h.commSvc.AcceptNegotiation(ctx, orderID, actor.UserID); err != nil {
			h.log.ErrorContext(ctx, "vendor accept negotiation failed", "error", err, "order_id", orderID)
			h.redirectWithNotice(w, r, "/vendor/orders", "error", i18n.T(langOf(r), "vendor.orders.accept_negotiation_error_prefix")+h.safeMessage(err, langOf(r)))
			return
		}

		// Dispatch notification to customer
		vendorName := h.resolveOrgName(ctx, actor.OrganizationID)
		orderNum := order.OrderNumber
		if orderNum == "" {
			orderNum = fmt.Sprintf("ORD-%d", order.ID)
		}
		go h.dispatchInAppNotification(context.Background(), order.CustomerID, nil,
			fmt.Sprintf(i18n.T("ar", "vendor.orders.negotiation_accepted_title"), orderNum),
			fmt.Sprintf(i18n.T("ar", "vendor.orders.negotiation_accepted_body"), vendorName))
	}

	h.redirectWithNotice(w, r, "/vendor/orders", "success", i18n.T(langOf(r), "vendor.orders.negotiation_accepted_success"))
}

// VendorNegotiationRejectSubmit rejects a customer's proposed negotiated price and cancels the order.
func (h *UIHandler) VendorNegotiationRejectSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/orders", http.StatusSeeOther)
		return
	}

	orderID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	_ = r.ParseForm()
	reason := r.PostFormValue("reason")
	if reason == "" {
		reason = i18n.T(langOf(r), "vendor.orders.negotiation_default_reject_reason")
	}

	if h.commSvc != nil && orderID > 0 {
		order, err := h.commSvc.GetOrder(ctx, orderID)
		if err != nil || order == nil {
			h.redirectWithNotice(w, r, "/vendor/orders", "error", i18n.T(langOf(r), "vendor.orders.order_not_found"))
			return
		}
		if !actor.IsStaff && !actor.Can("commerce.admin") {
			isVendorOrder := false
			for _, sh := range order.Shipments {
				if sh.OrganizationID == actor.OrganizationID {
					isVendorOrder = true
					break
				}
			}
			if !isVendorOrder {
				h.redirectWithNotice(w, r, "/vendor/orders", "error", i18n.T(langOf(r), "vendor.orders.unauthorized_order_management"))
				return
			}
		}

		if err := h.commSvc.RejectNegotiation(ctx, orderID, reason, actor.UserID); err != nil {
			h.log.ErrorContext(ctx, "vendor reject negotiation failed", "error", err, "order_id", orderID)
			h.redirectWithNotice(w, r, "/vendor/orders", "error", i18n.T(langOf(r), "vendor.orders.reject_negotiation_error_prefix")+h.safeMessage(err, langOf(r)))
			return
		}

		// Dispatch notification to customer
		vendorName := h.resolveOrgName(ctx, actor.OrganizationID)
		orderNum := order.OrderNumber
		if orderNum == "" {
			orderNum = fmt.Sprintf("ORD-%d", order.ID)
		}
		go h.dispatchInAppNotification(context.Background(), order.CustomerID, nil,
			fmt.Sprintf(i18n.T("ar", "vendor.orders.negotiation_rejected_title"), orderNum),
			fmt.Sprintf(i18n.T("ar", "vendor.orders.negotiation_rejected_body"), vendorName, reason))
	}

	h.redirectWithNotice(w, r, "/vendor/orders", "success", i18n.T(langOf(r), "vendor.orders.negotiation_rejected_success"))
}
