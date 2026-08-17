package ui

import (
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/billing"
	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/modules/chat"
	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/modules/hr"
	"github.com/muhiya/dawa24-store/internal/modules/identity"
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
		log:      log,
	}
}

// RegisterPageRoutes registers HTML view endpoints across all screens.
func (h *UIHandler) RegisterPageRoutes(r chi.Router) {
	r.Use(h.visitorMiddleware)
	RegisterStaticRoutes(r)

	// Public & Auth (8 screens)
	r.Get("/", h.HomePage)
	r.Get("/privacy", h.PrivacyPage)
	r.Get("/terms", h.TermsPage)
	r.Get("/about", h.AboutPage)
	r.Get("/how-it-works", h.HowItWorksPage)
	r.Get("/faq", h.FaqPage)
	r.Get("/help", h.HelpPage)
	r.Get("/contact", h.ContactPage)
	r.Get("/auth/login", h.LoginPage)
	r.Get("/auth/register", h.RegisterPage)
	r.Get("/auth/forgot", h.ForgotPasswordPage)
	r.Get("/auth/reset", h.ResetPasswordPage)
	r.Get("/onboarding", h.OnboardingPage)
	r.Get("/lang/{code}", h.SetLanguage)
	r.Get("/onboarding/pending", h.OnboardingPendingPage)

	// Customer Buyer Experience (7 screens)
	r.Get("/catalog", h.CustomerCatalogPage)
	r.Get("/catalog/{id}", h.CustomerProductDetailPage)
	r.Get("/cart", h.CustomerCartPage)
	r.Get("/checkout", h.CustomerCheckoutPage)
	r.Get("/orders", h.CustomerOrdersPage)
	r.Get("/orders/{id}", h.CustomerOrderDetailPage)
	r.Get("/notifications", h.NotificationsPage)
	r.Get("/wallet", h.WalletPage)
	r.Get("/invoices", h.InvoicesPage)
	r.Get("/favorites", h.FavoritesPage)

	// Public & Followed suppliers directory
	r.Get("/suppliers", h.SuppliersPage)
	r.Get("/suppliers/followed", h.FollowedSuppliersPage)
	r.Get("/suppliers/{id}", h.SupplierProfilePage)
	r.Get("/offers", h.OffersPage)
	r.Get("/offers/{id}", h.OfferDetailPage)
	r.Get("/messages", h.MessagesPage)
	r.Get("/messages/{id}", h.MessagesConversationPage)
	r.Get("/jobs", h.JobsPage)
	r.Get("/services", h.ServicesPage)
	r.Get("/finder", h.FinderPage)
	r.Get("/compare", h.ComparePlansPage)
	r.Get("/compare/tool", h.CompareToolPage)
	r.Get("/finder/{id}", h.FinderQuestionByIDPage)
	r.Get("/finder/result/{id}", h.FinderResultByIDPage)
	r.Get("/services/{id}", h.ServiceDetailPage)
	r.Post("/services/{id}/request", h.ServiceRequestSubmit)
	r.Get("/jobs/{id}", h.JobDetailPage)
	r.Get("/requests", h.RequestsPage)

	// Settings (account surface)
	r.Get("/settings", h.SettingsIndex)
	r.Get("/settings/profile", h.SettingsProfilePage)
	r.Get("/settings/addresses", h.SettingsAddressesPage)
	r.Get("/settings/security", h.SettingsSecurityPage)
	r.Get("/settings/organization", h.SettingsOrganizationPage)
	r.Get("/settings/preferences", h.SettingsPreferencesPage)
	r.Get("/settings/payment-methods", h.SettingsPaymentMethodsPage)

	// Vendor Supplier Experience
	r.Get("/vendor/dashboard", h.VendorDashboardPage)
	r.Get("/vendor/products", h.VendorProductsPage)
	r.Get("/vendor/products/new", h.VendorVariantNewPage)
	r.Get("/vendor/variants/new", h.VendorVariantNewPage)
	r.Post("/vendor/variants/new", h.VendorVariantNewSubmit)
	r.Post("/vendor/variants/{id}/delete", h.VendorVariantDeleteSubmit)
	r.Get("/vendor/branches", h.VendorBranchesPage)
	r.Post("/vendor/branches/new", h.VendorBranchNewSubmit)
	r.Post("/vendor/branches/{id}/delete", h.VendorBranchDeleteSubmit)
	r.Get("/vendor/team", h.VendorTeamPage)
	r.Post("/vendor/team/new", h.VendorTeamNewSubmit)
	r.Post("/vendor/team/{id}/toggle", h.VendorTeamToggleSubmit)
	r.Get("/vendor/inventory", h.VendorInventoryPage)
	r.Get("/vendor/transfers", h.VendorTransfersPage)
	r.Get("/vendor/ingest", h.VendorIngestPage)
	r.Get("/vendor/orders", h.VendorOrdersPage)
	r.Get("/vendor/offers", h.VendorOffersPage)
	r.Get("/vendor/storefront", h.VendorStorefrontPage)
	r.Get("/vendor/jobs", h.VendorJobsPage)

	// User / Individual Experience
	r.Get("/user/dashboard", h.UserDashboardPage)
	r.Get("/user/applications", h.UserDashboardPage)

	// Pharmacy Buyer Experience
	r.Get("/pharmacy/dashboard", h.PharmacyDashboardPage)

	// Platform Admin Experience (4 screens)
	r.Get("/admin/dashboard", h.AdminDashboardPage)
	r.Get("/admin/approvals", h.AdminApprovalsPage)
	r.Get("/admin/users", h.AdminUsersPage)
	r.Get("/admin/settings", h.AdminSettingsPage)
	r.Get("/admin/messages", h.AdminMessagesPage)
	r.Get("/admin/content", h.AdminContentPage)
	r.Get("/admin/analytics", h.AdminAnalyticsPage)
	r.Get("/admin/translations", h.AdminTranslationsPage)
	r.Get("/admin/audit", h.AdminAuditPage)
	r.Get("/admin/organizations", h.AdminOrganizationsPage)
	r.Get("/admin/orders", h.AdminOrdersPage)
	r.Get("/admin/products", h.AdminProductsPage)
	r.Get("/admin/offers", h.AdminOffersPage)
	r.Get("/admin/jobs", h.AdminJobsPage)
	r.Get("/admin/finder", h.AdminFinderPage)
	r.Get("/admin/services", h.AdminServicesPage)
	r.Get("/admin/plans", h.AdminPlansPage)

	// Interactive Form & Action Handlers
	r.Post("/auth/login", h.LoginSubmit)
	r.Post("/auth/logout", h.LogoutSubmit)
	r.Get("/auth/logout", h.LogoutSubmit)
	r.Post("/auth/register", h.RegisterSubmit)
	r.Post("/contact", h.ContactSubmit)
	r.Post("/cart/add", h.AddToCartSubmit)
	r.Post("/cart/remove", h.RemoveFromCartSubmit)
	r.Post("/cart/update-quantity", h.UpdateCartQuantitySubmit)
	r.Post("/checkout", h.CheckoutSubmit)
	r.Post("/favorites/{id}/remove", h.FavoriteRemoveSubmit)
	r.Post("/favorites/{id}/add", h.FavoriteAddSubmit)
	r.Post("/favorites/{id}/toggle", h.FavoriteToggleSubmit)
	r.Post("/favorites/toggle", h.FavoriteToggleSubmit)
	r.Post("/settings/profile", h.SettingsProfileSubmit)
	r.Post("/settings/addresses", h.SettingsAddressSubmit)
	r.Post("/settings/addresses/{id}/delete", h.SettingsAddressDeleteSubmit)
	r.Post("/settings/security/revoke", h.SettingsSessionRevokeSubmit)
	r.Post("/settings/security/plan/{id}", h.SettingsSessionPlanPurchaseSubmit)
	r.Post("/settings/organization", h.SettingsOrgUpdateSubmit)
	r.Post("/settings/organization/branch", h.SettingsBranchCreateSubmit)
	r.Post("/settings/organization/branch/{id}/delete", h.SettingsBranchDeleteSubmit)
	r.Post("/settings/organization/member/{userID}/role", h.SettingsMemberRoleSubmit)
	r.Post("/settings/organization/member", h.SettingsMemberAddSubmit)
	r.Post("/settings/preferences", h.SettingsPreferencesSubmit)
	r.Post("/settings/payment-methods", h.SettingsPaymentMethodsSubmit)
	r.Post("/wallet/deposit", h.WalletDepositSubmit)
	r.Post("/wallet/withdraw", h.WalletWithdrawSubmit)
	r.Post("/suppliers/{id}/follow", h.SupplierFollowSubmit)
	r.Post("/offers/{id}/click", h.OfferClickSubmit)
	r.Post("/messages/{id}/send", h.MessagesSendSubmit)
	r.Post("/jobs/{id}/apply", h.JobApplySubmit)
	r.Post("/finder/answer", h.FinderAnswerSubmit)
	r.Post("/compare/subscribe", h.CompareSubscribeSubmit)
	r.Post("/requests", h.RequestCreateSubmit)
	r.Post("/requests/{id}/respond", h.RequestRespondSubmit)
	r.Post("/suppliers/{id}/message", h.SupplierMessageSubmit)
	r.Post("/suppliers/{id}/quote", h.SupplierQuoteSubmit)
	r.Post("/notifications/{id}/read", h.MarkNotificationReadSubmit)
	r.Get("/notifications/dropdown", h.NotificationsDropdownPartial)
	r.Get("/notifications/unread-badge", h.NotificationsUnreadBadgePartial)
	r.Post("/notifications/read-all", h.NotificationsReadAllSubmit)
	r.Post("/vendor/orders/{id}/status", h.VendorOrderStatusSubmit)
	r.Post("/vendor/inventory/{id}/adjust", h.VendorStockAdjustSubmit)
	r.Post("/vendor/storefront/section", h.VendorStorefrontSectionSubmit)
	r.Post("/vendor/storefront/section/{id}/item", h.VendorStorefrontItemSubmit)
	r.Post("/vendor/jobs", h.VendorJobCreateSubmit)
	r.Post("/admin/settings", h.AdminSettingsSubmit)
	r.Post("/admin/content", h.AdminContentSubmit)
	r.Post("/admin/translations", h.AdminTranslationsSubmit)
	r.Post("/admin/organizations/{id}/approve", h.AdminOrgApproveSubmit)
	r.Post("/admin/organizations/{id}/reject", h.AdminOrgRejectSubmit)
	r.Post("/admin/organizations/{id}/suspend", h.AdminOrgSuspendSubmit)
	r.Post("/admin/products/{id}/status", h.AdminProductStatusSubmit)
	r.Post("/admin/offers/{id}/status", h.AdminOfferStatusSubmit)
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
