package ui

import (
	"net/http"

	platformadmin "github.com/muhiya/dawa24-store/internal/modules/platform_admin"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// renderCmsBlock renders a database-driven content page by block key.
func (h *UIHandler) renderCmsBlock(w http.ResponseWriter, r *http.Request, key, fallbackTitle string) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	title, body := fallbackTitle, ""
	if h.adminSvc != nil {
		if b, err := h.adminSvc.GetContentBlockByKey(ctx, key); err == nil && b != nil {
			title = b.Title.Get(i18n.Lang(lang))
			body = b.Body.Get(i18n.Lang(lang))
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.CmsPage(lang, dir, title, body).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render cms page", "key", key, "error", err)
	}
}

// AboutPage renders the about content block.
func (h *UIHandler) AboutPage(w http.ResponseWriter, r *http.Request) {
	h.renderCmsBlock(w, r, "about", "من نحن")
}

// HowItWorksPage renders the how-it-works content block.
func (h *UIHandler) HowItWorksPage(w http.ResponseWriter, r *http.Request) {
	h.renderCmsBlock(w, r, "how-it-works", "كيف يعمل")
}

// FaqPage renders the FAQ content block.
func (h *UIHandler) FaqPage(w http.ResponseWriter, r *http.Request) {
	h.renderCmsBlock(w, r, "faq", "الأسئلة الشائعة")
}

// HelpPage renders the help content block.
func (h *UIHandler) HelpPage(w http.ResponseWriter, r *http.Request) {
	h.renderCmsBlock(w, r, "help", "مركز المساعدة")
}

// renderPolicy renders a published legal document by slug.
func (h *UIHandler) renderPolicy(w http.ResponseWriter, r *http.Request, slug, fallbackTitle string) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	title, body := fallbackTitle, ""
	if h.adminSvc != nil {
		if p, err := h.adminSvc.GetPublishedPolicy(ctx, slug); err == nil && p != nil {
			title = p.Title.Get(i18n.Lang(lang))
			body = p.Content.Get(i18n.Lang(lang))
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.CmsPage(lang, dir, title, body).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render policy page", "slug", slug, "error", err)
	}
}

// AdminContentPage renders the CMS block editor.
func (h *UIHandler) AdminContentPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	var blocks []*platformadmin.ContentBlock
	if h.adminSvc != nil {
		blocks, _ = h.adminSvc.ListContentBlocks(ctx)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminContent(lang, dir, blocks).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin content", "error", err)
	}
}

// AdminContentSubmit upserts a CMS block.
func (h *UIHandler) AdminContentSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if h.adminSvc == nil {
		h.redirectWithNotice(w, r, "/admin/content", "error", "الخدمة غير متاحة حالياً.")
		return
	}

	block := &platformadmin.ContentBlock{
		Key:      r.PostFormValue("key"),
		Title:    i18n.New(r.PostFormValue("title_ar"), r.PostFormValue("title_en")),
		Body:     i18n.New(r.PostFormValue("body_ar"), r.PostFormValue("body_en")),
		Position: r.PostFormValue("position"),
		IsActive: true,
	}
	if block.Position == "" {
		block.Position = "page"
	}

	if err := h.adminSvc.UpsertContentBlock(ctx, block); err != nil {
		h.redirectWithNotice(w, r, "/admin/content", "error", h.safeMessage(err, langOf(r)))
		return
	}
	h.redirectWithNotice(w, r, "/admin/content", "success", "تم حفظ الكتلة.")
}
