package ui

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/arabic"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// AdminCategoriesPage renders master product categories CRUD.
func (h *UIHandler) AdminCategoriesPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	sysCtx := database.AsSystem(ctx)

	search := strings.TrimSpace(r.URL.Query().Get("q"))
	status := strings.TrimSpace(r.URL.Query().Get("status"))

	pageSize, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if pageSize <= 0 {
		pageSize = 25
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page <= 0 {
		page = 1
	}

	var allCats []*catalog.Category
	if h.catSvc != nil {
		allCats, _ = h.catSvc.ListCategories(sysCtx)
	}

	var filtered []*catalog.Category
	normSearch := arabic.Normalize(search)
	for _, c := range allCats {
		if c == nil {
			continue
		}
		if status != "" && status != "all" && c.Status != status {
			continue
		}
		if search != "" {
			nameAr := arabic.Normalize(c.Name.Get("ar"))
			nameEn := strings.ToLower(c.Name.Get("en"))
			descAr := arabic.Normalize(c.Description.Get("ar"))
			descEn := strings.ToLower(c.Description.Get("en"))
			sLower := strings.ToLower(search)

			if !strings.Contains(nameAr, normSearch) &&
				!strings.Contains(nameEn, sLower) &&
				!strings.Contains(descAr, normSearch) &&
				!strings.Contains(descEn, sLower) {
				continue
			}
		}
		filtered = append(filtered, c)
	}

	totalCount := len(filtered)
	totalPages := (totalCount + pageSize - 1) / pageSize
	if totalPages < 1 {
		totalPages = 1
	}
	if page > totalPages {
		page = totalPages
	}

	start := (page - 1) * pageSize
	if start < 0 {
		start = 0
	}
	end := start + pageSize
	if end > totalCount {
		end = totalCount
	}

	var items []pages.CategoryViewItem
	if start < totalCount {
		for _, c := range filtered[start:end] {
			count, _ := h.catSvc.CountProductsInCategory(sysCtx, c.ID)
			items = append(items, pages.CategoryViewItem{
				Category:     c,
				ProductCount: count,
			})
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminCategoriesPage(items, allCats, totalCount, page, pageSize, search, status, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin categories", "error", err)
	}
}

// AdminCategoryCreateSubmit creates a new product category.
func (h *UIHandler) AdminCategoryCreateSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if h.catSvc == nil {
		h.redirectWithNotice(w, r, "/admin/categories", "error", "خدمة الكتالوج غير متاحة.")
		return
	}

	_ = r.ParseForm()

	nameAr := strings.TrimSpace(r.FormValue("name_ar"))
	nameEn := strings.TrimSpace(r.FormValue("name_en"))
	if nameAr == "" && nameEn == "" {
		h.redirectWithNotice(w, r, "/admin/categories", "error", "يرجى كتابة اسم فئة المنتج بالعربية أو الإنجليزية.")
		return
	}
	if nameAr == "" {
		nameAr = nameEn
	}
	if nameEn == "" {
		nameEn = nameAr
	}

	sortOrder, _ := strconv.Atoi(r.FormValue("sort_order"))
	status := strings.TrimSpace(r.FormValue("status"))
	if status == "" {
		status = "active"
	}

	var parentIDPtr *int64
	if parentIDStr := strings.TrimSpace(r.FormValue("parent_id")); parentIDStr != "" {
		if pid, err := strconv.ParseInt(parentIDStr, 10, 64); err == nil && pid > 0 {
			parentIDPtr = &pid
		}
	}

	cat := &catalog.Category{
		ParentID:    parentIDPtr,
		Name:        i18n.New(nameAr, nameEn),
		Description: i18n.New(r.FormValue("description_ar"), r.FormValue("description_en")),
		SortOrder:   sortOrder,
		Status:      status,
	}

	if _, err := h.catSvc.CreateCategory(database.AsSystem(ctx), cat); err != nil {
		h.log.ErrorContext(ctx, "admin create category failed", "error", err)
		h.redirectWithNotice(w, r, "/admin/categories", "error", h.safeMessage(err, langOf(r)))
		return
	}

	h.redirectWithNotice(w, r, "/admin/categories", "success", "تم إنشاء فئة المنتجات بنجاح.")
}

// AdminCategoryEditSubmit updates an existing product category.
func (h *UIHandler) AdminCategoryEditSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if h.catSvc == nil {
		h.redirectWithNotice(w, r, "/admin/categories", "error", "خدمة الكتالوج غير متاحة.")
		return
	}

	idStr := chi.URLParam(r, "id")
	catID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || catID <= 0 {
		h.redirectWithNotice(w, r, "/admin/categories", "error", "معرف الفئة غير صالح.")
		return
	}

	_ = r.ParseForm()

	cat, err := h.catSvc.GetCategory(database.AsSystem(ctx), catID)
	if err != nil || cat == nil {
		h.redirectWithNotice(w, r, "/admin/categories", "error", "فئة المنتجات غير موجودة.")
		return
	}

	nameAr := strings.TrimSpace(r.FormValue("name_ar"))
	nameEn := strings.TrimSpace(r.FormValue("name_en"))
	if nameAr == "" && nameEn == "" {
		h.redirectWithNotice(w, r, "/admin/categories", "error", "يرجى كتابة اسم الفئة بالعربية أو الإنجليزية.")
		return
	}
	if nameAr == "" {
		nameAr = nameEn
	}
	if nameEn == "" {
		nameEn = nameAr
	}

	sortOrder, _ := strconv.Atoi(r.FormValue("sort_order"))
	status := strings.TrimSpace(r.FormValue("status"))
	if status == "" {
		status = cat.Status
	}

	var parentIDPtr *int64
	if parentIDStr := strings.TrimSpace(r.FormValue("parent_id")); parentIDStr != "" {
		if pid, err := strconv.ParseInt(parentIDStr, 10, 64); err == nil && pid > 0 {
			if pid != catID { // category cannot be its own parent
				parentIDPtr = &pid
			}
		}
	}

	cat.ParentID = parentIDPtr
	cat.Name = i18n.New(nameAr, nameEn)
	cat.Description = i18n.New(r.FormValue("description_ar"), r.FormValue("description_en"))
	cat.SortOrder = sortOrder
	cat.Status = status

	if err := h.catSvc.UpdateCategory(database.AsSystem(ctx), cat); err != nil {
		h.log.ErrorContext(ctx, "admin update category failed", "error", err)
		h.redirectWithNotice(w, r, "/admin/categories", "error", h.safeMessage(err, langOf(r)))
		return
	}

	h.redirectWithNotice(w, r, "/admin/categories", "success", "تم تحديث بيانات فئة المنتجات بنجاح.")
}

// AdminCategoryToggleSubmit toggles a category's active status.
func (h *UIHandler) AdminCategoryToggleSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	idStr := chi.URLParam(r, "id")
	catID, err := strconv.ParseInt(idStr, 10, 64)
	if err == nil && catID > 0 && h.catSvc != nil {
		cat, err := h.catSvc.GetCategory(database.AsSystem(ctx), catID)
		if err == nil && cat != nil {
			newStatus := "inactive"
			if cat.Status == "inactive" {
				newStatus = "active"
			}
			cat.Status = newStatus
			_ = h.catSvc.UpdateCategory(database.AsSystem(ctx), cat)
		}
	}
	h.redirectWithNotice(w, r, "/admin/categories", "success", "تم تحديث حالة الفئة بنجاح.")
}

// AdminCategoryDeleteSubmit deletes a category if no products are linked.
func (h *UIHandler) AdminCategoryDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if h.catSvc == nil {
		h.redirectWithNotice(w, r, "/admin/categories", "error", "خدمة الكتالوج غير متاحة.")
		return
	}

	idStr := chi.URLParam(r, "id")
	catID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || catID <= 0 {
		h.redirectWithNotice(w, r, "/admin/categories", "error", "معرف الفئة غير صالح.")
		return
	}

	count, _ := h.catSvc.CountProductsInCategory(database.AsSystem(ctx), catID)
	if count > 0 {
		h.redirectWithNotice(w, r, "/admin/categories", "error", fmt.Sprintf("لا يمكن حذف هذه الفئة لوجود %d صنف معتمد مرتبط بها. يرجى نقل الأصناف لفئة أخرى أولاً.", count))
		return
	}

	if err := h.catSvc.DeleteCategory(database.AsSystem(ctx), catID); err != nil {
		h.log.WarnContext(ctx, "admin delete category failed", "error", err)
		h.redirectWithNotice(w, r, "/admin/categories", "error", h.safeMessage(err, langOf(r)))
		return
	}

	h.redirectWithNotice(w, r, "/admin/categories", "success", "تم حذف فئة المنتجات بنجاح.")
}

// AdminBrandsPage renders the master pharmaceutical brands and manufacturers management console.
func (h *UIHandler) AdminBrandsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	sysCtx := database.AsSystem(ctx)

	search := strings.TrimSpace(r.URL.Query().Get("q"))
	status := strings.TrimSpace(r.URL.Query().Get("status"))

	pageSize, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if pageSize <= 0 {
		pageSize = 25
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page <= 0 {
		page = 1
	}

	var allBrands []*catalog.Brand
	if h.catSvc != nil {
		allBrands, _ = h.catSvc.ListBrands(sysCtx)
	}

	var filtered []*catalog.Brand
	normSearch := arabic.Normalize(search)
	for _, b := range allBrands {
		if b == nil {
			continue
		}
		if status != "" && status != "all" && b.Status != status {
			continue
		}
		if search != "" {
			nameAr := arabic.Normalize(b.Name.Get("ar"))
			nameEn := strings.ToLower(b.Name.Get("en"))
			descAr := arabic.Normalize(b.Description.Get("ar"))
			descEn := strings.ToLower(b.Description.Get("en"))
			sLower := strings.ToLower(search)

			if !strings.Contains(nameAr, normSearch) &&
				!strings.Contains(nameEn, sLower) &&
				!strings.Contains(descAr, normSearch) &&
				!strings.Contains(descEn, sLower) {
				continue
			}
		}
		filtered = append(filtered, b)
	}

	totalCount := len(filtered)
	totalPages := (totalCount + pageSize - 1) / pageSize
	if totalPages < 1 {
		totalPages = 1
	}
	if page > totalPages {
		page = totalPages
	}

	start := (page - 1) * pageSize
	if start < 0 {
		start = 0
	}
	end := start + pageSize
	if end > totalCount {
		end = totalCount
	}

	var brandItems []pages.BrandViewItem
	if start < totalCount {
		for _, b := range filtered[start:end] {
			count, _ := h.catSvc.CountProductsInBrand(sysCtx, b.ID)
			brandItems = append(brandItems, pages.BrandViewItem{
				Brand:        b,
				ProductCount: count,
			})
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminBrandsPage(brandItems, totalCount, page, pageSize, search, status, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin brands", "error", err)
	}
}

// AdminBrandCreateSubmit creates a new pharmaceutical brand / manufacturer in the database.
func (h *UIHandler) AdminBrandCreateSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if h.catSvc == nil {
		h.redirectWithNotice(w, r, "/admin/brands", "error", "خدمة الكتالوج غير متاحة.")
		return
	}

	_ = r.ParseMultipartForm(10 << 20)

	nameAr := strings.TrimSpace(r.FormValue("name_ar"))
	nameEn := strings.TrimSpace(r.FormValue("name_en"))
	if nameAr == "" && nameEn == "" {
		h.redirectWithNotice(w, r, "/admin/brands", "error", "يرجى كتابة اسم الشركة المصنعة بالعربية أو الإنجليزية.")
		return
	}
	if nameAr == "" {
		nameAr = nameEn
	}
	if nameEn == "" {
		nameEn = nameAr
	}

	imgURL, _ := saveUploadedFile(r, "brand_image", "brands")
	if imgURL == "" {
		imgURL = strings.TrimSpace(r.FormValue("image_url"))
	}

	status := strings.TrimSpace(r.FormValue("status"))
	if status == "" {
		status = "active"
	}

	brand := &catalog.Brand{
		Name:        i18n.New(nameAr, nameEn),
		Description: i18n.New(r.FormValue("description_ar"), r.FormValue("description_en")),
		Image:       imgURL,
		Status:      status,
	}

	if _, err := h.catSvc.CreateBrand(database.AsSystem(ctx), brand); err != nil {
		h.log.ErrorContext(ctx, "admin create brand failed", "error", err)
		h.redirectWithNotice(w, r, "/admin/brands", "error", h.safeMessage(err, langOf(r)))
		return
	}

	h.redirectWithNotice(w, r, "/admin/brands", "success", "تمت إضافة الشركة المصنعة بنجاح.")
}

// AdminBrandEditSubmit updates an existing brand / manufacturer in the database.
func (h *UIHandler) AdminBrandEditSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if h.catSvc == nil {
		h.redirectWithNotice(w, r, "/admin/brands", "error", "خدمة الكتالوج غير متاحة.")
		return
	}

	idStr := chi.URLParam(r, "id")
	brandID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || brandID <= 0 {
		h.redirectWithNotice(w, r, "/admin/brands", "error", "معرف الشركة غير صالح.")
		return
	}

	_ = r.ParseMultipartForm(10 << 20)

	brand, err := h.catSvc.GetBrand(database.AsSystem(ctx), brandID)
	if err != nil || brand == nil {
		h.redirectWithNotice(w, r, "/admin/brands", "error", "الشركة المصنعة غير موجودة.")
		return
	}

	nameAr := strings.TrimSpace(r.FormValue("name_ar"))
	nameEn := strings.TrimSpace(r.FormValue("name_en"))
	if nameAr == "" && nameEn == "" {
		h.redirectWithNotice(w, r, "/admin/brands", "error", "يرجى كتابة اسم الشركة المصنعة بالعربية أو الإنجليزية.")
		return
	}
	if nameAr == "" {
		nameAr = nameEn
	}
	if nameEn == "" {
		nameEn = nameAr
	}

	imgURL, _ := saveUploadedFile(r, "brand_image", "brands")
	if imgURL == "" {
		imgURL = strings.TrimSpace(r.FormValue("image_url"))
	}
	if imgURL == "" {
		imgURL = brand.Image
	}

	status := strings.TrimSpace(r.FormValue("status"))
	if status == "" {
		status = brand.Status
	}

	brand.Name = i18n.New(nameAr, nameEn)
	brand.Description = i18n.New(r.FormValue("description_ar"), r.FormValue("description_en"))
	brand.Image = imgURL
	brand.Status = status

	if err := h.catSvc.UpdateBrand(database.AsSystem(ctx), brand); err != nil {
		h.log.ErrorContext(ctx, "admin update brand failed", "error", err)
		h.redirectWithNotice(w, r, "/admin/brands", "error", h.safeMessage(err, langOf(r)))
		return
	}

	h.redirectWithNotice(w, r, "/admin/brands", "success", "تم تحديث بيانات الشركة المصنعة بنجاح.")
}

// AdminBrandStatusSubmit toggles a brand's active status.
func (h *UIHandler) AdminBrandStatusSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	idStr := chi.URLParam(r, "id")
	brandID, err := strconv.ParseInt(idStr, 10, 64)
	if err == nil && brandID > 0 && h.catSvc != nil {
		brand, err := h.catSvc.GetBrand(database.AsSystem(ctx), brandID)
		if err == nil && brand != nil {
			newStatus := r.FormValue("status")
			if newStatus == "" {
				if brand.Status == "active" {
					newStatus = "inactive"
				} else {
					newStatus = "active"
				}
			}
			brand.Status = newStatus
			_ = h.catSvc.UpdateBrand(database.AsSystem(ctx), brand)
		}
	}
	http.Redirect(w, r, "/admin/brands", http.StatusSeeOther)
}

// AdminBrandDeleteSubmit deletes a brand if no products are linked to it.
func (h *UIHandler) AdminBrandDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if h.catSvc == nil {
		h.redirectWithNotice(w, r, "/admin/brands", "error", "خدمة الكتالوج غير متاحة.")
		return
	}

	idStr := chi.URLParam(r, "id")
	brandID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || brandID <= 0 {
		h.redirectWithNotice(w, r, "/admin/brands", "error", "معرف الشركة غير صالح.")
		return
	}

	if err := h.catSvc.DeleteBrand(database.AsSystem(ctx), brandID); err != nil {
		h.log.WarnContext(ctx, "admin delete brand failed", "error", err)
		h.redirectWithNotice(w, r, "/admin/brands", "error", h.safeMessage(err, langOf(r)))
		return
	}

	h.redirectWithNotice(w, r, "/admin/brands", "success", "تم حذف الشركة المصنعة بنجاح.")
}

// AdminSocialMediaPage renders social media channel links.
func (h *UIHandler) AdminSocialMediaPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	var items []pages.ReferenceItem
	if h.adminSvc != nil {
		if ss, err := h.adminSvc.GetSiteSettings(ctx); err == nil && ss != nil {
			idx := int64(1)
			for platform, url := range ss.SocialLinks {
				items = append(items, pages.ReferenceItem{
					ID:          idx,
					Name:        platform,
					Description: url,
					Status:      "active",
					Extra:       platform,
				})
				idx++
			}
		}
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminReferenceCRUDPage("قنوات التواصل الاجتماعي للمنصة", "social-media", "قناة تواصل", items, "settings", lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin social media", "error", err)
	}
}

// AdminApiIntegrationsPage renders third-party API configurations.
func (h *UIHandler) AdminApiIntegrationsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	var items []pages.ReferenceItem
	if h.adminSvc != nil {
		if gw, err := h.adminSvc.GetGatewaySettings(ctx); err == nil && gw != nil {
			gwStatus := "inactive"
			if gw.IsActive {
				gwStatus = "active"
			}
			items = append(items, pages.ReferenceItem{
				ID:          1,
				Name:        "بوابة الواجهات البرمجية (API Gateway)",
				Description: fmt.Sprintf("البيئة: %s | الرابط: %s", gw.Environment, gw.EndpointURL),
				Status:      gwStatus,
				Extra:       fmt.Sprintf("المهلة: %d ثوانٍ", gw.TimeoutSeconds),
			})
		}
		if ai, err := h.adminSvc.GetAISettings(ctx); err == nil && ai != nil {
			aiStatus := "inactive"
			if ai.IsActive {
				aiStatus = "active"
			}
			items = append(items, pages.ReferenceItem{
				ID:          2,
				Name:        "بوابة الذكاء الاصطناعي (AI Gateway)",
				Description: fmt.Sprintf("الرابط: %s", ai.EndpointURL),
				Status:      aiStatus,
				Extra:       fmt.Sprintf("الرموز القصوى: %d", ai.MaxTokens),
			})
		}
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminReferenceCRUDPage("بوابات الربط والواجهات البرمجية (APIs)", "api-integrations", "واجهة ربط", items, "developers", lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin api integrations", "error", err)
	}
}

