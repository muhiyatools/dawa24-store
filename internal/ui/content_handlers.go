package ui

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	platformadmin "github.com/muhiya/dawa24-store/internal/modules/platform_admin"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// renderCmsBlock renders a database-driven content page by block key.
func (h *UIHandler) renderCmsBlock(w http.ResponseWriter, r *http.Request, key, fallbackTitle string) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	title, body := fallbackTitle, ""
	if h.adminSvc != nil {
		if b, err := h.adminSvc.GetContentBlockByKey(ctx, key); err == nil && b != nil && b.IsActive {
			title = b.Title.Get(i18n.Lang(lang))
			body = b.Body.Get(i18n.Lang(lang))
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.CmsPage(lang, dir, title, body).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render cms page", "key", key, "error", err)
	}
}

// AboutPage renders the interactive About Us page with database-driven content.
func (h *UIHandler) AboutPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	data := pages.AboutPageData{}
	if h.adminSvc != nil {
		if b, _ := h.adminSvc.GetContentBlockByKey(ctx, "about"); b != nil && b.IsActive {
			data.HeroTitle = b.Title.Get(i18n.Lang(lang))
			data.HeroSubtitle = b.Body.Get(i18n.Lang(lang))
		}
		if b, _ := h.adminSvc.GetContentBlockByKey(ctx, "about-hero"); b != nil && b.IsActive {
			if data.HeroTitle == "" {
				data.HeroTitle = b.Title.Get(i18n.Lang(lang))
			}
			if data.HeroSubtitle == "" {
				data.HeroSubtitle = b.Body.Get(i18n.Lang(lang))
			}
		}
		if b, _ := h.adminSvc.GetContentBlockByKey(ctx, "about-vision"); b != nil && b.IsActive {
			data.VisionTitle = b.Title.Get(i18n.Lang(lang))
			data.VisionText = b.Body.Get(i18n.Lang(lang))
		}
		if b, _ := h.adminSvc.GetContentBlockByKey(ctx, "about-mission"); b != nil && b.IsActive {
			data.MissionTitle = b.Title.Get(i18n.Lang(lang))
			data.MissionText = b.Body.Get(i18n.Lang(lang))
		}
		if b, _ := h.adminSvc.GetContentBlockByKey(ctx, "about-banner"); b != nil && b.IsActive {
			data.BannerTitle = b.Title.Get(i18n.Lang(lang))
			data.BannerText = b.Body.Get(i18n.Lang(lang))
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AboutPage(lang, dir, data).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render about page", "error", err)
	}
}

// HowItWorksPage renders the how-it-works content block.
func (h *UIHandler) HowItWorksPage(w http.ResponseWriter, r *http.Request) {
	h.renderCmsBlock(w, r, "how-it-works", "كيف يعمل")
}

// FaqPage renders the dedicated interactive FAQ page with dynamic database overrides.
func (h *UIHandler) FaqPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	customTitle, customSubtitle := "", ""
	if h.adminSvc != nil {
		if b, _ := h.adminSvc.GetContentBlockByKey(ctx, "faq"); b != nil && b.IsActive {
			customTitle = b.Title.Get(i18n.Lang(lang))
			customSubtitle = b.Body.Get(i18n.Lang(lang))
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.FAQPage(lang, dir, customTitle, customSubtitle).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render faq page", "error", err)
	}
}

// renderPolicy renders a published legal document by slug.
func (h *UIHandler) renderPolicy(w http.ResponseWriter, r *http.Request, slug, fallbackTitle string) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	title, body := fallbackTitle, ""
	var version string
	var publishedAt string
	if h.adminSvc != nil {
		if p, err := h.adminSvc.GetActivePolicy(ctx, slug); err == nil && p != nil {
			if t := p.Title.Get(i18n.Lang(lang)); t != "" {
				title = t
			}
			if c := p.Content.Get(i18n.Lang(lang)); c != "" {
				body = c
			}
			version = p.Version
			if p.PublishedAt != nil {
				publishedAt = p.PublishedAt.Format("2006-01-02")
			} else if !p.UpdatedAt.IsZero() {
				publishedAt = p.UpdatedAt.Format("2006-01-02")
			}
		}
	}

	if body == "" {
		switch slug {
		case "privacy":
			title = "سياسة الخصوصية وسرية البيانات"
			body = "تلتزم منصة دواء 24 بحماية سرية وخصوصية بيانات المشتركين والموردين والصيدليات وفقاً لأعلى معايير الأمان والتشفير.\n\n1. لا يتم مشاركة بيانات أوامر التوريد أو الأسعار إلا بين الأطراف المتعاقدة.\n2. يتم تشفير كلمات المرور ومفاتيح المصادقة بأحدث خوارزميات التشفير القياسية.\n3. يحق للمستخدم طلب تقرير بكافة عملياته وحركاته المسجلة على المنصة."
		case "terms":
			title = "الشروط والأحكام العامة"
			body = "أهلاً بكم في منصة دواء 24. يخضع استخدام هذه المنصة لكافة الضوابط واللوائح الصيدلانية والتجارية الصادرة عن هيئة الدواء والجهات المختصة.\n\n1. يجب على كافة المنشآت الطبية والصيدليات تقديم تراخيص مزاولة المهنة والسجل التجاري الساري.\n2. كافة المعاملات المالية وأوامر التوريد موثقة ومحمية إلكترونياً.\n3. يلتزم الموردون بضمان جودة وسلاسل تبريد الأدوية والمستلزمات الطبية."
		case "refund":
			title = "سياسة الاسترجاع والإلغاء"
			body = "تخضع عمليات استرجاع أو استبدال الأدوية والمستلزمات الطبية للاشتراطات الصحية المعتمدة.\n\n1. يحق للصيدلية رفض استلام أي شحنة دوائية في حال وجود تلف في العبوة أو عدم مطابقة درجات حرارة التبريد.\n2. يتم إرجاع المبالغ لحساب الصيدلية أو إصدار إشعار دائن فوري عند اعتماد طلب الإرجاع."
		case "vendor_agreement":
			title = "اتفاقية التوريد والاعتماد للموردين"
			body = "تحدد هذه الاتفاقية حقوق والتزامات الموردين والمستودعات الطبية المعتمدة على منصة دواء 24.\n\n1. يلتزم المورد بتحديث أسعار الكتالوج والمخزون المتوفر لحظياً.\n2. يتم تسليم أوامر التوريد للصيدليات خلال المواعيد المحددة مع وثائق التسليم الرسمية."
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.PolicyPage(lang, dir, title, body, slug, version, publishedAt).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render policy page", "slug", slug, "error", err)
	}
}

// AdminContentPage renders the comprehensive CMS content blocks and highlight sections editor.
func (h *UIHandler) AdminContentPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	var blocks []*platformadmin.ContentBlock
	if h.adminSvc != nil {
		blocks, _ = h.adminSvc.ListContentBlocks(database.AsSystem(ctx))
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminContent(lang, dir, blocks).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin content", "error", err)
	}
}

// AdminContentSubmit creates or updates a CMS block or highlight section.
func (h *UIHandler) AdminContentSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if h.adminSvc == nil {
		h.redirectWithNotice(w, r, "/admin/content", "error", "خدمة المحتوى غير متاحة حالياً.")
		return
	}

	_ = r.ParseForm()

	key := strings.TrimSpace(r.FormValue("key"))
	if key == "" {
		h.redirectWithNotice(w, r, "/admin/content", "error", "يرجى تحديد المفتاح التعريفي للكتلة.")
		return
	}

	pos := strings.TrimSpace(r.FormValue("position"))
	if pos == "" {
		pos = "page"
	}

	sortOrder, _ := strconv.Atoi(r.FormValue("sort_order"))
	isActive := r.FormValue("is_active") != "false"

	block := &platformadmin.ContentBlock{
		Key:       key,
		Title:     i18n.New(strings.TrimSpace(r.FormValue("title_ar")), strings.TrimSpace(r.FormValue("title_en"))),
		Body:      i18n.New(strings.TrimSpace(r.FormValue("body_ar")), strings.TrimSpace(r.FormValue("body_en"))),
		Position:  pos,
		SortOrder: sortOrder,
		IsActive:  isActive,
	}

	if err := h.adminSvc.UpsertContentBlock(database.AsSystem(ctx), block); err != nil {
		h.log.ErrorContext(ctx, "admin upsert content block failed", "key", key, "error", err)
		h.redirectWithNotice(w, r, "/admin/content", "error", h.safeMessage(err, langOf(r)))
		return
	}

	h.redirectWithNotice(w, r, "/admin/content", "success", "تم حفظ كتلة المحتوى بنجاح وتحديثها في المنصة.")
}

// AdminContentToggleSubmit toggles the active status of a content block.
func (h *UIHandler) AdminContentToggleSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 || h.adminSvc == nil {
		h.redirectWithNotice(w, r, "/admin/content", "error", "معرف الكتلة غير صالح.")
		return
	}

	if err := h.adminSvc.ToggleContentBlockStatus(database.AsSystem(ctx), id); err != nil {
		h.log.ErrorContext(ctx, "admin toggle content block failed", "id", id, "error", err)
		h.redirectWithNotice(w, r, "/admin/content", "error", h.safeMessage(err, langOf(r)))
		return
	}

	h.redirectWithNotice(w, r, "/admin/content", "success", "تم تحديث حالة تفعيل الكتلة بنجاح.")
}

// AdminContentDeleteSubmit deletes a content block.
func (h *UIHandler) AdminContentDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 || h.adminSvc == nil {
		h.redirectWithNotice(w, r, "/admin/content", "error", "معرف الكتلة غير صالح.")
		return
	}

	if err := h.adminSvc.DeleteContentBlock(database.AsSystem(ctx), id); err != nil {
		h.log.ErrorContext(ctx, "admin delete content block failed", "id", id, "error", err)
		h.redirectWithNotice(w, r, "/admin/content", "error", h.safeMessage(err, langOf(r)))
		return
	}

	h.redirectWithNotice(w, r, "/admin/content", "success", "تم حذف كتلة المحتوى بنجاح.")
}
