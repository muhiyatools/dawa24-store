package ui

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/ui/components"

	"github.com/muhiya/dawa24-store/internal/modules/assistant"
	"github.com/muhiya/dawa24-store/internal/modules/attachments"
	"github.com/muhiya/dawa24-store/internal/modules/billing"
	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/modules/chat"
	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/modules/compare"
	"github.com/muhiya/dawa24-store/internal/modules/hr"
	"github.com/muhiya/dawa24-store/internal/modules/identity"
	"github.com/muhiya/dawa24-store/internal/modules/ingest"
	"github.com/muhiya/dawa24-store/internal/modules/inventory"
	"github.com/muhiya/dawa24-store/internal/modules/notifications"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	platformadmin "github.com/muhiya/dawa24-store/internal/modules/platform_admin"
	"github.com/muhiya/dawa24-store/internal/modules/promo"
	"github.com/muhiya/dawa24-store/internal/modules/smartorder"
	"github.com/muhiya/dawa24-store/internal/modules/workflow"
	"github.com/muhiya/dawa24-store/internal/platform/aiusage"
	"github.com/muhiya/dawa24-store/internal/platform/antiscrape"
	"github.com/muhiya/dawa24-store/internal/platform/gateway"
	"github.com/muhiya/dawa24-store/internal/platform/pagecontrol"
	"github.com/muhiya/dawa24-store/internal/platform/rbac"
	"github.com/muhiya/dawa24-store/internal/platform/storage"
	"github.com/muhiya/dawa24-store/internal/shared/matchflow"
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

	// pageControl is the store behind the /admin/system-pages screen. Nil when
	// the feature is not wired; the screen then reports itself unavailable.
	pageControl *pagecontrol.Store

	// scrape meters the signed-out pages that publish marketplace data. Nil is
	// a passthrough, which is what a test harness gets: the guard is a property
	// of the deployment, not of the routing table, and a nil one must not
	// change which routes exist.
	scrape *antiscrape.Guard

	// guestMaxPage and guestMaxPageSize bound how deep into a public listing a
	// caller who has not signed in may page. Zero means unbounded, which is
	// what a test harness gets.
	guestMaxPage     int
	guestMaxPageSize int

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
	// matchMemory is the shared decision cache. Nil is allowed and simply means
	// every question is paid for again.
	matchMemory matchflow.Memory
}

// RoleSeederFunc provisions the starter roles for one company.
type RoleSeederFunc func(ctx context.Context, orgID int64, orgType string) error

// SmartOrderEnqueueFunc hands a prepared run to the background worker.
//
// A function rather than the queue client itself: the UI has no business knowing
// what a River job is, and a test can hand it a closure.
type SmartOrderEnqueueFunc func(ctx context.Context, runID, orgID int64) error

// GatewayKeyCache is the admin panel's provisioned Gateway credential, which
// has to be dropped when an operator changes the credentials it was issued
// from. It is an interface so the UI does not depend on the composition root.
type GatewayKeyCache interface {
	Invalidate()
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
	r.Get("/admin/gallery", h.AdminComponentGalleryPage)

	// Modular Area Routes with granular permission gates
	h.registerAdminCatalogRoutes(r)
	h.registerAdminOrgRoutes(r)
	h.registerAdminIdentityRoutes(r)
	h.registerAdminCommerceRoutes(r)
	h.registerAdminPlatformRoutes(r)
	h.registerAdminPageControlRoutes(r)
}

// RegisterPreApprovalRoutes mounts Tier A shared routes accessible by authenticated
// callers prior to organization approval: onboarding status, document submission,
// basic profile/password security, issue reporting, and logout.
func (h *UIHandler) RegisterPreApprovalRoutes(r chi.Router) {
	r.Get("/onboarding/pending", h.OnboardingPendingPage)

	// Documents (Rebuild V2 §4.2) - accessible by both pending and approved orgs
	r.Get("/documents", h.OrganizationDocumentsPage)
	r.Get("/documents/{id}/view", h.DocumentViewHandler)
	r.Get("/documents/{id}/download", h.DocumentDownloadHandler)
	r.Post("/documents/upload", h.OrganizationDocumentsUploadSubmit)
	r.Post("/documents/delete", h.OrganizationDocumentDeleteSubmit)

	// Issue reporting
	r.Get("/report-issue", h.CustomerReportIssuePage)
	r.Post("/report-issue", h.CustomerReportIssueSubmit)

	// Notifications Center & Bell Partials (Accessible by both pending and approved orgs)
	r.Get("/notifications", h.NotificationsPage)
	r.Get("/notifications/dropdown", h.NotificationsDropdownPartial)
	r.Get("/notifications/unread-badge", h.NotificationsUnreadBadgePartial)
	r.Post("/notifications/{id}/read", h.MarkNotificationReadSubmit)
	r.Post("/notifications/read-all", h.NotificationsReadAllSubmit)

	// Account settings, for the caller's own account.
	//
	// These were in the approved-only tier, which meant a member of a company
	// still under review could not change their own password or revoke a
	// session on a lost device — the two things someone in that position is
	// most likely to need. Nothing here reads or writes company data, so
	// nothing here needs the company to be approved.
	//
	// The payment-method routes stay in the approved tier: they are money, and
	// they belong to a company that may not trade yet.
	r.Get("/settings", h.SettingsIndex)
	r.Get("/settings/profile", redirectToSettingsTab("profile"))
	r.Post("/settings/profile", h.SettingsProfileSubmit)
	r.Post("/settings/password", h.SettingsPasswordSubmit)
	r.Get("/settings/addresses", redirectToSettingsTab("profile"))
	r.Get("/settings/security", redirectToSettingsTab("security"))
	r.Get("/settings/preferences", redirectToSettingsTab("preferences"))
	r.Post("/settings/addresses", h.SettingsAddressSubmit)
	r.Post("/settings/addresses/{id}/delete", h.SettingsAddressDeleteSubmit)
	r.Post("/settings/security/revoke", h.SettingsSessionRevokeSubmit)
	r.Post("/settings/sessions/revoke", h.SettingsSessionRevokeSubmit)
	r.Post("/settings/delete-request", h.SettingsDeleteRequestSubmit)
	r.Post("/settings/preferences", h.SettingsPreferencesSubmit)
}

// RegisterApprovedSharedRoutes mounts Tier B shared routes restricted to approved
// organizations (authctx.RequireApproved mounted): wallet, invoices, messaging,
// notifications, employees, org member management, payment methods, sessions, settings.
func (h *UIHandler) RegisterApprovedSharedRoutes(r chi.Router) {
	r.Get("/components/capsule-assistant", h.CapsuleAssistantPanel)
	r.Get("/org/switch/{id}", h.OrgSwitchSubmit)

	// Settings (Approved only)
	r.Get("/settings/payment-methods", redirectToSettingsTab("payments"))
	// /settings/employees was a third employee CRUD over the same org.members
	// rows, with its own create form, its own delete (which took a user id
	// where the others took a member id) and its own branch-manager assignment.
	// Three screens for one thing is how the three drifted apart. The team page
	// is the one that survives; this redirects rather than 404s so anyone
	// holding a bookmark lands somewhere useful.
	r.Get("/settings/employees", h.SettingsEmployeesRedirect)

	r.Post("/settings/security/plan/{id}", h.SettingsSessionPlanPurchaseSubmit)

	// Branch management lives at /customer/branches and /vendor/branches. The
	// settings page used to carry a third, lower-quality write path that even
	// invented branch codes when the form omitted one (PLAN_V7 Task 2.2).
	r.Post("/settings/branches/{id}/manager", h.SettingsBranchManagerAssignSubmit)
	// The payment-method writes live on the wallet screens, behind
	// pharmacy.wallet.manage / vendor.wallet.manage. These paths stayed
	// reachable by any approved member because this tier has no permission gate
	// of its own, so they redirect rather than accept a write.
	r.Post("/settings/payment-methods", h.PaymentMethodsRedirect)
	r.Post("/settings/payment-methods/{id}/edit", h.PaymentMethodsRedirect)
	r.Post("/settings/payment-methods/{id}/default", h.PaymentMethodsRedirect)
	r.Post("/settings/payment-methods/{id}/delete", h.PaymentMethodsRedirect)

	// Wallet, invoices, messages, requests
	r.Get("/wallet", h.WalletPage)
	r.Get("/invoices", h.InvoicesPage)
	r.Get("/invoices/{id}/print", h.InvoicePrintPage)
	r.Get("/orders/{id}/invoice/print", h.OrderInvoicePrintPage)
	r.Get("/messages", h.MessagesPage)
	r.Get("/messages/{id}", h.MessagesConversationPage)
	r.Get("/requests", h.RequestsPage)

	r.Post("/wallet/deposit", h.WalletDepositSubmit)
	r.Post("/wallet/deposit/{id}/edit", h.WalletDepositEditSubmit)
	r.Post("/wallet/withdraw", h.WalletWithdrawSubmit)
	r.Post("/messages/{id}/send", h.MessagesSendSubmit)
	r.Post("/requests", h.RequestCreateSubmit)
}

// RegisterCustomerSharedRoutes mounts Tier C customer audience-specific shared paths.
func (h *UIHandler) RegisterCustomerSharedRoutes(r chi.Router) {
	r.Get("/customer/documents", h.OrganizationDocumentsPage)
	r.Get("/customer/documents/{id}/view", h.DocumentViewHandler)
	r.Get("/customer/documents/{id}/download", h.DocumentDownloadHandler)
	r.Post("/customer/documents/upload", h.OrganizationDocumentsUploadSubmit)
	r.Post("/customer/documents/delete", h.OrganizationDocumentDeleteSubmit)
}

// RegisterVendorSharedRoutes mounts Tier C vendor audience-specific shared paths.
func (h *UIHandler) RegisterVendorSharedRoutes(r chi.Router) {
	r.Get("/vendor/documents", h.OrganizationDocumentsPage)
	r.Get("/vendor/documents/{id}/view", h.DocumentViewHandler)
	r.Get("/vendor/documents/{id}/download", h.DocumentDownloadHandler)
	r.Post("/vendor/documents/upload", h.OrganizationDocumentsUploadSubmit)
	r.Post("/vendor/documents/delete", h.OrganizationDocumentDeleteSubmit)
}

// CapsuleAssistantPanel renders the lazy-loaded Capsule AI assistant drawer over HTMX.
func (h *UIHandler) CapsuleAssistantPanel(w http.ResponseWriter, r *http.Request) {
	_ = components.CapsuleAssistantPanel().Render(r.Context(), w)
}
