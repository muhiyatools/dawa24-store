package ui

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
)

func (h *UIHandler) registerAdminCommerceRoutes(r chi.Router) {
	// Orders & Procurement
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("commerce.order.view"))
		g.Get("/admin/orders", h.AdminOrdersPage)
		g.Get("/admin/orders/offers", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/admin/orders?tab=negotiations", http.StatusMovedPermanently)
		})
		g.Get("/admin/orders/offers/{id}", func(w http.ResponseWriter, r *http.Request) {
			id := chi.URLParam(r, "id")
			http.Redirect(w, r, "/orders/"+id, http.StatusMovedPermanently)
		})
	})

	// Offers & Promotions
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("promo.offer.view"))
		g.Get("/admin/offers", h.AdminOffersPage)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("promo.offer.update"))
		g.Post("/admin/offers/{id}/status", h.AdminOfferStatusSubmit)
		g.Post("/admin/offers/{id}/approve", h.AdminOfferApproveSubmit)
		g.Post("/admin/offers/{id}/reject", h.AdminOfferRejectSubmit)
	})

	// Finance Hub, Invoices, Payments, Wallets & Earnings.
	//
	// Reading the finance hub and moving money through it are different
	// rights. Adjusting a wallet balance and approving a deposit change what a
	// tenant may spend; they sit behind their own grants, and the starter
	// "Administrator" role does not hold the wallet one.
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission(
			"billing.finance.view", "billing.invoice.view", "billing.payment.view", "billing.wallet.read"))
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

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("billing.wallet.manage"))
		g.Post("/admin/finance/wallets/{id}/adjust", h.AdminWalletAdjustSubmit)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("billing.payment.update"))
		g.Post("/admin/finance/deposits/{id}/approve", h.AdminDepositApproveSubmit)
		g.Post("/admin/finance/deposits/{id}/reject", h.AdminDepositRejectSubmit)
	})

	// Plans & Subscriptions Hub
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("billing.subscription_plan.view"))
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
		g.Use(authctx.RequirePagePermission("billing.subscription_plan.update"))
		g.Post("/admin/plans", h.AdminPlanSubmit)
		g.Post("/admin/plans/{id}/update", h.AdminPlanUpdateSubmit)
		g.Post("/admin/plans/{id}/toggle", h.AdminPlanToggleSubmit)
		g.Post("/admin/plans/{id}/set-default", h.AdminPlanSetDefaultSubmit)
		g.Post("/admin/plans/{id}/delete", h.AdminPlanDeleteSubmit)
	})

	// Offer packages, sponsorships, promotions and their analytics.
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("promo.offer_package.view"))
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
		g.Get("/admin/offers-packages/views", h.AdminOfferAnalyticsViewsPage)
		g.Get("/admin/offers-packages/views/{id}", h.AdminOfferAnalyticsViewsPage)
		g.Get("/admin/offers-packages/clicks", h.AdminOfferAnalyticsClicksPage)
		g.Get("/admin/offers-packages/clicks/{id}", h.AdminOfferAnalyticsClicksPage)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("promo.ad.view"))
		g.Get("/admin/ads", h.AdminAdsListPage)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("promo.ad_plan.view"))
		g.Get("/admin/ad-plan", h.AdminAdPlansPage)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("promo.offer_location.view"))
		g.Get("/admin/offers/locations", h.AdminOfferLocationsPage)
		g.Get("/admin/offers/{id}/locations", h.AdminOfferLocationsPage)
	})
}
