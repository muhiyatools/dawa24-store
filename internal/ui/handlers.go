package ui

import (
	"log/slog"
	"net/http"
	"net/url"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/modules/identity"
	"github.com/muhiya/dawa24-store/internal/modules/ingest"
	"github.com/muhiya/dawa24-store/internal/modules/inventory"
	"github.com/muhiya/dawa24-store/internal/modules/notifications"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	platformadmin "github.com/muhiya/dawa24-store/internal/modules/platform_admin"
	"github.com/muhiya/dawa24-store/internal/modules/promo"
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
		log:      log,
	}
}

// RegisterPageRoutes registers HTML view endpoints across all screens.
func (h *UIHandler) RegisterPageRoutes(r chi.Router) {
	RegisterStaticRoutes(r)

	// Public & Auth (8 screens)
	r.Get("/", h.HomePage)
	r.Get("/privacy", h.PrivacyPage)
	r.Get("/terms", h.TermsPage)
	r.Get("/auth/login", h.LoginPage)
	r.Get("/auth/register", h.RegisterPage)
	r.Get("/auth/forgot", h.ForgotPasswordPage)
	r.Get("/auth/reset", h.ResetPasswordPage)
	r.Get("/onboarding", h.OnboardingPage)

	// Customer Buyer Experience (7 screens)
	r.Get("/catalog", h.CustomerCatalogPage)
	r.Get("/catalog/{id}", h.CustomerProductDetailPage)
	r.Get("/cart", h.CustomerCartPage)
	r.Get("/checkout", h.CustomerCheckoutPage)
	r.Get("/orders", h.CustomerOrdersPage)
	r.Get("/orders/{id}", h.CustomerOrderDetailPage)
	r.Get("/notifications", h.NotificationsPage)

	// Vendor Supplier Experience (8 screens)
	r.Get("/vendor/products", h.VendorProductsPage)
	r.Get("/vendor/products/new", h.VendorProductNewPage)
	r.Get("/vendor/products/{id}", h.VendorProductEditorPage)
	r.Get("/vendor/inventory", h.VendorInventoryPage)
	r.Get("/vendor/transfers", h.VendorTransfersPage)
	r.Get("/vendor/ingest", h.VendorIngestPage)
	r.Get("/vendor/orders", h.VendorOrdersPage)
	r.Get("/vendor/offers", h.VendorOffersPage)

	// Platform Admin Experience (4 screens)
	r.Get("/admin/dashboard", h.AdminDashboardPage)
	r.Get("/admin/approvals", h.AdminApprovalsPage)
	r.Get("/admin/users", h.AdminUsersPage)
	r.Get("/admin/settings", h.AdminSettingsPage)

	// Interactive Form & Action Handlers
	r.Post("/auth/login", h.LoginSubmit)
	r.Post("/auth/logout", h.LogoutSubmit)
	r.Get("/auth/logout", h.LogoutSubmit)
	r.Post("/auth/register", h.RegisterSubmit)
	r.Post("/cart/add", h.AddToCartSubmit)
	r.Post("/cart/remove", h.RemoveFromCartSubmit)
	r.Post("/checkout", h.CheckoutSubmit)
	r.Post("/notifications/{id}/read", h.MarkNotificationReadSubmit)
	r.Post("/vendor/products", h.VendorProductSaveSubmit)
	r.Delete("/vendor/products/{id}", h.VendorProductDeleteSubmit)
	r.Post("/vendor/orders/{id}/status", h.VendorOrderStatusSubmit)
	r.Post("/admin/settings", h.AdminSettingsSubmit)
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

func (h *UIHandler) localeAndDir(r *http.Request) (string, string) {
	lang := r.URL.Query().Get("lang")
	if lang == "en" {
		return "en", "ltr"
	}
	return "ar", "rtl"
}
