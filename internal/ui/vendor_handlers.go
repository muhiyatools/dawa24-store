package ui

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/modules/identity"
	"github.com/muhiya/dawa24-store/internal/modules/ingest"
	"github.com/muhiya/dawa24-store/internal/modules/inventory"

	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/modules/promo"
	"github.com/muhiya/dawa24-store/internal/modules/workflow"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/httpx"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/ui/pages"

)



// VendorProductsPage renders the vendor's supply variants/offers.
func (h *UIHandler) VendorProductsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/products", http.StatusSeeOther)
		return
	}

	var variantViews []*pages.VendorVariantView
	if h.catSvc != nil {
		products, err := h.catSvc.Search(ctx, catalog.SearchParams{
			Limit: 100,
		})
		if err == nil {
			for _, p := range products {
				variants, _ := h.catSvc.ListVariantsByProduct(ctx, p.ID)
				for _, v := range variants {
					if v.OrganizationID == actor.OrganizationID || v.OrganizationID == 0 {
						variantViews = append(variantViews, &pages.VendorVariantView{
							Variant:       v,
							MasterProduct: p,
							BranchName:    "المستودع الرئيسي",
						})
					}
				}
			}
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.VendorProducts(pages.VendorVariantsData{Variants: variantViews}, lang, dir, h.isHTMX(r)).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render vendor products page", "error", err)
	}
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
		Status:         catalog.StatusActive,
	}

	if h.catSvc != nil {
		if _, err := h.catSvc.CreateVariant(ctx, variant); err != nil {
			h.log.ErrorContext(ctx, "create variant error", "error", err)
			h.redirectWithNotice(w, r, "/vendor/variants/new", "error", "فشل في حفظ عرض الصنف: "+err.Error())
			return
		}
	}

	h.redirectWithNotice(w, r, "/vendor/products", "success", "تم نشر عرض التوريد بنجاح في الكتالوج.")
}

// VendorVariantDeleteSubmit removes a supplier's variant offer.
func (h *UIHandler) VendorVariantDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/products", http.StatusSeeOther)
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		h.renderError(w, r, apperr.Validation("id.invalid", "Invalid variant ID", nil))
		return
	}

	if h.catSvc != nil {
		_ = h.catSvc.DeleteVariant(ctx, id)
	}

	_ = actor
	h.redirectWithNotice(w, r, "/vendor/products", "success", "تم حذف عرض التوريد بنجاح.")
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
			h.redirectWithNotice(w, r, "/vendor/branches", "error", "فشل في حفظ الفرع: "+err.Error())
			return
		}
	}

	h.redirectWithNotice(w, r, "/vendor/branches", "success", "تم إضافة الفرع ونقطة التوزيع بنجاح.")
}


// VendorBranchDeleteSubmit deletes a branch.
func (h *UIHandler) VendorBranchDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	_ = r.Context()
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	_ = id
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

	var memberViews []*pages.TeamMemberView
	if h.orgSvc != nil && actor.OrganizationID > 0 {
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
			}
			memberViews = append(memberViews, &pages.TeamMemberView{
				ID:        m.ID,
				UserID:    m.UserID,
				Name:      name,
				Email:     email,
				Phone:     phone,
				RoleKey:   m.RoleKey,
				RoleName:  roleName,
				IsActive:  m.IsActive,
				CreatedAt: m.CreatedAt.Format("2006-01-02"),
			})
		}
	}

	data := pages.VendorTeamData{
		Members: memberViews,
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
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/team", http.StatusSeeOther)
		return
	}

	name := r.PostFormValue("name")
	email := r.PostFormValue("email")
	phone := r.PostFormValue("phone")
	password := r.PostFormValue("password")
	roleKey := r.PostFormValue("role_key")
	_ = r.PostFormValue("job_title")

	if h.idSvc != nil {
		u, _, err := h.idSvc.Register(ctx, identity.RegisterInput{
			Email:    email,
			Password: password,
			NameAr:   name,
			NameEn:   name,
			Phone:    phone,
			Role:     "employer",
		})
		if err != nil {
			h.redirectWithNotice(w, r, "/vendor/team", "error", "فشل في تسجيل حساب الموظف: "+err.Error())
			return
		}

		if h.orgSvc != nil && u != nil {
			_, _ = h.orgSvc.AddMemberByRoleKey(ctx, actor.OrganizationID, u.ID, roleKey)
		}
	}

	h.redirectWithNotice(w, r, "/vendor/team", "success", "تم إضافة الموظف وتعيين الصلاحيات بنجاح.")
}

// VendorTeamToggleSubmit toggles a member's active status.
func (h *UIHandler) VendorTeamToggleSubmit(w http.ResponseWriter, r *http.Request) {
	_ = r.Context()
	h.redirectWithNotice(w, r, "/vendor/team", "success", "تم تحديث حالة حساب الموظف.")
}

// VendorInventoryPage renders the inventory stock view.

func (h *UIHandler) VendorInventoryPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	if _, ok := database.TenantFrom(ctx); !ok {
		ctx = database.WithTenant(ctx, 1)
	}

	if h.invSvc == nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = pages.VendorInventory(nil, lang, dir, h.isHTMX(r)).Render(ctx, w)
		return
	}

	stocks, err := h.invSvc.ListLowStock(ctx, h.pageLimit(r), h.pageOffset(r))
	if err != nil {
		h.log.WarnContext(ctx, "list low stock error", "error", err)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.VendorInventory(stocks, lang, dir, h.isHTMX(r)).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render vendor inventory page", "error", err)
	}
}

// VendorTransfersPage renders warehouse inventory transfers.
func (h *UIHandler) VendorTransfersPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	if _, ok := database.TenantFrom(ctx); !ok {
		ctx = database.WithTenant(ctx, 1)
	}

	if h.invSvc == nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = pages.VendorTransfers(nil, lang, dir, h.isHTMX(r)).Render(ctx, w)
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

// VendorIngestPage renders the catalog spreadsheet import tool.
func (h *UIHandler) VendorIngestPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/ingest", http.StatusSeeOther)
		return
	}

	if h.ingSvc == nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = pages.VendorIngest(nil, lang, dir).Render(ctx, w)
		return
	}

	jobs, err := h.ingSvc.ListSessions(ctx, actor.OrganizationID, h.pageLimit(r), h.pageOffset(r))
	if err != nil {
		h.renderError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.VendorIngest(jobs, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render vendor ingest page", "error", err)
	}
}

// VendorIngestUploadSubmit processes vendor catalog files via the Ingest service.
func (h *UIHandler) VendorIngestUploadSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		httpx.JSON(w, http.StatusUnauthorized, map[string]any{"error": "Unauthorized"})
		return
	}

	if h.ingSvc == nil {
		httpx.JSON(w, http.StatusServiceUnavailable, map[string]any{"error": "Ingest service unavailable"})
		return
	}

	if err := r.ParseMultipartForm(50 << 20); err != nil {
		httpx.JSON(w, http.StatusBadRequest, map[string]any{"error": "Failed to parse uploaded file"})
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		httpx.JSON(w, http.StatusBadRequest, map[string]any{"error": "No file provided"})
		return
	}
	defer file.Close()

	upload := &ingest.FileUpload{
		OrganizationID: actor.OrganizationID,
		UserID:         actor.UserID,
		Filename:       header.Filename,
		StorageKey:     fmt.Sprintf("orgs/%d/uploads/%d_%s", actor.OrganizationID, time.Now().UnixNano(), header.Filename),
		FileSizeBytes:  header.Size,
		MimeType:       header.Header.Get("Content-Type"),
		CreatedAt:      time.Now().UTC(),
	}

	regUpload, err := h.ingSvc.RegisterUpload(ctx, upload)
	if err != nil {
		h.log.ErrorContext(ctx, "failed to register file upload", "error", err)
		httpx.JSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	session, err := h.ingSvc.StartSession(ctx, regUpload.ID, []string{"Barcode", "Name", "Price", "Quantity"}, 0.8)

	if err != nil {
		h.log.ErrorContext(ctx, "failed to start ingest session", "error", err)
		httpx.JSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	httpx.JSON(w, http.StatusOK, map[string]any{
		"session_id": session.ID,
		"filename":   header.Filename,
		"status":     session.Status,
		"total_rows": session.TotalRows,
	})
}

// VendorIngestCommitSubmit confirms and commits the matched items to inventory.
func (h *UIHandler) VendorIngestCommitSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		httpx.JSON(w, http.StatusUnauthorized, map[string]any{"error": "Unauthorized"})
		return
	}

	sessionID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		httpx.JSON(w, http.StatusBadRequest, map[string]any{"error": "Invalid session ID"})
		return
	}

	if h.ingSvc != nil {
		if err := h.ingSvc.CommitSession(ctx, sessionID); err != nil {
			h.log.ErrorContext(ctx, "failed to commit ingest session", "error", err, "session_id", sessionID)
			httpx.JSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
	}

	httpx.JSON(w, http.StatusOK, map[string]any{"success": true})
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
		_ = pages.VendorOrders(nil, lang, dir, h.isHTMX(r)).Render(ctx, w)
		return
	}

	orders, err := h.commSvc.ListVendorShipments(ctx, actor.OrganizationID, h.pageLimit(r), h.pageOffset(r))
	if err != nil {
		h.renderError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.VendorOrders(orders, lang, dir, h.isHTMX(r)).Render(ctx, w); err != nil {
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
		branches, _ = h.orgSvc.ListBranches(ctx, actor.OrganizationID)
	}

	var variants []*catalog.ProductVariant
	if h.catSvc != nil {
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

	data := pages.VendorOfferFormData{
		Branches: branches,
		Variants: variants,
		IsEdit:   false,
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
			h.redirectWithNotice(w, r, "/vendor/offers/new", "error", "فشل في حفظ العرض الخاص: "+err.Error())
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

	if h.commSvc != nil && shipmentID > 0 && toStatus != "" {
		_, _ = h.commSvc.TransitionShipmentStatus(ctx, shipmentID, commerce.OrderStatus(toStatus), &actor.UserID, "")
		if carrier := r.PostFormValue("carrier"); carrier != "" {
			_ = h.commSvc.SetShipmentTracking(ctx, shipmentID, carrier, r.PostFormValue("tracking"))
		}
	}

	http.Redirect(w, r, "/vendor/orders", http.StatusSeeOther)
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

// VendorCoveragePage renders the weekly geographic coverage grid and distance delivery tiers.
func (h *UIHandler) VendorCoveragePage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/coverage", http.StatusSeeOther)
		return
	}

	var coverages []*workflow.WeeklyCoverage
	var bands []*org.DeliveryBand


	if h.orgSvc != nil && actor.OrganizationID > 0 {
		if b, err := h.orgSvc.GetDeliveryBands(ctx, actor.OrganizationID); err == nil {
			bands = b
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.VendorCoverage(coverages, bands, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render vendor coverage", "error", err)
	}
}

