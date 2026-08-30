package ui

import (
	"net/http"

	"github.com/muhiya/dawa24-store/internal/modules/workflow"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
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

	h.renderPage(ctx, w, "render customer report issue", pages.CustomerReportIssuePage(lang, dir))
}

// CustomerReportIssueSubmit saves issue report into workflow.report_issues.
func (h *UIHandler) CustomerReportIssueSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.UserID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/report-issue", http.StatusSeeOther)
		return
	}

	_ = r.ParseForm()
	issueType := r.FormValue("issue_type")
	description := r.FormValue("description")

	if h.wfSvc == nil {
		h.redirectWithNotice(w, r, "/report-issue", "error", "خدمة البلاغات غير متوفرة.")
		return
	}

	issue := &workflow.ReportIssue{
		ReportedBy:  actor.UserID,
		IssueType:   issueType,
		Description: description,
		Priority:    "medium",
	}
	if actor.OrganizationID > 0 {
		issue.OrganizationID = &actor.OrganizationID
	}

	if _, err := h.wfSvc.ReportIssue(ctx, issue); err != nil {
		h.log.ErrorContext(ctx, "failed reporting issue", "error", err, "user_id", actor.UserID)
		h.redirectWithNotice(w, r, "/report-issue", "error", h.safeMessage(err, langOf(r)))
		return
	}

	h.redirectWithNotice(w, r, "/report-issue", "success", "تم إرسال البلاغ بنجاح، سيقوم فريق الدعم بمتابعته.")
}

// AdminReportIssuesPage renders admin review queue for submitted user issues.
func (h *UIHandler) AdminReportIssuesPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	h.renderPage(ctx, w, "render admin report issues", pages.AdminReportIssuesPage(lang, dir))
}
