package ui

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/identity"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
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
	if err := pages.AdminFullUserPage(users, "new_clients", lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin new clients", "error", err)
	}
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
	if err := pages.AdminFullUserPage(users, "customers", lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin customers", "error", err)
	}
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
	if err := pages.AdminFullUserPage(users, "vendors", lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin vendors", "error", err)
	}
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
	if err := pages.AdminFullUserPage(users, "staff", lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin staff", "error", err)
	}
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

	actor := authctx.FromContext(ctx)
	view := pages.AdminUserDetailView{
		User:     user,
		SelfEdit: actor.UserID == user.ID,
	}
	// The assignable roles are offered only to a viewer who may assign them.
	// The route refuses the post regardless; not rendering the control means
	// the page does not offer an action it knows will fail.
	if actor.Can("identity.admin_role.assign") && h.idSvc != nil {
		roles, err := h.idSvc.ListPlatformRoles(ctx)
		if err != nil {
			h.log.ErrorContext(ctx, "list platform roles for user detail", "error", err)
		}
		for _, role := range roles {
			view.Roles = append(view.Roles, pages.AdminUserRoleOption{
				Key:      role.Key,
				Name:     role.Name.Get(i18n.ParseLang(lang)),
				IsStaff:  role.IsStaff,
				Selected: role.Key == user.Role,
			})
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminUserDetailPage(view, lang, dir).Render(ctx, w); err != nil {
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
	if err := pages.AdminFullUserPage(users, "addresses", lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin user addresses", "error", err)
	}
}

// AdminUserOrganizationPage renders user-to-organization membership directory.
func (h *UIHandler) AdminUserOrganizationPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	statusFilter := strings.TrimSpace(r.URL.Query().Get("status"))
	data := &pages.AdminUserOrgData{
		ActiveTab: statusFilter,
	}

	sysCtx := database.AsSystem(ctx)
	if h.orgSvc != nil {
		all, _ := h.orgSvc.ListAllUserOrganizations(sysCtx, "")
		data.TotalCount = len(all)

		list, err := h.orgSvc.ListAllUserOrganizations(sysCtx, statusFilter)
		if err == nil {
			data.UserOrgs = list
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminUserOrganizationsPage(lang, dir, data).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin user organizations", "error", err)
	}
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
