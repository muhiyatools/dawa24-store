package ui

import (
	"context"
	"fmt"
	"strings"

	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

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
