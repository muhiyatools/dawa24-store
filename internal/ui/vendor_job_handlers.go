package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/hr"
	"github.com/muhiya/dawa24-store/internal/modules/notifications"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

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

	branches := h.accessibleBranchesForActor(ctx, actor)

	noticeType := r.URL.Query().Get("notice")
	if noticeType == "" {
		noticeType = r.URL.Query().Get("notice_type")
	}
	noticeMsg := r.URL.Query().Get("msg")
	if noticeMsg == "" {
		noticeMsg = r.URL.Query().Get("message")
	}

	data := pages.VendorJobsData{
		Jobs:              jobItems,
		Branches:          branches,
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

// VendorJobCreateSubmit publishes a vacancy for the vendor organization.
func (h *UIHandler) VendorJobCreateSubmit(w http.ResponseWriter, r *http.Request) {
	h.handleJobCreateSubmit(w, r, "/vendor/jobs")
}

// VendorJobUpdateSubmit modifies an existing vacancy for the vendor organization.
func (h *UIHandler) VendorJobUpdateSubmit(w http.ResponseWriter, r *http.Request) {
	h.handleJobUpdateSubmit(w, r, "/vendor/jobs")
}

// VendorJobToggleSubmit toggles a vacancy between published and closed.
func (h *UIHandler) VendorJobToggleSubmit(w http.ResponseWriter, r *http.Request) {
	h.handleJobToggleSubmit(w, r, "/vendor/jobs")
}

// VendorJobDeleteSubmit removes a vacancy.
func (h *UIHandler) VendorJobDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	h.handleJobDeleteSubmit(w, r, "/vendor/jobs")
}

// VendorJobApplicationsJSON returns applicants for a specific vacancy as JSON.
func (h *UIHandler) VendorJobApplicationsJSON(w http.ResponseWriter, r *http.Request) {
	h.handleJobApplicationsJSON(w, r)
}

// CustomerJobsPage renders the pharmacy customer's job management surface.
func (h *UIHandler) CustomerJobsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/customer/jobs", http.StatusSeeOther)
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

	branches := h.accessibleBranchesForActor(ctx, actor)

	noticeType := r.URL.Query().Get("notice")
	if noticeType == "" {
		noticeType = r.URL.Query().Get("notice_type")
	}
	noticeMsg := r.URL.Query().Get("msg")
	if noticeMsg == "" {
		noticeMsg = r.URL.Query().Get("message")
	}

	data := pages.CustomerJobsData{
		Jobs:              jobItems,
		Branches:          branches,
		TotalCount:        len(jobItems),
		PublishedCount:    publishedCount,
		ClosedCount:       closedCount,
		TotalApplications: totalApps,
		NoticeType:        noticeType,
		NoticeMsg:         noticeMsg,
		Permissions:       actor.Permissions,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.CustomerJobs(lang, dir, data).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render customer jobs", "error", err)
	}
}

// CustomerJobCreateSubmit publishes a vacancy for the pharmacy organization.
func (h *UIHandler) CustomerJobCreateSubmit(w http.ResponseWriter, r *http.Request) {
	h.handleJobCreateSubmit(w, r, "/customer/jobs")
}

// CustomerJobUpdateSubmit modifies an existing vacancy for the pharmacy organization.
func (h *UIHandler) CustomerJobUpdateSubmit(w http.ResponseWriter, r *http.Request) {
	h.handleJobUpdateSubmit(w, r, "/customer/jobs")
}

// CustomerJobToggleSubmit toggles a vacancy between published and closed.
func (h *UIHandler) CustomerJobToggleSubmit(w http.ResponseWriter, r *http.Request) {
	h.handleJobToggleSubmit(w, r, "/customer/jobs")
}

// CustomerJobDeleteSubmit removes a vacancy.
func (h *UIHandler) CustomerJobDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	h.handleJobDeleteSubmit(w, r, "/customer/jobs")
}

// CustomerJobApplicationsJSON returns applicants for a specific vacancy as JSON.
func (h *UIHandler) CustomerJobApplicationsJSON(w http.ResponseWriter, r *http.Request) {
	h.handleJobApplicationsJSON(w, r)
}

// VendorJobApplicationAcceptSubmit accepts and onboards an applicant for vendor.
func (h *UIHandler) VendorJobApplicationAcceptSubmit(w http.ResponseWriter, r *http.Request) {
	h.handleJobApplicationAccept(w, r)
}

// CustomerJobApplicationAcceptSubmit accepts and onboards an applicant for pharmacy customer.
func (h *UIHandler) CustomerJobApplicationAcceptSubmit(w http.ResponseWriter, r *http.Request) {
	h.handleJobApplicationAccept(w, r)
}

// VendorJobApplicationRejectSubmit rejects an application for vendor.
func (h *UIHandler) VendorJobApplicationRejectSubmit(w http.ResponseWriter, r *http.Request) {
	h.handleJobApplicationReject(w, r)
}

// CustomerJobApplicationRejectSubmit rejects an application for pharmacy customer.
func (h *UIHandler) CustomerJobApplicationRejectSubmit(w http.ResponseWriter, r *http.Request) {
	h.handleJobApplicationReject(w, r)
}

// ================= Helpers =================

// accessibleBranchesForActor returns the branches of the actor's organization that the user is authorized to manage/select.
func (h *UIHandler) accessibleBranchesForActor(ctx context.Context, actor authctx.Actor) []*org.Branch {
	if h.orgSvc == nil || actor.OrganizationID <= 0 {
		return nil
	}
	allBranches, err := h.orgSvc.ListBranches(ctx, actor.OrganizationID)
	if err != nil || len(allBranches) == 0 {
		return nil
	}

	// Check if the user has permission to manage/view all branches across their organization.
	canManageAll := actor.IsOwner || actor.IsPlatformAdmin() ||
		actor.Can("pharmacy.branch.view") || actor.Can("pharmacy.branch.manage") ||
		actor.Can("pharmacy.branch.update") || actor.Can("vendor.branch.view") ||
		actor.Can("vendor.branch.manage") || actor.Can("vendor.branch.update")

	// If restricted to a specific branch and lacks global branch management permissions, restrict available branches.
	if !canManageAll && actor.BranchID != nil && *actor.BranchID > 0 {
		var filtered []*org.Branch
		for _, b := range allBranches {
			if b != nil && b.ID == *actor.BranchID {
				filtered = append(filtered, b)
			}
		}
		if len(filtered) > 0 {
			return filtered
		}
	}

	return allBranches
}

func (h *UIHandler) handleJobCreateSubmit(w http.ResponseWriter, r *http.Request, redirectURL string) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect="+redirectURL, http.StatusSeeOther)
		return
	}

	if h.hrSvc == nil {
		h.redirectWithNotice(w, r, redirectURL, "error", "خدمة التوظيف غير متاحة حالياً.")
		return
	}

	_ = r.ParseForm()

	titleAr := strings.TrimSpace(r.PostFormValue("title_ar"))
	titleEn := strings.TrimSpace(r.PostFormValue("title_en"))
	if titleAr == "" {
		h.redirectWithNotice(w, r, redirectURL, "error", "المسمى الوظيفي بالعربية مطلوب.")
		return
	}
	if titleEn == "" {
		titleEn = titleAr
	}

	accessibleBranches := h.accessibleBranchesForActor(ctx, actor)
	branchIDStr := strings.TrimSpace(r.PostFormValue("branch_id"))
	var location string

	if len(accessibleBranches) > 0 {
		if branchIDStr == "" {
			h.redirectWithNotice(w, r, redirectURL, "error", "يرجى اختيار الفرع المخصص للشاغر الوظيفي.")
			return
		}
		branchID, err := strconv.ParseInt(branchIDStr, 10, 64)
		if err != nil || branchID <= 0 {
			h.redirectWithNotice(w, r, redirectURL, "error", "معرف الفرع المحدد غير صالح.")
			return
		}

		var selectedBranch *org.Branch
		for _, b := range accessibleBranches {
			if b != nil && b.ID == branchID {
				selectedBranch = b
				break
			}
		}
		if selectedBranch == nil {
			h.redirectWithNotice(w, r, redirectURL, "error", "الفرع المختار غير متاح أو لا تملك صلاحية النشر عليه.")
			return
		}

		location = selectedBranch.Name.Get(i18n.AR)
		if location == "" {
			location = selectedBranch.Name.Get(i18n.EN)
		}
		if location == "" {
			location = fmt.Sprintf("فرع #%d", selectedBranch.ID)
		}
	} else {
		location = strings.TrimSpace(r.PostFormValue("location"))
		if location == "" {
			location = "الفرع الرئيسي"
		}
	}

	desc := strings.TrimSpace(r.PostFormValue("description"))
	reqs := strings.TrimSpace(r.PostFormValue("requirements"))

	salMin, _ := money.Parse(strings.TrimSpace(r.PostFormValue("salary_min")))
	salMax, _ := money.Parse(strings.TrimSpace(r.PostFormValue("salary_max")))

	status := strings.TrimSpace(r.PostFormValue("status"))
	if status == "" {
		status = "published"
	}

	j := &hr.JobOffer{
		Title:        i18n.New(titleAr, titleEn),
		Description:  desc,
		Requirements: reqs,
		Location:     location,
		SalaryMin:    salMin,
		SalaryMax:    salMax,
		Status:       status,
	}

	if _, err := h.hrSvc.CreateJobOffer(ctx, actor.OrganizationID, j); err != nil {
		h.redirectWithNotice(w, r, redirectURL, "error", h.safeMessage(err, langOf(r)))
		return
	}

	h.redirectWithNotice(w, r, redirectURL, "success", "تم نشر الشاغر الوظيفي بنجاح.")
}

func (h *UIHandler) handleJobUpdateSubmit(w http.ResponseWriter, r *http.Request, redirectURL string) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect="+redirectURL, http.StatusSeeOther)
		return
	}

	jobID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || jobID <= 0 {
		h.redirectWithNotice(w, r, redirectURL, "error", "معرف وظيفة غير صالح.")
		return
	}

	if h.hrSvc == nil {
		h.redirectWithNotice(w, r, redirectURL, "error", "خدمة التوظيف غير متاحة حالياً.")
		return
	}

	_ = r.ParseForm()

	titleAr := strings.TrimSpace(r.PostFormValue("title_ar"))
	titleEn := strings.TrimSpace(r.PostFormValue("title_en"))
	if titleAr == "" {
		h.redirectWithNotice(w, r, redirectURL, "error", "المسمى الوظيفي بالعربية مطلوب.")
		return
	}
	if titleEn == "" {
		titleEn = titleAr
	}

	accessibleBranches := h.accessibleBranchesForActor(ctx, actor)
	branchIDStr := strings.TrimSpace(r.PostFormValue("branch_id"))
	var location string

	if len(accessibleBranches) > 0 {
		if branchIDStr == "" {
			h.redirectWithNotice(w, r, redirectURL, "error", "يرجى اختيار الفرع المخصص للشاغر الوظيفي.")
			return
		}
		branchID, err := strconv.ParseInt(branchIDStr, 10, 64)
		if err != nil || branchID <= 0 {
			h.redirectWithNotice(w, r, redirectURL, "error", "معرف الفرع المحدد غير صالح.")
			return
		}

		var selectedBranch *org.Branch
		for _, b := range accessibleBranches {
			if b != nil && b.ID == branchID {
				selectedBranch = b
				break
			}
		}
		if selectedBranch == nil {
			h.redirectWithNotice(w, r, redirectURL, "error", "الفرع المختار غير متاح أو لا تملك صلاحية النشر عليه.")
			return
		}

		location = selectedBranch.Name.Get(i18n.AR)
		if location == "" {
			location = selectedBranch.Name.Get(i18n.EN)
		}
		if location == "" {
			location = fmt.Sprintf("فرع #%d", selectedBranch.ID)
		}
	} else {
		location = strings.TrimSpace(r.PostFormValue("location"))
		if location == "" {
			location = "الفرع الرئيسي"
		}
	}

	desc := strings.TrimSpace(r.PostFormValue("description"))
	reqs := strings.TrimSpace(r.PostFormValue("requirements"))

	salMin, _ := money.Parse(strings.TrimSpace(r.PostFormValue("salary_min")))
	salMax, _ := money.Parse(strings.TrimSpace(r.PostFormValue("salary_max")))

	status := strings.TrimSpace(r.PostFormValue("status"))
	if status == "" {
		status = "published"
	}

	j := &hr.JobOffer{
		ID:           jobID,
		Title:        i18n.New(titleAr, titleEn),
		Description:  desc,
		Requirements: reqs,
		Location:     location,
		SalaryMin:    salMin,
		SalaryMax:    salMax,
		Status:       status,
	}

	if err := h.hrSvc.UpdateJobOffer(ctx, actor.OrganizationID, j); err != nil {
		h.redirectWithNotice(w, r, redirectURL, "error", h.safeMessage(err, langOf(r)))
		return
	}

	h.redirectWithNotice(w, r, redirectURL, "success", "تم تحديث بيانات وحالة الوظيفة بنجاح.")
}

func (h *UIHandler) handleJobToggleSubmit(w http.ResponseWriter, r *http.Request, redirectURL string) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect="+redirectURL, http.StatusSeeOther)
		return
	}

	jobID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || jobID <= 0 {
		h.redirectWithNotice(w, r, redirectURL, "error", "معرف وظيفة غير صالح.")
		return
	}

	if h.hrSvc != nil {
		if err := h.hrSvc.ToggleJobOfferStatus(ctx, actor.OrganizationID, jobID); err != nil {
			h.redirectWithNotice(w, r, redirectURL, "error", h.safeMessage(err, langOf(r)))
			return
		}
	}

	h.redirectWithNotice(w, r, redirectURL, "success", "تم تحديث حالة الوظيفة بنجاح.")
}

func (h *UIHandler) handleJobDeleteSubmit(w http.ResponseWriter, r *http.Request, redirectURL string) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect="+redirectURL, http.StatusSeeOther)
		return
	}

	jobID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || jobID <= 0 {
		h.redirectWithNotice(w, r, redirectURL, "error", "معرف وظيفة غير صالح.")
		return
	}

	if h.hrSvc != nil {
		if err := h.hrSvc.DeleteJobOffer(ctx, actor.OrganizationID, jobID); err != nil {
			h.redirectWithNotice(w, r, redirectURL, "error", h.safeMessage(err, langOf(r)))
			return
		}
	}

	h.redirectWithNotice(w, r, redirectURL, "success", "تم حذف الوظيفة بنجاح.")
}

func (h *UIHandler) handleJobApplicationsJSON(w http.ResponseWriter, r *http.Request) {
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

type acceptApplicantReq struct {
	BranchID   int64  `json:"branch_id"`
	RoleKey    string `json:"role_key"`
	JobTitle   string `json:"job_title"`
	BaseSalary string `json:"base_salary"`
	Notes      string `json:"notes"`
}

func (h *UIHandler) handleJobApplicationAccept(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	appID, err := strconv.ParseInt(chi.URLParam(r, "appId"), 10, 64)
	if err != nil || appID <= 0 {
		http.Error(w, `{"error":"invalid application id"}`, http.StatusBadRequest)
		return
	}

	var req acceptApplicantReq
	if strings.Contains(r.Header.Get("Content-Type"), "application/json") {
		_ = json.NewDecoder(r.Body).Decode(&req)
	} else {
		_ = r.ParseForm()
		req.BranchID, _ = strconv.ParseInt(r.PostFormValue("branch_id"), 10, 64)
		req.RoleKey = r.PostFormValue("role_key")
		req.JobTitle = r.PostFormValue("job_title")
		req.BaseSalary = r.PostFormValue("base_salary")
		req.Notes = r.PostFormValue("notes")
	}

	if req.RoleKey == "" {
		if actor.IsVendor() {
			req.RoleKey = "org_sales_rep"
		} else {
			req.RoleKey = "org_pharmacist"
		}
	}

	var branchPtr *int64
	if req.BranchID > 0 {
		branchPtr = &req.BranchID
	}

	sal, _ := money.Parse(req.BaseSalary)

	if h.hrSvc == nil {
		http.Error(w, `{"error":"hr service unavailable"}`, http.StatusInternalServerError)
		return
	}

	app, err := h.hrSvc.AcceptAndOnboardApplicant(ctx, hr.AcceptApplicantInput{
		ApplicationID:  appID,
		OrganizationID: actor.OrganizationID,
		BranchID:       branchPtr,
		RoleKey:        req.RoleKey,
		JobTitle:       req.JobTitle,
		BaseSalary:     sal,
		Notes:          req.Notes,
	})
	if err != nil {
		h.log.WarnContext(ctx, "failed to accept applicant", "app_id", appID, "error", err)
		http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusBadRequest)
		return
	}

	// Dispatch In-App Notification to the Job Seeker
	if app != nil && app.ApplicantUserID != nil && *app.ApplicantUserID > 0 && h.notifSvc != nil {
		orgName := "المنشأة"
		if h.orgSvc != nil {
			if o, err := h.orgSvc.GetOrganization(ctx, actor.OrganizationID); err == nil && o != nil {
				orgName = o.TradeName.Get(i18n.AR)
				if orgName == "" {
					orgName = o.LegalName
				}
			}
		}
		branchName := "الفرع الرئيسي"
		if app.BranchName != "" {
			branchName = app.BranchName
		}
		jobTitle := app.JobTitle
		if jobTitle == "" {
			jobTitle = "المعلنة"
		}

		_, notifErr := h.notifSvc.Send(ctx, notifications.SendInput{
			UserID:         *app.ApplicantUserID,
			OrganizationID: &actor.OrganizationID,
			Channel:        notifications.ChannelInApp,
			Recipient:      app.ApplicantEmail,
			Title:          "🎉 تهانينا! تم قبول طلبك للوظيفة",
			Body:           fmt.Sprintf("تم قبول طلب انضمامك لوظيفة \"%s\" لدى \"%s\" في فرع \"%s\". تم تعيينك وتفعيل لوحة التحكم لتبدأ مهامك مباشرة.", jobTitle, orgName, branchName),
		})
		if notifErr != nil {
			h.log.WarnContext(ctx, "failed to send acceptance notification to seeker", "user_id", *app.ApplicantUserID, "error", notifErr)
		}
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success":     true,
		"application": app,
	})
}

type rejectApplicantReq struct {
	Notes string `json:"notes"`
}

func (h *UIHandler) handleJobApplicationReject(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	appID, err := strconv.ParseInt(chi.URLParam(r, "appId"), 10, 64)
	if err != nil || appID <= 0 {
		http.Error(w, `{"error":"invalid application id"}`, http.StatusBadRequest)
		return
	}

	var req rejectApplicantReq
	if strings.Contains(r.Header.Get("Content-Type"), "application/json") {
		_ = json.NewDecoder(r.Body).Decode(&req)
	} else {
		_ = r.ParseForm()
		req.Notes = r.PostFormValue("notes")
	}

	if h.hrSvc == nil {
		http.Error(w, `{"error":"hr service unavailable"}`, http.StatusInternalServerError)
		return
	}

	app, err := h.hrSvc.RejectApplicant(ctx, actor.OrganizationID, appID, req.Notes)
	if err != nil {
		h.log.WarnContext(ctx, "failed to reject applicant", "app_id", appID, "error", err)
		http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusBadRequest)
		return
	}

	// Dispatch In-App Notification to Job Seeker
	if app != nil && app.ApplicantUserID != nil && *app.ApplicantUserID > 0 && h.notifSvc != nil {
		orgName := "المنشأة"
		if h.orgSvc != nil {
			if o, err := h.orgSvc.GetOrganization(ctx, actor.OrganizationID); err == nil && o != nil {
				orgName = o.TradeName.Get(i18n.AR)
				if orgName == "" {
					orgName = o.LegalName
				}
			}
		}
		jobTitle := app.JobTitle
		if jobTitle == "" {
			jobTitle = "المعلنة"
		}

		_, _ = h.notifSvc.Send(ctx, notifications.SendInput{
			UserID:         *app.ApplicantUserID,
			OrganizationID: &actor.OrganizationID,
			Channel:        notifications.ChannelInApp,
			Recipient:      app.ApplicantEmail,
			Title:          "تحديث بخصوص طلب التوظيف",
			Body:           fmt.Sprintf("نعتذر عن عدم قبول طلبك لوظيفة \"%s\" لدى \"%s\". نتمنى لك كامل التوفيق في الفرص القادمة.", jobTitle, orgName),
		})
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success":     true,
		"application": app,
	})
}
