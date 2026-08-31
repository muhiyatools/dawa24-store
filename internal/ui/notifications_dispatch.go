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
	custTitle := fmt.Sprintf(i18n.T("ar", "notif.order_received_title"), orderNum)
	custBody := fmt.Sprintf(i18n.T("ar", "notif.order_received_body"), order.TotalAmount.String())
	h.dispatchInAppNotification(ctx, order.CustomerID, nil, custTitle, custBody)
	if order.OrganizationID != nil && *order.OrganizationID > 0 {
		h.dispatchOrgNotification(ctx, *order.OrganizationID, custTitle, custBody)
	}

	// 2. Notify each Vendor Organization
	if pharmacyName == "" {
		pharmacyName = i18n.T("ar", "notif.verified_pharmacy")
	}

	for _, sh := range order.Shipments {
		if sh == nil || sh.OrganizationID <= 0 {
			continue
		}
		itemCount := len(sh.Lines)
		vendorTitle := fmt.Sprintf(i18n.T("ar", "notif.new_supply_order_title"), orderNum)
		vendorBody := fmt.Sprintf(i18n.T("ar", "notif.new_supply_order_body"),
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
		vendorName = i18n.T("ar", "notif.the_vendor")
	}

	var title, body string
	switch toStatus {
	case commerce.StatusConfirmed:
		title = fmt.Sprintf(i18n.T("ar", "notif.order_confirmed_title"), orderNum)
		body = fmt.Sprintf(i18n.T("ar", "notif.order_confirmed_body"), vendorName)
	case commerce.StatusShipped:
		title = fmt.Sprintf(i18n.T("ar", "notif.order_shipped_title"), orderNum)
		body = fmt.Sprintf(i18n.T("ar", "notif.order_shipped_body"), vendorName)
	case commerce.StatusDelivered:
		title = fmt.Sprintf(i18n.T("ar", "notif.order_delivered_title"), orderNum)
		body = fmt.Sprintf(i18n.T("ar", "notif.order_delivered_body"), vendorName)
	case commerce.StatusCancelled:
		title = fmt.Sprintf(i18n.T("ar", "notif.order_cancelled_title"), orderNum)
		body = fmt.Sprintf(i18n.T("ar", "notif.order_cancelled_body"), vendorName)
		if strings.TrimSpace(notes) != "" {
			body += fmt.Sprintf(i18n.T("ar", "notif.reason_prefix"), notes)
		}
	default:
		title = fmt.Sprintf(i18n.T("ar", "notif.order_status_update_title"), orderNum)
		body = fmt.Sprintf(i18n.T("ar", "notif.order_status_update_body"), vendorName, string(toStatus))
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
		pharmacyName = i18n.T("ar", "notif.a_pharmacy")
	}
	title := fmt.Sprintf(i18n.T("ar", "notif.purchase_req_created_title"), requestID)
	body := fmt.Sprintf(i18n.T("ar", "notif.purchase_req_created_body"), pharmacyName, itemCount)
	h.dispatchOrgNotification(ctx, vendorOrgID, title, body)
}

// notifyPurchaseRequestResponded dispatches notification to the pharmacy when a vendor responds with prices.
func (h *UIHandler) notifyPurchaseRequestResponded(ctx context.Context, customerUserID int64, customerOrgID int64, vendorName string, requestID int64) {
	if vendorName == "" {
		vendorName = i18n.T("ar", "notif.the_vendor")
	}
	title := fmt.Sprintf(i18n.T("ar", "notif.purchase_req_responded_title"), requestID)
	body := fmt.Sprintf(i18n.T("ar", "notif.purchase_req_responded_body"), vendorName)
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
		title = i18n.T("ar", "notif.wallet_deposit_approved_title")
		body = fmt.Sprintf(i18n.T("ar", "notif.wallet_deposit_approved_body"), amount.String())
	} else {
		title = i18n.T("ar", "notif.wallet_deposit_pending_title")
		body = fmt.Sprintf(i18n.T("ar", "notif.wallet_deposit_pending_body"), amount.String())
	}
	h.dispatchInAppNotification(ctx, userID, &orgID, title, body)
	if orgID > 0 {
		h.dispatchOrgNotification(ctx, orgID, title, body)
	}
}

// notifyWalletDepositRejected dispatches notification when a deposit request is rejected with reason.
func (h *UIHandler) notifyWalletDepositRejected(ctx context.Context, userID int64, orgID int64, amount money.Amount, reason string) {
	title := i18n.T("ar", "notif.wallet_deposit_rejected_title")
	body := fmt.Sprintf(i18n.T("ar", "notif.wallet_deposit_rejected_body"), amount.String())
	if strings.TrimSpace(reason) != "" {
		body += fmt.Sprintf(i18n.T("ar", "notif.reason_prefix"), reason)
	}
	h.dispatchInAppNotification(ctx, userID, &orgID, title, body)
	if orgID > 0 {
		h.dispatchOrgNotification(ctx, orgID, title, body)
	}
}

// notifyAccountRegistered dispatches welcome notification to a newly registered account.
func (h *UIHandler) notifyAccountRegistered(ctx context.Context, userID int64, orgID *int64) {
	title := i18n.T("ar", "notif.account_registered_title")
	body := i18n.T("ar", "notif.account_registered_body")
	h.dispatchInAppNotification(ctx, userID, orgID, title, body)
	if orgID != nil && *orgID > 0 {
		h.dispatchOrgNotification(ctx, *orgID, title, body)
	}
}

// notifyOrgApproved dispatches celebration notification when admin approves an organization.
func (h *UIHandler) notifyOrgApproved(ctx context.Context, orgID int64) {
	if orgID <= 0 {
		return
	}
	title := i18n.T("ar", "notif.org_approved_title")
	body := i18n.T("ar", "notif.org_approved_body")
	h.dispatchOrgNotification(ctx, orgID, title, body)
}

// notifyOrgRejected dispatches notification when admin rejects an organization.
func (h *UIHandler) notifyOrgRejected(ctx context.Context, orgID int64, reason string) {
	if orgID <= 0 {
		return
	}
	title := i18n.T("ar", "notif.org_rejected_title")
	body := i18n.T("ar", "notif.org_rejected_body")
	if strings.TrimSpace(reason) != "" {
		body += fmt.Sprintf(i18n.T("ar", "notif.reason_prefix"), reason)
	}
	h.dispatchOrgNotification(ctx, orgID, title, body)
}

// notifyDocumentVerified dispatches notification when a business license or document is verified/rejected.
func (h *UIHandler) notifyDocumentVerified(ctx context.Context, orgID int64, docName string, verified bool, notes string) {
	if orgID <= 0 {
		return
	}
	if docName == "" {
		docName = i18n.TDefault("w4_ui.s_78_78")
	}
	var title, body string
	if verified {
		title = i18n.T("ar", "notif.doc_verified_title")
		body = fmt.Sprintf(i18n.T("ar", "notif.doc_verified_body"), docName)
	} else {
		title = i18n.T("ar", "notif.doc_rejected_title")
		body = fmt.Sprintf(i18n.T("ar", "notif.doc_rejected_body"), docName)
		if strings.TrimSpace(notes) != "" {
			body += fmt.Sprintf(i18n.T("ar", "notif.reason_prefix"), notes)
		}
	}
	h.dispatchOrgNotification(ctx, orgID, title, body)
}

// notifySpecialOfferStatus dispatches notification to the vendor when their special offer is approved or rejected.
func (h *UIHandler) notifySpecialOfferStatus(ctx context.Context, vendorOrgID int64, offerTitle string, approved bool, reason string) {
	if vendorOrgID <= 0 {
		return
	}
	if offerTitle == "" {
		offerTitle = i18n.TDefault("w4_ui.s_79_79")
	}
	var title, body string
	if approved {
		title = i18n.T("ar", "notif.special_offer_approved_title")
		body = fmt.Sprintf(i18n.T("ar", "notif.special_offer_approved_body"), offerTitle)
	} else {
		title = i18n.T("ar", "notif.special_offer_rejected_title")
		body = fmt.Sprintf(i18n.T("ar", "notif.special_offer_rejected_body"), offerTitle)
		if strings.TrimSpace(reason) != "" {
			body += fmt.Sprintf(i18n.T("ar", "notif.reason_prefix"), reason)
		}
	}
	h.dispatchOrgNotification(ctx, vendorOrgID, title, body)
}

// notifySponsorshipStatus dispatches notification when a sponsorship package request is approved or rejected.
func (h *UIHandler) notifySponsorshipStatus(ctx context.Context, vendorOrgID int64, pkgTitle string, approved bool, reason string) {
	if vendorOrgID <= 0 {
		return
	}
	if pkgTitle == "" {
		pkgTitle = i18n.TDefault("w4_ui.s_80_80")
	}
	var title, body string
	if approved {
		title = i18n.T("ar", "notif.sponsorship_approved_title")
		body = fmt.Sprintf(i18n.T("ar", "notif.sponsorship_approved_body"), pkgTitle)
	} else {
		title = i18n.T("ar", "notif.sponsorship_rejected_title")
		body = fmt.Sprintf(i18n.T("ar", "notif.sponsorship_rejected_body"), pkgTitle)
		if strings.TrimSpace(reason) != "" {
			body += fmt.Sprintf(i18n.T("ar", "notif.reason_prefix"), reason)
		}
	}
	h.dispatchOrgNotification(ctx, vendorOrgID, title, body)
}

// notifyAdStatus dispatches notification when an advertisement is approved or rejected.
func (h *UIHandler) notifyAdStatus(ctx context.Context, vendorOrgID int64, adTitle string, approved bool, reason string) {
	if vendorOrgID <= 0 {
		return
	}
	if adTitle == "" {
		adTitle = i18n.TDefault("w4_ui.s_81_81")
	}
	var title, body string
	if approved {
		title = i18n.T("ar", "notif.ad_approved_title")
		body = fmt.Sprintf(i18n.T("ar", "notif.ad_approved_body"), adTitle)
	} else {
		title = i18n.T("ar", "notif.ad_rejected_title")
		body = fmt.Sprintf(i18n.T("ar", "notif.ad_rejected_body"), adTitle)
		if strings.TrimSpace(reason) != "" {
			body += fmt.Sprintf(i18n.T("ar", "notif.reason_prefix"), reason)
		}
	}
	h.dispatchOrgNotification(ctx, vendorOrgID, title, body)
}

// notifyNegotiationOffer dispatches notification when a customer proposes a negotiated price.
func (h *UIHandler) notifyNegotiationOffer(ctx context.Context, vendorOrgID int64, customerOrgName string, orderNum string, proposedAmount money.Amount) {
	if vendorOrgID <= 0 {
		return
	}
	if customerOrgName == "" {
		customerOrgName = i18n.T("ar", "notif.verified_pharmacy")
	}
	title := i18n.T("ar", "notif.negotiation_offer_title")
	body := fmt.Sprintf(i18n.T("ar", "notif.negotiation_offer_body"), customerOrgName, proposedAmount.String(), orderNum)
	h.dispatchOrgNotification(ctx, vendorOrgID, title, body)
}

// notifyNegotiationDecision dispatches notification to the pharmacy when a vendor accepts or rejects price negotiation.
func (h *UIHandler) notifyNegotiationDecision(ctx context.Context, customerUserID int64, customerOrgID int64, vendorName string, orderNum string, accepted bool, reason string) {
	if vendorName == "" {
		vendorName = i18n.T("ar", "notif.the_vendor")
	}
	var title, body string
	if accepted {
		title = i18n.T("ar", "notif.negotiation_accepted_title")
		body = fmt.Sprintf(i18n.T("ar", "notif.negotiation_accepted_body"), vendorName, orderNum)
	} else {
		title = i18n.T("ar", "notif.negotiation_rejected_title")
		body = fmt.Sprintf(i18n.T("ar", "notif.negotiation_rejected_body"), orderNum, vendorName)
		if strings.TrimSpace(reason) != "" {
			body += fmt.Sprintf(i18n.T("ar", "notif.reason_prefix"), reason)
		}
	}
	if customerUserID > 0 {
		h.dispatchInAppNotification(ctx, customerUserID, nil, title, body)
	}
	if customerOrgID > 0 {
		h.dispatchOrgNotification(ctx, customerOrgID, title, body)
	}
}

// notifyQuoteRequest dispatches notification to the vendor when a new RFQ is created.
func (h *UIHandler) notifyQuoteRequest(ctx context.Context, vendorOrgID int64, customerName string, productName string, quantity int) {
	if vendorOrgID <= 0 {
		return
	}
	if customerName == "" {
		customerName = i18n.T("ar", "notif.verified_pharmacy")
	}
	title := i18n.T("ar", "notif.quote_req_title")
	body := fmt.Sprintf(i18n.T("ar", "notif.quote_req_body"), customerName, productName, quantity)
	h.dispatchOrgNotification(ctx, vendorOrgID, title, body)
}

// notifyQuoteProvided dispatches notification to the customer when a vendor submits a price quote.
func (h *UIHandler) notifyQuoteProvided(ctx context.Context, customerUserID int64, customerOrgID int64, vendorName string, productName string, quotePrice money.Amount) {
	if vendorName == "" {
		vendorName = i18n.T("ar", "notif.the_vendor")
	}
	title := i18n.T("ar", "notif.quote_provided_title")
	body := fmt.Sprintf(i18n.T("ar", "notif.quote_provided_body"), vendorName, productName, quotePrice.String())
	if customerUserID > 0 {
		h.dispatchInAppNotification(ctx, customerUserID, nil, title, body)
	}
	if customerOrgID > 0 {
		h.dispatchOrgNotification(ctx, customerOrgID, title, body)
	}
}

// notifyQuoteDecision dispatches notification to the vendor when a customer accepts or rejects a price quote.
func (h *UIHandler) notifyQuoteDecision(ctx context.Context, vendorOrgID int64, customerName string, productName string, accepted bool) {
	if vendorOrgID <= 0 {
		return
	}
	if customerName == "" {
		customerName = i18n.T("ar", "notif.verified_pharmacy")
	}
	var title, body string
	if accepted {
		title = i18n.T("ar", "notif.quote_accepted_title")
		body = fmt.Sprintf(i18n.T("ar", "notif.quote_accepted_body"), customerName, productName)
	} else {
		title = i18n.T("ar", "notif.quote_rejected_title")
		body = fmt.Sprintf(i18n.T("ar", "notif.quote_rejected_body"), productName, customerName)
	}
	h.dispatchOrgNotification(ctx, vendorOrgID, title, body)
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
