package ui

import (
	"fmt"
	"net/http"

	"github.com/muhiya/dawa24-store/internal/platform/database"
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
	_ = pages.AdminReferenceCRUDPage("إدارة التصنيفات الرئيسية", "categories", "تصنيف", items, lang, dir).Render(ctx, w)
}

// AdminBrandsPage renders master pharmaceutical brands CRUD.
func (h *UIHandler) AdminBrandsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	var items []pages.ReferenceItem
	if h.catSvc != nil {
		brands, _ := h.catSvc.ListBrands(database.AsSystem(ctx))
		for _, b := range brands {
			items = append(items, pages.ReferenceItem{
				ID:          b.ID,
				Name:        b.Name.Get("ar"),
				Description: b.Description.Get("ar"),
				Status:      b.Status,
			})
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = pages.AdminReferenceCRUDPage("إدارة الماركات والشركات المصنعة", "brands", "ماركة / شركة", items, lang, dir).Render(ctx, w)
}

// AdminCountriesPage renders country reference data.
func (h *UIHandler) AdminCountriesPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	items := []pages.ReferenceItem{
		{ID: 1, Name: "جمهورية مصر العربية (Egypt)", Description: "كود: EG (+20)", Status: "active", Extra: "العملة: EGP"},
		{ID: 2, Name: "المملكة العربية السعودية (Saudi Arabia)", Description: "كود: SA (+966)", Status: "active", Extra: "العملة: SAR"},
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = pages.AdminReferenceCRUDPage("دليل الدول والمناطق", "countries", "دولة", items, lang, dir).Render(ctx, w)
}

// AdminSocialMediaPage renders social media channel links.
func (h *UIHandler) AdminSocialMediaPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	items := []pages.ReferenceItem{
		{ID: 1, Name: "فيسبوك (Facebook)", Description: "https://facebook.com/dawa24", Status: "active"},
		{ID: 2, Name: "تويتر / إكس (X)", Description: "https://x.com/dawa24", Status: "active"},
		{ID: 3, Name: "لينكد إن (LinkedIn)", Description: "https://linkedin.com/company/dawa24", Status: "active"},
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = pages.AdminReferenceCRUDPage("قنوات التواصل الاجتماعي للمنصة", "social-media", "قناة تواصل", items, lang, dir).Render(ctx, w)
}

// AdminHighlightSectionsPage renders promotional highlight sections.
func (h *UIHandler) AdminHighlightSectionsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	items := []pages.ReferenceItem{
		{ID: 1, Name: "عروض الأسبوع الأكثر طلباً", Description: "قسم العروض البارزة بالصفحة الرئيسية", Status: "active"},
		{ID: 2, Name: "منتجات التوفير المميزة", Description: "خصومات الصيدليات المباشرة", Status: "active"},
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = pages.AdminReferenceCRUDPage("الأقسام المميزة والعروض البارزة", "highlight-sections", "قسم مميز", items, lang, dir).Render(ctx, w)
}

// AdminApiIntegrationsPage renders third-party API configurations with masked secrets.
func (h *UIHandler) AdminApiIntegrationsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	items := []pages.ReferenceItem{
		{ID: 1, Name: "بوابة الرسائل النصية SMS Gateway", Description: "مفتاح API: ****************4a8f", Status: "active", Extra: "مزود الخدمة: Twilio"},
		{ID: 2, Name: "بوابة الدفع الإلكتروني Payment Gateway", Description: "مفتاح API: ****************9e2c", Status: "active", Extra: "مزود الخدمة: Paymob"},
		{ID: 3, Name: "محرك الذكاء الاصطناعي AI Gateway", Description: "نقطة النهاية: http://localhost:8080/v1", Status: "active", Extra: "النموذج: gemini-2.5-flash"},
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = pages.AdminReferenceCRUDPage("واجهات الربط والتكامل (API Integrations)", "api-integrations", "واجهة تكامل", items, lang, dir).Render(ctx, w)
}
