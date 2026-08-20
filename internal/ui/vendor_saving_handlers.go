package ui

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// VendorSavingProductsPage renders the vendor's saving products directory.
func (h *UIHandler) VendorSavingProductsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/saving-products", http.StatusSeeOther)
		return
	}

	var items []*catalog.SavingProduct
	if h.catSvc != nil {
		items, _ = h.catSvc.ListSavingProducts(ctx, actor.OrganizationID, 50, 0)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.VendorSavingProductsPage(items, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render vendor saving products", "error", err)
	}
}

// VendorSavingProductDetailPage renders single saving product details.
func (h *UIHandler) VendorSavingProductDetailPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	idStr := chi.URLParam(r, "id")
	spID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || spID <= 0 {
		http.Redirect(w, r, "/vendor/saving-products", http.StatusSeeOther)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.VendorSavingProductDetailPage(spID, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render vendor saving product detail", "error", err)
	}
}

// VendorSavingProductsImportPage renders bulk import interface for saving products.
func (h *UIHandler) VendorSavingProductsImportPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.VendorSavingProductsImportPage(lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render vendor saving products import", "error", err)
	}
}

// VendorSavingProductsAlias redirects misspelled route /vendor/saveing-products.
func (h *UIHandler) VendorSavingProductsAlias(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/vendor/saving-products", http.StatusMovedPermanently)
}
