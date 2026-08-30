package ui

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// CourierDeliveryPage renders the dedicated, unlisted courier delivery portal.
func (h *UIHandler) CourierDeliveryPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	tracking := strings.TrimSpace(r.URL.Query().Get("tracking"))
	errMsg := strings.TrimSpace(r.URL.Query().Get("err"))
	succMsg := strings.TrimSpace(r.URL.Query().Get("msg"))

	data := pages.CourierDeliveryData{
		TrackingQuery:  tracking,
		ErrorMessage:   errMsg,
		SuccessMessage: succMsg,
	}

	if tracking != "" && h.commSvc != nil {
		shipment, err := h.commSvc.GetShipmentForDelivery(ctx, tracking)
		if err != nil {
			if _, ok := apperr.As(err); ok {
				data.ErrorMessage = i18n.T(lang, "courier.shipment_not_found")
			} else {
				data.ErrorMessage = i18n.T(lang, "courier.fetch_error")
			}
		} else {
			data.Shipment = shipment
		}
	}

	h.renderPage(ctx, w, "render courier delivery portal", pages.CourierDeliveryPage(data, lang, dir))
}

// CourierVerifyDeliverySubmit processes the 6-digit delivery PIN and completes the shipment delivery.
func (h *UIHandler) CourierVerifyDeliverySubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/delivery?err=invalid_data", http.StatusSeeOther)
		return
	}

	tracking := strings.TrimSpace(r.PostFormValue("tracking"))
	deliveryCode := strings.TrimSpace(r.PostFormValue("delivery_code"))
	notes := strings.TrimSpace(r.PostFormValue("notes"))

	if tracking == "" || deliveryCode == "" {
		http.Redirect(w, r, fmt.Sprintf("/delivery?tracking=%s&err=%s", tracking, "code_required"), http.StatusSeeOther)
		return
	}

	if h.commSvc == nil {
		http.Redirect(w, r, "/delivery?err=service_unavailable", http.StatusSeeOther)
		return
	}

	completedShipment, err := h.commSvc.VerifyAndCompleteDelivery(ctx, tracking, deliveryCode, notes, 0)
	if err != nil {
		h.log.WarnContext(ctx, "courier delivery verification failed", "tracking", tracking, "error", err)

		var errMsg string
		var isLocked bool
		if apErr, ok := apperr.As(err); ok {
			errMsg = apErr.Msg
			if apErr.Code == "delivery.locked" {
				isLocked = true
			}
		} else {
			errMsg = i18n.T(lang, "courier.delivery_failed")
		}

		// Re-fetch shipment to render the page with the error
		shipment, _ := h.commSvc.GetShipmentForDelivery(ctx, tracking)
		data := pages.CourierDeliveryData{
			TrackingQuery: tracking,
			Shipment:      shipment,
			ErrorMessage:  errMsg,
			IsLocked:      isLocked,
		}
		h.renderPage(ctx, w, "render courier delivery with error", pages.CourierDeliveryPage(data, lang, dir))
		return
	}

	// Dispatch notifications to Customer and Vendor
	if completedShipment != nil {
		if order, oErr := h.commSvc.GetOrder(database.AsSystem(ctx), completedShipment.OrderID); oErr == nil && order != nil {
			vendorName := h.resolveOrgName(ctx, completedShipment.OrganizationID)
			orderNum := order.OrderNumber
			if orderNum == "" {
				orderNum = fmt.Sprintf("ORD-%d", order.ID)
			}

			// Notify Customer
			go h.dispatchInAppNotification(context.Background(), order.CustomerID, nil,
				fmt.Sprintf(i18n.T(lang, "courier.customer_notif_title"), completedShipment.ShipmentNumber),
				fmt.Sprintf(i18n.T(lang, "courier.customer_notif_body"), orderNum, vendorName),
			)

			// Notify Vendor
			go h.dispatchInAppNotification(context.Background(), 0, &completedShipment.OrganizationID,
				fmt.Sprintf(i18n.T(lang, "courier.vendor_notif_title"), completedShipment.ShipmentNumber),
				fmt.Sprintf(i18n.T(lang, "courier.vendor_notif_body"), completedShipment.ShipmentNumber),
			)
		}
	}

	data := pages.CourierDeliveryData{
		TrackingQuery:  tracking,
		Shipment:       completedShipment,
		SuccessMessage: i18n.T(lang, "courier.delivery_success"),
		IsCompleted:    true,
	}
	h.renderPage(ctx, w, "render courier delivery completed", pages.CourierDeliveryPage(data, lang, dir))
}
