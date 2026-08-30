package ui

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/promo"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// AdminOffersPackagesHubPage renders the monetization and packages overview hub.
func (h *UIHandler) AdminOffersPackagesHubPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	tab := strings.TrimSpace(r.URL.Query().Get("tab"))
	if tab == "" {
		tab = "packages"
	}

	var packages []*promo.OfferPackage
	var requests []*promo.SponsorshipRequest
	var ads []*promo.Ad

	var totalPkgs, activePkgs int
	var totalRequests, pendingRequests, approvedRequests int
	var totalAds, pendingAds int

	if h.promoSvc != nil {
		if pkgs, err := h.promoSvc.AdminListPackages(ctx); err == nil {
			packages = pkgs
			totalPkgs = len(pkgs)
			for _, p := range pkgs {
				if p != nil && p.IsActive {
					activePkgs++
				}
			}
		}

		if reqs, err := h.promoSvc.AdminListSponsorshipRequests(database.AsSystem(ctx), 200, 0); err == nil {
			requests = reqs
			totalRequests = len(reqs)
			for _, req := range reqs {
				if req != nil {
					if req.AdminStatus == "pending" {
						pendingRequests++
					} else if req.AdminStatus == "approved" {
						approvedRequests++
					}
				}
			}
		}

		if adList, err := h.promoSvc.AdminListAds(database.AsSystem(ctx), 200, 0); err == nil {
			ads = adList
			totalAds = len(adList)
			for _, a := range adList {
				if a != nil && a.AdminStatus == "pending" {
					pendingAds++
				}
			}
		}
	}

	noticeType := r.URL.Query().Get("notice")
	if noticeType == "" {
		noticeType = r.URL.Query().Get("notice_type")
	}
	noticeMsg := r.URL.Query().Get("msg")
	if noticeMsg == "" {
		noticeMsg = r.URL.Query().Get("message")
	}

	data := pages.AdminOffersPackagesData{
		Packages:         packages,
		Requests:         requests,
		Ads:              ads,
		TotalPackages:    totalPkgs,
		ActivePackages:   activePkgs,
		TotalRequests:    totalRequests,
		PendingRequests:  pendingRequests,
		ApprovedRequests: approvedRequests,
		TotalAds:         totalAds,
		PendingAds:       pendingAds,
		ActiveTab:        tab,
		NoticeType:       noticeType,
		NoticeMsg:        noticeMsg,
	}

	h.renderPage(ctx, w, "render admin offers packages hub", pages.AdminOffersPackagesHubPage(lang, dir, data))
}

// AdminOfferPackagesListPage renders list of offer packages and pricing tiers.
func (h *UIHandler) AdminOfferPackagesListPage(w http.ResponseWriter, r *http.Request) {
	h.AdminOffersPackagesHubPage(w, r)
}

// AdminOfferPackageDetailPage renders single offer package details and features.
func (h *UIHandler) AdminOfferPackageDetailPage(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/admin/offers-packages?tab=packages", http.StatusMovedPermanently)
}

// AdminOfferSponsorshipsPage renders list of sponsored offers and pending requests.
func (h *UIHandler) AdminOfferSponsorshipsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	var requests []*promo.SponsorshipRequest
	if h.promoSvc != nil {
		requests, _ = h.promoSvc.AdminListSponsorshipRequests(database.AsSystem(ctx), 100, 0)
	}

	h.renderPage(ctx, w, "render admin offer sponsorships", pages.AdminOfferSponsorshipsPage(lang, dir, requests))
}

// AdminOfferPromotionsPage renders promotional campaigns list.
func (h *UIHandler) AdminOfferPromotionsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	h.renderPage(ctx, w, "render admin offer promotions", pages.AdminOfferPromotionsPage(lang, dir))
}

// AdminAdsListPage renders advertisements and banners management table with approval actions.
func (h *UIHandler) AdminAdsListPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	var ads []*promo.Ad
	if h.promoSvc != nil {
		ads, _ = h.promoSvc.AdminListAds(database.AsSystem(ctx), 100, 0)
	}

	h.renderPage(ctx, w, "render admin ads list", pages.AdminAdsListPage(lang, dir, ads))
}

// AdminAdPlansPage renders advertising placement plans (unifies into /admin/offers-packages).
func (h *UIHandler) AdminAdPlansPage(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/admin/offers-packages?tab=packages", http.StatusMovedPermanently)
}

// AdminOfferAnalyticsViewsPage renders views time-series and aggregate report.
func (h *UIHandler) AdminOfferAnalyticsViewsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	h.renderPage(ctx, w, "render admin offer analytics views", pages.AdminOfferAnalyticsViewsPage(lang, dir))
}

// AdminOfferAnalyticsClicksPage renders clicks time-series and CTR report.
func (h *UIHandler) AdminOfferAnalyticsClicksPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	h.renderPage(ctx, w, "render admin offer analytics clicks", pages.AdminOfferAnalyticsClicksPage(lang, dir))
}

// AdminOfferLocationsPage renders geographic coverage distribution for offers.
func (h *UIHandler) AdminOfferLocationsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	h.renderPage(ctx, w, "render admin offer locations", pages.AdminOfferLocationsPage(lang, dir))
}

// VendorOffersPackagesPage renders available packages and current purchases for vendor.
func (h *UIHandler) VendorOffersPackagesPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/offers-packages", http.StatusSeeOther)
		return
	}

	var packages []*promo.OfferPackage
	var purchases []*promo.SponsorshipPurchase
	if h.promoSvc != nil {
		packages, _ = h.promoSvc.ListPackages(ctx)
		purchases, _ = h.promoSvc.ListSponsorshipPurchases(ctx)
	}

	data := pages.SponsorshipRequestsData{
		Packages:  packages,
		Purchases: purchases,
		OrgID:     actor.OrganizationID,
	}

	h.renderPage(ctx, w, "render vendor offers packages", pages.VendorOffersPackagesPageWithData(lang, dir, data))
}

// VendorOffersPackagesSponsorshipsPage renders vendor's sponsored offers list.
func (h *UIHandler) VendorOffersPackagesSponsorshipsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	h.renderPage(ctx, w, "render vendor offers packages sponsorships", pages.VendorOffersPackagesSponsorshipsPage(lang, dir))
}

// VendorOffersPackagesPromotionsPage renders vendor's promotional campaigns.
func (h *UIHandler) VendorOffersPackagesPromotionsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	h.renderPage(ctx, w, "render vendor offers packages promotions", pages.VendorOffersPackagesPromotionsPage(lang, dir))
}

// VendorAdsPage renders vendor's active banners and ads with statistics and creation form.
func (h *UIHandler) VendorAdsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/ads", http.StatusSeeOther)
		return
	}

	var ads []*promo.Ad
	if h.promoSvc != nil {
		ads, _ = h.promoSvc.ListAdsByOrg(ctx, 100, 0)
	}

	h.renderPage(ctx, w, "render vendor ads", pages.VendorAdsPage(lang, dir, ads))
}

// VendorOffersLocationsPage renders vendor's offer geographic coverage.
func (h *UIHandler) VendorOffersLocationsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	h.renderPage(ctx, w, "render vendor offers locations", pages.VendorOffersLocationsPage(lang, dir))
}

// PublicPromotionTrackClick records a click to promo.offer_clicks with open redirect guard.
func (h *UIHandler) PublicPromotionTrackClick(w http.ResponseWriter, r *http.Request) {
	offerIDStr := chi.URLParam(r, "offer")
	offerID, _ := strconv.ParseInt(offerIDStr, 10, 64)

	h.log.InfoContext(r.Context(), "tracked promotion click", "offer_id", offerID)
	// Safe redirect strictly to internal offer detail
	http.Redirect(w, r, fmt.Sprintf("/offers/%d", offerID), http.StatusSeeOther)
}

// PublicAdClick records click to promo.ad_clicks and redirects safely.
func (h *UIHandler) PublicAdClick(w http.ResponseWriter, r *http.Request) {
	adIDStr := chi.URLParam(r, "ad")
	adID, _ := strconv.ParseInt(adIDStr, 10, 64)

	h.log.InfoContext(r.Context(), "tracked ad click", "ad_id", adID)
	http.Redirect(w, r, fmt.Sprintf("/offers?ad=%d", adID), http.StatusSeeOther)
}

// AdminSponsorshipRequestApproveSubmit approves a pending sponsorship request from the admin UI.
func (h *UIHandler) AdminSponsorshipRequestApproveSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if h.promoSvc == nil {
		http.Redirect(w, r, "/admin/offers-packages?tab=requests", http.StatusSeeOther)
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		http.Redirect(w, r, "/admin/offers-packages?tab=requests", http.StatusSeeOther)
		return
	}
	sysCtx := database.AsSystem(ctx)
	notes := r.PostFormValue("notes")
	req, err := h.promoSvc.AdminApproveSponsorshipRequest(sysCtx, id, notes)
	if err != nil {
		h.redirectWithNotice(w, r, "/admin/offers-packages?tab=requests", "error", h.safeMessage(err, langOf(r)))
		return
	}
	if req != nil && req.VendorOrganizationID > 0 {
		pkgName := "الباقة الإعلانية"
		if req.PackageName != "" {
			pkgName = req.PackageName
		}
		go h.notifySponsorshipStatus(context.Background(), req.VendorOrganizationID, pkgName, true, notes)
	}
	h.redirectWithNotice(w, r, "/admin/offers-packages?tab=requests", "success", i18n.T(langOf(r), "admin.promo.sponsorship_approved_success"))
}

// AdminSponsorshipRequestRejectSubmit rejects a pending sponsorship request from the admin UI.
func (h *UIHandler) AdminSponsorshipRequestRejectSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if h.promoSvc == nil {
		http.Redirect(w, r, "/admin/offers-packages?tab=requests", http.StatusSeeOther)
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		http.Redirect(w, r, "/admin/offers-packages?tab=requests", http.StatusSeeOther)
		return
	}
	sysCtx := database.AsSystem(ctx)
	notes := r.PostFormValue("notes")
	req, err := h.promoSvc.AdminRejectSponsorshipRequest(sysCtx, id, notes)
	if err != nil {
		h.redirectWithNotice(w, r, "/admin/offers-packages?tab=requests", "error", h.safeMessage(err, langOf(r)))
		return
	}
	if req != nil && req.VendorOrganizationID > 0 {
		pkgName := "الباقة الإعلانية"
		if req.PackageName != "" {
			pkgName = req.PackageName
		}
		go h.notifySponsorshipStatus(context.Background(), req.VendorOrganizationID, pkgName, false, notes)
	}
	h.redirectWithNotice(w, r, "/admin/offers-packages?tab=requests", "success", i18n.T(langOf(r), "admin.promo.sponsorship_rejected_success"))
}

// AdminAdApproveSubmit approves an ad from the admin UI.
func (h *UIHandler) AdminAdApproveSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if h.promoSvc == nil {
		http.Redirect(w, r, "/admin/offers-packages?tab=ads", http.StatusSeeOther)
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		http.Redirect(w, r, "/admin/offers-packages?tab=ads", http.StatusSeeOther)
		return
	}
	sysCtx := database.AsSystem(ctx)
	ad, _ := h.promoSvc.GetAd(sysCtx, id)
	notes := r.PostFormValue("notes")
	if err := h.promoSvc.AdminApproveAd(sysCtx, id, notes); err != nil {
		h.redirectWithNotice(w, r, "/admin/offers-packages?tab=ads", "error", h.safeMessage(err, langOf(r)))
		return
	}
	if ad != nil && ad.OrganizationID > 0 {
		adTitle := ad.Headline
		if adTitle == "" {
			adTitle = "الإعلان الترويجي"
		}
		go h.notifyAdStatus(context.Background(), ad.OrganizationID, adTitle, true, notes)
	}
	h.redirectWithNotice(w, r, "/admin/offers-packages?tab=ads", "success", i18n.T(langOf(r), "admin.promo.ad_approved_success"))
}

// AdminAdRejectSubmit rejects an ad from the admin UI.
func (h *UIHandler) AdminAdRejectSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if h.promoSvc == nil {
		http.Redirect(w, r, "/admin/offers-packages?tab=ads", http.StatusSeeOther)
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		http.Redirect(w, r, "/admin/offers-packages?tab=ads", http.StatusSeeOther)
		return
	}
	sysCtx := database.AsSystem(ctx)
	ad, _ := h.promoSvc.GetAd(sysCtx, id)
	notes := r.PostFormValue("notes")
	if err := h.promoSvc.AdminRejectAd(sysCtx, id, notes); err != nil {
		h.redirectWithNotice(w, r, "/admin/offers-packages?tab=ads", "error", h.safeMessage(err, langOf(r)))
		return
	}
	if ad != nil && ad.OrganizationID > 0 {
		adTitle := ad.Headline
		if adTitle == "" {
			adTitle = "الإعلان الترويجي"
		}
		go h.notifyAdStatus(context.Background(), ad.OrganizationID, adTitle, false, notes)
	}
	h.redirectWithNotice(w, r, "/admin/offers-packages?tab=ads", "success", i18n.T(langOf(r), "admin.promo.ad_rejected_success"))
	if err != nil || id <= 0 {
		http.Redirect(w, r, "/admin/offers-packages?tab=ads", http.StatusSeeOther)
		return
	}
	sysCtx := database.AsSystem(ctx)
	notes := r.PostFormValue("notes")
	if err := h.promoSvc.AdminRejectAd(sysCtx, id, notes); err != nil {
		h.redirectWithNotice(w, r, "/admin/offers-packages?tab=ads", "error", h.safeMessage(err, langOf(r)))
		return
	}
	h.redirectWithNotice(w, r, "/admin/offers-packages?tab=ads", "success", i18n.T(langOf(r), "admin.promo.ad_rejected_success"))
}

// AdminOfferPackageCreateSubmit creates a new monetization / sponsorship package.
func (h *UIHandler) AdminOfferPackageCreateSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	if h.promoSvc == nil {
		h.redirectWithNotice(w, r, "/admin/offers-packages?tab=packages", "error", i18n.T(lang, "admin.promo.packages_service_unavailable"))
		return
	}

	nameAR := strings.TrimSpace(r.PostFormValue("name_ar"))
	nameEN := strings.TrimSpace(r.PostFormValue("name_en"))
	if nameAR == "" && nameEN == "" {
		h.redirectWithNotice(w, r, "/admin/offers-packages?tab=packages", "error", i18n.T(lang, "admin.promo.package_name_required"))
		return
	}
	if nameAR == "" {
		nameAR = nameEN
	}
	if nameEN == "" {
		nameEN = nameAR
	}

	descAR := strings.TrimSpace(r.PostFormValue("desc_ar"))
	descEN := strings.TrimSpace(r.PostFormValue("desc_en"))

	priceStr := strings.TrimSpace(r.PostFormValue("price"))
	price, err := money.Parse(priceStr)
	if err != nil {
		price = money.Zero
	}

	durationDays, _ := strconv.Atoi(r.PostFormValue("duration_days"))
	if durationDays <= 0 {
		durationDays = 30
	}

	credits, _ := strconv.Atoi(r.PostFormValue("credits"))
	if credits <= 0 {
		credits = 10
	}

	maxOffers, _ := strconv.Atoi(r.PostFormValue("max_offers"))
	if maxOffers <= 0 {
		maxOffers = credits * 2
	}

	tierLevel, _ := strconv.Atoi(r.PostFormValue("tier_level"))
	if tierLevel <= 0 || tierLevel > 5 {
		tierLevel = 1
	}

	sortOrder, _ := strconv.Atoi(r.PostFormValue("sort_order"))
	badgeColor := strings.TrimSpace(r.PostFormValue("badge_color"))
	if badgeColor == "" {
		badgeColor = "#0284c7"
	}

	isFeatured := r.PostFormValue("is_featured") == "true"
	isActive := r.PostFormValue("is_active") == "true" || r.PostFormValue("is_active") == "on"

	pkg := &promo.OfferPackage{
		Name:         i18n.New(nameAR, nameEN),
		Description:  i18n.New(descAR, descEN),
		Price:        price,
		DurationDays: durationDays,
		Credits:      credits,
		MaxOffers:    maxOffers,
		TierLevel:    tierLevel,
		SortOrder:    sortOrder,
		BadgeColor:   badgeColor,
		IsFeatured:   isFeatured,
		IsActive:     isActive,
	}

	sysCtx := database.AsSystem(ctx)
	if _, err := h.promoSvc.AdminCreatePackage(sysCtx, pkg); err != nil {
		h.redirectWithNotice(w, r, "/admin/offers-packages?tab=packages", "error", h.safeMessage(err, lang))
		return
	}

	h.redirectWithNotice(w, r, "/admin/offers-packages?tab=packages", "success", i18n.T(lang, "admin.promo.package_created_success"))
}

// AdminOfferPackageEditSubmit updates an offer package.
func (h *UIHandler) AdminOfferPackageEditSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	if h.promoSvc == nil {
		h.redirectWithNotice(w, r, "/admin/offers-packages?tab=packages", "error", i18n.T(lang, "admin.promo.packages_service_unavailable"))
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/admin/offers-packages?tab=packages", "error", i18n.T(lang, "admin.promo.package_invalid_id"))
		return
	}

	nameAR := strings.TrimSpace(r.PostFormValue("name_ar"))
	nameEN := strings.TrimSpace(r.PostFormValue("name_en"))
	if nameAR == "" && nameEN == "" {
		h.redirectWithNotice(w, r, "/admin/offers-packages?tab=packages", "error", i18n.T(lang, "admin.promo.package_name_required"))
		return
	}
	if nameAR == "" {
		nameAR = nameEN
	}
	if nameEN == "" {
		nameEN = nameAR
	}

	descAR := strings.TrimSpace(r.PostFormValue("desc_ar"))
	descEN := strings.TrimSpace(r.PostFormValue("desc_en"))

	priceStr := strings.TrimSpace(r.PostFormValue("price"))
	price, err := money.Parse(priceStr)
	if err != nil {
		price = money.Zero
	}

	durationDays, _ := strconv.Atoi(r.PostFormValue("duration_days"))
	if durationDays <= 0 {
		durationDays = 30
	}

	credits, _ := strconv.Atoi(r.PostFormValue("credits"))
	if credits <= 0 {
		credits = 10
	}

	maxOffers, _ := strconv.Atoi(r.PostFormValue("max_offers"))
	if maxOffers <= 0 {
		maxOffers = credits * 2
	}

	tierLevel, _ := strconv.Atoi(r.PostFormValue("tier_level"))
	if tierLevel <= 0 || tierLevel > 5 {
		tierLevel = 1
	}

	sortOrder, _ := strconv.Atoi(r.PostFormValue("sort_order"))
	badgeColor := strings.TrimSpace(r.PostFormValue("badge_color"))
	if badgeColor == "" {
		badgeColor = "#0284c7"
	}

	isFeatured := r.PostFormValue("is_featured") == "true"
	isActive := r.PostFormValue("is_active") == "true" || r.PostFormValue("is_active") == "on"

	pkg := &promo.OfferPackage{
		ID:           id,
		Name:         i18n.New(nameAR, nameEN),
		Description:  i18n.New(descAR, descEN),
		Price:        price,
		DurationDays: durationDays,
		Credits:      credits,
		MaxOffers:    maxOffers,
		TierLevel:    tierLevel,
		SortOrder:    sortOrder,
		BadgeColor:   badgeColor,
		IsFeatured:   isFeatured,
		IsActive:     isActive,
	}

	sysCtx := database.AsSystem(ctx)
	if _, err := h.promoSvc.AdminUpdatePackage(sysCtx, pkg); err != nil {
		h.redirectWithNotice(w, r, "/admin/offers-packages?tab=packages", "error", h.safeMessage(err, lang))
		return
	}

	h.redirectWithNotice(w, r, "/admin/offers-packages?tab=packages", "success", i18n.T(lang, "admin.promo.package_updated_success"))
}

// AdminOfferPackageToggleSubmit toggles active status for an offer package.
func (h *UIHandler) AdminOfferPackageToggleSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	if h.promoSvc == nil {
		h.redirectWithNotice(w, r, "/admin/offers-packages?tab=packages", "error", i18n.T(lang, "admin.promo.packages_service_unavailable"))
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/admin/offers-packages?tab=packages", "error", i18n.T(lang, "admin.promo.package_invalid_id"))
		return
	}

	active := r.PostFormValue("active") == "true" || r.PostFormValue("active") == "1" || r.PostFormValue("active") == "on"
	sysCtx := database.AsSystem(ctx)
	if err := h.promoSvc.AdminTogglePackageActive(sysCtx, id, active); err != nil {
		h.redirectWithNotice(w, r, "/admin/offers-packages?tab=packages", "error", h.safeMessage(err, lang))
		return
	}

	msg := i18n.T(lang, "admin.promo.package_deactivated_success")
	if active {
		msg = i18n.T(lang, "admin.promo.package_activated_success")
	}
	h.redirectWithNotice(w, r, "/admin/offers-packages?tab=packages", "success", msg)
}
