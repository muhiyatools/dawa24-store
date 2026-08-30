package ui

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/identity"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/rbac"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// The platform roles screen, /admin/roles.
//
// What it replaced: a page that listed org.roles across every organization
// under database.AsSystem — a platform administrator looking at other
// companies' internal roles, which they could also edit — and an editor whose
// permission checkboxes were a hardcoded list of eleven strings in the
// template, four of which named permissions that did not exist.

// AdminRolesPage lists the platform roles a super admin manages.
func (h *UIHandler) AdminRolesPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	actor := authctx.FromContext(ctx)

	var roles []*identity.PlatformRole
	if h.idSvc != nil {
		var err error
		roles, err = h.idSvc.ListPlatformRoles(ctx)
		if err != nil {
			h.log.ErrorContext(ctx, "list platform roles", "error", err)
		}
	}

	view := pages.RolesView{
		Scope:      rbac.ScopeAdmin,
		BasePath:   "/admin/roles",
		Title:      i18n.T(lang, "admin.roles.title"),
		Subtitle:   i18n.T(lang, "admin.roles.subtitle"),
		CanCreate:  actor.Can("identity.admin_role.update"),
		CanEdit:    actor.Can("identity.admin_role.update"),
		CanDelete:  actor.Can("identity.admin_role.delete"),
		NoticeKind: r.URL.Query().Get("notice"),
		Notice:     r.URL.Query().Get("msg"),
	}
	for _, role := range roles {
		view.Roles = append(view.Roles, pages.RoleRow{
			ID:          role.Key,
			Name:        role.Name.Get(i18n.ParseLang(lang)),
			Description: role.Description,
			IsSystem:    role.IsSystem,
			IsOwner:     role.IsOwner,
			Badge:       staffBadge(role, lang),
			GrantCount:  len(role.Permissions),
			MemberCount: role.UserCount,
		})
	}

	h.renderPage(ctx, w, "render admin roles page", pages.RolesPage(view, lang, dir))
}

// AdminRoleDetailPage renders the permission matrix for one platform role.
func (h *UIHandler) AdminRoleDetailPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	actor := authctx.FromContext(ctx)
	key := chi.URLParam(r, "key")

	if h.idSvc == nil {
		h.redirectWithNotice(w, r, "/admin/roles", "error", i18n.T(lang, "admin.roles.id_service_unavailable"))
		return
	}
	role, err := h.idSvc.GetPlatformRole(ctx, key)
	if err != nil {
		h.redirectWithNotice(w, r, "/admin/roles", "error", i18n.T(lang, "admin.roles.not_found"))
		return
	}

	view := pages.RoleEditView{
		Scope:           rbac.ScopeAdmin,
		BasePath:        "/admin/roles",
		ID:              role.Key,
		Key:             role.Key,
		Name:            role.Name.Get(i18n.ParseLang(lang)),
		NameAr:          role.Name["ar"],
		NameEn:          role.Name["en"],
		Description:     role.Description,
		IsSystem:        role.IsSystem,
		IsOwner:         role.IsOwner,
		ShowStaffToggle: true,
		IsStaff:         role.IsStaff,
		ReadOnly:        role.IsOwner || !actor.Can("identity.admin_role.update"),
		Granted:         rbac.NewSet(role.Permissions),
		Sections:        rbac.Default().Matrix(rbac.ScopeAdmin),
		MemberCount:     role.UserCount,
	}

	h.renderPage(ctx, w, "render admin role editor", pages.RoleEditPage(view, lang, dir))
}

// AdminRoleCreateSubmit adds a moderator role.
func (h *UIHandler) AdminRoleCreateSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor := authctx.FromContext(ctx)
	lang := langOf(r)
	if h.idSvc == nil {
		h.redirectWithNotice(w, r, "/admin/roles", "error", i18n.T(lang, "admin.roles.id_service_unavailable"))
		return
	}
	_ = r.ParseForm()

	role, err := h.idSvc.CreatePlatformRole(ctx, identity.PlatformRoleInput{
		Key:         r.PostFormValue("key"),
		Name:        formName(r),
		Description: r.PostFormValue("description"),
		IsStaff:     r.PostFormValue("is_staff") != "",
		Permissions: r.Form["permissions"],
		ActorID:     actor.UserID,
	})
	if err != nil {
		h.redirectWithNotice(w, r, "/admin/roles", "error", h.safeMessage(err, lang))
		return
	}
	h.invalidatePermissions(actor.UserID, 0)
	h.redirectWithNotice(w, r, "/admin/roles/"+role.Key, "success", i18n.T(lang, "admin.roles.created_success"))
}

// AdminRoleUpdateSubmit saves a role's label, staff flag and permissions.
func (h *UIHandler) AdminRoleUpdateSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor := authctx.FromContext(ctx)
	lang := langOf(r)
	key := chi.URLParam(r, "key")
	if h.idSvc == nil {
		h.redirectWithNotice(w, r, "/admin/roles", "error", i18n.T(lang, "admin.roles.id_service_unavailable"))
		return
	}
	_ = r.ParseForm()

	if _, err := h.idSvc.UpdatePlatformRole(ctx, key, identity.PlatformRoleInput{
		Name:        formName(r),
		Description: r.PostFormValue("description"),
		IsStaff:     r.PostFormValue("is_staff") != "",
		Permissions: r.Form["permissions"],
		ActorID:     actor.UserID,
	}); err != nil {
		h.redirectWithNotice(w, r, "/admin/roles/"+key, "error", h.safeMessage(err, lang))
		return
	}
	// The editing administrator may have just changed their own role. Dropping
	// the cached grant here means the very next page they open reflects it,
	// rather than the version counter's few seconds later.
	h.invalidatePermissions(actor.UserID, 0)
	h.redirectWithNotice(w, r, "/admin/roles/"+key, "success", i18n.T(lang, "admin.roles.permissions_saved_success"))
}

// AdminRoleDeleteSubmit removes a custom role.
func (h *UIHandler) AdminRoleDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor := authctx.FromContext(ctx)
	lang := langOf(r)
	key := chi.URLParam(r, "key")
	if h.idSvc == nil {
		h.redirectWithNotice(w, r, "/admin/roles", "error", i18n.T(lang, "admin.roles.id_service_unavailable"))
		return
	}
	if err := h.idSvc.DeletePlatformRole(ctx, key, actor.UserID); err != nil {
		h.redirectWithNotice(w, r, "/admin/roles", "error", h.safeMessage(err, lang))
		return
	}
	h.invalidatePermissions(actor.UserID, 0)
	h.redirectWithNotice(w, r, "/admin/roles", "success", i18n.T(lang, "admin.roles.deleted_success"))
}

// AdminUserRoleAssignSubmit puts a user account into a platform role.
func (h *UIHandler) AdminUserRoleAssignSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor := authctx.FromContext(ctx)
	lang := langOf(r)
	userID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	target := "/admin/users/" + chi.URLParam(r, "id")

	if h.idSvc == nil || userID <= 0 {
		h.redirectWithNotice(w, r, target, "error", i18n.T(lang, "admin.roles.invalid_request"))
		return
	}
	// An administrator changing their own role could hand themselves the owner
	// role, or strip their own access with no way back. Both are refused; a
	// second administrator makes the change.
	if userID == actor.UserID {
		h.redirectWithNotice(w, r, target, "error", i18n.T(lang, "admin.roles.cannot_change_own_role"))
		return
	}
	key := strings.TrimSpace(r.PostFormValue("role"))
	if err := h.idSvc.AssignPlatformRole(ctx, userID, key, actor.UserID); err != nil {
		h.redirectWithNotice(w, r, target, "error", h.safeMessage(err, lang))
		return
	}
	h.invalidatePermissions(userID, 0)
	h.redirectWithNotice(w, r, target, "success", i18n.T(lang, "admin.roles.user_role_updated_success"))
}

// invalidatePermissions drops one caller's cached grant in this process. The
// version counter handles the other processes; this only removes the delay for
// the one that performed the write.
func (h *UIHandler) invalidatePermissions(userID, orgID int64) {
	if h.resolver != nil {
		h.resolver.Invalidate(userID, orgID)
	}
}

func formName(r *http.Request) i18n.Text {
	ar := strings.TrimSpace(r.PostFormValue("name_ar"))
	en := strings.TrimSpace(r.PostFormValue("name_en"))
	if ar == "" {
		ar = en
	}
	if en == "" {
		en = ar
	}
	return i18n.New(ar, en)
}

func staffBadge(role *identity.PlatformRole, lang any) string {
	if role.IsOwner {
		return i18n.T(lang, "admin.roles.badge_full_access")
	}
	if role.IsStaff {
		return i18n.T(lang, "admin.roles.badge_staff")
	}
	return i18n.T(lang, "admin.roles.badge_regular")
}

// AdminStaffCreateSubmit creates a moderator or administrator account from
// /admin/users and puts it into a staff role in one step.
//
// Gated on identity.admin_role.assign, not on identity.user.update: creating a
// staff account *is* granting a role, and whoever can do it can mint an
// account holding whatever any staff role holds.
func (h *UIHandler) AdminStaffCreateSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor := authctx.FromContext(ctx)
	lang := langOf(r)
	const target = "/admin/users?tab=staff"

	if h.idSvc == nil {
		h.redirectWithNotice(w, r, target, "error", i18n.T(lang, "admin.roles.id_service_unavailable"))
		return
	}
	if err := r.ParseForm(); err != nil {
		h.redirectWithNotice(w, r, target, "error", i18n.T(lang, "admin.roles.invalid_form"))
		return
	}

	user, err := h.idSvc.CreateStaffUser(ctx, identity.StaffUserInput{
		Email:    r.PostFormValue("email"),
		Password: r.PostFormValue("password"),
		NameAr:   r.PostFormValue("name_ar"),
		NameEn:   r.PostFormValue("name_en"),
		Phone:    r.PostFormValue("phone"),
		RoleKey:  r.PostFormValue("role"),
		ActorID:  actor.UserID,
	})
	if err != nil {
		h.redirectWithNotice(w, r, target, "error", h.safeMessage(err, lang))
		return
	}
	h.redirectWithNotice(w, r, fmt.Sprintf("/admin/users/%d", user.ID),
		"success", i18n.T(lang, "admin.roles.staff_created_success"))
}
