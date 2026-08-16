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

// RegisterPageRoutes registers HTML view endpoints.
func (h *UIHandler) RegisterPageRoutes(r chi.Router) {
	RegisterStaticRoutes(r)

	r.Get("/", h.HomePage)
	r.Get("/auth/login", h.LoginPage)
	r.Get("/admin/dashboard", h.AdminDashboardPage)
	r.Get("/admin/approvals", h.AdminApprovalsPage)
	r.Get("/vendor/ingest", h.VendorIngestPage)
}

// HomePage renders customer marketplace homepage.
func (h *UIHandler) HomePage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = pages.CustomerHome(nil, "ar", "rtl").Render(r.Context(), w)
}

// LoginPage renders login screen.
func (h *UIHandler) LoginPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = pages.LoginPage("ar", "rtl", "").Render(r.Context(), w)
}

// AdminDashboardPage renders platform admin dashboard.
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

// AdminApprovalsPage renders organization approval queue.
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

// VendorIngestPage renders bulk import wizard.
func (h *UIHandler) VendorIngestPage(w http.ResponseWriter, r *http.Request) {
	var sessions []*ingest.ImportSession
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = pages.VendorIngest(sessions, "en", "ltr").Render(r.Context(), w)
}
