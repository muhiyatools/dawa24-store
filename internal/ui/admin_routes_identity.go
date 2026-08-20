package ui

import (
	"fmt"
	"net/http"

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
		g.Get("/admin/users/{id}/edit", h.AdminUserDetailPage)

		// 301 Redirects for duplicate user screens (Task B.1)
		g.Get("/admin/full-user", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/admin/users", http.StatusMovedPermanently)
		})
		g.Get("/admin/full-user/new-clients", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/admin/users?type=new", http.StatusMovedPermanently)
		})
		g.Get("/admin/customer-list", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/admin/users?type=customer", http.StatusMovedPermanently)
		})
		g.Get("/admin/customer-list/{id}", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, fmt.Sprintf("/admin/users/%s", chi.URLParam(r, "id")), http.StatusMovedPermanently)
		})
		g.Get("/admin/vendor-list", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/admin/users?type=vendor", http.StatusMovedPermanently)
		})
		g.Get("/admin/vendor-list/{id}", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, fmt.Sprintf("/admin/users/%s", chi.URLParam(r, "id")), http.StatusMovedPermanently)
		})
		g.Get("/admin/admin-list", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/admin/users?type=staff", http.StatusMovedPermanently)
		})
		g.Get("/admin/admins", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/admin/users?type=staff", http.StatusMovedPermanently)
		})
		g.Get("/admin/admins/{id}", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, fmt.Sprintf("/admin/users/%s", chi.URLParam(r, "id")), http.StatusMovedPermanently)
		})
		g.Get("/admin/admins/{id}/edit", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, fmt.Sprintf("/admin/users/%s/edit", chi.URLParam(r, "id")), http.StatusMovedPermanently)
		})

		g.Get("/admin/employee-activities", h.AdminEmployeeActivitiesPage)
		g.Get("/admin/user-address", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/admin/users", http.StatusMovedPermanently)
		})
		g.Get("/admin/user-address/{id}", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/admin/users", http.StatusMovedPermanently)
		})
		g.Get("/admin/user-organization", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/admin/users", http.StatusMovedPermanently)
		})
		g.Get("/admin/want-delete", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/admin/users?tab=deletion_requests", http.StatusMovedPermanently)
		})
		g.Get("/admin/want-delete/{id}", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/admin/users?tab=deletion_requests", http.StatusMovedPermanently)
		})

		// Roles & RBAC
		g.Get("/admin/roles", h.AdminRolesPage)
		g.Get("/admin/admin-roles", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/admin/roles", http.StatusMovedPermanently)
		})
		g.Get("/admin/admin-roles/{id}", h.AdminRoleDetailPage)
		g.Get("/admin/admin-permissions", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/admin/roles", http.StatusMovedPermanently)
		})
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
