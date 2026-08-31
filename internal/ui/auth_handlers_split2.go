package ui

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/muhiya/dawa24-store/internal/modules/hr"
	"github.com/muhiya/dawa24-store/internal/modules/identity"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

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
