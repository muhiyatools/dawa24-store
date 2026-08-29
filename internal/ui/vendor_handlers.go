package ui

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/xuri/excelize/v2"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/modules/identity"
	"github.com/muhiya/dawa24-store/internal/modules/inventory"

	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/modules/promo"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// VendorProductsPage renders one page of the vendor's supply variants.
//
// Everything is paged, searched and filtered at the database. The screen this
// replaces asked for five hundred variants, rendered all of them, and filtered
// them in the browser — so a vendor with nine thousand could not reach the
// other eight and a half thousand, and the counters above the table told them
// they owned five hundred.
func (h *UIHandler) VendorProductsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/products", http.StatusSeeOther)
		return
	}

	branchOptions, branchMap := h.vendorBranchOptions(ctx, actor.OrganizationID)
	query := vendorVariantQueryFrom(r)

	data := pages.VendorVariantsData{
		Branches:   branchOptions,
		Filter:     query,
		PageSizes:  catalog.PageSizes,
		NoticeType: r.URL.Query().Get("notice_type"),
		NoticeMsg:  r.URL.Query().Get("notice"),
	}

	if h.catSvc != nil && actor.OrganizationID > 0 {
		variants, total, err := h.catSvc.ListVendorVariants(ctx, actor.OrganizationID, query)
		if err != nil {
			h.log.ErrorContext(ctx, "list vendor variants", "error", err)
			data.LoadError = h.safeMessage(err, langOf(r))
		} else {
			data.Total = total
			data.Variants = h.decorateVendorVariants(ctx, variants, branchMap)
		}

		stats, statsErr := h.catSvc.VendorVariantStats(ctx, actor.OrganizationID)
		if statsErr != nil {
			h.log.WarnContext(ctx, "vendor variant stats unavailable", "error", statsErr)
		} else {
			data.Stats = stats
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.VendorProducts(data, lang, dir, h.isHTMX(r)).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render vendor products page", "error", err)
	}
}

// vendorVariantQueryFrom reads the listing controls out of the URL.
//
// Every value is clamped rather than trusted: the page size to the offered
// sizes, the page to at least one, so a hand-edited link cannot ask for a
// hundred thousand rows in one response.
func vendorVariantQueryFrom(r *http.Request) catalog.VendorVariantQuery {
	q := r.URL.Query()
	query := catalog.VendorVariantQuery{
		Query:      strings.TrimSpace(q.Get("q")),
		Status:     q.Get("status"),
		Stock:      catalog.StockFilter(q.Get("stock")),
		Expiring:   q.Get("expiring") == "1",
		Sort:       q.Get("sort"),
		PageNumber: 1,
		PerPage:    catalog.DefaultPageSize,
	}
	if n, err := strconv.Atoi(q.Get("page")); err == nil && n > 1 {
		query.PageNumber = n
	}
	if n, err := strconv.Atoi(q.Get("limit")); err == nil {
		query.PerPage = n
	}
	switch query.Status {
	case string(catalog.StatusActive), string(catalog.StatusInactive),
		string(catalog.StatusPending), string(catalog.StatusRejected):
	default:
		query.Status = ""
	}
	switch query.Stock {
	case catalog.StockFilterIn, catalog.StockFilterLow, catalog.StockFilterOut:
	default:
		query.Stock = catalog.StockFilterAny
	}
	return query
}

// vendorBranchOptions lists the vendor's branches and a lookup for the table.
func (h *UIHandler) vendorBranchOptions(
	ctx context.Context, orgID int64,
) ([]pages.VendorBranchOption, map[int64]string) {
	names := make(map[int64]string)
	if h.orgSvc == nil || orgID <= 0 {
		return nil, names
	}
	branches, err := h.orgSvc.ListBranches(ctx, orgID)
	if err != nil {
		h.log.WarnContext(ctx, "vendor branches unavailable", "error", err)
		return nil, names
	}
	var options []pages.VendorBranchOption
	for _, b := range branches {
		name := b.Name.Get(i18n.AR)
		if name == "" {
			name = b.Name.Get(i18n.EN)
		}
		options = append(options, pages.VendorBranchOption{ID: b.ID, Name: name, IsMain: b.IsMain})
		names[b.ID] = name
	}
	return options, names
}

// decorateVendorVariants attaches the shared-catalogue product and the branch
// name to each row.
//
// The products are fetched in one query for the whole page. Fetching them per
// row is a hundred round trips for a hundred-row page, which is what the
// previous version did behind a cache that only helped when two variants of the
// same product happened to land on the same screen.
func (h *UIHandler) decorateVendorVariants(
	ctx context.Context, variants []*catalog.ProductVariant, branchMap map[int64]string,
) []*pages.VendorVariantView {
	ids := make([]int64, 0, len(variants))
	seen := make(map[int64]bool, len(variants))
	for _, v := range variants {
		if v.ProductID > 0 && !seen[v.ProductID] {
			seen[v.ProductID] = true
			ids = append(ids, v.ProductID)
		}
	}
	products, err := h.catSvc.ProductsByIDs(ctx, ids)
	if err != nil {
		h.log.WarnContext(ctx, "master products unavailable for vendor listing", "error", err)
		products = map[int64]*catalog.Product{}
	}

	out := make([]*pages.VendorVariantView, 0, len(variants))
	for _, v := range variants {
		branch := "المستودع الرئيسي"
		if v.BranchID != nil {
			if name, ok := branchMap[*v.BranchID]; ok {
				branch = name
			}
		}
		out = append(out, &pages.VendorVariantView{
			Variant:       v,
			MasterProduct: products[v.ProductID],
			BranchName:    branch,
			StockQuantity: v.StockQty,
		})
	}
	return out
}

// VendorVariantNewPage renders the variant creation form with master product selector.
func (h *UIHandler) VendorVariantNewPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/variants/new", http.StatusSeeOther)
		return
	}

	var masterProducts []*catalog.Product
	if h.catSvc != nil {
		masterProducts, _ = h.catSvc.Search(ctx, catalog.SearchParams{Limit: 200})
	}

	var branches []*org.Branch
	if h.orgSvc != nil && actor.OrganizationID > 0 {
		branches, _ = h.orgSvc.ListBranches(ctx, actor.OrganizationID)
	}

	selectedProdID, _ := strconv.ParseInt(r.URL.Query().Get("product_id"), 10, 64)

	data := pages.VendorVariantEditorData{
		MasterProducts: masterProducts,
		Branches:       branches,
		SelectedProdID: selectedProdID,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.VendorProductEditor(data, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render new variant page", "error", err)
	}
}

// VendorVariantNewSubmit processes vendor variant creation.
func (h *UIHandler) VendorVariantNewSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/products", http.StatusSeeOther)
		return
	}

	prodID, _ := strconv.ParseInt(r.PostFormValue("product_id"), 10, 64)
	nameAr := r.PostFormValue("name_ar")
	nameEn := r.PostFormValue("name_en")
	batch := r.PostFormValue("batch_number")
	priceStr := r.PostFormValue("price")
	costStr := r.PostFormValue("cost_price")
	discountStr := r.PostFormValue("discount")
	stockQty, _ := strconv.Atoi(r.PostFormValue("stock_qty"))
	minQty, _ := strconv.Atoi(r.PostFormValue("min_order_qty"))
	branchIDVal, _ := strconv.ParseInt(r.PostFormValue("branch_id"), 10, 64)
	sku := r.PostFormValue("sku")

	if minQty <= 0 {
		minQty = 1
	}

	var branchID *int64
	if branchIDVal > 0 {
		branchID = &branchIDVal
	} else if h.orgSvc != nil {
		if branches, err := h.orgSvc.ListBranches(ctx, actor.OrganizationID); err == nil && len(branches) > 0 {
			for _, b := range branches {
				if b.IsMain {
					branchID = &b.ID
					break
				}
			}
			if branchID == nil {
				branchID = &branches[0].ID
			}
		}
	}

	var expiryDate *time.Time
	if expStr := r.PostFormValue("expiry_date"); expStr != "" {
		if t, err := time.Parse("2006-01-02", expStr); err == nil {
			expiryDate = &t
		}
	}

	price, _ := money.Parse(priceStr)
	cost, _ := money.Parse(costStr)
	discount, _ := money.Parse(discountStr)
	isNegotiable := r.PostFormValue("is_negotiable") == "true" || r.PostFormValue("is_negotiable") == "1"

	variant := &catalog.ProductVariant{
		OrganizationID: actor.OrganizationID,
		ProductID:      prodID,
		Name:           i18n.New(nameAr, nameEn),
		BatchNumber:    batch,
		ExpiryDate:     expiryDate,
		Price:          price,
		CostPrice:      cost,
		Discount:       discount,
		StockQty:       stockQty,
		MinOrderQty:    minQty,
		BranchID:       branchID,
		SKU:            sku,
		IsNegotiable:   isNegotiable,
		Status:         catalog.StatusActive,
	}

	if h.catSvc == nil {
		h.redirectWithNotice(w, r, "/vendor/variants/new", "error", "خدمة الكتالوج غير متاحة حالياً.")
		return
	}
	created, err := h.catSvc.CreateVariant(ctx, variant)
	if err != nil {
		h.log.ErrorContext(ctx, "create variant error", "error", err)
		h.redirectWithNotice(w, r, "/vendor/variants/new", "error", h.safeMessage(err, langOf(r)))
		return
	}

	// The stock number the vendor typed does not live on the variant —
	// catalog.product_variants has no stock column. It belongs in
	// inventory.stocks against a warehouse. Writing it there is the difference
	// between "50 in stock" being real and being silently discarded, which is
	// what happened before.
	if stockQty > 0 && created != nil {
		if err := h.recordInitialStock(ctx, actor.OrganizationID, created, stockQty); err != nil {
			h.log.WarnContext(ctx, "variant created but its opening stock could not be recorded",
				"error", err, "variant", created.ID, "org", actor.OrganizationID, "qty", stockQty)
			h.redirectWithNotice(w, r, "/vendor/products", "error",
				"تم نشر الصنف، لكن تعذر تسجيل الكمية الافتتاحية. يرجى إضافة مستودع أولاً ثم تحديد الكمية من صفحة المخزون.")
			return
		}
	}

	h.redirectWithNotice(w, r, "/vendor/products", "success", "تم نشر عرض التوريد بنجاح في الكتالوج.")
}

// VendorVariantDeleteSubmit removes a supplier's variant offer and clears associated warehouse stocks.
func (h *UIHandler) VendorVariantDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/products", http.StatusSeeOther)
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		h.renderError(w, r, apperr.Validation("id.invalid", "Invalid variant ID", nil))
		return
	}

	if h.catSvc != nil {
		ctx = database.WithTenant(ctx, actor.OrganizationID)
		if err := h.catSvc.DeleteVariant(ctx, id); err != nil {
			h.redirectWithNotice(w, r, "/vendor/products", "error", "تعذر حذف الصنف: "+h.safeMessage(err, langOf(r)))
			return
		}
	}

	h.redirectWithNotice(w, r, "/vendor/products", "success", "تم حذف الصنف ومسح الأرصدة المرتبطة به بالمخازن بنجاح.")
}

// VendorBranchesPage renders the detailed branch management view.
func (h *UIHandler) VendorBranchesPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/branches", http.StatusSeeOther)
		return
	}

	var branches []*org.Branch
	var employees []*org.EmployeeView
	if h.orgSvc != nil && actor.OrganizationID > 0 {
		branches, _ = h.orgSvc.ListBranches(ctx, actor.OrganizationID)
		employees, _ = h.orgSvc.ListEmployees(ctx, actor.OrganizationID)
	}

	data := pages.VendorBranchesData{
		Branches:  branches,
		Cities:    h.listCities(ctx),
		Employees: employees,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.VendorBranchesPage(data, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render vendor branches page", "error", err)
	}
}

// VendorBranchNewSubmit creates a new physical branch or warehouse.
func (h *UIHandler) VendorBranchNewSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/branches", http.StatusSeeOther)
		return
	}

	if err := r.ParseForm(); err != nil {
		h.log.WarnContext(ctx, "parse form error", "error", err)
	}

	nameAr := r.PostFormValue("name_ar")
	nameEn := r.PostFormValue("name_en")
	if nameEn == "" {
		nameEn = nameAr
	}
	code := r.PostFormValue("code")
	warehouseType := r.PostFormValue("warehouse_type")
	address := r.PostFormValue("address")
	phone := r.PostFormValue("phone")
	gmaps := r.PostFormValue("google_maps_url")
	hours := r.PostFormValue("operating_hours")
	hasCold := r.PostFormValue("has_cold_storage") == "true"
	isMain := r.PostFormValue("is_main") == "true"

	managerIDVal, _ := strconv.ParseInt(r.PostFormValue("manager_id"), 10, 64)
	var managerID *int64
	if managerIDVal > 0 {
		managerID = &managerIDVal
	}

	cityIDVal, _ := strconv.ParseInt(r.PostFormValue("city_id"), 10, 64)
	var cityID *int64
	if cityIDVal > 0 {
		cityID = &cityIDVal
	}

	capSQM, _ := strconv.ParseFloat(r.PostFormValue("capacity_sqm"), 64)

	var latPtr, lngPtr *float64
	latStr := r.PostFormValue("latitude")
	if latStr == "" {
		latStr = r.PostFormValue("branch_lat")
	}
	if latStr != "" {
		if lat, err := strconv.ParseFloat(latStr, 64); err == nil {
			latPtr = &lat
		}
	}

	lngStr := r.PostFormValue("longitude")
	if lngStr == "" {
		lngStr = r.PostFormValue("branch_lon")
	}
	if lngStr != "" {
		if lng, err := strconv.ParseFloat(lngStr, 64); err == nil {
			lngPtr = &lng
		}
	}

	if gmaps == "" {
		gmaps = r.PostFormValue("branch_google_maps_url")
	}

	instWorks := r.Form["institutional_works"]

	var managerName string
	if managerID != nil && h.idSvc != nil {
		if u, err := h.idSvc.GetUserByID(ctx, *managerID); err == nil && u != nil {
			managerName = u.Name.Get("ar")
			if managerName == "" {
				managerName = u.Name.Get("en")
			}
			if managerName == "" {
				managerName = u.Email
			}
		}
	}

	b := &org.Branch{
		OrganizationID:     actor.OrganizationID,
		Name:               i18n.New(nameAr, nameEn),
		Code:               code,
		WarehouseType:      warehouseType,
		Address:            address,
		Phone:              phone,
		ManagerID:          managerID,
		ManagerName:        managerName,
		GoogleMapsURL:      gmaps,
		OperatingHours:     hours,
		HasColdStorage:     hasCold,
		CapacitySQM:        capSQM,
		CityID:             cityID,
		Latitude:           latPtr,
		Longitude:          lngPtr,
		IsMain:             isMain,
		Status:             "active",
		InstitutionalWorks: instWorks,
	}

	if h.orgSvc != nil {
		if err := h.orgSvc.CreateBranch(ctx, b); err != nil {
			h.log.ErrorContext(ctx, "create branch error", "error", err)
			h.redirectWithNotice(w, r, "/vendor/branches", "error", h.safeMessage(err, langOf(r)))
			return
		}
	}

	h.redirectWithNotice(w, r, "/vendor/branches", "success", "تم إضافة الفرع ونقطة التوزيع بنجاح.")
}

// VendorBranchEditPage renders the branch edit form with map and full parameters.
func (h *UIHandler) VendorBranchEditPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok || !actor.IsVendor() {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/branches", http.StatusSeeOther)
		return
	}

	branchID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || branchID <= 0 {
		h.redirectWithNotice(w, r, "/vendor/branches", "error", "معرف فرع غير صالح.")
		return
	}

	if h.orgSvc == nil {
		h.redirectWithNotice(w, r, "/vendor/branches", "error", "خدمة المنظمة غير متوفرة.")
		return
	}

	branch, err := h.orgSvc.GetBranch(ctx, branchID)
	if err != nil || branch == nil || branch.OrganizationID != actor.OrganizationID {
		h.redirectWithNotice(w, r, "/vendor/branches", "error", "الفرع غير موجود أو غير مصرح لك بتعديله.")
		return
	}

	employees, _ := h.orgSvc.ListEmployees(ctx, actor.OrganizationID)

	data := pages.VendorBranchFormData{
		Branch:     branch,
		Cities:     h.listCities(ctx),
		Employees:  employees,
		IsEdit:     true,
		NoticeType: r.URL.Query().Get("notice_type"),
		NoticeMsg:  r.URL.Query().Get("notice_msg"),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.VendorBranchFormPage(data, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render vendor branch edit page", "error", err)
	}
}

// VendorBranchEditSubmit saves updates to an existing branch.
func (h *UIHandler) VendorBranchEditSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || !actor.IsVendor() {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/branches", http.StatusSeeOther)
		return
	}

	branchID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || branchID <= 0 {
		h.redirectWithNotice(w, r, "/vendor/branches", "error", "معرف فرع غير صالح.")
		return
	}

	if h.orgSvc == nil {
		h.redirectWithNotice(w, r, "/vendor/branches", "error", "خدمة المنظمة غير متوفرة.")
		return
	}

	existing, err := h.orgSvc.GetBranch(ctx, branchID)
	if err != nil || existing == nil || existing.OrganizationID != actor.OrganizationID {
		h.redirectWithNotice(w, r, "/vendor/branches", "error", "الفرع غير موجود أو غير مصرح لك بتعديله.")
		return
	}

	if err := r.ParseForm(); err != nil {
		h.log.WarnContext(ctx, "parse form error", "error", err)
	}

	nameAr := strings.TrimSpace(r.PostFormValue("name_ar"))
	nameEn := strings.TrimSpace(r.PostFormValue("name_en"))
	if nameAr == "" {
		h.redirectWithNotice(w, r, fmt.Sprintf("/vendor/branches/%d/edit", branchID), "error", "اسم الفرع بالعربية مطلوب.")
		return
	}
	if nameEn == "" {
		nameEn = nameAr
	}

	code := strings.TrimSpace(r.PostFormValue("code"))
	warehouseType := strings.TrimSpace(r.PostFormValue("warehouse_type"))
	if warehouseType == "" {
		warehouseType = "warehouse"
	}
	address := strings.TrimSpace(r.PostFormValue("address"))
	phone := strings.TrimSpace(r.PostFormValue("phone"))
	gmaps := strings.TrimSpace(r.PostFormValue("google_maps_url"))
	hours := strings.TrimSpace(r.PostFormValue("operating_hours"))
	hasCold := r.PostFormValue("has_cold_storage") == "true"
	isMain := r.PostFormValue("is_main") == "true"
	status := strings.TrimSpace(r.PostFormValue("status"))
	if status == "" {
		status = "active"
	}

	managerIDVal, _ := strconv.ParseInt(r.PostFormValue("manager_id"), 10, 64)
	var managerID *int64
	if managerIDVal > 0 {
		managerID = &managerIDVal
	}

	cityIDVal, _ := strconv.ParseInt(r.PostFormValue("city_id"), 10, 64)
	var cityID *int64
	if cityIDVal > 0 {
		cityID = &cityIDVal
	}

	capSQM, _ := strconv.ParseFloat(r.PostFormValue("capacity_sqm"), 64)

	var latPtr, lngPtr *float64
	latStr := strings.TrimSpace(r.PostFormValue("latitude"))
	if latStr == "" {
		latStr = strings.TrimSpace(r.PostFormValue("branch_lat"))
	}
	if latStr != "" {
		if lat, err := strconv.ParseFloat(latStr, 64); err == nil {
			latPtr = &lat
		}
	}

	lngStr := strings.TrimSpace(r.PostFormValue("longitude"))
	if lngStr == "" {
		lngStr = strings.TrimSpace(r.PostFormValue("branch_lon"))
	}
	if lngStr != "" {
		if lng, err := strconv.ParseFloat(lngStr, 64); err == nil {
			lngPtr = &lng
		}
	}

	if gmaps == "" {
		gmaps = strings.TrimSpace(r.PostFormValue("branch_google_maps_url"))
	}

	instWorks := r.Form["institutional_works"]

	var managerName string
	if managerID != nil && h.idSvc != nil {
		if u, err := h.idSvc.GetUserByID(ctx, *managerID); err == nil && u != nil {
			managerName = u.Name.Get("ar")
			if managerName == "" {
				managerName = u.Name.Get("en")
			}
			if managerName == "" {
				managerName = u.Email
			}
		}
	}

	existing.Name = i18n.New(nameAr, nameEn)
	existing.Code = code
	existing.WarehouseType = warehouseType
	existing.Address = address
	existing.Phone = phone
	existing.ManagerID = managerID
	existing.ManagerName = managerName
	existing.GoogleMapsURL = gmaps
	existing.OperatingHours = hours
	existing.HasColdStorage = hasCold
	existing.CapacitySQM = capSQM
	existing.CityID = cityID
	existing.Latitude = latPtr
	existing.Longitude = lngPtr
	existing.IsMain = isMain
	existing.Status = status
	existing.InstitutionalWorks = instWorks

	if err := h.orgSvc.UpdateBranch(ctx, existing); err != nil {
		h.log.ErrorContext(ctx, "update branch error", "error", err)
		h.redirectWithNotice(w, r, fmt.Sprintf("/vendor/branches/%d/edit", branchID), "error", h.safeMessage(err, langOf(r)))
		return
	}

	h.redirectWithNotice(w, r, "/vendor/branches", "success", "تم تحديث وحفظ بيانات الفرع بنجاح.")
}

// VendorBranchDeleteSubmit deletes a branch scoped to the vendor organization.
func (h *UIHandler) VendorBranchDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/vendor/branches", "error", "معرف الفرع غير صالح.")
		return
	}
	if h.orgSvc == nil {
		h.redirectWithNotice(w, r, "/vendor/branches", "error", "خدمة المؤسسات غير متوفرة.")
		return
	}
	if err := h.orgSvc.DeleteBranch(ctx, id, actor.OrganizationID); err != nil {
		h.log.ErrorContext(ctx, "delete branch", "error", err, "branch", id, "org", actor.OrganizationID)
		h.redirectWithNotice(w, r, "/vendor/branches", "error", h.safeMessage(err, langOf(r)))
		return
	}
	h.redirectWithNotice(w, r, "/vendor/branches", "success", "تم حذف الفرع بنجاح.")
}

// VendorTeamPage renders the staff and RBAC roles configuration view.
func (h *UIHandler) VendorTeamPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/team", http.StatusSeeOther)
		return
	}

	noticeType := r.URL.Query().Get("notice_type")
	if noticeType == "" {
		noticeType = r.URL.Query().Get("notice")
	}
	noticeMsg := r.URL.Query().Get("notice_msg")
	if noticeMsg == "" {
		noticeMsg = r.URL.Query().Get("msg")
	}

	var memberViews []*pages.TeamMemberView
	var branchOptions []*pages.BranchOption

	if h.orgSvc != nil && actor.OrganizationID > 0 {
		// 1. Fetch employees with full profiles
		if employees, err := h.orgSvc.ListEmployees(ctx, actor.OrganizationID); err == nil && len(employees) > 0 {
			for _, emp := range employees {
				roleName := emp.RoleName
				switch emp.Member.RoleKey {
				case "org_owner":
					roleName = "مالك المنشأة"
				case "org_manager":
					roleName = "مدير عمليات"
				case "org_warehouse":
					roleName = "أمين مخزن"
				case "org_accountant":
					roleName = "محاسب مالي"
				case "org_employee":
					roleName = "موظف مبيعات وتوريد"
				default:
					if roleName == "" {
						roleName = "عضو فريق العمل"
					}
				}
				if emp.IsManager && emp.Member.RoleKey != "org_owner" {
					roleName = "مدير فرع / عمليات"
				}

				name := emp.UserName
				if name == "" {
					name = emp.UserEmail
				}

				memberViews = append(memberViews, &pages.TeamMemberView{
					ID:           emp.Member.ID,
					UserID:       emp.Member.UserID,
					Name:         name,
					Email:        emp.UserEmail,
					Phone:        emp.UserPhone,
					JobTitle:     emp.Member.JobTitle,
					EmployeeCode: emp.Member.EmployeeCode,
					BranchName:   emp.BranchName,
					RoleKey:      emp.Member.RoleKey,
					RoleName:     roleName,
					IsActive:     emp.Member.IsActive,
					CreatedAt:    emp.Member.CreatedAt.Format("2006-01-02"),
				})
			}
		} else {
			// Fallback to ListMembers if ListEmployees returns empty
			members, _ := h.orgSvc.ListMembers(ctx, actor.OrganizationID)
			for _, m := range members {
				name := "موظف"
				email := ""
				phone := ""
				if h.idSvc != nil {
					if u, err := h.idSvc.GetUserByID(ctx, m.UserID); err == nil && u != nil {
						name = u.Name.Get(i18n.AR)
						if name == "" {
							name = u.Name.Get(i18n.EN)
						}
						if name == "" {
							name = u.Email
						}
						email = u.Email
						phone = u.Phone
					}
				}
				roleName := "موظف مبيعات وتوريد"
				switch m.RoleKey {
				case "org_owner":
					roleName = "مالك المنشأة"
				case "org_manager":
					roleName = "مدير عمليات"
				case "org_warehouse":
					roleName = "أمين مخزن"
				case "org_accountant":
					roleName = "محاسب مالي"
				case "org_employee":
					roleName = "موظف مبيعات وتوريد"
				}
				memberViews = append(memberViews, &pages.TeamMemberView{
					ID:           m.ID,
					UserID:       m.UserID,
					Name:         name,
					Email:        email,
					Phone:        phone,
					JobTitle:     m.JobTitle,
					EmployeeCode: m.EmployeeCode,
					RoleKey:      m.RoleKey,
					RoleName:     roleName,
					IsActive:     m.IsActive,
					CreatedAt:    m.CreatedAt.Format("2006-01-02"),
				})
			}
		}

		// 2. Fetch branches for the branch dropdown
		if branches, err := h.orgSvc.ListBranches(ctx, actor.OrganizationID); err == nil {
			for _, b := range branches {
				bName := b.Name.Get(i18n.AR)
				if bName == "" {
					bName = b.Name.Get(i18n.EN)
				}
				branchOptions = append(branchOptions, &pages.BranchOption{
					ID:   b.ID,
					Name: bName,
				})
			}
		}
	}

	data := pages.VendorTeamData{
		NoticeType: noticeType,
		NoticeMsg:  noticeMsg,
		Members:    memberViews,
		Branches:   branchOptions,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.VendorTeamPage(data, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render vendor team page", "error", err)
	}
}

// VendorTeamNewSubmit registers an employee and links them to the vendor org.
func (h *UIHandler) VendorTeamNewSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/team", http.StatusSeeOther)
		return
	}

	if err := r.ParseForm(); err != nil {
		h.redirectWithNotice(w, r, "/vendor/team", "error", "بيانات النموذج غير صالحة.")
		return
	}

	name := strings.TrimSpace(r.PostFormValue("name"))
	email := strings.ToLower(strings.TrimSpace(r.PostFormValue("email")))
	phone := strings.TrimSpace(r.PostFormValue("phone"))
	password := strings.TrimSpace(r.PostFormValue("password"))
	roleKey := strings.TrimSpace(r.PostFormValue("role_key"))
	jobTitle := strings.TrimSpace(r.PostFormValue("job_title"))
	employeeCode := strings.TrimSpace(r.PostFormValue("employee_code"))

	if name == "" {
		h.redirectWithNotice(w, r, "/vendor/team", "error", "يرجى إدخال اسم الموظف بالكامل.")
		return
	}
	if email == "" || !strings.Contains(email, "@") {
		h.redirectWithNotice(w, r, "/vendor/team", "error", "يرجى إدخال بريد إلكتروني صحيح للدخول.")
		return
	}
	if roleKey == "" {
		roleKey = "org_employee"
	}
	if password == "" || len(password) < 6 {
		password = "Password123!"
	}

	var branchID *int64
	if bStr := strings.TrimSpace(r.PostFormValue("branch_id")); bStr != "" {
		if bID, err := strconv.ParseInt(bStr, 10, 64); err == nil && bID > 0 {
			branchID = &bID
		}
	}

	if h.idSvc == nil || h.orgSvc == nil {
		h.redirectWithNotice(w, r, "/vendor/team", "error", "خدمة المنظومة غير متاحة حالياً.")
		return
	}

	// 1. Locate existing user account or create a new one
	var targetUserID int64
	existingUser, err := h.idSvc.GetUserByEmail(ctx, email)
	if err == nil && existingUser != nil {
		targetUserID = existingUser.ID
	} else {
		newUser, _, regErr := h.idSvc.Register(ctx, identity.RegisterInput{
			Email:    email,
			Password: password,
			NameAr:   name,
			NameEn:   name,
			Phone:    phone,
			Role:     "user",
		})
		if regErr != nil {
			h.log.ErrorContext(ctx, "failed to register employee user", "email", email, "error", regErr)
			h.redirectWithNotice(w, r, "/vendor/team", "error", "فشل في إنشاء حساب الموظف: "+h.safeMessage(regErr, langOf(r)))
			return
		}
		targetUserID = newUser.ID
	}

	if employeeCode == "" {
		employeeCode = fmt.Sprintf("EMP-%d", targetUserID)
	}

	// 2. Link member to vendor organization with all specified attributes
	member := &org.Member{
		OrganizationID: actor.OrganizationID,
		UserID:         targetUserID,
		BranchID:       branchID,
		RoleKey:        roleKey,
		JobTitle:       jobTitle,
		EmployeeCode:   employeeCode,
		IsActive:       true,
	}

	if err := h.orgSvc.AddMemberDirect(ctx, member); err != nil {
		h.log.ErrorContext(ctx, "failed to add org member", "error", err, "org_id", actor.OrganizationID, "user_id", targetUserID)
		h.redirectWithNotice(w, r, "/vendor/team", "error", "فشل في ربط الموظف بالمنشأة: "+err.Error())
		return
	}

	h.redirectWithNotice(w, r, "/vendor/team", "success", fmt.Sprintf("تمت إضافة الموظف '%s' بنجاح وتفعيل صلاحياته على المنشأة.", name))
}

// VendorTeamToggleSubmit toggles a member's active status.
func (h *UIHandler) VendorTeamToggleSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/vendor/team", "error", "معرف الموظف غير صالح.")
		return
	}
	if h.orgSvc == nil {
		h.redirectWithNotice(w, r, "/vendor/team", "error", "خدمة المؤسسات غير متوفرة.")
		return
	}
	if err := h.orgSvc.ToggleMemberStatus(ctx, actor.OrganizationID, id); err != nil {
		h.log.ErrorContext(ctx, "toggle member status", "error", err, "member", id, "org", actor.OrganizationID)
		h.redirectWithNotice(w, r, "/vendor/team", "error", h.safeMessage(err, langOf(r)))
		return
	}
	h.redirectWithNotice(w, r, "/vendor/team", "success", "تم تحديث حالة حساب الموظف بنجاح.")
}

// VendorTeamDeleteSubmit removes an employee member from the organization.
func (h *UIHandler) VendorTeamDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/vendor/team", "error", "معرف الموظف غير صالح.")
		return
	}
	if h.orgSvc == nil {
		h.redirectWithNotice(w, r, "/vendor/team", "error", "خدمة المؤسسات غير متوفرة.")
		return
	}
	if err := h.orgSvc.RemoveMember(ctx, actor.OrganizationID, id); err != nil {
		h.log.ErrorContext(ctx, "remove member", "error", err, "member", id, "org", actor.OrganizationID)
		h.redirectWithNotice(w, r, "/vendor/team", "error", h.safeMessage(err, langOf(r)))
		return
	}
	h.redirectWithNotice(w, r, "/vendor/team", "success", "تم حذف الموظف من المنشأة بنجاح.")
}

// VendorRolesPage renders the full roles and permissions matrix for vendor organization.
func (h *UIHandler) VendorRolesPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/roles", http.StatusSeeOther)
		return
	}

	var roles []*org.Role
	memberCountMap := make(map[string]int)

	if h.orgSvc != nil && actor.OrganizationID > 0 {
		if rl, err := h.orgSvc.ListRoles(ctx, actor.OrganizationID); err != nil {
			h.log.WarnContext(ctx, "vendor roles: list roles", "error", err)
		} else {
			roles = rl
		}
		if members, err := h.orgSvc.ListMembers(ctx, actor.OrganizationID); err != nil {
			h.log.WarnContext(ctx, "vendor roles: list members", "error", err)
		} else {
			for _, m := range members {
				if m != nil && m.RoleKey != "" {
					memberCountMap[m.RoleKey]++
				}
			}
		}
	}

	// Defaults if empty
	if memberCountMap["org_owner"] == 0 {
		memberCountMap["org_owner"] = 1
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.VendorRoles(roles, memberCountMap, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render vendor roles page", "error", err)
	}
}

// VendorInventoryPage renders the inventory stock view with search, warehouse filtering, and pagination.
func (h *UIHandler) VendorInventoryPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/inventory", http.StatusSeeOther)
		return
	}

	var allStocks []*inventory.Stock
	var warehouses []*inventory.Warehouse
	if h.invSvc != nil {
		allStocks, _ = h.invSvc.ListStocksByOrg(ctx, actor.OrganizationID)
		warehouses, _ = h.invSvc.ListWarehouses(ctx)
	}

	var variants []*catalog.ProductVariant
	variantMap := make(map[int64]*catalog.ProductVariant)
	if h.catSvc != nil {
		vars, _, _ := h.catSvc.ListVariantsByOrganization(ctx, actor.OrganizationID, catalog.VariantSearchParams{
			Limit: 1000,
		})
		variants = vars
		for _, v := range vars {
			if v != nil {
				variantMap[v.ID] = v
			}
		}
	}

	// Filter stocks by query and warehouse_id
	q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	whID, _ := strconv.ParseInt(r.URL.Query().Get("warehouse_id"), 10, 64)

	var filteredStocks []*inventory.Stock
	for _, s := range allStocks {
		if s == nil {
			continue
		}
		if whID > 0 && s.WarehouseID != whID {
			continue
		}
		if q != "" {
			match := false
			if v, ok := variantMap[s.ProductVariantID]; ok && v != nil {
				if strings.Contains(strings.ToLower(v.Name["ar"]), q) ||
					strings.Contains(strings.ToLower(v.Name["en"]), q) ||
					strings.Contains(strings.ToLower(v.SKU), q) ||
					strings.Contains(strings.ToLower(v.BatchNumber), q) ||
					strings.Contains(strings.ToLower(v.Barcode), q) {
					match = true
				}
			}
			if !match {
				continue
			}
		}
		filteredStocks = append(filteredStocks, s)
	}

	// Pagination
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 25
	}
	if limit > 100 {
		limit = 100
	}

	total := len(filteredStocks)
	totalPages := (total + limit - 1) / limit
	if totalPages < 1 {
		totalPages = 1
	}
	if page > totalPages {
		page = totalPages
	}

	start := (page - 1) * limit
	end := start + limit
	if start > total {
		start = total
	}
	if end > total {
		end = total
	}
	pagedStocks := filteredStocks[start:end]

	data := pages.VendorInventoryData{
		Stocks:      pagedStocks,
		AllStocks:   allStocks,
		Warehouses:  warehouses,
		Variants:    variants,
		Total:       total,
		Page:        page,
		PerPage:     limit,
		TotalPages:  totalPages,
		Query:       r.URL.Query().Get("q"),
		WarehouseID: whID,
		NoticeType:  r.URL.Query().Get("notice_type"),
		NoticeMsg:   r.URL.Query().Get("notice"),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.VendorInventory(data, lang, dir, h.isHTMX(r)).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render vendor inventory page", "error", err)
	}
}

// VendorTransfersPage renders warehouse inventory transfers.
func (h *UIHandler) VendorTransfersPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	if _, ok := database.TenantFrom(ctx); !ok {
		if actor, ok := authctx.From(ctx); ok && actor.OrganizationID > 0 {
			ctx = database.WithTenant(ctx, actor.OrganizationID)
		}
	}

	if h.invSvc == nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := pages.VendorTransfers(nil, lang, dir, h.isHTMX(r)).Render(ctx, w); err != nil {
			h.log.ErrorContext(ctx, "render vendor transfers fallback", "error", err)
		}
		return
	}

	transfers, err := h.invSvc.ListTransfers(ctx, "", h.pageLimit(r), h.pageOffset(r))
	if err != nil {
		h.renderError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.VendorTransfers(transfers, lang, dir, h.isHTMX(r)).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render vendor transfers page", "error", err)
	}
}

// Vendor catalog import handlers are implemented in vendor_ingest_handlers.go (Plan V5 Phase 4).

// VendorIngestSampleXLSX streams a styled Excel template for vendor catalog and inventory upload.
func (h *UIHandler) VendorIngestSampleXLSX(w http.ResponseWriter, r *http.Request) {
	f := excelize.NewFile()
	defer f.Close()

	sheet := "Sheet1"
	headers := []string{
		"الباركود",
		"اسم الصنف بالعربي",
		"اسم الصنف بالإنجليزي",
		"الاسم العلمي",
		"المادة الفعالة",
		"الشكل الصيدلي",
		"الشركة المصنعة",
		"سعر التوريد",
		"الرصيد",
		"رقم التشغيلة",
		"تاريخ الصلاحية",
	}

	for i, head := range headers {
		colName, _ := excelize.ColumnNumberToName(i + 1)
		_ = f.SetCellValue(sheet, fmt.Sprintf("%s1", colName), head)
	}

	sampleRows := [][]string{
		{"6221142001234", "بانادول إكسترا 24 قرص", "Panadol Extra 24 Tab", "Paracetamol + Caffeine", "Paracetamol 500mg", "أقراص", "GSK", "48.50", "250", "BN-94812", "2027-12-31"},
		{"6221142005678", "أوجمنتين 1 جم 14 قرص", "Augmentin 1g 14 Tab", "Amoxicillin + Clavulanate", "Amoxicillin 875mg", "أقراص", "GlaxoSmithKline", "132.00", "120", "BN-88219", "2026-11-30"},
		{"6221142009999", "كتفاست 50 مجم فوار", "Catafast 50mg Sachets", "Diclofenac Potassium", "Diclofenac Potassium 50mg", "فوار", "Novartis", "58.00", "300", "BN-77192", "2028-05-31"},
		{"6221142003322", "كونجستال 20 قرص", "Congestal 20 Tablets", "Paracetamol + Pseudoephedrine", "Paracetamol 500mg", "أقراص", "Eva Pharma", "25.00", "500", "BN-10293", "2027-08-31"},
		{"6221142004455", "أنتينال 24 كبسولة", "Antinal 24 Capsules", "Nifuroxazide", "Nifuroxazide 200mg", "كبسولات", "Amoun", "30.00", "180", "BN-22194", "2027-10-31"},
	}

	for rIdx, row := range sampleRows {
		for cIdx, val := range row {
			colName, _ := excelize.ColumnNumberToName(cIdx + 1)
			_ = f.SetCellValue(sheet, fmt.Sprintf("%s%d", colName, rIdx+2), val)
		}
	}

	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", "attachment; filename=\"dawa24_vendor_catalog_template.xlsx\"")
	_ = f.Write(w)
}

// VendorIngestSampleCSV streams a UTF-8 BOM CSV template for vendor catalog and inventory upload.
func (h *UIHandler) VendorIngestSampleCSV(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=\"dawa24_vendor_catalog_template.csv\"")

	// UTF-8 BOM
	_, _ = w.Write([]byte{0xEF, 0xBB, 0xBF})

	writer := csv.NewWriter(w)
	defer writer.Flush()

	headers := []string{
		"الباركود",
		"اسم الصنف بالعربي",
		"اسم الصنف بالإنجليزي",
		"الاسم العلمي",
		"المادة الفعالة",
		"الشكل الصيدلي",
		"الشركة المصنعة",
		"سعر التوريد",
		"الرصيد",
		"رقم التشغيلة",
		"تاريخ الصلاحية",
	}
	_ = writer.Write(headers)

	sampleRows := [][]string{
		{"6221142001234", "بانادول إكسترا 24 قرص", "Panadol Extra 24 Tab", "Paracetamol + Caffeine", "Paracetamol 500mg", "أقراص", "GSK", "48.50", "250", "BN-94812", "2027-12-31"},
		{"6221142005678", "أوجمنتين 1 جم 14 قرص", "Augmentin 1g 14 Tab", "Amoxicillin + Clavulanate", "Amoxicillin 875mg", "أقراص", "GlaxoSmithKline", "132.00", "120", "BN-88219", "2026-11-30"},
		{"6221142009999", "كتفاست 50 مجم فوار", "Catafast 50mg Sachets", "Diclofenac Potassium", "Diclofenac Potassium 50mg", "فوار", "Novartis", "58.00", "300", "BN-77192", "2028-05-31"},
		{"6221142003322", "كونجستال 20 قرص", "Congestal 20 Tablets", "Paracetamol + Pseudoephedrine", "Paracetamol 500mg", "أقراص", "Eva Pharma", "25.00", "500", "BN-10293", "2027-08-31"},
		{"6221142004455", "أنتينال 24 كبسولة", "Antinal 24 Capsules", "Nifuroxazide", "Nifuroxazide 200mg", "كبسولات", "Amoun", "30.00", "180", "BN-22194", "2027-10-31"},
	}

	for _, row := range sampleRows {
		_ = writer.Write(row)
	}
}

// VendorIngestExport exports the vendor's real active inventory and pricing as a CSV.
func (h *UIHandler) VendorIngestExport(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=\"dawa24_vendor_inventory.csv\"")

	// UTF-8 BOM
	_, _ = w.Write([]byte{0xEF, 0xBB, 0xBF})

	writer := csv.NewWriter(w)
	defer writer.Flush()

	headers := []string{"الباركود / SKU", "اسم الصنف الدوائي", "سعر التوريد (ج.م)", "الرصيد المتاح (عبوة)", "رقم التشغيلة", "تاريخ الصلاحية", "الحالة"}
	_ = writer.Write(headers)

	if h.catSvc != nil {
		products, _ := h.catSvc.Search(database.AsSystem(ctx), catalog.SearchParams{Limit: 500})
		for _, p := range products {
			if p == nil {
				continue
			}
			variants, _ := h.catSvc.ListVariantsByProduct(ctx, p.ID)
			for _, v := range variants {
				if v != nil && v.OrganizationID == actor.OrganizationID {
					expStr := ""
					if v.ExpiryDate != nil {
						expStr = v.ExpiryDate.Format("2006-01-02")
					}
					status := "متاح للطلب"
					if v.Status != catalog.StatusActive && v.Status != "" {
						status = "غير متاح"
					}
					_ = writer.Write([]string{
						v.SKU,
						p.Name.Get(i18n.AR),
						v.Price.String(),
						fmt.Sprintf("%d", v.StockQty),
						v.BatchNumber,
						expStr,
						status,
					})
				}
			}
		}
	}
}

// VendorOrdersPage renders supplier order fulfillments.
func (h *UIHandler) VendorOrdersPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/orders", http.StatusSeeOther)
		return
	}

	if h.commSvc == nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := pages.VendorOrders(pages.VendorOrdersData{}, lang, dir, h.isHTMX(r)).Render(ctx, w); err != nil {
			h.log.ErrorContext(ctx, "render vendor orders fallback", "error", err)
		}
		return
	}

	shipments, err := h.commSvc.ListVendorShipments(ctx, actor.OrganizationID, 100, 0)
	if err != nil {
		h.renderError(w, r, err)
		return
	}

	filterStatus := r.URL.Query().Get("status")
	var pendingCount, confirmedCount, shippedCount, deliveredCount int
	var filtered []*commerce.OrderShipment

	for _, s := range shipments {
		switch s.Status {
		case commerce.StatusPending:
			pendingCount++
		case commerce.StatusConfirmed:
			confirmedCount++
		case commerce.StatusShipped:
			shippedCount++
		case commerce.StatusDelivered:
			deliveredCount++
		}

		if filterStatus == "" || string(s.Status) == filterStatus {
			filtered = append(filtered, s)
		}
	}

	data := pages.VendorOrdersData{
		Shipments:      filtered,
		FilterStatus:   filterStatus,
		TotalCount:     len(shipments),
		PendingCount:   pendingCount,
		ConfirmedCount: confirmedCount,
		ShippedCount:   shippedCount,
		DeliveredCount: deliveredCount,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.VendorOrders(data, lang, dir, h.isHTMX(r)).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render vendor orders page", "error", err)
	}
}

// VendorOffersPage renders the Laravel-parity Special Offers management view.
func (h *UIHandler) VendorOffersPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/offers", http.StatusSeeOther)
		return
	}

	var offers []*promo.SpecialOffer
	if h.promoSvc != nil && actor.OrganizationID > 0 {
		offers, _ = h.promoSvc.ListSpecialOffersByOrg(ctx, actor.OrganizationID)
	}

	data := pages.VendorSpecialOffersData{
		Offers:       offers,
		FilterStatus: r.URL.Query().Get("status"),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.VendorSpecialOffersPage(data, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render vendor offers page", "error", err)
	}
}

// VendorOfferNewPage renders the special offer creation form.
func (h *UIHandler) VendorOfferNewPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/offers/new", http.StatusSeeOther)
		return
	}

	var branches []*org.Branch
	if h.orgSvc != nil && actor.OrganizationID > 0 {
		bList, err := h.orgSvc.ListBranches(ctx, actor.OrganizationID)
		if err != nil {
			h.log.WarnContext(ctx, "vendor offer new: list branches", "error", err)
		} else {
			branches = bList
		}
	}

	var variants []*catalog.ProductVariant
	var itemOptions []pages.VendorOfferItemOption

	var warehouses []*inventory.Warehouse
	var stocks []*inventory.Stock
	whNameMap := make(map[int64]string)
	stockMap := make(map[int64]*inventory.Stock)

	if h.invSvc != nil && actor.OrganizationID > 0 {
		warehouses, _ = h.invSvc.ListWarehouses(ctx)
		stocks, _ = h.invSvc.ListStocksByOrg(ctx, actor.OrganizationID)
		for _, wh := range warehouses {
			if wh != nil {
				whNameMap[wh.ID] = wh.Name
			}
		}
		for _, s := range stocks {
			if s != nil {
				stockMap[s.ProductVariantID] = s
			}
		}
	}

	if h.catSvc != nil {
		vars, _, err := h.catSvc.ListVariantsByOrganization(ctx, actor.OrganizationID, catalog.VariantSearchParams{Limit: 500})
		if err == nil {
			variants = vars
		}
		if len(variants) == 0 {
			products, err := h.catSvc.Search(ctx, catalog.SearchParams{Limit: 100})
			if err == nil {
				for _, p := range products {
					pVars, _ := h.catSvc.ListVariantsByProduct(ctx, p.ID)
					for _, v := range pVars {
						if v.OrganizationID == actor.OrganizationID || v.OrganizationID == 0 {
							variants = append(variants, v)
						}
					}
				}
			}
		}

		for _, v := range variants {
			if v == nil {
				continue
			}
			whName := ""
			stockQty := v.StockQty
			if s, ok := stockMap[v.ID]; ok && s != nil {
				stockQty = s.Quantity
				if n, exists := whNameMap[s.WarehouseID]; exists {
					whName = n
				}
			}
			expStr := ""
			if v.ExpiryDate != nil {
				expStr = v.ExpiryDate.Format("2006-01-02")
			}
			itemOptions = append(itemOptions, pages.VendorOfferItemOption{
				VariantID:      v.ID,
				NameAr:         v.Name["ar"],
				NameEn:         v.Name["en"],
				SKU:            v.SKU,
				BatchNumber:    v.BatchNumber,
				ExpiryDate:     expStr,
				Price:          v.Price.String(),
				PriceFloat:     float64(v.Price.Minor()) / 100.0,
				WarehouseName:  whName,
				AvailableStock: stockQty,
			})
		}
	}

	data := pages.VendorOfferFormData{
		Branches:    branches,
		Variants:    variants,
		ItemOptions: itemOptions,
		IsEdit:      false,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.VendorOfferFormPage(data, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render vendor offer new page", "error", err)
	}
}

// VendorOfferNewSubmit handles special offer creation.
func (h *UIHandler) VendorOfferNewSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/offers", http.StatusSeeOther)
		return
	}

	if err := r.ParseForm(); err != nil {
		h.log.WarnContext(ctx, "parse form error", "error", err)
	}

	titleAr := r.PostFormValue("title_ar")
	titleEn := r.PostFormValue("title_en")
	if titleEn == "" {
		titleEn = titleAr
	}
	descAr := r.PostFormValue("description_ar")
	status := r.PostFormValue("status")
	if status == "" {
		status = "active"
	}
	image := r.PostFormValue("image")

	branchIDVal, _ := strconv.ParseInt(r.PostFormValue("branch_id"), 10, 64)
	var branchID *int64
	if branchIDVal > 0 {
		branchID = &branchIDVal
	}

	discPct, _ := strconv.ParseFloat(r.PostFormValue("discount_percentage"), 64)
	totalPriceFloat, _ := strconv.ParseFloat(r.PostFormValue("total_price"), 64)
	minOrderFloat, _ := strconv.ParseFloat(r.PostFormValue("min_order_amount"), 64)

	var startPtr, endPtr *time.Time
	if sDate := r.PostFormValue("start_date"); sDate != "" {
		if t, err := time.Parse("2006-01-02", sDate); err == nil {
			startPtr = &t
		}
	}
	if eDate := r.PostFormValue("end_date"); eDate != "" {
		if t, err := time.Parse("2006-01-02", eDate); err == nil {
			endPtr = &t
		}
	}

	var prods []*promo.SpecialOfferProduct
	for _, vIDStr := range r.Form["selected_variants"] {
		vID, _ := strconv.ParseInt(vIDStr, 10, 64)
		if vID > 0 {
			qty, _ := strconv.Atoi(r.PostFormValue(fmt.Sprintf("qty_%d", vID)))
			if qty <= 0 {
				qty = 1
			}
			customPrice, _ := strconv.ParseFloat(r.PostFormValue(fmt.Sprintf("custom_price_%d", vID)), 64)
			pDiscPct, _ := strconv.ParseFloat(r.PostFormValue(fmt.Sprintf("discount_pct_%d", vID)), 64)

			prods = append(prods, &promo.SpecialOfferProduct{
				VariantID:          vID,
				Quantity:           qty,
				CustomPrice:        money.FromMinor(int64(customPrice * 100)),
				DiscountPercentage: pDiscPct,
			})
		}
	}

	o := &promo.SpecialOffer{
		OrganizationID:     actor.OrganizationID,
		BranchID:           branchID,
		Title:              i18n.New(titleAr, titleEn),
		Description:        i18n.New(descAr, descAr),
		DiscountPercentage: discPct,
		TotalPrice:         money.FromMinor(int64(totalPriceFloat * 100)),
		MinOrderAmount:     money.FromMinor(int64(minOrderFloat * 100)),
		StartDate:          startPtr,
		EndDate:            endPtr,
		Status:             status,
		AdminStatus:        "approved",
		Image:              image,
		Products:           prods,
	}

	if h.promoSvc != nil {
		if _, err := h.promoSvc.CreateSpecialOffer(ctx, o); err != nil {
			h.log.ErrorContext(ctx, "create special offer error", "error", err)
			h.redirectWithNotice(w, r, "/vendor/offers/new", "error", h.safeMessage(err, langOf(r)))
			return
		}
	}

	h.redirectWithNotice(w, r, "/vendor/offers", "success", "تم إنشاء ونشر العرض الخاص بنجاح.")
}

// VendorOfferLocationsPage renders geographic location coverage management for an offer.
func (h *UIHandler) VendorOfferLocationsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/offers", http.StatusSeeOther)
		return
	}

	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if id <= 0 || h.promoSvc == nil {
		http.Redirect(w, r, "/vendor/offers", http.StatusSeeOther)
		return
	}

	offer, err := h.promoSvc.GetSpecialOffer(ctx, id)
	if err != nil || offer == nil || offer.OrganizationID != actor.OrganizationID {
		h.redirectWithNotice(w, r, "/vendor/offers", "error", "لم يتم العثور على هذا العرض الخاص.")
		return
	}

	locs, _ := h.promoSvc.ListSpecialOfferLocations(ctx, id)

	data := pages.VendorOfferLocationsData{
		Offer:     offer,
		Locations: locs,
		Cities:    h.listCities(ctx),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.VendorOfferLocationsPage(data, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render vendor offer locations page", "error", err)
	}
}

// VendorOfferLocationNewSubmit adds a geographic coverage location to an offer.
func (h *UIHandler) VendorOfferLocationNewSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/offers", http.StatusSeeOther)
		return
	}

	offerID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	cityIDVal, _ := strconv.ParseInt(r.PostFormValue("city_id"), 10, 64)
	var cityID *int64
	if cityIDVal > 0 {
		cityID = &cityIDVal
	}

	lat, _ := strconv.ParseFloat(r.PostFormValue("latitude"), 64)
	lon, _ := strconv.ParseFloat(r.PostFormValue("longitude"), 64)
	radius, _ := strconv.Atoi(r.PostFormValue("radius"))
	if radius <= 0 {
		radius = 500
	}
	day, _ := strconv.Atoi(r.PostFormValue("day_of_week"))
	if day <= 0 {
		day = 1
	}

	loc := &promo.SpecialOfferLocation{
		OfferID:     offerID,
		CityID:      cityID,
		AddressAr:   r.PostFormValue("address_ar"),
		AddressEn:   r.PostFormValue("address_ar"),
		Latitude:    lat,
		Longitude:   lon,
		Radius:      radius,
		DayOfWeek:   day,
		TimeFrom:    r.PostFormValue("time_from"),
		TimeTo:      r.PostFormValue("time_to"),
		Status:      "active",
		AdminStatus: "approved",
	}

	if h.promoSvc != nil {
		_ = h.promoSvc.AddSpecialOfferLocation(ctx, loc)
	}
	_ = actor

	h.redirectWithNotice(w, r, fmt.Sprintf("/vendor/offers/%d/locations", offerID), "success", "تم إضافة نطاق التغطية الجغرافي للعرض بنجاح.")
}

// VendorOfferDeleteSubmit deletes a special offer.
func (h *UIHandler) VendorOfferDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/offers", http.StatusSeeOther)
		return
	}

	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if h.promoSvc != nil && id > 0 {
		_ = h.promoSvc.DeleteSpecialOffer(ctx, id, actor.OrganizationID)
	}

	h.redirectWithNotice(w, r, "/vendor/offers", "success", "تم حذف العرض الخاص بنجاح.")
}

// VendorOrderStatusSubmit transitions shipment delivery states.
func (h *UIHandler) VendorOrderStatusSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/orders", http.StatusSeeOther)
		return
	}

	shipmentID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	toStatus := r.PostFormValue("status")
	notes := r.PostFormValue("notes")

	if actor.OrganizationID > 0 {
		ctx = database.WithTenant(ctx, actor.OrganizationID)
	}

	if h.commSvc != nil && shipmentID > 0 && toStatus != "" {
		_, err := h.commSvc.TransitionShipmentStatus(ctx, shipmentID, commerce.OrderStatus(toStatus), &actor.UserID, notes)
		if err != nil {
			h.log.ErrorContext(ctx, "vendor transition shipment status failed", "error", err, "shipment", shipmentID, "to", toStatus)
			h.redirectWithNotice(w, r, "/vendor/orders", "error", "تعذر تحديث حالة الشحنة: "+h.safeMessage(err, langOf(r)))
			return
		}
		if carrier := r.PostFormValue("carrier"); carrier != "" || r.PostFormValue("tracking") != "" {
			_ = h.commSvc.SetShipmentTracking(ctx, shipmentID, carrier, r.PostFormValue("tracking"))
		}
	}

	h.redirectWithNotice(w, r, "/vendor/orders", "success", "تم تحديث حالة الشحنة بنجاح.")
}

// VendorNegotiationAcceptSubmit accepts a customer's proposed negotiated price and confirms the order.
func (h *UIHandler) VendorNegotiationAcceptSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/orders", http.StatusSeeOther)
		return
	}

	orderID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if h.commSvc != nil && orderID > 0 {
		order, err := h.commSvc.GetOrder(ctx, orderID)
		if err != nil || order == nil {
			h.redirectWithNotice(w, r, "/vendor/orders", "error", "الطلب غير موجود.")
			return
		}
		if !actor.IsStaff && !actor.Can("commerce.admin") {
			isVendorOrder := false
			for _, sh := range order.Shipments {
				if sh.OrganizationID == actor.OrganizationID {
					isVendorOrder = true
					break
				}
			}
			if !isVendorOrder {
				h.redirectWithNotice(w, r, "/vendor/orders", "error", "غير مصرح لك بإدارة هذا الطلب.")
				return
			}
		}

		if err := h.commSvc.AcceptNegotiation(ctx, orderID, actor.UserID); err != nil {
			h.log.ErrorContext(ctx, "vendor accept negotiation failed", "error", err, "order_id", orderID)
			h.redirectWithNotice(w, r, "/vendor/orders", "error", "تعذر قبول التفاوض: "+h.safeMessage(err, langOf(r)))
			return
		}
	}

	h.redirectWithNotice(w, r, "/vendor/orders", "success", "تم قبول السعر المتفاوض عليه واعتماد الطلب بنجاح.")
}

// VendorNegotiationRejectSubmit rejects a customer's proposed negotiated price and cancels the order.
func (h *UIHandler) VendorNegotiationRejectSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/orders", http.StatusSeeOther)
		return
	}

	orderID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	_ = r.ParseForm()
	reason := r.PostFormValue("reason")
	if reason == "" {
		reason = "تم رفض السعر المقترح من قبل إدارة المبيعات"
	}

	if h.commSvc != nil && orderID > 0 {
		order, err := h.commSvc.GetOrder(ctx, orderID)
		if err != nil || order == nil {
			h.redirectWithNotice(w, r, "/vendor/orders", "error", "الطلب غير موجود.")
			return
		}
		if !actor.IsStaff && !actor.Can("commerce.admin") {
			isVendorOrder := false
			for _, sh := range order.Shipments {
				if sh.OrganizationID == actor.OrganizationID {
					isVendorOrder = true
					break
				}
			}
			if !isVendorOrder {
				h.redirectWithNotice(w, r, "/vendor/orders", "error", "غير مصرح لك بإدارة هذا الطلب.")
				return
			}
		}

		if err := h.commSvc.RejectNegotiation(ctx, orderID, reason, actor.UserID); err != nil {
			h.log.ErrorContext(ctx, "vendor reject negotiation failed", "error", err, "order_id", orderID)
			h.redirectWithNotice(w, r, "/vendor/orders", "error", "تعذر رفض التفاوض: "+h.safeMessage(err, langOf(r)))
			return
		}
	}

	h.redirectWithNotice(w, r, "/vendor/orders", "success", "تم رفض طلب التفاوض وإلغاء الطلب.")
}

// VendorStockAdjustSubmit adjusts a stock level with a reason.
func (h *UIHandler) VendorStockAdjustSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/inventory", http.StatusSeeOther)
		return
	}

	stockID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	delta, _ := strconv.Atoi(r.PostFormValue("delta"))
	if h.invSvc != nil && stockID > 0 && delta != 0 {
		_, _ = h.invSvc.AdjustStock(ctx, inventory.AdjustStockInput{
			StockID: stockID,
			Delta:   delta,
			Type:    inventory.MovementAdjustment,
			Details: r.PostFormValue("reason"),
			UserID:  &actor.UserID,
		})
	}
	http.Redirect(w, r, "/vendor/inventory", http.StatusSeeOther)
}

// recordInitialStock writes a variant's opening quantity into inventory.stocks.
//
// inventory.stocks.warehouse_id is NOT NULL, so this needs somewhere to put it.
// A supplier with no warehouse yet gets a clear message rather than a number
// that disappears.
func (h *UIHandler) recordInitialStock(ctx context.Context, orgID int64, v *catalog.ProductVariant, qty int) error {
	if h.invSvc == nil {
		return fmt.Errorf("inventory service unavailable")
	}
	warehouses, err := h.invSvc.ListWarehouses(ctx)
	if err != nil {
		return err
	}
	var warehouseID int64
	// Match warehouse associated with the variant's branch
	if v.BranchID != nil && *v.BranchID > 0 {
		for _, wh := range warehouses {
			if wh.OrganizationID == orgID && wh.BranchID != nil && *wh.BranchID == *v.BranchID && wh.IsActive {
				warehouseID = wh.ID
				break
			}
		}
	}
	// Fallback to any active warehouse of the vendor
	if warehouseID == 0 {
		for _, wh := range warehouses {
			if wh.OrganizationID == orgID && wh.IsActive {
				warehouseID = wh.ID
				break
			}
		}
	}
	// If no warehouse exists, auto-create a real warehouse linked to the vendor's branch
	if warehouseID == 0 {
		whName := "المخزن الرئيسي"
		if v.BranchID != nil && h.orgSvc != nil {
			if b, err := h.orgSvc.GetBranch(ctx, *v.BranchID); err == nil && b != nil {
				whName = "مخزن " + b.Name.Get(i18n.AR)
			}
		}
		newWh := &inventory.Warehouse{
			OrganizationID: orgID,
			BranchID:       v.BranchID,
			Name:           whName,
			Code:           "WH-MAIN",
			IsActive:       true,
		}
		createdWh, err := h.invSvc.CreateWarehouse(ctx, newWh)
		if err == nil && createdWh != nil {
			warehouseID = createdWh.ID
		}
	}
	if warehouseID == 0 {
		return fmt.Errorf("organization %d has no warehouse to hold stock", orgID)
	}
	return h.invSvc.SetStock(ctx, &inventory.Stock{
		OrganizationID:   orgID,
		WarehouseID:      warehouseID,
		ProductID:        v.ProductID,
		ProductVariantID: v.ID,
		Quantity:         qty,
	})
}

// VendorOrganizationPage displays the supplier's commercial profile, order price limits, contact details, description, logo and cover image.
func (h *UIHandler) VendorOrganizationPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/organization", http.StatusSeeOther)
		return
	}

	orgID := actor.OrganizationID
	if orgID <= 0 {
		http.Redirect(w, r, "/vendor/dashboard", http.StatusSeeOther)
		return
	}

	noticeType := r.URL.Query().Get("notice_type")
	noticeMsg := r.URL.Query().Get("notice_msg")

	profile, err := h.orgSvc.GetSupplierProfile(ctx, orgID)
	if err != nil {
		h.log.ErrorContext(ctx, "failed to get supplier organization profile", "org_id", orgID, "error", err)
		profile = &org.SupplierOrgProfile{
			ID:            orgID,
			NameAr:        actor.Email,
			Type:          "supplier",
			MinOrderPrice: money.FromMajor(10),
			MaxOrderPrice: money.FromMajor(50),
		}
	}

	_ = pages.VendorOrganizationPage(lang, dir, profile, noticeType, noticeMsg).Render(ctx, w)
}

// VendorOrganizationSubmit handles updating supplier organization commercial info, limits, and file uploads.
func (h *UIHandler) VendorOrganizationSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/organization", http.StatusSeeOther)
		return
	}

	orgID := actor.OrganizationID
	if orgID <= 0 {
		http.Redirect(w, r, "/vendor/dashboard", http.StatusSeeOther)
		return
	}

	_ = r.ParseMultipartForm(10 << 20) // 10MB

	nameAr := strings.TrimSpace(r.FormValue("name_ar"))
	nameEn := strings.TrimSpace(r.FormValue("name_en"))
	orgType := strings.TrimSpace(r.FormValue("type"))
	if orgType == "" {
		orgType = "supplier"
	}

	minPrice, err := money.Parse(strings.TrimSpace(r.FormValue("min_order_price")))
	if err != nil {
		minPrice = money.FromMajor(10)
	}
	maxPrice, err := money.Parse(strings.TrimSpace(r.FormValue("max_order_price")))
	if err != nil {
		maxPrice = money.FromMajor(50)
	}
	if maxPrice.Minor() < minPrice.Minor() {
		http.Redirect(w, r, "/vendor/organization?notice_type=error&notice_msg="+url.QueryEscape("الحد الأقصى لسعر الطلب يجب أن يكون أكبر من أو يساوي الحد الأدنى"), http.StatusSeeOther)
		return
	}

	orgNumber := strings.TrimSpace(r.FormValue("organization_number"))
	email := strings.TrimSpace(r.FormValue("email"))
	phone := strings.TrimSpace(r.FormValue("phone"))
	taxNumber := strings.TrimSpace(r.FormValue("tax_number"))
	address := strings.TrimSpace(r.FormValue("address"))
	descAr := strings.TrimSpace(r.FormValue("description_ar"))
	descEn := strings.TrimSpace(r.FormValue("description_en"))

	var logoURL, coverURL string
	if file, header, err := r.FormFile("logo_file"); err == nil && file != nil {
		defer file.Close()
		data, _ := io.ReadAll(file)
		if len(data) > 0 {
			if u, err := saveUploadedBytes(data, header.Filename, "org"); err == nil {
				logoURL = u
			}
		}
	}

	if file, header, err := r.FormFile("coverage_file"); err == nil && file != nil {
		defer file.Close()
		data, _ := io.ReadAll(file)
		if len(data) > 0 {
			if u, err := saveUploadedBytes(data, header.Filename, "org"); err == nil {
				coverURL = u
			}
		}
	}

	profile := &org.SupplierOrgProfile{
		ID:                 orgID,
		NameAr:             nameAr,
		NameEn:             nameEn,
		Type:               orgType,
		MinOrderPrice:      minPrice,
		MaxOrderPrice:      maxPrice,
		OrganizationNumber: orgNumber,
		Email:              email,
		Phone:              phone,
		TaxNumber:          taxNumber,
		Address:            address,
		DescriptionAr:      descAr,
		DescriptionEn:      descEn,
		Image:              logoURL,
		CoverageImage:      coverURL,
	}

	if err := h.orgSvc.UpdateSupplierProfile(ctx, profile); err != nil {
		h.log.ErrorContext(ctx, "failed to update supplier profile", "org_id", orgID, "error", err)
		http.Redirect(w, r, "/vendor/organization?notice_type=error&notice_msg="+url.QueryEscape("حدث خطأ أثناء حفظ التعديلات: "+err.Error()), http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/vendor/organization?notice_type=success&notice_msg="+url.QueryEscape("تم حفظ وتحديث بيانات المنشأة والهوية التجارية بنجاح"), http.StatusSeeOther)
}
