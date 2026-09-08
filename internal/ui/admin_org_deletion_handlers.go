package ui

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

const orgDeletionsPath = "/admin/organizations/deletion-requests"

// AdminOrgDeletionRequestsPage renders the organization deletion requests queue.
func (h *UIHandler) AdminOrgDeletionRequestsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	status := strings.TrimSpace(r.URL.Query().Get("status"))
	switch status {
	case "", string(org.OrgDeletionStatusPending), string(org.OrgDeletionStatusApproved),
		string(org.OrgDeletionStatusRejected), string(org.OrgDeletionStatusCancelled):
	default:
		status = ""
	}

	if r.URL.Query().Get("status") == "" && !r.URL.Query().Has("page") {
		status = string(org.OrgDeletionStatusPending)
	}

	page, perPage := 1, 25
	if n, err := strconv.Atoi(r.URL.Query().Get("page")); err == nil && n > 0 {
		page = n
	}
	if n, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && n > 0 && n <= 100 {
		perPage = n
	}

	var requests []*org.OrganizationDeletionRequest
	var total, pendingCount, approvedCount, rejectedCount int
	if h.orgSvc != nil {
		var err error
		requests, total, err = h.orgSvc.ListOrganizationDeletionRequests(ctx, status, perPage, (page-1)*perPage)
		if err != nil {
			h.log.ErrorContext(ctx, "list org deletion requests", "error", err)
			h.renderError(w, r, err)
			return
		}

		// Fetch counts for KPIs
		_, pendingCount, _ = h.orgSvc.ListOrganizationDeletionRequests(ctx, string(org.OrgDeletionStatusPending), 1, 0)
		_, approvedCount, _ = h.orgSvc.ListOrganizationDeletionRequests(ctx, string(org.OrgDeletionStatusApproved), 1, 0)
		_, rejectedCount, _ = h.orgSvc.ListOrganizationDeletionRequests(ctx, string(org.OrgDeletionStatusRejected), 1, 0)
	}

	view := pages.AdminOrgDeletionsView{
		Lang:          lang,
		Requests:      requests,
		Status:        status,
		Total:         total,
		PendingCount:  pendingCount,
		ApprovedCount: approvedCount,
		RejectedCount: rejectedCount,
		Page:          page,
		PerPage:       perPage,
		CanDecide:     actorCan(r, "org.organization.update"),
		NoticeKind:    r.URL.Query().Get("notice_type"),
		Notice:        r.URL.Query().Get("notice_msg"),
	}
	h.renderPage(ctx, w, "organization deletion requests", pages.AdminOrgDeletionsPage(view, lang, dir))
}

// AdminOrgDeletionApproveSubmit approves an organization deletion request, marking the organization and branches deleted.
func (h *UIHandler) AdminOrgDeletionApproveSubmit(w http.ResponseWriter, r *http.Request) {
	h.decideOrgDeletion(w, r, true)
}

// AdminOrgDeletionRejectSubmit rejects an organization deletion request.
func (h *UIHandler) AdminOrgDeletionRejectSubmit(w http.ResponseWriter, r *http.Request) {
	h.decideOrgDeletion(w, r, false)
}

func (h *UIHandler) decideOrgDeletion(w http.ResponseWriter, r *http.Request, approve bool) {
	ctx := r.Context()

	actor, ok := authctx.From(ctx)
	if !ok || actor.UserID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect="+url.QueryEscape(orgDeletionsPath), http.StatusSeeOther)
		return
	}

	id := parseInt64PathParam(r, "id")
	if id <= 0 {
		orgDeletionsNotice(w, r, "error", "معرف الطلب غير صالح.")
		return
	}

	if err := r.ParseForm(); err != nil {
		orgDeletionsNotice(w, r, "error", "بيانات الطلب غير صالحة.")
		return
	}
	notes := strings.TrimSpace(r.FormValue("admin_notes"))

	if _, err := h.orgSvc.ReviewOrganizationDeletion(ctx, id, actor.UserID, approve, notes); err != nil {
		h.log.ErrorContext(ctx, "decide org deletion request",
			"request_id", id, "approve", approve, "error", err)
		orgDeletionsNotice(w, r, "error", h.errorMessage(r, err))
		return
	}

	msg := "تمت الموافقة على طلب حذف المنشأة وتعطيلها بنجاح."
	if !approve {
		msg = "تم رفض طلب حذف المنشأة بنجاح."
	}
	orgDeletionsNotice(w, r, "success", msg)
}

func orgDeletionsNotice(w http.ResponseWriter, r *http.Request, kind, msg string) {
	target := fmt.Sprintf("%s?notice_type=%s&notice_msg=%s",
		orgDeletionsPath, kind, url.QueryEscape(msg))
	http.Redirect(w, r, target, http.StatusSeeOther)
}
