package ui

import (
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"

	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/filesecurity"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/pagination"
	"github.com/muhiya/dawa24-store/internal/shared/productmatch"
	"github.com/muhiya/dawa24-store/internal/shared/sheet"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// AdminOrgImportPage renders the organization product import hub.
func (h *UIHandler) AdminOrgImportPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	page := pagination.PageNumber(r)
	limit := pagination.RowsPerPage(r)
	offset := (page - 1) * limit

	tab := strings.TrimSpace(r.URL.Query().Get("tab"))
	if tab != "vendor" {
		tab = "pharmacy"
	}

	q := strings.TrimSpace(r.URL.Query().Get("q"))
	noticeType := r.URL.Query().Get("notice_type")
	noticeMsg := r.URL.Query().Get("notice")

	var orgType org.OrganizationType
	if tab == "vendor" {
		orgType = org.TypeVendor
	} else {
		orgType = org.TypeCustomer
	}

	var orgs []*org.Organization
	var totalCount int
	var totalPharmacies, totalVendors int

	if h.orgSvc != nil {
		sysCtx := database.AsSystem(ctx)
		var err error
		orgs, totalCount, err = h.orgSvc.ListOrganizationsWithTotal(sysCtx, q, &orgType, nil, limit, offset)
		if err != nil {
			h.log.ErrorContext(ctx, "failed to list organizations for import", "error", err, "type", orgType)
		}

		pharmType := org.TypeCustomer
		vendType := org.TypeVendor
		totalPharmacies, _ = h.orgSvc.CountOrganizations(sysCtx, &pharmType, nil)
		totalVendors, _ = h.orgSvc.CountOrganizations(sysCtx, &vendType, nil)
	}

	data := pages.AdminOrgImportPageData{
		Organizations:   orgs,
		TotalOrgs:       totalPharmacies + totalVendors,
		TotalPharmacies: totalPharmacies,
		TotalVendors:    totalVendors,
		ActiveTab:       tab,
		SearchQuery:     q,
		Page:            page,
		PerPage:         limit,
		TotalCount:      totalCount,
		NoticeType:      noticeType,
		NoticeMsg:       noticeMsg,
	}

	h.renderPage(ctx, w, "render admin org import page", pages.AdminOrgImportPage(data, lang, dir))
}

// AdminOrgImportSavingsUploadSubmit starts an import session for the selected target organization.
func (h *UIHandler) AdminOrgImportSavingsUploadSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, _ := authctx.From(ctx)

	if err := parseImportUpload(w, r); err != nil {
		h.redirectWithNotice(w, r, "/admin/organizations/import", "error", i18n.T(lang, "customer.saving.import.file_too_large_short"))
		return
	}

	targetOrgID, err := strconv.ParseInt(r.PostFormValue("org_id"), 10, 64)
	if err != nil || targetOrgID <= 0 {
		h.redirectWithNotice(w, r, "/admin/organizations/import", "error", i18n.T(lang, "validation.invalid_id"))
		return
	}

	file, fileHeader, err := r.FormFile("file")
	if err != nil {
		h.redirectWithNotice(w, r, "/admin/organizations/import", "error", i18n.T(lang, "customer.saving.import.select_file"))
		return
	}
	defer file.Close()

	if !SupportedUploadName(fileHeader.Filename) {
		h.redirectWithNotice(w, r, "/admin/organizations/import", "error", unsupportedUploadMsg(lang))
		return
	}

	fileBytes, err := io.ReadAll(file)
	if err != nil || len(fileBytes) == 0 {
		h.redirectWithNotice(w, r, "/admin/organizations/import", "error", i18n.T(lang, "customer.saving.import.file_empty"))
		return
	}

	if err := filesecurity.ValidateSpreadsheetSecurity(fileBytes, fileHeader.Filename); err != nil {
		h.redirectWithNotice(w, r, "/admin/organizations/import", "error", filesecurity.SecurityErrorMessage)
		return
	}

	rawRows, err := sheet.ReadRows(fileBytes, fileHeader.Filename)
	if err != nil || len(rawRows) < 2 {
		h.log.WarnContext(ctx, "failed to parse spreadsheet", "error", err, "filename", fileHeader.Filename)
		msg := i18n.T(lang, "customer.saving.import.parse_error_short")
		if err != nil && strings.Contains(err.Error(), filesecurity.SecurityErrorMessage) {
			msg = filesecurity.SecurityErrorMessage
		}
		h.redirectWithNotice(w, r, "/admin/organizations/import", "error", msg)
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
		h.redirectWithNotice(w, r, "/admin/organizations/import", "error", i18n.T(lang, "customer.saving.import.no_data_rows"))
		return
	}

	var sampleRows [][]string
	limit := 5
	if len(dataRows) < limit {
		limit = len(dataRows)
	}
	sampleRows = dataRows[:limit]

	nameCol, skuCol, qtyCol, priceCol, productIDCol := detectSavingProductColumns(
		headers,
		sampleRows,
		"", "", "", "", "",
	)

	session := globalSavingImportSessionStore.NewSession(targetOrgID, actor.UserID, fileHeader.Filename, len(dataRows))
	session.Phase = SavingPhaseMapping
	session.Headers = headers
	session.SampleRows = sampleRows
	session.RawDataRows = dataRows
	session.DetectedCols = SavingDetectedCols{
		NameCol:      nameCol,
		SKUCol:       skuCol,
		QtyCol:       qtyCol,
		PriceCol:     priceCol,
		ProductIDCol: productIDCol,
	}

	h.log.InfoContext(ctx, "admin initiated saving products import session", "actor_id", actor.UserID, "target_org_id", targetOrgID, "session_id", session.ID)
	http.Redirect(w, r, fmt.Sprintf("/admin/organizations/import/saving/%s", session.ID), http.StatusSeeOther)
}

// AdminOrgImportTempWarehouseUploadSubmit creates a temporary warehouse directly on behalf of the target organization.
func (h *UIHandler) AdminOrgImportTempWarehouseUploadSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, _ := authctx.From(ctx)

	if err := parseImportUpload(w, r); err != nil {
		h.redirectWithNotice(w, r, "/admin/organizations/import", "error", i18n.T(lang, "admin.temp_warehouse.upload_too_large"))
		return
	}

	targetOrgID, err := strconv.ParseInt(r.PostFormValue("org_id"), 10, 64)
	if err != nil || targetOrgID <= 0 {
		h.redirectWithNotice(w, r, "/admin/organizations/import", "error", i18n.T(lang, "validation.invalid_id"))
		return
	}

	var targetOrg *org.Organization
	if h.orgSvc != nil {
		targetOrg, _ = h.orgSvc.GetOrganization(database.AsSystem(ctx), targetOrgID)
	}

	var fileHeaders []*multipart.FileHeader
	if r.MultipartForm != nil && r.MultipartForm.File != nil {
		if files, ok := r.MultipartForm.File["files"]; ok && len(files) > 0 {
			fileHeaders = append(fileHeaders, files...)
		}
		if file, ok := r.MultipartForm.File["file"]; ok && len(file) > 0 {
			fileHeaders = append(fileHeaders, file...)
		}
	}

	if len(fileHeaders) == 0 {
		h.redirectWithNotice(w, r, "/admin/organizations/import", "error", i18n.T(lang, "admin.temp_warehouse.select_files_notice"))
		return
	}

	baseSupplierName := strings.TrimSpace(r.FormValue("supplier_name"))
	if baseSupplierName == "" && targetOrg != nil {
		if targetOrg.TradeName != nil && targetOrg.TradeName["ar"] != "" {
			baseSupplierName = targetOrg.TradeName["ar"]
		} else {
			baseSupplierName = targetOrg.LegalName
		}
	}

	res := h.processSingleTempWarehouseFile(
		database.AsSystem(ctx),
		fileHeaders[0],
		baseSupplierName,
		"", "", "", "",
		actor.UserID,
		&targetOrgID,
	)

	if !res.Success {
		h.redirectWithNotice(w, r, "/admin/organizations/import", "error", res.Error)
		return
	}

	h.log.InfoContext(ctx, "admin uploaded temp warehouse for organization", "actor_id", actor.UserID, "target_org_id", targetOrgID, "supplier_name", baseSupplierName)
	h.redirectWithNotice(w, r, "/admin/user/temparte-warehouses", "success", i18n.T(lang, "admin.temp_warehouse.file_uploaded_success"))
}
