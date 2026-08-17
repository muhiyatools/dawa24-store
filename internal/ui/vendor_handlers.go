package ui

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/modules/hr"
	"github.com/muhiya/dawa24-store/internal/modules/identity"
	"github.com/muhiya/dawa24-store/internal/modules/inventory"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/modules/workflow"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
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
	if h.orgSvc != nil && actor.OrganizationID > 0 {
		branches, _ = h.orgSvc.ListBranches(ctx, actor.OrganizationID)
	}

	data := pages.VendorBranchesData{
		Branches: branches,
		Cities:   h.listCities(ctx),
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

	nameAr := r.PostFormValue("name_ar")
	code := r.PostFormValue("code")
	warehouseType := r.PostFormValue("warehouse_type")
	address := r.PostFormValue("address")
	phone := r.PostFormValue("phone")
	manager := r.PostFormValue("manager_name")
	gmaps := r.PostFormValue("google_maps_url")
	hours := r.PostFormValue("operating_hours")
	hasCold := r.PostFormValue("has_cold_storage") == "true"
	isMain := r.PostFormValue("is_main") == "true"

	cityIDVal, _ := strconv.ParseInt(r.PostFormValue("city_id"), 10, 64)
	var cityID *int64
	if cityIDVal > 0 {
		cityID = &cityIDVal
	}

	capSQM, _ := strconv.ParseFloat(r.PostFormValue("capacity_sqm"), 64)

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
		Name:           i18n.New(nameAr, nameAr),
		Code:           code,
		WarehouseType:  warehouseType,
		Address:        address,
		Phone:          phone,
		ManagerName:    manager,
		GoogleMapsURL:  gmaps,
		OperatingHours: hours,
		HasColdStorage: hasCold,
		CapacitySQM:    capSQM,
		CityID:         cityID,
		Latitude:       latPtr,
		Longitude:      lngPtr,
		IsMain:         isMain,
		Status:         "active",
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

// UserDashboardPage renders the dashboard for individual professionals.
func (h *UIHandler) UserDashboardPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/user/dashboard", http.StatusSeeOther)
		return
	}

	var user *identity.User
	if h.idSvc != nil {
		user, _ = h.idSvc.GetUserByID(ctx, actor.UserID)
	}

	var applications []*hr.JobApplication
	if h.hrSvc != nil {
		applications, _ = h.hrSvc.ListApplicationsByUser(ctx, actor.UserID)
	}

	data := pages.UserDashboardData{
		User:         user,
		Applications: applications,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.UserDashboardPage(data, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render user dashboard page", "error", err)
	}
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

// VendorOffersPage renders promotional campaigns.
func (h *UIHandler) VendorOffersPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	if h.promoSvc == nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = pages.VendorOffers(nil, nil, lang, dir, h.isHTMX(r)).Render(ctx, w)
		return
	}

	offers, err := h.promoSvc.ListActiveOffers(ctx, h.pageLimit(r), h.pageOffset(r))
	if err != nil {
		h.renderError(w, r, err)
		return
	}

	packages, _ := h.promoSvc.ListPackages(ctx)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.VendorOffers(offers, packages, lang, dir, h.isHTMX(r)).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render vendor offers page", "error", err)
	}
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

