package ui

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/promo"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// VendorSponsorshipRequestsPage renders the vendor's sponsorship requests list
// and the package purchase form.
func (h *UIHandler) VendorSponsorshipRequestsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/sponsorship-requests", http.StatusSeeOther)
		return
	}

	var packages []*promo.OfferPackage
	var purchases []*promo.SponsorshipPurchase
	var requests []*promo.SponsorshipRequest
	var activePurchases []*promo.SponsorshipPurchase

	if h.promoSvc != nil {
		packages, _ = h.promoSvc.ListPackages(ctx)
		purchases, _ = h.promoSvc.ListSponsorshipPurchases(ctx)
		requests, _ = h.promoSvc.ListSponsorshipRequestsByOrg(ctx, 100, 0)
		activePurchases, _ = h.promoSvc.ListActiveSponsorshipPurchases(ctx)
	}

	data := pages.SponsorshipRequestsData{
		Packages:        packages,
		Purchases:       purchases,
		ActivePurchases: activePurchases,
		Requests:        requests,
		OrgID:           actor.OrganizationID,
	}

	h.renderPage(ctx, w, "render vendor sponsorship requests", pages.VendorSponsorshipRequestsPage(lang, dir, data))
}

// VendorSponsorshipRequestSubmit handles the submission of a new sponsorship request.
func (h *UIHandler) VendorSponsorshipRequestSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, _ := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	if h.promoSvc == nil {
		h.redirectWithNotice(w, r, "/vendor/sponsorship-requests", "error", i18n.T(lang, "common.service_unavailable"))
		return
	}

	itemType := strings.TrimSpace(r.PostFormValue("item_type"))
	if itemType != "product" && itemType != "offer" {
		itemType = "product"
	}

	itemID, err := strconv.ParseInt(r.PostFormValue("item_id"), 10, 64)
	if err != nil || itemID <= 0 {
		h.redirectWithNotice(w, r, "/vendor/sponsorship-requests", "error", i18n.T(lang, "vendor.sponsorship.select_item"))
		return
	}

	packageID, err := strconv.ParseInt(r.PostFormValue("package_id"), 10, 64)
	if err != nil || packageID <= 0 {
		h.redirectWithNotice(w, r, "/vendor/sponsorship-requests", "error", i18n.T(lang, "vendor.sponsorship.select_package"))
		return
	}

	_, err = h.promoSvc.SubmitSponsorshipRequest(ctx, promo.SponsorshipItemType(itemType), itemID, packageID)
	if err != nil {
		h.redirectWithNotice(w, r, "/vendor/sponsorship-requests", "error", h.safeMessage(err, lang))
		return
	}
	h.redirectWithNotice(w, r, "/vendor/sponsorship-requests", "success", i18n.T(lang, "vendor.sponsorship.request_submitted_success"))
}

// VendorSponsorshipRequestCancelSubmit cancels a pending sponsorship request.
func (h *UIHandler) VendorSponsorshipRequestCancelSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := h.localeAndDirLang(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	if h.promoSvc == nil {
		h.redirectWithNotice(w, r, "/vendor/sponsorship-requests", "error", i18n.T(lang, "common.service_unavailable"))
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/vendor/sponsorship-requests", "error", i18n.T(lang, "vendor.sponsorship.invalid_request_id"))
		return
	}

	if err := h.promoSvc.CancelSponsorshipRequest(ctx, id); err != nil {
		h.redirectWithNotice(w, r, "/vendor/sponsorship-requests", "error", h.safeMessage(err, lang))
		return
	}
	h.redirectWithNotice(w, r, "/vendor/sponsorship-requests", "success", i18n.T(lang, "vendor.sponsorship.request_cancelled_success"))
}

// VendorSponsorshipPackagePurchaseSubmit handles the purchase of a sponsorship package.
func (h *UIHandler) VendorSponsorshipPackagePurchaseSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := h.localeAndDirLang(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	if h.promoSvc == nil {
		h.redirectWithNotice(w, r, "/vendor/sponsorship-requests", "error", i18n.T(lang, "common.service_unavailable"))
		return
	}

	packageID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || packageID <= 0 {
		h.redirectWithNotice(w, r, "/vendor/sponsorship-requests", "error", i18n.T(lang, "vendor.sponsorship.invalid_package_id"))
		return
	}

	autoRenew := r.PostFormValue("auto_renew") == "true" || r.PostFormValue("auto_renew") == "on"
	billingCycle := r.PostFormValue("billing_cycle")
	if billingCycle == "" {
		billingCycle = "monthly"
	}

	_, err = h.promoSvc.PurchaseSponsorshipPackage(ctx, packageID, autoRenew, billingCycle)
	if err != nil {
		h.redirectWithNotice(w, r, "/vendor/sponsorship-requests", "error", h.safeMessage(err, lang))
		return
	}
	h.redirectWithNotice(w, r, "/vendor/sponsorship-requests", "success", i18n.T(lang, "vendor.sponsorship.package_purchased_success"))
}

// VendorAdCreateSubmit handles the creation of a new advertisement.
func (h *UIHandler) VendorAdCreateSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, _ := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	if h.promoSvc == nil {
		h.redirectWithNotice(w, r, "/vendor/ads", "error", i18n.T(lang, "common.service_unavailable"))
		return
	}

	ad := h.parseAdForm(r, actor.OrganizationID)
	created, err := h.promoSvc.CreateAd(ctx, ad)
	if err != nil {
		h.redirectWithNotice(w, r, "/vendor/ads", "error", h.safeMessage(err, lang))
		return
	}
	h.redirectWithNotice(w, r, "/vendor/ads/"+strconv.FormatInt(created.ID, 10)+"/edit", "success", i18n.T(lang, "vendor.ads.created_success"))
}

// VendorAdUpdateSubmit handles the update of an existing advertisement.
func (h *UIHandler) VendorAdUpdateSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, _ := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	if h.promoSvc == nil {
		h.redirectWithNotice(w, r, "/vendor/ads", "error", i18n.T(lang, "common.service_unavailable"))
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/vendor/ads", "error", i18n.T(lang, "vendor.ads.invalid_ad_id"))
		return
	}

	ad := h.parseAdForm(r, actor.OrganizationID)
	ad.ID = id
	if err := h.promoSvc.UpdateAd(ctx, ad); err != nil {
		h.redirectWithNotice(w, r, "/vendor/ads", "error", h.safeMessage(err, lang))
		return
	}
	h.redirectWithNotice(w, r, "/vendor/ads/"+strconv.FormatInt(id, 10)+"/edit", "success", i18n.T(lang, "vendor.ads.updated_success"))
}

func (h *UIHandler) parseAdForm(r *http.Request, orgID int64) *promo.Ad {
	titleAr := strings.TrimSpace(r.PostFormValue("title_ar"))
	titleEn := strings.TrimSpace(r.PostFormValue("title_en"))
	adTextAr := strings.TrimSpace(r.PostFormValue("ad_text_ar"))
	adTextEn := strings.TrimSpace(r.PostFormValue("ad_text_en"))
	mediaURL := strings.TrimSpace(r.PostFormValue("media_url"))
	thumbnailURL := strings.TrimSpace(r.PostFormValue("thumbnail_url"))
	mediaType := strings.TrimSpace(r.PostFormValue("media_type"))
	clickTarget := strings.TrimSpace(r.PostFormValue("click_target_type"))
	targetID := strings.TrimSpace(r.PostFormValue("click_target_id"))
	position := strings.TrimSpace(r.PostFormValue("position"))
	if position == "" {
		position = "home_banner"
	}
	if mediaType == "" {
		mediaType = "image"
	}
	if clickTarget == "" {
		clickTarget = "vendor_page"
	}

	var clickTargetID *int64
	if targetID != "" {
		if id, err := strconv.ParseInt(targetID, 10, 64); err == nil && id > 0 {
			clickTargetID = &id
		}
	}

	durationDays := 30
	if d, err := strconv.Atoi(r.PostFormValue("duration_days")); err == nil && d > 0 {
		durationDays = d
	}

	title := titleAr
	if title == "" {
		title = titleEn
	}

	return &promo.Ad{
		OrganizationID:  &orgID,
		Title:           title,
		TitleAr:         titleAr,
		TitleEn:         titleEn,
		AdTextAr:        adTextAr,
		AdTextEn:        adTextEn,
		MediaType:       promo.AdMediaType(mediaType),
		MediaURL:        mediaURL,
		ThumbnailURL:    thumbnailURL,
		Position:        position,
		ClickTargetType: promo.AdClickTarget(clickTarget),
		ClickTargetID:   clickTargetID,
		DurationDays:    durationDays,
		IsActive:        false,
	}
}

func (h *UIHandler) localeAndDirLang(r *http.Request) string {
	lang, _ := h.localeAndDir(r)
	return lang
}
