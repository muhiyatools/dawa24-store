package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

func (h *UIHandler) CustomerOrderEditSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.UserID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect="+url.QueryEscape(r.URL.Path), http.StatusSeeOther)
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/orders", "error", i18n.T(langOf(r), "customer.order.invalid_id"))
		return
	}

	if h.commSvc == nil {
		h.redirectWithNotice(w, r, fmt.Sprintf("/orders/%d", id), "error", i18n.T(langOf(r), "customer.order.service_unavailable"))
		return
	}

	_ = r.ParseForm()

	lineIDs := r.PostForm["line_id[]"]
	productNames := r.PostForm["product_name[]"]
	quantities := r.PostForm["quantity[]"]
	unitPrices := r.PostForm["unit_price[]"]
	discountAmounts := r.PostForm["discount_amount[]"]
	isDeletedList := r.PostForm["is_deleted[]"]

	lineCount := len(lineIDs)
	if len(productNames) > lineCount {
		lineCount = len(productNames)
	}
	if len(quantities) > lineCount {
		lineCount = len(quantities)
	}

	var lines []commerce.OrderLineEditItem
	for i := 0; i < lineCount; i++ {
		var lineID int64
		if i < len(lineIDs) {
			lineID, _ = strconv.ParseInt(lineIDs[i], 10, 64)
		}

		pName := ""
		if i < len(productNames) {
			pName = strings.TrimSpace(productNames[i])
		}

		qty := 1
		if i < len(quantities) {
			if qVal, err := strconv.Atoi(quantities[i]); err == nil && qVal > 0 {
				qty = qVal
			}
		}

		uPrice := money.Zero
		if i < len(unitPrices) {
			if parsed, err := money.Parse(unitPrices[i]); err == nil {
				uPrice = parsed
			}
		}

		dAmount := money.Zero
		if i < len(discountAmounts) {
			if parsed, err := money.Parse(discountAmounts[i]); err == nil {
				dAmount = parsed
			}
		}

		isDel := false
		if i < len(isDeletedList) {
			isDel = isDeletedList[i] == "true" || isDeletedList[i] == "1"
		}

		if lineID <= 0 && pName == "" && !isDel {
			continue
		}

		lines = append(lines, commerce.OrderLineEditItem{
			ID:             lineID,
			ProductName:    pName,
			Quantity:       qty,
			UnitPrice:      uPrice,
			DiscountAmount: dAmount,
			IsDeleted:      isDel,
		})
	}

	input := commerce.UpdateCustomerOrderInput{
		OrderID: id,
		Lines:   lines,
		Notes:   strings.TrimSpace(r.PostFormValue("notes")),
	}

	updatedOrder, err := h.commSvc.UpdateCustomerPendingOrder(ctx, actor, input)
	if err != nil {
		errMsg := fmt.Sprintf(i18n.T(langOf(r), "customer.order.edit_error"), h.safeMessage(err, langOf(r)))
		if r.Header.Get("X-Requested-With") == "XMLHttpRequest" || strings.Contains(r.Header.Get("Accept"), "application/json") {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": false,
				"error":   errMsg,
			})
			return
		}
		h.redirectWithNotice(w, r, fmt.Sprintf("/orders/%d", id), "error", errMsg)
		return
	}

	if updatedOrder != nil {
		pharmacyName := h.resolveOrgName(ctx, actor.OrganizationID)
		orderNum := updatedOrder.OrderNumber
		if orderNum == "" {
			orderNum = fmt.Sprintf("ORD-%d", updatedOrder.ID)
		}
		notifLang := langOf(r)
		for _, sh := range updatedOrder.Shipments {
			if sh != nil && sh.OrganizationID > 0 {
				go h.dispatchOrgNotification(context.Background(), sh.OrganizationID, "vendor.order.view",
					fmt.Sprintf(i18n.T(notifLang, "customer.order.edit_notification_title"), orderNum),
					fmt.Sprintf(i18n.T(notifLang, "customer.order.edit_notification_body"), pharmacyName, orderNum, updatedOrder.TotalAmount.String()))
			}
		}
	}

	if r.Header.Get("X-Requested-With") == "XMLHttpRequest" || strings.Contains(r.Header.Get("Accept"), "application/json") {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success":     true,
			"message":     i18n.T(langOf(r), "customer.order.edit_success"),
			"subtotal":    updatedOrder.Subtotal.String(),
			"discount":    updatedOrder.DiscountAmount.String(),
			"total":       updatedOrder.TotalAmount.String(),
			"shippingFee": updatedOrder.ShippingFee.String(),
			"taxAmount":   updatedOrder.TaxAmount.String(),
		})
		return
	}

	h.redirectWithNotice(w, r, fmt.Sprintf("/orders/%d", id), "success", i18n.T(langOf(r), "customer.order.edit_success"))
}

// CustomerOrderCancelSubmit handles buyer cancellation of an order before shipping/arrival cutoff.
func (h *UIHandler) CustomerOrderCancelSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.UserID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect="+url.QueryEscape(r.URL.Path), http.StatusSeeOther)
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/orders", "error", i18n.T(langOf(r), "customer.order.invalid_id"))
		return
	}

	if h.commSvc == nil {
		h.redirectWithNotice(w, r, fmt.Sprintf("/orders/%d", id), "error", i18n.T(langOf(r), "customer.order.service_unavailable"))
		return
	}

	order, err := h.commSvc.GetOrder(ctx, id)
	if err != nil || order == nil {
		h.redirectWithNotice(w, r, "/orders", "error", "الطلب غير موجود")
		return
	}

	// Verify buyer authorization
	isOwner := order.CustomerID == actor.UserID ||
		(order.OrganizationID != nil && actor.OrganizationID > 0 && *order.OrganizationID == actor.OrganizationID)
	if !isOwner && !actor.IsPlatformAdmin() {
		h.redirectWithNotice(w, r, fmt.Sprintf("/orders/%d", id), "error", "غير مصرح لك بإلغاء هذا الطلب")
		return
	}

	// Check eligible cancellation status
	switch order.Status {
	case commerce.StatusPending, commerce.StatusProcessing, commerce.StatusConfirmed, commerce.StatusOnHold:
		// Eligible for cancellation
	default:
		errMsg := fmt.Sprintf("لا يمكن إلغاء الطلب في حالته الحالية (%s)", string(order.Status))
		if order.Status == commerce.StatusShipped || order.Status == commerce.StatusInTransit || order.Status == commerce.StatusOutForDelivery || order.Status == commerce.StatusDelivered {
			errMsg = "لا يمكن إلغاء الطلب بعد خروج الشحنة للتوصيل أو تسليمها بالفعل."
		}
		if r.Header.Get("X-Requested-With") == "XMLHttpRequest" || strings.Contains(r.Header.Get("Accept"), "application/json") {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": errMsg})
			return
		}
		h.redirectWithNotice(w, r, fmt.Sprintf("/orders/%d", id), "error", errMsg)
		return
	}

	// Forbid cancellation if any shipment is already on the road or delivered
	for _, sh := range order.Shipments {
		if sh != nil {
			switch sh.Status {
			case commerce.StatusShipped, commerce.StatusInTransit, commerce.StatusOutForDelivery, commerce.StatusDelivered, commerce.StatusCompleted:
				errMsg := "لا يمكن إلغاء الطلب نظراً لبدء شحن أو تسليم إحدى الشحنات بالفعل."
				if r.Header.Get("X-Requested-With") == "XMLHttpRequest" || strings.Contains(r.Header.Get("Accept"), "application/json") {
					w.Header().Set("Content-Type", "application/json; charset=utf-8")
					w.WriteHeader(http.StatusBadRequest)
					_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": errMsg})
					return
				}
				h.redirectWithNotice(w, r, fmt.Sprintf("/orders/%d", id), "error", errMsg)
				return
			}
		}
	}

	_ = r.ParseForm()
	reason := strings.TrimSpace(r.PostFormValue("reason"))
	notes := strings.TrimSpace(r.PostFormValue("notes"))
	fullReason := reason
	if notes != "" {
		if fullReason != "" {
			fullReason += " - " + notes
		} else {
			fullReason = notes
		}
	}
	if fullReason == "" {
		fullReason = "إلغاء الطلبية من قبل المشتري"
	}

	if err := h.commSvc.CancelOrder(ctx, id, &actor.UserID, fullReason); err != nil {
		errMsg := fmt.Sprintf("فشل إلغاء الطلب: %s", h.safeMessage(err, langOf(r)))
		if r.Header.Get("X-Requested-With") == "XMLHttpRequest" || strings.Contains(r.Header.Get("Accept"), "application/json") {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": errMsg})
			return
		}
		h.redirectWithNotice(w, r, fmt.Sprintf("/orders/%d", id), "error", errMsg)
		return
	}

	// Dispatch notification to seller vendors
	buyerName := h.resolveOrgName(ctx, actor.OrganizationID)
	orderNum := order.OrderNumber
	if orderNum == "" {
		orderNum = fmt.Sprintf("ORD-%d", order.ID)
	}
	for _, sh := range order.Shipments {
		if sh != nil && sh.OrganizationID > 0 {
			go h.dispatchOrgNotification(context.Background(), sh.OrganizationID, "vendor.order.view",
				fmt.Sprintf("تم إلغاء الطلبية #%s", orderNum),
				fmt.Sprintf("قام العميل %s بإلغاء الطلبية #%s بالكامل. السبب: %s", buyerName, orderNum, fullReason))
		}
	}

	if r.Header.Get("X-Requested-With") == "XMLHttpRequest" || strings.Contains(r.Header.Get("Accept"), "application/json") {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"message": "تم إلغاء الطلبية بنجاح وإشعار الموردين",
		})
		return
	}

	h.redirectWithNotice(w, r, fmt.Sprintf("/orders/%d", id), "success", "تم إلغاء الطلبية بنجاح وإشعار الموردين")
}

// ReviewSubmit handles customer feedback submissions with multi-criteria rating.