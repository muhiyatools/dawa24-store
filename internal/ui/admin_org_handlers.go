package ui

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/attachments"
	"github.com/muhiya/dawa24-store/internal/modules/billing"
	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	platformadmin "github.com/muhiya/dawa24-store/internal/modules/platform_admin"
	"github.com/muhiya/dawa24-store/internal/modules/workflow"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// AdminOrganizationDetailPage renders the deep 360° profile overview for an organization.
func (h *UIHandler) AdminOrganizationDetailPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sysCtx := database.AsSystem(ctx)
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
	var docs []*attachments.Document
	var wallet *billing.Wallet
	var recentTxs []*billing.WalletTransaction
	var recentDeposits []*billing.WalletDeposit
	var ordersCount int
	var recentOrders []*commerce.Order

	if h.orgSvc != nil {
		organization, _ = h.orgSvc.GetOrganization(sysCtx, orgID)
		branches, _ = h.orgSvc.ListBranches(sysCtx, orgID)
		employees, _ = h.orgSvc.ListEmployees(sysCtx, orgID)
	}

	if organization == nil {
		h.redirectWithNotice(w, r, "/admin/organizations", "error", i18n.T(lang, "admin.org.not_found"))
		return
	}

	if h.attSvc != nil {
		docs, _ = h.attSvc.ListByOrganization(sysCtx, orgID)
	}

	if h.billSvc != nil && organization.OwnerID > 0 {
		wallet, _ = h.billSvc.GetWallet(sysCtx, organization.OwnerID, "EGP")
		if wallet != nil {
			recentTxs, _ = h.billSvc.ListWalletTransactions(sysCtx, wallet.ID, 5, 0)
		}
		recentDeposits, _ = h.billSvc.ListUserDeposits(sysCtx, organization.OwnerID, 5, 0)
	}

	if h.commSvc != nil {
		if ords, err := h.commSvc.AdminSearchOrders(sysCtx, "", 20, 0); err == nil {
			for _, o := range ords {
				if o != nil && o.OrganizationID != nil && *o.OrganizationID == orgID {
					ordersCount++
					if len(recentOrders) < 5 {
						recentOrders = append(recentOrders, o)
					}
				}
			}
		}
	}

	aiUserID, aiKey := h.EnsureOrgAIGatewayProvisioned(ctx, orgID)

	data := pages.AdminOrgDetailData{
		Organization:   organization,
		Branches:       branches,
		Employees:      employees,
		Documents:      docs,
		Wallet:         wallet,
		RecentTxs:      recentTxs,
		RecentDeposits: recentDeposits,
		OrdersCount:    ordersCount,
		RecentOrders:   recentOrders,
		AIUserID:       aiUserID,
		AIVirtualKey:   aiKey,
	}

	h.renderPage(ctx, w, "render admin org detail", pages.AdminOrganizationDetailPage(data, lang, dir))
}

// AdminOrganizationInfoPage redirects to the 360 org detail profile.
func (h *UIHandler) AdminOrganizationInfoPage(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	http.Redirect(w, r, "/admin/organizations/"+idStr, http.StatusMovedPermanently)
}

// AdminOrganizationUsersPage redirects to the 360 org detail profile.
func (h *UIHandler) AdminOrganizationUsersPage(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	http.Redirect(w, r, "/admin/organizations/"+idStr, http.StatusMovedPermanently)
}

// AdminOrganizationBranchesPage redirects to the branches filter for this org.
func (h *UIHandler) AdminOrganizationBranchesPage(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	http.Redirect(w, r, "/admin/branches?org_id="+idStr, http.StatusSeeOther)
}

// AdminOrganizationsPage renders the dedicated organizations management screen.
func (h *UIHandler) AdminOrganizationsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sysCtx := database.AsSystem(ctx)
	lang, dir := h.localeAndDir(r)

	searchQuery := strings.TrimSpace(r.URL.Query().Get("q"))
	typeParam := strings.TrimSpace(r.URL.Query().Get("type"))
	statusParam := strings.TrimSpace(r.URL.Query().Get("status"))

	var orgs []*org.Organization
	branchCounts := make(map[int64]int)
	userCounts := make(map[int64]int)
	var totalOrgs, totalPharmacies, totalVendors, pendingCount, approvedCount int

	if h.orgSvc != nil {
		var filterType *org.OrganizationType
		if typeParam != "" {
			t := org.OrganizationType(typeParam)
			filterType = &t
		}
		var filterStatus *org.OrganizationStatus
		if statusParam != "" {
			s := org.OrganizationStatus(statusParam)
			filterStatus = &s
		}

		allOrgs, _ := h.orgSvc.ListOrganizations(sysCtx, nil, nil, 500, 0)
		for _, o := range allOrgs {
			if o == nil {
				continue
			}
			totalOrgs++
			if o.Type == org.TypeVendor {
				totalVendors++
			} else {
				totalPharmacies++
			}
			if o.Status == org.StatusPending {
				pendingCount++
			} else if o.Status == org.StatusApproved {
				approvedCount++
			}
		}

		allBranches, _ := h.orgSvc.ListBranches(sysCtx, 0)
		for _, b := range allBranches {
			if b != nil {
				branchCounts[b.OrganizationID]++
			}
		}

		list, _ := h.orgSvc.ListOrganizations(sysCtx, filterType, filterStatus, 300, 0)
		for _, o := range list {
			if o == nil {
				continue
			}
			if searchQuery != "" {
				qLower := strings.ToLower(searchQuery)
				nameMatch := strings.Contains(strings.ToLower(o.LegalName), qLower)
				crMatch := strings.Contains(o.CommercialRegister, searchQuery)
				taxMatch := strings.Contains(o.TaxNumber, searchQuery)
				licMatch := strings.Contains(o.PharmacistLicense, searchQuery)
				if !nameMatch && !crMatch && !taxMatch && !licMatch {
					continue
				}
			}
			orgs = append(orgs, o)
		}
	}

	data := pages.AdminOrganizationsPageData{
		Organizations:   orgs,
		TotalOrgs:       totalOrgs,
		TotalPharmacies: totalPharmacies,
		TotalVendors:    totalVendors,
		PendingCount:    pendingCount,
		ApprovedCount:   approvedCount,
		BranchCounts:    branchCounts,
		UserCounts:      userCounts,
		SearchQuery:     searchQuery,
		TypeFilter:      typeParam,
		StatusFilter:    statusParam,
	}

	h.renderPage(ctx, w, "render admin organizations page", pages.AdminOrganizationsPage(data, lang, dir))
}

// AdminBranchesPage renders the dedicated branches and warehouses directory.
func (h *UIHandler) AdminBranchesPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sysCtx := database.AsSystem(ctx)
	lang, dir := h.localeAndDir(r)

	searchQuery := strings.TrimSpace(r.URL.Query().Get("q"))
	statusFilter := strings.TrimSpace(r.URL.Query().Get("status"))
	orgIDFilter, _ := strconv.ParseInt(r.URL.Query().Get("org_id"), 10, 64)

	var branches []*org.Branch
	var allOrgs []*org.Organization
	orgNames := make(map[int64]string)
	orgTypes := make(map[int64]string)
	var totalBranches, activeBranches, pharmacyBranches, vendorWarehouses int

	if h.orgSvc != nil {
		allOrgsList, _ := h.orgSvc.ListOrganizations(sysCtx, nil, nil, 500, 0)
		allOrgs = allOrgsList
		for _, o := range allOrgsList {
			if o != nil {
				orgNames[o.ID] = o.LegalName
				orgTypes[o.ID] = string(o.Type)
			}
		}

		allBranchesList, _ := h.orgSvc.ListBranches(sysCtx, 0)
		for _, b := range allBranchesList {
			if b == nil {
				continue
			}
			totalBranches++
			if b.Status == "active" || b.Status == "" {
				activeBranches++
			}
			if orgTypes[b.OrganizationID] == "vendor" {
				vendorWarehouses++
			} else {
				pharmacyBranches++
			}

			if orgIDFilter > 0 && b.OrganizationID != orgIDFilter {
				continue
			}
			if statusFilter != "" && b.Status != statusFilter {
				continue
			}
			if searchQuery != "" {
				qLower := strings.ToLower(searchQuery)
				nameAr := strings.ToLower(b.Name.Get("ar"))
				nameEn := strings.ToLower(b.Name.Get("en"))
				codeMatch := strings.Contains(strings.ToLower(b.Code), qLower)
				addrMatch := strings.Contains(strings.ToLower(b.Address), qLower)
				orgMatch := strings.Contains(strings.ToLower(orgNames[b.OrganizationID]), qLower)
				if !strings.Contains(nameAr, qLower) && !strings.Contains(nameEn, qLower) && !codeMatch && !addrMatch && !orgMatch {
					continue
				}
			}
			branches = append(branches, b)
		}
	}

	data := pages.AdminBranchesPageData{
		Branches:         branches,
		Organizations:    allOrgs,
		OrgNames:         orgNames,
		OrgTypes:         orgTypes,
		TotalBranches:    totalBranches,
		ActiveBranches:   activeBranches,
		PharmacyBranches: pharmacyBranches,
		VendorWarehouses: vendorWarehouses,
		SearchQuery:      searchQuery,
		SelectedOrgID:    orgIDFilter,
		StatusFilter:     statusFilter,
	}

	h.renderPage(ctx, w, "render admin branches page", pages.AdminBranchesPage(data, lang, dir))
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

	h.renderPage(ctx, w, "render admin branch detail", pages.AdminBranchDetailPage(branch, lang, dir))
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

	h.renderPage(ctx, w, "render admin branch products", pages.AdminBranchDetailPage(branch, lang, dir))
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

	h.renderPage(ctx, w, "render admin branch users", pages.AdminBranchDetailPage(branch, lang, dir))
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

	h.renderPage(ctx, w, "render admin weekly coverages", pages.AdminWeeklyCoveragesPage(data, lang, dir))
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

	h.redirectWithNotice(w, r, "/admin/weekly-coverages?org_id="+redirectOrgID, "success", i18n.T(langOf(r), "admin.org.coverage_created_success"))
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

	h.redirectWithNotice(w, r, "/admin/weekly-coverages?org_id="+orgID, "success", i18n.T(langOf(r), "admin.org.coverage_status_updated_success"))
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

	h.redirectWithNotice(w, r, "/admin/weekly-coverages?org_id="+orgID, "success", i18n.T(langOf(r), "admin.org.coverage_deleted_success"))
}
