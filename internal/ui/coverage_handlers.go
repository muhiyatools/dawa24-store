package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/org"
	platformadmin "github.com/muhiya/dawa24-store/internal/modules/platform_admin"
	"github.com/muhiya/dawa24-store/internal/modules/workflow"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// VendorCoveragePage renders the weekly geographic coverage grid, modal forms, and distance delivery tiers.
func (h *UIHandler) VendorCoveragePage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/coverage", http.StatusSeeOther)
		return
	}

	if actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/onboarding/pending", http.StatusSeeOther)
		return
	}

	data := pages.VendorCoverageData{
		NoticeType:    r.URL.Query().Get("notice"),
		NoticeMessage: r.URL.Query().Get("msg"),
	}

	if h.wfSvc != nil {
		coverages, err := h.wfSvc.ListCoverageForOrganization(ctx, actor.OrganizationID)
		if err != nil {
			h.log.ErrorContext(ctx, "list weekly coverage", "error", err, "org", actor.OrganizationID)
			data.CoverageUnavailable = true
		} else {
			data.Coverages = coverages
		}
	}

	if h.orgSvc != nil {
		branches, err := h.orgSvc.ListBranches(ctx, actor.OrganizationID)
		if err != nil {
			h.log.WarnContext(ctx, "list vendor branches for coverage", "error", err, "org", actor.OrganizationID)
		} else {
			data.Branches = branches
		}

		bands, err := h.orgSvc.GetDeliveryBands(ctx, actor.OrganizationID)
		if err != nil {
			h.log.WarnContext(ctx, "list delivery bands", "error", err, "org", actor.OrganizationID)
		} else {
			data.Bands = bands
		}
	}

	if h.adminSvc != nil {
		govs, err := h.adminSvc.ListGovernorates(ctx, 1)
		if err != nil {
			h.log.WarnContext(ctx, "list governorates for coverage", "error", err)
		} else {
			data.Governorates = govs
		}

		cities, err := h.adminSvc.ListCities(ctx, 1)
		if err != nil {
			h.log.WarnContext(ctx, "list cities for coverage", "error", err)
		} else {
			data.Cities = cities
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.VendorCoveragePage(data, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render vendor coverage", "error", err)
		h.renderError(w, r, err)
	}
}

// VendorCoverageCreateSubmit processes creation of weekly coverage for multiple days and multiple cities.
func (h *UIHandler) VendorCoverageCreateSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/coverage", http.StatusSeeOther)
		return
	}

	if err := r.ParseForm(); err != nil {
		h.redirectWithNotice(w, r, "/vendor/coverage", "error", "تعذر قراءة بيانات النموذج.")
		return
	}

	branchID, err := strconv.ParseInt(r.PostFormValue("branch_id"), 10, 64)
	if err != nil || branchID <= 0 {
		// Fallback to first branch of organization
		if h.orgSvc != nil {
			branches, bErr := h.orgSvc.ListBranches(ctx, actor.OrganizationID)
			if bErr == nil && len(branches) > 0 {
				branchID = branches[0].ID
			}
		}
	}

	if branchID <= 0 {
		h.redirectWithNotice(w, r, "/vendor/coverage", "error", "يجب اختيار أو إنشاء فرع تابع لمنشأتكم أولاً.")
		return
	}

	var targetBranch *org.Branch
	if h.orgSvc != nil {
		branch, err := h.orgSvc.GetBranch(ctx, branchID)
		if err != nil || branch.OrganizationID != actor.OrganizationID {
			h.log.WarnContext(ctx, "cross-tenant branch coverage creation attempt",
				"actor_org", actor.OrganizationID, "target_branch", branchID)
			h.redirectWithNotice(w, r, "/vendor/coverage", "error", "الفرع المحدد لا ينتمي إلى منشأتكم.")
			return
		}
		targetBranch = branch
	}

	// 1. Parse Days of Week (supports multi-select or 'all')
	daysForm := r.PostForm["days_of_week"]
	if len(daysForm) == 0 {
		dayStr := strings.TrimSpace(r.PostFormValue("day_of_week"))
		if dayStr != "" {
			daysForm = []string{dayStr}
		}
	}

	applyAllDays := r.PostFormValue("apply_to_all_days") == "true" || r.PostFormValue("apply_to_all_days") == "on" || r.PostFormValue("apply_to_all_days") == "1"
	for _, d := range daysForm {
		if d == "all" || d == "-1" {
			applyAllDays = true
			break
		}
	}

	var daysToCreate []int
	if applyAllDays {
		daysToCreate = []int{0, 1, 2, 3, 4, 5, 6}
	} else {
		for _, d := range daysForm {
			dayNum, err := strconv.Atoi(strings.TrimSpace(d))
			if err == nil && dayNum >= 0 && dayNum <= 6 {
				// avoid duplicates
				found := false
				for _, existing := range daysToCreate {
					if existing == dayNum {
						found = true
						break
					}
				}
				if !found {
					daysToCreate = append(daysToCreate, dayNum)
				}
			}
		}
	}

	if len(daysToCreate) == 0 {
		h.redirectWithNotice(w, r, "/vendor/coverage", "error", "يرجى تحديد يوم واحد على الأقل من أيام الأسبوع لتطبيق التغطية.")
		return
	}

	// 2. Parse Governorate & Cities
	var govID *int64
	if gID, err := strconv.ParseInt(r.PostFormValue("governorate_id"), 10, 64); err == nil && gID > 0 {
		govID = &gID
	}

	var targetCities []*platformadmin.City
	allCitiesInGov := r.PostFormValue("all_cities_in_gov") == "true" || r.PostFormValue("all_cities_in_gov") == "on" || r.PostFormValue("all_cities_in_gov") == "1"

	if allCitiesInGov && govID != nil && h.adminSvc != nil {
		citiesInGov, err := h.adminSvc.ListCitiesByGovernorate(ctx, *govID)
		if err == nil && len(citiesInGov) > 0 {
			targetCities = citiesInGov
		}
	}

	if len(targetCities) == 0 {
		cityIDsForm := r.PostForm["city_ids"]
		if len(cityIDsForm) == 0 {
			cIDStr := strings.TrimSpace(r.PostFormValue("city_id"))
			if cIDStr != "" {
				parts := strings.Split(cIDStr, ",")
				for _, p := range parts {
					if strings.TrimSpace(p) != "" {
						cityIDsForm = append(cityIDsForm, strings.TrimSpace(p))
					}
				}
			}
		}

		if len(cityIDsForm) > 0 && h.adminSvc != nil {
			for _, cidStr := range cityIDsForm {
				if cID, err := strconv.ParseInt(strings.TrimSpace(cidStr), 10, 64); err == nil && cID > 0 {
					city, err := h.adminSvc.GetCity(ctx, cID)
					if err == nil && city != nil {
						targetCities = append(targetCities, city)
						if govID == nil && city.GovernorateID != nil {
							govID = city.GovernorateID
						}
					}
				}
			}
		}
	}

	distanceMeters, _ := strconv.Atoi(r.PostFormValue("distance_meters"))
	if distanceMeters <= 0 {
		distanceMeters = 5000 // default 5km from city center
	}

	isActive := r.PostFormValue("is_active") == "true" || r.PostFormValue("is_active") == "on" || r.PostFormValue("is_active") == "1" || r.PostFormValue("is_active") == ""
	fromTime := workflow.TimeOfDay(r.PostFormValue("coverage_from"))
	toTime := workflow.TimeOfDay(r.PostFormValue("coverage_to"))

	var newCoverages []*workflow.WeeklyCoverage

	if len(targetCities) > 0 {
		for _, city := range targetCities {
			cityID := city.ID
			cityName := city.Name.Get("ar")
			lat := city.Latitude
			lon := city.Longitude

			// Read city-specific timing and radius override if provided:
			cityFromStr := strings.TrimSpace(r.PostFormValue(fmt.Sprintf("coverage_from_%d", city.ID)))
			cityToStr := strings.TrimSpace(r.PostFormValue(fmt.Sprintf("coverage_to_%d", city.ID)))
			cityDistStr := strings.TrimSpace(r.PostFormValue(fmt.Sprintf("distance_meters_%d", city.ID)))

			cFromTime := fromTime
			if cityFromStr != "" {
				cFromTime = workflow.TimeOfDay(cityFromStr)
			}
			cToTime := toTime
			if cityToStr != "" {
				cToTime = workflow.TimeOfDay(cityToStr)
			}
			cDistance := distanceMeters
			if d, err := strconv.Atoi(cityDistStr); err == nil && d > 0 {
				cDistance = d
			}

			for _, day := range daysToCreate {
				newCoverages = append(newCoverages, &workflow.WeeklyCoverage{
					OrganizationID: actor.OrganizationID,
					BranchID:       branchID,
					GovernorateID:  city.GovernorateID,
					CityID:         &cityID,
					DayOfWeek:      day,
					CoverageFrom:   cFromTime,
					CoverageTo:     cToTime,
					Address:        cityName,
					Latitude:       &lat,
					Longitude:      &lon,
					DistanceMeters: cDistance,
					IsActive:       isActive,
				})
			}
		}
	} else {
		// Single record fallback
		var latVal, lngVal *float64
		if latStr := r.PostFormValue("latitude"); latStr != "" {
			if lat, err := strconv.ParseFloat(latStr, 64); err == nil {
				latVal = &lat
			}
		}
		if lngStr := r.PostFormValue("longitude"); lngStr != "" {
			if lng, err := strconv.ParseFloat(lngStr, 64); err == nil {
				lngVal = &lng
			}
		}
		if latVal == nil && targetBranch != nil && targetBranch.Latitude != nil {
			latVal = targetBranch.Latitude
		}
		if lngVal == nil && targetBranch != nil && targetBranch.Longitude != nil {
			lngVal = targetBranch.Longitude
		}

		address := r.PostFormValue("address")
		if address == "" && targetBranch != nil && targetBranch.Address != "" {
			address = targetBranch.Address
		}

		for _, day := range daysToCreate {
			newCoverages = append(newCoverages, &workflow.WeeklyCoverage{
				OrganizationID: actor.OrganizationID,
				BranchID:       branchID,
				GovernorateID:  govID,
				DayOfWeek:      day,
				CoverageFrom:   fromTime,
				CoverageTo:     toTime,
				Address:        address,
				Latitude:       latVal,
				Longitude:      lngVal,
				DistanceMeters: distanceMeters,
				IsActive:       isActive,
			})
		}
	}

	if len(newCoverages) == 0 {
		h.redirectWithNotice(w, r, "/vendor/coverage", "error", "يرجى اختيار مدينة أو محافظة صالحة لإنشاء التغطية.")
		return
	}

	if h.wfSvc != nil {
		if err := h.wfSvc.CreateBatchWeeklyCoverage(ctx, newCoverages); err != nil {
			h.log.ErrorContext(ctx, "create batch weekly coverage failed", "error", err, "count", len(newCoverages))
			h.redirectWithNotice(w, r, "/vendor/coverage", "error", "حدث خطأ أثناء حفظ نطاقات التغطية: "+err.Error())
			return
		}
	}

	h.redirectWithNotice(w, r, "/vendor/coverage", "success",
		fmt.Sprintf("تم بنجاح إضافة وتفعيل %d نطاق تغطية أسبوعية للفرع المتنقل.", len(newCoverages)))
}

// VendorCoverageUpdateSubmit processes updates to a single weekly coverage record.
func (h *UIHandler) VendorCoverageUpdateSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/coverage", http.StatusSeeOther)
		return
	}

	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		id, err = strconv.ParseInt(r.PostFormValue("id"), 10, 64)
	}
	if err != nil || id <= 0 {
		id, err = strconv.ParseInt(r.PostFormValue("coverage_id"), 10, 64)
	}
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/vendor/coverage", "error", "معرف التغطية غير صالح.")
		return
	}

	existingCov, err := h.wfSvc.GetWeeklyCoverage(ctx, id)
	if err != nil || existingCov == nil || existingCov.OrganizationID != actor.OrganizationID {
		h.log.WarnContext(ctx, "cross-tenant coverage update attempt",
			"actor_org", actor.OrganizationID, "coverage_id", id)
		h.redirectWithNotice(w, r, "/vendor/coverage", "error", "نطاق التغطية غير موجود أو لا ينتمي إلى منشأتكم.")
		return
	}

	if err := r.ParseForm(); err != nil {
		h.redirectWithNotice(w, r, "/vendor/coverage", "error", "تعذر قراءة بيانات النموذج.")
		return
	}

	branchID, err := strconv.ParseInt(r.PostFormValue("branch_id"), 10, 64)
	if err != nil || branchID <= 0 {
		branchID = existingCov.BranchID
	}

	var targetBranch *org.Branch
	if h.orgSvc != nil {
		branch, err := h.orgSvc.GetBranch(ctx, branchID)
		if err != nil || branch.OrganizationID != actor.OrganizationID {
			h.log.WarnContext(ctx, "cross-tenant branch coverage update attempt",
				"actor_org", actor.OrganizationID, "target_branch", branchID)
			h.redirectWithNotice(w, r, "/vendor/coverage", "error", "الفرع المحدد لا ينتمي إلى منشأتكم.")
			return
		}
		targetBranch = branch
	}

	dayOfWeek, err := strconv.Atoi(r.PostFormValue("day_of_week"))
	if err != nil || dayOfWeek < 0 || dayOfWeek > 6 {
		dayOfWeek = existingCov.DayOfWeek
	}

	distanceMeters, _ := strconv.Atoi(r.PostFormValue("distance_meters"))
	if distanceMeters <= 0 {
		distanceMeters = existingCov.DistanceMeters
	}
	if distanceMeters <= 0 {
		distanceMeters = 5000
	}

	var govID *int64
	if gID, err := strconv.ParseInt(r.PostFormValue("governorate_id"), 10, 64); err == nil && gID > 0 {
		govID = &gID
	} else {
		govID = existingCov.GovernorateID
	}

	var cityID *int64
	var selectedCity *platformadmin.City
	if cID, err := strconv.ParseInt(r.PostFormValue("city_id"), 10, 64); err == nil && cID > 0 {
		cityID = &cID
		if h.adminSvc != nil {
			selectedCity, _ = h.adminSvc.GetCity(ctx, cID)
			if selectedCity != nil && selectedCity.GovernorateID != nil {
				govID = selectedCity.GovernorateID
			}
		}
	} else {
		cityID = existingCov.CityID
	}

	var latVal, lngVal *float64
	if selectedCity != nil {
		lat := selectedCity.Latitude
		lon := selectedCity.Longitude
		latVal = &lat
		lngVal = &lon
	} else {
		if latStr := r.PostFormValue("latitude"); latStr != "" {
			if lat, err := strconv.ParseFloat(latStr, 64); err == nil {
				latVal = &lat
			}
		}
		if lngStr := r.PostFormValue("longitude"); lngStr != "" {
			if lng, err := strconv.ParseFloat(lngStr, 64); err == nil {
				lngVal = &lng
			}
		}
		if latVal == nil && targetBranch != nil && targetBranch.Latitude != nil {
			latVal = targetBranch.Latitude
		} else if latVal == nil {
			latVal = existingCov.Latitude
		}
		if lngVal == nil && targetBranch != nil && targetBranch.Longitude != nil {
			lngVal = targetBranch.Longitude
		} else if lngVal == nil {
			lngVal = existingCov.Longitude
		}
	}

	address := r.PostFormValue("address")
	if address == "" && selectedCity != nil {
		address = selectedCity.Name.Get("ar")
	} else if address == "" && targetBranch != nil && targetBranch.Address != "" {
		address = targetBranch.Address
	} else if address == "" {
		address = existingCov.Address
	}

	isActive := existingCov.IsActive
	if activeStr := r.PostFormValue("is_active"); activeStr != "" {
		isActive = activeStr == "true" || activeStr == "on" || activeStr == "1"
	}

	fromTime := workflow.TimeOfDay(r.PostFormValue("coverage_from"))
	toTime := workflow.TimeOfDay(r.PostFormValue("coverage_to"))

	cov := workflow.WeeklyCoverage{
		ID:             id,
		OrganizationID: actor.OrganizationID,
		BranchID:       branchID,
		GovernorateID:  govID,
		CityID:         cityID,
		DayOfWeek:      dayOfWeek,
		CoverageFrom:   fromTime,
		CoverageTo:     toTime,
		Address:        address,
		Latitude:       latVal,
		Longitude:      lngVal,
		DistanceMeters: distanceMeters,
		IsActive:       isActive,
	}

	if err := h.wfSvc.UpdateWeeklyCoverage(ctx, &cov); err != nil {
		h.log.ErrorContext(ctx, "update weekly coverage", "error", err, "org", actor.OrganizationID, "id", id)
		h.redirectWithNotice(w, r, "/vendor/coverage", "error", "فشل تحديث بيانات التغطية: "+err.Error())
		return
	}

	h.redirectWithNotice(w, r, "/vendor/coverage", "success", "تم تحديث نطاق التغطية الأسبوعية بنجاح.")
}

// VendorCoverageDeleteSubmit deletes a weekly coverage record.
func (h *UIHandler) VendorCoverageDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/coverage", http.StatusSeeOther)
		return
	}

	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		id, err = strconv.ParseInt(r.PostFormValue("id"), 10, 64)
	}
	if err != nil || id <= 0 {
		id, err = strconv.ParseInt(r.PostFormValue("coverage_id"), 10, 64)
	}
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/vendor/coverage", "error", "معرف التغطية غير صالح.")
		return
	}

	existingCov, err := h.wfSvc.GetWeeklyCoverage(ctx, id)
	if err != nil || existingCov == nil || existingCov.OrganizationID != actor.OrganizationID {
		h.log.WarnContext(ctx, "cross-tenant coverage delete attempt",
			"actor_org", actor.OrganizationID, "coverage_id", id)
		h.redirectWithNotice(w, r, "/vendor/coverage", "error", "نطاق التغطية غير موجود أو لا ينتمي إلى منشأتكم.")
		return
	}

	if err := h.wfSvc.DeleteWeeklyCoverage(ctx, id); err != nil {
		h.log.ErrorContext(ctx, "delete weekly coverage", "error", err, "org", actor.OrganizationID, "id", id)
		h.redirectWithNotice(w, r, "/vendor/coverage", "error", "حدث خطأ أثناء حذف نطاق التغطية.")
		return
	}

	h.redirectWithNotice(w, r, "/vendor/coverage", "success", "تم حذف نطاق التغطية بنجاح.")
}

// VendorCoverageToggleSubmit toggles the active state of a weekly coverage record.
func (h *UIHandler) VendorCoverageToggleSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/coverage", http.StatusSeeOther)
		return
	}

	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		id, err = strconv.ParseInt(r.PostFormValue("id"), 10, 64)
	}
	if err != nil || id <= 0 {
		id, err = strconv.ParseInt(r.PostFormValue("coverage_id"), 10, 64)
	}
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/vendor/coverage", "error", "معرف التغطية غير صالح.")
		return
	}

	existingCov, err := h.wfSvc.GetWeeklyCoverage(ctx, id)
	if err != nil || existingCov == nil || existingCov.OrganizationID != actor.OrganizationID {
		h.log.WarnContext(ctx, "cross-tenant coverage toggle attempt",
			"actor_org", actor.OrganizationID, "coverage_id", id)
		h.redirectWithNotice(w, r, "/vendor/coverage", "error", "نطاق التغطية غير موجود أو لا ينتمي إلى منشأتكم.")
		return
	}

	newActive := !existingCov.IsActive
	if err := h.wfSvc.ToggleWeeklyCoverage(ctx, id, newActive); err != nil {
		h.log.ErrorContext(ctx, "toggle weekly coverage", "error", err, "org", actor.OrganizationID, "id", id)
		h.redirectWithNotice(w, r, "/vendor/coverage", "error", "حدث خطأ أثناء تعديل حالة التغطية.")
		return
	}

	stateLabel := "تفعيل"
	if !newActive {
		stateLabel = "تعطيل"
	}
	h.redirectWithNotice(w, r, "/vendor/coverage", "success", fmt.Sprintf("تم %s نطاق التغطية بنجاح.", stateLabel))
}

// VendorDeliveryBandCreateSubmit creates a new distance delivery fee tier for the vendor.
func (h *UIHandler) VendorDeliveryBandCreateSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/coverage", http.StatusSeeOther)
		return
	}

	if err := r.ParseForm(); err != nil {
		h.redirectWithNotice(w, r, "/vendor/coverage", "error", "تعذر قراءة بيانات الشريحة.")
		return
	}

	fromMeters := 0
	if val := r.PostFormValue("from_meters"); val != "" {
		fromMeters, _ = strconv.Atoi(val)
	} else if val := r.PostFormValue("min_distance_meters"); val != "" {
		fromMeters, _ = strconv.Atoi(val)
	} else if val := r.PostFormValue("min_distance_km"); val != "" {
		km, _ := strconv.Atoi(val)
		fromMeters = km * 1000
	}

	toMeters := 0
	if val := r.PostFormValue("to_meters"); val != "" {
		toMeters, _ = strconv.Atoi(val)
	} else if val := r.PostFormValue("max_distance_meters"); val != "" {
		toMeters, _ = strconv.Atoi(val)
	} else if val := r.PostFormValue("max_distance_km"); val != "" {
		km, _ := strconv.Atoi(val)
		toMeters = km * 1000
	}

	feeAmount, _ := strconv.ParseFloat(r.PostFormValue("delivery_fee"), 64)

	if toMeters <= fromMeters || fromMeters < 0 || feeAmount < 0 {
		h.redirectWithNotice(w, r, "/vendor/coverage", "error", "يرجى التحقق من صحة المسافات بالمتر وقيمة رسوم التوصيل (يجب أن تكون مسافة النهاية أكبر من البداية).")
		return
	}

	if h.orgSvc != nil {
		bands, err := h.orgSvc.GetDeliveryBands(ctx, actor.OrganizationID)
		if err != nil {
			bands = []*org.DeliveryBand{}
		}
		feeAmt, _ := money.Parse(fmt.Sprintf("%.2f", feeAmount))
		newBand := &org.DeliveryBand{
			OrganizationID: actor.OrganizationID,
			FromMeters:     fromMeters,
			ToMeters:       toMeters,
			Fee:            feeAmt,
			IsActive:       true,
		}
		bands = append(bands, newBand)
		if err := h.orgSvc.SaveDeliveryBands(ctx, actor.OrganizationID, bands); err != nil {
			h.log.ErrorContext(ctx, "save delivery bands", "error", err, "org", actor.OrganizationID)
			h.redirectWithNotice(w, r, "/vendor/coverage", "error", "فشل حفظ شريحة التوصيل: "+err.Error())
			return
		}
	}

	h.redirectWithNotice(w, r, "/vendor/coverage", "success", "تم إضافة شريحة تسعير التوصيل بنجاح.")
}

// VendorDeliveryBandDeleteSubmit removes a distance delivery fee tier.
func (h *UIHandler) VendorDeliveryBandDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/coverage", http.StatusSeeOther)
		return
	}

	bandID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || bandID <= 0 {
		h.redirectWithNotice(w, r, "/vendor/coverage", "error", "معرف الشريحة غير صالح.")
		return
	}

	if h.orgSvc != nil {
		bands, err := h.orgSvc.GetDeliveryBands(ctx, actor.OrganizationID)
		if err == nil {
			var updated []*org.DeliveryBand
			for _, b := range bands {
				if b.ID != bandID {
					updated = append(updated, b)
				}
			}
			_ = h.orgSvc.SaveDeliveryBands(ctx, actor.OrganizationID, updated)
		}
	}

	h.redirectWithNotice(w, r, "/vendor/coverage", "success", "تم حذف شريحة التوصيل بنجاح.")
}

// ResolveVendorShippingFee calculates the dynamic distance-based delivery fee between a vendor
// (or vendor warehouse/coverage city) and the customer pharmacy branch.
func (h *UIHandler) ResolveVendorShippingFee(ctx context.Context, vendorOrgID int64, vendorBranchID *int64, customerBranchID *int64) money.Amount {
	if h.orgSvc == nil || vendorOrgID <= 0 {
		return money.Zero
	}

	// 1. Resolve Customer Branch Coordinates
	var custLat, custLon *float64
	if customerBranchID != nil && *customerBranchID > 0 {
		if cb, err := h.orgSvc.GetBranch(ctx, *customerBranchID); err == nil && cb != nil {
			if cb.Latitude != nil && cb.Longitude != nil && *cb.Latitude != 0 && *cb.Longitude != 0 {
				custLat = cb.Latitude
				custLon = cb.Longitude
			}
		}
	}

	// 2. Resolve Vendor Coordinates (Branch or Coverage Center)
	var vendLat, vendLon *float64
	if vendorBranchID != nil && *vendorBranchID > 0 {
		if vb, err := h.orgSvc.GetBranch(ctx, *vendorBranchID); err == nil && vb != nil {
			if vb.Latitude != nil && vb.Longitude != nil && *vb.Latitude != 0 && *vb.Longitude != 0 {
				vendLat = vb.Latitude
				vendLon = vb.Longitude
			}
		}
	}

	if (vendLat == nil || vendLon == nil) && h.wfSvc != nil {
		if coverages, err := h.wfSvc.ListCoverageForOrganization(ctx, vendorOrgID); err == nil && len(coverages) > 0 {
			for _, cov := range coverages {
				if cov.Latitude != nil && cov.Longitude != nil && *cov.Latitude != 0 && *cov.Longitude != 0 {
					vendLat = cov.Latitude
					vendLon = cov.Longitude
					break
				}
			}
		}
	}

	// 3. Compute Distance in Meters (default 5,000 meters / 5 km if in same delivery zone without GPS)
	distMeters := 5000
	if custLat != nil && custLon != nil && vendLat != nil && vendLon != nil {
		distMeters = haversineCoverageDistance(*vendLat, *vendLon, *custLat, *custLon)
	}

	// 4. Match against vendor's DeliveryBands
	fee, matched, err := h.orgSvc.CalculateDeliveryFee(ctx, vendorOrgID, distMeters)
	if err == nil && matched {
		return fee
	}

	return money.Zero
}

// VendorBranchCoveragePage redirects branch-specific coverage view to the main coverage console.
func (h *UIHandler) VendorBranchCoveragePage(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/vendor/coverage", http.StatusSeeOther)
}

// APIGovernorateCitiesJSON returns all cities belonging to a governorate as JSON.
func (h *UIHandler) APIGovernorateCitiesJSON(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	govID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || govID <= 0 {
		http.Error(w, `{"error":"invalid governorate id"}`, http.StatusBadRequest)
		return
	}

	if h.adminSvc == nil {
		http.Error(w, `{"error":"admin service unavailable"}`, http.StatusServiceUnavailable)
		return
	}

	cities, err := h.adminSvc.ListCitiesByGovernorate(ctx, govID)
	if err != nil {
		h.log.ErrorContext(ctx, "failed to list cities by governorate", "error", err, "gov_id", govID)
		http.Error(w, `{"error":"failed to query cities"}`, http.StatusInternalServerError)
		return
	}

	type cityResponse struct {
		ID        int64   `json:"id"`
		GovID     int64   `json:"gov_id"`
		NameAR    string  `json:"name_ar"`
		NameEN    string  `json:"name_en"`
		Lat       float64 `json:"lat"`
		Lon       float64 `json:"lon"`
		IsCapital bool    `json:"is_capital"`
	}

	resp := make([]cityResponse, 0, len(cities))
	for _, c := range cities {
		resp = append(resp, cityResponse{
			ID:        c.ID,
			GovID:     govID,
			NameAR:    c.Name.Get("ar"),
			NameEN:    c.Name.Get("en"),
			Lat:       c.Latitude,
			Lon:       c.Longitude,
			IsCapital: c.IsCapital,
		})
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(resp)
}
