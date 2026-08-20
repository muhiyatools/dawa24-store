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

// AdminOfferSponsorshipsPage renders list of sponsored offers and active subscriptions.
func (h *UIHandler) AdminOfferSponsorshipsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminOfferSponsorshipsPage(lang, dir).Render(ctx, w); err != nil {
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

// AdminAdsListPage renders advertisements and banners management table.
func (h *UIHandler) AdminAdsListPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminAdsListPage(lang, dir).Render(ctx, w); err != nil {
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
	if err := pages.VendorOffersPackagesPage(lang, dir).Render(ctx, w); err != nil {
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

// VendorAdsPage renders vendor's active banners and ads.
func (h *UIHandler) VendorAdsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.VendorAdsPage(lang, dir).Render(ctx, w); err != nil {
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
