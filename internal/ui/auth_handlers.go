package ui

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/muhiya/dawa24-store/internal/modules/hr"
	"github.com/muhiya/dawa24-store/internal/modules/identity"
	platformadmin "github.com/muhiya/dawa24-store/internal/modules/platform_admin"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

func (h *UIHandler) LoginPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if actor, ok := authctx.From(ctx); ok && actor.UserID > 0 {
		http.Redirect(w, r, landingPathForActor(actor), http.StatusSeeOther)
		return
	}

	lang, dir := h.localeAndDir(r)
	errorKey := r.URL.Query().Get("error")
	var errorMsg string
	switch errorKey {
	case "concurrent_limit", "session_evicted":
		errorMsg = i18n.T(lang, "auth.login.session_evicted")
	case "invalid_credentials":
		errorMsg = i18n.T(lang, "auth.login.invalid_credentials")
	case "locked":
		errorMsg = i18n.T(lang, "auth.login.account_locked")
	case "mfa_unavailable":
		errorMsg = i18n.T(lang, "auth.login.mfa_unavailable")
	case "auth_service_unavailable":
		errorMsg = i18n.T(lang, "auth.login.service_unavailable")
	default:
		errorMsg = errorKey
	}

	h.renderPage(ctx, w, "render login page", pages.LoginPage(lang, dir, errorMsg))
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

	h.renderPage(ctx, w, "render register page", pages.RegisterPage(lang, dir, form, h.listCities(ctx)))
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
	h.renderPage(ctx, w, "render forgot password page", pages.PasswordReset())
}

func (h *UIHandler) ResetPasswordPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	token := r.URL.Query().Get("token")
	h.renderPage(ctx, w, "render reset password page", pages.PasswordResetConfirm(token))
}

func (h *UIHandler) OnboardingPage(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/auth/register", http.StatusFound)
}

func (h *UIHandler) LoginSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	email := r.PostFormValue("email")
	password := r.PostFormValue("password")
	rememberMe := r.PostFormValue("remember_me")
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
		uid := int64(0)
		uEmail := email
		if res.User != nil {
			uid = res.User.ID
			uEmail = res.User.Email
		}
		p := &pendingMFAPayload{
			UserID:      uid,
			Email:       uEmail,
			OrgID:       0,
			RememberMe:  rememberMe == "1" || rememberMe == "true" || rememberMe == "",
			RedirectURL: redirectURL,
			ExpiresAt:   time.Now().Add(5 * time.Minute).Unix(),
		}
		signedToken := h.signPendingMFAToken(p)
		http.SetCookie(w, &http.Cookie{
			Name:     "dawa24_mfa_pending",
			Value:    signedToken,
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
			MaxAge:   300, // 5 minutes
		})
		h.log.InfoContext(ctx, "mfa challenge required for login", "user_id", uid)
		http.Redirect(w, r, "/auth/mfa-verify?redirect="+redirectURL, http.StatusSeeOther)
		return
	}

	maxAge := 86400 * 30 // Default remember-me: 30 days
	if rememberMe == "0" || rememberMe == "false" {
		maxAge = 86400 // 1 day session
	}

	if res.Session != nil {
		http.SetCookie(w, &http.Cookie{
			Name:     "dawa24_session",
			Value:    res.Session.Token,
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
			MaxAge:   maxAge,
		})

		if res.Session.ActiveOrgID > 0 {
			orgID := res.Session.ActiveOrgID
			go h.EnsureOrgAIGatewayProvisioned(context.Background(), orgID)
		}
	}

	// An explicit redirect (from a protected page the user was heading to)
	// wins; otherwise route by session — account type, approval state and
	// platform role decide where the user lands.
	if redirectURL == "" {
		redirectURL = landingPathForSession(res.Session)
	}

	http.Redirect(w, r, redirectURL, http.StatusSeeOther)
}

// MFAVerifyPage renders the 6-digit TOTP challenge screen during login.
func (h *UIHandler) MFAVerifyPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	redirectURL := r.URL.Query().Get("redirect")
	errorKey := r.URL.Query().Get("error")

	cookie, err := r.Cookie("dawa24_mfa_pending")
	if err != nil || cookie == nil || cookie.Value == "" {
		http.Redirect(w, r, "/auth/login?error=mfa_session_expired", http.StatusSeeOther)
		return
	}

	payload, err := h.parsePendingMFAToken(cookie.Value)
	if err != nil || payload == nil || time.Now().Unix() > payload.ExpiresAt {
		http.Redirect(w, r, "/auth/login?error=mfa_session_expired", http.StatusSeeOther)
		return
	}

	var errorMsg string
	if errorKey == "invalid_code" {
		errorMsg = i18n.T(lang, "mfa.code_invalid_or_expired")
	} else if errorKey != "" {
		errorMsg = errorKey
	}

	h.renderPage(ctx, w, "render mfa verify page", pages.MFAVerifyPage(lang, dir, payload.Email, errorMsg, redirectURL))
}

// MFAVerifySubmit processes the 6-digit TOTP or backup code and issues the full session cookie.
func (h *UIHandler) MFAVerifySubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	cookie, err := r.Cookie("dawa24_mfa_pending")
	if err != nil || cookie == nil || cookie.Value == "" {
		http.Redirect(w, r, "/auth/login?error=mfa_session_expired", http.StatusSeeOther)
		return
	}

	payload, err := h.parsePendingMFAToken(cookie.Value)
	if err != nil || payload == nil || time.Now().Unix() > payload.ExpiresAt {
		http.Redirect(w, r, "/auth/login?error=mfa_session_expired", http.StatusSeeOther)
		return
	}

	code := strings.TrimSpace(r.PostFormValue("code"))
	recoveryCode := strings.TrimSpace(r.PostFormValue("recovery_code"))
	targetCode := code
	if targetCode == "" {
		targetCode = recoveryCode
	}

	redirectURL := r.PostFormValue("redirect")
	if redirectURL == "" {
		redirectURL = payload.RedirectURL
	}

	if targetCode == "" {
		http.Redirect(w, r, "/auth/mfa-verify?error=invalid_code&redirect="+redirectURL, http.StatusSeeOther)
		return
	}

	if h.idSvc == nil {
		http.Redirect(w, r, "/auth/login?error=auth_service_unavailable", http.StatusSeeOther)
		return
	}

	valid, err := h.idSvc.VerifyMFA(ctx, payload.UserID, targetCode)
	if err != nil || !valid {
		h.log.WarnContext(ctx, "mfa verification failed", "user_id", payload.UserID, "error", err)
		http.Redirect(w, r, "/auth/mfa-verify?error=invalid_code&redirect="+redirectURL, http.StatusSeeOther)
		return
	}

	// MFA verified! Clear pending cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "dawa24_mfa_pending",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})

	sess, err := h.idSvc.CompleteMFALogin(ctx, payload.UserID, payload.OrgID, r.RemoteAddr, r.UserAgent())
	if err != nil {
		h.log.ErrorContext(ctx, "complete mfa login error", "user_id", payload.UserID, "error", err)
		http.Redirect(w, r, "/auth/login?error=auth_service_unavailable", http.StatusSeeOther)
		return
	}

	maxAge := 86400 * 30
	if !payload.RememberMe {
		maxAge = 86400
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "dawa24_session",
		Value:    sess.Token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   maxAge,
	})

	if sess.ActiveOrgID > 0 {
		go h.EnsureOrgAIGatewayProvisioned(context.Background(), sess.ActiveOrgID)
	}

	if redirectURL == "" {
		redirectURL = landingPathForSession(sess)
	}

	http.Redirect(w, r, redirectURL, http.StatusSeeOther)
}

type pendingMFAPayload struct {
	UserID      int64  `json:"uid"`
	Email       string `json:"em"`
	OrgID       int64  `json:"oid"`
	RememberMe  bool   `json:"rm"`
	RedirectURL string `json:"rd,omitempty"`
	ExpiresAt   int64  `json:"exp"`
}

func (h *UIHandler) signPendingMFAToken(p *pendingMFAPayload) string {
	b, _ := json.Marshal(p)
	payloadB64 := base64.RawURLEncoding.EncodeToString(b)
	sigKey := []byte("dawa24_mfa_intermediate_key_secret_2026")
	mac := hmac.New(sha256.New, sigKey)
	mac.Write([]byte(payloadB64))
	sig := hex.EncodeToString(mac.Sum(nil))
	return payloadB64 + "." + sig
}

func (h *UIHandler) parsePendingMFAToken(tokenStr string) (*pendingMFAPayload, error) {
	parts := strings.Split(tokenStr, ".")
	if len(parts) != 2 {
		return nil, identity.ErrInvalidCredentials
	}
	payloadB64, sig := parts[0], parts[1]
	sigKey := []byte("dawa24_mfa_intermediate_key_secret_2026")
	mac := hmac.New(sha256.New, sigKey)
	mac.Write([]byte(payloadB64))
	expectedSig := hex.EncodeToString(mac.Sum(nil))
	if hmac.Equal([]byte(sig), []byte(expectedSig)) == false {
		return nil, identity.ErrInvalidCredentials
	}

	b, err := base64.RawURLEncoding.DecodeString(payloadB64)
	if err != nil {
		return nil, err
	}
	var p pendingMFAPayload
	if err := json.Unmarshal(b, &p); err != nil {
		return nil, err
	}
	return &p, nil
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
	if accountType == "job_seeker" || accountType == "seeker" {
		accountType = "job_seeker"
	} else if accountType == "supplier" || accountType == "vendor" {
		accountType = "vendor"
	} else {
		accountType = "customer"
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
			legalName = i18n.T(langOf(r), "auth.register.default_legal_name")
		}
	}
	if tradeNameAr == "" {
		tradeNameAr = legalName
	}
	if name == "" {
		name = legalName
	}
	if address == "" {
		address = i18n.T(langOf(r), "auth.register.default_address")
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
		Specialisation:     strings.TrimSpace(r.PostFormValue("specialisation")),
		YearsExperience:    strings.TrimSpace(r.PostFormValue("years_experience")),
		Bio:                strings.TrimSpace(r.PostFormValue("bio")),
		ExpectedSalary:     strings.TrimSpace(r.PostFormValue("expected_salary")),
	}

	password := r.PostFormValue("password")
	if err := identity.ValidatePassword(password); err != nil {
		form.Error = err.Error()
		h.renderPage(ctx, w, "render register page validation error", pages.RegisterPage(lang, dir, form, h.listCities(ctx)))
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

	// 1. If Job Seeker, create direct user + job seeker profile
	if form.AccountType == "job_seeker" {
		cvURL, _ := saveUploadedFile(r, "cv_file", "cvs")
		user, sess, err := h.idSvc.Register(ctx, identity.RegisterInput{
			Email:    form.Email,
			Password: password,
			NameAr:   form.Name,
			NameEn:   form.Name,
			Role:     identity.RoleJobSeeker,
			Phone:    form.Phone,
		})
		if err != nil {
			h.log.WarnContext(ctx, "ui job seeker registration failed", "email", form.Email, "error", err)
			form.Error = h.safeMessage(err, lang)
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			if rerr := pages.RegisterPage(lang, dir, form, h.listCities(ctx)).Render(ctx, w); rerr != nil {
				h.log.ErrorContext(ctx, "render register page after error", "error", rerr)
			}
			return
		}

		if h.hrSvc != nil && user != nil {
			exp, _ := strconv.Atoi(form.YearsExperience)
			sal, _ := money.Parse(form.ExpectedSalary)
			spec := form.Specialisation
			if spec == "" {
				spec = "pharmacist"
			}
			_ = h.hrSvc.SaveJobSeekerProfile(ctx, &hr.JobSeekerProfile{
				UserID:          user.ID,
				Specialisation:  spec,
				YearsExperience: exp,
				IsOpenToWork:    true,
				ExpectedSalary:  sal,
				PreferredCityID: cityIDPtr,
				Bio:             form.Bio,
			})
			_ = cvURL
		}

		if user != nil {
			go h.notifyAccountRegistered(context.Background(), user.ID, nil)
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
		http.Redirect(w, r, "/jobs", http.StatusSeeOther)
		return
	}

	// 2. Organization Registration (Customer / Pharmacy or Vendor / Supplier)
	_, sess, regResult, err := h.idSvc.RegisterOrganization(ctx, identity.RegisterOrganizationInput{
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

	if regResult != nil && regResult.OrganizationID > 0 {
		h.ensureCompanyRoles(database.AsSystem(ctx), regResult.OrganizationID, form.AccountType)
		if sess != nil {
			go h.notifyAccountRegistered(context.Background(), sess.UserID, &regResult.OrganizationID)
		}
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

// landingPathForSession routes an authenticated session to their home surface.
func landingPathForSession(sess *identity.Session) string {
	if sess == nil {
		return "/catalog"
	}
	if sess.IsStaff() {
		return "/admin/dashboard"
	}

	switch sess.OrgStatus {
	case "pending", "under_review":
		return "/onboarding/pending"
	case "rejected":
		return "/onboarding/pending?rejected=1"
	case "suspended":
		return "/onboarding/pending?state=suspended"
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
	case "pending", "under_review":
		return "/onboarding/pending"
	case "rejected":
		return "/onboarding/pending?rejected=1"
	case "suspended":
		return "/onboarding/pending?state=suspended"
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
