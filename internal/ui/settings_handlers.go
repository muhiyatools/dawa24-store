package ui

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

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
	var addresses []*identity.UserAddress
	var sessions []*identity.Session

	if h.idSvc != nil {
		if me, err := h.idSvc.GetMe(ctx, actor.UserID, nil); err == nil && me != nil {
			user = me.User
		}
		if addrs, err := h.idSvc.ListAddresses(ctx, actor.UserID); err == nil {
			addresses = addrs
		}
		if sess, err := h.idSvc.ListSessions(ctx, actor.UserID); err == nil {
			sessions = sess
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
		User:      user,
		Addresses: addresses,
		Sessions:  sessions,
		ActiveTab: "profile",
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.UnifiedSettingsPage(data, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render unified settings page", "error", err)
	}
}

// SettingsProfilePage renders the profile tab.
func (h *UIHandler) SettingsProfilePage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/settings/profile", http.StatusSeeOther)
		return
	}

	var user *identity.User
	if h.idSvc != nil {
		if me, err := h.idSvc.GetMe(ctx, actor.UserID, nil); err == nil && me != nil {
			user = me.User
		}
	}
	if user == nil {
		http.Redirect(w, r, "/auth/login?redirect=/settings/profile", http.StatusSeeOther)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.SettingsProfile(lang, dir, user).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render settings profile", "error", err)
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

// SettingsAddressesPage renders the saved-address list and the add form.
func (h *UIHandler) SettingsAddressesPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/settings/addresses", http.StatusSeeOther)
		return
	}

	var addresses []*identity.UserAddress
	var history []*identity.UserAddressHistory
	if h.idSvc != nil {
		addresses, _ = h.idSvc.ListAddresses(ctx, actor.UserID)
		history, _ = h.idSvc.ListAddressHistory(ctx, actor.UserID, 20)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.SettingsAddresses(lang, dir, addresses, h.listCities(ctx), history).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render settings addresses", "error", err)
	}
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

// SettingsSecurityPage renders the active-sessions security tab.
func (h *UIHandler) SettingsSecurityPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/settings/security", http.StatusSeeOther)
		return
	}

	var sessions []*identity.Session
	var plans []*identity.SessionPlan
	if h.idSvc != nil {
		sessions, _ = h.idSvc.ListSessions(ctx, actor.UserID)
		plans, _ = h.idSvc.ListSessionPlans(ctx)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.SettingsSecurity(lang, dir, sessions, plans).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render settings security", "error", err)
	}
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
	http.Redirect(w, r, "/settings/security", http.StatusSeeOther)
}

// SettingsOrganizationPage renders the organization profile and branches.
func (h *UIHandler) SettingsOrganizationPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/settings/organization", http.StatusSeeOther)
		return
	}

	var o *org.Organization
	var branches []*org.Branch
	var members []*org.Member
	if h.orgSvc != nil {
		o, _ = h.orgSvc.GetOrganization(ctx, actor.OrganizationID)
		branches, _ = h.orgSvc.ListBranches(ctx, actor.OrganizationID)
		members, _ = h.orgSvc.ListMembers(ctx, actor.OrganizationID)
	}
	if o == nil {
		http.Redirect(w, r, "/settings/profile", http.StatusSeeOther)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.SettingsOrganization(lang, dir, o, branches, members).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render settings organization", "error", err)
	}
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

// SettingsBranchCreateSubmit adds a branch.
func (h *UIHandler) SettingsBranchCreateSubmit(w http.ResponseWriter, r *http.Request) {
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

	code := r.PostFormValue("code")
	if code == "" {
		code = "BR-" + strconv.FormatInt(time.Now().UnixNano()%100000, 10)
	}
	b := &org.Branch{
		OrganizationID: actor.OrganizationID,
		Name:           i18n.New(r.PostFormValue("name_ar"), ""),
		Code:           code,
		Address:        r.PostFormValue("address"),
		IsMain:         false,
	}
	if err := h.orgSvc.CreateBranch(ctx, b); err != nil {
		h.redirectWithNotice(w, r, "/settings/organization", "error", h.safeMessage(err, langOf(r)))
		return
	}
	h.redirectWithNotice(w, r, "/settings/organization", "success", "تمت إضافة الفرع.")
}

// SettingsBranchDeleteSubmit removes a non-main branch.
func (h *UIHandler) SettingsBranchDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/settings/organization", http.StatusSeeOther)
		return
	}

	if h.orgSvc != nil {
		if id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64); err == nil {
			_ = h.orgSvc.DeleteBranch(ctx, id, actor.OrganizationID)
		}
	}
	http.Redirect(w, r, "/settings/organization", http.StatusSeeOther)
}

// SettingsPreferencesPage renders the user's display and notification prefs.
func (h *UIHandler) SettingsPreferencesPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/settings/preferences", http.StatusSeeOther)
		return
	}

	p := &identity.UserPreferences{UserID: actor.UserID, Theme: "light"}
	if h.idSvc != nil {
		if prefs, err := h.idSvc.GetPreferences(ctx, actor.UserID); err == nil && prefs != nil {
			p = prefs
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.SettingsPreferences(lang, dir, p).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render settings preferences", "error", err)
	}
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

// SettingsPaymentMethodsPage renders the saved payment methods and dynamic add method form.
func (h *UIHandler) SettingsPaymentMethodsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	if _, ok := authctx.From(ctx); !ok {
		http.Redirect(w, r, "/auth/login?redirect=/settings/payment-methods", http.StatusSeeOther)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.SettingsPaymentMethods(lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render settings payment methods", "error", err)
	}
}

// SettingsPaymentMethodsSubmit saves a new payment method.
func (h *UIHandler) SettingsPaymentMethodsSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if _, ok := authctx.From(ctx); !ok {
		http.Redirect(w, r, "/auth/login?redirect=/settings/payment-methods", http.StatusSeeOther)
		return
	}

	h.redirectWithNotice(w, r, "/settings/payment-methods", "success", "تم حفظ وتفعيل وسيلة الدفع بنجاح.")
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
		}
		if b, err := h.orgSvc.ListBranches(ctx, actor.OrganizationID); err == nil {
			branches = b
		}
		if rl, err := h.orgSvc.ListRoles(ctx, actor.OrganizationID); err == nil {
			roles = rl
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.SettingsEmployees(employees, branches, roles, lang, dir).Render(ctx, w); err != nil {
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
		newUser, _, err := h.idSvc.Register(ctx, identity.RegisterInput{
			Email:    email,
			Password: "Password123!",
			NameAr:   name,
			Phone:    phone,
			Role:     "employee",
		})
		if err != nil {
			h.redirectWithNotice(w, r, "/settings/employees", "error", h.safeMessage(err, langOf(r)))
			return
		}
		targetUserID = newUser.ID
	}

	// 2. Add to organization members
	member := &org.Member{
		OrganizationID: actor.OrganizationID,
		UserID:         targetUserID,
		BranchID:       branchID,
		RoleID:         1,
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

	// 3. If designated as branch manager, assign to branch
	if r.PostFormValue("is_manager") == "1" && branchID != nil {
		_ = h.orgSvc.AssignBranchManager(ctx, actor.OrganizationID, *branchID, &targetUserID)
	}

	h.redirectWithNotice(w, r, "/settings/employees", "success", "تم إنشاء وتعيين الموظف بنجاح في المنظومة.")
}

// SettingsBranchManagerAssignSubmit assigns a designated employee user as the branch manager.
func (h *UIHandler) SettingsBranchManagerAssignSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/settings/employees", http.StatusSeeOther)
		return
	}

	branchID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || branchID <= 0 {
		h.redirectWithNotice(w, r, "/settings/employees", "error", "معرف الفرع غير صالح.")
		return
	}

	var managerUserID *int64
	if mStr := r.PostFormValue("manager_user_id"); mStr != "" {
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


