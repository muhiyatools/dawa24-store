package ui

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/xuri/excelize/v2"

	"github.com/muhiya/dawa24-store/internal/modules/identity"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/filesecurity"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/productmatch"
	"github.com/muhiya/dawa24-store/internal/shared/sheet"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

type TeamImportPhase = pages.TeamImportPhase

const (
	TeamPhaseUpload    = pages.TeamPhaseUpload
	TeamPhaseMapping   = pages.TeamPhaseMapping
	TeamPhaseReview    = pages.TeamPhaseReview
	TeamPhaseCompleted = pages.TeamPhaseCompleted
)

type TeamDetectedCols = pages.TeamDetectedCols
type ExcelRoleInfo = pages.ExcelRoleInfo
type TeamImportRow = pages.TeamImportRow
type TeamRoleOption = pages.TeamRoleOption
type TeamBranchOption = pages.TeamBranchOption
type TeamImportSession = pages.TeamImportSession
type TeamImportView = pages.TeamImportView

// Global in-memory session store for team imports.
var globalTeamImportSessionStore = NewTeamImportSessionStore()

type TeamImportSessionStore struct {
	mu       sync.RWMutex
	sessions map[string]*TeamImportSession
}

func NewTeamImportSessionStore() *TeamImportSessionStore {
	store := &TeamImportSessionStore{
		sessions: make(map[string]*TeamImportSession),
	}
	// Periodic cleanup of sessions older than 4 hours.
	go func() {
		for {
			time.Sleep(30 * time.Minute)
			store.cleanup(4 * time.Hour)
		}
	}()
	return store
}

func (s *TeamImportSessionStore) cleanup(maxAge time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cutoff := time.Now().Add(-maxAge)
	for id, sess := range s.sessions {
		if sess.UpdatedAt.Before(cutoff) {
			delete(s.sessions, id)
		}
	}
}

func (s *TeamImportSessionStore) NewSession(orgID, userID int64, orgType, filename string, totalRows int) *TeamImportSession {
	s.mu.Lock()
	defer s.mu.Unlock()

	randBytes := make([]byte, 12)
	_, _ = rand.Read(randBytes)
	sessionID := hex.EncodeToString(randBytes)

	now := time.Now()
	sess := &TeamImportSession{
		ID:             sessionID,
		OrganizationID: orgID,
		OrgType:        orgType,
		UserID:         userID,
		Filename:       filename,
		Phase:          TeamPhaseUpload,
		TotalRows:      totalRows,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	s.sessions[sessionID] = sess
	return sess
}

func (s *TeamImportSessionStore) GetSession(sessionID string, orgID int64) (*TeamImportSession, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sess, ok := s.sessions[sessionID]
	if !ok || sess.OrganizationID != orgID {
		return nil, false
	}
	return sess, true
}

func (s *TeamImportSessionStore) ListSessions(orgID int64) []*TeamImportSession {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var list []*TeamImportSession
	for _, sess := range s.sessions {
		if sess.OrganizationID == orgID {
			list = append(list, sess)
		}
	}
	return list
}

func (s *TeamImportSessionStore) DeleteSession(sessionID string, orgID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sess, ok := s.sessions[sessionID]; ok && sess.OrganizationID == orgID {
		delete(s.sessions, sessionID)
	}
}

// normalizeArabicText simplifies Arabic text for fuzzy matching.
func normalizeArabicText(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	replacements := map[rune]rune{
		'أ': 'ا', 'إ': 'ا', 'آ': 'ا', 'ٱ': 'ا',
		'ة': 'ه', 'ى': 'ي', 'ؤ': 'و', 'ئ': 'ي',
		'ـ': ' ',
	}
	var sb strings.Builder
	for _, r := range s {
		if sub, ok := replacements[r]; ok {
			sb.WriteRune(sub)
		} else {
			sb.WriteRune(r)
		}
	}
	return strings.Join(strings.Fields(sb.String()), " ")
}

// DetectTeamColumns analyzes headers and sample rows to auto-detect employee columns.
func DetectTeamColumns(headers []string, sampleRows [][]string) TeamDetectedCols {
	cols := TeamDetectedCols{
		NameCol:     -1,
		EmailCol:    -1,
		PhoneCol:    -1,
		RoleCol:     -1,
		JobTitleCol: -1,
		BranchCol:   -1,
		CodeCol:     -1,
		NotesCol:    -1,
	}

	nameKeywords := []string{i18n.TDefault("w4_ui.s_105_105"), i18n.TDefault("w4_ui.s_106_106"), i18n.TDefault("w4_ui.s_107_107"), i18n.TDefault("w4_ui.s_108_108"), i18n.TDefault("w4_ui.s_109_109"), i18n.TDefault("w4_ui.s_110_110"), i18n.TDefault("w4_ui.s_30_30"), "employee name", "staff name", "full name", "name", "username"}
	emailKeywords := []string{i18n.T("ar", "excel.col.email"), i18n.TDefault("w4_ui.s_111_111"), i18n.TDefault("w4_ui.s_112_112"), i18n.TDefault("w4_ui.s_113_113"), i18n.TDefault("w4_ui.s_114_114"), i18n.TDefault("w4_ui.s_115_115"), "email", "e-mail", "mail"}
	phoneKeywords := []string{i18n.T("ar", "excel.col.phone"), i18n.TDefault("w4_ui.s_116_116"), i18n.TDefault("w4_ui.s_117_117"), i18n.TDefault("w4_ui.s_118_118"), i18n.TDefault("w4_ui.s_119_119"), i18n.TDefault("w4_ui.s_120_120"), i18n.TDefault("w4_ui.s_121_121"), i18n.TDefault("w4_ui.s_122_122"), i18n.TDefault("w4_ui.s_123_123"), "phone", "mobile", "tel", "cellphone"}
	roleKeywords := []string{i18n.TDefault("w4_ui.s_124_124"), i18n.TDefault("w4_ui.s_125_125"), i18n.TDefault("w4_ui.s_126_126"), i18n.TDefault("w4_ui.s_127_127"), i18n.TDefault("w4_ui.s_128_128"), i18n.TDefault("w4_ui.s_129_129"), "role", "roles", "permission", "user role"}
	jobTitleKeywords := []string{i18n.T("ar", "excel.col.job_title"), i18n.TDefault("w4_ui.s_130_130"), i18n.TDefault("w4_ui.s_131_131"), i18n.TDefault("w4_ui.s_132_132"), "job title", "position", "title", "designation"}
	branchKeywords := []string{i18n.TDefault("w4_ui.s_133_133"), i18n.TDefault("w4_ui.s_134_134"), i18n.TDefault("w4_ui.s_135_135"), i18n.TDefault("w4_ui.s_136_136"), i18n.TDefault("w4_ui.s_137_137"), i18n.TDefault("w4_ui.s_138_138"), "branch", "warehouse", "store", "location"}
	codeKeywords := []string{i18n.T("ar", "excel.col.emp_code"), i18n.TDefault("w4_ui.s_139_139"), i18n.TDefault("w4_ui.s_2_2"), i18n.TDefault("w4_ui.s_140_140"), i18n.TDefault("w4_ui.s_141_141"), i18n.TDefault("w4_ui.s_142_142"), "employee code", "emp code", "code", "staff id", "employee id", "emp id"}
	notesKeywords := []string{i18n.TDefault("w4_ui.s_56_56"), i18n.TDefault("w4_ui.s_143_143"), i18n.TDefault("w4_ui.s_144_144"), i18n.TDefault("w4_ui.s_145_145"), "notes", "remarks", "national id"}

	matchesHeader := func(header string, keywords []string) bool {
		normH := normalizeArabicText(header)
		for _, kw := range keywords {
			normKW := normalizeArabicText(kw)
			if normH == normKW || strings.Contains(normH, normKW) {
				return true
			}
		}
		return false
	}

	for i, h := range headers {
		hTrim := strings.TrimSpace(h)
		if hTrim == "" {
			continue
		}

		if cols.EmailCol == -1 && matchesHeader(hTrim, emailKeywords) {
			cols.EmailCol = i
			continue
		}
		if cols.PhoneCol == -1 && matchesHeader(hTrim, phoneKeywords) {
			cols.PhoneCol = i
			continue
		}
		if cols.NameCol == -1 && matchesHeader(hTrim, nameKeywords) {
			cols.NameCol = i
			continue
		}
		if cols.RoleCol == -1 && matchesHeader(hTrim, roleKeywords) {
			cols.RoleCol = i
			continue
		}
		if cols.JobTitleCol == -1 && matchesHeader(hTrim, jobTitleKeywords) {
			cols.JobTitleCol = i
			continue
		}
		if cols.BranchCol == -1 && matchesHeader(hTrim, branchKeywords) {
			cols.BranchCol = i
			continue
		}
		if cols.CodeCol == -1 && matchesHeader(hTrim, codeKeywords) {
			cols.CodeCol = i
			continue
		}
		if cols.NotesCol == -1 && matchesHeader(hTrim, notesKeywords) {
			cols.NotesCol = i
			continue
		}
	}

	// Content inspection fallback for sample rows if email or phone are still missing
	if cols.EmailCol == -1 && len(sampleRows) > 0 {
		for colIdx := range headers {
			emailHits := 0
			for _, row := range sampleRows {
				if colIdx < len(row) && strings.Contains(row[colIdx], "@") && strings.Contains(row[colIdx], ".") {
					emailHits++
				}
			}
			if emailHits > 0 {
				cols.EmailCol = colIdx
				break
			}
		}
	}

	return cols
}

// extractUniqueExcelRoles extracts all distinct role strings from the role column.
func extractUniqueExcelRoles(rawRows [][]string, roleCol int, companyRoles []TeamRoleOption) []*ExcelRoleInfo {
	roleCounts := make(map[string]int)
	if roleCol >= 0 {
		for _, row := range rawRows {
			if roleCol < len(row) {
				roleName := strings.TrimSpace(row[roleCol])
				if roleName != "" {
					roleCounts[roleName]++
				}
			}
		}
	}

	var results []*ExcelRoleInfo
	for rawRole, count := range roleCounts {
		matchedID, matchedKey, matchedName, isAuto := MatchRoleByName(rawRole, companyRoles)
		results = append(results, &ExcelRoleInfo{
			RawName:        rawRole,
			RowCount:       count,
			MatchedRoleID:  matchedID,
			MatchedRoleKey: matchedKey,
			MatchedName:    matchedName,
			IsAutoMatched:  isAuto,
		})
	}
	return results
}

// MatchRoleByName finds the most appropriate platform role for a raw Excel role name.
func MatchRoleByName(rawRole string, companyRoles []TeamRoleOption, langOptional ...string) (int64, string, string, bool) {
	lang := "ar"
	if len(langOptional) > 0 && langOptional[0] != "" {
		lang = langOptional[0]
	}
	normRaw := normalizeArabicText(rawRole)

	// 1. Exact match on role name
	for _, cr := range companyRoles {
		if normalizeArabicText(cr.Name) == normRaw {
			return cr.ID, cr.Key, cr.Name, true
		}
	}

	// 2. Substring & Semantic matching
	type rolePattern struct {
		keywords []string
		keys     []string
	}
	patterns := []rolePattern{
		{
			keywords: []string{i18n.TDefault("w4_ui.s_146_146"), i18n.TDefault("w4_ui.s_147_147"), i18n.TDefault("w4_ui.s_148_148"), i18n.TDefault("w4_ui.s_149_149"), "pharmacist", "pharmacy"},
			keys:     []string{"org_pharmacist", "pharmacist", "staff_pharmacist"},
		},
		{
			keywords: []string{i18n.TDefault("w4_ui.s_150_150"), i18n.TDefault("w4_ui.s_151_151"), i18n.TDefault("w4_ui.s_152_152"), i18n.TDefault("w4_ui.s_153_153"), i18n.TDefault("w4_ui.s_154_154"), "manager", "admin", "supervisor", "lead"},
			keys:     []string{"org_admin", "admin", "branch_manager", "manager"},
		},
		{
			keywords: []string{i18n.TDefault("w4_ui.s_155_155"), i18n.TDefault("w4_ui.s_156_156"), i18n.TDefault("w4_ui.s_157_157"), "owner", "partner"},
			keys:     []string{"org_owner", "owner"},
		},
		{
			keywords: []string{i18n.TDefault("w4_ui.s_158_158"), i18n.TDefault("w4_ui.s_159_159"), i18n.TDefault("w4_ui.s_160_160"), i18n.TDefault("w4_ui.s_161_161"), i18n.TDefault("w4_ui.s_162_162"), i18n.TDefault("w4_ui.s_163_163"), "employee", "cashier", "sales", "accountant", "staff"},
			keys:     []string{"org_employee", "employee", "staff"},
		},
	}

	for _, pat := range patterns {
		matches := false
		for _, kw := range pat.keywords {
			if strings.Contains(normRaw, normalizeArabicText(kw)) {
				matches = true
				break
			}
		}
		if matches {
			// Find among companyRoles by key or name
			for _, targetKey := range pat.keys {
				for _, cr := range companyRoles {
					if cr.Key == targetKey || strings.Contains(normalizeArabicText(cr.Name), normalizeArabicText(targetKey)) {
						return cr.ID, cr.Key, cr.Name, true
					}
				}
			}
		}
	}

	// Fallback to the first non-owner role if available
	for _, cr := range companyRoles {
		if !cr.IsOwner && cr.Key != "org_owner" {
			return cr.ID, cr.Key, cr.Name, false
		}
	}

	if len(companyRoles) > 0 {
		return companyRoles[0].ID, companyRoles[0].Key, companyRoles[0].Name, false
	}

	return 0, "org_employee", i18n.T(lang, "team.import.default_role_name"), false
}

func (h *UIHandler) handleTeamImportPage(w http.ResponseWriter, r *http.Request, audience string) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, fmt.Sprintf("/auth/login?redirect=/%s/team/import", audience), http.StatusSeeOther)
		return
	}

	sessions := globalTeamImportSessionStore.ListSessions(actor.OrganizationID)
	noticeType := r.URL.Query().Get("notice_type")
	noticeMsg := r.URL.Query().Get("notice")
	if noticeMsg == "" {
		noticeMsg = r.URL.Query().Get("msg")
	}

	view := pages.TeamImportView{
		Audience:   audience,
		BaseURL:    fmt.Sprintf("/%s/team", audience),
		ImportURL:  fmt.Sprintf("/%s/team/import", audience),
		Sessions:   sessions,
		NoticeType: noticeType,
		NoticeMsg:  noticeMsg,
	}

	h.renderPage(ctx, w, fmt.Sprintf("render %s team import", audience), pages.TeamImportPage(view, lang, dir))
}

func (h *UIHandler) handleTeamImportUploadSubmit(w http.ResponseWriter, r *http.Request, audience string) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, fmt.Sprintf("/auth/login?redirect=/%s/team/import", audience), http.StatusSeeOther)
		return
	}

	baseImportURL := fmt.Sprintf("/%s/team/import", audience)

	if err := parseImportUpload(w, r); err != nil {
		h.redirectWithNotice(w, r, baseImportURL, "error", i18n.T(langOf(r), "customer.saving.import.file_too_large_short"))
		return
	}

	file, fileHeader, err := r.FormFile("file")
	if err != nil {
		h.redirectWithNotice(w, r, baseImportURL, "error", i18n.T(langOf(r), "customer.saving.import.select_file"))
		return
	}
	defer file.Close()

	if !SupportedUploadName(fileHeader.Filename) {
		h.redirectWithNotice(w, r, baseImportURL, "error", unsupportedUploadMsg(langOf(r)))
		return
	}

	fileBytes, err := io.ReadAll(file)
	if err != nil || len(fileBytes) == 0 {
		h.redirectWithNotice(w, r, baseImportURL, "error", i18n.T(langOf(r), "customer.saving.import.file_empty"))
		return
	}

	if err := filesecurity.ValidateSpreadsheetSecurity(fileBytes, fileHeader.Filename, filesecurity.WithAllowEmails(true)); err != nil {
		h.redirectWithNotice(w, r, baseImportURL, "error", filesecurity.SecurityErrorMessage)
		return
	}

	rawRows, err := sheet.ReadRows(fileBytes, fileHeader.Filename, sheet.WithAllowEmails(true))
	if err != nil || len(rawRows) < 2 {
		h.log.WarnContext(ctx, "failed to parse spreadsheet", "error", err, "filename", fileHeader.Filename)
		msg := i18n.T(langOf(r), "customer.saving.import.parse_error_short")
		if err != nil && strings.Contains(err.Error(), filesecurity.SecurityErrorMessage) {
			msg = filesecurity.SecurityErrorMessage
		}
		h.redirectWithNotice(w, r, baseImportURL, "error", msg)
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
		h.redirectWithNotice(w, r, baseImportURL, "error", i18n.T(langOf(r), "customer.team.import.no_employee_rows"))
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

	session := globalTeamImportSessionStore.NewSession(actor.OrganizationID, actor.UserID, audience, fileHeader.Filename, len(dataRows))
	session.Phase = pages.TeamPhaseMapping
	session.Headers = headers
	session.SampleRows = sampleRows
	session.RawDataRows = dataRows
	session.DetectedCols = detectedCols
	session.ExcelRoles = excelRoles
	session.CompanyRoles = companyRoles
	session.Branches = branches
	session.DefaultRoleID = defaultRoleID

	http.Redirect(w, r, fmt.Sprintf("/%s/team/import/%s", audience, session.ID), http.StatusSeeOther)
}

func (h *UIHandler) handleTeamImportSampleDownload(w http.ResponseWriter, audience, filename string) {
	bytes, err := GenerateTeamSampleExcel(audience)
	if err != nil {
		http.Error(w, "Failed to generate sample spreadsheet", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	w.Header().Set("Content-Length", strconv.Itoa(len(bytes)))
	_, _ = w.Write(bytes)
}

func (h *UIHandler) handleTeamImportSessionPage(w http.ResponseWriter, r *http.Request, audience string) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, fmt.Sprintf("/auth/login?redirect=/%s/team/import", audience), http.StatusSeeOther)
		return
	}

	sessionID := chi.URLParam(r, "id")
	session, ok := globalTeamImportSessionStore.GetSession(sessionID, actor.OrganizationID)
	if !ok {
		h.redirectWithNotice(w, r, fmt.Sprintf("/%s/team/import", audience), "error", i18n.T(langOf(r), "customer.saving.import.session_not_found"))
		return
	}

	noticeType := r.URL.Query().Get("notice_type")
	noticeMsg := r.URL.Query().Get("notice")
	if noticeMsg == "" {
		noticeMsg = r.URL.Query().Get("msg")
	}

	view := pages.TeamImportView{
		Audience:   audience,
		BaseURL:    fmt.Sprintf("/%s/team", audience),
		ImportURL:  fmt.Sprintf("/%s/team/import", audience),
		Session:    session,
		NoticeType: noticeType,
		NoticeMsg:  noticeMsg,
	}

	h.renderPage(ctx, w, fmt.Sprintf("render %s team import session", audience), pages.TeamImportPage(view, lang, dir))
}

func (h *UIHandler) handleTeamImportMapSubmit(w http.ResponseWriter, r *http.Request, audience string) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	baseImportURL := fmt.Sprintf("/%s/team/import", audience)
	sessionID := chi.URLParam(r, "id")
	session, ok := globalTeamImportSessionStore.GetSession(sessionID, actor.OrganizationID)
	if !ok {
		h.redirectWithNotice(w, r, baseImportURL, "error", i18n.T(langOf(r), "customer.saving.import.session_not_found"))
		return
	}

	if err := r.ParseForm(); err != nil {
		h.redirectWithNotice(w, r, fmt.Sprintf("/%s/team/import/%s", audience, sessionID), "error", i18n.T(langOf(r), "customer.team.import.data_invalid"))
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
		h.redirectWithNotice(w, r, fmt.Sprintf("/%s/team/import/%s", audience, sessionID), "error", i18n.T(langOf(r), "customer.team.import.columns_required"))
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

	http.Redirect(w, r, fmt.Sprintf("/%s/team/import/%s", audience, sessionID), http.StatusSeeOther)
}

func (h *UIHandler) handleTeamImportCommitSubmit(w http.ResponseWriter, r *http.Request, audience, defaultRoleKey string) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	baseImportURL := fmt.Sprintf("/%s/team/import", audience)
	sessionID := chi.URLParam(r, "id")
	session, ok := globalTeamImportSessionStore.GetSession(sessionID, actor.OrganizationID)
	if !ok || session.Phase != pages.TeamPhaseReview {
		h.redirectWithNotice(w, r, baseImportURL, "error", i18n.T(langOf(r), "customer.team.import.session_invalid"))
		return
	}

	if h.idSvc == nil || h.orgSvc == nil {
		h.redirectWithNotice(w, r, fmt.Sprintf("/%s/team/import/%s", audience, sessionID), "error", i18n.T(langOf(r), "customer.team.import.system_unavailable"))
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
			roleKey = defaultRoleKey
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

	http.Redirect(w, r, fmt.Sprintf("/%s/team/import/%s", audience, sessionID), http.StatusSeeOther)
}

func (h *UIHandler) handleTeamImportCancelSubmit(w http.ResponseWriter, r *http.Request, audience string) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	sessionID := chi.URLParam(r, "id")
	globalTeamImportSessionStore.DeleteSession(sessionID, actor.OrganizationID)
	h.redirectWithNotice(w, r, fmt.Sprintf("/%s/team/import", audience), "info", i18n.T(langOf(r), "customer.team.import.cancelled"))
}

// ParseAndValidateTeamRows builds TeamImportRow items from raw rows and mappings.
func ParseAndValidateTeamRows(
	rawRows [][]string,
	cols TeamDetectedCols,
	roleMap map[string]int64, // rawRole -> roleID
	defaultRoleID int64,
	companyRoles []TeamRoleOption,
	branches []TeamBranchOption,
	langOptional ...string,
) []*TeamImportRow {
	lang := "ar"
	if len(langOptional) > 0 && langOptional[0] != "" {
		lang = langOptional[0]
	}
	var rows []*TeamImportRow

	roleByID := make(map[int64]TeamRoleOption)
	for _, cr := range companyRoles {
		roleByID[cr.ID] = cr
	}

	branchByName := make(map[string]TeamBranchOption)
	for _, b := range branches {
		branchByName[normalizeArabicText(b.Name)] = b
		if b.Code != "" {
			branchByName[normalizeArabicText(b.Code)] = b
		}
	}

	seenEmails := make(map[string]int)

	for i, rawRow := range rawRows {
		rowIndex := i + 1

		getValue := func(col int) string {
			if col >= 0 && col < len(rawRow) {
				return strings.TrimSpace(rawRow[col])
			}
			return ""
		}

		name := getValue(cols.NameCol)
		email := strings.ToLower(getValue(cols.EmailCol))
		phone := getValue(cols.PhoneCol)
		rawRole := getValue(cols.RoleCol)
		jobTitle := getValue(cols.JobTitleCol)
		rawBranch := getValue(cols.BranchCol)
		code := getValue(cols.CodeCol)

		// Skip completely empty rows
		if name == "" && email == "" && phone == "" {
			continue
		}

		// Fallback name if missing
		if name == "" && email != "" {
			name = strings.Split(email, "@")[0]
		}

		// Resolve role
		targetRoleID := defaultRoleID
		if rID, ok := roleMap[rawRole]; ok && rID > 0 {
			targetRoleID = rID
		}

		roleOpt, hasRole := roleByID[targetRoleID]
		roleKey := "org_employee"
		roleName := i18n.T(lang, "team.import.role_employee")
		if hasRole {
			roleKey = roleOpt.Key
			roleName = roleOpt.Name
		}

		// Fallback job title
		if jobTitle == "" {
			if rawRole != "" {
				jobTitle = rawRole
			} else {
				jobTitle = roleName
			}
		}

		// Fallback employee code
		if code == "" {
			code = fmt.Sprintf("EMP-%03d", rowIndex)
		}

		// Resolve branch
		var assignedBranchID *int64
		var assignedBranchName string
		if rawBranch != "" {
			normB := normalizeArabicText(rawBranch)
			if b, ok := branchByName[normB]; ok {
				assignedBranchID = &b.ID
				assignedBranchName = b.Name
			} else {
				// Partial match
				for _, b := range branches {
					if strings.Contains(normalizeArabicText(b.Name), normB) || strings.Contains(normB, normalizeArabicText(b.Name)) {
						assignedBranchID = &b.ID
						assignedBranchName = b.Name
						break
					}
				}
			}
		}

		// Validation
		isValid := true
		var valErr string

		if email == "" || !strings.Contains(email, "@") || !strings.Contains(email, ".") {
			isValid = false
			valErr = i18n.T(lang, "team.import.email_invalid_or_missing")
		} else if prevRow, duplicate := seenEmails[email]; duplicate {
			isValid = false
			valErr = fmt.Sprintf(i18n.T(lang, "team.import.email_duplicate_in_file"), prevRow)
		} else {
			seenEmails[email] = rowIndex
		}

		if name == "" {
			isValid = false
			if valErr != "" {
				valErr += " | "
			}
			valErr += i18n.T(lang, "team.import.employee_name_missing")
		}

		status := "ready"
		if !isValid {
			status = "skipped"
		}

		rows = append(rows, &TeamImportRow{
			Index:              rowIndex,
			RawName:            name,
			Email:              email,
			Phone:              phone,
			RawRole:            rawRole,
			AssignedRoleID:     targetRoleID,
			AssignedRoleKey:    roleKey,
			AssignedRoleName:   roleName,
			JobTitle:           jobTitle,
			EmployeeCode:       code,
			RawBranch:          rawBranch,
			AssignedBranchID:   assignedBranchID,
			AssignedBranchName: assignedBranchName,
			IsValid:            isValid,
			ValidationError:    valErr,
			ImportStatus:       status,
		})
	}

	return rows
}

// GenerateTeamSampleExcel creates a downloadable sample Excel file for employee import.
func GenerateTeamSampleExcel(orgType string) ([]byte, error) {
	f := excelize.NewFile()
	defer f.Close()

	sheetName := i18n.T("ar", "excel.sheet.employees")
	f.SetSheetName("Sheet1", sheetName)

	headers := []string{
		i18n.T("ar", "excel.col.full_name"),
		i18n.T("ar", "excel.col.email"),
		i18n.T("ar", "excel.col.phone"),
		i18n.T("ar", "excel.col.role"),
		i18n.T("ar", "excel.col.job_title"),
		i18n.T("ar", "excel.col.emp_code"),
		i18n.T("ar", "excel.col.branch_warehouse"),
	}

	var sampleData [][]string
	if orgType == "vendor" {
		sampleData = [][]string{
			{i18n.T("ar", "excel.sample.vendor_emp1_name"), "ahmed.ali@example.com", "01012345678", i18n.T("ar", "role.branch_manager"), i18n.T("ar", "excel.sample.vendor_emp1_title"), "V-EMP-01", i18n.T("ar", "excel.sample.vendor_emp1_wh")},
			{i18n.T("ar", "excel.sample.vendor_emp2_name"), "sara.hassan@example.com", "01123456789", i18n.T("ar", "role.sales_rep"), i18n.T("ar", "excel.sample.vendor_emp2_title"), "V-EMP-02", i18n.T("ar", "excel.sample.vendor_emp2_wh")},
			{i18n.T("ar", "excel.sample.vendor_emp3_name"), "mohamed.kareem@example.com", "01234567890", i18n.T("ar", "role.data_entry"), i18n.T("ar", "excel.sample.vendor_emp3_title"), "V-EMP-03", i18n.T("ar", "excel.sample.vendor_emp1_wh")},
			{i18n.T("ar", "excel.sample.vendor_emp4_name"), "heba.fouad@example.com", "01511223344", i18n.T("ar", "role.accountant"), i18n.T("ar", "excel.sample.vendor_emp4_title"), "V-EMP-04", i18n.T("ar", "excel.sample.vendor_emp4_wh")},
		}
	} else {
		sampleData = [][]string{
			{i18n.T("ar", "excel.sample.pharm_emp1_name"), "dr.ahmed@pharmacy.com", "01012345678", i18n.T("ar", "role.pharmacist"), i18n.T("ar", "excel.sample.pharm_emp1_title"), "PH-001", i18n.T("ar", "excel.sample.vendor_emp1_wh")},
			{i18n.T("ar", "excel.sample.pharm_emp2_name"), "dr.reem@pharmacy.com", "01123456789", i18n.T("ar", "role.pharmacist"), i18n.T("ar", "excel.sample.pharm_emp2_title"), "PH-002", i18n.T("ar", "excel.sample.pharm_emp2_wh")},
			{i18n.T("ar", "excel.sample.pharm_emp3_name"), "tarek@pharmacy.com", "01234567890", i18n.T("ar", "role.data_entry"), i18n.T("ar", "excel.sample.pharm_emp3_title"), "PH-003", i18n.T("ar", "excel.sample.vendor_emp1_wh")},
			{i18n.T("ar", "excel.sample.pharm_emp4_name"), "mona@pharmacy.com", "01511223344", i18n.T("ar", "role.branch_manager"), i18n.T("ar", "excel.sample.pharm_emp4_title"), "PH-004", i18n.T("ar", "excel.sample.pharm_emp4_wh")},
		}
	}

	// Style header
	headerStyle, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Color: "#FFFFFF", Size: 11},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"#0284C7"}, Pattern: 1},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
	})

	for colIdx, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(colIdx+1, 1)
		_ = f.SetCellValue(sheetName, cell, h)
		_ = f.SetCellStyle(sheetName, cell, cell, headerStyle)
		_ = f.SetColWidth(sheetName, string(rune('A'+colIdx)), string(rune('A'+colIdx)), 22)
	}

	for rowIdx, row := range sampleData {
		for colIdx, val := range row {
			cell, _ := excelize.CoordinatesToCellName(colIdx+1, rowIdx+2)
			_ = f.SetCellValue(sheetName, cell, val)
		}
	}

	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
