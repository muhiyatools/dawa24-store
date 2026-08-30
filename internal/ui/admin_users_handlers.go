package ui

import (
	"context"
	"fmt"
	"net/http"
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
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// AdminUsersPage renders the dedicated user management directory.
func (h *UIHandler) AdminUsersPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sysCtx := database.AsSystem(ctx)
	lang, dir := h.localeAndDir(r)

	searchQuery := strings.TrimSpace(r.URL.Query().Get("q"))
	roleFilter := strings.TrimSpace(r.URL.Query().Get("role"))
	statusFilter := strings.TrimSpace(r.URL.Query().Get("status"))

	var users []*identity.User
	var allOrgs []*org.Organization
	orgNames := make(map[int64]string)
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
	}

	if roleFilter == "" {
		roleFilter = strings.TrimSpace(r.URL.Query().Get("type"))
	}

	if h.idSvc != nil {
		allUsers, _ := h.idSvc.AdminListUsers(sysCtx, "", "")
		for _, u := range allUsers {
			if u == nil {
				continue
			}
			totalUsers++
			rLower := strings.ToLower(u.Role)
			if rLower == "super_admin" || rLower == "admin" || rLower == "staff" || rLower == "support" || rLower == "developer" || rLower == "finance" || rLower == "auditor" || rLower == "employer" {
				staffUsers++
			} else if rLower == "vendor" || rLower == "supplier" || rLower == "warehouse_keeper" || rLower == "sales_rep" || rLower == "driver" {
				vendorUsers++
			} else {
				customerUsers++
			}
			if u.Status == identity.StatusActive {
				activeUsers++
			} else if u.Status == identity.StatusSuspended {
				suspendedUsers++
			}
		}

		for _, u := range allUsers {
			if u == nil {
				continue
			}
			if statusFilter != "" && string(u.Status) != statusFilter {
				continue
			}
			if roleFilter != "" {
				rLower := strings.ToLower(u.Role)
				switch strings.ToLower(roleFilter) {
				case "customer", "pharmacy", "pharmacies":
					if rLower != "customer" && rLower != "pharmacy" && rLower != "individual" && rLower != "pharmacist" && rLower != "buyer" && rLower != "pharmacist_assistant" {
						continue
					}
				case "vendor", "supplier", "suppliers", "vendors":
					if rLower != "vendor" && rLower != "supplier" && rLower != "warehouse_keeper" && rLower != "sales_rep" && rLower != "driver" {
						continue
					}
				case "staff", "admin", "admins", "super_admin":
					if rLower != "super_admin" && rLower != "admin" && rLower != "staff" && rLower != "support" && rLower != "developer" && rLower != "finance" && rLower != "auditor" && rLower != "employer" {
						continue
					}
				case "new":
					if time.Since(u.CreatedAt) > 30*24*time.Hour {
						continue
					}
				default:
					if !strings.EqualFold(u.Role, roleFilter) {
						continue
					}
				}
			}
			if searchQuery != "" {
				qLower := strings.ToLower(searchQuery)
				nameAr := strings.ToLower(u.Name.Get("ar"))
				nameEn := strings.ToLower(u.Name.Get("en"))
				email := strings.ToLower(u.Email)
				phone := strings.ToLower(u.Phone)
				role := strings.ToLower(u.Role)
				idStr := fmt.Sprintf("%d", u.ID)
				usrStr := fmt.Sprintf("usr-%d", u.ID)
				if !strings.Contains(nameAr, qLower) &&
					!strings.Contains(nameEn, qLower) &&
					!strings.Contains(email, qLower) &&
					!strings.Contains(phone, qLower) &&
					!strings.Contains(role, qLower) &&
					!strings.Contains(idStr, qLower) &&
					!strings.Contains(usrStr, qLower) {
					continue
				}
			}
			users = append(users, u)
		}

		deletionRequests, _ = h.idSvc.AdminListDeletionRequests(sysCtx, "")
	}

	data := pages.AdminUsersPageData{
		Users:            users,
		Organizations:    allOrgs,
		OrgNames:         orgNames,
		DeletionRequests: deletionRequests,
		TotalUsers:       totalUsers,
		CustomerUsers:    customerUsers,
		VendorUsers:      vendorUsers,
		StaffUsers:       staffUsers,
		ActiveUsers:      activeUsers,
		SuspendedUsers:   suspendedUsers,
		SearchQuery:      searchQuery,
		RoleFilter:       roleFilter,
		StatusFilter:     statusFilter,
		NoticeKind:       r.URL.Query().Get("notice"),
		Notice:           r.URL.Query().Get("msg"),
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

	if err := action(ctx, id, actor.UserID); err != nil {
		h.redirectWithNotice(w, r, "/admin/users", "error", h.safeMessage(err, langOf(r)))
		return
	}

	h.redirectWithNotice(w, r, "/admin/users", "success", i18n.T(langOf(r), "admin.users.action_success"))
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

// AdminUserDeletionApproveSubmit approves an account deletion request and deletes/suspends the account.
func (h *UIHandler) AdminUserDeletionApproveSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok || (!actor.IsStaff && !actor.IsPlatformAdmin()) {
		http.Redirect(w, r, "/auth/login?redirect=/admin/users?tab=deletion_requests", http.StatusSeeOther)
		return
	}

	reqID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || reqID <= 0 {
		h.redirectWithNotice(w, r, "/admin/users?tab=deletion_requests", "error", i18n.T(lang, "admin.docs.invalid_request_id"))
		return
	}

	if h.idSvc != nil {
		if err := h.idSvc.AdminReviewDeletionRequest(ctx, reqID, actor.UserID, true, i18n.T(lang, "admin.users.deletion_approved_reason")); err != nil {
			h.redirectWithNotice(w, r, "/admin/users?tab=deletion_requests", "error", h.safeMessage(err, lang))
			return
		}
	}

	h.redirectWithNotice(w, r, "/admin/users?tab=deletion_requests", "success", i18n.T(lang, "admin.users.deletion_approved_success"))
}

// AdminUserDeletionRejectSubmit rejects an account deletion request.
func (h *UIHandler) AdminUserDeletionRejectSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok || (!actor.IsStaff && !actor.IsPlatformAdmin()) {
		http.Redirect(w, r, "/auth/login?redirect=/admin/users?tab=deletion_requests", http.StatusSeeOther)
		return
	}

	reqID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || reqID <= 0 {
		h.redirectWithNotice(w, r, "/admin/users?tab=deletion_requests", "error", i18n.T(lang, "admin.docs.invalid_request_id"))
		return
	}

	if h.idSvc != nil {
		if err := h.idSvc.AdminReviewDeletionRequest(ctx, reqID, actor.UserID, false, i18n.T(lang, "admin.users.deletion_rejected_reason")); err != nil {
			h.redirectWithNotice(w, r, "/admin/users?tab=deletion_requests", "error", h.safeMessage(err, lang))
			return
		}
	}

	h.redirectWithNotice(w, r, "/admin/users?tab=deletion_requests", "success", i18n.T(lang, "admin.users.deletion_rejected_success"))
}
