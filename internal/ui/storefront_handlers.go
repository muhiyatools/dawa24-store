package ui

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/modules/promo"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// VendorStorefrontPage renders the supplier's merchandising manager.
func (h *UIHandler) VendorStorefrontPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/storefront", http.StatusSeeOther)
		return
	}

	var sections []*promo.HighlightSection
	var products []*catalog.Product
	if h.promoSvc != nil {
		sections, _ = h.promoSvc.ListHighlightSectionsByOrg(ctx, actor.OrganizationID)
	}
	if h.catSvc != nil {
		products, _ = h.catSvc.Search(ctx, catalog.SearchParams{OrganizationID: &actor.OrganizationID, Limit: 100})
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.VendorStorefront(lang, dir, sections, products).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render vendor storefront", "error", err)
	}
}

// VendorStorefrontSectionSubmit creates a merchandising section.
func (h *UIHandler) VendorStorefrontSectionSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/storefront", http.StatusSeeOther)
		return
	}

	if h.promoSvc == nil {
		h.redirectWithNotice(w, r, "/vendor/storefront", "error", "الخدمة غير متاحة حالياً.")
		return
	}

	title := i18n.New(r.PostFormValue("title_ar"), r.PostFormValue("title_en"))
	if _, err := h.promoSvc.CreateOrganizationHighlightSection(ctx, actor.OrganizationID, title, r.PostFormValue("slug")); err != nil {
		h.redirectWithNotice(w, r, "/vendor/storefront", "error", h.safeMessage(err, langOf(r)))
		return
	}
	h.redirectWithNotice(w, r, "/vendor/storefront", "success", "تم إضافة القسم.")
}

// VendorStorefrontItemSubmit adds a product to a merchandising section.
func (h *UIHandler) VendorStorefrontItemSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/storefront", http.StatusSeeOther)
		return
	}

	sectionID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	productID, _ := strconv.ParseInt(r.PostFormValue("product_id"), 10, 64)
	if h.promoSvc != nil && sectionID > 0 && productID > 0 {
		pid := productID
		_ = h.promoSvc.AddHighlightItem(ctx, sectionID, &pid, nil)
	}
	http.Redirect(w, r, "/vendor/storefront", http.StatusSeeOther)
}
