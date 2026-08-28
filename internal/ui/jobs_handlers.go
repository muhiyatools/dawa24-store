package ui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/hr"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/features"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// JobsPage renders the job board (public or authenticated in customer/vendor dashboard).
func (h *UIHandler) JobsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !features.Enabled(ctx, "jobs.enabled") {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	lang, dir := h.localeAndDir(r)
	actor, isLoggedIn := authctx.From(ctx)

	var rawJobs []*hr.JobOffer
	if h.hrSvc != nil {
		rawJobs, _ = h.hrSvc.ListPublishedJobs(ctx, 100, 0)
	}

	// Enrich with organization names
	orgNames := make(map[int64]string)
	if h.orgSvc != nil {
		for _, j := range rawJobs {
			if _, exists := orgNames[j.OrganizationID]; !exists && j.OrganizationID > 0 {
				if o, err := h.orgSvc.GetOrganization(ctx, j.OrganizationID); err == nil && o != nil {
					cName := o.TradeName.Get(i18n.ParseLang(lang))
					if cName == "" {
						cName = o.LegalName
					}
					if cName != "" {
						orgNames[j.OrganizationID] = cName
					}
				}
			}
		}
	}

	var jobItems []*pages.JobItemView
	for _, j := range rawJobs {
		compName := orgNames[j.OrganizationID]
		if compName == "" {
			compName = "جهة صيدلانية معتمدة"
		}
		jobItems = append(jobItems, &pages.JobItemView{
			Job:         j,
			CompanyName: compName,
			IsVerified:  true,
		})
	}

	// The post job button shows ONLY for vendors & staff, NEVER for pharmacy customers
	canPost := isLoggedIn && (actor.IsVendor() || actor.IsStaff)

	data := pages.JobsPageData{
		Jobs:       jobItems,
		TotalCount: len(jobItems),
		Cities:     h.listCities(ctx),
		Actor:      actor,
		IsLoggedIn: isLoggedIn,
		IsCustomer: isLoggedIn && actor.IsCustomer(),
		IsVendor:   isLoggedIn && actor.IsVendor(),
		IsAdmin:    isLoggedIn && actor.IsStaff,
		CanPostJob: canPost,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.JobsPage(data, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render jobs page", "error", err)
	}
}

// JobDetailPage renders one vacancy with an apply form (adapted to dashboard shell).
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

	compName := "جهة صيدلانية معتمدة"
	if h.orgSvc != nil && j.OrganizationID > 0 {
		if o, err := h.orgSvc.GetOrganization(ctx, j.OrganizationID); err == nil && o != nil {
			compName = o.TradeName.Get(i18n.ParseLang(lang))
			if compName == "" {
				compName = o.LegalName
			}
		}
	}

	actor, isLoggedIn := authctx.From(ctx)
	userEmail, userName, userPhone := "", "", ""
	if isLoggedIn && h.idSvc != nil {
		if u, err := h.idSvc.GetUserByID(ctx, actor.UserID); err == nil && u != nil {
			userEmail = u.Email
			userName = u.Name.Get(i18n.Lang(lang))
			userPhone = u.Phone
		}
	}

	data := pages.JobDetailData{
		Job:         j,
		CompanyName: compName,
		Actor:       actor,
		IsLoggedIn:  isLoggedIn,
		IsCustomer:  isLoggedIn && actor.IsCustomer(),
		IsVendor:    isLoggedIn && actor.IsVendor(),
		IsAdmin:     isLoggedIn && actor.IsStaff,
		Submitted:   false,
		UserEmail:   userEmail,
		UserName:    userName,
		UserPhone:   userPhone,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.JobDetail(data, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render job detail", "error", err)
	}
}

// JobApplySubmit records an application (requires logged-in user).
func (h *UIHandler) JobApplySubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	offerID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, fmt.Sprintf("/auth/login?redirect=/jobs/%d", offerID), http.StatusSeeOther)
		return
	}

	if h.hrSvc == nil {
		h.redirectWithNotice(w, r, "/jobs/"+strconv.FormatInt(offerID, 10), "error", "الخدمة غير متاحة حالياً.")
		return
	}

	offer, err := h.hrSvc.GetJobOffer(ctx, offerID)
	if err != nil {
		h.renderError(w, r, err)
		return
	}

	compName := "جهة صيدلانية معتمدة"
	if h.orgSvc != nil && offer.OrganizationID > 0 {
		if o, err := h.orgSvc.GetOrganization(ctx, offer.OrganizationID); err == nil && o != nil {
			compName = o.TradeName.Get(i18n.ParseLang(lang))
			if compName == "" {
				compName = o.LegalName
			}
		}
	}

	// Process attached CV
	cvURL, _ := saveUploadedFile(r, "resume_file", "resumes")

	app := &hr.JobApplication{
		JobOfferID:      offerID,
		OrganizationID:  offer.OrganizationID,
		ApplicantUserID: &actor.UserID,
		ApplicantName:   strings.TrimSpace(r.PostFormValue("applicant_name")),
		ApplicantEmail:  strings.TrimSpace(r.PostFormValue("applicant_email")),
		ApplicantPhone:  strings.TrimSpace(r.PostFormValue("applicant_phone")),
		CVStorageKey:    cvURL,
		Notes:           strings.TrimSpace(r.PostFormValue("notes")),
		Status:          "pending",
	}
	if err := h.hrSvc.ApplyToJob(ctx, app); err != nil {
		h.redirectWithNotice(w, r, "/jobs/"+strconv.FormatInt(offerID, 10), "error", h.safeMessage(err, langOf(r)))
		return
	}

	// Re-render with the success state so the applicant stays on the page.
	data := pages.JobDetailData{
		Job:         offer,
		CompanyName: compName,
		Actor:       actor,
		IsLoggedIn:  true,
		IsCustomer:  actor.IsCustomer(),
		IsVendor:    actor.IsVendor(),
		IsAdmin:     actor.IsStaff,
		Submitted:   true,
		UserEmail:   app.ApplicantEmail,
		UserName:    app.ApplicantName,
		UserPhone:   app.ApplicantPhone,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.JobDetail(data, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render job detail after apply", "error", err)
	}
}

// VendorJobsPage renders the supplier's job management surface.
func (h *UIHandler) VendorJobsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/jobs", http.StatusSeeOther)
		return
	}

	var jobItems []*pages.VendorJobItem
	var publishedCount, closedCount, totalApps int

	if h.hrSvc != nil {
		jobs, _ := h.hrSvc.ListOrgJobs(ctx, actor.OrganizationID, 100, 0)
		for _, j := range jobs {
			if j == nil {
				continue
			}
			cnt := 0
			if appCount, err := h.hrSvc.CountApplications(ctx, j.ID); err == nil {
				cnt = appCount
			}
			totalApps += cnt
			if j.Status == "published" {
				publishedCount++
			} else {
				closedCount++
			}
			jobItems = append(jobItems, &pages.VendorJobItem{
				Job:               j,
				ApplicationsCount: cnt,
			})
		}
	}

	noticeType := r.URL.Query().Get("notice_type")
	noticeMsg := r.URL.Query().Get("notice")

	data := pages.VendorJobsData{
		Jobs:              jobItems,
		TotalCount:        len(jobItems),
		PublishedCount:    publishedCount,
		ClosedCount:       closedCount,
		TotalApplications: totalApps,
		NoticeType:        noticeType,
		NoticeMsg:         noticeMsg,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.VendorJobs(lang, dir, data).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render vendor jobs", "error", err)
	}
}

// VendorJobCreateSubmit publishes a vacancy for the tenant organization.
func (h *UIHandler) VendorJobCreateSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/jobs", http.StatusSeeOther)
		return
	}

	if h.hrSvc == nil {
		h.redirectWithNotice(w, r, "/vendor/jobs", "error", "خدمة التوظيف غير متاحة حالياً.")
		return
	}

	_ = r.ParseForm()

	titleAr := strings.TrimSpace(r.PostFormValue("title_ar"))
	titleEn := strings.TrimSpace(r.PostFormValue("title_en"))
	if titleAr == "" {
		h.redirectWithNotice(w, r, "/vendor/jobs", "error", "المسمى الوظيفي بالعربية مطلوب.")
		return
	}
	if titleEn == "" {
		titleEn = titleAr
	}

	location := strings.TrimSpace(r.PostFormValue("location"))
	desc := strings.TrimSpace(r.PostFormValue("description"))
	reqs := strings.TrimSpace(r.PostFormValue("requirements"))

	salMin, _ := money.Parse(strings.TrimSpace(r.PostFormValue("salary_min")))
	salMax, _ := money.Parse(strings.TrimSpace(r.PostFormValue("salary_max")))

	j := &hr.JobOffer{
		Title:        i18n.New(titleAr, titleEn),
		Description:  desc,
		Requirements: reqs,
		Location:     location,
		SalaryMin:    salMin,
		SalaryMax:    salMax,
		Status:       "published",
	}

	if _, err := h.hrSvc.CreateJobOffer(ctx, actor.OrganizationID, j); err != nil {
		h.redirectWithNotice(w, r, "/vendor/jobs", "error", h.safeMessage(err, langOf(r)))
		return
	}

	h.redirectWithNotice(w, r, "/vendor/jobs", "success", "تم نشر الشاغر الوظيفي بنجاح.")
}

// VendorJobToggleSubmit toggles a vacancy between published and closed.
func (h *UIHandler) VendorJobToggleSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/jobs", http.StatusSeeOther)
		return
	}

	jobID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || jobID <= 0 {
		h.redirectWithNotice(w, r, "/vendor/jobs", "error", "معرف وظيفة غير صالح.")
		return
	}

	if h.hrSvc != nil {
		if err := h.hrSvc.ToggleJobOfferStatus(ctx, actor.OrganizationID, jobID); err != nil {
			h.redirectWithNotice(w, r, "/vendor/jobs", "error", h.safeMessage(err, langOf(r)))
			return
		}
	}

	h.redirectWithNotice(w, r, "/vendor/jobs", "success", "تم تحديث حالة الوظيفة بنجاح.")
}

// VendorJobDeleteSubmit removes a vacancy.
func (h *UIHandler) VendorJobDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/jobs", http.StatusSeeOther)
		return
	}

	jobID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || jobID <= 0 {
		h.redirectWithNotice(w, r, "/vendor/jobs", "error", "معرف وظيفة غير صالح.")
		return
	}

	if h.hrSvc != nil {
		if err := h.hrSvc.DeleteJobOffer(ctx, actor.OrganizationID, jobID); err != nil {
			h.redirectWithNotice(w, r, "/vendor/jobs", "error", h.safeMessage(err, langOf(r)))
			return
		}
	}

	h.redirectWithNotice(w, r, "/vendor/jobs", "success", "تم حذف الوظيفة بنجاح.")
}

// VendorJobApplicationsJSON returns applicants for a specific vacancy as JSON.
func (h *UIHandler) VendorJobApplicationsJSON(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	jobID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || jobID <= 0 {
		http.Error(w, `{"error":"invalid job id"}`, http.StatusBadRequest)
		return
	}

	var apps []*hr.JobApplication
	if h.hrSvc != nil {
		aList, _ := h.hrSvc.ListApplicationsByOffer(ctx, jobID, 100, 0)
		apps = aList
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(apps)
}
