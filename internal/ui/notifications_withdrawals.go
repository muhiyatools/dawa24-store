package ui

import (
	"context"
	"fmt"
	"strings"

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
	var orgPtr *int64
	if orgID > 0 {
		orgPtr = &orgID
	}
	h.dispatchInAppNotification(ctx, userID, orgPtr, title, body)
	if orgID > 0 {
		h.dispatchOrgNotification(ctx, orgID, title, body)
	}
}

// notifyWalletWithdrawalRejected dispatches notification when a withdrawal request is rejected with reason.
func (h *UIHandler) notifyWalletWithdrawalRejected(ctx context.Context, userID int64, orgID int64, amount money.Amount, reason string) {
	title := i18n.T("ar", "notif.wallet_withdrawal_rejected_title")
	body := fmt.Sprintf(i18n.T("ar", "notif.wallet_withdrawal_rejected_body"), amount.String())
	if strings.TrimSpace(reason) != "" {
		body += fmt.Sprintf(i18n.T("ar", "notif.reason_prefix"), reason)
	}
	var orgPtr *int64
	if orgID > 0 {
		orgPtr = &orgID
	}
	h.dispatchInAppNotification(ctx, userID, orgPtr, title, body)
	if orgID > 0 {
		h.dispatchOrgNotification(ctx, orgID, title, body)
	}
}
