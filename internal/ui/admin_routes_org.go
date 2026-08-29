package ui

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
)

func (h *UIHandler) registerAdminOrgRoutes(r chi.Router) {
	// Organizations & Approvals
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("org.organization.view"))
		g.Get("/admin/organizations", h.AdminOrganizationsPage)
		g.Get("/admin/organizations/{id}", h.AdminOrganizationDetailPage)
		g.Get("/admin/organizations/{id}/info", h.AdminOrganizationInfoPage)
		g.Get("/admin/organizations/{id}/users", h.AdminOrganizationUsersPage)
		g.Get("/admin/organizations/{id}/branches", h.AdminOrganizationBranchesPage)
		g.Get("/admin/vendors", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/admin/organizations?type=vendor", http.StatusMovedPermanently)
		})
		g.Get("/admin/suppliers", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/admin/organizations?type=vendor", http.StatusMovedPermanently)
		})
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("org.organization.update"))
		g.Post("/admin/organizations/{id}/suspend", h.AdminOrgSuspendSubmit)
	})

	// Approving an organization admits a company to the marketplace. It is a
	// separate right from editing one, because the reviewer who checks a
	// pharmacy licence is not the operator who corrects a typo in its address.
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("org.approval.view"))
		g.Get("/admin/approvals", h.AdminApprovalsPage)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("org.approval.decide"))
		g.Post("/admin/organizations/{id}/approve", h.AdminOrgApproveSubmit)
		g.Post("/admin/organizations/{id}/reject", h.AdminOrgRejectSubmit)
		g.Post("/admin/approvals/{id}/approve", h.AdminApproveOrgSubmit)
		g.Post("/admin/approvals/{id}/reject", h.AdminRejectOrgSubmit)
		g.Post("/admin/approvals/{id}/review", h.AdminOrgReviewSubmit)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("hr.document.view"))
		g.Get("/admin/documents/{id}/view", h.DocumentViewHandler)
		g.Get("/admin/documents/{id}/download", h.DocumentDownloadHandler)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("hr.document.update"))
		g.Post("/admin/document-requests", h.AdminCreateDocumentRequestSubmit)
		g.Post("/admin/document-requests/{id}/cancel", h.AdminCancelDocumentRequestSubmit)
		g.Post("/admin/documents/{id}/verify", h.AdminVerifyUploadedDocSubmit)
	})

	// Branches Oversight
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("org.branch.view"))
		g.Get("/admin/branches", h.AdminBranchesPage)
		g.Get("/admin/branches/{id}", h.AdminBranchDetailPage)
		g.Get("/admin/branches/{id}/products", h.AdminBranchProductsPage)
		g.Get("/admin/branches/{id}/users", h.AdminBranchUsersPage)
	})

	// Weekly Coverages Oversight & Management
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("workflow.coverage.view"))
		g.Get("/admin/weekly-coverages", h.AdminWeeklyCoveragesPage)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("workflow.coverage.update"))
		g.Post("/admin/weekly-coverages/new", h.AdminWeeklyCoverageCreateSubmit)
		g.Post("/admin/weekly-coverages/{id}/toggle", h.AdminWeeklyCoverageToggleSubmit)
		g.Post("/admin/weekly-coverages/{id}/delete", h.AdminWeeklyCoverageDeleteSubmit)
	})

	// Institutional Works
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("org.institutional_work.view"))
		g.Get("/admin/institutional", h.AdminInstitutionalPage)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("org.institutional_work.update"))
		g.Post("/admin/institutional", h.AdminInstitutionalNewSubmit)
		g.Post("/admin/institutional/new", h.AdminInstitutionalNewSubmit)
		g.Post("/admin/institutional/{id}", h.AdminInstitutionalEditSubmit)
		g.Post("/admin/institutional/{id}/edit", h.AdminInstitutionalEditSubmit)
		g.Post("/admin/institutional/{id}/status", h.AdminInstitutionalStatusSubmit)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("org.institutional_work.delete"))
		g.Post("/admin/institutional/{id}/delete", h.AdminInstitutionalDeleteSubmit)
	})
}
