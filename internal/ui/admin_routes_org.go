package ui

import (
	"github.com/go-chi/chi/v5"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
)

func (h *UIHandler) registerAdminOrgRoutes(r chi.Router) {
	// Organizations & Approvals
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("org.organization.view", h.log))
		g.Get("/admin/organizations", h.AdminOrganizationsPage)
		g.Get("/admin/organizations/{id}", h.AdminOrganizationDetailPage)
		g.Get("/admin/organizations/{id}/info", h.AdminOrganizationInfoPage)
		g.Get("/admin/organizations/{id}/users", h.AdminOrganizationUsersPage)
		g.Get("/admin/organizations/{id}/branches", h.AdminOrganizationBranchesPage)
		g.Get("/admin/vendors", h.AdminOrganizationsPage)
		g.Get("/admin/suppliers", h.AdminOrganizationsPage)
		g.Get("/admin/approvals", h.AdminApprovalsPage)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("org.organization.update", h.log))
		g.Post("/admin/organizations/{id}/approve", h.AdminOrgApproveSubmit)
		g.Post("/admin/organizations/{id}/reject", h.AdminOrgRejectSubmit)
		g.Post("/admin/organizations/{id}/suspend", h.AdminOrgSuspendSubmit)
		g.Post("/admin/approvals/{id}/approve", h.AdminApproveOrgSubmit)
		g.Post("/admin/approvals/{id}/reject", h.AdminRejectOrgSubmit)
		g.Post("/admin/approvals/{id}/review", h.AdminOrgReviewSubmit)
	})

	// Branches Oversight
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("org.branch.view", h.log))
		g.Get("/admin/branches", h.AdminBranchesPage)
		g.Get("/admin/branches/{id}", h.AdminBranchDetailPage)
		g.Get("/admin/branches/{id}/products", h.AdminBranchProductsPage)
		g.Get("/admin/branches/{id}/users", h.AdminBranchUsersPage)
	})

	// Weekly Coverages Oversight
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("workflow.coverage.manage", h.log))
		g.Get("/admin/weekly-coverages", h.AdminWeeklyCoveragesPage)
		g.Get("/admin/weekly-coverages/add", h.AdminWeeklyCoverageNewPage)
		g.Post("/admin/weekly-coverages", h.AdminWeeklyCoverageCreateSubmit)
		g.Get("/admin/weekly-coverages/edit/{id}", h.AdminWeeklyCoverageEditPage)
		g.Get("/admin/weekly-coverages/{id}", h.AdminWeeklyCoverageDetailPage)
		g.Post("/admin/weekly-coverages/{id}", h.AdminWeeklyCoverageUpdateSubmit)
		g.Post("/admin/weekly-coverages/{id}/delete", h.AdminWeeklyCoverageDeleteSubmit)
		g.Post("/admin/weekly-coverages/{id}/toggle", h.AdminWeeklyCoverageToggleSubmit)
	})

	// Institutional Works
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("org.institutional_work.view", h.log))
		g.Get("/admin/institutional", h.AdminInstitutionalPage)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("org.institutional_work.update", h.log))
		g.Post("/admin/institutional", h.AdminInstitutionalNewSubmit)
		g.Post("/admin/institutional/new", h.AdminInstitutionalNewSubmit)
		g.Post("/admin/institutional/{id}", h.AdminInstitutionalEditSubmit)
		g.Post("/admin/institutional/{id}/edit", h.AdminInstitutionalEditSubmit)
		g.Post("/admin/institutional/{id}/status", h.AdminInstitutionalStatusSubmit)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("org.institutional_work.delete", h.log))
		g.Post("/admin/institutional/{id}/delete", h.AdminInstitutionalDeleteSubmit)
	})
}
