package ui

import (
	"context"
	"fmt"
	"strings"

	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/modules/notifications"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// dispatchInAppNotification sends a direct in-app notification to a single user.
func (h *UIHandler) dispatchInAppNotification(ctx context.Context, userID int64, orgID *int64, title, body string) {
	if h.notifSvc == nil || userID <= 0 {
		return
	}
	sysCtx := database.AsSystem(ctx)
	_, err := h.notifSvc.Send(sysCtx, notifications.SendInput{
		UserID:         userID,
		OrganizationID: orgID,
		Channel:        notifications.ChannelInApp,
		Recipient:      fmt.Sprintf("user-%d", userID),
		Title:          title,
		Body:           body,
	})
	if err != nil {
		h.log.WarnContext(ctx, "failed to dispatch in-app notification", "user_id", userID, "error", err)
	}
}

// dispatchOrgNotification sends an in-app notification to all active members of an organization.
func (h *UIHandler) dispatchOrgNotification(ctx context.Context, orgID int64, title, body string) {
	if h.notifSvc == nil || h.orgSvc == nil || orgID <= 0 {
		return
	}
	sysCtx := database.AsSystem(ctx)
	members, err := h.orgSvc.ListMembers(sysCtx, orgID)
	if err != nil || len(members) == 0 {
		return
	}
	for _, m := range members {
		if m != nil && m.UserID > 0 && m.IsActive {
			h.dispatchInAppNotification(ctx, m.UserID, &orgID, title, body)
		}
	}
}

// notifyOrderPlaced dispatches live notifications to the customer and all fulfilling vendor teams.
func (h *UIHandler) notifyOrderPlaced(ctx context.Context, order *commerce.Order, pharmacyName string) {
	if order == nil {
		return
	}

	orderNum := order.OrderNumber
	if orderNum == "" {
		orderNum = fmt.Sprintf("ORD-%d", order.ID)
	}

	// 1. Notify Customer / Pharmacy
	custTitle := fmt.Sprintf("تم استلام طلب التوريد #%s", orderNum)
	custBody := fmt.Sprintf("تم تسجيل طلبك بقيمة %s ج.م بنجاح، وجاري مراجعته وتجهيزه من قِبل الموردين المعتمدين.", order.TotalAmount.String())
	h.dispatchInAppNotification(ctx, order.CustomerID, nil, custTitle, custBody)
	if order.OrganizationID != nil && *order.OrganizationID > 0 {
		h.dispatchOrgNotification(ctx, *order.OrganizationID, custTitle, custBody)
	}

	// 2. Notify each Vendor Organization
	if pharmacyName == "" {
		pharmacyName = "إحدى الصيدليات المعتمدة"
	}

	for _, sh := range order.Shipments {
		if sh == nil || sh.OrganizationID <= 0 {
			continue
		}
		itemCount := len(sh.Lines)
		vendorTitle := fmt.Sprintf("طلب توريد جديد #%s", orderNum)
		vendorBody := fmt.Sprintf("ورد طلب توريد جديد من %s لعدد %d أصناف بقيمة %s ج.م. يرجى مراجعة وتأكيد الشحنة.",
			pharmacyName, itemCount, sh.Subtotal.String())
		h.dispatchOrgNotification(ctx, sh.OrganizationID, vendorTitle, vendorBody)
	}
}

// notifyOrderStatusChanged dispatches updates to the customer when a vendor updates shipment status.
func (h *UIHandler) notifyOrderStatusChanged(
	ctx context.Context,
	order *commerce.Order,
	shipmentID int64,
	toStatus commerce.OrderStatus,
	vendorName string,
	notes string,
) {
	if order == nil {
		return
	}

	orderNum := order.OrderNumber
	if orderNum == "" {
		orderNum = fmt.Sprintf("ORD-%d", order.ID)
	}

	if vendorName == "" {
		vendorName = "المورد"
	}

	var title, body string
	switch toStatus {
	case commerce.StatusConfirmed:
		title = fmt.Sprintf("تم قبول وتأكيد طلبك #%s", orderNum)
		body = fmt.Sprintf("قام %s بقبول طلبك وجاري تجهيز الشحنة بالمستودع تمهيداً للإرسال.", vendorName)
	case commerce.StatusShipped:
		title = fmt.Sprintf("شحنتك للطلب #%s في الطريق", orderNum)
		body = fmt.Sprintf("تم خروج شحنتك من مستودع %s وفي طريقها للتسليم لصيدليتك.", vendorName)
	case commerce.StatusDelivered:
		title = fmt.Sprintf("تم تسليم طلب التوريد #%s", orderNum)
		body = fmt.Sprintf("تم تأكيد تسليم وتوثيق استلام الشحنة من %s بنجاح.", vendorName)
	case commerce.StatusCancelled:
		title = fmt.Sprintf("تم إلغاء شحنة الطلب #%s", orderNum)
		body = fmt.Sprintf("تم إلغاء الشحنة من قِبل %s.", vendorName)
		if strings.TrimSpace(notes) != "" {
			body += fmt.Sprintf(" السبب: %s", notes)
		}
	default:
		title = fmt.Sprintf("تحديث حالة الطلب #%s", orderNum)
		body = fmt.Sprintf("تم تحديث حالة شحنتك لدى %s إلى: %s", vendorName, string(toStatus))
	}

	h.dispatchInAppNotification(ctx, order.CustomerID, nil, title, body)
	if order.OrganizationID != nil && *order.OrganizationID > 0 {
		h.dispatchOrgNotification(ctx, *order.OrganizationID, title, body)
	}
}

// notifyPurchaseRequestCreated dispatches notification to the vendor when a pharmacy submits a purchase request.
func (h *UIHandler) notifyPurchaseRequestCreated(ctx context.Context, vendorOrgID int64, pharmacyName string, requestID int64, itemCount int) {
	if vendorOrgID <= 0 {
		return
	}
	if pharmacyName == "" {
		pharmacyName = "إحدى الصيدليات"
	}
	title := fmt.Sprintf("طلب تسعير وشراء مباشر جديد #REQ-%d", requestID)
	body := fmt.Sprintf("ورد طلب تسعير وتوريد مباشر من %s لعدد %d أصناف. يرجى تقديم عروض الأسعار والكميات المتاحة.", pharmacyName, itemCount)
	h.dispatchOrgNotification(ctx, vendorOrgID, title, body)
}

// notifyPurchaseRequestResponded dispatches notification to the pharmacy when a vendor responds with prices.
func (h *UIHandler) notifyPurchaseRequestResponded(ctx context.Context, customerUserID int64, customerOrgID int64, vendorName string, requestID int64) {
	if vendorName == "" {
		vendorName = "المورد"
	}
	title := fmt.Sprintf("رد على طلب التسعير والشراء #REQ-%d", requestID)
	body := fmt.Sprintf("قام %s بالرد على طلب التسعير الخاص بك وتقديم عروض الأسعار والكميات.", vendorName)
	if customerUserID > 0 {
		h.dispatchInAppNotification(ctx, customerUserID, nil, title, body)
	}
	if customerOrgID > 0 {
		h.dispatchOrgNotification(ctx, customerOrgID, title, body)
	}
}

// notifyWalletDeposit dispatches notification when a deposit request is submitted or credited.
func (h *UIHandler) notifyWalletDeposit(ctx context.Context, userID int64, orgID int64, amount money.Amount, status string) {
	var title, body string
	if status == "approved" || status == "completed" {
		title = "تم شحن وإيداع الرصيد في المحفظة"
		body = fmt.Sprintf("تم قيد مبلغ %s ج.م بنجاح في رصيد محفظة منشأتكم.", amount.String())
	} else {
		title = "طلب شحن رصيد قيد المراجعة"
		body = fmt.Sprintf("تم استلام إشعار الإيداع بمبلغ %s ج.م وجاري تدقيقه واعتماده.", amount.String())
	}
	h.dispatchInAppNotification(ctx, userID, &orgID, title, body)
	if orgID > 0 {
		h.dispatchOrgNotification(ctx, orgID, title, body)
	}
}

// resolveOrgName retrieves the localized name of an organization.
func (h *UIHandler) resolveOrgName(ctx context.Context, orgID int64) string {
	if h.orgSvc == nil || orgID <= 0 {
		return ""
	}
	sysCtx := database.AsSystem(ctx)
	orgObj, err := h.orgSvc.GetOrganization(sysCtx, orgID)
	if err != nil || orgObj == nil {
		return ""
	}
	name := orgObj.TradeName.Get(i18n.AR)
	if name == "" {
		name = orgObj.TradeName.Get(i18n.EN)
	}
	if name == "" {
		name = orgObj.LegalName
	}
	return name
}
