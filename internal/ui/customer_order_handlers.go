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
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
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

	if h.commSvc == nil {
		h.renderPage(ctx, w, "render customer orders page", pages.CustomerOrders(nil, lang, dir, h.isHTMX(r)))
		return
	}

	orders, err := h.commSvc.ListCustomerOrders(ctx, userID, h.pageLimit(r), h.pageOffset(r))
	if err != nil {
		h.renderError(w, r, err)
		return
	}

	h.renderPage(ctx, w, "render customer orders page", pages.CustomerOrders(orders, lang, dir, h.isHTMX(r)))
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
		h.redirectWithNotice(w, r, "/orders", "error", "معرف الطلب غير صالح.")
		return
	}

	if h.commSvc == nil {
		h.redirectWithNotice(w, r, fmt.Sprintf("/orders/%d", id), "error", "خدمة إدارة الطلبات غير متوفرة حالياً.")
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
		errMsg := "تعذر تعديل الطلب: " + h.safeMessage(err, langOf(r))
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
		for _, sh := range updatedOrder.Shipments {
			if sh != nil && sh.OrganizationID > 0 {
				go h.dispatchOrgNotification(context.Background(), sh.OrganizationID,
					fmt.Sprintf("تعديل على الطلب #%s", orderNum),
					fmt.Sprintf("قامت صيدلية %s بتعديل كميات وأصناف الطلب #%s بقيمة جديدة %s ج.م.", pharmacyName, orderNum, updatedOrder.TotalAmount.String()))
			}
		}
	}

	if r.Header.Get("X-Requested-With") == "XMLHttpRequest" || strings.Contains(r.Header.Get("Accept"), "application/json") {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success":     true,
			"message":     "تم حفظ وتعديل بيانات الطلب وتحديث الإجماليات بنجاح.",
			"subtotal":    updatedOrder.Subtotal.String(),
			"discount":    updatedOrder.DiscountAmount.String(),
			"total":       updatedOrder.TotalAmount.String(),
			"shippingFee": updatedOrder.ShippingFee.String(),
			"taxAmount":   updatedOrder.TaxAmount.String(),
		})
		return
	}

	h.redirectWithNotice(w, r, fmt.Sprintf("/orders/%d", id), "success", "تم حفظ وتعديل بيانات الطلب وتحديث الإجماليات بنجاح.")
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
		h.redirectWithNotice(w, r, "/suppliers", "error", "المؤسسة المستهدفة غير صالحة.")
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

	h.redirectWithNotice(w, r, fmt.Sprintf("/suppliers/%d", targetOrgID), "success", "تم إرسال تقييمك بنجاح. شكراً لمشاركتك!")
}

// CustomerNegotiateOrderSubmit initiates a price negotiation order with a supplier.
func (h *UIHandler) CustomerNegotiateOrderSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/catalog", http.StatusSeeOther)
		return
	}

	if err := r.ParseForm(); err != nil {
		h.redirectWithNotice(w, r, "/catalog", "error", "بيانات النموذج غير صالحة.")
		return
	}

	variantID, _ := strconv.ParseInt(r.PostFormValue("variant_id"), 10, 64)
	vendorOrgID, _ := strconv.ParseInt(r.PostFormValue("vendor_org_id"), 10, 64)
	qty, _ := strconv.Atoi(r.PostFormValue("qty"))
	if qty <= 0 {
		qty = 1
	}

	proposedPriceStr := r.PostFormValue("proposed_price")
	proposedPrice, err := money.Parse(proposedPriceStr)
	if err != nil || proposedPrice.IsZero() || proposedPrice.IsNegative() {
		h.redirectWithNotice(w, r, "/catalog", "error", "يرجى إدخال سعر تفاوضي صالح وموجب.")
		return
	}

	notes := strings.TrimSpace(r.PostFormValue("notes"))
	if notes == "" {
		notes = fmt.Sprintf("طلب تفاوض على سعر الصنف: %s ج.م للعبوة (كمية: %d)", proposedPrice.String(), qty)
	}

	var branchID *int64
	if bStr := r.PostFormValue("branch_id"); bStr != "" {
		if bID, err := strconv.ParseInt(bStr, 10, 64); err == nil && bID > 0 {
			branchID = &bID
		}
	}
	if branchID == nil {
		if buying, ok := authctx.BuyingBranchFrom(ctx); ok && buying.Active != nil && *buying.Active > 0 {
			branchID = buying.Active
		} else if actor.BranchID != nil && *actor.BranchID > 0 {
			branchID = actor.BranchID
		}
	}

	var productID int64
	prodName := i18n.Text{"ar": "صنف دوائي للتفاوض", "en": "Negotiated Medicine"}
	if h.catSvc != nil && variantID > 0 {
		if v, err := h.catSvc.GetVariant(database.AsSystem(ctx), variantID); err == nil && v != nil {
			productID = v.ProductID
			if vendorOrgID <= 0 {
				vendorOrgID = v.OrganizationID
			}
			if len(v.Name) > 0 {
				prodName = v.Name
			}
		}
	}

	paymentMethod := r.PostFormValue("payment_method")
	if paymentMethod == "" {
		paymentMethod = "cod"
	}

	var pIDPtr *int64
	if productID > 0 {
		pIDPtr = &productID
	}
	var vIDPtr *int64
	if variantID > 0 {
		vIDPtr = &variantID
	}

	input := commerce.CheckoutInput{
		CustomerID:       actor.UserID,
		CustomerOrgID:    actor.OrganizationID,
		BranchID:         branchID,
		PaymentMethod:    paymentMethod,
		Notes:            notes,
		IsNegotiation:    true,
		NegotiationNotes: notes,
		Items: []commerce.CheckoutLineItem{
			{
				ProductVariantID:  vIDPtr,
				ProductID:         pIDPtr,
				VendorOrgID:       vendorOrgID,
				ProductName:       prodName,
				Quantity:          qty,
				UnitPrice:         proposedPrice,
				ProposedUnitPrice: proposedPrice,
				IsNegotiated:      true,
			},
		},
	}

	if h.commSvc == nil {
		h.redirectWithNotice(w, r, "/catalog", "error", "خدمة المبيعات غير متاحة حالياً.")
		return
	}

	order, err := h.commSvc.Checkout(ctx, input)
	if err != nil {
		h.log.ErrorContext(ctx, "failed to submit negotiation order", "error", err, "variant_id", variantID)
		h.redirectWithNotice(w, r, "/catalog", "error", "تعذر إرسال طلب التفاوض: "+h.safeMessage(err, langOf(r)))
		return
	}

	h.redirectWithNotice(w, r, fmt.Sprintf("/orders/%d", order.ID), "success", "تم إرسال طلب التفاوض على السعر إلى المورد بنجاح وهو الآن قيد المراجعة.")
}
