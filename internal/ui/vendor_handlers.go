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

// VendorProductsPage renders the vendor's supply variants/offers.
func (h *UIHandler) VendorProductsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/products", http.StatusSeeOther)
		return
	}

	var branchOptions []pages.VendorBranchOption
	branchMap := make(map[int64]string)
	if h.orgSvc != nil && actor.OrganizationID > 0 {
		branches, err := h.orgSvc.ListBranches(ctx, actor.OrganizationID)
		if err == nil {
			for _, b := range branches {
				bName := b.Name.Get(i18n.AR)
				if bName == "" {
					bName = b.Name.Get(i18n.EN)
				}
				branchOptions = append(branchOptions, pages.VendorBranchOption{
					ID:   b.ID,
					Name: bName,
				})
				branchMap[b.ID] = bName
			}
		}
	}

	var variantViews []*pages.VendorVariantView
	if h.catSvc != nil {
		variants, _, err := h.catSvc.ListVariantsByOrganization(ctx, actor.OrganizationID, catalog.VariantSearchParams{
			Limit: 500,
		})
		if err == nil && len(variants) > 0 {
			productCache := make(map[int64]*catalog.Product)
			for _, v := range variants {
				var mp *catalog.Product
				if v.ProductID > 0 {
					if cached, found := productCache[v.ProductID]; found {
						mp = cached
					} else {
						p, _, pErr := h.catSvc.GetProduct(database.AsSystem(ctx), v.ProductID)
						if pErr == nil && p != nil {
							productCache[v.ProductID] = p
							mp = p
						}
					}
				}

				bName := "المستودع الرئيسي"
				if v.BranchID != nil && *v.BranchID > 0 {
					if name, ok := branchMap[*v.BranchID]; ok {
						bName = name
					}
				}

				variantViews = append(variantViews, &pages.VendorVariantView{
					Variant:       v,
					MasterProduct: mp,
					BranchName:    bName,
					StockQuantity: v.StockQty,
				})
			}
		} else {
			// Fallback: search products and check variants
			products, _ := h.catSvc.Search(ctx, catalog.SearchParams{Limit: 100})
			for _, p := range products {
				pVars, _ := h.catSvc.ListVariantsByProduct(ctx, p.ID)
				for _, v := range pVars {
					if v.OrganizationID == actor.OrganizationID || v.OrganizationID == 0 {
						bName := "المستودع الرئيسي"
						if v.BranchID != nil && *v.BranchID > 0 {
							if name, ok := branchMap[*v.BranchID]; ok {
								bName = name
							}
						}
						variantViews = append(variantViews, &pages.VendorVariantView{
							Variant:       v,
							MasterProduct: p,
							BranchName:    bName,
							StockQuantity: v.StockQty,
						})
					}
				}
			}
		}
	}

	noticeType := r.URL.Query().Get("notice_type")
	noticeMsg := r.URL.Query().Get("notice")

	pageData := pages.VendorVariantsData{
		Variants:   variantViews,
		Branches:   branchOptions,
		NoticeType: noticeType,
		NoticeMsg:  noticeMsg,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.VendorProducts(pageData, lang, dir, h.isHTMX(r)).Render(ctx, w); err != nil {
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
			h.redirectWithNotice(w, r, "/vendor/branches", "error", h.safeMessage(err, langOf(r)))
			return
		}
	}

	h.redirectWithNotice(w, r, "/vendor/branches", "success", "تم إضافة الفرع ونقطة التوزيع بنجاح.")
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

// VendorInventoryPage renders the inventory stock view.

func (h *UIHandler) VendorInventoryPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/inventory", http.StatusSeeOther)
		return
	}

	var stocks []*inventory.Stock
	var warehouses []*inventory.Warehouse
	if h.invSvc != nil {
		stocks, _ = h.invSvc.ListStocksByOrg(ctx, actor.OrganizationID)
		warehouses, _ = h.invSvc.ListWarehouses(ctx)
	}

	var variants []*catalog.ProductVariant
	if h.catSvc != nil {
		variants, _, _ = h.catSvc.ListVariantsByOrganization(ctx, actor.OrganizationID, catalog.VariantSearchParams{
			Limit: 500,
		})
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.VendorInventory(stocks, warehouses, variants, lang, dir, h.isHTMX(r)).Render(ctx, w); err != nil {
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
	if h.catSvc != nil {
		products, err := h.catSvc.Search(ctx, catalog.SearchParams{Limit: 100})
		if err != nil {
			h.log.WarnContext(ctx, "vendor offer new: search catalog", "error", err)
		} else {
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
	for _, wh := range warehouses {
		if wh.OrganizationID == orgID {
			warehouseID = wh.ID
			break
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
