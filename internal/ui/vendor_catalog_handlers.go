package ui

import (
	"net/http"
	"strconv"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// VendorCatalogSelectPage renders searchable master catalog picker to activate available products in vendor warehouse.
func (h *UIHandler) VendorCatalogSelectPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/catalog/select", http.StatusSeeOther)
		return
	}

	search := r.URL.Query().Get("q")
	var products []*catalog.Product
	if h.catSvc != nil {
		// Read master catalog products
		products, _ = h.catSvc.ListProducts(database.AsSystem(ctx), search, 100, 0)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.VendorCatalogSelectPage(products, search, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render vendor catalog select", "error", err)
	}
}

// VendorCatalogSelectSubmit adds selected master products as vendor variants.
func (h *UIHandler) VendorCatalogSelectSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	_ = r.ParseForm()
	productIDs := r.Form["product_ids"]

	if h.catSvc != nil && len(productIDs) > 0 {
		for _, idStr := range productIDs {
			pID, _ := strconv.ParseInt(idStr, 10, 64)
			if pID > 0 {
				p, _, err := h.catSvc.GetProduct(database.AsSystem(ctx), pID)
				if err == nil && p != nil {
					_, _ = h.catSvc.CreateVariant(ctx, &catalog.ProductVariant{
						ProductID:      p.ID,
						OrganizationID: actor.OrganizationID,
						Name:           p.Name,
						Status:         catalog.StatusActive,
					})
				}
			}
		}
	}

	h.redirectWithNotice(w, r, "/vendor/products", "success", "تمت إضافة المنتجات المختارة إلى كتالوج المنشأة بنجاح.")
}
