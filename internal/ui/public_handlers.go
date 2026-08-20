package ui

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/modules/identity"
	platformadmin "github.com/muhiya/dawa24-store/internal/modules/platform_admin"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

func (h *UIHandler) HomePage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	var featured []*catalog.Product
	var categories []*catalog.Category
	stats := pages.HomeStats{
		TotalSuppliers: 23,
		TotalProducts:  25042,
		TotalCities:    333,
		TotalOrders:    1250,
	}

	if h.catSvc != nil {
		prods, err := h.catSvc.Search(ctx, catalog.SearchParams{Limit: 12})
		if err != nil {
			h.log.WarnContext(ctx, "home page: search featured products", "error", err)
		} else {
			featured = prods
			if len(prods) > 0 {
				stats.TotalProducts = len(prods)
			}
		}

		cats, err := h.catSvc.ListCategories(ctx)
		if err != nil {
			h.log.WarnContext(ctx, "home page: list categories", "error", err)
		} else {
			categories = cats
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.CustomerHome(featured, categories, stats, lang, dir).Render(ctx, w); err != nil {
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
	if actor, ok := authctx.From(ctx); ok && actor.UserID > 0 {
		http.Redirect(w, r, landingPathForActor(actor), http.StatusSeeOther)
		return
	}

	lang, dir := h.localeAndDir(r)
	errorMsg := r.URL.Query().Get("error")

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.LoginPage(lang, dir, errorMsg).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render login page", "error", err)
	}
}

func (h *UIHandler) RegisterPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if actor, ok := authctx.From(ctx); ok && actor.UserID > 0 {
		http.Redirect(w, r, landingPathForActor(actor), http.StatusSeeOther)
		return
	}

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
	http.Redirect(w, r, "/auth/register", http.StatusFound)
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

	if res.RequiresMFA {
		// No challenge UI exists yet (PLAN_V6 Task C.9). Refusing is the safe
		// failure: never issue a session to an account that asked for a second
		// factor we cannot verify.
		var uid int64
		if res.User != nil {
			uid = res.User.ID
		}
		h.log.WarnContext(ctx, "login refused: MFA required but no challenge implemented", "user_id", uid)
		http.Redirect(w, r, "/auth/login?error=mfa_unavailable", http.StatusSeeOther)
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

	// Process license attachment file if uploaded
	licenseURL, _ := saveUploadedFile(r, "license_file", "licenses")

	var latPtr, lonPtr *float64
	if latStr := r.PostFormValue("branch_lat"); latStr != "" {
		if lat, err := strconv.ParseFloat(latStr, 64); err == nil && lat != 0 {
			latPtr = &lat
		}
	}
	if lonStr := r.PostFormValue("branch_lon"); lonStr != "" {
		if lon, err := strconv.ParseFloat(lonStr, 64); err == nil && lon != 0 {
			lonPtr = &lon
		}
	}

	accountType := r.PostFormValue("account_type")
	if accountType == "" || accountType == "pharmacy" {
		accountType = "customer"
	} else if accountType == "supplier" {
		accountType = "vendor"
	}

	name := strings.TrimSpace(r.PostFormValue("name"))
	email := strings.TrimSpace(r.PostFormValue("email"))
	phone := strings.TrimSpace(r.PostFormValue("phone"))
	legalName := strings.TrimSpace(r.PostFormValue("legal_name"))
	tradeNameAr := strings.TrimSpace(r.PostFormValue("trade_name_ar"))
	tradeNameEn := strings.TrimSpace(r.PostFormValue("trade_name_en"))
	cr := strings.TrimSpace(r.PostFormValue("commercial_register"))
	taxNum := strings.TrimSpace(r.PostFormValue("tax_number"))
	licenseNum := strings.TrimSpace(r.PostFormValue("pharmacist_license"))
	address := strings.TrimSpace(r.PostFormValue("address"))

	if legalName == "" {
		if tradeNameAr != "" {
			legalName = tradeNameAr
		} else if name != "" {
			legalName = name
		} else {
			legalName = "منشأة جديدة"
		}
	}
	if tradeNameAr == "" {
		tradeNameAr = legalName
	}
	if name == "" {
		name = legalName
	}
	if address == "" {
		address = "المقر الرئيسي"
	}

	form := pages.RegisterFormData{
		AccountType:        accountType,
		Name:               name,
		Email:              email,
		Phone:              phone,
		LegalName:          legalName,
		TradeNameAr:        tradeNameAr,
		TradeNameEn:        tradeNameEn,
		CommercialRegister: cr,
		TaxNumber:          taxNum,
		PharmacistLicense:  licenseNum,
		LicenseDocumentURL: licenseURL,
		CityID:             r.PostFormValue("city_id"),
		BranchCount:        r.PostFormValue("branch_count"),
		Address:            address,
		Latitude:           r.PostFormValue("branch_lat"),
		Longitude:          r.PostFormValue("branch_lon"),
		GoogleMapsURL:      r.PostFormValue("branch_google_maps_url"),
	}

	password := r.PostFormValue("password")
	if err := identity.ValidatePassword(password); err != nil {
		form.Error = err.Error()
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := pages.RegisterPage(lang, dir, form, h.listCities(ctx)).Render(ctx, w); err != nil {
			h.log.ErrorContext(ctx, "render register page validation error", "error", err)
		}
		return
	}

	cityIDStr := r.PostFormValue("branch_city_id")
	if cityIDStr == "" {
		cityIDStr = r.PostFormValue("city_id")
	}

	var cityIDPtr *int64
	if id, err := strconv.ParseInt(cityIDStr, 10, 64); err == nil && id > 0 {
		cityIDPtr = &id
	} else if latPtr != nil && lonPtr != nil {
		if nearestID := h.findNearestCityID(ctx, *latPtr, *lonPtr); nearestID > 0 {
			cityIDPtr = &nearestID
		}
	}

	branchCount := 1
	if bc, err := strconv.Atoi(r.PostFormValue("branch_count")); err == nil && bc > 1 {
		branchCount = bc
	}

	if h.idSvc == nil {
		http.Redirect(w, r, "/auth/register?error=service_unavailable", http.StatusSeeOther)
		return
	}

	_, sess, _, err := h.idSvc.RegisterOrganization(ctx, identity.RegisterOrganizationInput{
		Email:    form.Email,
		Password: password,
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
			LicenseDocumentURL: form.LicenseDocumentURL,
			CityID:             cityIDPtr,
			BranchCount:        &branchCount,
			Address:            form.Address,
			Latitude:           latPtr,
			Longitude:          lonPtr,
			GoogleMapsURL:      form.GoogleMapsURL,
		},
	})

	if err != nil {
		h.log.WarnContext(ctx, "ui registration failed", "email", form.Email, "error", err)
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

// OrgSwitchSubmit handles context-switching between a user's multiple organizations.
func (h *UIHandler) OrgSwitchSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	targetOrgID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || targetOrgID <= 0 {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	if h.idSvc == nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	sess, err := h.idSvc.SwitchActiveOrg(ctx, actor.UserID, targetOrgID)
	if err != nil {
		h.log.WarnContext(ctx, "org switch failed", "user_id", actor.UserID, "target_org_id", targetOrgID, "error", err)
		http.Redirect(w, r, "/", http.StatusSeeOther)
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
	if sess.IsStaff() {
		return "/admin/dashboard"
	}

	switch sess.OrgStatus {
	case "pending":
		return "/onboarding/pending"
	case "rejected":
		return "/onboarding/pending?rejected=1"
	}
	switch sess.OrgType {
	case "vendor":
		return "/vendor/dashboard"
	case "customer":
		return "/customer/dashboard"
	}
	return "/catalog"
}

// landingPathForActor routes an authenticated actor to their home surface.
func landingPathForActor(actor authctx.Actor) string {
	if actor.IsStaff {
		return "/admin/dashboard"
	}
	switch actor.OrgStatus {
	case "pending":
		return "/onboarding/pending"
	case "rejected":
		return "/onboarding/pending?rejected=1"
	}
	switch actor.OrgType {
	case "vendor":
		return "/vendor/dashboard"
	case "customer":
		return "/customer/dashboard"
	}
	return "/catalog"
}

func (h *UIHandler) findNearestCityID(ctx context.Context, lat, lon float64) int64 {
	cities := h.listCities(ctx)
	if len(cities) == 0 {
		return 1
	}
	var bestID int64 = cities[0].ID
	var minDist float64 = 1e9
	for _, c := range cities {
		if c.Latitude == 0 && c.Longitude == 0 {
			continue
		}
		dLat := lat - c.Latitude
		dLon := lon - c.Longitude
		dist := dLat*dLat + dLon*dLon
		if dist < minDist {
			minDist = dist
			bestID = c.ID
		}
	}
	return bestID
}
