package ui

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/xuri/excelize/v2"
)

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

	nameKeywords := []string{"اسم الموظف", "اسم العامل", "الاسم بالكامل", "الاسم", "الموظف", "العامل", "اسم", "employee name", "staff name", "full name", "name", "username"}
	emailKeywords := []string{i18n.T("ar", "excel.col.email"), "البريد الالكتروني", "البريد", "الايميل", "الإيميل", "ايميل", "email", "e-mail", "mail"}
	phoneKeywords := []string{i18n.T("ar", "excel.col.phone"), "رقم الموبايل", "الهاتف", "الموبايل", "الجوال", "التليفون", "تليفون", "موبايل", "هاتف", "phone", "mobile", "tel", "cellphone"}
	roleKeywords := []string{"الدور", "الصلاحية", "الصلاحيات", "نوع الحساب", "نوع الدور", "المسمى الوظيفي / الدور", "role", "roles", "permission", "user role"}
	jobTitleKeywords := []string{i18n.T("ar", "excel.col.job_title"), "الوظيفة", "المهنة", "المنصب", "job title", "position", "title", "designation"}
	branchKeywords := []string{"الفرع", "المخزن", "المستودع", "مكان العمل", "فرع", "مخزن", "branch", "warehouse", "store", "location"}
	codeKeywords := []string{i18n.T("ar", "excel.col.emp_code"), "الرقم الوظيفي", "كود", "الكود", "رمز الموظف", "الرقم التعريفي", "employee code", "emp code", "code", "staff id", "employee id", "emp id"}
	notesKeywords := []string{"ملاحظات", "ملاحظة", "الرقم القومي", "بطاقة الرقم القومي", "notes", "remarks", "national id"}

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
			keywords: []string{"صيدلي", "دكتور صيدلي", "مساعد صيدلي", "صيدلاني", "pharmacist", "pharmacy"},
			keys:     []string{"org_pharmacist", "pharmacist", "staff_pharmacist"},
		},
		{
			keywords: []string{"مدير", "مشرف", "مسؤول", "ادمن", "إدارة", "manager", "admin", "supervisor", "lead"},
			keys:     []string{"org_admin", "admin", "branch_manager", "manager"},
		},
		{
			keywords: []string{"مالك", "صاحب", "مؤسس", "owner", "partner"},
			keys:     []string{"org_owner", "owner"},
		},
		{
			keywords: []string{"موظف", "كاشير", "مبيعات", "محاسب", "مندوب", "عامل", "employee", "cashier", "sales", "accountant", "staff"},
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

func detectTeamColumns(headers []string, sampleRows [][]string) TeamDetectedCols {
	return DetectTeamColumns(headers, sampleRows)
}

func matchRoleByName(rawRole string, companyRoles []TeamRoleOption) (int64, string, string, bool) {
	return MatchRoleByName(rawRole, companyRoles)
}

func parseAndValidateTeamRows(
	rawRows [][]string,
	cols TeamDetectedCols,
	roleMap map[string]int64,
	defaultRoleID int64,
	companyRoles []TeamRoleOption,
	branches []TeamBranchOption,
) []*TeamImportRow {
	return ParseAndValidateTeamRows(rawRows, cols, roleMap, defaultRoleID, companyRoles, branches)
}
