package ui

import (
	"net/http"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// CustomerTeamPage lists the pharmacy's employees and the role each holds.
//
// The pharmacy dashboard had employee create, edit, delete and status routes
// but no page: an owner could add staff and then have nowhere to see them, and
// no way to say what any of them were allowed to do. The roles they can be
// assigned are the pharmacy's own — a role belonging to another company is not
// in the list and would be refused on submit even if it were.
func (h *UIHandler) CustomerTeamPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	actor := authctx.FromContext(ctx)

	if h.orgSvc == nil || actor.OrganizationID <= 0 {
		http.NotFound(w, r)
		return
	}
	h.ensureCompanyRoles(ctx, actor.OrganizationID, actor.OrgType)

	view := pages.TenantTeamView{
		Title:      "فريق العمل والموظفون",
		RolesPath:  "/customer/roles",
		ActionBase: "/customer/employees",
		CanCreate:  actor.Can("pharmacy.team.create"),
		CanUpdate:  actor.Can("pharmacy.team.update"),
		CanDelete:  actor.Can("pharmacy.team.delete"),
		CanAssign:  actor.Can("pharmacy.role.assign"),
		NoticeKind: r.URL.Query().Get("notice"),
		Notice:     r.URL.Query().Get("msg"),
	}

	roles, err := h.orgSvc.ListRoles(ctx, actor.OrganizationID)
	if err != nil {
		h.log.ErrorContext(ctx, "list company roles for team page",
			"error", err, "organization_id", actor.OrganizationID)
	}
	// roleByKey lets a member carrying only a legacy role_key still show the
	// company's own role for it, rather than an empty cell.
	roleByKey := map[string]int64{}
	roleNameByID := map[int64]string{}
	for _, role := range roles {
		name := role.Name.Get(i18n.ParseLang(lang))
		view.Roles = append(view.Roles, pages.TenantRoleOption{ID: role.ID, Name: name})
		roleByKey[role.Key] = role.ID
		roleNameByID[role.ID] = name
	}

	employees, err := h.orgSvc.ListEmployees(ctx, actor.OrganizationID)
	if err != nil {
		h.log.ErrorContext(ctx, "list pharmacy employees",
			"error", err, "organization_id", actor.OrganizationID)
	}
	for _, emp := range employees {
		if emp == nil || emp.Member == nil {
			continue
		}
		roleID := int64(0)
		if emp.Member.OrgRoleID != nil {
			roleID = *emp.Member.OrgRoleID
		} else if id, ok := roleByKey[emp.Member.RoleKey]; ok {
			roleID = id
		}
		name := emp.UserName
		if name == "" {
			name = emp.UserEmail
		}
		view.Members = append(view.Members, pages.TenantTeamMember{
			ID:           emp.Member.ID,
			UserID:       emp.Member.UserID,
			Name:         name,
			Email:        emp.UserEmail,
			Phone:        emp.UserPhone,
			JobTitle:     emp.Member.JobTitle,
			EmployeeCode: emp.Member.EmployeeCode,
			BranchName:   emp.BranchName,
			RoleID:       roleID,
			RoleName:     roleNameByID[roleID],
			IsActive:     emp.Member.IsActive,
			JoinedAt:     emp.Member.CreatedAt.Format("2006-01-02"),
		})
	}

	if branches, err := h.orgSvc.ListBranches(ctx, actor.OrganizationID); err == nil {
		for _, b := range branches {
			name := b.Name.Get(i18n.AR)
			if name == "" {
				name = b.Name.Get(i18n.EN)
			}
			view.Branches = append(view.Branches, &pages.BranchOption{ID: b.ID, Name: name})
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.TenantTeamPage(view, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render pharmacy team page", "error", err)
	}
}
