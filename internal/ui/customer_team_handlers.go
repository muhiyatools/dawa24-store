package ui

import (
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/pagination"
	"github.com/muhiya/dawa24-store/internal/shared/productmatch"
	"github.com/muhiya/dawa24-store/internal/shared/sheet"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// CustomerTeamPage lists the pharmacy's employees and the role each holds.
func (h *UIHandler) CustomerTeamPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/customer/team", http.StatusSeeOther)
		return
	}

	page := pagination.PageNumber(r)
	limit := pagination.RowsPerPage(r)
	offset := (page - 1) * limit

	var totalCount int
	employees, total, err := h.orgSvc.ListEmployeesWithTotal(ctx, actor.OrganizationID, limit, offset)
	if err != nil {
		h.log.ErrorContext(ctx, "list pharmacy employees",
			"error", err, "organization_id", actor.OrganizationID)
	} else {
		totalCount = total
	}

	view := pages.TenantTeamView{
		Title:      i18n.T(lang, "customer.team.title"),
		RolesPath:  "/customer/roles",
		ActionBase: "/customer/employees",
		CanCreate:  actor.Can("pharmacy.team.create"),
		CanUpdate:  actor.Can("pharmacy.team.update"),
		CanDelete:  actor.Can("pharmacy.team.delete"),
		CanAssign:  actor.Can("pharmacy.role.assign"),
		NoticeKind: r.URL.Query().Get("notice"),
		Notice:     r.URL.Query().Get("msg"),
		Page:       page,
		PerPage:    limit,
		TotalCount: totalCount,
	}

	roles, err := h.orgSvc.ListRoles(ctx, actor.OrganizationID)
	if err != nil {
		h.log.ErrorContext(ctx, "list company roles for team page",
			"error", err, "organization_id", actor.OrganizationID)
	}
	// roleByKey lets a member carrying only a legacy role_key still show the
	// company's own role for it, rather than an empty cell.
	roleByKey := map[string]int64{}
	roleNameByID := map[int64]string{}
	for _, role := range roles {
		name := role.Name.Get(i18n.ParseLang(lang))
		view.Roles = append(view.Roles, pages.TenantRoleOption{ID: role.ID, Name: name})
		roleByKey[role.Key] = role.ID
		roleNameByID[role.ID] = name
	}

	for _, emp := range employees {
		if emp == nil || emp.Member == nil {
			continue
		}
		roleID := int64(0)
		if emp.Member.OrgRoleID != nil {
			roleID = *emp.Member.OrgRoleID
		} else if id, ok := roleByKey[emp.Member.RoleKey]; ok {
			roleID = id
		}
		name := emp.UserName
		if name == "" {
			name = emp.UserEmail
		}
		view.Members = append(view.Members, pages.TenantTeamMember{
			ID:           emp.Member.ID,
			UserID:       emp.Member.UserID,
			Name:         name,
			Email:        emp.UserEmail,
			Phone:        emp.UserPhone,
			JobTitle:     emp.Member.JobTitle,
			EmployeeCode: emp.Member.EmployeeCode,
			BranchName:   emp.BranchName,
			RoleID:       roleID,
			RoleName:     roleNameByID[roleID],
			IsActive:     emp.Member.IsActive,
			JoinedAt:     emp.Member.CreatedAt.Format("2006-01-02"),
		})
	}

	if branches, err := h.orgSvc.ListBranches(ctx, actor.OrganizationID); err == nil {
		for _, b := range branches {
			name := b.Name.Get(i18n.AR)
			if name == "" {
				name = b.Name.Get(i18n.EN)
			}
			view.Branches = append(view.Branches, &pages.BranchOption{ID: b.ID, Name: name})
		}
	}

	h.renderPage(ctx, w, "render pharmacy team page", pages.TenantTeamPage(view, lang, dir))
}

// CustomerTeamImportPage renders bulk employee spreadsheet upload page for pharmacies.
func (h *UIHandler) CustomerTeamImportPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/customer/team/import", http.StatusSeeOther)
		return
	}

	sessions := globalTeamImportSessionStore.ListSessions(actor.OrganizationID)
	noticeType := r.URL.Query().Get("notice_type")
	noticeMsg := r.URL.Query().Get("notice")
	if noticeMsg == "" {
		noticeMsg = r.URL.Query().Get("msg")
	}

	view := pages.TeamImportView{
		Audience:   "customer",
		BaseURL:    "/customer/team",
		ImportURL:  "/customer/team/import",
		Sessions:   sessions,
		NoticeType: noticeType,
		NoticeMsg:  noticeMsg,
	}

	h.renderPage(ctx, w, "render customer team import", pages.TeamImportPage(view, lang, dir))
}

// CustomerTeamImportUploadSubmit handles file upload and creates new employee import session for pharmacy.
func (h *UIHandler) CustomerTeamImportUploadSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/customer/team/import", http.StatusSeeOther)
		return
	}

	if err := r.ParseMultipartForm(32 << 20); err != nil {
		h.redirectWithNotice(w, r, "/customer/team/import", "error", i18n.T(langOf(r), "customer.saving.import.file_too_large_short"))
		return
	}

	file, fileHeader, err := r.FormFile("file")
	if err != nil {
		h.redirectWithNotice(w, r, "/customer/team/import", "error", i18n.T(langOf(r), "customer.saving.import.select_file"))
		return
	}
	defer file.Close()

	if !SupportedUploadName(fileHeader.Filename) {
		h.redirectWithNotice(w, r, "/customer/team/import", "error", unsupportedUploadMsg(langOf(r)))
		return
	}

	fileBytes, err := io.ReadAll(file)
	if err != nil || len(fileBytes) == 0 {
		h.redirectWithNotice(w, r, "/customer/team/import", "error", i18n.T(langOf(r), "customer.saving.import.file_empty"))
		return
	}

	rawRows, err := sheet.ReadRows(fileBytes, fileHeader.Filename)
	if err != nil || len(rawRows) < 2 {
		h.log.WarnContext(ctx, "failed to parse spreadsheet", "error", err, "filename", fileHeader.Filename)
		h.redirectWithNotice(w, r, "/customer/team/import", "error", i18n.T(langOf(r), "customer.saving.import.parse_error_short"))
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
		h.redirectWithNotice(w, r, "/customer/team/import", "error", i18n.T(langOf(r), "customer.team.import.no_employee_rows"))
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

	session := globalTeamImportSessionStore.NewSession(actor.OrganizationID, actor.UserID, "customer", fileHeader.Filename, len(dataRows))
	session.Phase = pages.TeamPhaseMapping
	session.Headers = headers
	session.SampleRows = sampleRows
	session.RawDataRows = dataRows
	session.DetectedCols = detectedCols
	session.ExcelRoles = excelRoles
	session.CompanyRoles = companyRoles
	session.Branches = branches
	session.DefaultRoleID = defaultRoleID

	http.Redirect(w, r, fmt.Sprintf("/customer/team/import/%s", session.ID), http.StatusSeeOther)
}

// CustomerTeamImportSampleDownload downloads sample spreadsheet template for pharmacy team.
func (h *UIHandler) CustomerTeamImportSampleDownload(w http.ResponseWriter, r *http.Request) {
	bytes, err := GenerateTeamSampleExcel("customer")
	if err != nil {
		http.Error(w, "Failed to generate sample spreadsheet", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", `attachment; filename="dawa24_pharmacy_team_sample.xlsx"`)
	w.Header().Set("Content-Length", strconv.Itoa(len(bytes)))
	_, _ = w.Write(bytes)
}

// CustomerTeamImportSessionPage renders current session stage for pharmacy.
func (h *UIHandler) CustomerTeamImportSessionPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/customer/team/import", http.StatusSeeOther)
		return
	}

	sessionID := chi.URLParam(r, "id")
	session, ok := globalTeamImportSessionStore.GetSession(sessionID, actor.OrganizationID)
	if !ok {
		h.redirectWithNotice(w, r, "/customer/team/import", "error", i18n.T(langOf(r), "customer.saving.import.session_not_found"))
		return
	}

	noticeType := r.URL.Query().Get("notice_type")
	noticeMsg := r.URL.Query().Get("notice")
	if noticeMsg == "" {
		noticeMsg = r.URL.Query().Get("msg")
	}

	view := pages.TeamImportView{
		Audience:   "customer",
		BaseURL:    "/customer/team",
		ImportURL:  "/customer/team/import",
		Session:    session,
		NoticeType: noticeType,
		NoticeMsg:  noticeMsg,
	}

	h.renderPage(ctx, w, "render customer team import session", pages.TeamImportPage(view, lang, dir))
}
