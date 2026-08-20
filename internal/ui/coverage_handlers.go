package ui

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/workflow"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
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

// VendorCoverageCreateSubmit processes creation of weekly coverage for a branch.
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
		h.redirectWithNotice(w, r, "/vendor/coverage", "error", "يجب اختيار الفرع التابع لمنشأتكم.")
		return
	}

	// Tenancy Check: verify the branch belongs to the actor's organization.
	if h.orgSvc != nil {
		branch, err := h.orgSvc.GetBranch(ctx, branchID)
		if err != nil || branch.OrganizationID != actor.OrganizationID {
			h.log.WarnContext(ctx, "cross-tenant branch coverage creation attempt",
				"actor_org", actor.OrganizationID, "target_branch", branchID)
			h.redirectWithNotice(w, r, "/vendor/coverage", "error", "الفرع المحدد لا ينتمي إلى منشأتكم.")
			return
		}
	}

	dayOfWeek, err := strconv.Atoi(r.PostFormValue("day_of_week"))
	if err != nil || dayOfWeek < 0 || dayOfWeek > 6 {
		h.redirectWithNotice(w, r, "/vendor/coverage", "error", "يجب تحديد يوم أسبوع صالح (0 إلى 6).")
		return
	}

	distanceMeters, _ := strconv.Atoi(r.PostFormValue("distance_meters"))
	if distanceMeters <= 0 {
		distanceMeters = 25000 // default 25km
	}

	var cityID *int64
	if cID, err := strconv.ParseInt(r.PostFormValue("city_id"), 10, 64); err == nil && cID > 0 {
		cityID = &cID
	}

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

	cov := workflow.WeeklyCoverage{
		OrganizationID: actor.OrganizationID,
		BranchID:       branchID,
		CityID:         cityID,
		DayOfWeek:      dayOfWeek,
		CoverageFrom:   workflow.TimeOfDay(r.PostFormValue("coverage_from")),
		CoverageTo:     workflow.TimeOfDay(r.PostFormValue("coverage_to")),
		Address:        r.PostFormValue("address"),
		Latitude:       latVal,
		Longitude:      lngVal,
		DistanceMeters: distanceMeters,
		IsActive:       r.PostFormValue("is_active") == "true" || r.PostFormValue("is_active") == "on" || r.PostFormValue("is_active") == "1",
	}

	if err := h.wfSvc.CreateWeeklyCoverage(ctx, &cov); err != nil {
		h.log.ErrorContext(ctx, "create weekly coverage", "error", err, "org", actor.OrganizationID)
		h.redirectWithNotice(w, r, "/vendor/coverage", "error", "حدث خطأ أثناء حفظ نطاق التغطية.")
		return
	}

	h.redirectWithNotice(w, r, "/vendor/coverage", "success", "تم إضافة نطاق التغطية الأسبوعية بنجاح.")
}

// VendorCoverageUpdateSubmit processes updates to a weekly coverage record.
func (h *UIHandler) VendorCoverageUpdateSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/coverage", http.StatusSeeOther)
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/vendor/coverage", "error", "معرف التغطية غير صالح.")
		return
	}

	if err := r.ParseForm(); err != nil {
		h.redirectWithNotice(w, r, "/vendor/coverage", "error", "تعذر قراءة بيانات النموذج.")
		return
	}

	branchID, err := strconv.ParseInt(r.PostFormValue("branch_id"), 10, 64)
	if err != nil || branchID <= 0 {
		h.redirectWithNotice(w, r, "/vendor/coverage", "error", "يجب اختيار الفرع التابع لمنشأتكم.")
		return
	}

	if h.orgSvc != nil {
		branch, err := h.orgSvc.GetBranch(ctx, branchID)
		if err != nil || branch.OrganizationID != actor.OrganizationID {
			h.log.WarnContext(ctx, "cross-tenant branch coverage update attempt",
				"actor_org", actor.OrganizationID, "target_branch", branchID)
			h.redirectWithNotice(w, r, "/vendor/coverage", "error", "الفرع المحدد لا ينتمي إلى منشأتكم.")
			return
		}
	}

	dayOfWeek, err := strconv.Atoi(r.PostFormValue("day_of_week"))
	if err != nil || dayOfWeek < 0 || dayOfWeek > 6 {
		h.redirectWithNotice(w, r, "/vendor/coverage", "error", "يجب تحديد يوم أسبوع صالح (0 إلى 6).")
		return
	}

	distanceMeters, _ := strconv.Atoi(r.PostFormValue("distance_meters"))
	if distanceMeters <= 0 {
		distanceMeters = 25000
	}

	var cityID *int64
	if cID, err := strconv.ParseInt(r.PostFormValue("city_id"), 10, 64); err == nil && cID > 0 {
		cityID = &cID
	}

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

	cov := workflow.WeeklyCoverage{
		ID:             id,
		OrganizationID: actor.OrganizationID,
		BranchID:       branchID,
		CityID:         cityID,
		DayOfWeek:      dayOfWeek,
		CoverageFrom:   workflow.TimeOfDay(r.PostFormValue("coverage_from")),
		CoverageTo:     workflow.TimeOfDay(r.PostFormValue("coverage_to")),
		Address:        r.PostFormValue("address"),
		Latitude:       latVal,
		Longitude:      lngVal,
		DistanceMeters: distanceMeters,
		IsActive:       r.PostFormValue("is_active") == "true" || r.PostFormValue("is_active") == "on" || r.PostFormValue("is_active") == "1",
	}

	if err := h.wfSvc.UpdateWeeklyCoverage(ctx, &cov); err != nil {
		h.log.ErrorContext(ctx, "update weekly coverage", "error", err, "org", actor.OrganizationID, "id", id)
		h.redirectWithNotice(w, r, "/vendor/coverage", "error", "حدث خطأ أثناء تحديث نطاق التغطية.")
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

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/vendor/coverage", "error", "معرف التغطية غير صالح.")
		return
	}

	cov, err := h.wfSvc.GetWeeklyCoverage(ctx, id)
	if err != nil || cov.OrganizationID != actor.OrganizationID {
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

// VendorCoverageToggleSubmit toggles the active state of a coverage record.
func (h *UIHandler) VendorCoverageToggleSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/coverage", http.StatusSeeOther)
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/vendor/coverage", "error", "معرف التغطية غير صالح.")
		return
	}

	cov, err := h.wfSvc.GetWeeklyCoverage(ctx, id)
	if err != nil || cov.OrganizationID != actor.OrganizationID {
		h.log.WarnContext(ctx, "cross-tenant coverage toggle attempt",
			"actor_org", actor.OrganizationID, "coverage_id", id)
		h.redirectWithNotice(w, r, "/vendor/coverage", "error", "نطاق التغطية غير موجود أو لا ينتمي إلى منشأتكم.")
		return
	}

	if err := h.wfSvc.ToggleWeeklyCoverage(ctx, id, !cov.IsActive); err != nil {
		h.log.ErrorContext(ctx, "toggle weekly coverage", "error", err, "org", actor.OrganizationID, "id", id)
		h.redirectWithNotice(w, r, "/vendor/coverage", "error", "حدث خطأ أثناء تغيير حالة التغطية.")
		return
	}

	h.redirectWithNotice(w, r, "/vendor/coverage", "success", "تم تعديل حالة نطاق التغطية بنجاح.")
}

// VendorBranchCoveragePage renders the coverage specifically for one branch.
func (h *UIHandler) VendorBranchCoveragePage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/coverage", http.StatusSeeOther)
		return
	}

	branchID, err := strconv.ParseInt(chi.URLParam(r, "branchID"), 10, 64)
	if err != nil || branchID <= 0 {
		h.redirectWithNotice(w, r, "/vendor/coverage", "error", "معرف الفرع غير صالح.")
		return
	}

	http.Redirect(w, r, fmt.Sprintf("/vendor/coverage?branch=%d", branchID), http.StatusSeeOther)
}
