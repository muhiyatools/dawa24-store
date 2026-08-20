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
	if err := pages.AdminReferenceCRUDPage("إدارة التصنيفات الرئيسية", "categories", "تصنيف", items, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin categories", "error", err)
	}
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
	if err := pages.AdminReferenceCRUDPage("إدارة الماركات والشركات المصنعة", "brands", "ماركة / شركة", items, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin brands", "error", err)
	}
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
	if err := pages.AdminReferenceCRUDPage("دليل الدول والمناطق", "countries", "دولة", items, lang, dir).Render(ctx, w); err != nil {
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
	if err := pages.AdminReferenceCRUDPage("قنوات التواصل الاجتماعي للمنصة", "social-media", "قناة تواصل", items, lang, dir).Render(ctx, w); err != nil {
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
	if err := pages.AdminReferenceCRUDPage("الأقسام المميزة والعروض البارزة", "highlight-sections", "قسم مميز", items, lang, dir).Render(ctx, w); err != nil {
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
	if err := pages.AdminReferenceCRUDPage("بوابات الربط والواجهات البرمجية (APIs)", "api-integrations", "واجهة ربط", items, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin api integrations", "error", err)
	}
}
