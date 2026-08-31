package ui

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/xuri/excelize/v2"
)

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
