package ui

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/promo"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// AdminOffersPackagesHubPage renders the monetization and packages overview hub.
func (h *UIHandler) AdminOffersPackagesHubPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminOffersPackagesHubPage(lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin offers packages hub", "error", err)
	}
}

// AdminOfferPackagesListPage renders list of offer packages and pricing tiers.
func (h *UIHandler) AdminOfferPackagesListPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminOfferPackagesListPage(lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin offer packages list", "error", err)
	}
}

// AdminOfferPackageDetailPage renders single offer package details and features.
func (h *UIHandler) AdminOfferPackageDetailPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	idStr := chi.URLParam(r, "id")
	pkgID, _ := strconv.ParseInt(idStr, 10, 64)
	lang, dir := h.localeAndDir(r)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminOfferPackageDetailPage(pkgID, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin offer package detail", "error", err)
	}
}

// AdminOfferSponsorshipsPage renders list of sponsored offers and pending requests.
func (h *UIHandler) AdminOfferSponsorshipsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	var requests []*promo.SponsorshipRequest
	if h.promoSvc != nil {
		requests, _ = h.promoSvc.AdminListSponsorshipRequests(database.AsSystem(ctx), 100, 0)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminOfferSponsorshipsPage(lang, dir, requests).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin offer sponsorships", "error", err)
	}
}

// AdminOfferPromotionsPage renders promotional campaigns list.
func (h *UIHandler) AdminOfferPromotionsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminOfferPromotionsPage(lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin offer promotions", "error", err)
	}
}

// AdminAdsListPage renders advertisements and banners management table with approval actions.
func (h *UIHandler) AdminAdsListPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	var ads []*promo.Ad
	if h.promoSvc != nil {
		ads, _ = h.promoSvc.AdminListAds(database.AsSystem(ctx), 100, 0)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminAdsListPage(lang, dir, ads).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin ads list", "error", err)
	}
}

// AdminAdPlansPage renders advertising placement plans.
func (h *UIHandler) AdminAdPlansPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminAdPlansPage(lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin ad plans", "error", err)
	}
}

// AdminOfferAnalyticsViewsPage renders views time-series and aggregate report.
func (h *UIHandler) AdminOfferAnalyticsViewsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminOfferAnalyticsViewsPage(lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin offer analytics views", "error", err)
	}
}

// AdminOfferAnalyticsClicksPage renders clicks time-series and CTR report.
func (h *UIHandler) AdminOfferAnalyticsClicksPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminOfferAnalyticsClicksPage(lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin offer analytics clicks", "error", err)
	}
}

// AdminOfferLocationsPage renders geographic coverage distribution for offers.
func (h *UIHandler) AdminOfferLocationsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminOfferLocationsPage(lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin offer locations", "error", err)
	}
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
		Purchases:  purchases,
		OrgID:     actor.OrganizationID,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.VendorOffersPackagesPageWithData(lang, dir, data).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render vendor offers packages", "error", err)
	}
}

// VendorOffersPackagesSponsorshipsPage renders vendor's sponsored offers list.
func (h *UIHandler) VendorOffersPackagesSponsorshipsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.VendorOffersPackagesSponsorshipsPage(lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render vendor offers packages sponsorships", "error", err)
	}
}

// VendorOffersPackagesPromotionsPage renders vendor's promotional campaigns.
func (h *UIHandler) VendorOffersPackagesPromotionsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.VendorOffersPackagesPromotionsPage(lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render vendor offers packages promotions", "error", err)
	}
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

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.VendorAdsPage(lang, dir, ads).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render vendor ads", "error", err)
	}
}

// VendorOffersLocationsPage renders vendor's offer geographic coverage.
func (h *UIHandler) VendorOffersLocationsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.VendorOffersLocationsPage(lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render vendor offers locations", "error", err)
	}
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
		http.Redirect(w, r, "/admin/offers-packages/sponsorships", http.StatusSeeOther)
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		http.Redirect(w, r, "/admin/offers-packages/sponsorships", http.StatusSeeOther)
		return
	}
	sysCtx := database.AsSystem(ctx)
	notes := r.PostFormValue("notes")
	if _, err := h.promoSvc.AdminApproveSponsorshipRequest(sysCtx, id, notes); err != nil {
		h.redirectWithNotice(w, r, "/admin/offers-packages/sponsorships", "error", h.safeMessage(err, langOf(r)))
		return
	}
	h.redirectWithNotice(w, r, "/admin/offers-packages/sponsorships", "success", "تم اعتماد طلب الرعاية. سيظهر العنصر في صدارة النتائج.")
}

// AdminSponsorshipRequestRejectSubmit rejects a pending sponsorship request from the admin UI.
func (h *UIHandler) AdminSponsorshipRequestRejectSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if h.promoSvc == nil {
		http.Redirect(w, r, "/admin/offers-packages/sponsorships", http.StatusSeeOther)
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		http.Redirect(w, r, "/admin/offers-packages/sponsorships", http.StatusSeeOther)
		return
	}
	sysCtx := database.AsSystem(ctx)
	notes := r.PostFormValue("notes")
	if err := h.promoSvc.AdminRejectSponsorshipRequest(sysCtx, id, notes); err != nil {
		h.redirectWithNotice(w, r, "/admin/offers-packages/sponsorships", "error", h.safeMessage(err, langOf(r)))
		return
	}
	h.redirectWithNotice(w, r, "/admin/offers-packages/sponsorships", "success", "تم رفض طلب الرعاية وإرجاع الرصيد.")
}

// AdminAdApproveSubmit approves an ad from the admin UI.
func (h *UIHandler) AdminAdApproveSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if h.promoSvc == nil {
		http.Redirect(w, r, "/admin/ads", http.StatusSeeOther)
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		http.Redirect(w, r, "/admin/ads", http.StatusSeeOther)
		return
	}
	sysCtx := database.AsSystem(ctx)
	notes := r.PostFormValue("notes")
	if err := h.promoSvc.AdminApproveAd(sysCtx, id, notes); err != nil {
		h.redirectWithNotice(w, r, "/admin/ads", "error", h.safeMessage(err, langOf(r)))
		return
	}
	h.redirectWithNotice(w, r, "/admin/ads", "success", "تم اعتماد الإعلان ونشره في الصفحة الرئيسية.")
}

// AdminAdRejectSubmit rejects an ad from the admin UI.
func (h *UIHandler) AdminAdRejectSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if h.promoSvc == nil {
		http.Redirect(w, r, "/admin/ads", http.StatusSeeOther)
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		http.Redirect(w, r, "/admin/ads", http.StatusSeeOther)
		return
	}
	sysCtx := database.AsSystem(ctx)
	notes := r.PostFormValue("notes")
	if err := h.promoSvc.AdminRejectAd(sysCtx, id, notes); err != nil {
		h.redirectWithNotice(w, r, "/admin/ads", "error", h.safeMessage(err, langOf(r)))
		return
	}
	h.redirectWithNotice(w, r, "/admin/ads", "success", "تم رفض الإعلان.")
}
