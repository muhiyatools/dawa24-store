package ui

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/identity"

	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/pagination"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// VendorTeamPage renders the staff and RBAC roles configuration view.
func (h *UIHandler) VendorTeamPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/team", http.StatusSeeOther)
		return
	}

	noticeType := r.URL.Query().Get("notice_type")
	if noticeType == "" {
		noticeType = r.URL.Query().Get("notice")
	}
	noticeMsg := r.URL.Query().Get("notice_msg")
	if noticeMsg == "" {
		noticeMsg = r.URL.Query().Get("msg")
	}

	page := pagination.PageNumber(r)
	limit := pagination.RowsPerPage(r)
	offset := (page - 1) * limit

	var memberViews []*pages.TeamMemberView
	var branchOptions []*pages.BranchOption
	var totalCount int

	if h.orgSvc != nil && actor.OrganizationID > 0 {
		// 1. Fetch employees with full profiles
		if employees, total, err := h.orgSvc.ListEmployeesWithTotal(ctx, actor.OrganizationID, limit, offset); err == nil && len(employees) > 0 {
			totalCount = total
			for _, emp := range employees {
				roleName := emp.RoleName
				switch emp.Member.RoleKey {
				case "org_owner":
					roleName = i18n.T(lang, "role.org_owner")
				case "org_manager":
					roleName = i18n.T(lang, "role.org_manager")
				case "org_warehouse":
					roleName = i18n.T(lang, "role.org_warehouse")
				case "org_accountant":
					roleName = i18n.T(lang, "role.org_accountant")
				case "org_employee":
					roleName = i18n.T(lang, "role.org_employee")
				default:
					if roleName == "" {
						roleName = i18n.T(lang, "role.team_member")
					}
				}
				if emp.IsManager && emp.Member.RoleKey != "org_owner" {
					roleName = i18n.T(lang, "role.branch_manager")
				}

				name := emp.UserName
				if name == "" {
					name = emp.UserEmail
				}

				memberViews = append(memberViews, &pages.TeamMemberView{
					ID:           emp.Member.ID,
					UserID:       emp.Member.UserID,
					Name:         name,
					Email:        emp.UserEmail,
					Phone:        emp.UserPhone,
					JobTitle:     emp.Member.JobTitle,
					EmployeeCode: emp.Member.EmployeeCode,
					BranchID:     derefBranchID(emp.Member.BranchID),
					BranchName:   emp.BranchName,
					RoleKey:      emp.Member.RoleKey,
					RoleName:     roleName,
					RoleID:       derefRoleID(emp.Member.OrgRoleID),
					IsActive:     emp.Member.IsActive,
					CreatedAt:    emp.Member.CreatedAt.Format("2006-01-02"),
				})
			}
		} else {
			// Fallback to ListMembers if ListEmployees returns empty
			members, _ := h.orgSvc.ListMembers(ctx, actor.OrganizationID)
			for _, m := range members {
				name := i18n.T(lang, "role.employee")
				email := ""
				phone := ""
				if h.idSvc != nil {
					if u, err := h.idSvc.GetUserByID(ctx, m.UserID); err == nil && u != nil {
						name = u.Name.Get(i18n.AR)
						if name == "" {
							name = u.Name.Get(i18n.EN)
						}
						if name == "" {
							name = u.Email
						}
						email = u.Email
						phone = u.Phone
					}
				}
				roleName := i18n.T(lang, "role.org_employee")
				switch m.RoleKey {
				case "org_owner":
					roleName = i18n.T(lang, "role.org_owner")
				case "org_manager":
					roleName = i18n.T(lang, "role.org_manager")
				case "org_warehouse":
					roleName = i18n.T(lang, "role.org_warehouse")
				case "org_accountant":
					roleName = i18n.T(lang, "role.org_accountant")
				case "org_employee":
					roleName = i18n.T(lang, "role.org_employee")
				}
				memberViews = append(memberViews, &pages.TeamMemberView{
					ID:           m.ID,
					UserID:       m.UserID,
					Name:         name,
					Email:        email,
					Phone:        phone,
					JobTitle:     m.JobTitle,
					EmployeeCode: m.EmployeeCode,
					BranchID:     derefBranchID(m.BranchID),
					RoleKey:      m.RoleKey,
					RoleName:     roleName,
					RoleID:       derefRoleID(m.OrgRoleID),
					IsActive:     m.IsActive,
					CreatedAt:    m.CreatedAt.Format("2006-01-02"),
				})
			}
		}

		// 2. Fetch branches for the branch dropdown
		if branches, err := h.orgSvc.ListBranches(ctx, actor.OrganizationID); err == nil {
			for _, b := range branches {
				bName := b.Name.Get(i18n.AR)
				if bName == "" {
					bName = b.Name.Get(i18n.EN)
				}
				branchOptions = append(branchOptions, &pages.BranchOption{
					ID:   b.ID,
					Name: bName,
				})
			}
		}
	}

	data := pages.VendorTeamData{
		NoticeType:    noticeType,
		NoticeMsg:     noticeMsg,
		Members:       memberViews,
		Branches:      branchOptions,
		CanAssignRole: actor.Can("vendor.role.assign"),
		CurrentUserID: actor.UserID,
		Page:          page,
		PerPage:       limit,
		TotalCount:    totalCount,
	}
	// The assignable roles are this company's own. Resolving each member's
	// current role through the same list is what makes the selector show
	// where they stand rather than defaulting everyone to the first option.
	if h.orgSvc != nil && actor.OrganizationID > 0 {
		h.ensureCompanyRoles(ctx, actor.OrganizationID, actor.OrgType)
		roles, err := h.orgSvc.ListRoles(ctx, actor.OrganizationID)
		if err != nil {
			h.log.ErrorContext(ctx, "list company roles for team page",
				"error", err, "organization_id", actor.OrganizationID)
		}
		roleIDByKey := map[string]int64{}
		for _, role := range roles {
			data.CompanyRoles = append(data.CompanyRoles, pages.TenantRoleOption{
				ID:   role.ID,
				Name: role.Name.Get(i18n.ParseLang(lang)),
			})
			roleIDByKey[role.Key] = role.ID
		}
		for _, m := range memberViews {
			if m.RoleID == 0 {
				m.RoleID = roleIDByKey[m.RoleKey]
			}
		}
	}

	h.renderPage(ctx, w, "render vendor team page", pages.VendorTeamPage(data, lang, dir))
}

// VendorTeamNewSubmit registers an employee and links them to the vendor org.
func (h *UIHandler) VendorTeamNewSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/team", http.StatusSeeOther)
		return
	}

	if err := r.ParseForm(); err != nil {
		h.redirectWithNotice(w, r, "/vendor/team", "error", i18n.T(langOf(r), "common.invalid_form_data"))
		return
	}

	name := strings.TrimSpace(r.PostFormValue("name"))
	email := strings.ToLower(strings.TrimSpace(r.PostFormValue("email")))
	phone := strings.TrimSpace(r.PostFormValue("phone"))
	password := strings.TrimSpace(r.PostFormValue("password"))
	roleKey := strings.TrimSpace(r.PostFormValue("role_key"))
	jobTitle := strings.TrimSpace(r.PostFormValue("job_title"))
	employeeCode := strings.TrimSpace(r.PostFormValue("employee_code"))

	if name == "" {
		h.redirectWithNotice(w, r, "/vendor/team", "error", i18n.T(langOf(r), "vendor.team.enter_full_name"))
		return
	}
	if email == "" || !strings.Contains(email, "@") {
		h.redirectWithNotice(w, r, "/vendor/team", "error", i18n.T(langOf(r), "vendor.team.enter_valid_email"))
		return
	}
	if roleKey == "" {
		roleKey = "org_employee"
	}
	if password == "" || len(password) < 6 {
		password = "Password123!"
	}

	var branchID *int64
	if bStr := strings.TrimSpace(r.PostFormValue("branch_id")); bStr != "" {
		if bID, err := strconv.ParseInt(bStr, 10, 64); err == nil && bID > 0 {
			branchID = &bID
		}
	}

	if h.idSvc == nil || h.orgSvc == nil {
		h.redirectWithNotice(w, r, "/vendor/team", "error", i18n.T(langOf(r), "common.service_unavailable"))
		return
	}

	// 1. Locate existing user account or create a new one
	var targetUserID int64
	existingUser, err := h.idSvc.GetUserByEmail(ctx, email)
	if err == nil && existingUser != nil {
		targetUserID = existingUser.ID
	} else {
		newUser, _, regErr := h.idSvc.Register(ctx, identity.RegisterInput{
			Email:    email,
			Password: password,
			NameAr:   name,
			NameEn:   name,
			Phone:    phone,
			Role:     "user",
		})
		if regErr != nil {
			h.log.ErrorContext(ctx, "failed to register employee user", "email", email, "error", regErr)
			h.redirectWithNotice(w, r, "/vendor/team", "error", i18n.T(langOf(r), "vendor.team.register_failed_prefix")+h.safeMessage(regErr, langOf(r)))
			return
		}
		targetUserID = newUser.ID
	}

	if employeeCode == "" {
		employeeCode = fmt.Sprintf("EMP-%d", targetUserID)
	}

	// 2. Link member to vendor organization with all specified attributes
	member := &org.Member{
		OrganizationID: actor.OrganizationID,
		UserID:         targetUserID,
		BranchID:       branchID,
		RoleKey:        roleKey,
		JobTitle:       jobTitle,
		EmployeeCode:   employeeCode,
		IsActive:       true,
	}

	if err := h.orgSvc.AddMemberDirect(ctx, member); err != nil {
		h.log.ErrorContext(ctx, "failed to add org member", "error", err, "org_id", actor.OrganizationID, "user_id", targetUserID)
		h.redirectWithNotice(w, r, "/vendor/team", "error", i18n.T(langOf(r), "vendor.team.link_failed_prefix")+err.Error())
		return
	}

	h.redirectWithNotice(w, r, "/vendor/team", "success", fmt.Sprintf(i18n.T(langOf(r), "vendor.team.employee_added_success"), name))
}

// VendorTeamEditSubmit updates an existing employee's profile (name, phone) and
// membership (job title, employee code, role, branch, active flag). The vendor
// team screen addresses employees by membership id, so this resolves the
// underlying user through GetMemberByID before touching the identity record.
func (h *UIHandler) VendorTeamEditSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/team", http.StatusSeeOther)
		return
	}

	memberID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || memberID <= 0 {
		h.redirectWithNotice(w, r, "/vendor/team", "error", i18n.T(langOf(r), "vendor.team.invalid_employee_id"))
		return
	}

	if h.orgSvc == nil {
		h.redirectWithNotice(w, r, "/vendor/team", "error", i18n.T(langOf(r), "common.org_service_unavailable"))
		return
	}

	member, err := h.orgSvc.GetMemberByID(ctx, actor.OrganizationID, memberID)
	if err != nil || member == nil || member.UserID <= 0 {
		h.redirectWithNotice(w, r, "/vendor/team", "error", i18n.T(langOf(r), "vendor.team.invalid_employee_id"))
		return
	}
	if member.RoleKey == "org_owner" {
		h.redirectWithNotice(w, r, "/vendor/team", "error", i18n.T(langOf(r), "vendor.team.cannot_edit_owner"))
		return
	}

	if err := r.ParseForm(); err != nil {
		h.redirectWithNotice(w, r, "/vendor/team", "error", i18n.T(langOf(r), "common.invalid_form_data"))
		return
	}

	name := strings.TrimSpace(r.PostFormValue("name"))
	phone := strings.TrimSpace(r.PostFormValue("phone"))
	jobTitle := strings.TrimSpace(r.PostFormValue("job_title"))
	employeeCode := strings.TrimSpace(r.PostFormValue("employee_code"))
	roleKey := strings.TrimSpace(r.PostFormValue("role_key"))
	if roleKey == "" {
		roleKey = member.RoleKey
	}
	if roleKey == "" {
		roleKey = "org_employee"
	}
	isActive := r.PostFormValue("is_active") == "true" || r.PostFormValue("is_active") == "on" || r.PostFormValue("is_active") == "1"

	var branchID *int64
	if bStr := strings.TrimSpace(r.PostFormValue("branch_id")); bStr != "" {
		if bID, e := strconv.ParseInt(bStr, 10, 64); e == nil && bID > 0 {
			branchID = &bID
		}
	}

	sysCtx := database.AsSystem(ctx)

	// 1. Profile fields on the identity user. UpdateProfile ignores empty
	//    values, so a blank field leaves the stored one untouched.
	if h.idSvc != nil && (name != "" || phone != "") {
		if _, e := h.idSvc.UpdateProfile(sysCtx, member.UserID, name, name, phone, "", ""); e != nil {
			h.log.WarnContext(ctx, "vendor employee edit: profile update failed", "user_id", member.UserID, "error", e)
		}
	}

	// 2. Membership fields (AddMember upserts on organization_id + user_id).
	updated := &org.Member{
		OrganizationID: actor.OrganizationID,
		UserID:         member.UserID,
		BranchID:       branchID,
		RoleKey:        roleKey,
		OrgRoleID:      member.OrgRoleID,
		RoleID:         member.RoleID,
		JobTitle:       jobTitle,
		EmployeeCode:   employeeCode,
		IsActive:       isActive,
	}
	if err := h.orgSvc.AddMemberDirect(sysCtx, updated); err != nil {
		h.log.ErrorContext(ctx, "vendor employee edit: member update failed", "member_id", memberID, "error", err)
		h.redirectWithNotice(w, r, "/vendor/team", "error", h.safeMessage(err, langOf(r)))
		return
	}

	if roleKey == "org_manager" && branchID != nil {
		_ = h.orgSvc.AssignBranchManager(sysCtx, actor.OrganizationID, *branchID, &member.UserID)
	}

	h.redirectWithNotice(w, r, "/vendor/team", "success", i18n.T(langOf(r), "vendor.team.employee_updated_success"))
}

// VendorTeamToggleSubmit toggles a member's active status.
func (h *UIHandler) VendorTeamToggleSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/vendor/team", "error", i18n.T(langOf(r), "vendor.team.invalid_employee_id"))
		return
	}
	if h.orgSvc == nil {
		h.redirectWithNotice(w, r, "/vendor/team", "error", i18n.T(langOf(r), "common.org_service_unavailable"))
		return
	}
	member, err := h.orgSvc.GetMemberByID(ctx, actor.OrganizationID, id)
	if err != nil || member == nil {
		h.redirectWithNotice(w, r, "/vendor/team", "error", i18n.T(langOf(r), "vendor.team.invalid_employee_id"))
		return
	}
	if member.UserID == actor.UserID {
		h.redirectWithNotice(w, r, "/vendor/team", "error", "لا يمكنك تغيير حالة تفعيل حسابك الخاص")
		return
	}
	if member.RoleKey == "org_owner" {
		h.redirectWithNotice(w, r, "/vendor/team", "error", i18n.T(langOf(r), "vendor.team.cannot_edit_owner"))
		return
	}
	if err := h.orgSvc.ToggleMemberStatus(ctx, actor.OrganizationID, id); err != nil {
		h.log.ErrorContext(ctx, "toggle member status", "error", err, "member", id, "org", actor.OrganizationID)
		h.redirectWithNotice(w, r, "/vendor/team", "error", h.safeMessage(err, langOf(r)))
		return
	}
	h.redirectWithNotice(w, r, "/vendor/team", "success", i18n.T(langOf(r), "vendor.team.status_updated_success"))
}

// VendorTeamDeleteSubmit removes an employee member from the organization.
func (h *UIHandler) VendorTeamDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/vendor/team", "error", i18n.T(langOf(r), "vendor.team.invalid_employee_id"))
		return
	}
	if h.orgSvc == nil {
		h.redirectWithNotice(w, r, "/vendor/team", "error", i18n.T(langOf(r), "common.org_service_unavailable"))
		return
	}
	// The row is addressed by membership id; RemoveMember deletes by user id.
	member, err := h.orgSvc.GetMemberByID(ctx, actor.OrganizationID, id)
	if err != nil || member == nil || member.UserID <= 0 {
		h.redirectWithNotice(w, r, "/vendor/team", "error", i18n.T(langOf(r), "vendor.team.invalid_employee_id"))
		return
	}
	if member.UserID == actor.UserID {
		h.redirectWithNotice(w, r, "/vendor/team", "error", "لا يمكنك حذف حسابك الخاص من المنشأة")
		return
	}
	if member.RoleKey == "org_owner" {
		h.redirectWithNotice(w, r, "/vendor/team", "error", i18n.T(langOf(r), "vendor.team.cannot_edit_owner"))
		return
	}
	if err := h.orgSvc.RemoveMember(ctx, actor.OrganizationID, member.UserID); err != nil {
		h.log.ErrorContext(ctx, "remove member", "error", err, "member", id, "org", actor.OrganizationID)
		h.redirectWithNotice(w, r, "/vendor/team", "error", h.safeMessage(err, langOf(r)))
		return
	}
	h.redirectWithNotice(w, r, "/vendor/team", "success", i18n.T(langOf(r), "vendor.team.deleted_success"))
}

// derefRoleID unwraps the optional custom-role link on a membership. Zero
// means "no custom role assigned", and the caller falls back to the company's
// starter role for the member's role_key.
func derefRoleID(id *int64) int64 {
	if id == nil {
		return 0
	}
	return *id
}

// derefBranchID unwraps a member's optional branch assignment; zero means the
// employee is not tied to a specific branch.
func derefBranchID(id *int64) int64 {
	if id == nil {
		return 0
	}
	return *id
}
