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
	"github.com/muhiya/dawa24-store/internal/shared/pagination"
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

	h.renderPage(ctx, w, "render admin full user page", pages.AdminFullUserPage(users, "all", lang, dir))
}

// AdminNewClientsPage renders recent new customer registrations.
func (h *UIHandler) AdminNewClientsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	var users []*identity.User
	if h.idSvc != nil {
		users, _ = h.idSvc.AdminListUsers(database.AsSystem(ctx), "customer", "")
	}

	h.renderPage(ctx, w, "render admin new clients", pages.AdminFullUserPage(users, "new_clients", lang, dir))
}

// AdminCustomerListPage renders pharmacy customer accounts.
func (h *UIHandler) AdminCustomerListPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	var users []*identity.User
	if h.idSvc != nil {
		users, _ = h.idSvc.AdminListUsers(database.AsSystem(ctx), "customer", "")
	}

	h.renderPage(ctx, w, "render admin customers", pages.AdminFullUserPage(users, "customers", lang, dir))
}

// AdminVendorListPage renders vendor and supplier user accounts.
func (h *UIHandler) AdminVendorListPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	var users []*identity.User
	if h.idSvc != nil {
		users, _ = h.idSvc.AdminListUsers(database.AsSystem(ctx), "vendor", "")
	}

	h.renderPage(ctx, w, "render admin vendors", pages.AdminFullUserPage(users, "vendors", lang, dir))
}

// AdminStaffListPage renders platform administrators and staff users.
func (h *UIHandler) AdminStaffListPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	var users []*identity.User
	if h.idSvc != nil {
		users, _ = h.idSvc.AdminListUsers(database.AsSystem(ctx), "staff", "")
	}

	h.renderPage(ctx, w, "render admin staff", pages.AdminFullUserPage(users, "staff", lang, dir))
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
		h.redirectWithNotice(w, r, "/admin/users", "error", i18n.T(lang, "admin.users.not_found"))
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

	// The hierarchy panel, for a مشرف and a viewer who may reassign one.
	view.IsModerator = user.Role == identity.RoleModerator
	view.CanAssignModerator = actor.Can("identity.moderator.assign")
	if view.IsModerator && view.CanAssignModerator && h.idSvc != nil {
		sysCtx := database.AsSystem(ctx)
		if mods, err := h.idSvc.ListModerators(sysCtx); err == nil {
			for _, m := range mods {
				if m == nil {
					continue
				}
				if m.UserID == user.ID {
					view.ModeratorParentID = m.ParentID
					view.SubordinateCount = m.SubordinateCount
					continue
				}
				// Only top-level moderators can be parents, and nobody can be
				// their own. Offering either would produce a form whose only
				// outcome is the service refusing it.
				if !m.IsMain() {
					continue
				}
				view.MainModerators = append(view.MainModerators, pages.AdminModeratorOption{
					UserID: m.UserID,
					Name:   m.Name.Get(i18n.ParseLang(lang)),
					Team:   m.SubordinateCount,
				})
			}
		} else {
			h.log.ErrorContext(ctx, "list moderators for user detail", "error", err)
		}
	}

	h.renderPage(ctx, w, "render admin user detail page", pages.AdminUserDetailPage(view, lang, dir))
}

// AdminUserAddressesPage renders address records for users.
func (h *UIHandler) AdminUserAddressesPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	var users []*identity.User
	if h.idSvc != nil {
		users, _ = h.idSvc.AdminListUsers(database.AsSystem(ctx), "", "")
	}

	h.renderPage(ctx, w, "render admin user addresses", pages.AdminFullUserPage(users, "addresses", lang, dir))
}

// AdminUserOrganizationPage renders user-to-organization membership directory.
func (h *UIHandler) AdminUserOrganizationPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	limit := pagination.RowsPerPage(r)
	page := pagination.PageNumber(r)
	offset := (page - 1) * limit

	statusFilter := strings.TrimSpace(r.URL.Query().Get("status"))
	data := &pages.AdminUserOrgData{
		ActiveTab: statusFilter,
		Page:      page,
		PerPage:   limit,
	}

	sysCtx := database.AsSystem(ctx)
	if h.orgSvc != nil {
		list, total, err := h.orgSvc.ListAllUserOrganizationsWithTotal(sysCtx, statusFilter, limit, offset)
		if err == nil {
			data.UserOrgs = list
			data.TotalCount = total
		}
	}

	h.renderPage(ctx, w, "render admin user organizations", pages.AdminUserOrganizationsPage(lang, dir, data))
}

// AdminWantDeletePage redirects account deletion requests to the unified deletion requests screen.
func (h *UIHandler) AdminWantDeletePage(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/admin/deletion-requests?tab=users", http.StatusMovedPermanently)
}
