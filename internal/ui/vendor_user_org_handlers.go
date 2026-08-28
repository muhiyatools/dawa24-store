package ui

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// VendorUserOrganizationsPage renders the vendor's customer user connections and requests.
func (h *UIHandler) VendorUserOrganizationsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	actor, ok := authctx.From(ctx)
	if !ok || !actor.IsVendor() {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/user-organization", http.StatusSeeOther)
		return
	}

	orgID := actor.OrganizationID
	if orgID <= 0 {
		orgID = actor.OrgID
	}
	if orgID <= 0 {
		http.Redirect(w, r, "/vendor/dashboard", http.StatusSeeOther)
		return
	}

	statusFilter := strings.TrimSpace(r.URL.Query().Get("status"))
	data := &pages.VendorUserOrgData{
		ActiveTab:  statusFilter,
		NoticeType: r.URL.Query().Get("notice_type"),
		NoticeMsg:  r.URL.Query().Get("notice_msg"),
	}

	sysCtx := database.AsSystem(ctx)
	if h.orgSvc != nil {
		allLinks, _ := h.orgSvc.ListUserOrganizationsByVendor(sysCtx, orgID, "")
		data.TotalCount = len(allLinks)
		for _, link := range allLinks {
			if link == nil {
				continue
			}
			switch link.Status {
			case org.UserOrgStatusPending:
				data.PendingCount++
			case org.UserOrgStatusApproved:
				data.ApprovedCount++
			case org.UserOrgStatusRejected:
				data.RejectedCount++
			}
		}

		filteredLinks, err := h.orgSvc.ListUserOrganizationsByVendor(sysCtx, orgID, statusFilter)
		if err == nil {
			data.UserOrgs = filteredLinks
		}

		// Load customer users for "ADD USER" modal
		if h.idSvc != nil {
			customers, err := h.idSvc.AdminListUsers(sysCtx, "", "customer")
			if err == nil {
				data.AllCustomers = customers
			}
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.VendorUserOrganizationsPage(lang, dir, data).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render vendor user organizations", "error", err)
	}
}

// VendorUserOrganizationCreateSubmit creates and immediately approves a customer link.
func (h *UIHandler) VendorUserOrganizationCreateSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || !actor.IsVendor() {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/user-organization", http.StatusSeeOther)
		return
	}

	orgID := actor.OrganizationID
	if orgID <= 0 {
		orgID = actor.OrgID
	}
	if orgID <= 0 {
		http.Redirect(w, r, "/vendor/dashboard", http.StatusSeeOther)
		return
	}

	targetUserID, err := strconv.ParseInt(strings.TrimSpace(r.FormValue("user_id")), 10, 64)
	if err != nil || targetUserID <= 0 {
		http.Redirect(w, r, "/vendor/user-organization?notice_type=error&notice_msg="+url.QueryEscape("يجب اختيار مستخدم صالح."), http.StatusSeeOther)
		return
	}

	orgNumber := strings.TrimSpace(r.FormValue("organization_number"))
	if orgNumber == "" {
		http.Redirect(w, r, "/vendor/user-organization?notice_type=error&notice_msg="+url.QueryEscape("رقم المنظمة مطلوب."), http.StatusSeeOther)
		return
	}

	sysCtx := database.AsSystem(ctx)
	if h.orgSvc != nil {
		_, err := h.orgSvc.CreateUserOrgLink(sysCtx, targetUserID, nil, orgID, orgNumber, org.UserOrgStatusApproved)
		if err != nil {
			h.log.ErrorContext(ctx, "vendor create user org link failed", "error", err)
			http.Redirect(w, r, "/vendor/user-organization?notice_type=error&notice_msg="+url.QueryEscape(err.Error()), http.StatusSeeOther)
			return
		}
	}

	http.Redirect(w, r, "/vendor/user-organization?notice_type=success&notice_msg="+url.QueryEscape("تم ربط واعتماد المستخدم بنجاح."), http.StatusSeeOther)
}

// VendorUserOrganizationApproveSubmit approves a pending pharmacy connection.
func (h *UIHandler) VendorUserOrganizationApproveSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || !actor.IsVendor() {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/user-organization", http.StatusSeeOther)
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		http.Redirect(w, r, "/vendor/user-organization?notice_type=error&notice_msg="+url.QueryEscape("معرف غير صالح."), http.StatusSeeOther)
		return
	}

	sysCtx := database.AsSystem(ctx)
	if h.orgSvc != nil {
		if err := h.orgSvc.ApproveUserOrgLink(sysCtx, id); err != nil {
			http.Redirect(w, r, "/vendor/user-organization?notice_type=error&notice_msg="+url.QueryEscape(err.Error()), http.StatusSeeOther)
			return
		}
	}

	http.Redirect(w, r, "/vendor/user-organization?notice_type=success&notice_msg="+url.QueryEscape("تم اعتماد وقبول المستخدم (APPROVAL ✓) بنجاح."), http.StatusSeeOther)
}

// VendorUserOrganizationRejectSubmit rejects a pharmacy connection request.
func (h *UIHandler) VendorUserOrganizationRejectSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || !actor.IsVendor() {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/user-organization", http.StatusSeeOther)
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		http.Redirect(w, r, "/vendor/user-organization?notice_type=error&notice_msg="+url.QueryEscape("معرف غير صالح."), http.StatusSeeOther)
		return
	}

	notes := strings.TrimSpace(r.FormValue("notes"))

	sysCtx := database.AsSystem(ctx)
	if h.orgSvc != nil {
		if err := h.orgSvc.RejectUserOrgLink(sysCtx, id, notes); err != nil {
			http.Redirect(w, r, "/vendor/user-organization?notice_type=error&notice_msg="+url.QueryEscape(err.Error()), http.StatusSeeOther)
			return
		}
	}

	http.Redirect(w, r, "/vendor/user-organization?notice_type=success&notice_msg="+url.QueryEscape("تم رفض طلب الربط."), http.StatusSeeOther)
}

// VendorUserOrganizationUpdateSubmit updates the organization number for a user.
func (h *UIHandler) VendorUserOrganizationUpdateSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || !actor.IsVendor() {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/user-organization", http.StatusSeeOther)
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		http.Redirect(w, r, "/vendor/user-organization?notice_type=error&notice_msg="+url.QueryEscape("معرف غير صالح."), http.StatusSeeOther)
		return
	}

	orgNumber := strings.TrimSpace(r.FormValue("organization_number"))
	notes := strings.TrimSpace(r.FormValue("notes"))

	sysCtx := database.AsSystem(ctx)
	if h.orgSvc != nil {
		if err := h.orgSvc.UpdateUserOrgLink(sysCtx, id, orgNumber, notes); err != nil {
			http.Redirect(w, r, "/vendor/user-organization?notice_type=error&notice_msg="+url.QueryEscape(err.Error()), http.StatusSeeOther)
			return
		}
	}

	http.Redirect(w, r, "/vendor/user-organization?notice_type=success&notice_msg="+url.QueryEscape("تم تحديث بيانات المستخدم بنجاح."), http.StatusSeeOther)
}

// VendorUserOrganizationDeleteSubmit removes a user link.
func (h *UIHandler) VendorUserOrganizationDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || !actor.IsVendor() {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/user-organization", http.StatusSeeOther)
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		http.Redirect(w, r, "/vendor/user-organization?notice_type=error&notice_msg="+url.QueryEscape("معرف غير صالح."), http.StatusSeeOther)
		return
	}

	sysCtx := database.AsSystem(ctx)
	if h.orgSvc != nil {
		if err := h.orgSvc.DeleteUserOrgLink(sysCtx, id); err != nil {
			http.Redirect(w, r, "/vendor/user-organization?notice_type=error&notice_msg="+url.QueryEscape(err.Error()), http.StatusSeeOther)
			return
		}
	}

	http.Redirect(w, r, "/vendor/user-organization?notice_type=success&notice_msg="+url.QueryEscape("تم حذف المستخدم بنجاح."), http.StatusSeeOther)
}
