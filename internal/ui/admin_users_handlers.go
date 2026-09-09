package ui

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/identity"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"

	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/pagination"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// AdminUsersPage renders the dedicated user management directory.
func (h *UIHandler) AdminUsersPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sysCtx := database.AsSystem(ctx)
	lang, dir := h.localeAndDir(r)

	searchQuery := strings.TrimSpace(r.URL.Query().Get("q"))
	roleFilter := strings.TrimSpace(r.URL.Query().Get("role"))
	typeFilter := strings.TrimSpace(r.URL.Query().Get("type"))
	statusFilter := strings.TrimSpace(r.URL.Query().Get("status"))
	mfaFilter := strings.TrimSpace(r.URL.Query().Get("mfa"))
	loginFromFilter := strings.TrimSpace(r.URL.Query().Get("login_from"))
	loginToFilter := strings.TrimSpace(r.URL.Query().Get("login_to"))
	orgIDFilter, _ := strconv.ParseInt(r.URL.Query().Get("org_id"), 10, 64)

	var mfaBool *bool
	if mfaFilter == "1" || mfaFilter == "enabled" || mfaFilter == "true" {
		t := true
		mfaBool = &t
	} else if mfaFilter == "0" || mfaFilter == "disabled" || mfaFilter == "false" {
		f := false
		mfaBool = &f
	}

	var loginFromTime, loginToTime *time.Time
	if loginFromFilter != "" {
		if t, err := time.Parse("2006-01-02", loginFromFilter); err == nil {
			loginFromTime = &t
		}
	}
	if loginToFilter != "" {
		if t, err := time.Parse("2006-01-02", loginToFilter); err == nil {
			endDay := t.Add(23*time.Hour + 59*time.Minute + 59*time.Second)
			loginToTime = &endDay
		}
	}

	page := pagination.PageNumber(r)
	limit := pagination.RowsPerPage(r)
	offset := (page - 1) * limit

	var users []*identity.User
	var totalCount int
	var allOrgs []*org.Organization
	orgNames := make(map[int64]string)
	var allBranches []*org.Branch
	var deletionRequests []*identity.AccountDeletionRequest
	var totalUsers, customerUsers, vendorUsers, staffUsers, activeUsers, suspendedUsers int

	if h.orgSvc != nil {
		if orgsList, err := h.orgSvc.ListOrganizations(sysCtx, nil, nil, 500, 0); err == nil {
			allOrgs = orgsList
			for _, o := range orgsList {
				if o != nil {
					orgNames[o.ID] = o.LegalName
				}
			}
		}
		if branchesList, _, err := h.orgSvc.ListBranchesWithTotal(sysCtx, org.BranchFilter{Status: "active"}, 1000, 0); err == nil {
			allBranches = branchesList
		}
	}

	userNationalIDs := make(map[int64]string)
	userMemberships := make(map[int64]*identity.UserOrgMembership)

	if h.idSvc != nil {
		if stats, err := h.idSvc.AdminUserStats(sysCtx); err == nil {
			totalUsers = stats.Total
			staffUsers = stats.Staff
			vendorUsers = stats.Vendor
			customerUsers = stats.Customer
			activeUsers = stats.Active
			suspendedUsers = stats.Suspended
		}

		filter := identity.AdminUserFilter{
			Role:          roleFilter,
			Type:          typeFilter,
			Status:        statusFilter,
			Search:        searchQuery,
			OrgID:         orgIDFilter,
			MFA:           mfaBool,
			LastLoginFrom: loginFromTime,
			LastLoginTo:   loginToTime,
		}
		uList, tot, err := h.idSvc.AdminListUsersWithTotal(sysCtx, filter, limit, offset)
		if err == nil {
			users = uList
			totalCount = tot
		}
		deletionRequests, _ = h.idSvc.AdminListDeletionRequests(sysCtx, "")

		for _, u := range users {
			if u == nil {
				continue
			}
			if nid, err := h.idSvc.GetNationalID(sysCtx, u.ID); err == nil && nid != "" {
				userNationalIDs[u.ID] = nid
			}
			if orgs, err := h.idSvc.ListUserOrganizations(sysCtx, u.ID); err == nil && len(orgs) > 0 {
				userMemberships[u.ID] = orgs[0]
			}
		}
	}

	data := pages.AdminUsersPageData{
		Users:            users,
		Organizations:    allOrgs,
		OrgNames:         orgNames,
		AllBranches:      allBranches,
		UserNationalIDs:  userNationalIDs,
		UserMemberships:  userMemberships,
		DeletionRequests: deletionRequests,
		TotalUsers:       totalUsers,
		CustomerUsers:    customerUsers,
		VendorUsers:      vendorUsers,
		StaffUsers:       staffUsers,
		ActiveUsers:      activeUsers,
		SuspendedUsers:   suspendedUsers,
		Page:             page,
		PerPage:          limit,
		TotalCount:       totalCount,
		SearchQuery:      searchQuery,
		RoleFilter:       roleFilter,
		TypeFilter:       typeFilter,
		StatusFilter:     statusFilter,
		OrgFilter:        orgIDFilter,
		MFAFilter:        mfaFilter,
		LoginFromFilter:  loginFromFilter,
		LoginToFilter:    loginToFilter,
		Notice:           r.URL.Query().Get("notice"),
		NoticeKind:       r.URL.Query().Get("kind"),
	}

	// The create-administrator form appears only for a viewer who may assign
	// roles. Only staff roles are offered: creating an account into a
	// non-staff role produces someone who cannot open the dashboard they were
	// hired for, and the service refuses it anyway.
	if actor := authctx.FromContext(ctx); actor.Can("identity.admin_role.assign") && h.idSvc != nil {
		roles, err := h.idSvc.ListPlatformRoles(ctx)
		if err != nil {
			h.log.ErrorContext(ctx, "list platform roles for the users page", "error", err)
		}
		for _, role := range roles {
			if !role.IsStaff {
				continue
			}
			data.StaffRoles = append(data.StaffRoles, pages.AdminUserRoleOption{
				Key:     role.Key,
				Name:    role.Name.Get(i18n.ParseLang(lang)),
				IsStaff: true,
			})
		}

		// The main moderators a new sub-moderator can be placed under. Only
		// top-level ones are offered: the hierarchy is one level deep, so a
		// moderator who already reports to somebody cannot be a parent, and
		// SetModeratorParent refuses it anyway.
		if mods, err := h.idSvc.ListModerators(sysCtx); err == nil {
			for _, m := range mods {
				if m == nil || !m.IsMain() {
					continue
				}
				data.MainModerators = append(data.MainModerators, pages.AdminModeratorOption{
					UserID: m.UserID,
					Name:   m.Name.Get(i18n.ParseLang(lang)),
					Team:   m.SubordinateCount,
				})
			}
		} else {
			h.log.ErrorContext(ctx, "list moderators for the users page", "error", err)
		}
	}

	h.renderPage(ctx, w, "render admin users page", pages.AdminUsersPage(data, lang, dir))
}

// Administrative actions on user accounts.
//
// The admin users screen already had buttons for these, but they posted to
// /api/v1/identity/admin/users/... while the routes are registered at
// /api/v1/admin/identity/users/... - the two path segments are the other way
// round, so every button returned 404. They also carried hx-swap="none", so
// nothing on the page changed either way and the operator had no way to tell
// a successful suspension from a failed one.
//
// These handlers do the work through the service and return the refreshed
// table, so the row reflects the new state immediately.

func (h *UIHandler) adminUserAction(
	w http.ResponseWriter,
	r *http.Request,
	action func(ctx context.Context, userID, actorID int64) error,
) {
	ctx := r.Context()

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/admin/users", http.StatusSeeOther)
		return
	}
	if h.idSvc == nil {
		h.renderError(w, r, apperr.Unavailable("identity", nil))
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		h.renderError(w, r, apperr.Validation("id.invalid", "Invalid user ID", nil))
		return
	}

	// An administrator suspending their own account would lock themselves out
	// of the screen they are standing on, and revoking their own sessions ends
	// the request that did it.
	if id == actor.UserID {
		h.renderError(w, r, apperr.Validation("user.self_action",
			"You cannot apply this action to your own account.", nil))
		return
	}

	redirectTarget := r.PostFormValue("redirect")
	if err := action(ctx, id, actor.UserID); err != nil {
		h.redirectAdminUsers(w, r, redirectTarget, "error", h.safeMessage(err, langOf(r)))
		return
	}

	h.redirectAdminUsers(w, r, redirectTarget, "success", i18n.T(langOf(r), "admin.users.action_success"))
}

func (h *UIHandler) redirectAdminUsers(w http.ResponseWriter, r *http.Request, targetURL, kind, notice string) {
	if targetURL == "" {
		targetURL = r.PostFormValue("redirect")
	}
	if targetURL == "" {
		targetURL = r.URL.Query().Get("redirect")
	}
	if targetURL == "" {
		targetURL = r.Header.Get("Referer")
	}
	if targetURL == "" || !strings.HasPrefix(targetURL, "/admin/users") {
		targetURL = "/admin/users"
	}

	u, err := url.Parse(targetURL)
	if err != nil {
		u = &url.URL{Path: "/admin/users"}
	}
	q := u.Query()
	if notice != "" {
		q.Set("notice", notice)
		q.Set("kind", kind)
	}
	u.RawQuery = q.Encode()

	http.Redirect(w, r, u.String(), http.StatusSeeOther)
}

// AdminUserSuspendSubmit blocks an account and ends its sessions.
func (h *UIHandler) AdminUserSuspendSubmit(w http.ResponseWriter, r *http.Request) {
	h.adminUserAction(w, r, func(ctx context.Context, userID, actorID int64) error {
		return h.idSvc.AdminSuspendUser(ctx, userID, actorID)
	})
}

// AdminUserReactivateSubmit restores a suspended account.
func (h *UIHandler) AdminUserReactivateSubmit(w http.ResponseWriter, r *http.Request) {
	h.adminUserAction(w, r, func(ctx context.Context, userID, actorID int64) error {
		return h.idSvc.AdminReactivateUser(ctx, userID, actorID)
	})
}

// AdminUserResetMFASubmit clears a user's second factor.
func (h *UIHandler) AdminUserResetMFASubmit(w http.ResponseWriter, r *http.Request) {
	h.adminUserAction(w, r, func(ctx context.Context, userID, actorID int64) error {
		return h.idSvc.AdminResetMFA(ctx, userID, actorID)
	})
}

// AdminUserEditSubmit processes form submission to update user profile, national ID, and org membership.
func (h *UIHandler) AdminUserEditSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sysCtx := database.AsSystem(ctx)

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/admin/users", http.StatusSeeOther)
		return
	}
	if h.idSvc == nil {
		h.renderError(w, r, apperr.Unavailable("identity", nil))
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.renderError(w, r, apperr.Validation("id.invalid", "Invalid user ID", nil))
		return
	}

	nameAr := strings.TrimSpace(r.PostFormValue("name_ar"))
	nameEn := strings.TrimSpace(r.PostFormValue("name_en"))
	email := strings.TrimSpace(r.PostFormValue("email"))
	phone := strings.TrimSpace(r.PostFormValue("phone"))
	avatarURL := strings.TrimSpace(r.PostFormValue("avatar_url"))
	statusStr := strings.TrimSpace(r.PostFormValue("status"))
	nationalID := strings.TrimSpace(r.PostFormValue("national_id"))
	orgID, _ := strconv.ParseInt(r.PostFormValue("org_id"), 10, 64)
	branchID, _ := strconv.ParseInt(r.PostFormValue("branch_id"), 10, 64)
	jobTitle := strings.TrimSpace(r.PostFormValue("job_title"))
	redirectTarget := r.PostFormValue("redirect")

	if email == "" {
		h.redirectAdminUsers(w, r, redirectTarget, "error", i18n.T(langOf(r), "admin.settings.invalid_email"))
		return
	}
	if nameAr == "" && nameEn == "" {
		h.redirectAdminUsers(w, r, redirectTarget, "error", "يجب إدخال اسم المستخدم.")
		return
	}

	// 1. Update identity.users and KYC records
	editIn := identity.AdminEditUserInput{
		NameAr:     nameAr,
		NameEn:     nameEn,
		Email:      email,
		Phone:      phone,
		AvatarURL:  avatarURL,
		Status:     identity.UserStatus(statusStr),
		NationalID: nationalID,
	}

	if err := h.idSvc.AdminUpdateUserDetails(sysCtx, id, editIn, actor.UserID); err != nil {
		h.redirectAdminUsers(w, r, redirectTarget, "error", h.safeMessage(err, langOf(r)))
		return
	}

	// 2. Handle organization membership & branch assignment
	if h.orgSvc != nil && orgID > 0 {
		members, _ := h.orgSvc.ListMembers(sysCtx, orgID)
		var existingMember *org.Member
		for _, m := range members {
			if m != nil && m.UserID == id {
				existingMember = m
				break
			}
		}

		var branchPtr *int64
		if branchID > 0 {
			branchPtr = &branchID
		}

		if existingMember != nil {
			patch := org.MemberPatch{
				BranchID: branchPtr,
				JobTitle: &jobTitle,
			}
			if err := h.orgSvc.UpdateMember(sysCtx, orgID, existingMember.ID, patch); err != nil {
				h.log.ErrorContext(ctx, "update member branch and job title", "error", err)
			}
		} else {
			if newMem, err := h.orgSvc.AddMember(sysCtx, orgID, id, 0); err == nil && newMem != nil {
				patch := org.MemberPatch{
					BranchID: branchPtr,
					JobTitle: &jobTitle,
				}
				_ = h.orgSvc.UpdateMember(sysCtx, orgID, newMem.ID, patch)
				h.notifyUserAddedToOrg(ctx, orgID, id, jobTitle)
			} else if err != nil {
				h.log.ErrorContext(ctx, "add user organization membership", "error", err)
			}
		}
	}

	roleVal := strings.TrimSpace(r.PostFormValue("role"))
	if roleVal == "" && jobTitle != "" {
		roleVal = jobTitle
	}
	if roleVal != "" {
		h.notifyUserRoleChanged(ctx, id, orgID, roleVal)
	}

	if h.resolver != nil {
		h.resolver.Invalidate(id, orgID)
	}

	h.redirectAdminUsers(w, r, redirectTarget, "success", i18n.T(langOf(r), "admin.users.edit_success"))
}

// AdminUserPasswordSubmit sets a new password for a user, revokes active sessions, and notifies the user.
func (h *UIHandler) AdminUserPasswordSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/admin/users", http.StatusSeeOther)
		return
	}
	if h.idSvc == nil {
		h.renderError(w, r, apperr.Unavailable("identity", nil))
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.renderError(w, r, apperr.Validation("id.invalid", "Invalid user ID", nil))
		return
	}

	password := r.PostFormValue("password")
	confirmPassword := r.PostFormValue("confirm_password")
	redirectTarget := r.PostFormValue("redirect")

	if password == "" {
		h.redirectAdminUsers(w, r, redirectTarget, "error", "يرجى إدخال كلمة المرور الجديدة.")
		return
	}
	if password != confirmPassword {
		h.redirectAdminUsers(w, r, redirectTarget, "error", i18n.T(langOf(r), "admin.users.password_mismatch"))
		return
	}

	if err := h.idSvc.AdminSetPassword(ctx, id, actor.UserID, password); err != nil {
		h.redirectAdminUsers(w, r, redirectTarget, "error", h.safeMessage(err, langOf(r)))
		return
	}

	// Notify user of administrative password reset
	h.notifyUserPasswordSetByAdmin(ctx, id)

	h.redirectAdminUsers(w, r, redirectTarget, "success", i18n.T(langOf(r), "admin.users.password_success"))
}
