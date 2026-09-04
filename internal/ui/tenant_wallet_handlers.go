package ui

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/billing"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/shared/pagination"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

func walletDestFor(actor authctx.Actor) string {
	if actor.IsVendor() {
		return "/vendor/wallet"
	}
	return "/customer/wallet"
}

// normalizeWalletStatusFilter keeps only the supported wallet status values:
// pending (قيد المراجعة), completed (مكتمل), rejected (مرفوض).
// "approved" is accepted as an alias of completed.
func normalizeWalletStatusFilter(v string) string {
	switch strings.TrimSpace(v) {
	case "pending":
		return "pending"
	case "completed", "approved":
		return "completed"
	case "rejected":
		return "rejected"
	default:
		return ""
	}
}

// normalizeWalletTypeFilter keeps only the supported wallet transaction types:
// deposit (إيداع), withdrawal (عملية سحب), purchase (مشتريات).
func normalizeWalletTypeFilter(v string) string {
	switch strings.TrimSpace(v) {
	case string(billing.TxDeposit), string(billing.TxWithdrawal), string(billing.TxPurchase):
		return strings.TrimSpace(v)
	default:
		return ""
	}
}

// TenantWalletPage renders the dedicated unified Wallet, Balance, and Payment Methods page
// for Pharmacy customers and Vendors.
func (h *UIHandler) TenantWalletPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.UserID == 0 {
		http.Redirect(w, r, "/auth/login?redirect="+r.URL.RequestURI(), http.StatusSeeOther)
		return
	}

	lang, dir := h.localeAndDir(r)
	isVendor := actor.IsVendor()

	page := pagination.PageNumber(r)
	limit := pagination.RowsPerPage(r)
	offset := (page - 1) * limit

	// Transaction status filter: pending (قيد المراجعة) | completed (مكتمل) | rejected (مرفوض).
	txStatus := normalizeWalletStatusFilter(r.URL.Query().Get("status"))
	// Transaction type filter: deposit (إيداع) | withdrawal (عملية سحب) | purchase (مشتريات).
	txType := normalizeWalletTypeFilter(r.URL.Query().Get("type"))

	var wallet *billing.Wallet
	var txs []*billing.WalletTransaction
	var totalTxCount int
	var depositRequests []*billing.WalletDeposit
	var paymentMethods []*billing.UserPaymentMethod
	var platformPaymentMethods []*billing.PlatformPaymentMethod

	if h.billSvc != nil {
		if pms, err := h.billSvc.ListPaymentMethods(ctx, actor.UserID); err == nil {
			paymentMethods = pms
		}
		if ppms, err := h.billSvc.ListPlatformPaymentMethods(ctx, true); err == nil {
			platformPaymentMethods = ppms
		}
		// Deposit requests back the status filter (billing.wallet_deposits.status).
		// pending/rejected show only matching requests; completed shows the
		// ledger only (approved deposits already have ledger rows).
		// A type filter other than deposit hides deposit requests entirely.
		wantDeposits := txStatus != "completed" && (txType == "" || txType == string(billing.TxDeposit))
		if wantDeposits {
			depStatus := ""
			if txStatus == "pending" || txStatus == "rejected" {
				depStatus = txStatus
			}
			if deps, err := h.billSvc.ListUserDepositsWithStatus(ctx, actor.UserID, depStatus, 50, 0); err == nil {
				depositRequests = deps
			}
		}
		if wItem, err := h.billSvc.GetWallet(ctx, actor.UserID, "EGP"); err == nil && wItem != nil {
			wallet = wItem
			// Ledger rows back the type filter (billing.wallet_transactions.type).
			// A status of pending/rejected shows requests only, no ledger rows.
			if txStatus == "" || txStatus == "completed" {
				if list, total, err := h.billSvc.ListWalletTransactionsWithTypeTotal(ctx, wItem.ID, txType, limit, offset); err == nil {
					txs = list
					totalTxCount = total
				}
			}
		}
	}

	viewData := pages.WalletViewData{
		IsVendor:               isVendor,
		Wallet:                 wallet,
		Transactions:           txs,
		DepositRequests:        depositRequests,
		PaymentMethods:         paymentMethods,
		PlatformPaymentMethods: platformPaymentMethods,
		NoticeType:             r.URL.Query().Get("notice"),
		NoticeMessage:          r.URL.Query().Get("msg"),
		TxStatus:               txStatus,
		TxType:                 txType,
		Page:                   page,
		PerPage:                limit,
		TotalCount:             totalTxCount,
	}

	h.renderPage(ctx, w, "render tenant wallet page", pages.WalletPage(viewData, lang, dir))
}

// TenantWalletDepositSubmit handles wallet recharge deposit submissions.
func (h *UIHandler) TenantWalletDepositSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok || actor.UserID == 0 {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	dest := walletDestFor(actor)

	_ = r.ParseMultipartForm(MaxUploadBytes)
	amountStr := r.PostFormValue("amount")
	amt, err := money.Parse(amountStr)
	if err != nil || amt.IsZero() || amt.IsNegative() {
		h.redirectWithNotice(w, r, dest, "error", i18n.T(lang, "wallet.deposit.invalid_amount"))
		return
	}

	method := strings.TrimSpace(r.PostFormValue("payment_method"))
	ref := strings.TrimSpace(r.PostFormValue("reference_number"))
	notes := strings.TrimSpace(r.PostFormValue("notes"))

	if method == "" {
		h.redirectWithNotice(w, r, dest, "error", i18n.T(lang, "wallet.deposit.method_required"))
		return
	}
	if ref == "" {
		h.redirectWithNotice(w, r, dest, "error", i18n.T(lang, "wallet.deposit.reference_required"))
		return
	}

	var attachmentURL string
	if file, _, err := r.FormFile("receipt"); err == nil && file != nil {
		_ = file.Close()
		if savedPath, err := saveUploadedFile(r, "receipt", "receipts"); err == nil {
			attachmentURL = savedPath
		}
	}

	if h.billSvc == nil {
		h.redirectWithNotice(w, r, dest, "error", i18n.T(lang, "wallet.service_unavailable"))
		return
	}

	var orgPtr *int64
	if actor.OrganizationID > 0 {
		orgPtr = &actor.OrganizationID
	}

	if _, err := h.billSvc.RequestDeposit(ctx, actor.UserID, orgPtr, "EGP", amt, method, ref, attachmentURL, notes); err != nil {
		h.log.ErrorContext(ctx, "failed to submit deposit request", "error", err)
		h.redirectWithNotice(w, r, dest, "error", h.safeMessage(err, lang))
		return
	}

	// Dispatch in-app notification
	go h.notifyWalletDeposit(context.Background(), actor.UserID, actor.OrganizationID, amt, "pending")

	h.redirectWithNotice(w, r, dest, "success", i18n.T(lang, "wallet.deposit.pending_success"))
}

// TenantWalletWithdrawSubmit handles wallet withdrawal requests.
func (h *UIHandler) TenantWalletWithdrawSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok || actor.UserID == 0 {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	dest := walletDestFor(actor)

	_ = r.ParseForm()
	amountStr := r.PostFormValue("amount")
	amt, err := money.Parse(amountStr)
	if err != nil || amt.IsZero() || amt.IsNegative() {
		h.redirectWithNotice(w, r, dest, "error", i18n.T(lang, "wallet.withdraw.invalid_amount"))
		return
	}

	destAcc := strings.TrimSpace(r.PostFormValue("destination_id"))
	if destAcc == "" {
		h.redirectWithNotice(w, r, dest, "error", i18n.T(lang, "wallet.withdraw.destination_required"))
		return
	}

	reason := r.PostFormValue("reason")
	desc := fmt.Sprintf(i18n.T(lang, "wallet.withdraw.desc_prefix"), destAcc)
	if reason != "" {
		desc += fmt.Sprintf(i18n.T(lang, "wallet.withdraw.reason_suffix"), reason)
	}

	if h.billSvc == nil {
		h.redirectWithNotice(w, r, dest, "error", i18n.T(lang, "wallet.service_unavailable"))
		return
	}

	if _, err := h.billSvc.Withdraw(ctx, actor.UserID, "EGP", amt, "user_withdrawal", nil, desc); err != nil {
		h.log.ErrorContext(ctx, "failed wallet withdrawal", "error", err)
		h.redirectWithNotice(w, r, dest, "error", h.safeMessage(err, lang))
		return
	}

	h.redirectWithNotice(w, r, dest, "success", i18n.T(lang, "wallet.withdraw.success"))
}

// TenantPaymentMethodAddSubmit saves a new payment method from the dedicated wallet page.
func (h *UIHandler) TenantPaymentMethodAddSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok || actor.UserID == 0 {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	dest := walletDestFor(actor)

	if h.billSvc == nil {
		h.redirectWithNotice(w, r, dest, "error", i18n.T(lang, "payment.service_unavailable"))
		return
	}

	if err := r.ParseForm(); err != nil {
		h.redirectWithNotice(w, r, dest, "error", i18n.T(lang, "common.invalid_form_data"))
		return
	}
	in, err := readPaymentMethodForm(r)
	if err != nil {
		h.redirectWithNotice(w, r, dest, "error", err.Error())
		return
	}

	pm := &billing.UserPaymentMethod{
		UserID:            actor.UserID,
		Provider:          in.Provider,
		AccountIdentifier: in.Identifier,
		Details:           in.Details,
		IsDefault:         r.PostFormValue("is_default") == "1",
	}

	if err := h.billSvc.AddPaymentMethod(ctx, pm); err != nil {
		h.log.ErrorContext(ctx, "failed to add payment method", "error", err)
		h.redirectWithNotice(w, r, dest, "error", h.safeMessage(err, lang))
		return
	}

	h.redirectWithNotice(w, r, dest, "success", i18n.T(lang, "payment.created_success"))
}

// TenantPaymentMethodDeleteSubmit deletes a saved payment method from the dedicated wallet page.
func (h *UIHandler) TenantPaymentMethodDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok || actor.UserID == 0 {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	dest := walletDestFor(actor)

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, dest, "error", i18n.T(lang, "payment.invalid_id"))
		return
	}

	if h.billSvc != nil {
		if err := h.billSvc.DeletePaymentMethod(ctx, actor.UserID, id); err != nil {
			h.log.ErrorContext(ctx, "failed to delete payment method", "error", err)
			h.redirectWithNotice(w, r, dest, "error", h.safeMessage(err, lang))
			return
		}
	}

	h.redirectWithNotice(w, r, dest, "success", i18n.T(lang, "payment.deleted_success"))
}
