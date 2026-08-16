package ui

import (
	"net/http"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/modules/identity"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

func (h *UIHandler) HomePage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	var featured []*catalog.Product
	if h.catSvc != nil {
		prods, err := h.catSvc.Search(ctx, catalog.SearchParams{Limit: 8})
		if err == nil {
			featured = prods
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.CustomerHome(featured, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render home page", "error", err)
	}
}

func (h *UIHandler) PrivacyPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.PrivacyPolicy().Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render privacy page", "error", err)
	}
}

func (h *UIHandler) TermsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.TermsOfService().Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render terms page", "error", err)
	}
}

func (h *UIHandler) LoginPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	errorMsg := r.URL.Query().Get("error")

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.LoginPage(lang, dir, errorMsg).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render login page", "error", err)
	}
}

func (h *UIHandler) RegisterPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	errorMsg := r.URL.Query().Get("error")

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.RegisterPage(lang, dir, errorMsg).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render register page", "error", err)
	}
}

func (h *UIHandler) ForgotPasswordPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.PasswordReset().Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render forgot password page", "error", err)
	}
}

func (h *UIHandler) ResetPasswordPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	token := r.URL.Query().Get("token")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.PasswordResetConfirm(token).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render reset password page", "error", err)
	}
}

func (h *UIHandler) OnboardingPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.Onboarding().Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render onboarding page", "error", err)
	}
}

func (h *UIHandler) LoginSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	email := r.PostFormValue("email")
	password := r.PostFormValue("password")
	redirectURL := r.URL.Query().Get("redirect")
	if redirectURL == "" {
		redirectURL = "/catalog"
	}

	if h.idSvc == nil {
		http.Redirect(w, r, "/auth/login?error=auth_service_unavailable", http.StatusSeeOther)
		return
	}

	res, err := h.idSvc.Login(ctx, identity.LoginInput{
		Email:     email,
		Password:  password,
		IP:        r.RemoteAddr,
		UserAgent: r.UserAgent(),
	})
	if err != nil {
		h.log.WarnContext(ctx, "ui login failed", "email", email, "error", err)
		http.Redirect(w, r, "/auth/login?error=invalid_credentials", http.StatusSeeOther)
		return
	}

	if res.Session != nil {
		http.SetCookie(w, &http.Cookie{
			Name:     "dawa24_session",
			Value:    res.Session.Token,
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
			MaxAge:   86400 * 30,
		})
	}

	http.Redirect(w, r, redirectURL, http.StatusSeeOther)
}

func (h *UIHandler) LogoutSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	cookie, err := r.Cookie("dawa24_session")
	if err == nil && cookie.Value != "" && h.idSvc != nil {
		_ = h.idSvc.Logout(ctx, cookie.Value)
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "dawa24_session",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (h *UIHandler) RegisterSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	email := r.PostFormValue("email")
	password := r.PostFormValue("password")
	name := r.PostFormValue("name")
	phone := r.PostFormValue("phone")

	if h.idSvc == nil {
		http.Redirect(w, r, "/auth/register?error=service_unavailable", http.StatusSeeOther)
		return
	}

	_, sess, err := h.idSvc.Register(ctx, identity.RegisterInput{
		Email:    email,
		Password: password,
		NameAr:   name,
		NameEn:   name,
		Role:     "customer",
		Phone:    phone,
	})
	if err != nil {
		h.log.WarnContext(ctx, "ui registration failed", "email", email, "error", err)
		http.Redirect(w, r, "/auth/register?error=registration_failed", http.StatusSeeOther)
		return
	}

	if sess != nil {
		http.SetCookie(w, &http.Cookie{
			Name:     "dawa24_session",
			Value:    sess.Token,
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
			MaxAge:   86400 * 30,
		})
	}

	http.Redirect(w, r, "/catalog", http.StatusSeeOther)
}

