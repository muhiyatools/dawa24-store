package ui

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/billing"
	"github.com/muhiya/dawa24-store/internal/modules/identity"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// SettingsIndex renders the comprehensive unified tab-based account settings hub.
func (h *UIHandler) SettingsIndex(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/settings", http.StatusSeeOther)
		return
	}

	var user *identity.User
	var sessions []*identity.Session
	var sessionPlans []*identity.SessionPlan
	var paymentMethods []*billing.UserPaymentMethod
	var wallet *billing.Wallet
	var txs []*billing.WalletTransaction

	var platformPaymentMethods []*billing.PlatformPaymentMethod

	if h.idSvc != nil {
		if me, err := h.idSvc.GetMe(ctx, actor.UserID, nil); err != nil {
			h.log.DebugContext(ctx, "settings: get me optional", "error", err)
		} else if me != nil {
			user = me.User
		}
		if sess, err := h.idSvc.ListSessions(ctx, actor.UserID); err != nil {
			h.log.WarnContext(ctx, "settings: list sessions", "error", err)
		} else {
			sessions = sess
		}
		if plans, err := h.idSvc.ListSessionPlans(ctx); err != nil {
			h.log.WarnContext(ctx, "settings: list session plans", "error", err)
		} else {
			sessionPlans = plans
		}
	}

	if h.billSvc != nil {
		if pms, err := h.billSvc.ListPaymentMethods(ctx, actor.UserID); err != nil {
			h.log.WarnContext(ctx, "settings: list payment methods", "error", err)
		} else {
			paymentMethods = pms
		}
		if ppms, err := h.billSvc.ListPlatformPaymentMethods(ctx, true); err != nil {
			h.log.WarnContext(ctx, "settings: list platform payment methods", "error", err)
		} else {
			platformPaymentMethods = ppms
		}
		if w, err := h.billSvc.GetWallet(ctx, actor.UserID, "EGP"); err != nil {
			h.log.DebugContext(ctx, "settings: get wallet optional", "error", err)
		} else if w != nil {
			wallet = w
			if list, err := h.billSvc.ListWalletTransactions(ctx, w.ID, 50, 0); err != nil {
				h.log.WarnContext(ctx, "settings: list wallet transactions", "error", err)
			} else {
				txs = list
			}
		}
	}

	if user == nil {
		user = &identity.User{
			ID:    actor.UserID,
			Email: "user@dawa24.eg",
			Name:  i18n.Text{i18n.AR: "طبيب / صيدلي معتمد", i18n.EN: "Verified Pharmacist"},
			Role:  actor.Role,
		}
	}

	data := pages.UnifiedSettingsData{
		User:                   user,
		Wallet:                 wallet,
		Transactions:           txs,
		PaymentMethods:         paymentMethods,
		PlatformPaymentMethods: platformPaymentMethods,
		Sessions:               sessions,
		SessionPlans:           sessionPlans,
		ActiveTab:              "profile",
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.UnifiedSettingsPage(data, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render unified settings page", "error", err)
	}
}

// SettingsProfileSubmit saves name, phone, timezone, language, and avatar.
func (h *UIHandler) SettingsProfileSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/settings", http.StatusSeeOther)
		return
	}

	if h.idSvc == nil {
		h.redirectWithNotice(w, r, "/settings#profile", "error", "الخدمة غير متاحة حالياً.")
		return
	}

	name := r.PostFormValue("name")
	nameAr := r.PostFormValue("name_ar")
	if nameAr == "" && name != "" {
		nameAr = name
	}
	nameEn := r.PostFormValue("name_en")
	if nameEn == "" && name != "" {
		nameEn = name
	}

	_, err := h.idSvc.UpdateProfile(ctx, actor.UserID,
		nameAr, nameEn,
		r.PostFormValue("phone"), r.PostFormValue("timezone"),
		r.PostFormValue("lang"),
	)
	if err != nil {
		h.log.WarnContext(ctx, "profile update failed", "user_id", actor.UserID, "error", err)
		h.redirectWithNotice(w, r, "/settings#profile", "error", h.safeMessage(err, langOf(r)))
		return
	}

	if avatarURL := r.PostFormValue("avatar_url"); avatarURL != "" {
		_, _ = h.idSvc.UpdateAvatar(ctx, actor.UserID, avatarURL)
	}

	h.redirectWithNotice(w, r, "/settings#profile", "success", "تم حفظ التغييرات وتحديث الملف الشخصي بنجاح.")
}

// SettingsAddressSubmit adds a saved address.
func (h *UIHandler) SettingsAddressSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/settings/addresses", http.StatusSeeOther)
		return
	}

	if h.idSvc == nil {
		h.redirectWithNotice(w, r, "/settings/addresses", "error", "الخدمة غير متاحة حالياً.")
		return
	}

	cityID, _ := strconv.ParseInt(r.PostFormValue("city_id"), 10, 64)
	addr := &identity.UserAddress{
		UserID:    actor.UserID,
		Title:     r.PostFormValue("title"),
		Recipient: r.PostFormValue("recipient"),
		Phone:     r.PostFormValue("phone"),
		CityID:    cityID,
		Address:   r.PostFormValue("address"),
		Building:  r.PostFormValue("building"),
		Floor:     r.PostFormValue("floor"),
		Apartment: r.PostFormValue("apartment"),
		IsDefault: r.PostFormValue("is_default") == "1",
	}

	if _, err := h.idSvc.CreateAddress(ctx, addr); err != nil {
		h.redirectWithNotice(w, r, "/settings/addresses", "error", h.safeMessage(err, langOf(r)))
		return
	}
	h.redirectWithNotice(w, r, "/settings/addresses", "success", "تم حفظ العنوان.")
}

// SettingsAddressDeleteSubmit removes a saved address.
func (h *UIHandler) SettingsAddressDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/settings/addresses", http.StatusSeeOther)
		return
	}

	if h.idSvc != nil {
		if id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64); err == nil {
			_ = h.idSvc.DeleteAddress(ctx, id, actor.UserID)
		}
	}
	h.redirectWithNotice(w, r, "/settings/addresses", "success", "تم حذف العنوان.")
}

// SettingsSessionPlanPurchaseSubmit applies a session plan's concurrency limit.
func (h *UIHandler) SettingsSessionPlanPurchaseSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/settings/security", http.StatusSeeOther)
		return
	}

	if h.idSvc == nil {
		h.redirectWithNotice(w, r, "/settings/security", "error", "الخدمة غير متاحة حالياً.")
		return
	}
	planID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err := h.idSvc.PurchaseSessionPlan(ctx, actor.UserID, planID); err != nil {
		h.redirectWithNotice(w, r, "/settings/security", "error", h.safeMessage(err, langOf(r)))
		return
	}
	h.redirectWithNotice(w, r, "/settings/security", "success", "تم تفعيل الخطة.")
}

// SettingsSessionRevokeSubmit revokes one of the user's sessions.
func (h *UIHandler) SettingsSessionRevokeSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/settings/security", http.StatusSeeOther)
		return
	}

	if h.idSvc != nil {
		_ = h.idSvc.RevokeSession(ctx, r.PostFormValue("token"), actor.UserID)
	}
	h.redirectWithNotice(w, r, "/settings#security", "success", "تم إلغاء الجلسة.")
}

// SettingsPasswordSubmit updates the user's password from the settings form.
func (h *UIHandler) SettingsPasswordSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/settings", http.StatusSeeOther)
		return
	}

	if h.idSvc == nil {
		h.redirectWithNotice(w, r, "/settings#security", "error", "الخدمة غير متاحة حالياً.")
		return
	}

	_ = r.ParseForm()
	curr := r.PostFormValue("current_password")
	newPass := r.PostFormValue("new_password")
	confirmPass := r.PostFormValue("new_password_confirmation")

	if newPass == "" || newPass != confirmPass {
		h.redirectWithNotice(w, r, "/settings#security", "error", "كلمة المرور الجديدة غير متطابقة.")
		return
	}

	if err := h.idSvc.ChangePassword(ctx, actor.UserID, curr, newPass); err != nil {
		h.redirectWithNotice(w, r, "/settings#security", "error", h.safeMessage(err, langOf(r)))
		return
	}

	h.redirectWithNotice(w, r, "/settings#security", "success", "تم تغيير كلمة المرور بنجاح.")
}

// SettingsMemberRoleSubmit changes a member's organization role.
func (h *UIHandler) SettingsMemberRoleSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/settings/organization", http.StatusSeeOther)
		return
	}

	if h.orgSvc != nil {
		if userID, err := strconv.ParseInt(chi.URLParam(r, "userID"), 10, 64); err == nil {
			_ = h.orgSvc.UpdateMemberRole(ctx, actor.OrganizationID, userID, r.PostFormValue("role"))
		}
	}
	http.Redirect(w, r, "/settings/organization", http.StatusSeeOther)
}

// SettingsOrgUpdateSubmit saves organization profile fields.
func (h *UIHandler) SettingsOrgUpdateSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/settings/organization", http.StatusSeeOther)
		return
	}

	if h.orgSvc == nil {
		h.redirectWithNotice(w, r, "/settings/organization", "error", "الخدمة غير متاحة حالياً.")
		return
	}

	o, err := h.orgSvc.GetOrganization(ctx, actor.OrganizationID)
	if err != nil {
		h.redirectWithNotice(w, r, "/settings/organization", "error", h.safeMessage(err, langOf(r)))
		return
	}
	o.LegalName = r.PostFormValue("legal_name")
	o.TradeName = i18n.New(r.PostFormValue("trade_name_ar"), r.PostFormValue("trade_name_en"))
	o.TaxNumber = r.PostFormValue("tax_number")
	o.CommercialRegister = r.PostFormValue("commercial_register")

	if err := h.orgSvc.UpdateOrganization(ctx, o); err != nil {
		h.redirectWithNotice(w, r, "/settings/organization", "error", h.safeMessage(err, langOf(r)))
		return
	}
	h.redirectWithNotice(w, r, "/settings/organization", "success", "تم حفظ بيانات المؤسسة.")
}

// SettingsPreferencesSubmit saves the user's preferences.
func (h *UIHandler) SettingsPreferencesSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/settings/preferences", http.StatusSeeOther)
		return
	}

	if h.idSvc == nil {
		h.redirectWithNotice(w, r, "/settings/preferences", "error", "الخدمة غير متاحة حالياً.")
		return
	}

	p := &identity.UserPreferences{
		UserID: actor.UserID,
		Theme:  r.PostFormValue("theme"),
		NotificationChannels: map[string]bool{
			"email": r.PostFormValue("ch_email") == "on",
			"sms":   r.PostFormValue("ch_sms") == "on",
			"push":  r.PostFormValue("ch_push") == "on",
		},
		NotificationTopics: map[string]bool{"offers": true, "blog": false, "newsletter": true},
		MarketingConsent:   r.PostFormValue("marketing_consent") == "on",
	}
	if p.Theme == "" {
		p.Theme = "light"
	}
	if err := h.idSvc.UpdatePreferences(ctx, p); err != nil {
		h.redirectWithNotice(w, r, "/settings/preferences", "error", h.safeMessage(err, langOf(r)))
		return
	}
	h.redirectWithNotice(w, r, "/settings/preferences", "success", "تم حفظ التفضيلات.")
}

// SettingsMemberAddSubmit adds an existing user to the organization by email.
func (h *UIHandler) SettingsMemberAddSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/settings/organization", http.StatusSeeOther)
		return
	}

	if h.idSvc == nil || h.orgSvc == nil {
		h.redirectWithNotice(w, r, "/settings/organization", "error", "الخدمة غير متاحة حالياً.")
		return
	}

	user, err := h.idSvc.GetUserByEmail(ctx, r.PostFormValue("email"))
	if err != nil {
		h.redirectWithNotice(w, r, "/settings/organization", "error", "لا يوجد مستخدم بهذا البريد الإلكتروني.")
		return
	}

	if _, err := h.orgSvc.AddMemberByRoleKey(ctx, actor.OrganizationID, user.ID, r.PostFormValue("role")); err != nil {
		h.redirectWithNotice(w, r, "/settings/organization", "error", h.safeMessage(err, langOf(r)))
		return
	}
	h.redirectWithNotice(w, r, "/settings/organization", "success", "تمت إضافة العضو.")
}

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
		http.Redirect(w, r, "/auth/login?redirect=/settings", http.StatusSeeOther)
		return
	}

	if h.billSvc == nil {
		h.redirectWithNotice(w, r, "/settings?tab=payments", "error", "خدمة المدفوعات غير متاحة حالياً.")
		return
	}

	_ = r.ParseForm()
	provider, identifier, err := buildPaymentMethodIdentifier(r)
	if err != nil {
		h.redirectWithNotice(w, r, "/settings?tab=payments", "error", err.Error())
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
		h.redirectWithNotice(w, r, "/settings?tab=payments", "error", h.safeMessage(err, langOf(r)))
		return
	}

	h.redirectWithNotice(w, r, "/settings?tab=payments", "success", "تمت إضافة وحفظ وسيلة الدفع بنجاح.")
}

// SettingsPaymentMethodEditSubmit updates an existing saved payment method or bank account.
func (h *UIHandler) SettingsPaymentMethodEditSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/settings", http.StatusSeeOther)
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/settings?tab=payments", "error", "معرف وسيلة الدفع غير صالح.")
		return
	}

	if h.billSvc == nil {
		h.redirectWithNotice(w, r, "/settings?tab=payments", "error", "خدمة المدفوعات غير متاحة حالياً.")
		return
	}

	_ = r.ParseForm()
	provider, identifier, err := buildPaymentMethodIdentifier(r)
	if err != nil {
		h.redirectWithNotice(w, r, "/settings?tab=payments", "error", err.Error())
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
		h.redirectWithNotice(w, r, "/settings?tab=payments", "error", h.safeMessage(err, langOf(r)))
		return
	}

	h.redirectWithNotice(w, r, "/settings?tab=payments", "success", "تم تحديث بيانات وسيلة الدفع والحساب بنجاح.")
}

// SettingsPaymentMethodSetDefaultSubmit marks a payment method as default.
func (h *UIHandler) SettingsPaymentMethodSetDefaultSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/settings", http.StatusSeeOther)
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/settings?tab=payments", "error", "معرف وسيلة الدفع غير صالح.")
		return
	}

	if h.billSvc != nil {
		if err := h.billSvc.SetDefaultPaymentMethod(ctx, actor.UserID, id); err != nil {
			h.log.ErrorContext(ctx, "failed to set default payment method", "error", err, "id", id)
			h.redirectWithNotice(w, r, "/settings?tab=payments", "error", h.safeMessage(err, langOf(r)))
			return
		}
	}

	h.redirectWithNotice(w, r, "/settings?tab=payments", "success", "تم تعيين وسيلة الدفع كافتراضية بنجاح.")
}

// SettingsPaymentMethodDeleteSubmit deletes a saved payment method.
func (h *UIHandler) SettingsPaymentMethodDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/settings", http.StatusSeeOther)
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/settings?tab=payments", "error", "معرف وسيلة الدفع غير صالح.")
		return
	}

	if h.billSvc != nil {
		if err := h.billSvc.DeletePaymentMethod(ctx, actor.UserID, id); err != nil {
			h.log.ErrorContext(ctx, "failed to delete payment method", "error", err)
			h.redirectWithNotice(w, r, "/settings?tab=payments", "error", h.safeMessage(err, langOf(r)))
			return
		}
	}

	h.redirectWithNotice(w, r, "/settings?tab=payments", "success", "تم حذف وسيلة الدفع بنجاح.")
}

// SettingsEmployeesPage renders the employee roster and branch manager assignments.
func (h *UIHandler) SettingsEmployeesPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/settings/employees", http.StatusSeeOther)
		return
	}

	var employees []*org.EmployeeView
	var branches []*org.Branch
	var roles []*org.Role

	if h.orgSvc != nil && actor.OrganizationID > 0 {
		if emps, err := h.orgSvc.ListEmployees(ctx, actor.OrganizationID); err == nil {
			employees = emps
		} else {
			h.log.ErrorContext(ctx, "failed to list employees", "error", err, "org_id", actor.OrganizationID)
		}
		if b, err := h.orgSvc.ListBranches(ctx, actor.OrganizationID); err == nil {
			branches = b
		} else {
			h.log.ErrorContext(ctx, "failed to list branches", "error", err, "org_id", actor.OrganizationID)
		}
		if rl, err := h.orgSvc.ListRoles(ctx, actor.OrganizationID); err == nil {
			roles = rl
		} else {
			h.log.ErrorContext(ctx, "failed to list roles", "error", err, "org_id", actor.OrganizationID)
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.SettingsEmployees(employees, branches, roles, lang, dir, actor).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render settings employees", "error", err)
	}
}

// SettingsEmployeeCreateSubmit creates a new employee account and assigns them to the organization and branch.
func (h *UIHandler) SettingsEmployeeCreateSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/settings/employees", http.StatusSeeOther)
		return
	}

	if h.idSvc == nil || h.orgSvc == nil {
		h.redirectWithNotice(w, r, "/settings/employees", "error", "الخدمة غير متاحة حالياً.")
		return
	}

	_ = r.ParseForm()

	email := r.PostFormValue("email")
	name := r.PostFormValue("name")
	phone := r.PostFormValue("phone")
	jobTitle := r.PostFormValue("job_title")
	employeeCode := r.PostFormValue("employee_code")
	roleKey := r.PostFormValue("role_key")
	if roleKey == "" {
		roleKey = "org_employee"
	}
	salaryStr := r.PostFormValue("base_salary")
	var baseSalary money.Amount
	if salaryStr != "" {
		baseSalary, _ = money.Parse(salaryStr)
	}

	var branchID *int64
	if bStr := r.PostFormValue("branch_id"); bStr != "" {
		if bID, err := strconv.ParseInt(bStr, 10, 64); err == nil && bID > 0 {
			branchID = &bID
		}
	}

	// 1. Locate or create user account
	var targetUserID int64
	existingUser, err := h.idSvc.GetUserByEmail(ctx, email)
	if err == nil && existingUser != nil {
		targetUserID = existingUser.ID
	} else {
		randBytes := make([]byte, 8)
		_, _ = rand.Read(randBytes)
		tempPassword := fmt.Sprintf("Dawa24!%s", hex.EncodeToString(randBytes))

		newUser, _, err := h.idSvc.Register(ctx, identity.RegisterInput{
			Email:    email,
			Password: tempPassword,
			NameAr:   name,
			NameEn:   name,
			Phone:    phone,
			Role:     "user",
		})
		if err != nil {
			h.redirectWithNotice(w, r, "/settings/employees", "error", h.safeMessage(err, langOf(r)))
			return
		}
		targetUserID = newUser.ID
	}

	if employeeCode == "" {
		employeeCode = fmt.Sprintf("EMP-%d", targetUserID)
	}

	// 2. Add to organization members
	member := &org.Member{
		OrganizationID: actor.OrganizationID,
		UserID:         targetUserID,
		BranchID:       branchID,
		RoleKey:        roleKey,
		EmployeeCode:   employeeCode,
		JobTitle:       jobTitle,
		BaseSalary:     baseSalary,
		IsActive:       true,
	}

	if err := h.orgSvc.AddMemberDirect(ctx, member); err != nil {
		h.redirectWithNotice(w, r, "/settings/employees", "error", h.safeMessage(err, langOf(r)))
		return
	}

	// 3. If role is org_manager and branch is specified, assign as branch manager
	if roleKey == "org_manager" && branchID != nil {
		_ = h.orgSvc.AssignBranchManager(ctx, actor.OrganizationID, *branchID, &targetUserID)
	}

	h.redirectWithNotice(w, r, "/settings/employees", "success", "تم إنشاء وتعيين الموظف وتطبيق الصلاحيات بنجاح.")
}

// SettingsBranchManagerAssignSubmit assigns a designated employee user as the branch manager.
func (h *UIHandler) SettingsBranchManagerAssignSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/settings/employees", http.StatusSeeOther)
		return
	}

	_ = r.ParseForm()

	branchID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if branchID <= 0 {
		branchID, _ = strconv.ParseInt(r.PostFormValue("branch_id"), 10, 64)
	}
	if branchID <= 0 {
		h.redirectWithNotice(w, r, "/settings/employees", "error", "معرف الفرع غير صالح.")
		return
	}

	var managerUserID *int64
	if mStr := r.PostFormValue("manager_user_id"); mStr != "" && mStr != "0" {
		if mID, err := strconv.ParseInt(mStr, 10, 64); err == nil && mID > 0 {
			managerUserID = &mID
		}
	}

	if err := h.orgSvc.AssignBranchManager(ctx, actor.OrganizationID, branchID, managerUserID); err != nil {
		h.redirectWithNotice(w, r, "/settings/employees", "error", h.safeMessage(err, langOf(r)))
		return
	}

	h.redirectWithNotice(w, r, "/settings/employees", "success", "تم تعيين وتثبيت مدير الفرع بنجاح.")
}

// SettingsEmployeeDeleteSubmit removes an employee member from the organization.
func (h *UIHandler) SettingsEmployeeDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/settings/employees", http.StatusSeeOther)
		return
	}

	userID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || userID <= 0 {
		h.redirectWithNotice(w, r, "/settings/employees", "error", "معرف الموظف غير صالح.")
		return
	}

	if err := h.orgSvc.RemoveMember(ctx, actor.OrganizationID, userID); err != nil {
		h.redirectWithNotice(w, r, "/settings/employees", "error", h.safeMessage(err, langOf(r)))
		return
	}

	h.redirectWithNotice(w, r, "/settings/employees", "success", "تم حذف الموظف من المنشأة بنجاح.")
}

// SettingsDeleteRequestSubmit receives an account deletion request from a user.
func (h *UIHandler) SettingsDeleteRequestSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/settings", http.StatusSeeOther)
		return
	}

	_ = r.ParseForm()
	reason := strings.TrimSpace(r.PostFormValue("reason"))

	var orgID *int64
	if actor.OrganizationID > 0 {
		orgID = &actor.OrganizationID
	}

	if h.idSvc == nil {
		h.redirectWithNotice(w, r, "/settings?tab=security", "error", "الخدمة غير متاحة حالياً.")
		return
	}

	if err := h.idSvc.RequestAccountDeletion(ctx, actor.UserID, orgID, reason); err != nil {
		h.redirectWithNotice(w, r, "/settings?tab=security", "error", h.safeMessage(err, langOf(r)))
		return
	}

	h.redirectWithNotice(w, r, "/settings?tab=security", "success", "تم استلام طلب حذف الحساب بنجاح، وسيتم مراجعته من قبل إدارة المنصة.")
}
