package ui

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/org"
	platformadmin "github.com/muhiya/dawa24-store/internal/modules/platform_admin"
	"github.com/muhiya/dawa24-store/internal/modules/workflow"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// AdminOrganizationDetailPage renders the main profile overview for an organization.
func (h *UIHandler) AdminOrganizationDetailPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	idStr := chi.URLParam(r, "id")
	orgID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || orgID <= 0 {
		http.Redirect(w, r, "/admin/organizations", http.StatusSeeOther)
		return
	}

	var organization *org.Organization
	var branches []*org.Branch
	var employees []*org.EmployeeView
	if h.orgSvc != nil {
		// AsSystem justified: platform admin inspecting organization details across tenants
		sysCtx := database.AsSystem(ctx)
		if o, err := h.orgSvc.GetOrganization(sysCtx, orgID); err == nil && o != nil {
			organization = o
		}
		branches, _ = h.orgSvc.ListBranches(sysCtx, orgID)
		employees, _ = h.orgSvc.ListEmployees(sysCtx, orgID)
	}

	if organization == nil {
		h.redirectWithNotice(w, r, "/admin/organizations", "error", "المنشأة غير موجودة.")
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminOrganizationDetail(organization, branches, employees, "overview", lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin org detail", "error", err)
	}
}

// AdminOrganizationInfoPage renders registration and documents info.
func (h *UIHandler) AdminOrganizationInfoPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	idStr := chi.URLParam(r, "id")
	orgID, _ := strconv.ParseInt(idStr, 10, 64)

	var organization *org.Organization
	if h.orgSvc != nil {
		organization, _ = h.orgSvc.GetOrganization(database.AsSystem(ctx), orgID)
	}
	if organization == nil {
		http.Redirect(w, r, "/admin/organizations", http.StatusSeeOther)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminOrganizationDetail(organization, nil, nil, "info", lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin organization detail info", "error", err)
	}
}

// AdminOrganizationUsersPage renders members and employees list.
func (h *UIHandler) AdminOrganizationUsersPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	idStr := chi.URLParam(r, "id")
	orgID, _ := strconv.ParseInt(idStr, 10, 64)

	var organization *org.Organization
	var employees []*org.EmployeeView
	if h.orgSvc != nil {
		sysCtx := database.AsSystem(ctx)
		organization, _ = h.orgSvc.GetOrganization(sysCtx, orgID)
		employees, _ = h.orgSvc.ListEmployees(sysCtx, orgID)
	}
	if organization == nil {
		http.Redirect(w, r, "/admin/organizations", http.StatusSeeOther)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminOrganizationDetail(organization, nil, employees, "users", lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin organization detail users", "error", err)
	}
}

// AdminOrganizationBranchesPage renders branches for a specific org.
func (h *UIHandler) AdminOrganizationBranchesPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	idStr := chi.URLParam(r, "id")
	orgID, _ := strconv.ParseInt(idStr, 10, 64)

	var organization *org.Organization
	var branches []*org.Branch
	if h.orgSvc != nil {
		sysCtx := database.AsSystem(ctx)
		organization, _ = h.orgSvc.GetOrganization(sysCtx, orgID)
		branches, _ = h.orgSvc.ListBranches(sysCtx, orgID)
	}
	if organization == nil {
		http.Redirect(w, r, "/admin/organizations", http.StatusSeeOther)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminOrganizationDetail(organization, branches, nil, "branches", lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin organization detail branches", "error", err)
	}
}

// AdminBranchesPage renders list of all branches within the unified enterprise hub.
func (h *UIHandler) AdminBranchesPage(w http.ResponseWriter, r *http.Request) {
	h.renderAdminEnterpriseHub(w, r, "branches")
}

// AdminBranchDetailPage renders detail for a specific branch.
func (h *UIHandler) AdminBranchDetailPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	idStr := chi.URLParam(r, "id")
	branchID, _ := strconv.ParseInt(idStr, 10, 64)

	var branch *org.Branch
	if h.orgSvc != nil {
		branch, _ = h.orgSvc.GetBranch(database.AsSystem(ctx), branchID)
	}
	if branch == nil {
		http.Redirect(w, r, "/admin/branches", http.StatusSeeOther)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminBranchDetailPage(branch, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin branch detail", "error", err)
	}
}

// AdminBranchProductsPage renders catalog assigned to a branch.
func (h *UIHandler) AdminBranchProductsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	idStr := chi.URLParam(r, "id")
	branchID, _ := strconv.ParseInt(idStr, 10, 64)

	var branch *org.Branch
	if h.orgSvc != nil {
		branch, _ = h.orgSvc.GetBranch(database.AsSystem(ctx), branchID)
	}
	if branch == nil {
		http.Redirect(w, r, "/admin/branches", http.StatusSeeOther)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminBranchDetailPage(branch, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin branch products", "error", err)
	}
}

// AdminBranchUsersPage renders staff assigned to a branch.
func (h *UIHandler) AdminBranchUsersPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	idStr := chi.URLParam(r, "id")
	branchID, _ := strconv.ParseInt(idStr, 10, 64)

	var branch *org.Branch
	if h.orgSvc != nil {
		branch, _ = h.orgSvc.GetBranch(database.AsSystem(ctx), branchID)
	}
	if branch == nil {
		http.Redirect(w, r, "/admin/branches", http.StatusSeeOther)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminBranchDetailPage(branch, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin branch users", "error", err)
	}
}

// AdminWeeklyCoveragesPage renders weekly delivery coverages filtered by organization dropdown.
func (h *UIHandler) AdminWeeklyCoveragesPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	var orgs []*org.Organization
	if h.orgSvc != nil {
		orgs, _ = h.orgSvc.ListOrganizations(database.AsSystem(ctx), nil, nil, 200, 0)
	}

	var selectedOrgID int64
	if orgIDStr := r.URL.Query().Get("org_id"); orgIDStr != "" {
		selectedOrgID, _ = strconv.ParseInt(orgIDStr, 10, 64)
	}

	var coverages []*workflow.CoverageView
	if h.wfSvc != nil {
		// AsSystem justified: platform admin viewing weekly coverages across tenants
		coverages, _ = h.wfSvc.ListCoverageForOrganization(database.AsSystem(ctx), selectedOrgID)
	}

	var branches []*org.Branch
	if h.orgSvc != nil {
		if selectedOrgID > 0 {
			branches, _ = h.orgSvc.ListBranches(database.AsSystem(ctx), selectedOrgID)
		} else {
			branches, _ = h.orgSvc.ListBranches(database.AsSystem(ctx), 0)
		}
	}

	var cities []*platformadmin.City
	if h.adminSvc != nil {
		cities, _ = h.adminSvc.ListCities(database.AsSystem(ctx), 1)
	}

	data := pages.AdminWeeklyCoveragesData{
		Coverages:     coverages,
		Organizations: orgs,
		Branches:      branches,
		Cities:        cities,
		SelectedOrgID: selectedOrgID,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminWeeklyCoveragesPage(data, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin weekly coverages", "error", err)
	}
}

// AdminWeeklyCoverageCreateSubmit creates a new weekly coverage schedule.
func (h *UIHandler) AdminWeeklyCoverageCreateSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_ = r.ParseForm()

	orgID, _ := strconv.ParseInt(r.PostFormValue("organization_id"), 10, 64)
	branchID, _ := strconv.ParseInt(r.PostFormValue("branch_id"), 10, 64)
	cityIDVal, _ := strconv.ParseInt(r.PostFormValue("city_id"), 10, 64)
	var cityID *int64
	if cityIDVal > 0 {
		cityID = &cityIDVal
	}
	dayOfWeek, _ := strconv.Atoi(r.PostFormValue("day_of_week"))
	distKm, _ := strconv.Atoi(r.PostFormValue("distance_km"))
	if distKm <= 0 {
		distKm = 50
	}
	address := r.PostFormValue("address")

	var covFrom, covTo *string
	if f := r.PostFormValue("coverage_from"); f != "" {
		covFrom = &f
	}
	if t := r.PostFormValue("coverage_to"); t != "" {
		covTo = &t
	}

	cov := workflow.WeeklyCoverage{
		OrganizationID: orgID,
		BranchID:       branchID,
		CityID:         cityID,
		DayOfWeek:      dayOfWeek,
		CoverageFrom:   covFrom,
		CoverageTo:     covTo,
		DistanceMeters: distKm * 1000,
		Address:        address,
		IsActive:       true,
	}

	redirectOrgID := r.PostFormValue("redirect_org_id")

	if h.wfSvc != nil {
		if err := h.wfSvc.CreateWeeklyCoverage(database.AsSystem(ctx), &cov); err != nil {
			h.log.ErrorContext(ctx, "create weekly coverage", "error", err)
			h.redirectWithNotice(w, r, "/admin/weekly-coverages?org_id="+redirectOrgID, "error", h.safeMessage(err, langOf(r)))
			return
		}
	}

	h.redirectWithNotice(w, r, "/admin/weekly-coverages?org_id="+redirectOrgID, "success", "تم إضافة جدول التغطية الأسبوعية بنجاح.")
}

// AdminWeeklyCoverageToggleSubmit toggles the active status of a coverage schedule.
func (h *UIHandler) AdminWeeklyCoverageToggleSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	_ = r.ParseForm()
	isActive := r.PostFormValue("is_active") == "true"
	orgID := r.PostFormValue("org_id")

	if h.wfSvc != nil && id > 0 {
		_ = h.wfSvc.ToggleWeeklyCoverage(database.AsSystem(ctx), id, isActive)
	}

	h.redirectWithNotice(w, r, "/admin/weekly-coverages?org_id="+orgID, "success", "تم تحديث حالة جدول التغطية بنجاح.")
}

// AdminWeeklyCoverageDeleteSubmit deletes a coverage schedule.
func (h *UIHandler) AdminWeeklyCoverageDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	_ = r.ParseForm()
	orgID := r.PostFormValue("org_id")

	if h.wfSvc != nil && id > 0 {
		_ = h.wfSvc.DeleteWeeklyCoverage(database.AsSystem(ctx), id)
	}

	h.redirectWithNotice(w, r, "/admin/weekly-coverages?org_id="+orgID, "success", "تم حذف جدول التغطية بنجاح.")
}
