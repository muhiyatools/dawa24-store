package ui

import (
	"time"

	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/filesecurity"
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

	h.renderPage(ctx, w, "render vendor team import", pages.TeamImportPage(view, lang, dir))
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
		h.redirectWithNotice(w, r, "/vendor/team/import", "error", i18n.T(langOf(r), "customer.saving.import.file_too_large_short"))
		return
	}

	file, fileHeader, err := r.FormFile("file")
	if err != nil {
		h.redirectWithNotice(w, r, "/vendor/team/import", "error", i18n.T(langOf(r), "customer.saving.import.select_file"))
		return
	}
	defer file.Close()

	if !SupportedUploadName(fileHeader.Filename) {
		h.redirectWithNotice(w, r, "/vendor/team/import", "error", unsupportedUploadMsg(langOf(r)))
		return
	}

	fileBytes, err := io.ReadAll(file)
	if err != nil || len(fileBytes) == 0 {
		h.redirectWithNotice(w, r, "/vendor/team/import", "error", i18n.T(langOf(r), "customer.saving.import.file_empty"))
		return
	}

	if err := filesecurity.ValidateSpreadsheetSecurity(fileBytes, fileHeader.Filename, filesecurity.WithAllowEmails(true)); err != nil {
		h.redirectWithNotice(w, r, "/vendor/team/import", "error", filesecurity.SecurityErrorMessage)
		return
	}

	rawRows, err := sheet.ReadRows(fileBytes, fileHeader.Filename, sheet.WithAllowEmails(true))
	if err != nil || len(rawRows) < 2 {
		h.log.WarnContext(ctx, "failed to parse spreadsheet", "error", err, "filename", fileHeader.Filename)
		msg := i18n.T(langOf(r), "customer.saving.import.parse_error_short")
		if err != nil && strings.Contains(err.Error(), filesecurity.SecurityErrorMessage) {
			msg = filesecurity.SecurityErrorMessage
		}
		h.redirectWithNotice(w, r, "/vendor/team/import", "error", msg)
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
		h.redirectWithNotice(w, r, "/vendor/team/import", "error", i18n.T(langOf(r), "customer.team.import.no_employee_rows"))
		return
	}

	var sampleRows [][]string
	limit := 5
	if len(dataRows) < limit {
		limit = len(dataRows)
	}
	sampleRows = dataRows[:limit]

	detectedCols := DetectTeamColumns(headers, sampleRows)

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
		h.redirectWithNotice(w, r, "/vendor/team/import", "error", i18n.T(langOf(r), "customer.saving.import.session_not_found"))
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

	h.renderPage(ctx, w, "render vendor team import session", pages.TeamImportPage(view, lang, dir))
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
		h.redirectWithNotice(w, r, "/vendor/team/import", "error", i18n.T(langOf(r), "customer.saving.import.session_not_found"))
		return
	}

	if err := r.ParseForm(); err != nil {
		h.redirectWithNotice(w, r, fmt.Sprintf("/vendor/team/import/%s", sessionID), "error", i18n.T(langOf(r), "customer.team.import.data_invalid"))
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
		h.redirectWithNotice(w, r, fmt.Sprintf("/vendor/team/import/%s", sessionID), "error", i18n.T(langOf(r), "customer.team.import.columns_required"))
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

	parsedRows := ParseAndValidateTeamRows(
		session.RawDataRows,
		session.DetectedCols,
		roleMap,
		session.DefaultRoleID,
		session.CompanyRoles,
		session.Branches,
		langOf(r),
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
	session.UpdatedAt = time.Now()

	http.Redirect(w, r, fmt.Sprintf("/vendor/team/import/%s", sessionID), http.StatusSeeOther)
}
