package ui

import (
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
		Title:      "الأدوار والصلاحيات",
		Subtitle:   "أدوار مشرفي المنصة: ما الذي يراه كل مشرف وما الذي يستطيع تغييره.",
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
			Badge:       staffBadge(role),
			GrantCount:  len(role.Permissions),
			MemberCount: role.UserCount,
		})
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.RolesPage(view, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin roles page", "error", err)
	}
}

// AdminRoleDetailPage renders the permission matrix for one platform role.
func (h *UIHandler) AdminRoleDetailPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	actor := authctx.FromContext(ctx)
	key := chi.URLParam(r, "key")

	if h.idSvc == nil {
		h.redirectWithNotice(w, r, "/admin/roles", "error", "خدمة الهوية غير متوفرة.")
		return
	}
	role, err := h.idSvc.GetPlatformRole(ctx, key)
	if err != nil {
		h.redirectWithNotice(w, r, "/admin/roles", "error", "الدور غير موجود.")
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

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.RoleEditPage(view, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin role editor", "error", err)
	}
}

// AdminRoleCreateSubmit adds a moderator role.
func (h *UIHandler) AdminRoleCreateSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor := authctx.FromContext(ctx)
	if h.idSvc == nil {
		h.redirectWithNotice(w, r, "/admin/roles", "error", "خدمة الهوية غير متوفرة.")
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
		h.redirectWithNotice(w, r, "/admin/roles", "error", h.safeMessage(err, langOf(r)))
		return
	}
	h.invalidatePermissions(actor.UserID, 0)
	h.redirectWithNotice(w, r, "/admin/roles/"+role.Key, "success", "تم إنشاء الدور. حدّد صلاحياته الآن.")
}

// AdminRoleUpdateSubmit saves a role's label, staff flag and permissions.
func (h *UIHandler) AdminRoleUpdateSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor := authctx.FromContext(ctx)
	key := chi.URLParam(r, "key")
	if h.idSvc == nil {
		h.redirectWithNotice(w, r, "/admin/roles", "error", "خدمة الهوية غير متوفرة.")
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
		h.redirectWithNotice(w, r, "/admin/roles/"+key, "error", h.safeMessage(err, langOf(r)))
		return
	}
	// The editing administrator may have just changed their own role. Dropping
	// the cached grant here means the very next page they open reflects it,
	// rather than the version counter's few seconds later.
	h.invalidatePermissions(actor.UserID, 0)
	h.redirectWithNotice(w, r, "/admin/roles/"+key, "success", "تم حفظ صلاحيات الدور.")
}

// AdminRoleDeleteSubmit removes a custom role.
func (h *UIHandler) AdminRoleDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor := authctx.FromContext(ctx)
	key := chi.URLParam(r, "key")
	if h.idSvc == nil {
		h.redirectWithNotice(w, r, "/admin/roles", "error", "خدمة الهوية غير متوفرة.")
		return
	}
	if err := h.idSvc.DeletePlatformRole(ctx, key, actor.UserID); err != nil {
		h.redirectWithNotice(w, r, "/admin/roles", "error", h.safeMessage(err, langOf(r)))
		return
	}
	h.invalidatePermissions(actor.UserID, 0)
	h.redirectWithNotice(w, r, "/admin/roles", "success", "تم حذف الدور.")
}

// AdminUserRoleAssignSubmit puts a user account into a platform role.
func (h *UIHandler) AdminUserRoleAssignSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor := authctx.FromContext(ctx)
	userID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	target := "/admin/users/" + chi.URLParam(r, "id")

	if h.idSvc == nil || userID <= 0 {
		h.redirectWithNotice(w, r, target, "error", "طلب غير صالح.")
		return
	}
	// An administrator changing their own role could hand themselves the owner
	// role, or strip their own access with no way back. Both are refused; a
	// second administrator makes the change.
	if userID == actor.UserID {
		h.redirectWithNotice(w, r, target, "error", "لا يمكنك تغيير دور حسابك بنفسك.")
		return
	}
	key := strings.TrimSpace(r.PostFormValue("role"))
	if err := h.idSvc.AssignPlatformRole(ctx, userID, key, actor.UserID); err != nil {
		h.redirectWithNotice(w, r, target, "error", h.safeMessage(err, langOf(r)))
		return
	}
	h.invalidatePermissions(userID, 0)
	h.redirectWithNotice(w, r, target, "success", "تم تحديث دور المستخدم وإنهاء جلساته.")
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

func staffBadge(role *identity.PlatformRole) string {
	if role.IsOwner {
		return "كامل الصلاحيات"
	}
	if role.IsStaff {
		return "لوحة الإدارة"
	}
	return "حساب عادي"
}
