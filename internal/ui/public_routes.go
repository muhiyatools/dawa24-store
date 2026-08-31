package ui

import (
	"github.com/go-chi/chi/v5"

	identityHttp "github.com/muhiya/dawa24-store/internal/modules/identity/http"
)

// RegisterPublicRoutes mounts everything a visitor may reach without signing
// in, wrapped only in OptionalAuth for the visitor analytics middleware.
// Rebuild V2 §1.3: these are the only routes without a forced audience.
func (h *UIHandler) RegisterPublicRoutes(r chi.Router) {
	// Public routes take the visitor-analytics middleware and nothing else.
	// They are mounted through Group rather than r.Use: by the time this runs
	// the root mux already carries routes, and chi panics on a Use() after the
	// first route is defined. Group gives these routes their own middleware
	// stack without touching the parent, and without wrapping them in a gate.
	// Assets are served on their own group without the audience middlewares:
	// no session lookup, branch listing or settings query is worth running for
	// every CSS/JS/image request, and they are typically the majority of
	// traffic.
	r.Group(func(assets chi.Router) {
		RegisterStaticRoutes(assets)
		RegisterUploadRoutes(assets)
	})

	r.Group(func(pub chi.Router) {
		if h.idSvc != nil {
			pub.Use(identityHttp.OptionalAuth(h.idSvc, h.resolver, "dawa24_session", h.log))
		}
		pub.Use(h.BuyingBranchSelector)
		pub.Use(h.siteSettingsMiddleware)
		pub.Use(h.visitorMiddleware)

		// Public & Auth (marketing, catalogue browsing, sign-in)
		pub.Get("/", h.HomePage)
		pub.Get("/privacy", h.PrivacyPage)
		pub.Get("/terms", h.TermsPage)
		pub.Get("/about", h.AboutPage)
		pub.Get("/how-it-works", h.HowItWorksPage)
		pub.Get("/faq", h.FaqPage)
		pub.Get("/contact", h.ContactPage)
		pub.Get("/auth/login", h.LoginPage)
		pub.Get("/auth/mfa-verify", h.MFAVerifyPage)
		pub.Get("/auth/register", h.RegisterPage)
		pub.Get("/auth/forgot", h.ForgotPasswordPage)
		pub.Get("/auth/reset", h.ResetPasswordPage)
		pub.Get("/onboarding", h.OnboardingPage)
		pub.Get("/lang/{code}", h.SetLanguage)

		// The bait path. It exists only to be followed: robots.txt forbids it
		// and the single link to it is hidden from anyone using the page. A
		// caller that fetches it has both ignored robots.txt and harvested
		// every href in the HTML, which is the definition of the thing this
		// guard is for, so it is put on the refused list for an hour.
		pub.Get("/trap/*", h.ScrapeTrap)

		// Everything below publishes marketplace data — supplier identity, net
		// supply price, stock, expiry — to callers who have not signed in, and
		// is therefore what a scraper comes for. The guard meters it; the rest
		// of the public surface (marketing copy, the sign-in form) is left
		// alone, because a request budget on /about buys nothing and costs a
		// middleware on every render.
		pub.Group(func(data chi.Router) {
			data.Use(h.scrape.Protect)

			// Public catalogue and directory
			data.Get("/catalog", h.CustomerCatalogPage)
			data.Get("/catalog/{id}", h.CustomerProductDetailPage)
			data.Get("/suppliers", h.SuppliersPage)
			data.Get("/suppliers/{id}", h.SupplierProfilePage)
			data.Get("/offers", h.OffersPage)
			data.Get("/offers/{id}", h.OfferDetailPage)
			data.Get("/jobs", h.JobsPage)
			data.Get("/jobs/{id}", h.JobDetailPage)
		})

		// The two search endpoints answer a bare URL with structured JSON,
		// which makes them the cheapest thing here to harvest and the only
		// ones that also have to prove they were called from a page of this
		// site rather than from a script holding the URL.
		pub.Group(func(search chi.Router) {
			search.Use(h.scrape.Protect)
			search.Use(h.scrape.RequireSiteOrigin)

			search.Get("/compare/search", h.CompareQuickSearch)
			search.Get("/api/v1/compare/search", h.CompareQuickSearch)
		})

		// Unlisted Courier Delivery Portal (Dedicated Delivery Representative Interface)
		pub.Get("/delivery", h.CourierDeliveryPage)
		pub.Post("/delivery/verify", h.CourierVerifyDeliverySubmit)
		pub.Get("/compare", h.ComparePlansPage)
		pub.Post("/compare/subscribe", h.CompareSubscribeSubmit)
		pub.Get("/compare/tool", h.CompareToolPage)
		pub.Get("/compare/sample", h.CompareSampleDownload)
		pub.Get("/compare/template", h.CompareSampleDownload)
		pub.Post("/compare/upload", h.CompareUploadSubmit)
		pub.Post("/compare/files/{id}/rename", h.CompareFileRenameSubmit)
		pub.Post("/compare/file/{id}/rename", h.CompareFileRenameSubmit)
		pub.Post("/compare/files/{id}/archive", h.CompareFileArchiveSubmit)
		pub.Post("/compare/file/{id}/archive", h.CompareFileArchiveSubmit)
		pub.Post("/compare/files/{id}/unarchive", h.CompareFileUnarchiveSubmit)
		pub.Post("/compare/file/{id}/unarchive", h.CompareFileUnarchiveSubmit)
		pub.Post("/compare/files/{id}/delete", h.CompareFileDeleteSubmit)
		pub.Post("/compare/file/{id}/delete", h.CompareFileDeleteSubmit)
		pub.Post("/compare/files/{id}/visibility", h.CompareFileVisibilitySubmit)
		pub.Post("/compare/file/{id}/visibility", h.CompareFileVisibilitySubmit)
		pub.Post("/compare/files/{id}/skip", h.CompareFileSkipSubmit)
		pub.Post("/compare/file/{id}/skip", h.CompareFileSkipSubmit)
		pub.Get("/compare/files/{id}/mapping", h.CompareFileMappingPage)
		pub.Get("/compare/file/{id}/mapping", h.CompareFileMappingPage)
		pub.Get("/compare/files/{id}/mapping-modal", h.CompareFileMappingModal)
		pub.Get("/compare/file/{id}/mapping-modal", h.CompareFileMappingModal)
		pub.Post("/compare/files/{id}/mapping", h.CompareFileMappingSubmit)
		pub.Post("/compare/file/{id}/mapping", h.CompareFileMappingSubmit)
		pub.Post("/compare/rows/{id}/match", h.CompareRowManualMatchSubmit)
		pub.Post("/compare/files/{id}/match", h.CompareFileMatchSubmit)
		pub.Post("/compare/file/{id}/match", h.CompareFileMatchSubmit)
		pub.Post("/compare/run", h.CompareRunSubmit)
		pub.Get("/compare/results", h.CompareResultsPage)
		pub.Get("/compare/head-to-head", h.CompareHeadToHeadPage)
		pub.Get("/compare/market-benchmark", h.CompareMarketBenchmarkPage)
		pub.Get("/compare/market-intelligence", h.CompareMarketIntelligencePage)
		pub.Get("/market-discounts", h.MarketDiscountsPage)
		pub.Get("/tracking", h.GuestOrderTrackingPage)
		pub.Get("/promotions/track-click/{offer}", h.PublicPromotionTrackClick)
		pub.Get("/promotions/track-click/{offer}/{promotion}", h.PublicPromotionTrackClick)
		pub.Get("/ads/click/{ad}", h.PublicAdClick)

		// Form actions that work signed-out (sign-up must be reachable pre-login)
		pub.Post("/auth/login", h.LoginSubmit)
		pub.Post("/auth/mfa-verify", h.MFAVerifySubmit)
		pub.Post("/auth/logout", h.LogoutSubmit)
		pub.Get("/auth/logout", h.LogoutSubmit)
		pub.Post("/auth/register", h.RegisterSubmit)
		pub.Post("/contact", h.ContactSubmit)
		pub.Post("/upload", h.UploadAPISubmit)
		pub.Post("/offers/{id}/click", h.OfferClickSubmit)
		pub.Post("/jobs/{id}/apply", h.JobApplySubmit)
	})
}
