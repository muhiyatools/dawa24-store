package ui

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/billing"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"

	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// AdminPlatformPaymentMethodSubmit creates or updates a platform supported payment channel.
func (h *UIHandler) AdminPlatformPaymentMethodSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || (!actor.IsStaff && !actor.IsPlatformAdmin()) {
		http.Redirect(w, r, "/auth/login?redirect=/admin/settings?tab=payment_methods", http.StatusSeeOther)
		return
	}

	_ = r.ParseForm()
	id := strings.TrimSpace(strings.ToLower(r.PostFormValue("id")))
	if id == "" {
		h.redirectWithNotice(w, r, "/admin/settings?tab=payment_methods", "error", "المعرف الفريد لوسيلة الدفع مطلوب.")
		return
	}

	nameAr := strings.TrimSpace(r.PostFormValue("name_ar"))
	nameEn := strings.TrimSpace(r.PostFormValue("name_en"))
	if nameEn == "" {
		nameEn = nameAr
	}
	if nameAr == "" {
		h.redirectWithNotice(w, r, "/admin/settings?tab=payment_methods", "error", "اسم وسيلة الدفع بالعربية مطلوب.")
		return
	}

	providerType := strings.TrimSpace(r.PostFormValue("provider_type"))
	if providerType == "" {
		providerType = "bank"
	}

	displayOrder, _ := strconv.Atoi(r.PostFormValue("display_order"))

	pm := &billing.PlatformPaymentMethod{
		ID:                id,
		Name:              i18n.New(nameAr, nameEn),
		ProviderType:      providerType,
		Description:       i18n.New(strings.TrimSpace(r.PostFormValue("description_ar")), strings.TrimSpace(r.PostFormValue("description_en"))),
		AccountName:       strings.TrimSpace(r.PostFormValue("account_name")),
		BankName:          strings.TrimSpace(r.PostFormValue("bank_name")),
		AccountNumber:     strings.TrimSpace(r.PostFormValue("account_number")),
		IBAN:              strings.TrimSpace(r.PostFormValue("iban")),
		SwiftCode:         strings.TrimSpace(r.PostFormValue("swift_code")),
		BranchName:        strings.TrimSpace(r.PostFormValue("branch_name")),
		InstaPayHandle:    strings.TrimSpace(r.PostFormValue("instapay_handle")),
		PhoneNumber:       strings.TrimSpace(r.PostFormValue("phone_number")),
		IsActive:          r.PostFormValue("is_active") == "1" || r.PostFormValue("is_active") == "true",
		IsDepositEnabled:  r.PostFormValue("is_deposit_enabled") == "1" || r.PostFormValue("is_deposit_enabled") == "true",
		IsCheckoutEnabled: r.PostFormValue("is_checkout_enabled") == "1" || r.PostFormValue("is_checkout_enabled") == "true",
		DisplayOrder:      displayOrder,
	}

	if h.billSvc != nil {
		if err := h.billSvc.SavePlatformPaymentMethod(ctx, pm); err != nil {
			h.log.ErrorContext(ctx, "failed to save platform payment method", "error", err, "id", id)
			h.redirectWithNotice(w, r, "/admin/settings?tab=payment_methods", "error", h.safeMessage(err, langOf(r)))
			return
		}
	}

	h.redirectWithNotice(w, r, "/admin/settings?tab=payment_methods", "success", "تم حفظ وتحديث وسيلة وقناة الدفع بنجاح.")
}

// AdminPlatformPaymentMethodToggleSubmit toggles the active state of a platform payment channel.
func (h *UIHandler) AdminPlatformPaymentMethodToggleSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || (!actor.IsStaff && !actor.IsPlatformAdmin()) {
		http.Redirect(w, r, "/auth/login?redirect=/admin/settings?tab=payment_methods", http.StatusSeeOther)
		return
	}

	_ = r.ParseForm()
	id := strings.TrimSpace(r.PostFormValue("id"))
	enabled := r.PostFormValue("enabled") == "1" || r.PostFormValue("enabled") == "true"

	if h.billSvc != nil && id != "" {
		if err := h.billSvc.TogglePlatformPaymentMethod(ctx, id, enabled); err != nil {
			h.log.ErrorContext(ctx, "failed to toggle platform payment method", "error", err, "id", id)
			h.redirectWithNotice(w, r, "/admin/settings?tab=payment_methods", "error", "فشل تحديث حالة وسيلة الدفع.")
			return
		}
	}

	msg := "تم تعطيل وسيلة الدفع مؤقتاً."
	if enabled {
		msg = "تم تفعيل وسيلة الدفع بنجاح."
	}
	h.redirectWithNotice(w, r, "/admin/settings?tab=payment_methods", "success", msg)
}

// AdminPlatformPaymentMethodDeleteSubmit deletes a platform payment channel.
func (h *UIHandler) AdminPlatformPaymentMethodDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || (!actor.IsStaff && !actor.IsPlatformAdmin()) {
		http.Redirect(w, r, "/auth/login?redirect=/admin/settings?tab=payment_methods", http.StatusSeeOther)
		return
	}

	id := chi.URLParam(r, "id")
	if h.billSvc != nil && id != "" {
		if err := h.billSvc.DeletePlatformPaymentMethod(ctx, id); err != nil {
			h.log.ErrorContext(ctx, "failed to delete platform payment method", "error", err, "id", id)
			h.redirectWithNotice(w, r, "/admin/settings?tab=payment_methods", "error", "فشل حذف وسيلة الدفع.")
			return
		}
	}

	h.redirectWithNotice(w, r, "/admin/settings?tab=payment_methods", "success", "تم حذف وسيلة الدفع بنجاح.")
}
