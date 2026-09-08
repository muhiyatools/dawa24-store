package ui

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

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

	page := pagination.PageNumber(r)
	limit := pagination.RowsPerPage(r)
	offset := (page - 1) * limit

	var branches []*org.Branch
	var allOrgs []*org.Organization
	orgNames := make(map[int64]string)
	orgTypes := make(map[int64]string)
	var instWorks []*org.InstitutionalWork
	var totalBranches, activeBranches, pharmacyBranches, vendorWarehouses, filteredCount int

	if h.orgSvc != nil {
		allOrgsList, _ := h.orgSvc.ListOrganizations(sysCtx, nil, nil, 500, 0)
		allOrgs = allOrgsList
		for _, o := range allOrgsList {
			if o != nil {
				orgNames[o.ID] = o.LegalName
				orgTypes[o.ID] = string(o.Type)
			}
		}

		stats, _ := h.orgSvc.AdminBranchStats(sysCtx)
		totalBranches = stats.TotalBranches
		activeBranches = stats.ActiveBranches
		pharmacyBranches = stats.PharmacyBranches
		vendorWarehouses = stats.VendorWarehouses

		filter := org.BranchFilter{
			SearchQuery:    searchQuery,
			OrganizationID: orgIDFilter,
			Status:         statusFilter,
		}

		bList, total, err := h.orgSvc.ListBranchesWithTotal(sysCtx, filter, limit, offset)
		if err == nil {
			branches = bList
			filteredCount = total
		}

		instWorks, _ = h.orgSvc.ListAllFlatInstitutionalWorks(sysCtx, true)
	}

	noticeType := r.URL.Query().Get("notice")
	noticeMsg := r.URL.Query().Get("msg")
	if noticeType == "" {
		noticeType = r.URL.Query().Get("notice_type")
	}
	if noticeMsg == "" {
		noticeMsg = r.URL.Query().Get("notice_msg")
	}

	data := pages.AdminBranchesPageData{
		Branches:           branches,
		Organizations:      allOrgs,
		OrgNames:           orgNames,
		OrgTypes:           orgTypes,
		Cities:             h.listCities(ctx),
		InstitutionalWorks: instWorks,
		TotalBranches:      totalBranches,
		FilteredCount:      filteredCount,
		ActiveBranches:     activeBranches,
		PharmacyBranches:   pharmacyBranches,
		VendorWarehouses:   vendorWarehouses,
		Page:               page,
		PerPage:            limit,
		SearchQuery:        searchQuery,
		SelectedOrgID:      orgIDFilter,
		StatusFilter:       statusFilter,
		NoticeType:         noticeType,
		NoticeMsg:          noticeMsg,
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

// AdminBranchNewSubmit creates a new branch or warehouse for any organization.
func (h *UIHandler) AdminBranchNewSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	if h.orgSvc == nil {
		h.redirectWithNotice(w, r, "/admin/branches", "error", i18n.T(lang, "customer.branch.service_unavailable"))
		return
	}

	_ = r.ParseForm()

	orgID, err := strconv.ParseInt(r.PostFormValue("org_id"), 10, 64)
	if err != nil || orgID <= 0 {
		h.redirectWithNotice(w, r, "/admin/branches", "error", "يرجى تحديد المنشأة التابع لها الفرع")
		return
	}

	var orgType string
	if targetOrg, err := h.orgSvc.GetOrganization(database.AsSystem(ctx), orgID); err == nil && targetOrg != nil {
		orgType = string(targetOrg.Type)
	}

	nameAr := strings.TrimSpace(r.PostFormValue("name_ar"))
	nameEn := strings.TrimSpace(r.PostFormValue("name_en"))
	if nameAr == "" {
		if orgType == string(org.TypeCustomer) {
			nameAr = "فرع صيدلية جديد"
		} else {
			nameAr = "مستودع جديد"
		}
	}
	if nameEn == "" {
		nameEn = nameAr
	}
	code := strings.TrimSpace(r.PostFormValue("code"))
	if code == "" {
		code = fmt.Sprintf("BR-%d", time.Now().Unix())
	}
	warehouseType := strings.TrimSpace(r.PostFormValue("warehouse_type"))
	if orgType == string(org.TypeCustomer) {
		warehouseType = "pharmacy"
	} else {
		if warehouseType == "" || warehouseType == "branch" || warehouseType == "pharmacy" {
			warehouseType = "warehouse"
		}
	}
	capacitySQM, _ := strconv.ParseFloat(r.PostFormValue("capacity_sqm"), 64)
	address := strings.TrimSpace(r.PostFormValue("address"))
	phone := strings.TrimSpace(r.PostFormValue("phone"))
	operatingHours := strings.TrimSpace(r.PostFormValue("operating_hours"))
	gmaps := strings.TrimSpace(r.PostFormValue("google_maps_url"))
	isMain := r.PostFormValue("is_main") == "true" || r.PostFormValue("is_main") == "on" || r.PostFormValue("is_main") == "1"
	hasColdStorage := r.PostFormValue("has_cold_storage") == "true" || r.PostFormValue("has_cold_storage") == "on" || r.PostFormValue("has_cold_storage") == "1"

	cityIDVal, _ := strconv.ParseInt(r.PostFormValue("city_id"), 10, 64)
	var cityID *int64
	if cityIDVal > 0 {
		cityID = &cityIDVal
	}

	var latPtr, lngPtr *float64
	if latStr := r.PostFormValue("latitude"); latStr != "" {
		if lat, err := strconv.ParseFloat(latStr, 64); err == nil {
			latPtr = &lat
		}
	}
	if lngStr := r.PostFormValue("longitude"); lngStr != "" {
		if lng, err := strconv.ParseFloat(lngStr, 64); err == nil {
			lngPtr = &lng
		}
	}

	b := &org.Branch{
		OrganizationID:     orgID,
		Name:               i18n.New(nameAr, nameEn),
		Code:               code,
		WarehouseType:      warehouseType,
		CapacitySQM:        capacitySQM,
		Address:            address,
		Phone:              phone,
		OperatingHours:     operatingHours,
		HasColdStorage:     hasColdStorage,
		GoogleMapsURL:      gmaps,
		CityID:             cityID,
		Latitude:           latPtr,
		Longitude:          lngPtr,
		IsMain:             isMain,
		Status:             "active",
		InstitutionalWorks: r.Form["institutional_works"],
	}

	if err := h.orgSvc.CreateBranch(database.AsSystem(ctx), b); err != nil {
		h.log.ErrorContext(ctx, "admin create branch error", "error", err)
		h.redirectWithNotice(w, r, "/admin/branches", "error", h.safeMessage(err, lang))
		return
	}
	InvalidateBranchOptionsCache(orgID)

	h.redirectWithNotice(w, r, "/admin/branches", "success", "تمت إضافة الفرع بنجاح.")
}

// AdminBranchEditSubmit updates an existing branch.
func (h *UIHandler) AdminBranchEditSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	if h.orgSvc == nil {
		h.redirectWithNotice(w, r, "/admin/branches", "error", i18n.T(lang, "customer.branch.service_unavailable"))
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/admin/branches", "error", "معرف الفرع غير صالح")
		return
	}

	existing, err := h.orgSvc.GetBranch(database.AsSystem(ctx), id)
	if err != nil || existing == nil {
		h.redirectWithNotice(w, r, "/admin/branches", "error", "لم يتم العثور على الفرع المطلوب")
		return
	}

	_ = r.ParseForm()

	orgID, _ := strconv.ParseInt(r.PostFormValue("org_id"), 10, 64)
	if orgID <= 0 {
		orgID = existing.OrganizationID
	}

	var orgType string
	if targetOrg, err := h.orgSvc.GetOrganization(database.AsSystem(ctx), orgID); err == nil && targetOrg != nil {
		orgType = string(targetOrg.Type)
	}

	nameAr := strings.TrimSpace(r.PostFormValue("name_ar"))
	nameEn := strings.TrimSpace(r.PostFormValue("name_en"))
	if nameAr == "" {
		nameAr = existing.Name.Get(i18n.AR)
	}
	if nameEn == "" {
		nameEn = nameAr
	}
	code := strings.TrimSpace(r.PostFormValue("code"))
	if code == "" {
		code = existing.Code
	}
	warehouseType := strings.TrimSpace(r.PostFormValue("warehouse_type"))
	if orgType == string(org.TypeCustomer) {
		warehouseType = "pharmacy"
	} else {
		if warehouseType == "" {
			warehouseType = existing.WarehouseType
		}
		if warehouseType == "" || warehouseType == "branch" || warehouseType == "pharmacy" {
			warehouseType = "warehouse"
		}
	}
	capSQMVal := strings.TrimSpace(r.PostFormValue("capacity_sqm"))
	var capacitySQM float64
	if capSQMVal != "" {
		capacitySQM, _ = strconv.ParseFloat(capSQMVal, 64)
	} else if orgType != string(org.TypeCustomer) {
		capacitySQM = existing.CapacitySQM
	}
	address := strings.TrimSpace(r.PostFormValue("address"))
	if address == "" {
		address = existing.Address
	}
	phone := strings.TrimSpace(r.PostFormValue("phone"))
	if phone == "" {
		phone = existing.Phone
	}
	operatingHours := strings.TrimSpace(r.PostFormValue("operating_hours"))
	gmaps := strings.TrimSpace(r.PostFormValue("google_maps_url"))
	status := strings.TrimSpace(r.PostFormValue("status"))
	if status == "" {
		status = existing.Status
	}
	isMain := r.PostFormValue("is_main") == "true" || r.PostFormValue("is_main") == "on" || r.PostFormValue("is_main") == "1"
	hasColdStorage := r.PostFormValue("has_cold_storage") == "true" || r.PostFormValue("has_cold_storage") == "on" || r.PostFormValue("has_cold_storage") == "1"

	cityIDVal, _ := strconv.ParseInt(r.PostFormValue("city_id"), 10, 64)
	var cityID *int64
	if cityIDVal > 0 {
		cityID = &cityIDVal
	} else {
		cityID = existing.CityID
	}

	var latPtr, lngPtr *float64
	if latStr := r.PostFormValue("latitude"); latStr != "" {
		if lat, err := strconv.ParseFloat(latStr, 64); err == nil {
			latPtr = &lat
		}
	}
	if lngStr := r.PostFormValue("longitude"); lngStr != "" {
		if lng, err := strconv.ParseFloat(lngStr, 64); err == nil {
			lngPtr = &lng
		}
	}
	if latPtr == nil {
		latPtr = existing.Latitude
	}
	if lngPtr == nil {
		lngPtr = existing.Longitude
	}

	b := &org.Branch{
		ID:                 id,
		OrganizationID:     orgID,
		Name:               i18n.New(nameAr, nameEn),
		Code:               code,
		WarehouseType:      warehouseType,
		CapacitySQM:        capacitySQM,
		Address:            address,
		Phone:              phone,
		OperatingHours:     operatingHours,
		HasColdStorage:     hasColdStorage,
		GoogleMapsURL:      gmaps,
		CityID:             cityID,
		Latitude:           latPtr,
		Longitude:          lngPtr,
		IsMain:             isMain,
		Status:             status,
		InstitutionalWorks: r.Form["institutional_works"],
	}

	if err := h.orgSvc.UpdateBranch(database.AsSystem(ctx), b); err != nil {
		h.log.ErrorContext(ctx, "admin update branch error", "error", err, "branch_id", id)
		h.redirectWithNotice(w, r, "/admin/branches", "error", h.safeMessage(err, lang))
		return
	}
	InvalidateBranchOptionsCache(orgID)

	h.redirectWithNotice(w, r, "/admin/branches", "success", "تم تحديث بيانات الفرع بنجاح.")
}

// AdminBranchDeleteSubmit removes a branch.
func (h *UIHandler) AdminBranchDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	if h.orgSvc == nil {
		h.redirectWithNotice(w, r, "/admin/branches", "error", i18n.T(lang, "customer.branch.service_unavailable"))
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/admin/branches", "error", "معرف الفرع غير صالح")
		return
	}

	existing, err := h.orgSvc.GetBranch(database.AsSystem(ctx), id)
	if err != nil || existing == nil {
		h.redirectWithNotice(w, r, "/admin/branches", "error", "لم يتم العثور على الفرع المطلوب")
		return
	}
	if existing.IsMain {
		h.redirectWithNotice(w, r, "/admin/branches", "error", "لا يمكن حذف الفرع الرئيسي للمنشأة")
		return
	}

	if err := h.orgSvc.DeleteBranch(database.AsSystem(ctx), id, existing.OrganizationID); err != nil {
		h.log.ErrorContext(ctx, "admin delete branch error", "error", err, "branch_id", id)
		h.redirectWithNotice(w, r, "/admin/branches", "error", h.safeMessage(err, lang))
		return
	}
	InvalidateBranchOptionsCache(existing.OrganizationID)

	h.redirectWithNotice(w, r, "/admin/branches", "success", "تم حذف الفرع بنجاح.")
}
