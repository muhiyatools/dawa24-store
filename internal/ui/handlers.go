package ui

import (
	"log/slog"
	"net/http"
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
}

func (h *UIHandler) renderError(w http.ResponseWriter, r *http.Request, err error) {
	ctx := r.Context()
	h.log.ErrorContext(ctx, "ui error rendering page", "error", err, "path", r.URL.Path)

	lang, dir := h.localeAndDir(r)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	if h.isHTMX(r) {
		w.WriteHeader(http.StatusOK)
		_ = components.ErrorState(components.ErrorStateProps{
			Title:      "حدث خطأ أثناء تحميل البيانات",
			Message:    err.Error(),
			RetryURL:   r.URL.String(),
			RetryLabel: "إعادة المحاولة",
		}).Render(ctx, w)
		return
	}

	w.WriteHeader(http.StatusOK)
	_ = pages.ErrorPage(
		"عذراً، حدث خطأ",
		err.Error(),
		"/",
		lang,
		dir,
	).Render(ctx, w)
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
