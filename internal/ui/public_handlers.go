package ui

import (
	"context"
	"net/http"
	"strconv"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/modules/identity"
	platformadmin "github.com/muhiya/dawa24-store/internal/modules/platform_admin"
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
	h.renderPolicy(w, r, "privacy", "سياسة الخصوصية")
}

func (h *UIHandler) TermsPage(w http.ResponseWriter, r *http.Request) {
	h.renderPolicy(w, r, "terms", "الشروط والأحكام")
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

	form := pages.RegisterFormData{
		Error: r.URL.Query().Get("error"),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.RegisterPage(lang, dir, form, h.listCities(ctx)).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render register page", "error", err)
	}
}

// listCities loads the Egyptian cities for the registration form's city picker.
func (h *UIHandler) listCities(ctx context.Context) []*platformadmin.City {
	if h.adminSvc == nil {
		return nil
	}
	countries, err := h.adminSvc.ListCountries(ctx)
	if err != nil || len(countries) == 0 {
		return nil
	}
	var countryID int64
	for _, c := range countries {
		if c.Code == "EG" {
			countryID = c.ID
			break
		}
	}
	if countryID == 0 {
		countryID = countries[0].ID
	}
	cities, _ := h.adminSvc.ListCities(ctx, countryID)
	return cities
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

	// An explicit redirect (from a protected page the user was heading to)
	// wins; otherwise route by session — account type, approval state and
	// platform role decide where the user lands.
	if redirectURL == "" {
		redirectURL = landingPathForSession(res.Session)
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
	lang, dir := h.localeAndDir(r)

	form := pages.RegisterFormData{
		AccountType:        r.PostFormValue("account_type"),
		Name:               r.PostFormValue("name"),
		Email:              r.PostFormValue("email"),
		Phone:              r.PostFormValue("phone"),
		LegalName:          r.PostFormValue("legal_name"),
		TradeNameAr:        r.PostFormValue("trade_name_ar"),
		TradeNameEn:        r.PostFormValue("trade_name_en"),
		CommercialRegister: r.PostFormValue("commercial_register"),
		TaxNumber:          r.PostFormValue("tax_number"),
		PharmacistLicense:  r.PostFormValue("pharmacist_license"),
		CityID:             r.PostFormValue("city_id"),
		BranchCount:        r.PostFormValue("branch_count"),
	}

	if h.idSvc == nil {
		http.Redirect(w, r, "/auth/register?error=service_unavailable", http.StatusSeeOther)
		return
	}

	_, sess, _, err := h.idSvc.RegisterOrganization(ctx, identity.RegisterOrganizationInput{
		Email:    form.Email,
		Password: r.PostFormValue("password"),
		NameAr:   form.Name,
		NameEn:   form.Name,
		Phone:    form.Phone,
		Org: identity.RegisterOrgInput{
			Type:               form.AccountType,
			LegalName:          form.LegalName,
			TradeNameAr:        form.TradeNameAr,
			TradeNameEn:        form.TradeNameEn,
			CommercialRegister: form.CommercialRegister,
			TaxNumber:          form.TaxNumber,
			PharmacistLicense:  form.PharmacistLicense,
			CityID:             parseInt64Ptr(form.CityID),
			BranchCount:        parseIntPtr(form.BranchCount),
		},
	})
	if err != nil {
		h.log.WarnContext(ctx, "ui registration failed", "email", form.Email, "error", err)
		// Re-render with the entered values still in the fields — an empty form
		// on error loses the signup.
		form.Error = h.safeMessage(err, lang)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if rerr := pages.RegisterPage(lang, dir, form, h.listCities(ctx)).Render(ctx, w); rerr != nil {
			h.log.ErrorContext(ctx, "render register page after error", "error", rerr)
		}
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

	http.Redirect(w, r, landingPathForSession(sess), http.StatusSeeOther)
}

func parseInt64Ptr(s string) *int64 {
	if s == "" {
		return nil
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return nil
	}
	return &v
}

func parseIntPtr(s string) *int {
	if s == "" {
		return nil
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return nil
	}
	return &v
}

// landingPathForSession routes a fresh session to the right surface: the
// platform admin dashboard for platform staff, the approval gate for a pending
// or rejected organization, and the type's own dashboard otherwise.
func landingPathForSession(sess *identity.Session) string {
	if sess == nil {
		return "/catalog"
	}
	if sess.Role == "super_admin" || sess.Role == "admin" || sess.Role == "developer" {
		return "/admin/dashboard"
	}
	switch sess.OrgStatus {
	case "pending":
		return "/onboarding/pending"
	case "rejected":
		return "/onboarding/pending?rejected=1"
	}
	switch sess.OrgType {
	case "supplier":
		return "/vendor/dashboard"
	case "pharmacy", "chain_pharmacy":
		return "/pharmacy/dashboard"
	}
	return "/catalog"
}
