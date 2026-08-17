package ui

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/hr"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// JobsPage renders the public job board.
func (h *UIHandler) JobsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	var jobs []*hr.JobOffer
	if h.hrSvc != nil {
		jobs, _ = h.hrSvc.ListPublishedJobs(ctx, 50, 0)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.JobsPage(lang, dir, jobs).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render jobs page", "error", err)
	}
}

// JobDetailPage renders one vacancy with an apply form.
func (h *UIHandler) JobDetailPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || h.hrSvc == nil {
		h.renderError(w, r, err)
		return
	}

	j, err := h.hrSvc.GetJobOffer(ctx, id)
	if err != nil {
		h.renderError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.JobDetail(lang, dir, j, false).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render job detail", "error", err)
	}
}

// JobApplySubmit records an application.
func (h *UIHandler) JobApplySubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	offerID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if h.hrSvc == nil {
		h.redirectWithNotice(w, r, "/jobs/"+strconv.FormatInt(offerID, 10), "error", "الخدمة غير متاحة حالياً.")
		return
	}

	offer, err := h.hrSvc.GetJobOffer(ctx, offerID)
	if err != nil {
		h.renderError(w, r, err)
		return
	}

	app := &hr.JobApplication{
		JobOfferID:     offerID,
		OrganizationID: offer.OrganizationID,
		ApplicantName:  r.PostFormValue("applicant_name"),
		ApplicantEmail: r.PostFormValue("applicant_email"),
		ApplicantPhone: r.PostFormValue("applicant_phone"),
	}
	if err := h.hrSvc.ApplyToJob(ctx, app); err != nil {
		h.redirectWithNotice(w, r, "/jobs/"+strconv.FormatInt(offerID, 10), "error", h.safeMessage(err, langOf(r)))
		return
	}

	// Re-render with the success state so the visitor stays on the page.
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.JobDetail(lang, dir, offer, true).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render job detail after apply", "error", err)
	}
}

// VendorJobsPage renders the supplier's job management surface.
func (h *UIHandler) VendorJobsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/jobs", http.StatusSeeOther)
		return
	}

	var jobs []*hr.JobOffer
	if h.hrSvc != nil {
		jobs, _ = h.hrSvc.ListOrgJobs(ctx, actor.OrganizationID, 50, 0)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.VendorJobs(lang, dir, jobs).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render vendor jobs", "error", err)
	}
}

// VendorJobCreateSubmit publishes a vacancy for the tenant organization.
func (h *UIHandler) VendorJobCreateSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/jobs", http.StatusSeeOther)
		return
	}

	if h.hrSvc == nil {
		h.redirectWithNotice(w, r, "/vendor/jobs", "error", "الخدمة غير متاحة حالياً.")
		return
	}

	j := &hr.JobOffer{
		Title:        i18n.New(r.PostFormValue("title_ar"), r.PostFormValue("title_en")),
		Description:  r.PostFormValue("description"),
		Requirements: r.PostFormValue("requirements"),
		Location:     r.PostFormValue("location"),
		Status:       "published",
	}
	if _, err := h.hrSvc.CreateJobOffer(ctx, actor.OrganizationID, j); err != nil {
		h.redirectWithNotice(w, r, "/vendor/jobs", "error", h.safeMessage(err, langOf(r)))
		return
	}
	h.redirectWithNotice(w, r, "/vendor/jobs", "success", "تم نشر الوظيفة.")
}
