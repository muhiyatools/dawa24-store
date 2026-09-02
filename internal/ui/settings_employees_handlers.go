package ui

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/identity"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/shared/pagination"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// SettingsMemberRoleSubmit changes a member's organization role.
func (h *UIHandler) SettingsMemberRoleSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/settings/organization", http.StatusSeeOther)
		return
	}

	if h.orgSvc != nil {
		if userID, err := strconv.ParseInt(chi.URLParam(r, "userID"), 10, 64); err == nil {
			_ = h.orgSvc.UpdateMemberRole(ctx, actor.OrganizationID, userID, r.PostFormValue("role"))
		}
	}
	http.Redirect(w, r, "/settings/organization", http.StatusSeeOther)
}

// SettingsMemberAddSubmit adds an existing user to the organization by email.
func (h *UIHandler) SettingsMemberAddSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/settings/organization", http.StatusSeeOther)
		return
	}

	if h.idSvc == nil || h.orgSvc == nil {
		h.redirectWithNotice(w, r, "/settings/organization", "error", i18n.T(lang, "settings.emp.service_unavailable"))
		return
	}

	user, err := h.idSvc.GetUserByEmail(ctx, r.PostFormValue("email"))
	if err != nil {
		h.redirectWithNotice(w, r, "/settings/organization", "error", i18n.T(lang, "settings.emp.user_not_found"))
		return
	}

	if _, err := h.orgSvc.AddMemberByRoleKey(ctx, actor.OrganizationID, user.ID, r.PostFormValue("role")); err != nil {
		h.redirectWithNotice(w, r, "/settings/organization", "error", h.safeMessage(err, lang))
		return
	}
	h.redirectWithNotice(w, r, "/settings/organization", "success", i18n.T(lang, "settings.emp.member_added_success"))
}

// SettingsEmployeesPage renders the employee roster and branch manager assignments.
func (h *UIHandler) SettingsEmployeesPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/settings/employees", http.StatusSeeOther)
		return
	}

	page := pagination.PageNumber(r)
	limit := pagination.RowsPerPage(r)
	offset := (page - 1) * limit

	var employees []*org.EmployeeView
	var branches []*org.Branch
	var roles []*org.Role
	var totalCount int

	if h.orgSvc != nil && actor.OrganizationID > 0 {
		if emps, total, err := h.orgSvc.ListEmployeesWithTotal(ctx, actor.OrganizationID, limit, offset); err == nil {
			employees = emps
			totalCount = total
		} else {
			h.log.ErrorContext(ctx, "failed to list employees", "error", err, "org_id", actor.OrganizationID)
		}
		if b, err := h.orgSvc.ListBranches(ctx, actor.OrganizationID); err == nil {
			branches = b
		} else {
			h.log.ErrorContext(ctx, "failed to list branches", "error", err, "org_id", actor.OrganizationID)
		}
		if rl, err := h.orgSvc.ListRoles(ctx, actor.OrganizationID); err == nil {
			roles = rl
		} else {
			h.log.ErrorContext(ctx, "failed to list roles", "error", err, "org_id", actor.OrganizationID)
		}
	}

	data := pages.SettingsEmployeesPageData{
		Employees:  employees,
		Branches:   branches,
		Roles:      roles,
		Actor:      actor,
		Page:       page,
		PerPage:    limit,
		TotalCount: totalCount,
	}

	h.renderPage(ctx, w, "render settings employees", pages.SettingsEmployees(data, lang, dir))
}

// SettingsEmployeeCreateSubmit creates a new employee account and assigns them to the organization and branch.
func (h *UIHandler) SettingsEmployeeCreateSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/settings/employees", http.StatusSeeOther)
		return
	}

	if h.idSvc == nil || h.orgSvc == nil {
		h.redirectWithNotice(w, r, "/settings/employees", "error", i18n.T(lang, "settings.emp.service_unavailable"))
		return
	}

	_ = r.ParseForm()

	email := r.PostFormValue("email")
	name := r.PostFormValue("name")
	phone := r.PostFormValue("phone")
	jobTitle := r.PostFormValue("job_title")
	employeeCode := r.PostFormValue("employee_code")
	roleKey := r.PostFormValue("role_key")
	if roleKey == "" {
		roleKey = "org_employee"
	}
	salaryStr := r.PostFormValue("base_salary")
	var baseSalary money.Amount
	if salaryStr != "" {
		baseSalary, _ = money.Parse(salaryStr)
	}

	var branchID *int64
	if bStr := r.PostFormValue("branch_id"); bStr != "" {
		if bID, err := strconv.ParseInt(bStr, 10, 64); err == nil && bID > 0 {
			branchID = &bID
		}
	}

	// 1. Locate or create user account
	var targetUserID int64
	existingUser, err := h.idSvc.GetUserByEmail(ctx, email)
	if err == nil && existingUser != nil {
		targetUserID = existingUser.ID
	} else {
		randBytes := make([]byte, 8)
		_, _ = rand.Read(randBytes)
		tempPassword := fmt.Sprintf("Dawa24!%s", hex.EncodeToString(randBytes))

		newUser, _, err := h.idSvc.Register(ctx, identity.RegisterInput{
			Email:    email,
			Password: tempPassword,
			NameAr:   name,
			NameEn:   name,
			Phone:    phone,
			Role:     "user",
		})
		if err != nil {
			h.redirectWithNotice(w, r, "/settings/employees", "error", h.safeMessage(err, lang))
			return
		}
		targetUserID = newUser.ID
	}

	if employeeCode == "" {
		employeeCode = fmt.Sprintf("EMP-%d", targetUserID)
	}

	// 2. Add to organization members
	member := &org.Member{
		OrganizationID: actor.OrganizationID,
		UserID:         targetUserID,
		BranchID:       branchID,
		RoleKey:        roleKey,
		EmployeeCode:   employeeCode,
		JobTitle:       jobTitle,
		BaseSalary:     baseSalary,
		IsActive:       true,
	}

	if err := h.orgSvc.AddMemberDirect(ctx, member); err != nil {
		h.redirectWithNotice(w, r, "/settings/employees", "error", h.safeMessage(err, lang))
		return
	}

	// 3. If role is org_manager and branch is specified, assign as branch manager
	if roleKey == "org_manager" && branchID != nil {
		_ = h.orgSvc.AssignBranchManager(ctx, actor.OrganizationID, *branchID, &targetUserID)
	}

	h.redirectWithNotice(w, r, "/settings/employees", "success", i18n.T(lang, "settings.emp.created_success"))
}

// SettingsBranchManagerAssignSubmit assigns a designated employee user as the branch manager.
func (h *UIHandler) SettingsBranchManagerAssignSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/settings/employees", http.StatusSeeOther)
		return
	}

	_ = r.ParseForm()

	branchID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if branchID <= 0 {
		branchID, _ = strconv.ParseInt(r.PostFormValue("branch_id"), 10, 64)
	}
	if branchID <= 0 {
		h.redirectWithNotice(w, r, "/settings/employees", "error", i18n.T(lang, "settings.emp.invalid_branch_id"))
		return
	}

	var managerUserID *int64
	if mStr := r.PostFormValue("manager_user_id"); mStr != "" && mStr != "0" {
		if mID, err := strconv.ParseInt(mStr, 10, 64); err == nil && mID > 0 {
			managerUserID = &mID
		}
	}

	if err := h.orgSvc.AssignBranchManager(ctx, actor.OrganizationID, branchID, managerUserID); err != nil {
		h.redirectWithNotice(w, r, "/settings/employees", "error", h.safeMessage(err, lang))
		return
	}

	h.redirectWithNotice(w, r, "/settings/employees", "success", i18n.T(lang, "settings.emp.manager_assigned_success"))
}

// SettingsEmployeeDeleteSubmit removes an employee member from the organization.
func (h *UIHandler) SettingsEmployeeDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/settings/employees", http.StatusSeeOther)
		return
	}

	userID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || userID <= 0 {
		h.redirectWithNotice(w, r, "/settings/employees", "error", i18n.T(lang, "settings.emp.invalid_employee_id"))
		return
	}

	if err := h.orgSvc.RemoveMember(ctx, actor.OrganizationID, userID); err != nil {
		h.redirectWithNotice(w, r, "/settings/employees", "error", h.safeMessage(err, lang))
		return
	}

	h.redirectWithNotice(w, r, "/settings/employees", "success", i18n.T(lang, "settings.emp.deleted_success"))
}
