package ui

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/billing"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

func walletDestFor(actor authctx.Actor) string {
	if actor.IsVendor() {
		return "/vendor/wallet"
	}
	return "/customer/wallet"
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

	var wallet *billing.Wallet
	var txs []*billing.WalletTransaction
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
		if deps, err := h.billSvc.ListUserDeposits(ctx, actor.UserID, 100, 0); err == nil {
			depositRequests = deps
		}
		if wItem, err := h.billSvc.GetWallet(ctx, actor.UserID, "EGP"); err == nil && wItem != nil {
			wallet = wItem
			if list, err := h.billSvc.ListWalletTransactions(ctx, wItem.ID, 100, 0); err == nil {
				txs = list
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
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.WalletPage(viewData, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render tenant wallet page", "error", err)
	}
}

// TenantWalletDepositSubmit handles wallet recharge deposit submissions.
func (h *UIHandler) TenantWalletDepositSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
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
		h.redirectWithNotice(w, r, dest, "error", "يرجى إدخال مبلغ إيداع صالح وموجب.")
		return
	}

	method := strings.TrimSpace(r.PostFormValue("payment_method"))
	ref := strings.TrimSpace(r.PostFormValue("reference_number"))
	notes := strings.TrimSpace(r.PostFormValue("notes"))

	if method == "" {
		h.redirectWithNotice(w, r, dest, "error", "يرجى اختيار وسيلة الدفع أو التحويل.")
		return
	}
	if ref == "" {
		h.redirectWithNotice(w, r, dest, "error", "يرجى إدخال رقم الإشعار أو مرجع التحويل.")
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
		h.redirectWithNotice(w, r, dest, "error", "خدمة المحفظة والفواتير غير متوفرة حالياً.")
		return
	}

	var orgPtr *int64
	if actor.OrganizationID > 0 {
		orgPtr = &actor.OrganizationID
	}

	if _, err := h.billSvc.RequestDeposit(ctx, actor.UserID, orgPtr, "EGP", amt, method, ref, attachmentURL, notes); err != nil {
		h.log.ErrorContext(ctx, "failed to submit deposit request", "error", err)
		h.redirectWithNotice(w, r, dest, "error", h.safeMessage(err, langOf(r)))
		return
	}

	h.redirectWithNotice(w, r, dest, "success", "تم تسجيل طلب شحن الرصيد بنجاح، والعملية قيد مراجعة وتدقيق الإدارة المالية.")
}

// TenantWalletWithdrawSubmit handles wallet withdrawal requests.
func (h *UIHandler) TenantWalletWithdrawSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
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
		h.redirectWithNotice(w, r, dest, "error", "يرجى إدخال مبلغ سحب صالح وموجب.")
		return
	}

	destAcc := strings.TrimSpace(r.PostFormValue("destination_id"))
	if destAcc == "" {
		h.redirectWithNotice(w, r, dest, "error", "يرجى إدخال رقم الحساب أو الآيبان المستلم.")
		return
	}

	reason := r.PostFormValue("reason")
	desc := fmt.Sprintf("طلب سحب رصيد إلى: %s", destAcc)
	if reason != "" {
		desc += fmt.Sprintf(" (السبب: %s)", reason)
	}

	if h.billSvc == nil {
		h.redirectWithNotice(w, r, dest, "error", "خدمة المحفظة والفواتير غير متوفرة حالياً.")
		return
	}

	if _, err := h.billSvc.Withdraw(ctx, actor.UserID, "EGP", amt, "user_withdrawal", nil, desc); err != nil {
		h.log.ErrorContext(ctx, "failed wallet withdrawal", "error", err)
		h.redirectWithNotice(w, r, dest, "error", h.safeMessage(err, langOf(r)))
		return
	}

	h.redirectWithNotice(w, r, dest, "success", "تم خصم وتسجيل طلب السحب بنجاح.")
}

// TenantPaymentMethodAddSubmit saves a new payment method from the dedicated wallet page.
func (h *UIHandler) TenantPaymentMethodAddSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.UserID == 0 {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	dest := walletDestFor(actor)

	if h.billSvc == nil {
		h.redirectWithNotice(w, r, dest, "error", "خدمة المدفوعات غير متاحة حالياً.")
		return
	}

	_ = r.ParseForm()
	provider, identifier, err := buildPaymentMethodIdentifier(r)
	if err != nil {
		h.redirectWithNotice(w, r, dest, "error", err.Error())
		return
	}

	isDefault := r.PostFormValue("is_default") == "1"

	pm := &billing.UserPaymentMethod{
		UserID:            actor.UserID,
		Provider:          provider,
		AccountIdentifier: identifier,
		IsDefault:         isDefault,
	}

	if err := h.billSvc.AddPaymentMethod(ctx, pm); err != nil {
		h.log.ErrorContext(ctx, "failed to add payment method", "error", err)
		h.redirectWithNotice(w, r, dest, "error", h.safeMessage(err, langOf(r)))
		return
	}

	h.redirectWithNotice(w, r, dest, "success", "تمت إضافة وحفظ وسيلة الدفع بنجاح.")
}

// TenantPaymentMethodDeleteSubmit deletes a saved payment method from the dedicated wallet page.
func (h *UIHandler) TenantPaymentMethodDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.UserID == 0 {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	dest := walletDestFor(actor)

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, dest, "error", "معرف وسيلة الدفع غير صالح.")
		return
	}

	if h.billSvc != nil {
		if err := h.billSvc.DeletePaymentMethod(ctx, actor.UserID, id); err != nil {
			h.log.ErrorContext(ctx, "failed to delete payment method", "error", err)
			h.redirectWithNotice(w, r, dest, "error", h.safeMessage(err, langOf(r)))
			return
		}
	}

	h.redirectWithNotice(w, r, dest, "success", "تم حذف وسيلة الدفع بنجاح.")
}
