package ui

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// WalletPage redirects to the dedicated wallet page.
func (h *UIHandler) WalletPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.UserID == 0 {
		http.Redirect(w, r, "/customer/wallet", http.StatusMovedPermanently)
		return
	}
	http.Redirect(w, r, walletDestFor(actor), http.StatusMovedPermanently)
}

// WalletDepositSubmit handles submitting a funds deposit request, placing it in pending status for admin review.
func (h *UIHandler) WalletDepositSubmit(w http.ResponseWriter, r *http.Request) {
	h.TenantWalletDepositSubmit(w, r)
}

// WalletDepositEditSubmit handles updating a pending deposit request before admin review.
func (h *UIHandler) WalletDepositEditSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	dest := walletDestFor(actor)

	depositID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || depositID <= 0 {
		h.redirectWithNotice(w, r, dest, "error", "معرف عملية الإيداع غير صالح.")
		return
	}

	_ = r.ParseMultipartForm(MaxUploadBytes)
	amountStr := r.PostFormValue("amount")
	amt, err := money.Parse(amountStr)
	if err != nil || amt.IsZero() || amt.IsNegative() {
		h.redirectWithNotice(w, r, dest, "error", "يرجى إدخال مبلغ إيداع صالح.")
		return
	}

	method := strings.TrimSpace(r.PostFormValue("payment_method"))
	ref := strings.TrimSpace(r.PostFormValue("reference_number"))
	notes := strings.TrimSpace(r.PostFormValue("notes"))

	var attachmentURL string
	if file, _, err := r.FormFile("receipt"); err == nil && file != nil {
		_ = file.Close()
		if savedPath, err := saveUploadedFile(r, "receipt", "receipts"); err == nil {
			attachmentURL = savedPath
		}
	}

	if h.billSvc == nil {
		h.redirectWithNotice(w, r, dest, "error", "خدمة المحفظة والفواتير غير متوفرة.")
		return
	}

	if _, err := h.billSvc.EditPendingDeposit(ctx, actor.UserID, depositID, amt, method, ref, attachmentURL, notes); err != nil {
		h.log.ErrorContext(ctx, "failed to update pending deposit", "error", err, "deposit_id", depositID)
		h.redirectWithNotice(w, r, dest, "error", h.safeMessage(err, langOf(r)))
		return
	}

	h.redirectWithNotice(w, r, dest, "success", "تم تحديث بيانات طلب شحن الرصيد بنجاح.")
}

// WalletWithdrawSubmit handles submitting a funds withdrawal request and debiting the wallet.
func (h *UIHandler) WalletWithdrawSubmit(w http.ResponseWriter, r *http.Request) {
	h.TenantWalletWithdrawSubmit(w, r)
}

// TenantSubscriptionCheckoutSubmit handles purchasing/upgrading a plan with wallet deduction.
func (h *UIHandler) TenantSubscriptionCheckoutSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || (actor.OrganizationID <= 0 && actor.UserID <= 0) {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	redirectURL := "/customer/subscription"
	if actor.IsVendor() {
		redirectURL = "/vendor/subscription"
	}

	if err := r.ParseForm(); err != nil {
		h.redirectWithNotice(w, r, redirectURL, "error", "بيانات غير صالحة.")
		return
	}

	planSlug := strings.TrimSpace(r.FormValue("plan_slug"))
	billingCycle := strings.TrimSpace(r.FormValue("billing_cycle"))
	autoRenew := r.FormValue("auto_renew") == "1" || r.FormValue("auto_renew") == "true" || r.FormValue("auto_renew") == "on"

	if planSlug == "" {
		h.redirectWithNotice(w, r, redirectURL, "error", "يرجى اختيار باقة صالحة.")
		return
	}

	if billingCycle != "annual" && billingCycle != "monthly" {
		billingCycle = "monthly"
	}

	var orgPtr *int64
	if actor.OrganizationID > 0 {
		orgPtr = &actor.OrganizationID
	}

	if h.billSvc == nil {
		h.redirectWithNotice(w, r, redirectURL, "error", "خدمة الاشتراكات غير متوفرة حالياً.")
		return
	}

	_, err := h.billSvc.SubscribeWithWallet(ctx, actor.UserID, orgPtr, planSlug, billingCycle, autoRenew)
	if err != nil {
		h.log.ErrorContext(ctx, "subscription checkout failed", "error", err, "user_id", actor.UserID, "plan", planSlug)
		h.redirectWithNotice(w, r, redirectURL, "error", h.safeMessage(err, langOf(r)))
		return
	}

	h.redirectWithNotice(w, r, redirectURL, "success", "تم تفعيل باقة الاشتراك وخصم القيمة من محفظتك بنجاح.")
}
