package ui

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/promo"
	"github.com/muhiya/dawa24-store/internal/platform/features"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// OffersPage renders the public offers listing.
func (h *UIHandler) OffersPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !features.Enabled(ctx, "offers.enabled") {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	lang, dir := h.localeAndDir(r)


	var offers []*promo.Offer
	if h.promoSvc != nil {
		offers, _ = h.promoSvc.ListActiveOffers(ctx, 20, 0)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.OffersPage(lang, dir, offers).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render offers page", "error", err)
	}
}

// OfferDetailPage renders one offer and records an impression.
func (h *UIHandler) OfferDetailPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || h.promoSvc == nil {
		h.renderError(w, r, err)
		return
	}

	o, err := h.promoSvc.GetOffer(ctx, id)
	if err != nil {
		h.renderError(w, r, err)
		return
	}
	_ = h.promoSvc.RecordOfferView(ctx, id)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.OfferDetail(lang, dir, o).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render offer detail", "error", err)
	}
}

// OfferClickSubmit records an offer click and sends the user to the catalogue.
func (h *UIHandler) OfferClickSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if h.promoSvc != nil {
		if id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64); err == nil {
			_ = h.promoSvc.RecordOfferClick(ctx, id)
		}
	}
	http.Redirect(w, r, "/catalog", http.StatusSeeOther)
}
