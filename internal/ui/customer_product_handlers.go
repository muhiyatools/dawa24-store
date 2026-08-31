package ui

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

func (h *UIHandler) CustomerProductDetailPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if _, ok := authctx.From(ctx); !ok {
		http.Redirect(w, r, "/auth/login?redirect="+r.URL.RequestURI(), http.StatusSeeOther)
		return
	}
	lang, dir := h.localeAndDir(r)

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		h.renderError(w, r, err)
		return
	}

	if h.catSvc == nil {
		h.renderError(w, r, http.ErrNotSupported)
		return
	}

	// Try lookup as variant first, then fallback to product
	var targetProductID int64
	var targetVariantID int64

	if variant, err := h.catSvc.GetVariant(ctx, id); err == nil && variant != nil {
		targetProductID = variant.ProductID
		targetVariantID = variant.ID
	} else {
		targetProductID = id
	}

	product, variants, err := h.catSvc.GetProduct(ctx, targetProductID)
	if err != nil {
		h.renderError(w, r, err)
		return
	}

	env := h.buildOfferEnv(ctx,
		[]int64{product.ID},
		map[int64][]*catalog.ProductVariant{product.ID: variants},
	)
	offers := h.offersForProduct(ctx, product, variants, env, lang)

	// If target variant was specified, prioritize its offer to the top
	if targetVariantID > 0 && len(offers) > 1 {
		for i, off := range offers {
			if off.VariantID == targetVariantID && i > 0 {
				targetOffer := offers[i]
				offers = append([]pages.SupplierOffer{targetOffer}, append(offers[:i], offers[i+1:]...)...)
				break
			}
		}
	}

	h.renderPage(ctx, w, "render product detail page", pages.CustomerProductDetail(product, variants, offers, lang, dir))
}
