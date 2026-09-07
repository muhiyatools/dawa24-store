package ui

import (
	"context"
	"fmt"
	"strings"

	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// notifyWalletWithdrawal dispatches notification when a withdrawal request is submitted or approved.
func (h *UIHandler) notifyWalletWithdrawal(ctx context.Context, userID int64, orgID int64, amount money.Amount, status string) {
	var title, body string
	if status == "approved" || status == "completed" {
		title = i18n.T("ar", "notif.wallet_withdrawal_approved_title")
		body = fmt.Sprintf(i18n.T("ar", "notif.wallet_withdrawal_approved_body"), amount.String())
	} else {
		title = i18n.T("ar", "notif.wallet_withdrawal_pending_title")
		body = fmt.Sprintf(i18n.T("ar", "notif.wallet_withdrawal_pending_body"), amount.String())
	}
	perm := "vendor.wallet.view"
	if orgID > 0 && h.orgSvc != nil {
		if orgObj, err := h.orgSvc.GetOrganization(database.AsSystem(ctx), orgID); err == nil && orgObj != nil {
			if orgObj.Type == "pharmacy" || orgObj.Type == "customer" {
				perm = "pharmacy.wallet.view"
			}
		}
	}
	var orgPtr *int64
	if orgID > 0 {
		orgPtr = &orgID
	}
	h.dispatchInAppNotification(ctx, userID, orgPtr, perm, title, body)
	if orgID > 0 {
		h.dispatchOrgNotification(ctx, orgID, perm, title, body)
	}
}

// notifyWalletWithdrawalRejected dispatches notification when a withdrawal request is rejected with reason.
func (h *UIHandler) notifyWalletWithdrawalRejected(ctx context.Context, userID int64, orgID int64, amount money.Amount, reason string) {
	title := i18n.T("ar", "notif.wallet_withdrawal_rejected_title")
	body := fmt.Sprintf(i18n.T("ar", "notif.wallet_withdrawal_rejected_body"), amount.String())
	if strings.TrimSpace(reason) != "" {
		body += fmt.Sprintf(i18n.T("ar", "notif.reason_prefix"), reason)
	}
	perm := "vendor.wallet.view"
	if orgID > 0 && h.orgSvc != nil {
		if orgObj, err := h.orgSvc.GetOrganization(database.AsSystem(ctx), orgID); err == nil && orgObj != nil {
			if orgObj.Type == "pharmacy" || orgObj.Type == "customer" {
				perm = "pharmacy.wallet.view"
			}
		}
	}
	var orgPtr *int64
	if orgID > 0 {
		orgPtr = &orgID
	}
	h.dispatchInAppNotification(ctx, userID, orgPtr, perm, title, body)
	if orgID > 0 {
		h.dispatchOrgNotification(ctx, orgID, perm, title, body)
	}
}
