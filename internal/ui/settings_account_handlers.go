package ui

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/identity"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
)

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

	_ = r.ParseMultipartForm(10 << 20)

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

	// Handle avatar file upload or removal
	if r.PostFormValue("remove_avatar") == "1" {
		_, _ = h.idSvc.UpdateAvatar(ctx, actor.UserID, "")
	} else if uploadedURL, err := saveUploadedFile(r, "avatar_file", "avatars"); err == nil && uploadedURL != "" {
		_, _ = h.idSvc.UpdateAvatar(ctx, actor.UserID, uploadedURL)
	} else if uploadedURL, err := saveUploadedFile(r, "avatar", "avatars"); err == nil && uploadedURL != "" {
		_, _ = h.idSvc.UpdateAvatar(ctx, actor.UserID, uploadedURL)
	} else if avatarURL := r.PostFormValue("avatar_url"); avatarURL != "" {
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
