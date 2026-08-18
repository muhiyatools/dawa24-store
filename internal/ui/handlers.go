package ui

import (
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/attachments"
	"github.com/muhiya/dawa24-store/internal/modules/billing"
	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/modules/chat"
	"github.com/muhiya/dawa24-store/internal/modules/commerce"
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
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/ui/components"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)


// UIHandler serves server-rendered HTML pages via Templ.
type UIHandler struct {
	catSvc   *catalog.Service
	orgSvc   *org.Service
	ingSvc   *ingest.Service
	commSvc  *commerce.Service
	invSvc   *inventory.Service
	idSvc    *identity.Service
	notifSvc *notifications.Service
	promoSvc *promo.Service
	adminSvc *platformadmin.Service
	billSvc  *billing.Service
	chatSvc  *chat.Service
	wfSvc    *workflow.Service
	hrSvc    *hr.Service
	attSvc   *attachments.Service
	log      *slog.Logger
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
		pub.Get("/compare/tool", h.CompareToolPage)

		// Form actions that work signed-out (sign-up must be reachable pre-login)
		pub.Post("/auth/login", h.LoginSubmit)
		pub.Post("/auth/logout", h.LogoutSubmit)
		pub.Get("/auth/logout", h.LogoutSubmit)
		pub.Post("/auth/register", h.RegisterSubmit)
		pub.Post("/contact", h.ContactSubmit)
		pub.Post("/upload", h.UploadAPISubmit)
		pub.Post("/offers/{id}/click", h.OfferClickSubmit)
		pub.Post("/services/{id}/request", h.ServiceRequestSubmit)
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

	r.Post("/customer/set-branch", h.SetBuyingBranchSubmit)

	r.Get("/cart", h.CustomerCartPage)
	r.Get("/checkout", h.CustomerCheckoutPage)
	r.Get("/orders", h.CustomerOrdersPage)
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
	r.Post("/customer/branches/new", h.CustomerBranchNewSubmit)
	r.Post("/customer/branches/{id}/delete", h.CustomerBranchDeleteSubmit)

	// Customer interactions
	r.Post("/suppliers/{id}/follow", h.SupplierFollowSubmit)
	r.Post("/suppliers/{id}/message", h.SupplierMessageSubmit)
	r.Post("/suppliers/{id}/quote", h.SupplierQuoteSubmit)
	r.Post("/finder/answer", h.FinderAnswerSubmit)
	r.Post("/compare/subscribe", h.CompareSubscribeSubmit)
	r.Post("/reviews/submit", h.ReviewSubmit)
}


// RegisterVendorRoutes mounts the vendor (مورّد) surface, gated by
// RequireVendor.
func (h *UIHandler) RegisterVendorRoutes(r chi.Router) {
	r.Get("/vendor/dashboard", h.VendorDashboardPage)
	r.Get("/vendor/products", h.VendorProductsPage)
	r.Get("/vendor/products/new", h.VendorVariantNewPage)
	r.Get("/vendor/variants/new", h.VendorVariantNewPage)
	r.Post("/vendor/variants/new", h.VendorVariantNewSubmit)
	r.Post("/vendor/variants/{id}/delete", h.VendorVariantDeleteSubmit)
	r.Get("/vendor/branches", h.VendorBranchesPage)
	r.Post("/vendor/branches/new", h.VendorBranchNewSubmit)
	r.Post("/vendor/branches/{id}/delete", h.VendorBranchDeleteSubmit)
	r.Post("/vendor/branches/{id}/manager", h.SettingsBranchManagerAssignSubmit)
	r.Get("/vendor/team", h.VendorTeamPage)
	r.Post("/vendor/team/new", h.VendorTeamNewSubmit)
	r.Post("/vendor/team/{id}/toggle", h.VendorTeamToggleSubmit)
	r.Get("/vendor/inventory", h.VendorInventoryPage)
	r.Post("/vendor/inventory/{id}/adjust", h.VendorStockAdjustSubmit)
	r.Get("/vendor/coverage", h.VendorCoveragePage)
	r.Get("/vendor/transfers", h.VendorTransfersPage)
	r.Get("/vendor/ingest", h.VendorIngestPage)
	r.Post("/vendor/ingest/upload", h.VendorIngestUploadSubmit)
	r.Post("/vendor/ingest/{id}/commit", h.VendorIngestCommitSubmit)
	r.Get("/vendor/orders", h.VendorOrdersPage)

	r.Post("/vendor/orders/{id}/status", h.VendorOrderStatusSubmit)
	r.Get("/vendor/offers", h.VendorOffersPage)
	r.Get("/vendor/offers/new", h.VendorOfferNewPage)
	r.Post("/vendor/offers/new", h.VendorOfferNewSubmit)
	r.Get("/vendor/offers/{id}/locations", h.VendorOfferLocationsPage)
	r.Post("/vendor/offers/{id}/locations/new", h.VendorOfferLocationNewSubmit)
	r.Post("/vendor/offers/{id}/delete", h.VendorOfferDeleteSubmit)
	r.Get("/vendor/storefront", h.VendorStorefrontPage)
	r.Post("/vendor/storefront/section", h.VendorStorefrontSectionSubmit)
	r.Post("/vendor/storefront/section/{id}/item", h.VendorStorefrontItemSubmit)
	r.Get("/vendor/jobs", h.VendorJobsPage)
	r.Post("/vendor/jobs", h.VendorJobCreateSubmit)

	r.Get("/vendor/documents", h.OrganizationDocumentsPage)
	r.Post("/documents/upload", h.OrganizationDocumentsUploadSubmit)
	r.Post("/documents/delete", h.OrganizationDocumentDeleteSubmit)
	r.Post("/requests/{id}/respond", h.RequestRespondSubmit)
}

// RegisterAdminRoutes mounts the platform staff surface, gated by RequireStaff
// (super_admin, admin, support, developer). Every handler behind it runs
// platform-wide work (database.AsSystem) — this gate is what keeps a pharmacy
// account away from /admin/users and the rest.
func (h *UIHandler) RegisterAdminRoutes(r chi.Router) {
	r.Get("/admin/dashboard", h.AdminDashboardPage)
	r.Get("/admin/approvals", h.AdminApprovalsPage)
	r.Get("/admin/documents", h.AdminDocumentsPage)
	r.Get("/admin/users", h.AdminUsersPage)
	r.Get("/admin/settings", h.AdminSettingsPage)
	r.Get("/admin/messages", h.AdminMessagesPage)
	r.Get("/admin/content", h.AdminContentPage)
	r.Get("/admin/analytics", h.AdminAnalyticsPage)
	r.Get("/admin/translations", h.AdminTranslationsPage)
	r.Get("/admin/audit", h.AdminAuditPage)
	r.Get("/admin/organizations", h.AdminOrganizationsPage)
	r.Get("/admin/vendors", h.AdminOrganizationsPage)
	r.Get("/admin/suppliers", h.AdminOrganizationsPage)
	r.Get("/admin/orders", h.AdminOrdersPage)
	r.Get("/admin/products", h.AdminProductsPage)
	r.Get("/admin/offers", h.AdminOffersPage)
	r.Get("/admin/jobs", h.AdminJobsPage)
	r.Get("/admin/policies", h.AdminPoliciesPage)
	r.Get("/admin/finder", h.AdminFinderPage)
	r.Get("/admin/services", h.AdminServicesPage)
	r.Get("/admin/plans", h.AdminPlansPage)
	r.Get("/admin/cities", h.AdminCitiesPage)

	r.Post("/admin/settings", h.AdminSettingsSubmit)
	r.Post("/admin/settings/features/toggle", h.AdminFeatureToggleSubmit)
	r.Post("/admin/content", h.AdminContentSubmit)
	r.Post("/admin/translations", h.AdminTranslationsSubmit)
	r.Post("/admin/organizations/{id}/approve", h.AdminOrgApproveSubmit)
	r.Post("/admin/organizations/{id}/reject", h.AdminOrgRejectSubmit)
	r.Post("/admin/organizations/{id}/suspend", h.AdminOrgSuspendSubmit)
	r.Post("/admin/products/{id}/status", h.AdminProductStatusSubmit)
	r.Post("/admin/offers/{id}/status", h.AdminOfferStatusSubmit)
	r.Post("/admin/cities/new", h.AdminCityCreateSubmit)
	r.Post("/admin/cities/{id}/toggle", h.AdminCityToggleSubmit)

	r.Post("/admin/finder/question", h.AdminFinderQuestionSubmit)
	r.Post("/admin/finder/result", h.AdminFinderResultSubmit)
	r.Post("/admin/finder/option", h.AdminFinderOptionSubmit)
	r.Post("/admin/services", h.AdminServiceSubmit)
	r.Post("/admin/plans", h.AdminPlanSubmit)
	r.Post("/admin/users/{id}/suspend", h.AdminUserSuspendSubmit)
	r.Post("/admin/users/{id}/reactivate", h.AdminUserReactivateSubmit)
	r.Post("/admin/users/{id}/reset-mfa", h.AdminUserResetMFASubmit)
	r.Post("/admin/approvals/{id}/approve", h.AdminApproveOrgSubmit)
	r.Post("/admin/approvals/{id}/reject", h.AdminRejectOrgSubmit)
	r.Post("/admin/approvals/{id}/review", h.AdminOrgReviewSubmit)
	r.Post("/admin/products/new", h.AdminProductCreateSubmit)
	r.Post("/admin/policies", h.AdminPolicyCreateSubmit)
	r.Post("/admin/policies/{id}/publish", h.AdminPolicyPublishSubmit)
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

	r.Get("/settings", h.SettingsIndex)
	r.Get("/settings/profile", h.SettingsProfilePage)
	r.Get("/settings/addresses", h.SettingsAddressesPage)
	r.Get("/settings/security", h.SettingsSecurityPage)
	r.Get("/settings/organization", h.SettingsOrganizationPage)
	r.Get("/settings/employees", h.SettingsEmployeesPage)
	r.Get("/settings/preferences", h.SettingsPreferencesPage)
	r.Get("/settings/payment-methods", h.SettingsPaymentMethodsPage)

	r.Post("/settings/profile", h.SettingsProfileSubmit)
	r.Post("/settings/password", h.SettingsPasswordSubmit)
	r.Post("/settings/addresses", h.SettingsAddressSubmit)
	r.Post("/settings/addresses/{id}/delete", h.SettingsAddressDeleteSubmit)
	r.Post("/settings/security/revoke", h.SettingsSessionRevokeSubmit)
	r.Post("/settings/sessions/revoke", h.SettingsSessionRevokeSubmit)
	r.Post("/settings/security/plan/{id}", h.SettingsSessionPlanPurchaseSubmit)

	r.Post("/settings/organization", h.SettingsOrgUpdateSubmit)
	r.Post("/settings/organization/branch", h.SettingsBranchCreateSubmit)
	r.Post("/settings/organization/branch/{id}/delete", h.SettingsBranchDeleteSubmit)
	r.Post("/settings/organization/member/{userID}/role", h.SettingsMemberRoleSubmit)
	r.Post("/settings/organization/member", h.SettingsMemberAddSubmit)
	r.Post("/settings/employees/create", h.SettingsEmployeeCreateSubmit)
	r.Post("/settings/employees/add", h.SettingsEmployeeCreateSubmit)
	r.Post("/settings/employees/{id}/delete", h.SettingsEmployeeDeleteSubmit)
	r.Post("/settings/employees/assign-manager", h.SettingsBranchManagerAssignSubmit)
	r.Post("/settings/branches/{id}/manager", h.SettingsBranchManagerAssignSubmit)
	r.Post("/settings/preferences", h.SettingsPreferencesSubmit)
	r.Post("/settings/payment-methods", h.SettingsPaymentMethodsSubmit)

	// Wallet, invoices, messages, requests
	r.Get("/wallet", h.WalletPage)
	r.Get("/invoices", h.InvoicesPage)
	r.Get("/messages", h.MessagesPage)
	r.Get("/messages/{id}", h.MessagesConversationPage)
	r.Get("/requests", h.RequestsPage)

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
	if appErr, ok := apperr.As(err); ok {
		return appErr.LocalizedMsg(lang)
	}
	if lang == "ar" {
		return "حدث خطأ غير متوقع. يرجى المحاولة مرة أخرى."
	}
	return "An unexpected error occurred. Please try again."
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
// one. The outcome travels in the query string and the layout renders it as a
// toast on arrival.
func (h *UIHandler) redirectWithNotice(w http.ResponseWriter, r *http.Request, path, kind, message string) {
	q := url.Values{}
	q.Set("notice", kind)
	q.Set("msg", message)
	http.Redirect(w, r, path+"?"+q.Encode(), http.StatusSeeOther)
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
