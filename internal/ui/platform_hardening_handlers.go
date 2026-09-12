package ui

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/muhiya/dawa24-store/internal/modules/workflow"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// AdminSessionPlansPage renders session plan management and seats list.
func (h *UIHandler) AdminSessionPlansPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	h.renderPage(ctx, w, "render admin session plans", pages.AdminSessionPlansPage(lang, dir))
}

// AdminSessionPlanRequestsPage renders multi-session seat request queue.
func (h *UIHandler) AdminSessionPlanRequestsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	h.renderPage(ctx, w, "render admin session plan requests", pages.AdminSessionPlanRequestsPage(lang, dir))
}

// CustomerReportIssuePage renders issue report form for customers and vendors.
func (h *UIHandler) CustomerReportIssuePage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	var userIssues []*workflow.ReportIssue
	if actor, ok := authctx.From(ctx); ok && actor.UserID > 0 && h.wfSvc != nil {
		if list, err := h.wfSvc.ListIssuesByReporter(ctx, actor.UserID, 25, 0); err == nil {
			userIssues = list
		}
	}

	h.renderPage(ctx, w, "render customer report issue", pages.CustomerReportIssuePage(lang, dir, userIssues))
}

// CustomerReportIssueSubmit saves issue report into workflow.report_issues.
func (h *UIHandler) CustomerReportIssueSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok || actor.UserID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/report-issue", http.StatusSeeOther)
		return
	}

	_ = r.ParseForm()
	issueType := strings.TrimSpace(r.FormValue("issue_type"))
	if issueType == "" {
		issueType = "technical"
	}
	priority := strings.TrimSpace(r.FormValue("priority"))
	if priority != "low" && priority != "high" {
		priority = "medium"
	}
	description := strings.TrimSpace(r.FormValue("description"))
	orderIDStr := strings.TrimSpace(r.FormValue("order_id"))

	if description == "" {
		h.redirectWithNotice(w, r, "/report-issue", "error", "يرجى كتابة تفاصيل البلاغ بشكل واضح.")
		return
	}

	if h.wfSvc == nil {
		h.redirectWithNotice(w, r, "/report-issue", "error", i18n.T(lang, "issues.service_unavailable"))
		return
	}

	issue := &workflow.ReportIssue{
		ReportedBy:  actor.UserID,
		IssueType:   issueType,
		Description: description,
		Priority:    priority,
	}
	if actor.OrganizationID > 0 {
		issue.OrganizationID = &actor.OrganizationID
	}
	if orderIDStr != "" {
		if oid, err := strconv.ParseInt(orderIDStr, 10, 64); err == nil && oid > 0 {
			issue.OrderID = &oid
		}
	}

	created, err := h.wfSvc.ReportIssue(ctx, issue)
	if err != nil {
		h.log.ErrorContext(ctx, "failed reporting issue", "error", err, "user_id", actor.UserID)
		h.redirectWithNotice(w, r, "/report-issue", "error", h.safeMessage(err, lang))
		return
	}

	// Dispatch in-app notification to administrators
	reporterName := h.resolveUserName(ctx, actor.UserID)
	orgName := ""
	if actor.OrganizationID > 0 {
		orgName = h.resolveOrgName(ctx, actor.OrganizationID)
	}
	h.notifyAdminNewIssue(ctx, created, reporterName, orgName)

	h.redirectWithNotice(w, r, "/report-issue", "success", "تم إرسال البلاغ بنجاح وسيتم متابعته من قبل فريق الدعم الفني.")
}

// AdminReportIssuesPage renders admin review queue for submitted user issues.
func (h *UIHandler) AdminReportIssuesPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	status := strings.TrimSpace(r.URL.Query().Get("status"))
	issueType := strings.TrimSpace(r.URL.Query().Get("type"))
	priority := strings.TrimSpace(r.URL.Query().Get("priority"))
	search := strings.TrimSpace(r.URL.Query().Get("q"))

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	pageSize := 20
	offset := (page - 1) * pageSize

	var issues []*workflow.ReportIssueDetail
	var total int
	var stats *workflow.ReportIssueStats

	if h.wfSvc != nil {
		var err error
		filter := workflow.ReportIssueFilter{
			Status:    status,
			IssueType: issueType,
			Priority:  priority,
			Search:    search,
			Limit:     pageSize,
			Offset:    offset,
		}
		issues, total, stats, err = h.wfSvc.ListIssuesAdmin(ctx, filter)
		if err != nil {
			h.log.ErrorContext(ctx, "failed listing report issues for admin", "error", err)
		}
	}

	totalPages := 1
	if total > 0 {
		totalPages = (total + pageSize - 1) / pageSize
	}

	data := pages.AdminReportIssuesData{
		Issues:      issues,
		Total:       total,
		Stats:       stats,
		Status:      status,
		IssueType:   issueType,
		Priority:    priority,
		Search:      search,
		CurrentPage: page,
		TotalPages:  totalPages,
		PageSize:    pageSize,
	}

	h.renderPage(ctx, w, "render admin report issues", pages.AdminReportIssuesPage(lang, dir, data))
}

// AdminReportIssueUpdateSubmit handles updating issue status and adding response notes.
func (h *UIHandler) AdminReportIssueUpdateSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/admin/report-issues", "error", "معرف البلاغ غير صالح.")
		return
	}

	_ = r.ParseForm()
	status := strings.TrimSpace(r.FormValue("status"))
	responseNotes := strings.TrimSpace(r.FormValue("response_notes"))

	if status != "pending" && status != "in_progress" && status != "resolved" {
		h.redirectWithNotice(w, r, "/admin/report-issues", "error", "حالة البلاغ المحددة غير صحيحة.")
		return
	}

	if h.wfSvc == nil {
		h.redirectWithNotice(w, r, "/admin/report-issues", "error", "خدمة البلاغات غير متاحة حالياً.")
		return
	}

	existing, err := h.wfSvc.GetIssueByID(ctx, id)
	if err != nil || existing == nil {
		h.log.ErrorContext(ctx, "failed retrieving issue for update", "error", err, "id", id)
		h.redirectWithNotice(w, r, "/admin/report-issues", "error", "لم يتم العثور على البلاغ المطلوب.")
		return
	}

	if err := h.wfSvc.UpdateIssueStatus(ctx, id, status, responseNotes); err != nil {
		h.log.ErrorContext(ctx, "failed updating issue status", "error", err, "id", id)
		h.redirectWithNotice(w, r, "/admin/report-issues", "error", "حدث خطأ أثناء تحديث حالة البلاغ.")
		return
	}

	// Dispatch notification to the user who reported the issue
	h.notifyUserIssueResponse(ctx, existing, status, responseNotes)

	h.redirectWithNotice(w, r, "/admin/report-issues", "success", "تم تحديث حالة البلاغ بنجاح وإشعار المستخدم بالرد.")
}
