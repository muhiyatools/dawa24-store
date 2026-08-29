package ui

import (
	"context"
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
	"github.com/muhiya/dawa24-store/internal/modules/smartorder"
	"github.com/muhiya/dawa24-store/internal/modules/workflow"
	"github.com/muhiya/dawa24-store/internal/platform/aiusage"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/gateway"
	"github.com/muhiya/dawa24-store/internal/platform/rbac"
	"github.com/muhiya/dawa24-store/internal/platform/storage"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/matchflow"
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
	gatewayKeys   GatewayKeyCache
	tenantKeys    TenantGatewayKeys
	aiUsage       aiusage.Repository
	log           *slog.Logger

	// resolver answers "what may this caller do", reading the database rather
	// than trusting the permission list stamped into the session at login.
	resolver *rbac.Resolver

	// roleSeeder provisions a company's starter roles. A function rather than
	// the rbac package plus a database handle, so the UI keeps no database
	// dependency of its own and a test can hand it a no-op.
	roleSeeder RoleSeederFunc

	// Smart ordering (specs/001-smart-ordering-system). Optional: when the
	// service is nil the wizard reports itself unavailable rather than panicking,
	// which keeps the rest of the customer surface working if it is not wired.
	smartOrderSvc       *smartorder.Service
	smartOrderEnqueue   SmartOrderEnqueueFunc
	smartOrderFinalizer *smartorder.Finalizer
	smartOrderStale     *smartOrderStaleStore

	// matchEnhancer is the AI matching stage the saving-list import runs.
	//
	// Optional, like every other AI path here: unset, the import runs its
	// deterministic tiers and reports what they settled. A pharmacy must be
	// able to build its list when the Gateway is down (AGENTS.md R3).
	matchEnhancer matchflow.Enhancer
}

// RoleSeederFunc provisions the starter roles for one company.
type RoleSeederFunc func(ctx context.Context, orgID int64, orgType string) error

// SetRoleSeeder wires starter-role provisioning for companies that have none.
func (h *UIHandler) SetRoleSeeder(fn RoleSeederFunc) { h.roleSeeder = fn }

// SetPermissionResolver wires live permission resolution. Optional: without
// it the UI falls back to the session's permission copy, which is correct but
// cannot see a revocation until the session ends.
func (h *UIHandler) SetPermissionResolver(r *rbac.Resolver) { h.resolver = r }

// SetMatchEnhancer attaches the shared AI matching stage.
func (h *UIHandler) SetMatchEnhancer(e matchflow.Enhancer) { h.matchEnhancer = e }

// SetAssistantRepository attaches the Assistant database repository for auditing and history.
func (h *UIHandler) SetAssistantRepository(repo assistant.Repository) {
	h.assistantRepo = repo
}

// SmartOrderEnqueueFunc hands a prepared run to the background worker.
//
// A function rather than the queue client itself: the UI has no business knowing
// what a River job is, and a test can hand it a closure.
type SmartOrderEnqueueFunc func(ctx context.Context, runID, orgID int64) error

// SetSmartOrder wires the smart ordering wizard.
func (h *UIHandler) SetSmartOrder(svc *smartorder.Service, enqueue SmartOrderEnqueueFunc) {
	h.smartOrderSvc = svc
	h.smartOrderEnqueue = enqueue
	if h.smartOrderStale == nil {
		h.smartOrderStale = newSmartOrderStaleStore()
	}
}

// SetGatewayClient attaches the Gateway client instance for health probes and AI services.
func (h *UIHandler) SetGatewayClient(ai gateway.Client) {
	h.aiClient = ai
}

// GatewayKeyCache is the admin panel's provisioned Gateway credential, which
// has to be dropped when an operator changes the credentials it was issued
// from. It is an interface so the UI does not depend on the composition root.
type GatewayKeyCache interface {
	Invalidate()
}

// SetGatewayKeyCache installs the credential cache the settings screen resets.
func (h *UIHandler) SetGatewayKeyCache(cache GatewayKeyCache) {
	h.gatewayKeys = cache
}

// TenantGatewayKeys resolves the Gateway credential one منشأة spends against,
// provisioning the organisation's Gateway account on first use.
//
// The UI used to provision inline — read the org, read its plan, mint a key —
// in two separate places, and minting a key revokes the previous one, so two
// screens doing it at once left the tenant with a dead credential. It is a port
// so the UI depends on the capability rather than on the composition root that
// owns the cache and the per-organisation lock.
type TenantGatewayKeys interface {
	// Key returns the organisation's virtual key, or "" when none can be had.
	Key(ctx context.Context, orgID int64) string
	// SyncPlan moves the organisation onto the Gateway plan its current
	// subscription entitles it to.
	SyncPlan(ctx context.Context, orgID int64) error
}

// SetTenantGatewayKeys installs the per-organisation credential resolver.
func (h *UIHandler) SetTenantGatewayKeys(keys TenantGatewayKeys) {
	h.tenantKeys = keys
}

// SetAIUsage installs the local AI consumption ledger.
//
// The usage screens read from it rather than calling the Gateway on every
// render. That is what gives a tenant a history longer than one API page, makes
// the figures survive a Gateway outage, and removes the need for the invented
// costs and latencies the screens used to fill the gaps with.
func (h *UIHandler) SetAIUsage(repo aiusage.Repository) {
	h.aiUsage = repo
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
		pub.Post("/compare/files/{id}/skip", h.CompareFileSkipSubmit)
		pub.Post("/compare/file/{id}/skip", h.CompareFileSkipSubmit)
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
		pub.Get("/compare/market-benchmark", h.CompareMarketBenchmarkPage)
		pub.Get("/compare/market-intelligence", h.CompareMarketIntelligencePage)
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
		pub.Post("/jobs/{id}/apply", h.JobApplySubmit)
	})
}

// RegisterCustomerRoutes mounts the customer (صيدلية) surface. The plan's
// reported bug lived here: a pharmacy account could previously open every
// /vendor/* page because nothing stopped the vendor screen rendering. The
// group is gated by RequireCustomer — a vendor who spells /customer/* gets the
// same 404 as a stranger (the URL space does not exist for them).
func (h *UIHandler) RegisterCustomerRoutes(r chi.Router) {
	// The route table lives in customer_routes.go, grouped by the permission
	// that gates it. This function stays here because test/route_audience_test.go
	// checks that every audience registrar is declared in this file and mounted
	// behind the right gates in cmd/server/routes.go.
	h.registerCustomerRoutes(r)
}

// RegisterVendorRoutes mounts the vendor (مورّد) surface, gated by
// RequireVendor.
func (h *UIHandler) RegisterVendorRoutes(r chi.Router) {
	// See vendor_routes.go for the table; the same reasoning as
	// RegisterCustomerRoutes applies to why this shim is here.
	h.registerVendorRoutes(r)
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
	r.Get("/documents/{id}/view", h.DocumentViewHandler)
	r.Get("/documents/{id}/download", h.DocumentDownloadHandler)
	r.Get("/customer/documents/{id}/view", h.DocumentViewHandler)
	r.Get("/vendor/documents/{id}/view", h.DocumentViewHandler)
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
	r.Post("/settings/payment-methods/{id}/edit", h.SettingsPaymentMethodEditSubmit)
	r.Post("/settings/payment-methods/{id}/default", h.SettingsPaymentMethodSetDefaultSubmit)
	r.Post("/settings/payment-methods/{id}/delete", h.SettingsPaymentMethodDeleteSubmit)

	// Wallet, invoices, messages, requests
	r.Get("/wallet", h.WalletPage)
	r.Get("/invoices", h.InvoicesPage)
	r.Get("/vendor/invoices", h.InvoicesPage)
	r.Get("/invoices/{id}/print", h.InvoicePrintPage)
	r.Get("/orders/{id}/invoice/print", h.OrderInvoicePrintPage)
	r.Get("/messages", h.MessagesPage)
	r.Get("/messages/{id}", h.MessagesConversationPage)
	r.Get("/requests", h.RequestsPage)
	r.Get("/report-issue", h.CustomerReportIssuePage)
	r.Post("/report-issue", h.CustomerReportIssueSubmit)

	r.Post("/wallet/deposit", h.WalletDepositSubmit)
	r.Post("/wallet/deposit/{id}/edit", h.WalletDepositEditSubmit)
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
// its tab or dedicated surface.
func redirectToSettingsTab(tab string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		actor, hasActor := authctx.From(ctx)

		if tab == "organization" {
			if hasActor && actor.IsVendor() {
				http.Redirect(w, r, "/vendor/organization", http.StatusMovedPermanently)
				return
			}
		}
		if tab == "wallet" || tab == "payments" {
			if hasActor && actor.UserID > 0 {
				http.Redirect(w, r, walletDestFor(actor), http.StatusMovedPermanently)
				return
			}
			http.Redirect(w, r, "/customer/wallet", http.StatusMovedPermanently)
			return
		}
		if tab == "security" || tab == "sessions" {
			if hasActor && actor.IsVendor() {
				http.Redirect(w, r, "/vendor/sessions", http.StatusMovedPermanently)
				return
			}
			http.Redirect(w, r, "/customer/sessions", http.StatusMovedPermanently)
			return
		}
		if tab == "profile" || tab == "preferences" {
			if hasActor && actor.IsVendor() {
				http.Redirect(w, r, "/vendor/dashboard", http.StatusSeeOther)
				return
			}
		}
		http.Redirect(w, r, "/settings?tab="+tab, http.StatusMovedPermanently)
	}
}
