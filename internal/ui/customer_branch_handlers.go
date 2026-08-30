package ui

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// CustomerBranchCreatePage redirects to the unified branches page in add mode.
func (h *UIHandler) CustomerBranchCreatePage(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/customer/branches", http.StatusSeeOther)
}

// CustomerBranchEditPage redirects to the unified branches page with edit mode preloaded.
func (h *UIHandler) CustomerBranchEditPage(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id != "" {
		http.Redirect(w, r, "/customer/branches?edit="+id, http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/customer/branches", http.StatusSeeOther)
}

// CustomerBranchesPage renders the pharmacy's own branches and employees management screen in CustomerShell.
func (h *UIHandler) CustomerBranchesPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	orgID := actor.OrganizationID
	if orgID <= 0 {
		orgID = actor.OrgID
	}
	if !ok || orgID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/customer/branches", http.StatusSeeOther)
		return
	}

	var branches []*org.Branch
	var employees []*org.EmployeeView
	if h.orgSvc != nil {
		branches, _ = h.orgSvc.ListBranches(ctx, orgID)
		employees, _ = h.orgSvc.ListEmployees(ctx, orgID)
	}

	activeTab := r.URL.Query().Get("tab")
	if activeTab != "employees" {
		activeTab = "branches"
	}

	noticeType := r.URL.Query().Get("notice")
	noticeMsg := r.URL.Query().Get("msg")
	if noticeType == "" {
		noticeType = r.URL.Query().Get("notice_type")
	}
	if noticeMsg == "" {
		noticeMsg = r.URL.Query().Get("notice_msg")
	}

	data := pages.CustomerBranchesData{
		Branches:   branches,
		Employees:  employees,
		Cities:     h.listCities(ctx),
		ActiveTab:  activeTab,
		NoticeType: noticeType,
		NoticeMsg:  noticeMsg,
	}

	h.renderPage(ctx, w, "render customer branches page", pages.CustomerBranches(data, lang, dir, actor.Permissions))
}

// CustomerBranchNewSubmit creates a new pharmacy branch for the customer organization.
func (h *UIHandler) CustomerBranchNewSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/customer/branches", http.StatusSeeOther)
		return
	}

	_ = r.ParseForm()

	nameAr := strings.TrimSpace(r.PostFormValue("name_ar"))
	nameEn := strings.TrimSpace(r.PostFormValue("name_en"))
	if nameAr == "" {
		nameAr = "فرع صيدلية"
	}
	if nameEn == "" {
		nameEn = nameAr
	}
	code := strings.TrimSpace(r.PostFormValue("code"))
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
		OrganizationID: actor.OrganizationID,
		Name:           i18n.New(nameAr, nameEn),
		Code:           code,
		WarehouseType:  "pharmacy",
		Address:        address,
		Phone:          phone,
		OperatingHours: operatingHours,
		HasColdStorage: hasColdStorage,
		GoogleMapsURL:  gmaps,
		CityID:         cityID,
		Latitude:       latPtr,
		Longitude:      lngPtr,
		IsMain:         isMain,
		Status:         "active",
	}

	if h.orgSvc != nil {
		if err := h.orgSvc.CreateBranch(ctx, b); err != nil {
			h.log.ErrorContext(ctx, "customer create branch error", "error", err)
			h.redirectWithNotice(w, r, "/customer/branches", "error", h.safeMessage(err, langOf(r)))
			return
		}
	}

	h.redirectWithNotice(w, r, "/customer/branches", "success", "تم إضافة فرع الصيدلية بنجاح.")
}

// CustomerBranchEditSubmit updates an existing pharmacy branch.
func (h *UIHandler) CustomerBranchEditSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/customer/branches", http.StatusSeeOther)
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/customer/branches", "error", "معرف الفرع غير صالح.")
		return
	}

	if h.orgSvc == nil {
		h.redirectWithNotice(w, r, "/customer/branches", "error", "الخدمة غير متاحة حالياً.")
		return
	}

	existing, err := h.orgSvc.GetBranch(ctx, id)
	if err != nil || existing == nil || existing.OrganizationID != actor.OrganizationID {
		h.redirectWithNotice(w, r, "/customer/branches", "error", "الفرع غير موجود أو لا تملك صلاحية تعديله.")
		return
	}

	_ = r.ParseForm()

	nameAr := strings.TrimSpace(r.PostFormValue("name_ar"))
	nameEn := strings.TrimSpace(r.PostFormValue("name_en"))
	if nameAr == "" {
		nameAr = existing.Name.Get(i18n.AR)
	}
	if nameEn == "" {
		nameEn = nameAr
	}
	code := strings.TrimSpace(r.PostFormValue("code"))
	address := strings.TrimSpace(r.PostFormValue("address"))
	phone := strings.TrimSpace(r.PostFormValue("phone"))
	operatingHours := strings.TrimSpace(r.PostFormValue("operating_hours"))
	gmaps := strings.TrimSpace(r.PostFormValue("google_maps_url"))
	status := strings.TrimSpace(r.PostFormValue("status"))
	if status == "" {
		status = "active"
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
		ID:             id,
		OrganizationID: actor.OrganizationID,
		Name:           i18n.New(nameAr, nameEn),
		Code:           code,
		WarehouseType:  "pharmacy",
		Address:        address,
		Phone:          phone,
		OperatingHours: operatingHours,
		HasColdStorage: hasColdStorage,
		GoogleMapsURL:  gmaps,
		CityID:         cityID,
		Latitude:       latPtr,
		Longitude:      lngPtr,
		IsMain:         isMain,
		Status:         status,
	}

	if err := h.orgSvc.UpdateBranch(ctx, b); err != nil {
		h.log.ErrorContext(ctx, "customer edit branch error", "error", err, "branch_id", id)
		h.redirectWithNotice(w, r, "/customer/branches", "error", h.safeMessage(err, langOf(r)))
		return
	}

	h.redirectWithNotice(w, r, "/customer/branches", "success", "تم تحديث بيانات الفرع بنجاح.")
}

// CustomerBranchDeleteSubmit deletes a branch owned by the customer organization.
func (h *UIHandler) CustomerBranchDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/customer/branches", http.StatusSeeOther)
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/customer/branches", "error", "معرف الفرع غير صالح.")
		return
	}

	if h.orgSvc != nil {
		existing, err := h.orgSvc.GetBranch(ctx, id)
		if err != nil || existing == nil || existing.OrganizationID != actor.OrganizationID {
			h.redirectWithNotice(w, r, "/customer/branches", "error", "الفرع غير موجود أو لا تملك صلاحية حذفه.")
			return
		}
		if existing.IsMain {
			h.redirectWithNotice(w, r, "/customer/branches", "error", "لا يمكن حذف الفرع الرئيسي، يرجى تعيين فرع رئيسي آخر أولاً.")
			return
		}
		if err := h.orgSvc.DeleteBranch(ctx, id, actor.OrganizationID); err != nil {
			h.redirectWithNotice(w, r, "/customer/branches", "error", h.safeMessage(err, langOf(r)))
			return
		}
	}

	h.redirectWithNotice(w, r, "/customer/branches", "success", "تم حذف الفرع بنجاح.")
}

// CustomerSwitchActiveBranchSubmit handles switching the active branch for customer context.
func (h *UIHandler) CustomerSwitchActiveBranchSubmit(w http.ResponseWriter, r *http.Request) {
	h.SetBuyingBranchSubmit(w, r)
}
