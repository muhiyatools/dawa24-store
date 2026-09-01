package ui

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/modules/promo"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

func (h *UIHandler) HomePage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	var featured []*catalog.Product
	var categories []*catalog.Category
	var offers []*promo.Offer
	stats := pages.HomeStats{
		TotalSuppliers: 47,
		TotalProducts:  8340,
		TotalCities:    86,
		TotalOrders:    1420,
	}

	if h.catSvc != nil {
		prods, err := h.catSvc.Search(ctx, catalog.SearchParams{Limit: 8})
		if err != nil {
			h.log.WarnContext(ctx, "home page: search featured products", "error", err)
		} else {
			featured = prods
			if len(prods) > 0 {
				stats.TotalProducts = 8340 + len(prods)
			}
		}

		cats, err := h.catSvc.ListCategories(ctx)
		if err != nil {
			h.log.WarnContext(ctx, "home page: list categories", "error", err)
		} else {
			categories = cats
		}
	}

	if h.promoSvc != nil {
		if activeOffers, err := h.promoSvc.ListActiveOffers(ctx, 4, 0); err == nil {
			offers = activeOffers
			stats.TotalOffers = len(activeOffers)
		}
		// Fetch approved ads for the landing page hero showcase and advertising gallery.
		if allActiveAds, err := h.promoSvc.ListActiveAds(ctx, ""); err == nil && len(allActiveAds) > 0 {
			stats.Ads = allActiveAds
		} else {
			if heroAds, err := h.promoSvc.ListActiveAds(ctx, promo.PositionHomeHero); err == nil {
				stats.Ads = append(stats.Ads, heroAds...)
			}
			if bannerAds, err := h.promoSvc.ListActiveAds(ctx, promo.PositionHomeBanner); err == nil {
				stats.Ads = append(stats.Ads, bannerAds...)
			}
		}
		if dealsAds, err := h.promoSvc.ListActiveAds(ctx, promo.PositionHomeDeals); err == nil {
			stats.DealsAds = dealsAds
		}
		if bottomAds, err := h.promoSvc.ListActiveAds(ctx, promo.PositionHomeBottom); err == nil {
			stats.BottomAds = bottomAds
		}
	}

	if h.orgSvc != nil {
		typ := org.TypeVendor
		if orgs, err := h.orgSvc.ListOrganizations(database.AsSystem(ctx), &typ, nil, 100, 0); err == nil && len(orgs) > 0 {
			stats.TotalSuppliers = len(orgs)
		}
	}

	if cities := h.listCities(ctx); len(cities) > 0 {
		stats.TotalCities = len(cities)
	}

	if h.adminSvc != nil {
		if b, err := h.adminSvc.GetContentBlockByKey(ctx, "home-hero"); err == nil && b != nil && b.IsActive {
			stats.HeroTitle = b.Title.Get(i18n.Lang(lang))
			stats.HeroSubtitle = b.Body.Get(i18n.Lang(lang))
		}
	}

	h.renderPage(ctx, w, "render home page", pages.CustomerHome(featured, categories, offers, stats, lang, dir))
}

func (h *UIHandler) PrivacyPage(w http.ResponseWriter, r *http.Request) {
	h.renderPolicy(w, r, "privacy", "سياسة الخصوصية وحماية البيانات")
}

func (h *UIHandler) TermsPage(w http.ResponseWriter, r *http.Request) {
	h.renderPolicy(w, r, "terms", "شروط وأحكام الاستخدام")
}

func (h *UIHandler) ShippingReturnsPage(w http.ResponseWriter, r *http.Request) {
	h.renderPolicy(w, r, "shipping_return", "سياسة الشحن والتسليم والاسترجاع والإلغاء")
}

func (h *UIHandler) CookiesPolicyPage(w http.ResponseWriter, r *http.Request) {
	h.renderPolicy(w, r, "cookies", "سياسة ملفات تعريف الارتباط (Cookies)")
}

func (h *UIHandler) PaymentPolicyPage(w http.ResponseWriter, r *http.Request) {
	h.renderPolicy(w, r, "payment", "سياسة الدفع والتعاملات المالية")
}

func (h *UIHandler) DynamicPolicyPage(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	h.renderPolicy(w, r, slug, "السياسات والضوابط القانونية")
}

