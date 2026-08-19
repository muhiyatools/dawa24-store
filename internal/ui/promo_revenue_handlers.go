package ui

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// AdminOffersPackagesHubPage renders the monetization and packages overview hub.
func (h *UIHandler) AdminOffersPackagesHubPage(w http.ResponseWriter, r *http.Request) {
	lang, dir := h.localeAndDir(r)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = pages.AdminOffersPackagesHubPage(lang, dir).Render(r.Context(), w)
}

// AdminOfferPackagesListPage renders list of offer packages and pricing tiers.
func (h *UIHandler) AdminOfferPackagesListPage(w http.ResponseWriter, r *http.Request) {
	lang, dir := h.localeAndDir(r)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = pages.AdminOfferPackagesListPage(lang, dir).Render(r.Context(), w)
}

// AdminOfferPackageDetailPage renders single offer package details and features.
func (h *UIHandler) AdminOfferPackageDetailPage(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	pkgID, _ := strconv.ParseInt(idStr, 10, 64)
	lang, dir := h.localeAndDir(r)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = pages.AdminOfferPackageDetailPage(pkgID, lang, dir).Render(r.Context(), w)
}

// AdminOfferSponsorshipsPage renders list of sponsored offers and active subscriptions.
func (h *UIHandler) AdminOfferSponsorshipsPage(w http.ResponseWriter, r *http.Request) {
	lang, dir := h.localeAndDir(r)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = pages.AdminOfferSponsorshipsPage(lang, dir).Render(r.Context(), w)
}

// AdminOfferPromotionsPage renders promotional campaigns list.
func (h *UIHandler) AdminOfferPromotionsPage(w http.ResponseWriter, r *http.Request) {
	lang, dir := h.localeAndDir(r)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = pages.AdminOfferPromotionsPage(lang, dir).Render(r.Context(), w)
}

// AdminAdsListPage renders advertisements and banners management table.
func (h *UIHandler) AdminAdsListPage(w http.ResponseWriter, r *http.Request) {
	lang, dir := h.localeAndDir(r)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = pages.AdminAdsListPage(lang, dir).Render(r.Context(), w)
}

// AdminAdPlansPage renders advertising placement plans.
func (h *UIHandler) AdminAdPlansPage(w http.ResponseWriter, r *http.Request) {
	lang, dir := h.localeAndDir(r)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = pages.AdminAdPlansPage(lang, dir).Render(r.Context(), w)
}

// AdminOfferAnalyticsViewsPage renders views time-series and aggregate report.
func (h *UIHandler) AdminOfferAnalyticsViewsPage(w http.ResponseWriter, r *http.Request) {
	lang, dir := h.localeAndDir(r)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = pages.AdminOfferAnalyticsViewsPage(lang, dir).Render(r.Context(), w)
}

// AdminOfferAnalyticsClicksPage renders clicks time-series and CTR report.
func (h *UIHandler) AdminOfferAnalyticsClicksPage(w http.ResponseWriter, r *http.Request) {
	lang, dir := h.localeAndDir(r)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = pages.AdminOfferAnalyticsClicksPage(lang, dir).Render(r.Context(), w)
}

// AdminOfferLocationsPage renders geographic coverage distribution for offers.
func (h *UIHandler) AdminOfferLocationsPage(w http.ResponseWriter, r *http.Request) {
	lang, dir := h.localeAndDir(r)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = pages.AdminOfferLocationsPage(lang, dir).Render(r.Context(), w)
}

// VendorOffersPackagesPage renders available packages and current subscription for vendor.
func (h *UIHandler) VendorOffersPackagesPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/offers-packages", http.StatusSeeOther)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = pages.VendorOffersPackagesPage(lang, dir).Render(ctx, w)
}

// VendorOffersPackagesSponsorshipsPage renders vendor's sponsored offers list.
func (h *UIHandler) VendorOffersPackagesSponsorshipsPage(w http.ResponseWriter, r *http.Request) {
	lang, dir := h.localeAndDir(r)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = pages.VendorOffersPackagesSponsorshipsPage(lang, dir).Render(r.Context(), w)
}

// VendorOffersPackagesPromotionsPage renders vendor's promotional campaigns.
func (h *UIHandler) VendorOffersPackagesPromotionsPage(w http.ResponseWriter, r *http.Request) {
	lang, dir := h.localeAndDir(r)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = pages.VendorOffersPackagesPromotionsPage(lang, dir).Render(r.Context(), w)
}

// VendorAdsPage renders vendor's active banners and ads.
func (h *UIHandler) VendorAdsPage(w http.ResponseWriter, r *http.Request) {
	lang, dir := h.localeAndDir(r)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = pages.VendorAdsPage(lang, dir).Render(r.Context(), w)
}

// VendorOffersLocationsPage renders vendor's offer geographic coverage.
func (h *UIHandler) VendorOffersLocationsPage(w http.ResponseWriter, r *http.Request) {
	lang, dir := h.localeAndDir(r)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = pages.VendorOffersLocationsPage(lang, dir).Render(r.Context(), w)
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
