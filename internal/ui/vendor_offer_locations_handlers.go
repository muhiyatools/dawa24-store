package ui

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/promo"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// VendorOfferLocationsPage renders geographic location coverage management for an offer.
func (h *UIHandler) VendorOfferLocationsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/offers", http.StatusSeeOther)
		return
	}

	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if id <= 0 || h.promoSvc == nil {
		http.Redirect(w, r, "/vendor/offers", http.StatusSeeOther)
		return
	}

	offer, err := h.promoSvc.GetSpecialOffer(ctx, id)
	if err != nil || offer == nil || offer.OrganizationID != actor.OrganizationID {
		h.redirectWithNotice(w, r, "/vendor/offers", "error", i18n.T(lang, "vendor.offer.not_found"))
		return
	}

	locs, _ := h.promoSvc.ListSpecialOfferLocations(ctx, id)

	data := pages.VendorOfferLocationsData{
		Offer:     offer,
		Locations: locs,
		Cities:    h.listCities(ctx),
	}

	h.renderPage(ctx, w, "render vendor offer locations page", pages.VendorOfferLocationsPage(data, lang, dir))
}

// VendorOfferLocationNewSubmit adds a geographic coverage location to an offer.
func (h *UIHandler) VendorOfferLocationNewSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/offers", http.StatusSeeOther)
		return
	}

	offerID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	cityIDVal, _ := strconv.ParseInt(r.PostFormValue("city_id"), 10, 64)
	var cityID *int64
	if cityIDVal > 0 {
		cityID = &cityIDVal
	}

	lat, _ := strconv.ParseFloat(r.PostFormValue("latitude"), 64)
	lon, _ := strconv.ParseFloat(r.PostFormValue("longitude"), 64)
	radius, _ := strconv.Atoi(r.PostFormValue("radius"))
	if radius <= 0 {
		radius = 500
	}
	day, _ := strconv.Atoi(r.PostFormValue("day_of_week"))
	if day <= 0 {
		day = 1
	}

	loc := &promo.SpecialOfferLocation{
		OfferID:     offerID,
		CityID:      cityID,
		AddressAr:   r.PostFormValue("address_ar"),
		AddressEn:   r.PostFormValue("address_ar"),
		Latitude:    lat,
		Longitude:   lon,
		Radius:      radius,
		DayOfWeek:   day,
		TimeFrom:    r.PostFormValue("time_from"),
		TimeTo:      r.PostFormValue("time_to"),
		Status:      "active",
		AdminStatus: "approved",
	}

	back := fmt.Sprintf("/vendor/offers/%d/locations", offerID)
	if h.promoSvc == nil {
		h.redirectWithNotice(w, r, back, "error", i18n.T(lang, "common.service_unavailable"))
		return
	}

	// The offer must be this vendor's. Without the check any signed-in supplier
	// could bolt a coverage area onto a competitor's offer by changing the id
	// in the URL.
	offer, err := h.promoSvc.GetSpecialOffer(ctx, offerID)
	if err != nil || offer == nil || offer.OrganizationID != actor.OrganizationID {
		h.redirectWithNotice(w, r, "/vendor/offers", "error", i18n.T(lang, "vendor.offer.forbidden"))
		return
	}

	if err := h.promoSvc.AddSpecialOfferLocation(ctx, loc); err != nil {
		h.log.ErrorContext(ctx, "add special offer location", "error", err, "offer_id", offerID)
		h.redirectWithNotice(w, r, back, "error", i18n.T(lang, "vendor.offer.location_failed"))
		return
	}

	h.redirectWithNotice(w, r, back, "success", i18n.T(lang, "vendor.offer.location_added_success"))
}

// VendorOfferDeleteSubmit deletes a special offer.
func (h *UIHandler) VendorOfferDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/offers", http.StatusSeeOther)
		return
	}

	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if id <= 0 || h.promoSvc == nil {
		h.redirectWithNotice(w, r, "/vendor/offers", "error", i18n.T(lang, "vendor.offer.not_found"))
		return
	}

	// DeleteSpecialOffer is scoped by organization, so a foreign id deletes
	// nothing — but silence made that indistinguishable from success.
	if err := h.promoSvc.DeleteSpecialOffer(ctx, id, actor.OrganizationID); err != nil {
		h.log.ErrorContext(ctx, "delete special offer", "error", err, "offer_id", id)
		h.redirectWithNotice(w, r, "/vendor/offers", "error", i18n.T(lang, "vendor.offer.delete_failed"))
		return
	}

	h.redirectWithNotice(w, r, "/vendor/offers", "success", i18n.T(lang, "vendor.offer.deleted_success"))
}
