package ui

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	platformadmin "github.com/muhiya/dawa24-store/internal/modules/platform_admin"
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
	govs := h.listGovernorates(ctx)
	cities := h.listCities(ctx)

	cityMap := make(map[int64]*platformadmin.City, len(cities))
	for _, c := range cities {
		if c != nil {
			cityMap[c.ID] = c
		}
	}
	govMap := make(map[int64]*platformadmin.Governorate, len(govs))
	for _, g := range govs {
		if g != nil {
			govMap[g.ID] = g
		}
	}

	var editLoc *promo.SpecialOfferLocation
	var editID int64
	if ep := r.URL.Query().Get("edit"); ep != "" {
		editID, _ = strconv.ParseInt(ep, 10, 64)
	}

	for _, loc := range locs {
		if loc.CityID != nil {
			if c, ok := cityMap[*loc.CityID]; ok && c != nil {
				if loc.CityName == "" {
					loc.CityName = c.Name.Get(i18n.ParseLang(lang))
					if loc.CityName == "" {
						loc.CityName = c.Name.Get("ar")
					}
				}
				if c.GovernorateID != nil {
					loc.GovernorateID = c.GovernorateID
					if g, ok := govMap[*c.GovernorateID]; ok && g != nil {
						loc.GovernorateName = g.Name.Get(i18n.ParseLang(lang))
						if loc.GovernorateName == "" {
							loc.GovernorateName = g.Name.Get("ar")
						}
					}
				}
			}
		}
		if editID > 0 && loc.ID == editID {
			editLoc = loc
		}
	}

	noticeType := r.URL.Query().Get("notice_type")
	if noticeType == "" {
		noticeType = r.URL.Query().Get("notice")
	}
	noticeMsg := r.URL.Query().Get("notice")
	if r.URL.Query().Get("msg") != "" {
		noticeMsg = r.URL.Query().Get("msg")
	}

	data := pages.VendorOfferLocationsData{
		Offer:         offer,
		Locations:     locs,
		Governorates:  govs,
		Cities:        cities,
		EditLocation:  editLoc,
		NoticeType:    noticeType,
		NoticeMessage: noticeMsg,
	}

	h.renderPage(ctx, w, "render vendor offer locations page", pages.VendorOfferLocationsPage(data, lang, dir))
}

// VendorOfferLocationNewSubmit adds or updates a geographic coverage location for an offer.
func (h *UIHandler) VendorOfferLocationNewSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/offers", http.StatusSeeOther)
		return
	}

	offerID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	back := fmt.Sprintf("/vendor/offers/%d/locations", offerID)
	if offerID <= 0 || h.promoSvc == nil {
		h.redirectWithNotice(w, r, "/vendor/offers", "error", i18n.T(lang, "common.service_unavailable"))
		return
	}

	offer, err := h.promoSvc.GetSpecialOffer(ctx, offerID)
	if err != nil || offer == nil || offer.OrganizationID != actor.OrganizationID {
		h.redirectWithNotice(w, r, "/vendor/offers", "error", i18n.T(lang, "vendor.offer.forbidden"))
		return
	}

	locIDVal, _ := strconv.ParseInt(chi.URLParam(r, "locId"), 10, 64)
	if locIDVal <= 0 {
		locIDVal, _ = strconv.ParseInt(r.PostFormValue("loc_id"), 10, 64)
	}

	cityIDVal, _ := strconv.ParseInt(r.PostFormValue("city_id"), 10, 64)
	govIDStr := strings.TrimSpace(r.PostFormValue("governorate_id"))

	if cityIDVal <= 0 {
		h.redirectWithNotice(w, r, back, "error", i18n.T(lang, "vendor.offer.city_required"))
		return
	}

	if govIDStr != "" && !h.cityBelongsToGovernorate(ctx, cityIDVal, govIDStr) {
		h.redirectWithNotice(w, r, back, "error", i18n.T(lang, "vendor.offer.city_mismatch"))
		return
	}

	var cityID *int64 = &cityIDVal

	lat, _ := strconv.ParseFloat(r.PostFormValue("latitude"), 64)
	lon, _ := strconv.ParseFloat(r.PostFormValue("longitude"), 64)
	radius, _ := strconv.Atoi(r.PostFormValue("radius"))
	addrAr := strings.TrimSpace(r.PostFormValue("address_ar"))
	addrEn := strings.TrimSpace(r.PostFormValue("address_en"))
	if addrEn == "" {
		addrEn = addrAr
	}

	// If city is selected, populate defaults if coords/radius are missing or default
	cities := h.listCities(ctx)
	for _, c := range cities {
		if c != nil && c.ID == cityIDVal {
			if (lat == 0 && lon == 0) || (lat == 30.0444 && lon == 31.2357 && c.Latitude != 0) {
				lat = c.Latitude
				lon = c.Longitude
			}
			if radius <= 0 {
				radius = c.NormalizedRadius()
			}
			if addrAr == "" {
				addrAr = c.Name.Get("ar")
				addrEn = c.Name.Get("en")
			}
			break
		}
	}
	if radius < platformadmin.MinCoverageRadiusMeters {
		radius = platformadmin.MinCoverageRadiusMeters
	}
	if radius > platformadmin.MaxCoverageRadiusMeters {
		radius = platformadmin.MaxCoverageRadiusMeters
	}

	day, _ := strconv.Atoi(r.PostFormValue("day_of_week"))
	if day <= 0 || day > 7 {
		day = 1
	}

	loc := &promo.SpecialOfferLocation{
		OfferID:     offerID,
		CityID:      cityID,
		AddressAr:   addrAr,
		AddressEn:   addrEn,
		Latitude:    lat,
		Longitude:   lon,
		Radius:      radius,
		DayOfWeek:   day,
		TimeFrom:    strings.TrimSpace(r.PostFormValue("time_from")),
		TimeTo:      strings.TrimSpace(r.PostFormValue("time_to")),
		Status:      "active",
		AdminStatus: "approved",
	}

	// If editing an existing location and city was updated, remove previous location row
	if locIDVal > 0 {
		_ = h.promoSvc.DeleteSpecialOfferLocation(ctx, locIDVal, offerID, actor.OrganizationID)
	}

	if err := h.promoSvc.AddSpecialOfferLocation(ctx, loc); err != nil {
		h.log.ErrorContext(ctx, "add special offer location", "error", err, "offer_id", offerID)
		h.redirectWithNotice(w, r, back, "error", i18n.T(lang, "vendor.offer.location_failed"))
		return
	}

	if locIDVal > 0 {
		h.redirectWithNotice(w, r, back, "success", i18n.T(lang, "vendor.offer.location_updated_success"))
	} else {
		h.redirectWithNotice(w, r, back, "success", i18n.T(lang, "vendor.offer.location_added_success"))
	}
}

// VendorOfferLocationDeleteSubmit deletes a geographic coverage location from an offer.
func (h *UIHandler) VendorOfferLocationDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/offers", http.StatusSeeOther)
		return
	}

	offerID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	locID, _ := strconv.ParseInt(chi.URLParam(r, "locId"), 10, 64)
	back := fmt.Sprintf("/vendor/offers/%d/locations", offerID)

	if offerID <= 0 || locID <= 0 || h.promoSvc == nil {
		h.redirectWithNotice(w, r, back, "error", i18n.T(lang, "vendor.offer.location_not_found"))
		return
	}

	offer, err := h.promoSvc.GetSpecialOffer(ctx, offerID)
	if err != nil || offer == nil || offer.OrganizationID != actor.OrganizationID {
		h.redirectWithNotice(w, r, "/vendor/offers", "error", i18n.T(lang, "vendor.offer.forbidden"))
		return
	}

	if err := h.promoSvc.DeleteSpecialOfferLocation(ctx, locID, offerID, actor.OrganizationID); err != nil {
		h.log.ErrorContext(ctx, "delete special offer location", "error", err, "offer_id", offerID, "loc_id", locID)
		h.redirectWithNotice(w, r, back, "error", i18n.T(lang, "vendor.offer.location_delete_failed"))
		return
	}

	h.redirectWithNotice(w, r, back, "success", i18n.T(lang, "vendor.offer.location_deleted_success"))
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
