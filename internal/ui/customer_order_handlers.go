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
	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/shared/pagination"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

func (h *UIHandler) CustomerOrdersPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	userID, err := authctx.UserID(ctx)
	if err != nil {
		http.Redirect(w, r, "/auth/login?redirect=/orders", http.StatusSeeOther)
		return
	}

	page := pagination.PageNumber(r)
	limit := pagination.RowsPerPage(r)
	offset := (page - 1) * limit

	if h.commSvc == nil {
		h.renderPage(ctx, w, "render customer orders page", pages.CustomerOrders(pages.CustomerOrdersData{}, lang, dir, h.isHTMX(r)))
		return
	}

	orders, total, err := h.commSvc.ListCustomerOrdersWithTotal(ctx, userID, limit, offset)
	if err != nil {
		h.renderError(w, r, err)
		return
	}

	data := pages.CustomerOrdersData{
		Orders:     orders,
		Page:       page,
		PerPage:    limit,
		TotalCount: total,
	}

	h.renderPage(ctx, w, "render customer orders page", pages.CustomerOrders(data, lang, dir, h.isHTMX(r)))
}

func (h *UIHandler) CustomerOrderDetailPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		h.renderError(w, r, err)
		return
	}

	if h.commSvc == nil {
		h.renderError(w, r, http.ErrNotSupported)
		return
	}

	order, err := h.commSvc.GetOrder(ctx, id)
	if err != nil {
		h.renderError(w, r, err)
		return
	}

	history, _ := h.commSvc.GetOrderHistory(ctx, id)

	noticeType := r.URL.Query().Get("notice")
	noticeMsg := r.URL.Query().Get("msg")
	if noticeType == "" {
		noticeType = r.URL.Query().Get("notice_type")
	}
	if noticeMsg == "" {
		noticeMsg = r.URL.Query().Get("notice_msg")
	}

	h.renderPage(ctx, w, "render customer order detail page", pages.CustomerOrderDetail(order, history, noticeType, noticeMsg, lang, dir))
}

// CustomerOrderEditSubmit handles customer edits to quantities and items of a pending order.
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
				go h.dispatchOrgNotification(context.Background(), sh.OrganizationID,
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

// ReviewSubmit handles customer feedback submissions with multi-criteria rating.
func (h *UIHandler) ReviewSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	_ = r.ParseForm()
	targetOrgID, _ := strconv.ParseInt(r.PostFormValue("organization_id"), 10, 64)
	if targetOrgID <= 0 {
		h.redirectWithNotice(w, r, "/suppliers", "error", i18n.T(langOf(r), "customer.order.invalid_target_org"))
		return
	}

	var orderID *int64
	if oIDStr := r.PostFormValue("order_id"); oIDStr != "" {
		if oID, err := strconv.ParseInt(oIDStr, 10, 64); err == nil && oID > 0 {
			orderID = &oID
		}
	}

	repScore, _ := strconv.Atoi(r.PostFormValue("rating_rep"))
	speedScore, _ := strconv.Atoi(r.PostFormValue("rating_speed"))
	qualityScore, _ := strconv.Atoi(r.PostFormValue("rating_quality"))

	if repScore < 1 {
		repScore = 5
	}
	if speedScore < 1 {
		speedScore = 5
	}
	if qualityScore < 1 {
		qualityScore = 5
	}

	rev := &org.Review{
		OrganizationID: targetOrgID,
		UserID:         actor.UserID,
		OrderID:        orderID,
		Title:          r.PostFormValue("title"),
		ReviewText:     r.PostFormValue("review_text"),
		Context:        r.PostFormValue("context"),
		IsApproved:     true,
		Ratings: []org.ReviewRating{
			{Criterion: "rep", Score: repScore},
			{Criterion: "speed", Score: speedScore},
			{Criterion: "quality", Score: qualityScore},
		},
	}

	if h.orgSvc != nil {
		if err := h.orgSvc.SubmitReview(ctx, rev); err != nil {
			h.log.ErrorContext(ctx, "failed to submit review", "error", err, "target_org_id", targetOrgID)
			h.redirectWithNotice(w, r, fmt.Sprintf("/suppliers/%d", targetOrgID), "error", h.safeMessage(err, langOf(r)))
			return
		}
	}

	h.redirectWithNotice(w, r, fmt.Sprintf("/suppliers/%d", targetOrgID), "success", i18n.T(langOf(r), "customer.order.review_success"))
}
