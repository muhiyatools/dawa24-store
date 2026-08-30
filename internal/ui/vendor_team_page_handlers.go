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
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
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

	var memberViews []*pages.TeamMemberView
	var branchOptions []*pages.BranchOption

	if h.orgSvc != nil && actor.OrganizationID > 0 {
		// 1. Fetch employees with full profiles
		if employees, err := h.orgSvc.ListEmployees(ctx, actor.OrganizationID); err == nil && len(employees) > 0 {
			for _, emp := range employees {
				roleName := emp.RoleName
				switch emp.Member.RoleKey {
				case "org_owner":
					roleName = "مالك المنشأة"
				case "org_manager":
					roleName = "مدير عمليات"
				case "org_warehouse":
					roleName = "أمين مخزن"
				case "org_accountant":
					roleName = "محاسب مالي"
				case "org_employee":
					roleName = "موظف مبيعات وتوريد"
				default:
					if roleName == "" {
						roleName = "عضو فريق العمل"
					}
				}
				if emp.IsManager && emp.Member.RoleKey != "org_owner" {
					roleName = "مدير فرع / عمليات"
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
				name := "موظف"
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
				roleName := "موظف مبيعات وتوريد"
				switch m.RoleKey {
				case "org_owner":
					roleName = "مالك المنشأة"
				case "org_manager":
					roleName = "مدير عمليات"
				case "org_warehouse":
					roleName = "أمين مخزن"
				case "org_accountant":
					roleName = "محاسب مالي"
				case "org_employee":
					roleName = "موظف مبيعات وتوريد"
				}
				memberViews = append(memberViews, &pages.TeamMemberView{
					ID:           m.ID,
					UserID:       m.UserID,
					Name:         name,
					Email:        email,
					Phone:        phone,
					JobTitle:     m.JobTitle,
					EmployeeCode: m.EmployeeCode,
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
		h.redirectWithNotice(w, r, "/vendor/team", "error", "بيانات النموذج غير صالحة.")
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
		h.redirectWithNotice(w, r, "/vendor/team", "error", "يرجى إدخال اسم الموظف بالكامل.")
		return
	}
	if email == "" || !strings.Contains(email, "@") {
		h.redirectWithNotice(w, r, "/vendor/team", "error", "يرجى إدخال بريد إلكتروني صحيح للدخول.")
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
		h.redirectWithNotice(w, r, "/vendor/team", "error", "خدمة المنظومة غير متاحة حالياً.")
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
			h.redirectWithNotice(w, r, "/vendor/team", "error", "فشل في إنشاء حساب الموظف: "+h.safeMessage(regErr, langOf(r)))
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
		h.redirectWithNotice(w, r, "/vendor/team", "error", "فشل في ربط الموظف بالمنشأة: "+err.Error())
		return
	}

	h.redirectWithNotice(w, r, "/vendor/team", "success", fmt.Sprintf("تمت إضافة الموظف '%s' بنجاح وتفعيل صلاحياته على المنشأة.", name))
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
		h.redirectWithNotice(w, r, "/vendor/team", "error", "معرف الموظف غير صالح.")
		return
	}
	if h.orgSvc == nil {
		h.redirectWithNotice(w, r, "/vendor/team", "error", "خدمة المؤسسات غير متوفرة.")
		return
	}
	if err := h.orgSvc.ToggleMemberStatus(ctx, actor.OrganizationID, id); err != nil {
		h.log.ErrorContext(ctx, "toggle member status", "error", err, "member", id, "org", actor.OrganizationID)
		h.redirectWithNotice(w, r, "/vendor/team", "error", h.safeMessage(err, langOf(r)))
		return
	}
	h.redirectWithNotice(w, r, "/vendor/team", "success", "تم تحديث حالة حساب الموظف بنجاح.")
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
		h.redirectWithNotice(w, r, "/vendor/team", "error", "معرف الموظف غير صالح.")
		return
	}
	if h.orgSvc == nil {
		h.redirectWithNotice(w, r, "/vendor/team", "error", "خدمة المؤسسات غير متوفرة.")
		return
	}
	if err := h.orgSvc.RemoveMember(ctx, actor.OrganizationID, id); err != nil {
		h.log.ErrorContext(ctx, "remove member", "error", err, "member", id, "org", actor.OrganizationID)
		h.redirectWithNotice(w, r, "/vendor/team", "error", h.safeMessage(err, langOf(r)))
		return
	}
	h.redirectWithNotice(w, r, "/vendor/team", "success", "تم حذف الموظف من المنشأة بنجاح.")
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
