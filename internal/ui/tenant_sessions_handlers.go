package ui

import (
	"context"
	"net/http"
	"strings"

	"github.com/muhiya/dawa24-store/internal/modules/identity"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// TenantSessionsPage renders the dedicated Connected Devices & Active Sessions page
// for Pharmacy customers and Vendors.
func (h *UIHandler) TenantSessionsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.UserID == 0 {
		http.Redirect(w, r, "/auth/login?redirect="+r.URL.RequestURI(), http.StatusSeeOther)
		return
	}

	lang, dir := h.localeAndDir(r)
	isVendor := actor.IsVendor()

	// 1. Resolve limits
	maxSessions := 3
	maxDevices := 3
	planName := "الباقة الأساسية"
	if h.idSvc != nil {
		if sMax, dMax, pName, err := h.idSvc.GetOrgPlanLimits(ctx, actor.OrganizationID); err == nil {
			if sMax > 0 {
				maxSessions = sMax
			}
			if dMax > 0 {
				maxDevices = dMax
			}
			if pName != "" {
				planName = pName
			}
		}
	}

	// 2. Read live sessions
	var currentToken string
	if cookie, err := r.Cookie("dawa24_session"); err == nil && cookie != nil {
		currentToken = cookie.Value
	}

	allSessions, _ := h.fetchLiveSessions(ctx, actor)

	viewData := pages.TenantSessionsViewData{
		Sessions:         allSessions,
		CurrentToken:     currentToken,
		MaxLoginSessions: maxSessions,
		MaxDevices:       maxDevices,
		PlanName:         planName,
		IsVendor:         isVendor,
		NoticeType:       r.URL.Query().Get("notice"),
		NoticeMessage:    r.URL.Query().Get("msg"),
	}

	h.renderPage(ctx, w, "render tenant sessions page", pages.TenantSessionsPage(lang, dir, viewData))
}

// TenantSessionRevokeSubmit terminates a specific active session.
func (h *UIHandler) TenantSessionRevokeSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.UserID == 0 {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	token := r.PostFormValue("token")
	dest := "/customer/sessions"
	if actor.IsVendor() {
		dest = "/vendor/sessions"
	}

	if token != "" && h.idSvc != nil {
		if actor.OrganizationID > 0 {
			_ = h.idSvc.RevokeOrgSession(ctx, actor.OrganizationID, token)
		} else {
			_ = h.idSvc.RevokeSession(ctx, token, actor.UserID)
		}
	}

	h.redirectWithNotice(w, r, dest, "success", "تم إنهاء جلسة الجهاز المحدد بنجاح.")
}

// TenantSessionRevokeAllSubmit terminates all sessions except the caller's current session.
func (h *UIHandler) TenantSessionRevokeAllSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.UserID == 0 {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	currentToken := r.PostFormValue("current_token")
	if currentToken == "" {
		if cookie, err := r.Cookie("dawa24_session"); err == nil && cookie != nil {
			currentToken = cookie.Value
		}
	}

	dest := "/customer/sessions"
	if actor.IsVendor() {
		dest = "/vendor/sessions"
	}

	if h.idSvc != nil {
		if actor.OrganizationID > 0 {
			_ = h.idSvc.RevokeAllOtherOrgSessions(ctx, actor.OrganizationID, currentToken)
		} else {
			_ = h.idSvc.RevokeAllOtherUserSessions(ctx, actor.UserID, currentToken)
		}
	}

	h.redirectWithNotice(w, r, dest, "success", "تم إنهاء كافة الجلسات الأخرى بنجاح.")
}

// TenantPasswordChangeSubmit updates password from the dedicated sessions & security page.
func (h *UIHandler) TenantPasswordChangeSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.UserID == 0 {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	dest := "/customer/sessions"
	if actor.IsVendor() {
		dest = "/vendor/sessions"
	}

	_ = r.ParseForm()
	curr := r.PostFormValue("current_password")
	newPass := r.PostFormValue("new_password")
	confirmPass := r.PostFormValue("confirm_password")

	if strings.TrimSpace(newPass) == "" || newPass != confirmPass {
		h.redirectWithNotice(w, r, dest, "error", "كلمة المرور الجديدة وتأكيدها غير متطابقين.")
		return
	}

	if len(newPass) < 8 {
		h.redirectWithNotice(w, r, dest, "error", "كلمة المرور الجديدة يجب ألا تقل عن 8 أحرف.")
		return
	}

	if h.idSvc == nil {
		h.redirectWithNotice(w, r, dest, "error", "خدمة تحديث الحساب غير متاحة حالياً.")
		return
	}

	if err := h.idSvc.ChangePassword(ctx, actor.UserID, curr, newPass); err != nil {
		h.redirectWithNotice(w, r, dest, "error", h.safeMessage(err, langOf(r)))
		return
	}

	h.redirectWithNotice(w, r, dest, "success", "تم تحديث كلمة المرور وتأمين الحساب بنجاح.")
}

// Helper to fetch live sessions for the actor/org
func (h *UIHandler) fetchLiveSessions(ctx context.Context, actor authctx.Actor) ([]*identity.Session, error) {
	if h.idSvc == nil {
		return nil, nil
	}
	if actor.OrganizationID > 0 {
		return h.idSvc.ListOrgSessions(ctx, actor.OrganizationID)
	}
	return h.idSvc.ListSessions(ctx, actor.UserID)
}
