package ui

import (
	"net/http"

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
		pub.Use(h.LinkResponseHeadersMiddleware)
		pub.Use(h.MarkdownNegotiationMiddleware)

		// Sitemap & Agent Discovery metadata
		pub.Get("/sitemap.xml", h.SitemapXML)
		pub.Get("/llms.txt", h.LLMsTxt)
		pub.Get("/.well-known/agents-index.json", h.AgentsIndexJSON)

		// Public & Auth (marketing, catalogue browsing, sign-in, legal policies)
		pub.Get("/", h.HomePage)
		pub.Get("/privacy", h.PrivacyPage)
		pub.Get("/terms", h.TermsPage)
		pub.Get("/shipping-returns", h.ShippingReturnsPage)
		pub.Get("/refund", h.ShippingReturnsPage)
		pub.Get("/cookies", h.CookiesPolicyPage)
		pub.Get("/payment-policy", h.PaymentPolicyPage)
		pub.Get("/payments", h.PaymentPolicyPage)
		pub.Get("/policies/{slug}", h.DynamicPolicyPage)
		pub.Get("/vendor_agreement", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/terms", http.StatusMovedPermanently)
		})
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

		// The guarded routes: the catalogue listing and the supplier
		// directory, and nothing else.
		//
		// These two are the whole business asset in list form. /catalog
		// publishes supplier identity, net supply price, stock and expiry
		// across the market in one paginated view; /suppliers publishes who
		// every distributor is, where their branches are and what they cover.
		// Taken together they are the answer to "who sells what, at what
		// price, where" — which is the question this company exists to answer,
		// and the one a competitor would otherwise get for the cost of an
		// afternoon.
		//
		// The supplier profile is in the group with its listing: a directory is
		// only worth taking in bulk, and the profiles are the payload the
		// directory indexes.
		//
		// Everything else stays open on purpose. /catalog/{id} yields one
		// product per request, /offers and /jobs are already bounded
		// server-side, and the marketing pages have nothing to take. A request
		// budget on /about buys nothing and costs a middleware on every render.
		// Jobs board is open to guests (seekers and visitors)
		pub.Get("/jobs", h.JobsPage)
		pub.Get("/jobs/{id}", h.JobDetailPage)
		pub.Get("/compare/search", h.CompareQuickSearch)
		pub.Get("/api/v1/compare/search", h.CompareQuickSearch)

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
