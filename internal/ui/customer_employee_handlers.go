package ui

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/identity"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// CustomerEmployeeCreateSubmit creates a new employee user and binds them to the branch and role.
func (h *UIHandler) CustomerEmployeeCreateSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	orgID := actor.OrganizationID
	if orgID <= 0 {
		orgID = actor.OrgID
	}
	if !ok || orgID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/customer/branches?tab=employees", http.StatusSeeOther)
		return
	}

	if h.idSvc == nil || h.orgSvc == nil {
		h.redirectWithNotice(w, r, "/customer/branches?tab=employees", "error", "الخدمة غير متاحة حالياً.")
		return
	}

	_ = r.ParseForm()

	email := strings.TrimSpace(r.PostFormValue("email"))
	name := strings.TrimSpace(r.PostFormValue("name"))
	phone := strings.TrimSpace(r.PostFormValue("phone"))
	jobTitle := strings.TrimSpace(r.PostFormValue("job_title"))
	employeeCode := strings.TrimSpace(r.PostFormValue("employee_code"))
	roleKey := strings.TrimSpace(r.PostFormValue("role_key"))
	if roleKey == "" || roleKey == "org_admin" {
		if roleKey == "org_admin" {
			roleKey = "org_owner"
		} else {
			roleKey = "org_pharmacist"
		}
	}
	password := strings.TrimSpace(r.PostFormValue("password"))

	if email == "" || name == "" {
		h.redirectWithNotice(w, r, "/customer/branches?tab=employees", "error", "الاسم والبريد الإلكتروني حقول إلزامية.")
		return
	}

	var branchID *int64
	if bStr := r.PostFormValue("branch_id"); bStr != "" {
		if bID, err := strconv.ParseInt(bStr, 10, 64); err == nil && bID > 0 {
			branchID = &bID
		}
	}

	sysCtx := database.AsSystem(ctx)

	// 1. Locate or create user account
	var targetUserID int64
	existingUser, err := h.idSvc.GetUserByEmail(sysCtx, email)
	if err == nil && existingUser != nil {
		targetUserID = existingUser.ID
	} else {
		if password == "" {
			randBytes := make([]byte, 8)
			_, _ = rand.Read(randBytes)
			password = fmt.Sprintf("Dawa24!%s", hex.EncodeToString(randBytes))
		}

		newUser, _, err := h.idSvc.Register(sysCtx, identity.RegisterInput{
			Email:    email,
			Password: password,
			NameAr:   name,
			NameEn:   name,
			Phone:    phone,
			Role:     "user",
		})
		if err != nil {
			h.redirectWithNotice(w, r, "/customer/branches?tab=employees", "error", h.safeMessage(err, langOf(r)))
			return
		}
		targetUserID = newUser.ID
	}

	if employeeCode == "" {
		employeeCode = fmt.Sprintf("EMP-%d", targetUserID)
	}

	// 2. Add to organization members
	member := &org.Member{
		OrganizationID: orgID,
		UserID:         targetUserID,
		BranchID:       branchID,
		RoleKey:        roleKey,
		EmployeeCode:   employeeCode,
		JobTitle:       jobTitle,
		IsActive:       true,
	}

	if err := h.orgSvc.AddMemberDirect(sysCtx, member); err != nil {
		h.redirectWithNotice(w, r, "/customer/branches?tab=employees", "error", h.safeMessage(err, langOf(r)))
		return
	}

	// 3. If role is org_manager and branch is specified, assign as branch manager
	if roleKey == "org_manager" && branchID != nil {
		_ = h.orgSvc.AssignBranchManager(sysCtx, orgID, *branchID, &targetUserID)
	}

	h.redirectWithNotice(w, r, "/customer/branches?tab=employees", "success", "تم إضافة وتعيين الموظف بالفرع وتحديد صلاحياته بنجاح.")
}

// CustomerEmployeeEditSubmit updates an employee's branch assignment, role, job title, and details.
func (h *UIHandler) CustomerEmployeeEditSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	orgID := actor.OrganizationID
	if orgID <= 0 {
		orgID = actor.OrgID
	}
	if !ok || orgID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/customer/branches?tab=employees", http.StatusSeeOther)
		return
	}

	targetUserID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || targetUserID <= 0 {
		h.redirectWithNotice(w, r, "/customer/branches?tab=employees", "error", "معرف المستخدم غير صالح.")
		return
	}

	_ = r.ParseForm()

	jobTitle := strings.TrimSpace(r.PostFormValue("job_title"))
	employeeCode := strings.TrimSpace(r.PostFormValue("employee_code"))
	roleKey := strings.TrimSpace(r.PostFormValue("role_key"))
	if roleKey == "" || roleKey == "org_admin" {
		if roleKey == "org_admin" {
			roleKey = "org_owner"
		} else {
			roleKey = "org_pharmacist"
		}
	}
	isActive := r.PostFormValue("is_active") == "true" || r.PostFormValue("is_active") == "on" || r.PostFormValue("is_active") == "1"

	var branchID *int64
	if bStr := r.PostFormValue("branch_id"); bStr != "" {
		if bID, err := strconv.ParseInt(bStr, 10, 64); err == nil && bID > 0 {
			branchID = &bID
		}
	}

	sysCtx := database.AsSystem(ctx)
	member := &org.Member{
		OrganizationID: orgID,
		UserID:         targetUserID,
		BranchID:       branchID,
		RoleKey:        roleKey,
		EmployeeCode:   employeeCode,
		JobTitle:       jobTitle,
		IsActive:       isActive,
	}

	if err := h.orgSvc.AddMemberDirect(sysCtx, member); err != nil {
		h.redirectWithNotice(w, r, "/customer/branches?tab=employees", "error", h.safeMessage(err, langOf(r)))
		return
	}

	if roleKey == "org_manager" && branchID != nil {
		_ = h.orgSvc.AssignBranchManager(sysCtx, orgID, *branchID, &targetUserID)
	}

	h.redirectWithNotice(w, r, "/customer/branches?tab=employees", "success", "تم حفظ وتحديث بيانات وصلاحيات الموظف بنجاح.")
}

// CustomerEmployeeDeleteSubmit removes an employee member from the organization and branch.
func (h *UIHandler) CustomerEmployeeDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	orgID := actor.OrganizationID
	if orgID <= 0 {
		orgID = actor.OrgID
	}
	if !ok || orgID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/customer/branches?tab=employees", http.StatusSeeOther)
		return
	}

	targetUserID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || targetUserID <= 0 {
		h.redirectWithNotice(w, r, "/customer/branches?tab=employees", "error", "معرف الموظف غير صالح.")
		return
	}

	if targetUserID == actor.UserID {
		h.redirectWithNotice(w, r, "/customer/branches?tab=employees", "error", "لا يمكنك إزالة حسابك الحالي من المؤسسة.")
		return
	}

	sysCtx := database.AsSystem(ctx)
	if h.orgSvc != nil {
		if err := h.orgSvc.RemoveMember(sysCtx, orgID, targetUserID); err != nil {
			h.redirectWithNotice(w, r, "/customer/branches?tab=employees", "error", h.safeMessage(err, langOf(r)))
			return
		}
	}

	h.redirectWithNotice(w, r, "/customer/branches?tab=employees", "success", "تم حذف الموظف وإلغاء ربطه بالفرع بنجاح.")
}

// CustomerEmployeeStatusSubmit toggles active status for an employee.
func (h *UIHandler) CustomerEmployeeStatusSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	orgID := actor.OrganizationID
	if orgID <= 0 {
		orgID = actor.OrgID
	}
	if !ok || orgID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/customer/branches?tab=employees", http.StatusSeeOther)
		return
	}

	memberID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || memberID <= 0 {
		h.redirectWithNotice(w, r, "/customer/branches?tab=employees", "error", "معرف الموظف غير صالح.")
		return
	}

	sysCtx := database.AsSystem(ctx)
	if h.orgSvc != nil {
		if err := h.orgSvc.ToggleMemberStatus(sysCtx, orgID, memberID); err != nil {
			h.redirectWithNotice(w, r, "/customer/branches?tab=employees", "error", h.safeMessage(err, langOf(r)))
			return
		}
	}

	h.redirectWithNotice(w, r, "/customer/branches?tab=employees", "success", "تم تحديث حالة تفعيل الموظف بنجاح.")
}
