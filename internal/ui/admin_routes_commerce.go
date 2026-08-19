package ui

import (
	"github.com/go-chi/chi/v5"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
)

func (h *UIHandler) registerAdminCommerceRoutes(r chi.Router) {
	// Orders & Procurement
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("commerce.order.view", h.log))
		g.Get("/admin/orders", h.AdminOrdersPage)
		g.Get("/admin/orders/offers", h.AdminOfferOrdersPage)
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

	// Finance & Earnings & Invoices & Wallets
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("billing.invoice.view", h.log))
		g.Get("/admin/earnings/order", h.AdminEarningsOrderPage)
		g.Get("/admin/earnings/offers", h.AdminEarningsOffersPage)
		g.Get("/admin/invoices", h.AdminInvoicesPage)
		g.Get("/admin/payments", h.AdminPaymentsPage)
		g.Get("/admin/wallets", h.AdminWalletsPage)
	})

	// Plans & Subscriptions
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("billing.subscription_plan.view", h.log))
		g.Get("/admin/plans", h.AdminPlansPage)
		g.Get("/admin/plans-info", h.AdminPlansInfoPage)
		g.Get("/admin/plan-types", h.AdminPlanTypesPage)
		g.Get("/admin/plan-features", h.AdminPlanFeaturesPage)
		g.Get("/admin/plans/subscriptions", h.AdminPlansSubscriptionsPage)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("billing.subscription_plan.update", h.log))
		g.Post("/admin/plans", h.AdminPlanSubmit)
	})

	// Phase 8: Offer Packages, Sponsorships, Promotions, Ads, Analytics, Locations
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("promo.offer.view", h.log))
		g.Get("/admin/offers-packages", h.AdminOffersPackagesHubPage)
		g.Get("/admin/offers-packages/packages", h.AdminOfferPackagesListPage)
		g.Get("/admin/offers-packages/packages/{id}", h.AdminOfferPackageDetailPage)
		g.Get("/admin/offer-sponsorships", h.AdminOfferSponsorshipsPage)
		g.Get("/admin/offer-sponsorships/{id}", h.AdminOfferSponsorshipsPage)
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
