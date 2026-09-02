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
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/pagination"
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

	page := pagination.PageNumber(r)
	limit := pagination.RowsPerPage(r)
	offset := (page - 1) * limit

	var orgs []*org.Organization
	var totalMatching int
	var stats org.AdminOrgStatsResult
	var branchCounts map[int64]int
	userCounts := make(map[int64]int)

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

		stats, _ = h.orgSvc.AdminOrgStats(sysCtx)
		branchCounts, _ = h.orgSvc.CountBranchesByOrg(sysCtx)
		orgs, totalMatching, _ = h.orgSvc.ListOrganizationsWithTotal(sysCtx, searchQuery, filterType, filterStatus, limit, offset)
	}

	data := pages.AdminOrganizationsPageData{
		Organizations:   orgs,
		TotalOrgs:       stats.TotalOrgs,
		TotalPharmacies: stats.TotalPharmacies,
		TotalVendors:    stats.TotalVendors,
		PendingCount:    stats.PendingCount,
		ApprovedCount:   stats.ApprovedCount,
		BranchCounts:    branchCounts,
		UserCounts:      userCounts,
		SearchQuery:     searchQuery,
		TypeFilter:      typeParam,
		StatusFilter:    statusParam,
		Page:            page,
		PerPage:         limit,
		TotalCount:      totalMatching,
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
