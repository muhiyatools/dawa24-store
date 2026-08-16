package ui

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/modules/ingest"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// UIHandler serves server-rendered HTML pages via Templ.
type UIHandler struct {
	catSvc *catalog.Service
	orgSvc *org.Service
	ingSvc *ingest.Service
}

// NewUIHandler creates a new UI page handler.
func NewUIHandler(catSvc *catalog.Service, orgSvc *org.Service, ingSvc *ingest.Service) *UIHandler {
	return &UIHandler{
		catSvc: catSvc,
		orgSvc: orgSvc,
		ingSvc: ingSvc,
	}
}

// RegisterPageRoutes registers HTML view endpoints across all 20 screens.
func (h *UIHandler) RegisterPageRoutes(r chi.Router) {
	RegisterStaticRoutes(r)

	// Public & Auth
	r.Get("/", h.HomePage)
	r.Get("/privacy", h.PrivacyPage)
	r.Get("/terms", h.TermsPage)
	r.Get("/auth/login", h.LoginPage)
	r.Get("/auth/forgot", h.ForgotPasswordPage)
	r.Get("/auth/reset", h.ResetPasswordPage)
	r.Get("/auth/register", h.OnboardingPage)
	r.Get("/onboarding", h.OnboardingPage)

	// Customer Buyer Experience
	r.Get("/catalog", h.CustomerCatalogPage)
	r.Get("/catalog/{id}", h.CustomerProductDetailPage)
	r.Get("/cart", h.CustomerCartPage)
	r.Get("/checkout", h.CustomerCheckoutPage)
	r.Get("/orders", h.CustomerOrdersPage)
	r.Get("/orders/{id}", h.CustomerOrdersPage)
	r.Get("/notifications", h.NotificationsPage)

	// Vendor Supplier Experience
	r.Get("/vendor/products", h.VendorProductsPage)
	r.Get("/vendor/products/new", h.VendorProductEditorPage)
	r.Get("/vendor/products/{id}", h.VendorProductEditorPage)
	r.Get("/vendor/inventory", h.VendorInventoryPage)
	r.Get("/vendor/transfers", h.VendorTransfersPage)
	r.Get("/vendor/ingest", h.VendorIngestPage)
	r.Get("/vendor/orders", h.VendorOrdersPage)
	r.Get("/vendor/offers", h.VendorOffersPage)

	// Platform Admin Experience
	r.Get("/admin/dashboard", h.AdminDashboardPage)
	r.Get("/admin/approvals", h.AdminApprovalsPage)
	r.Get("/admin/users", h.AdminUsersPage)
	r.Get("/admin/settings", h.AdminSettingsPage)
}

func (h *UIHandler) HomePage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = pages.CustomerHome(nil, "ar", "rtl").Render(r.Context(), w)
}

func (h *UIHandler) PrivacyPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = pages.PrivacyPolicy().Render(r.Context(), w)
}

func (h *UIHandler) TermsPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = pages.TermsOfService().Render(r.Context(), w)
}

func (h *UIHandler) LoginPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = pages.LoginPage("ar", "rtl", "").Render(r.Context(), w)
}

func (h *UIHandler) ForgotPasswordPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = pages.PasswordReset().Render(r.Context(), w)
}

func (h *UIHandler) ResetPasswordPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = pages.PasswordResetConfirm("").Render(r.Context(), w)
}

func (h *UIHandler) OnboardingPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = pages.Onboarding().Render(r.Context(), w)
}

func (h *UIHandler) CustomerCatalogPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = pages.CustomerCatalog().Render(r.Context(), w)
}

func (h *UIHandler) CustomerProductDetailPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = pages.CustomerProductDetail().Render(r.Context(), w)
}

func (h *UIHandler) CustomerCartPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = pages.CustomerCart().Render(r.Context(), w)
}

func (h *UIHandler) CustomerCheckoutPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = pages.CustomerCheckout().Render(r.Context(), w)
}

func (h *UIHandler) CustomerOrdersPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = pages.CustomerOrders().Render(r.Context(), w)
}

func (h *UIHandler) NotificationsPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = pages.Notifications().Render(r.Context(), w)
}

func (h *UIHandler) VendorProductsPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = pages.VendorProducts().Render(r.Context(), w)
}

func (h *UIHandler) VendorProductEditorPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = pages.VendorProductEditor().Render(r.Context(), w)
}

func (h *UIHandler) VendorInventoryPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = pages.VendorInventory().Render(r.Context(), w)
}

func (h *UIHandler) VendorTransfersPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = pages.VendorTransfers().Render(r.Context(), w)
}

func (h *UIHandler) VendorOrdersPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = pages.VendorOrders().Render(r.Context(), w)
}

func (h *UIHandler) VendorOffersPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = pages.VendorOffers().Render(r.Context(), w)
}

func (h *UIHandler) VendorIngestPage(w http.ResponseWriter, r *http.Request) {
	var sessions []*ingest.ImportSession
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = pages.VendorIngest(sessions, "en", "ltr").Render(r.Context(), w)
}

func (h *UIHandler) AdminDashboardPage(w http.ResponseWriter, r *http.Request) {
	stats := pages.AdminDashboardStats{
		TotalUsers:         150,
		TotalOrganizations: 45,
		PendingApprovals:   3,
		TotalOrders:        1280,
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = pages.AdminDashboard(stats, "en", "ltr").Render(r.Context(), w)
}

func (h *UIHandler) AdminApprovalsPage(w http.ResponseWriter, r *http.Request) {
	var pending []*org.Organization
	if h.orgSvc != nil {
		pendingStatus := org.StatusPending
		list, err := h.orgSvc.ListOrganizations(r.Context(), nil, &pendingStatus, 50, 0)
		if err == nil {
			pending = list
		}
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = pages.AdminApprovals(pending, "en", "ltr").Render(r.Context(), w)
}

func (h *UIHandler) AdminUsersPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = pages.AdminUsers().Render(r.Context(), w)
}

func (h *UIHandler) AdminSettingsPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = pages.AdminSettings().Render(r.Context(), w)
}
