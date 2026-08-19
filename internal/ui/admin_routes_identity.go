package ui

import (
	"github.com/go-chi/chi/v5"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
)

func (h *UIHandler) registerAdminIdentityRoutes(r chi.Router) {
	// Users Directory & Classifications
	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("identity.user.view", h.log))
		g.Get("/admin/users", h.AdminUsersPage)
		g.Get("/admin/users/{id}", h.AdminUserDetailPage)
		g.Get("/admin/users/{id}/info", h.AdminUserDetailPage)
		g.Get("/admin/full-user", h.AdminFullUserPage)
		g.Get("/admin/full-user/new-clients", h.AdminNewClientsPage)
		g.Get("/admin/customer-list", h.AdminCustomerListPage)
		g.Get("/admin/customer-list/{id}", h.AdminUserDetailPage)
		g.Get("/admin/vendor-list", h.AdminVendorListPage)
		g.Get("/admin/vendor-list/{id}", h.AdminUserDetailPage)
		g.Get("/admin/admin-list", h.AdminStaffListPage)
		g.Get("/admin/admins", h.AdminStaffListPage)
		g.Get("/admin/admins/{id}", h.AdminUserDetailPage)
		g.Get("/admin/admins/{id}/edit", h.AdminUserDetailPage)
		g.Get("/admin/employee-activities", h.AdminEmployeeActivitiesPage)
		g.Get("/admin/user-address", h.AdminUserAddressesPage)
		g.Get("/admin/user-address/{id}", h.AdminUserAddressesPage)
		g.Get("/admin/user-organization", h.AdminUserOrganizationPage)
		g.Get("/admin/want-delete", h.AdminWantDeletePage)
		g.Get("/admin/want-delete/{id}", h.AdminWantDeletePage)
		
		// Roles & RBAC
		g.Get("/admin/roles", h.AdminRolesPage)
		g.Get("/admin/admin-roles", h.AdminRolesPage)
		g.Get("/admin/admin-roles/{id}", h.AdminRoleDetailPage)
		g.Get("/admin/admin-permissions", h.AdminRolesPage)
	})

	r.Group(func(g chi.Router) {
		g.Use(authctx.RequirePagePermission("identity.user.update", h.log))
		g.Post("/admin/users/{id}/suspend", h.AdminUserSuspendSubmit)
		g.Post("/admin/users/{id}/reactivate", h.AdminUserReactivateSubmit)
		g.Post("/admin/users/{id}/reset-mfa", h.AdminUserResetMFASubmit)
		g.Post("/admin/users/deletion/{id}/approve", h.AdminUserDeletionApproveSubmit)
		g.Post("/admin/users/deletion/{id}/reject", h.AdminUserDeletionRejectSubmit)
		g.Post("/admin/admin-roles/{id}", h.AdminRoleUpdateSubmit)
	})
}
