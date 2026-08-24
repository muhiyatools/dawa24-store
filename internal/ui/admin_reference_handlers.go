package ui

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// AdminCategoriesPage renders master product categories CRUD.
func (h *UIHandler) AdminCategoriesPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	var items []pages.ReferenceItem
	if h.catSvc != nil {
		cats, _ := h.catSvc.ListCategories(database.AsSystem(ctx))
		for _, c := range cats {
			items = append(items, pages.ReferenceItem{
				ID:          c.ID,
				Name:        c.Name.Get("ar"),
				Description: c.Description.Get("ar"),
				Status:      c.Status,
				Extra:       fmt.Sprintf("ترتيب: %d", c.SortOrder),
			})
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminReferenceCRUDPage("إدارة التصنيفات الرئيسية", "categories", "تصنيف", items, "categories", lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin categories", "error", err)
	}
}

// AdminBrandsPage renders the master pharmaceutical brands and manufacturers management console.
func (h *UIHandler) AdminBrandsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	var brandItems []pages.BrandViewItem
	if h.catSvc != nil {
		brands, _ := h.catSvc.ListBrands(database.AsSystem(ctx))
		for _, b := range brands {
			count, _ := h.catSvc.CountProductsInBrand(database.AsSystem(ctx), b.ID)
			brandItems = append(brandItems, pages.BrandViewItem{
				Brand:        b,
				ProductCount: count,
			})
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminBrandsPage(brandItems, lang, dir).Render(ctx, w); err != nil {
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

// AdminCountriesPage renders country reference data.
func (h *UIHandler) AdminCountriesPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	var items []pages.ReferenceItem
	if h.adminSvc != nil {
		countries, _ := h.adminSvc.ListCountries(ctx)
		for _, c := range countries {
			status := "inactive"
			if c.IsActive {
				status = "active"
			}
			items = append(items, pages.ReferenceItem{
				ID:          c.ID,
				Name:        c.Name.Get("ar") + " (" + c.Code + ")",
				Description: fmt.Sprintf("رمز الاتصال: %s | العملة: %s", c.PhoneCode, c.Currency),
				Status:      status,
				Extra:       c.Code,
			})
		}
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminReferenceCRUDPage("دليل الدول والمناطق", "countries", "دولة", items, "countries", lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin countries", "error", err)
	}
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

// AdminHighlightSectionsPage renders promotional highlight sections.
func (h *UIHandler) AdminHighlightSectionsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	var items []pages.ReferenceItem
	if h.adminSvc != nil {
		blocks, _ := h.adminSvc.ListContentBlocks(ctx)
		for _, b := range blocks {
			status := "inactive"
			if b.IsActive {
				status = "active"
			}
			items = append(items, pages.ReferenceItem{
				ID:          b.ID,
				Name:        b.Title.Get("ar"),
				Description: fmt.Sprintf("مفتاح: %s", b.Key),
				Status:      status,
				Extra:       fmt.Sprintf("موضع: %s | ترتيب: %d", b.Position, b.SortOrder),
			})
		}
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminReferenceCRUDPage("الأقسام المميزة والعروض البارزة", "highlight-sections", "قسم مميز", items, "highlight_sections", lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin highlight sections", "error", err)
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

