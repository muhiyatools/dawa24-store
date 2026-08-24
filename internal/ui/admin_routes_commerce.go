package ui

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
)

func (h *UIHandler) registerAdminCommerceRoutes(r chi.Router) {
	// Orders & Procurement
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("commerce.order.view", h.log))
		g.Get("/admin/orders", h.AdminOrdersPage)
		g.Get("/admin/orders/offers", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/admin/orders?tab=offers", http.StatusMovedPermanently)
		})
		g.Get("/admin/orders/offers/{id}", h.AdminOfferOrderDetailPage)
	})

	// Offers & Promotions
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("promo.offer.view", h.log))
		g.Get("/admin/offers", h.AdminOffersPage)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("promo.offer.update", h.log))
		g.Post("/admin/offers/{id}/status", h.AdminOfferStatusSubmit)
	})

	// Finance Hub, Invoices, Payments, Wallets & Earnings
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("billing.invoice.view", h.log))
		g.Get("/admin/finance", h.AdminFinancePage)
		g.Get("/admin/earnings/order", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/admin/finance?tab=earnings", http.StatusMovedPermanently)
		})
		g.Get("/admin/earnings/offers", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/admin/finance?tab=earnings", http.StatusMovedPermanently)
		})
		g.Get("/admin/invoices", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/admin/finance?tab=invoices", http.StatusMovedPermanently)
		})
		g.Get("/admin/payments", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/admin/finance?tab=payments", http.StatusMovedPermanently)
		})
		g.Get("/admin/wallets", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/admin/finance?tab=wallets", http.StatusMovedPermanently)
		})
	})

	// Plans & Subscriptions Hub
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("billing.subscription_plan.view", h.log))
		g.Get("/admin/plans", h.AdminPlansPage)
		g.Get("/admin/plans-info", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/admin/plans?tab=plans", http.StatusMovedPermanently)
		})
		g.Get("/admin/plan-types", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/admin/plans?tab=plans", http.StatusMovedPermanently)
		})
		g.Get("/admin/plan-features", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/admin/plans?tab=plans", http.StatusMovedPermanently)
		})
		g.Get("/admin/plans/subscriptions", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/admin/plans?tab=subscriptions", http.StatusMovedPermanently)
		})
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("billing.subscription_plan.update", h.log))
		g.Post("/admin/plans", h.AdminPlanSubmit)
		g.Post("/admin/plans/{id}/update", h.AdminPlanUpdateSubmit)
	})

	// Phase 8: Offer Packages, Sponsorships, Promotions, Ads, Analytics, Locations
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("promo.offer.view", h.log))
		g.Get("/admin/offers-packages", h.AdminOffersPackagesHubPage)
		g.Get("/admin/offers-packages/packages", h.AdminOfferPackagesListPage)
		g.Get("/admin/offers-packages/packages/{id}", h.AdminOfferPackageDetailPage)
		g.Get("/admin/offer-sponsorships", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/admin/offers-packages/sponsorships", http.StatusMovedPermanently)
		})
		g.Get("/admin/offer-sponsorships/{id}", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/admin/offers-packages/sponsorships", http.StatusMovedPermanently)
		})
		g.Get("/admin/offers-packages/sponsorships", h.AdminOfferSponsorshipsPage)
		g.Get("/admin/offers-packages/sponsorships/{id}", h.AdminOfferSponsorshipsPage)
		g.Get("/admin/offers-packages/promotions", h.AdminOfferPromotionsPage)
		g.Get("/admin/offers-packages/promotions/{id}", h.AdminOfferPromotionsPage)
		g.Get("/admin/ads", h.AdminAdsListPage)
		g.Get("/admin/ad-plan", h.AdminAdPlansPage)
		g.Get("/admin/offers-packages/views", h.AdminOfferAnalyticsViewsPage)
		g.Get("/admin/offers-packages/views/{id}", h.AdminOfferAnalyticsViewsPage)
		g.Get("/admin/offers-packages/clicks", h.AdminOfferAnalyticsClicksPage)
		g.Get("/admin/offers-packages/clicks/{id}", h.AdminOfferAnalyticsClicksPage)
		g.Get("/admin/offers/locations", h.AdminOfferLocationsPage)
		g.Get("/admin/offers/{id}/locations", h.AdminOfferLocationsPage)
	})
}
