package ui

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/org"
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
	_ = pages.AdminOrganizationDetail(organization, nil, nil, "info", lang, dir).Render(ctx, w)
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
	_ = pages.AdminOrganizationDetail(organization, nil, employees, "users", lang, dir).Render(ctx, w)
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
	_ = pages.AdminOrganizationDetail(organization, branches, nil, "branches", lang, dir).Render(ctx, w)
}

// AdminBranchesPage renders list of all branches across all organizations.
func (h *UIHandler) AdminBranchesPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	var branches []*org.Branch
	if h.orgSvc != nil {
		// AsSystem justified: platform admin viewing branches across all tenant orgs
		branches, _ = h.orgSvc.ListBranches(database.AsSystem(ctx), 0)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminBranchesPage(branches, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin branches page", "error", err)
	}
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
	_ = pages.AdminBranchDetailPage(branch, lang, dir).Render(ctx, w)
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
	_ = pages.AdminBranchDetailPage(branch, lang, dir).Render(ctx, w)
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
	_ = pages.AdminBranchDetailPage(branch, lang, dir).Render(ctx, w)
}

// AdminWeeklyCoveragesPage renders all weekly delivery coverages across all vendors.
func (h *UIHandler) AdminWeeklyCoveragesPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	var coverages []*workflow.CoverageView
	if h.wfSvc != nil {
		// AsSystem justified: platform admin viewing weekly coverages across all vendors
		coverages, _ = h.wfSvc.ListCoverageForOrganization(database.AsSystem(ctx), 0)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminWeeklyCoveragesPage(coverages, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin weekly coverages", "error", err)
	}
}

// AdminWeeklyCoverageNewPage renders coverage creation form.
func (h *UIHandler) AdminWeeklyCoverageNewPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	var branches []*org.Branch
	if h.orgSvc != nil {
		branches, _ = h.orgSvc.ListBranches(database.AsSystem(ctx), 0)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = pages.AdminWeeklyCoverageForm(nil, branches, lang, dir).Render(ctx, w)
}

// AdminWeeklyCoverageCreateSubmit handles creating weekly coverage rule.
func (h *UIHandler) AdminWeeklyCoverageCreateSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_ = r.ParseForm()

	branchID, _ := strconv.ParseInt(r.PostFormValue("branch_id"), 10, 64)
	dayOfWeek, _ := strconv.Atoi(r.PostFormValue("day_of_week"))
	distMeters, _ := strconv.Atoi(r.PostFormValue("distance_meters"))

	var orgID int64
	if h.orgSvc != nil && branchID > 0 {
		if b, err := h.orgSvc.GetBranch(database.AsSystem(ctx), branchID); err == nil && b != nil {
			orgID = b.OrganizationID
		}
	}

	if h.wfSvc != nil && orgID > 0 && branchID > 0 {
		cov := &workflow.WeeklyCoverage{
			OrganizationID: orgID,
			BranchID:       branchID,
			DayOfWeek:      dayOfWeek,
			DistanceMeters: distMeters,
			IsActive:       true,
		}
		_ = h.wfSvc.CreateWeeklyCoverage(database.AsSystem(ctx), cov)
	}

	http.Redirect(w, r, "/admin/weekly-coverages", http.StatusSeeOther)
}

// AdminWeeklyCoverageEditPage renders edit form for a weekly coverage rule.
func (h *UIHandler) AdminWeeklyCoverageEditPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	idStr := chi.URLParam(r, "id")
	covID, _ := strconv.ParseInt(idStr, 10, 64)

	var cov *workflow.WeeklyCoverage
	var branches []*org.Branch
	if h.wfSvc != nil {
		cov, _ = h.wfSvc.GetWeeklyCoverage(database.AsSystem(ctx), covID)
	}
	if h.orgSvc != nil {
		branches, _ = h.orgSvc.ListBranches(database.AsSystem(ctx), 0)
	}

	if cov == nil {
		http.Redirect(w, r, "/admin/weekly-coverages", http.StatusSeeOther)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = pages.AdminWeeklyCoverageForm(cov, branches, lang, dir).Render(ctx, w)
}

// AdminWeeklyCoverageDetailPage renders detail & map view for coverage.
func (h *UIHandler) AdminWeeklyCoverageDetailPage(w http.ResponseWriter, r *http.Request) {
	h.AdminWeeklyCoverageEditPage(w, r)
}

// AdminWeeklyCoverageUpdateSubmit updates an existing coverage rule.
func (h *UIHandler) AdminWeeklyCoverageUpdateSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	idStr := chi.URLParam(r, "id")
	covID, _ := strconv.ParseInt(idStr, 10, 64)
	_ = r.ParseForm()

	dayOfWeek, _ := strconv.Atoi(r.PostFormValue("day_of_week"))
	distMeters, _ := strconv.Atoi(r.PostFormValue("distance_meters"))

	if h.wfSvc != nil && covID > 0 {
		cov, err := h.wfSvc.GetWeeklyCoverage(database.AsSystem(ctx), covID)
		if err == nil && cov != nil {
			cov.DayOfWeek = dayOfWeek
			cov.DistanceMeters = distMeters
			_ = h.wfSvc.UpdateWeeklyCoverage(database.AsSystem(ctx), cov)
		}
	}

	http.Redirect(w, r, "/admin/weekly-coverages", http.StatusSeeOther)
}

// AdminWeeklyCoverageDeleteSubmit removes a coverage rule.
func (h *UIHandler) AdminWeeklyCoverageDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	idStr := chi.URLParam(r, "id")
	covID, _ := strconv.ParseInt(idStr, 10, 64)

	if h.wfSvc != nil && covID > 0 {
		_ = h.wfSvc.DeleteWeeklyCoverage(database.AsSystem(ctx), covID)
	}

	http.Redirect(w, r, "/admin/weekly-coverages", http.StatusSeeOther)
}

// AdminWeeklyCoverageToggleSubmit toggles active status of a coverage rule.
func (h *UIHandler) AdminWeeklyCoverageToggleSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	idStr := chi.URLParam(r, "id")
	covID, _ := strconv.ParseInt(idStr, 10, 64)

	if h.wfSvc != nil && covID > 0 {
		cov, err := h.wfSvc.GetWeeklyCoverage(database.AsSystem(ctx), covID)
		if err == nil && cov != nil {
			_ = h.wfSvc.ToggleWeeklyCoverage(database.AsSystem(ctx), covID, !cov.IsActive)
		}
	}

	http.Redirect(w, r, "/admin/weekly-coverages", http.StatusSeeOther)
}
