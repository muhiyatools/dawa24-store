package ui

import (
	"net/http"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
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
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.Onboarding().Render(ctx, w); err != nil {
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
