package ui

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/identity"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// AdminFullUserPage renders the centralized user management index.
func (h *UIHandler) AdminFullUserPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	var users []*identity.User
	if h.idSvc != nil {
		// AsSystem justified: platform administrator browsing users across all tenants
		users, _ = h.idSvc.AdminListUsers(database.AsSystem(ctx), "", "")
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminFullUserPage(users, "all", lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin full user page", "error", err)
	}
}

// AdminNewClientsPage renders recent new customer registrations.
func (h *UIHandler) AdminNewClientsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	var users []*identity.User
	if h.idSvc != nil {
		users, _ = h.idSvc.AdminListUsers(database.AsSystem(ctx), "customer", "")
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = pages.AdminFullUserPage(users, "new_clients", lang, dir).Render(ctx, w)
}

// AdminCustomerListPage renders pharmacy customer accounts.
func (h *UIHandler) AdminCustomerListPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	var users []*identity.User
	if h.idSvc != nil {
		users, _ = h.idSvc.AdminListUsers(database.AsSystem(ctx), "customer", "")
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = pages.AdminFullUserPage(users, "customers", lang, dir).Render(ctx, w)
}

// AdminVendorListPage renders vendor and supplier user accounts.
func (h *UIHandler) AdminVendorListPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	var users []*identity.User
	if h.idSvc != nil {
		users, _ = h.idSvc.AdminListUsers(database.AsSystem(ctx), "vendor", "")
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = pages.AdminFullUserPage(users, "vendors", lang, dir).Render(ctx, w)
}

// AdminStaffListPage renders platform administrators and staff users.
func (h *UIHandler) AdminStaffListPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	var users []*identity.User
	if h.idSvc != nil {
		users, _ = h.idSvc.AdminListUsers(database.AsSystem(ctx), "staff", "")
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = pages.AdminFullUserPage(users, "staff", lang, dir).Render(ctx, w)
}

// AdminUserDetailPage renders detailed user profile with role and organization info.
func (h *UIHandler) AdminUserDetailPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	idStr := chi.URLParam(r, "id")
	userID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || userID <= 0 {
		http.Redirect(w, r, "/admin/users", http.StatusSeeOther)
		return
	}

	var user *identity.User
	if h.idSvc != nil {
		user, _ = h.idSvc.GetUserByID(database.AsSystem(ctx), userID)
	}

	if user == nil {
		h.redirectWithNotice(w, r, "/admin/users", "error", "المستخدم غير موجود.")
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminUserDetailPage(user, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin user detail page", "error", err)
	}
}

// AdminUserAddressesPage renders address records for users.
func (h *UIHandler) AdminUserAddressesPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	var users []*identity.User
	if h.idSvc != nil {
		users, _ = h.idSvc.AdminListUsers(database.AsSystem(ctx), "", "")
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = pages.AdminFullUserPage(users, "addresses", lang, dir).Render(ctx, w)
}

// AdminUserOrganizationPage renders user-to-organization membership directory.
func (h *UIHandler) AdminUserOrganizationPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	var users []*identity.User
	if h.idSvc != nil {
		users, _ = h.idSvc.AdminListUsers(database.AsSystem(ctx), "", "")
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = pages.AdminFullUserPage(users, "organizations", lang, dir).Render(ctx, w)
}

// AdminWantDeletePage renders account deletion requests.
func (h *UIHandler) AdminWantDeletePage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	var requests []*identity.AccountDeletionRequest
	if h.idSvc != nil {
		requests, _ = h.idSvc.AdminListDeletionRequests(database.AsSystem(ctx), "")
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminDeletionRequestsPage(requests, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render deletion requests page", "error", err)
	}
}

// AdminEmployeeActivitiesPage renders employee audit trail.
func (h *UIHandler) AdminEmployeeActivitiesPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminEmployeeActivitiesPage(nil, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render employee activities", "error", err)
	}
}

// AdminRolesPage renders list of roles and permissions (RBAC).
func (h *UIHandler) AdminRolesPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	var roles []*org.Role
	if h.orgSvc != nil {
		// AsSystem justified: platform admin inspecting custom roles across all organizations
		roles, _ = h.orgSvc.ListRoles(database.AsSystem(ctx), 0)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminRolesPage(roles, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin roles page", "error", err)
	}
}

// AdminRoleDetailPage renders role permissions matrix editor.
func (h *UIHandler) AdminRoleDetailPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	idStr := chi.URLParam(r, "id")
	roleID, _ := strconv.ParseInt(idStr, 10, 64)

	var role *org.Role
	if h.orgSvc != nil && roleID > 0 {
		role, _ = h.orgSvc.GetRole(database.AsSystem(ctx), roleID)
	}

	if role == nil {
		http.Redirect(w, r, "/admin/roles", http.StatusSeeOther)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = pages.AdminRoleEditPage(role, lang, dir).Render(ctx, w)
}

// AdminRoleUpdateSubmit saves updated permissions for a role.
func (h *UIHandler) AdminRoleUpdateSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	idStr := chi.URLParam(r, "id")
	roleID, _ := strconv.ParseInt(idStr, 10, 64)

	_ = r.ParseForm()
	permissions := r.Form["permissions"]
	nameAr := r.PostFormValue("name_ar")
	desc := r.PostFormValue("description")

	if h.orgSvc != nil && roleID > 0 {
		role, err := h.orgSvc.GetRole(database.AsSystem(ctx), roleID)
		if err == nil && role != nil {
			role.Name = i18n.New(nameAr, nameAr)
			role.Description = desc
			role.Permissions = permissions
			_ = h.orgSvc.UpdateRole(database.AsSystem(ctx), role)
		}
	}

	http.Redirect(w, r, fmt.Sprintf("/admin/admin-roles/%d", roleID), http.StatusSeeOther)
}
