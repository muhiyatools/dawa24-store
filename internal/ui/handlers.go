package ui

import (
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/assistant"
	"github.com/muhiya/dawa24-store/internal/modules/attachments"
	"github.com/muhiya/dawa24-store/internal/modules/billing"
	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/modules/chat"
	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/modules/compare"
	"github.com/muhiya/dawa24-store/internal/modules/hr"
	"github.com/muhiya/dawa24-store/internal/modules/identity"
	identityHttp "github.com/muhiya/dawa24-store/internal/modules/identity/http"
	"github.com/muhiya/dawa24-store/internal/modules/ingest"
	"github.com/muhiya/dawa24-store/internal/modules/inventory"
	"github.com/muhiya/dawa24-store/internal/modules/notifications"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	platformadmin "github.com/muhiya/dawa24-store/internal/modules/platform_admin"
	"github.com/muhiya/dawa24-store/internal/modules/promo"
	"github.com/muhiya/dawa24-store/internal/modules/workflow"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/gateway"
	"github.com/muhiya/dawa24-store/internal/platform/storage"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/ui/components"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// UIHandler serves server-rendered HTML pages via Templ.
type UIHandler struct {
	catSvc        *catalog.Service
	orgSvc        *org.Service
	ingSvc        *ingest.Service
	commSvc       *commerce.Service
	invSvc        *inventory.Service
	idSvc         *identity.Service
	notifSvc      *notifications.Service
	promoSvc      *promo.Service
	adminSvc      *platformadmin.Service
	billSvc       *billing.Service
	compareSvc    *compare.Service
	chatSvc       *chat.Service
	assistantRepo assistant.Repository
	wfSvc         *workflow.Service
	hrSvc         *hr.Service
	attSvc        *attachments.Service
	storage       *storage.Client
	aiClient      gateway.Client
	log           *slog.Logger
}

// SetAssistantRepository attaches the Assistant database repository for auditing and history.
func (h *UIHandler) SetAssistantRepository(repo assistant.Repository) {
	h.assistantRepo = repo
}

// SetGatewayClient attaches the Gateway client instance for health probes and AI services.
func (h *UIHandler) SetGatewayClient(ai gateway.Client) {
	h.aiClient = ai
}

// NewUIHandler creates a new UI page handler with all platform domain services wired.
func NewUIHandler(
	catSvc *catalog.Service,
	orgSvc *org.Service,
	ingSvc *ingest.Service,
	commSvc *commerce.Service,
	invSvc *inventory.Service,
	idSvc *identity.Service,
	notifSvc *notifications.Service,
	promoSvc *promo.Service,
	adminSvc *platformadmin.Service,
	billSvc *billing.Service,
	chatSvc *chat.Service,
	wfSvc *workflow.Service,
	hrSvc *hr.Service,
	attSvc *attachments.Service,
	log *slog.Logger,
) *UIHandler {
	if log == nil {
		log = slog.Default()
	}
	return &UIHandler{
		catSvc:   catSvc,
		orgSvc:   orgSvc,
		ingSvc:   ingSvc,
		commSvc:  commSvc,
		invSvc:   invSvc,
		idSvc:    idSvc,
		notifSvc: notifSvc,
		promoSvc: promoSvc,
		adminSvc: adminSvc,
		billSvc:  billSvc,
		chatSvc:  chatSvc,
		wfSvc:    wfSvc,
		hrSvc:    hrSvc,
		attSvc:   attSvc,
		log:      log,
	}
}

// SetStorage configures object storage (MinIO/S3) for UI handlers.
func (h *UIHandler) SetStorage(s *storage.Client) {
	h.storage = s
}

// SetCompareService configures the compare module service for UI handlers.
func (h *UIHandler) SetCompareService(s *compare.Service) {
	h.compareSvc = s
}

// SiteSettingsMiddleware injects live SiteSettings from database into every request context.
func (h *UIHandler) SiteSettingsMiddleware(next http.Handler) http.Handler {
	return h.siteSettingsMiddleware(next)
}

// RegisterPublicRoutes mounts everything a visitor may reach without signing
// in, wrapped only in OptionalAuth for the visitor analytics middleware.
// Rebuild V2 §1.3: these are the only routes without a forced audience.
func (h *UIHandler) RegisterPublicRoutes(r chi.Router) {
	// Public routes take the visitor-analytics middleware and nothing else.
	// They are mounted through Group rather than r.Use: by the time this runs
	// the root mux already carries routes, and chi panics on a Use() after the
	// first route is defined. Group gives these routes their own middleware
	// stack without touching the parent, and without wrapping them in a gate.
	r.Group(func(pub chi.Router) {
		if h.idSvc != nil {
			pub.Use(identityHttp.OptionalAuth(h.idSvc, "dawa24_session"))
		}
		pub.Use(h.BuyingBranchSelector)
		pub.Use(h.siteSettingsMiddleware)
		pub.Use(h.visitorMiddleware)
		RegisterStaticRoutes(pub)
		RegisterUploadRoutes(pub)

		// Public & Auth (marketing, catalogue browsing, sign-in)
		pub.Get("/", h.HomePage)
		pub.Get("/privacy", h.PrivacyPage)
		pub.Get("/terms", h.TermsPage)
		pub.Get("/about", h.AboutPage)
		pub.Get("/how-it-works", h.HowItWorksPage)
		pub.Get("/faq", h.FaqPage)
		pub.Get("/help", h.HelpPage)
		pub.Get("/contact", h.ContactPage)
		pub.Get("/auth/login", h.LoginPage)
		pub.Get("/auth/register", h.RegisterPage)
		pub.Get("/auth/forgot", h.ForgotPasswordPage)
		pub.Get("/auth/reset", h.ResetPasswordPage)
		pub.Get("/onboarding", h.OnboardingPage)
		pub.Get("/lang/{code}", h.SetLanguage)

		// Public catalogue and directory
		pub.Get("/catalog", h.CustomerCatalogPage)
		pub.Get("/catalog/{id}", h.CustomerProductDetailPage)
		pub.Get("/suppliers", h.SuppliersPage)
		pub.Get("/suppliers/{id}", h.SupplierProfilePage)
		pub.Get("/offers", h.OffersPage)
		pub.Get("/offers/{id}", h.OfferDetailPage)
		pub.Get("/jobs", h.JobsPage)
		pub.Get("/jobs/{id}", h.JobDetailPage)
		pub.Get("/services", h.ServicesPage)
		pub.Get("/services/{id}", h.ServiceDetailPage)
		pub.Get("/finder", h.FinderPage)
		pub.Get("/finder/{id}", h.FinderQuestionByIDPage)
		pub.Get("/finder/result/{id}", h.FinderResultByIDPage)
		pub.Get("/compare", h.ComparePlansPage)
		pub.Post("/compare/subscribe", h.CompareSubscribeSubmit)
		pub.Get("/compare/tool", h.CompareToolPage)
		pub.Get("/compare/search", h.CompareQuickSearch)
		pub.Get("/api/v1/compare/search", h.CompareQuickSearch)
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
		pub.Get("/compare/files/{id}/mapping", h.CompareFileMappingPage)
		pub.Get("/compare/file/{id}/mapping", h.CompareFileMappingPage)
		pub.Get("/compare/files/{id}/mapping-modal", h.CompareFileMappingModal)
		pub.Get("/compare/file/{id}/mapping-modal", h.CompareFileMappingModal)
		pub.Post("/compare/files/{id}/mapping", h.CompareFileMappingSubmit)
		pub.Post("/compare/file/{id}/mapping", h.CompareFileMappingSubmit)
		pub.Post("/compare/rows/{id}/match", h.CompareRowManualMatchSubmit)
		pub.Post("/compare/run", h.CompareRunSubmit)
		pub.Get("/compare/results", h.CompareResultsPage)
		pub.Get("/compare/head-to-head", h.CompareHeadToHeadPage)
		pub.Get("/market-discounts", h.MarketDiscountsPage)
		pub.Get("/tracking", h.GuestOrderTrackingPage)
		pub.Get("/promotions/track-click/{offer}", h.PublicPromotionTrackClick)
		pub.Get("/promotions/track-click/{offer}/{promotion}", h.PublicPromotionTrackClick)
		pub.Get("/ads/click/{ad}", h.PublicAdClick)

		// Form actions that work signed-out (sign-up must be reachable pre-login)
		pub.Post("/auth/login", h.LoginSubmit)
		pub.Post("/auth/logout", h.LogoutSubmit)
		pub.Get("/auth/logout", h.LogoutSubmit)
		pub.Post("/auth/register", h.RegisterSubmit)
		pub.Post("/contact", h.ContactSubmit)
		pub.Post("/upload", h.UploadAPISubmit)
		pub.Post("/offers/{id}/click", h.OfferClickSubmit)
		pub.Post("/services/{id}/request", h.ServiceRequestSubmit)
		pub.Post("/jobs/{id}/apply", h.JobApplySubmit)
	})
}

// RegisterCustomerRoutes mounts the customer (صيدلية) surface. The plan's
// reported bug lived here: a pharmacy account could previously open every
// /vendor/* page because nothing stopped the vendor screen rendering. The
// group is gated by RequireCustomer — a vendor who spells /customer/* gets the
// same 404 as a stranger (the URL space does not exist for them).
func (h *UIHandler) RegisterCustomerRoutes(r chi.Router) {
	r.Get("/customer/dashboard", h.PharmacyDashboardPage)
	// Legacy URL kept as a redirect so bookmarks survive the rename.
	r.Get("/pharmacy/dashboard", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/customer/dashboard", http.StatusFound)
	})

	// The cPanel was a link hub to /customer/branches, /settings/organization
	// and /wallet — all three already in the sidebar. Nothing on it was
	// reachable only from there (PLAN_V7 Task 3.2).
	r.Get("/customer/cpanel", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/customer/dashboard", http.StatusMovedPermanently)
	})
	r.Get("/customer/saving-products", h.CustomerSavingProductsPage)
	r.Get("/customer/saving-products/import", h.CustomerSavingProductsImportPage)
	r.Get("/customer/saving-products/{id}", h.CustomerSavingProductDetailPage)
	r.Get("/customer/saveing-products", h.CustomerSavingProductsAlias)
	r.Get("/customer/add-order", h.CustomerAddOrderPage)
	r.Get("/customer/products/main/{id}", h.CustomerProductsMainAlias)

	r.Post("/customer/set-branch", h.SetBuyingBranchSubmit)

	r.Get("/cart", h.CustomerCartPage)
	r.Get("/checkout", h.CustomerCheckoutPage)
	r.Get("/offers/{id}/checkout", h.CustomerOfferCheckoutPage)
	r.Get("/orders", h.CustomerOrdersPage)
	r.Get("/orders/offers", h.CustomerOfferOrdersPage)
	r.Get("/orders/offers/{id}", h.CustomerOfferOrderDetailPage)
	r.Get("/orders/{id}", h.CustomerOrderDetailPage)
	r.Get("/favorites", h.FavoritesPage)
	r.Get("/suppliers/followed", h.FollowedSuppliersPage)

	r.Post("/cart/add", h.AddToCartSubmit)
	r.Post("/cart/remove", h.RemoveFromCartSubmit)
	r.Post("/cart/update-quantity", h.UpdateCartQuantitySubmit)
	r.Post("/checkout", h.CheckoutSubmit)
	r.Post("/favorites/{id}/remove", h.FavoriteRemoveSubmit)
	r.Post("/favorites/{id}/add", h.FavoriteAddSubmit)
	r.Post("/favorites/{id}/toggle", h.FavoriteToggleSubmit)
	r.Post("/favorites/toggle", h.FavoriteToggleSubmit)

	// Organization documents (Rebuild V2 §4.2) — the upload/delete POSTs live
	// in both audience groups, the page itself is audience-prefixed.
	r.Get("/customer/documents", h.OrganizationDocumentsPage)
	r.Post("/documents/upload", h.OrganizationDocumentsUploadSubmit)
	r.Post("/documents/delete", h.OrganizationDocumentDeleteSubmit)

	// Pharmacy Branches & Delivery Receiving Locations (Rebuild V2 §5)
	r.Get("/customer/branches", h.CustomerBranchesPage)
	r.Post("/customer/branches/active", h.CustomerSwitchActiveBranchSubmit)
	r.Post("/customer/branches/new", h.CustomerBranchNewSubmit)
	r.Post("/customer/branches/{id}/delete", h.CustomerBranchDeleteSubmit)

	// Customer interactions
	r.Post("/suppliers/{id}/follow", h.SupplierFollowSubmit)
	r.Post("/suppliers/{id}/message", h.SupplierMessageSubmit)
	r.Post("/suppliers/{id}/quote", h.SupplierQuoteSubmit)
	r.Post("/finder/answer", h.FinderAnswerSubmit)
	r.Post("/compare/subscribe", h.CompareSubscribeSubmit)
	r.Post("/reviews/submit", h.ReviewSubmit)

	// Customer Purchase Requests Gateway (Plan V5 Phase 3 Task 3.1)
	r.Get("/customer/purchase-request", h.CustomerPurchaseRequestWizardPage)
	r.Get("/customer/purchase-request/products", h.CustomerPurchaseRequestProductsRedirect)
	r.Get("/customer/purchase-request/previous", h.CustomerPurchaseRequestPreviousRedirect)
	r.Get("/customer/purchase-request/supplier", h.CustomerPurchaseRequestSupplierRedirect)
	r.Get("/customer/purchase-request/supplier/{id}", h.CustomerPurchaseRequestSupplierRedirect)

	// Customer Purchase Priority Engine (Plan V5 Phase 3 Task 3.2)
	r.Get("/customer/purchase-priority", h.CustomerPurchasePriorityPage)
	r.Post("/customer/purchase-priority/run", h.CustomerPurchasePriorityRunSubmit)
	r.Get("/customer/purchase-priority/{id}", h.CustomerPurchasePriorityDetailPage)

	// Automatic Purchase Requests & Optimization (Plan V5 Phase 3 Task 3.3)
	r.Get("/customer/automation", h.CustomerAutomationPage)
	r.Post("/customer/automation/upload", h.CustomerAutomationUploadSubmit)
	r.Get("/customer/automation/previous", h.CustomerAutomationPreviousPage)
	r.Get("/customer/automation/{id}", h.CustomerAutomationDetailPage)
}

// RegisterVendorRoutes mounts the vendor (مورّد) surface, gated by
// RequireVendor.
func (h *UIHandler) RegisterVendorRoutes(r chi.Router) {
	r.Get("/vendor/dashboard", h.VendorDashboardPage)
	r.Get("/vendor/organization", h.VendorOrganizationPage)
	r.Post("/vendor/organization", h.VendorOrganizationSubmit)
	r.Get("/vendor/settings/organization", h.VendorOrganizationPage)
	r.Get("/vendor/products", h.VendorProductsPage)
	r.Get("/vendor/products/new", h.VendorVariantNewPage)
	r.Get("/vendor/variants/new", h.VendorVariantNewPage)
	r.Post("/vendor/variants/new", h.VendorVariantNewSubmit)
	r.Post("/vendor/variants/{id}/delete", h.VendorVariantDeleteSubmit)
	r.Get("/vendor/catalog/select", h.VendorCatalogSelectPage)
	r.Post("/vendor/catalog/select", h.VendorCatalogSelectSubmit)
	r.Get("/vendor/branches", h.VendorBranchesPage)
	r.Post("/vendor/branches/new", h.VendorBranchNewSubmit)
	r.Post("/vendor/branches/{id}/delete", h.VendorBranchDeleteSubmit)
	r.Post("/vendor/branches/{id}/manager", h.SettingsBranchManagerAssignSubmit)
	r.Get("/vendor/team", h.VendorTeamPage)
	r.Get("/vendor/team/import", h.VendorTeamImportPage)
	r.Get("/vendor/team/fast-add", h.VendorTeamFastAddPage)
	r.Get("/vendor/team/{id}", h.VendorTeamUserDetailPage)
	r.Get("/vendor/team/{id}/info", h.VendorTeamUserInfoPage)
	r.Get("/vendor/roles", h.VendorRolesPage)
	r.Post("/vendor/team/new", h.VendorTeamNewSubmit)
	r.Post("/vendor/team/{id}/toggle", h.VendorTeamToggleSubmit)
	r.Post("/vendor/team/{id}/delete", h.VendorTeamDeleteSubmit)
	r.Get("/vendor/inventory", h.VendorInventoryPage)
	r.Post("/vendor/inventory/{id}/adjust", h.VendorStockAdjustSubmit)
	r.Get("/vendor/warehouses", h.VendorWarehousesPage)
	r.Post("/vendor/warehouses", h.VendorWarehouseCreateSubmit)
	r.Get("/vendor/warehouses/{id}", h.VendorWarehouseDetailPage)
	r.Post("/vendor/warehouses/{id}", h.VendorWarehouseUpdateSubmit)
	r.Post("/vendor/warehouses/{id}/toggle", h.VendorWarehouseToggleSubmit)
	r.Get("/vendor/saving-products", h.VendorSavingProductsPage)
	r.Post("/vendor/saving-products", h.VendorSavingProductCreateSubmit)
	r.Post("/vendor/saving-products/{id}/update", h.VendorSavingProductUpdateSubmit)
	r.Post("/vendor/saving-products/{id}/delete", h.VendorSavingProductDeleteSubmit)
	r.Post("/vendor/saving-products/import", h.VendorSavingProductsImportSubmit)
	r.Get("/vendor/saving-products/export", h.VendorSavingProductsExport)
	r.Get("/vendor/saving-products/providers/{id}", h.VendorSavingProductProvidersJSON)
	r.Get("/vendor/saving-products/search-products", h.VendorSavingProductSearchJSON)
	r.Get("/vendor/saving-products/import", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/vendor/saving-products", http.StatusMovedPermanently)
	})
	r.Get("/vendor/saveing-products", h.VendorSavingProductsAlias)
	r.Get("/vendor/coverage", h.VendorCoveragePage)
	r.Post("/vendor/coverage", h.VendorCoverageCreateSubmit)
	r.Post("/vendor/coverage/{id}", h.VendorCoverageUpdateSubmit)
	r.Post("/vendor/coverage/{id}/delete", h.VendorCoverageDeleteSubmit)
	r.Post("/vendor/coverage/{id}/toggle", h.VendorCoverageToggleSubmit)
	r.Get("/vendor/coverage/branch/{branchID}", h.VendorBranchCoveragePage)
	r.Get("/vendor/pharmacy-coverage", h.VendorPharmacyCoveragePage)
	r.Get("/vendor/pharmacy-coverage/{id}", h.VendorPharmacyCoverageDetailPage)
	r.Get("/vendor/transfers", h.VendorTransfersPage)
	r.Get("/vendor/ingest", h.VendorIngestPage)
	r.Get("/vendor/ingest/{sessionID}", h.VendorIngestSessionPage)
	r.Get("/vendor/ingest/sample.xlsx", h.VendorIngestSampleXLSX)
	r.Get("/vendor/ingest/sample.csv", h.VendorIngestSampleCSV)
	r.Get("/vendor/ingest/export", h.VendorIngestExport)
	r.Post("/vendor/ingest/upload", h.VendorIngestUploadSubmit)
	r.Post("/vendor/ingest/{id}/mapping", h.VendorIngestMappingSubmit)
	r.Get("/vendor/ingest/{id}/rows", h.VendorIngestRowsPartial)
	r.Post("/vendor/ingest/{id}/rows/{rid}", h.VendorIngestRowUpdateSubmit)
	r.Post("/vendor/ingest/{id}/commit", h.VendorIngestCommitSubmit)
	r.Post("/vendor/ingest/{id}/cancel", h.VendorIngestCancelSubmit)
	r.Get("/vendor/orders", h.VendorOrdersPage)
	r.Get("/vendor/orders/offers", h.VendorOfferOrdersPage)
	r.Get("/vendor/orders/offers/{id}", h.VendorOfferOrderDetailPage)
	r.Post("/vendor/orders/{id}/status", h.VendorOrderStatusSubmit)
	r.Get("/vendor/offers", h.VendorOffersPage)
	r.Get("/vendor/offers/new", h.VendorOfferNewPage)
	r.Post("/vendor/offers/new", h.VendorOfferNewSubmit)
	r.Get("/vendor/offers/{id}/locations", h.VendorOfferLocationsPage)
	r.Post("/vendor/offers/{id}/locations/new", h.VendorOfferLocationNewSubmit)
	r.Post("/vendor/offers/{id}/delete", h.VendorOfferDeleteSubmit)
	r.Get("/vendor/offers/locations", h.VendorOffersLocationsPage)
	r.Get("/vendor/offers-packages", h.VendorOffersPackagesPage)
	r.Get("/vendor/offers-packages/{id}", h.VendorOffersPackagesPage)
	r.Get("/vendor/offers-packages/sponsorships", h.VendorOffersPackagesSponsorshipsPage)
	r.Get("/vendor/offers-packages/sponsorships/{id}", h.VendorOffersPackagesSponsorshipsPage)
	r.Get("/vendor/offers-packages/promotions", h.VendorOffersPackagesPromotionsPage)
	r.Get("/vendor/ads", h.VendorAdsPage)
	r.Get("/vendor/ads/add", h.VendorAdsPage)
	r.Get("/vendor/ads/{id}", h.VendorAdsPage)
	r.Get("/vendor/ads/{id}/edit", h.VendorAdsPage)
	r.Get("/vendor/payments", h.VendorPaymentsPage)
	r.Get("/vendor/earnings/order", h.VendorEarningsOrderPage)
	r.Get("/vendor/earnings/offers", h.VendorEarningsOffersPage)
	r.Get("/vendor/policies", h.VendorPoliciesPage)
	r.Post("/vendor/policies", h.VendorPoliciesSubmit)
	r.Get("/vendor/social-media", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/vendor/policies", http.StatusMovedPermanently)
	})
	r.Get("/vendor/storefront", h.VendorStorefrontPage)
	r.Post("/vendor/storefront/section", h.VendorStorefrontSectionSubmit)
	r.Post("/vendor/storefront/section/{id}/update", h.VendorStorefrontSectionUpdateSubmit)
	r.Post("/vendor/storefront/section/{id}/delete", h.VendorStorefrontSectionDeleteSubmit)
	r.Post("/vendor/storefront/section/{id}/toggle", h.VendorStorefrontSectionToggleSubmit)
	r.Get("/vendor/activities", h.VendorActivitiesPage)
	r.Get("/vendor/institutional-work", h.VendorInstitutionalWorkPage)
	r.Get("/vendor/jobs", h.VendorJobsPage)
	r.Post("/vendor/jobs", h.VendorJobCreateSubmit)
	r.Post("/vendor/jobs/{id}/toggle", h.VendorJobToggleSubmit)
	r.Post("/vendor/jobs/{id}/delete", h.VendorJobDeleteSubmit)
	r.Get("/vendor/jobs/{id}/applications", h.VendorJobApplicationsJSON)

	r.Get("/vendor/documents", h.OrganizationDocumentsPage)
	r.Post("/documents/upload", h.OrganizationDocumentsUploadSubmit)
	r.Post("/documents/delete", h.OrganizationDocumentDeleteSubmit)
	r.Post("/requests/{id}/respond", h.RequestRespondSubmit)

	// Vendor Purchase Requests (Plan V5 Phase 3 §3.1.5)
	r.Get("/vendor/purchase-requests", h.VendorPurchaseRequestsPage)
	r.Get("/vendor/purchase-requests/{id}", h.VendorPurchaseRequestDetailPage)
	r.Post("/vendor/purchase-requests/{id}/respond", h.VendorPurchaseRequestRespondSubmit)
	r.Post("/vendor/purchase-requests/lines/{id}/respond", h.VendorPurchaseRequestLineRespondSubmit)
}

// RegisterAdminRoutes mounts the platform staff surface, gated by RequireStaff
// and per-page granular permission gates (RequirePagePermission).
func (h *UIHandler) RegisterAdminRoutes(r chi.Router) {
	// Dashboard is reachable by any authenticated platform staff member.
	r.Get("/admin/dashboard", h.AdminDashboardPage)

	// Modular Area Routes with granular permission gates
	h.registerAdminCatalogRoutes(r)
	h.registerAdminOrgRoutes(r)
	h.registerAdminIdentityRoutes(r)
	h.registerAdminCommerceRoutes(r)
	h.registerAdminPlatformRoutes(r)
}

// RegisterSharedRoutes mounts the account surface that both customers and
// vendors use — settings, documents, wallet, invoices, messages,
// notifications, requests. The pages render inside the caller's own shell,
// chosen from actor.OrgType (Rebuild V2 §1.5), so no audience gate is needed
// here beyond authentication.
func (h *UIHandler) RegisterSharedRoutes(r chi.Router) {
	r.Get("/onboarding/pending", h.OnboardingPendingPage)
	r.Get("/org/switch/{id}", h.OrgSwitchSubmit)

	// Documents (Rebuild V2 §4.2) - accessible by both pending and approved orgs
	r.Get("/customer/documents", h.OrganizationDocumentsPage)
	r.Get("/vendor/documents", h.OrganizationDocumentsPage)
	r.Get("/documents", h.OrganizationDocumentsPage)
	r.Post("/documents/upload", h.OrganizationDocumentsUploadSubmit)
	r.Post("/documents/delete", h.OrganizationDocumentDeleteSubmit)

	// Settings (account surface, both shells)

	// One settings surface: the tabbed page. Six separate sub-pages used to
	// render the same data through a second tab component, so the two could
	// disagree about what the account looked like. They are 301s now — the
	// paths stay reachable because they were linked from sidebars and may be
	// bookmarked (PLAN_V7 Task 2.1).
	r.Get("/settings", h.SettingsIndex)
	r.Get("/settings/profile", redirectToSettingsTab("profile"))
	r.Get("/settings/addresses", redirectToSettingsTab("profile"))
	r.Get("/settings/security", redirectToSettingsTab("security"))
	r.Get("/settings/organization", redirectToSettingsTab("organization"))
	r.Get("/settings/preferences", redirectToSettingsTab("preferences"))
	r.Get("/settings/payment-methods", redirectToSettingsTab("payments"))
	// Employees is a real management screen, not a settings tab: it lists
	// staff, assigns branch managers and creates accounts.
	r.Get("/settings/employees", h.SettingsEmployeesPage)

	r.Post("/settings/profile", h.SettingsProfileSubmit)
	r.Post("/settings/password", h.SettingsPasswordSubmit)
	r.Post("/settings/addresses", h.SettingsAddressSubmit)
	r.Post("/settings/addresses/{id}/delete", h.SettingsAddressDeleteSubmit)
	r.Post("/settings/security/revoke", h.SettingsSessionRevokeSubmit)
	r.Post("/settings/sessions/revoke", h.SettingsSessionRevokeSubmit)
	r.Post("/settings/security/plan/{id}", h.SettingsSessionPlanPurchaseSubmit)
	r.Post("/settings/delete-request", h.SettingsDeleteRequestSubmit)

	r.Post("/settings/organization", h.SettingsOrgUpdateSubmit)
	// Branch management lives at /customer/branches and /vendor/branches. The
	// settings page used to carry a third, lower-quality write path that even
	// invented branch codes when the form omitted one (PLAN_V7 Task 2.2).
	r.Post("/settings/organization/member/{userID}/role", h.SettingsMemberRoleSubmit)
	r.Post("/settings/organization/member", h.SettingsMemberAddSubmit)
	r.Post("/settings/employees", h.SettingsEmployeeCreateSubmit)
	r.Post("/settings/employees/create", h.SettingsEmployeeCreateSubmit)
	r.Post("/settings/employees/add", h.SettingsEmployeeCreateSubmit)
	r.Post("/settings/employees/{id}/delete", h.SettingsEmployeeDeleteSubmit)
	r.Post("/settings/employees/assign-manager", h.SettingsBranchManagerAssignSubmit)
	r.Post("/settings/branches/{id}/manager", h.SettingsBranchManagerAssignSubmit)
	r.Post("/settings/preferences", h.SettingsPreferencesSubmit)
	r.Post("/settings/payment-methods", h.SettingsPaymentMethodsSubmit)
	r.Post("/settings/payment-methods/{id}/delete", h.SettingsPaymentMethodDeleteSubmit)

	// Wallet, invoices, messages, requests
	r.Get("/wallet", h.WalletPage)
	r.Get("/invoices", h.InvoicesPage)
	r.Get("/messages", h.MessagesPage)
	r.Get("/messages/{id}", h.MessagesConversationPage)
	r.Get("/requests", h.RequestsPage)
	r.Get("/report-issue", h.CustomerReportIssuePage)
	r.Post("/report-issue", h.CustomerReportIssueSubmit)

	r.Post("/wallet/deposit", h.WalletDepositSubmit)
	r.Post("/wallet/withdraw", h.WalletWithdrawSubmit)
	r.Post("/messages/{id}/send", h.MessagesSendSubmit)
	r.Post("/requests", h.RequestCreateSubmit)

	// Notifications (bell and page)
	r.Get("/notifications", h.NotificationsPage)
	r.Get("/notifications/dropdown", h.NotificationsDropdownPartial)
	r.Get("/notifications/unread-badge", h.NotificationsUnreadBadgePartial)
	r.Post("/notifications/{id}/read", h.MarkNotificationReadSubmit)
	r.Post("/notifications/read-all", h.NotificationsReadAllSubmit)
}

func (h *UIHandler) renderError(w http.ResponseWriter, r *http.Request, err error) {
	ctx := r.Context()
	h.log.ErrorContext(ctx, "ui error rendering page", "error", err, "path", r.URL.Path)

	lang, dir := h.localeAndDir(r)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	// err.Error() renders the internal form - "conflict [order.already_confirmed]:
	// ... (detail)" for an apperr, and for anything else the raw driver text,
	// which names tables, columns and constraints. apperr.Msg is documented as
	// user-safe and LocalizedMsg gives the Arabic wording by code; anything that
	// is not an apperr gets a generic message and lives only in the log above.
	msg := h.safeMessage(err, lang)

	if h.isHTMX(r) {
		// 200 on purpose: HTMX swaps the error state into the target region. A
		// non-2xx would leave the old content in place with nothing explaining why.
		w.WriteHeader(http.StatusOK)
		if rerr := components.ErrorState(components.ErrorStateProps{
			Title:      "حدث خطأ أثناء تحميل البيانات",
			Message:    msg,
			RetryURL:   r.URL.String(),
			RetryLabel: "إعادة المحاولة",
		}).Render(ctx, w); rerr != nil {
			h.log.ErrorContext(ctx, "render error state", "error", rerr)
		}
		return
	}

	w.WriteHeader(statusForError(err))
	if rerr := pages.ErrorPage(
		"عذراً، حدث خطأ",
		msg,
		"/",
		lang,
		dir,
	).Render(ctx, w); rerr != nil {
		h.log.ErrorContext(ctx, "render error page", "error", rerr)
	}
}

// safeMessage returns wording that may be shown to a user.
func (h *UIHandler) safeMessage(err error, lang string) string {
	if err == nil {
		return ""
	}
	if appErr, ok := apperr.As(err); ok {
		return appErr.LocalizedMsg(lang)
	}
	errStr := err.Error()
	if strings.Contains(errStr, "email") && (strings.Contains(errStr, "unique") || strings.Contains(errStr, "duplicate key") || strings.Contains(errStr, "23505") || strings.Contains(errStr, "users_email_key")) {
		return "البريد الإلكتروني مسجل مسبقاً في النظام. يرجى تسجيل الدخول أو استخدام بريد آخر."
	}
	if strings.Contains(errStr, "commercial_register") && (strings.Contains(errStr, "unique") || strings.Contains(errStr, "duplicate key") || strings.Contains(errStr, "23505")) {
		return "رقم السجل التجاري مسجل مسبقاً لمنشأة أخرى."
	}
	if strings.Contains(errStr, "city_id") || strings.Contains(errStr, "branches_city_id_fkey") {
		return "بيانات الموقع أو المدينة غير صالحة. يرجى إعادة اختيار المدينة من الخريطة."
	}
	if strings.Contains(errStr, "order_shipments_organization_id_fkey") || strings.Contains(errStr, "order_lines_organization_id_fkey") {
		return "تعذر تحديد بيانات شركة التوريد المسؤولة عن هذا الصنف (رمز المورد غير مسجل). يرجى مراجعة الأصناف بالسلة."
	}
	if strings.Contains(errStr, "orders_branch_id_fkey") || strings.Contains(errStr, "order_shipments_branch_id_fkey") {
		return "فرع الصيدلية المحدد غير صالح أو تم حذفه. يرجى اختيار فرع صيدلية نشط."
	}
	if strings.Contains(errStr, "orders_vendor_branch_id_fkey") {
		return "فرع التوريد المحدد للمورد غير صالح أو غير مسجل."
	}
	if strings.Contains(errStr, "foreign key") || strings.Contains(errStr, "23503") {
		return "تعذر إتمام العملية بسبب عدم تطابق البيانات المرجعية (" + errStr + ")."
	}
	if lang == "ar" {
		return "حدث خطأ أثناء المعالجة: " + errStr
	}
	return "Operation could not be completed: " + errStr
}

// statusForError maps an error onto a response code. A full page load that
// returns 200 for a failure is invisible to uptime checks and to the browser.
func statusForError(err error) int {
	switch apperr.KindOf(err) {
	case apperr.KindNotFound:
		return http.StatusNotFound
	case apperr.KindValidation:
		return http.StatusBadRequest
	case apperr.KindUnauthorized:
		return http.StatusUnauthorized
	case apperr.KindForbidden:
		return http.StatusForbidden
	case apperr.KindConflict:
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}

// redirectWithNotice sends the user on with a message to show when they land.
//
// Form posts here redirect after handling, which is correct — it stops a
// refresh from resubmitting. But it also throws away everything the handler
// learned, which is why a failed save was indistinguishable from a successful
func (h *UIHandler) redirectWithNotice(w http.ResponseWriter, r *http.Request, path, kind, message string) {
	u, err := url.Parse(path)
	if err != nil {
		http.Redirect(w, r, path, http.StatusSeeOther)
		return
	}
	q := u.Query()
	q.Set("notice", kind)
	q.Set("msg", message)
	u.RawQuery = q.Encode()
	http.Redirect(w, r, u.String(), http.StatusSeeOther)
}

// SetLanguage persists the chosen UI language in the dawa24_lang cookie and
// returns the user to where they were. Signed-in users get the same choice
// written to their profile preference via UpdateProfile when they save settings.
func (h *UIHandler) SetLanguage(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	if code != "en" {
		code = "ar"
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "dawa24_lang",
		Value:    code,
		Path:     "/",
		MaxAge:   86400 * 365,
		SameSite: http.SameSiteLaxMode,
	})
	back := r.Header.Get("Referer")
	if back == "" {
		back = "/"
	}
	http.Redirect(w, r, back, http.StatusSeeOther)
}

// langOf is the language alone, for callers that do not need the direction.
func langOf(r *http.Request) string {
	if r.URL.Query().Get("lang") == "en" {
		return "en"
	}
	return "ar"
}

func (h *UIHandler) pageLimit(r *http.Request) int {
	lim, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if lim <= 0 || lim > 100 {
		return 20
	}
	return lim
}

func (h *UIHandler) pageOffset(r *http.Request) int {
	off, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if off < 0 {
		return 0
	}
	return off
}

func (h *UIHandler) isHTMX(r *http.Request) bool {
	return r.Header.Get("HX-Request") == "true"
}

// localeAndDir resolves the request language and text direction.
//
// Precedence: query ?lang= → dawa24_lang cookie → Accept-Language → Arabic.
// (User preference from profile.user_preferences is layered in later once the
// settings surface exists; the cookie already persists the choice for signed-out
// visitors.) Arabic is the default and the primary language of the marketplace.
func (h *UIHandler) localeAndDir(r *http.Request) (string, string) {
	if lang := r.URL.Query().Get("lang"); lang != "" {
		return dirForLang(lang)
	}
	if cookie, err := r.Cookie("dawa24_lang"); err == nil && cookie.Value != "" {
		return dirForLang(cookie.Value)
	}
	if header := r.Header.Get("Accept-Language"); header != "" {
		if lang := acceptLanguage(header); lang != "" {
			return dirForLang(lang)
		}
	}
	return "ar", "rtl"
}

// dirForLang returns the language and the matching text direction. Unknown
// values fall back to Arabic rather than erroring — language is a display
// preference, never a request failure.
func dirForLang(lang string) (string, string) {
	if lang == "en" {
		return "en", "ltr"
	}
	return "ar", "rtl"
}

// acceptLanguage maps an Accept-Language header onto a supported language by
// taking the first weighted entry, ignoring the rest. It is a best effort: a
// browser sending "fr-CH, fr;q=0.9" simply gets Arabic.
func acceptLanguage(header string) string {
	for _, part := range strings.Split(header, ",") {
		lang := strings.TrimSpace(strings.SplitN(part, ";", 2)[0])
		if lang == "en" || lang == "ar" {
			return lang
		}
	}
	return ""
}

// redirectToSettingsTab permanently redirects a retired settings sub-page to
// its tab on the unified page.
func redirectToSettingsTab(tab string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if tab == "organization" {
			if actor, ok := authctx.From(r.Context()); ok && actor.OrgType == "vendor" {
				http.Redirect(w, r, "/vendor/organization", http.StatusSeeOther)
				return
			}
		}
		http.Redirect(w, r, "/settings?tab="+tab, http.StatusMovedPermanently)
	}
}
