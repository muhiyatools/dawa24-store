package ui

import (
	"net/http"
	"net/url"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

func (h *UIHandler) CustomerProductDetailPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	// /catalog/{id} is authenticated-only, matching the listing page.
	if actor, ok := authctx.From(ctx); !ok || actor.UserID == 0 {
		http.Redirect(w, r, "/auth/login?redirect="+url.QueryEscape(r.URL.RequestURI()), http.StatusSeeOther)
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

	// When a customer/pharmacist is viewing the product, only show choices that are
	// actually covered and available to use for their receiving branch.
	if actor, ok := authctx.From(ctx); ok && actor.IsCustomer() {
		var coveredOffers []pages.SupplierOffer
		for _, off := range offers {
			if off.IsCovered {
				coveredOffers = append(coveredOffers, off)
			}
		}
		offers = coveredOffers
	}

	// If the URL named a specific variant, surface it — but only ahead of the
	// offers it already ranks with.
	promoteFocusedOffer(offers, targetVariantID)

	h.renderPage(ctx, w, "render product detail page", pages.CustomerProductDetail(product, variants, offers, lang, dir))
}
