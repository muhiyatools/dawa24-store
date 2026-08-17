package ui

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/identity"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// SettingsIndex redirects the bare /settings route to its first tab.
func (h *UIHandler) SettingsIndex(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/settings/profile", http.StatusSeeOther)
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

// SettingsProfileSubmit saves name, phone, timezone and language.
func (h *UIHandler) SettingsProfileSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/settings/profile", http.StatusSeeOther)
		return
	}

	if h.idSvc == nil {
		h.redirectWithNotice(w, r, "/settings/profile", "error", "الخدمة غير متاحة حالياً.")
		return
	}

	_, err := h.idSvc.UpdateProfile(ctx, actor.UserID,
		r.PostFormValue("name_ar"), r.PostFormValue("name_en"),
		r.PostFormValue("phone"), r.PostFormValue("timezone"),
		r.PostFormValue("lang"),
	)
	if err != nil {
		h.log.WarnContext(ctx, "profile update failed", "user_id", actor.UserID, "error", err)
		h.redirectWithNotice(w, r, "/settings/profile", "error", h.safeMessage(err, langOf(r)))
		return
	}
	h.redirectWithNotice(w, r, "/settings/profile", "success", "تم حفظ التغييرات بنجاح.")
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
	if h.idSvc != nil {
		sessions, _ = h.idSvc.ListSessions(ctx, actor.UserID)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.SettingsSecurity(lang, dir, sessions).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render settings security", "error", err)
	}
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
