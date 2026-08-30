package ui

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/billing"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
)

func buildPaymentMethodIdentifier(r *http.Request) (string, string, error) {
	payType := strings.TrimSpace(r.PostFormValue("type"))
	switch payType {
	case "bank":
		bankName := strings.TrimSpace(r.PostFormValue("bank_name"))
		holder := strings.TrimSpace(r.PostFormValue("account_holder"))
		iban := strings.TrimSpace(r.PostFormValue("iban"))
		accNum := strings.TrimSpace(r.PostFormValue("account_number"))
		swift := strings.TrimSpace(r.PostFormValue("swift_code"))
		branch := strings.TrimSpace(r.PostFormValue("branch_name"))

		if iban == "" && accNum == "" {
			return "", "", fmt.Errorf("رقم الآيبان (IBAN) أو رقم الحساب البنكي مطلوب")
		}
		if bankName == "" {
			bankName = "حساب بنكي"
		}

		parts := []string{bankName}
		if holder != "" {
			parts = append(parts, holder)
		}
		if iban != "" {
			parts = append(parts, "IBAN: "+iban)
		}
		if accNum != "" {
			parts = append(parts, "حساب: "+accNum)
		}
		if swift != "" {
			parts = append(parts, "SWIFT: "+swift)
		}
		if branch != "" {
			parts = append(parts, "فرع "+branch)
		}
		return "bank", strings.Join(parts, " • "), nil

	case "instapay":
		handle := strings.TrimSpace(r.PostFormValue("instapay_handle"))
		holder := strings.TrimSpace(r.PostFormValue("account_holder"))
		if handle == "" {
			return "", "", fmt.Errorf("معرف إنستاباي (IPA) أو رقم الهاتف مطلوب")
		}
		if holder != "" {
			return "instapay", fmt.Sprintf("InstaPay: %s • %s", handle, holder), nil
		}
		return "instapay", "InstaPay: " + handle, nil

	case "wallet", "vodafone_cash":
		walletName := strings.TrimSpace(r.PostFormValue("wallet_provider"))
		if walletName == "" {
			walletName = "محفظة إلكترونية"
		}
		phone := strings.TrimSpace(r.PostFormValue("wallet_phone"))
		holder := strings.TrimSpace(r.PostFormValue("account_holder"))
		if phone == "" {
			return "", "", fmt.Errorf("رقم الهاتف المحمول للمحفظة مطلوب")
		}
		if holder != "" {
			return "wallet", fmt.Sprintf("%s: %s • %s", walletName, phone, holder), nil
		}
		return "wallet", fmt.Sprintf("%s: %s", walletName, phone), nil

	case "card":
		cardNum := strings.TrimSpace(r.PostFormValue("card_number"))
		cardName := strings.TrimSpace(r.PostFormValue("card_name"))
		cardBrand := strings.TrimSpace(r.PostFormValue("card_brand"))
		if cardBrand == "" {
			cardBrand = "Card"
		}
		if cardNum == "" {
			return "", "", fmt.Errorf("رقم البطاقة مطلوب")
		}
		cleanNum := strings.ReplaceAll(cardNum, " ", "")
		last4 := cleanNum
		if len(cleanNum) > 4 {
			last4 = cleanNum[len(cleanNum)-4:]
		}
		if cardName != "" {
			return "card", fmt.Sprintf("%s (•••• %s) - %s", cardBrand, last4, cardName), nil
		}
		return "card", fmt.Sprintf("%s (•••• %s)", cardBrand, last4), nil

	default:
		return "", "", fmt.Errorf("نوع وسيلة الدفع غير صالح")
	}
}

// SettingsPaymentMethodsSubmit saves a new payment method.
func (h *UIHandler) SettingsPaymentMethodsSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
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

// SettingsPaymentMethodEditSubmit updates an existing saved payment method or bank account.
func (h *UIHandler) SettingsPaymentMethodEditSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	dest := walletDestFor(actor)

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, dest, "error", "معرف وسيلة الدفع غير صالح.")
		return
	}

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
		ID:                id,
		UserID:            actor.UserID,
		Provider:          provider,
		AccountIdentifier: identifier,
		IsDefault:         isDefault,
	}

	if err := h.billSvc.UpdatePaymentMethod(ctx, pm); err != nil {
		h.log.ErrorContext(ctx, "failed to update payment method", "error", err, "id", id)
		h.redirectWithNotice(w, r, dest, "error", h.safeMessage(err, langOf(r)))
		return
	}

	h.redirectWithNotice(w, r, dest, "success", "تم تحديث بيانات وسيلة الدفع والحساب بنجاح.")
}

// SettingsPaymentMethodSetDefaultSubmit marks a payment method as default.
func (h *UIHandler) SettingsPaymentMethodSetDefaultSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
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
		if err := h.billSvc.SetDefaultPaymentMethod(ctx, actor.UserID, id); err != nil {
			h.log.ErrorContext(ctx, "failed to set default payment method", "error", err, "id", id)
			h.redirectWithNotice(w, r, dest, "error", h.safeMessage(err, langOf(r)))
			return
		}
	}

	h.redirectWithNotice(w, r, dest, "success", "تم تعيين وسيلة الدفع كافتراضية بنجاح.")
}

// SettingsPaymentMethodDeleteSubmit deletes a saved payment method.
func (h *UIHandler) SettingsPaymentMethodDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
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
