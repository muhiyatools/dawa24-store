package ui

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/identity"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/productmatch"
	"github.com/muhiya/dawa24-store/internal/shared/sheet"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// VendorTeamImportPage renders bulk employee spreadsheet upload page.
func (h *UIHandler) VendorTeamImportPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/team/import", http.StatusSeeOther)
		return
	}

	sessions := globalTeamImportSessionStore.ListSessions(actor.OrganizationID)
	noticeType := r.URL.Query().Get("notice_type")
	noticeMsg := r.URL.Query().Get("notice")
	if noticeMsg == "" {
		noticeMsg = r.URL.Query().Get("msg")
	}

	view := pages.TeamImportView{
		Audience:   "vendor",
		BaseURL:    "/vendor/team",
		ImportURL:  "/vendor/team/import",
		Sessions:   sessions,
		NoticeType: noticeType,
		NoticeMsg:  noticeMsg,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.TeamImportPage(view, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render vendor team import", "error", err)
	}
}

// VendorTeamImportUploadSubmit handles file upload and creates new employee import session for vendor.
func (h *UIHandler) VendorTeamImportUploadSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/team/import", http.StatusSeeOther)
		return
	}

	if err := r.ParseMultipartForm(32 << 20); err != nil {
		h.redirectWithNotice(w, r, "/vendor/team/import", "error", "الملف المرفوع كبير جداً أو غير صالح.")
		return
	}

	file, fileHeader, err := r.FormFile("file")
	if err != nil {
		h.redirectWithNotice(w, r, "/vendor/team/import", "error", "يرجى اختيار ملف Excel أو CSV صالح.")
		return
	}
	defer file.Close()

	if !SupportedUploadName(fileHeader.Filename) {
		h.redirectWithNotice(w, r, "/vendor/team/import", "error", unsupportedUploadMessage)
		return
	}

	fileBytes, err := io.ReadAll(file)
	if err != nil || len(fileBytes) == 0 {
		h.redirectWithNotice(w, r, "/vendor/team/import", "error", "الملف فارغ أو تعذر قراءته.")
		return
	}

	rawRows, err := sheet.ReadRows(fileBytes, fileHeader.Filename)
	if err != nil || len(rawRows) < 2 {
		h.log.WarnContext(ctx, "failed to parse spreadsheet", "error", err, "filename", fileHeader.Filename)
		h.redirectWithNotice(w, r, "/vendor/team/import", "error", "تعذر قراءة ملف البيانات المرفوع أو أن الملف لا يحتوي على صفوف بيانات.")
		return
	}

	layout, _ := productmatch.AnalyzeLayout(rawRows)
	var headers []string
	dataStart := 1
	if layout.HeaderRow >= 0 && layout.HeaderRow < len(rawRows) {
		headers = layout.Headers
		dataStart = layout.FirstDataRow
	} else if len(rawRows) > 0 {
		headers = rawRows[0]
		dataStart = 1
	}

	if dataStart > len(rawRows) {
		dataStart = len(rawRows)
	}

	dataRows := rawRows[dataStart:]
	if len(dataRows) == 0 {
		h.redirectWithNotice(w, r, "/vendor/team/import", "error", "لم يتم العثور على أي صفوف بيانات للموظفين بعد صف العناوين.")
		return
	}

	var sampleRows [][]string
	limit := 5
	if len(dataRows) < limit {
		limit = len(dataRows)
	}
	sampleRows = dataRows[:limit]

	detectedCols := detectTeamColumns(headers, sampleRows)

	// Fetch company roles
	h.ensureCompanyRoles(ctx, actor.OrganizationID, actor.OrgType)
	var companyRoles []pages.TeamRoleOption
	if h.orgSvc != nil {
		if roles, err := h.orgSvc.ListRoles(ctx, actor.OrganizationID); err == nil {
			for _, role := range roles {
				companyRoles = append(companyRoles, pages.TeamRoleOption{
					ID:      role.ID,
					Key:     role.Key,
					Name:    role.Name.Get(i18n.AR),
					IsOwner: role.IsOwner,
				})
			}
		}
	}

	// Fetch branches
	var branches []pages.TeamBranchOption
	if h.orgSvc != nil {
		if branchList, err := h.orgSvc.ListBranches(ctx, actor.OrganizationID); err == nil {
			for _, b := range branchList {
				branches = append(branches, pages.TeamBranchOption{
					ID:   b.ID,
					Name: b.Name.Get(i18n.AR),
					Code: b.Code,
				})
			}
		}
	}

	excelRoles := extractUniqueExcelRoles(dataRows, detectedCols.RoleCol, companyRoles)

	var defaultRoleID int64
	for _, cr := range companyRoles {
		if !cr.IsOwner && cr.Key != "org_owner" {
			defaultRoleID = cr.ID
			break
		}
	}
	if defaultRoleID == 0 && len(companyRoles) > 0 {
		defaultRoleID = companyRoles[0].ID
	}

	session := globalTeamImportSessionStore.NewSession(actor.OrganizationID, actor.UserID, "vendor", fileHeader.Filename, len(dataRows))
	session.Phase = pages.TeamPhaseMapping
	session.Headers = headers
	session.SampleRows = sampleRows
	session.RawDataRows = dataRows
	session.DetectedCols = detectedCols
	session.ExcelRoles = excelRoles
	session.CompanyRoles = companyRoles
	session.Branches = branches
	session.DefaultRoleID = defaultRoleID

	http.Redirect(w, r, fmt.Sprintf("/vendor/team/import/%s", session.ID), http.StatusSeeOther)
}

// VendorTeamImportSampleDownload downloads the sample spreadsheet template for vendor team.
func (h *UIHandler) VendorTeamImportSampleDownload(w http.ResponseWriter, r *http.Request) {
	bytes, err := GenerateTeamSampleExcel("vendor")
	if err != nil {
		http.Error(w, "Failed to generate sample spreadsheet", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", `attachment; filename="dawa24_vendor_team_sample.xlsx"`)
	w.Header().Set("Content-Length", strconv.Itoa(len(bytes)))
	_, _ = w.Write(bytes)
}

// VendorTeamImportSessionPage renders current session stage for vendor.
func (h *UIHandler) VendorTeamImportSessionPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/team/import", http.StatusSeeOther)
		return
	}

	sessionID := chi.URLParam(r, "id")
	session, ok := globalTeamImportSessionStore.GetSession(sessionID, actor.OrganizationID)
	if !ok {
		h.redirectWithNotice(w, r, "/vendor/team/import", "error", "جلسة الاستيراد غير موجودة أو انتهت صلاحيتها.")
		return
	}

	noticeType := r.URL.Query().Get("notice_type")
	noticeMsg := r.URL.Query().Get("notice")
	if noticeMsg == "" {
		noticeMsg = r.URL.Query().Get("msg")
	}

	view := pages.TeamImportView{
		Audience:   "vendor",
		BaseURL:    "/vendor/team",
		ImportURL:  "/vendor/team/import",
		Session:    session,
		NoticeType: noticeType,
		NoticeMsg:  noticeMsg,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.TeamImportPage(view, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render vendor team import session", "error", err)
	}
}

// VendorTeamImportMapSubmit saves user column mappings and role mappings, then builds review rows.
func (h *UIHandler) VendorTeamImportMapSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	sessionID := chi.URLParam(r, "id")
	session, ok := globalTeamImportSessionStore.GetSession(sessionID, actor.OrganizationID)
	if !ok {
		h.redirectWithNotice(w, r, "/vendor/team/import", "error", "جلسة الاستيراد غير موجودة أو انتهت صلاحيتها.")
		return
	}

	if err := r.ParseForm(); err != nil {
		h.redirectWithNotice(w, r, fmt.Sprintf("/vendor/team/import/%s", sessionID), "error", "البيانات المرسلة غير صالحة.")
		return
	}

	parseInt := func(name string, defVal int) int {
		if v, err := strconv.Atoi(r.PostFormValue(name)); err == nil {
			return v
		}
		return defVal
	}

	session.DetectedCols = pages.TeamDetectedCols{
		NameCol:     parseInt("col_name", -1),
		EmailCol:    parseInt("col_email", -1),
		PhoneCol:    parseInt("col_phone", -1),
		RoleCol:     parseInt("col_role", -1),
		JobTitleCol: parseInt("col_job_title", -1),
		BranchCol:   parseInt("col_branch", -1),
		CodeCol:     parseInt("col_code", -1),
	}

	if session.DetectedCols.NameCol == -1 || session.DetectedCols.EmailCol == -1 {
		h.redirectWithNotice(w, r, fmt.Sprintf("/vendor/team/import/%s", sessionID), "error", "يرجى تحديد عمود اسم الموظف وعمود البريد الإلكتروني على الأقل.")
		return
	}

	if defRoleID, err := strconv.ParseInt(r.PostFormValue("default_role_id"), 10, 64); err == nil && defRoleID > 0 {
		session.DefaultRoleID = defRoleID
	}

	roleMap := make(map[string]int64)
	for _, er := range session.ExcelRoles {
		formKey := "role_map_" + er.RawName
		if val := r.PostFormValue(formKey); val != "" {
			if rID, err := strconv.ParseInt(val, 10, 64); err == nil && rID > 0 {
				roleMap[er.RawName] = rID
				er.MatchedRoleID = rID
			}
		}
	}

	parsedRows := parseAndValidateTeamRows(
		session.RawDataRows,
		session.DetectedCols,
		roleMap,
		session.DefaultRoleID,
		session.CompanyRoles,
		session.Branches,
	)

	// Check existing user accounts
	if h.idSvc != nil {
		sysCtx := database.AsSystem(ctx)
		for _, row := range parsedRows {
			if row.IsValid && row.Email != "" {
				if existingUser, err := h.idSvc.GetUserByEmail(sysCtx, row.Email); err == nil && existingUser != nil {
					row.IsExistingUser = true
				}
			}
		}
	}

	session.Rows = parsedRows
	session.Phase = pages.TeamPhaseReview
	session.UpdatedAt = session.UpdatedAt

	http.Redirect(w, r, fmt.Sprintf("/vendor/team/import/%s", sessionID), http.StatusSeeOther)
}

// VendorTeamImportCommitSubmit creates user accounts and links employees to vendor org.
func (h *UIHandler) VendorTeamImportCommitSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	sessionID := chi.URLParam(r, "id")
	session, ok := globalTeamImportSessionStore.GetSession(sessionID, actor.OrganizationID)
	if !ok || session.Phase != pages.TeamPhaseReview {
		h.redirectWithNotice(w, r, "/vendor/team/import", "error", "جلسة الاستيراد غير صالحة أو غير جاهزة للتأكيد.")
		return
	}

	if h.idSvc == nil || h.orgSvc == nil {
		h.redirectWithNotice(w, r, fmt.Sprintf("/vendor/team/import/%s", sessionID), "error", "خدمة المنظومة غير متاحة حالياً.")
		return
	}

	sysCtx := database.AsSystem(ctx)
	importedCount := 0
	skippedCount := 0
	failedCount := 0

	for _, row := range session.Rows {
		if !row.IsValid {
			skippedCount++
			row.ImportStatus = "skipped"
			continue
		}

		// 1. Check or register user
		var targetUserID int64
		existingUser, err := h.idSvc.GetUserByEmail(sysCtx, row.Email)
		if err == nil && existingUser != nil {
			targetUserID = existingUser.ID
		} else {
			randBytes := make([]byte, 6)
			_, _ = rand.Read(randBytes)
			defaultPass := fmt.Sprintf("Dawa24!%s", hex.EncodeToString(randBytes))

			newUser, _, regErr := h.idSvc.Register(sysCtx, identity.RegisterInput{
				Email:    row.Email,
				Password: defaultPass,
				NameAr:   row.RawName,
				NameEn:   row.RawName,
				Phone:    row.Phone,
				Role:     "user",
			})
			if regErr != nil {
				h.log.ErrorContext(ctx, "failed to register imported employee user", "email", row.Email, "error", regErr)
				failedCount++
				row.ImportStatus = "failed"
				continue
			}
			targetUserID = newUser.ID
		}

		// 2. Add member to organization
		roleKey := row.AssignedRoleKey
		if roleKey == "" {
			roleKey = "org_employee"
		}

		member := &org.Member{
			OrganizationID: actor.OrganizationID,
			UserID:         targetUserID,
			BranchID:       row.AssignedBranchID,
			RoleID:         row.AssignedRoleID,
			RoleKey:        roleKey,
			JobTitle:       row.JobTitle,
			EmployeeCode:   row.EmployeeCode,
			IsActive:       true,
		}
		if row.AssignedRoleID > 0 {
			member.OrgRoleID = &row.AssignedRoleID
		}

		if err := h.orgSvc.AddMemberDirect(sysCtx, member); err != nil {
			h.log.ErrorContext(ctx, "failed to link imported employee to org", "org_id", actor.OrganizationID, "user_id", targetUserID, "error", err)
			failedCount++
			row.ImportStatus = "failed"
			continue
		}

		importedCount++
		row.ImportStatus = "imported"
	}

	session.ImportedCount = importedCount
	session.SkippedCount = skippedCount
	session.FailedCount = failedCount
	session.Phase = pages.TeamPhaseCompleted

	http.Redirect(w, r, fmt.Sprintf("/vendor/team/import/%s", sessionID), http.StatusSeeOther)
}

// VendorTeamImportCancelSubmit cancels and deletes an import session.
func (h *UIHandler) VendorTeamImportCancelSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	sessionID := chi.URLParam(r, "id")
	globalTeamImportSessionStore.DeleteSession(sessionID, actor.OrganizationID)
	h.redirectWithNotice(w, r, "/vendor/team/import", "info", "تم إلغاء جلسة الاستيراد.")
}

// VendorTeamFastAddPage renders fast add form for single employee account.
func (h *UIHandler) VendorTeamFastAddPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/team/fast-add", http.StatusSeeOther)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.VendorTeamFastAddPage(lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render vendor team fast add", "error", err)
	}
}

// VendorTeamUserDetailPage renders single employee profile and assigned permissions.
func (h *UIHandler) VendorTeamUserDetailPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	idStr := chi.URLParam(r, "id")
	empID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || empID <= 0 {
		http.Redirect(w, r, "/settings/employees", http.StatusSeeOther)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.VendorTeamUserDetailPage(empID, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render vendor team user detail", "error", err)
	}
}

// VendorTeamUserInfoPage renders employee audit information.
func (h *UIHandler) VendorTeamUserInfoPage(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	http.Redirect(w, r, fmt.Sprintf("/vendor/team/%s", idStr), http.StatusSeeOther)
}
