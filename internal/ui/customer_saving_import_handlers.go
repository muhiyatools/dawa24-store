package ui

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/xuri/excelize/v2"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/productmatch"
	"github.com/muhiya/dawa24-store/internal/shared/sheet"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// CustomerSavingProductsImportPage renders the sessions list and file dropzone.
func (h *UIHandler) CustomerSavingProductsImportPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/customer/saving-products/import", http.StatusSeeOther)
		return
	}

	sessions := globalSavingImportSessionStore.ListSessions(actor.OrganizationID)
	noticeType := r.URL.Query().Get("notice_type")
	noticeMsg := r.URL.Query().Get("notice")

	view := pages.SavingImportView{
		AIAvailable:         h.matchEnhancer != nil,
		AIUnavailableReason: savingAIUnavailableReason(h.matchEnhancer),
		Audience:            "customer",
		BaseURL:             "/customer/saving-products",
		ImportURL:           "/customer/saving-products/import",
		Sessions:            sessions,
		NoticeType:          noticeType,
		NoticeMsg:           noticeMsg,
	}

	h.renderPage(ctx, w, "render customer saving import page", pages.SavingImportPage(view, lang, dir))
}

// CustomerSavingProductsImportUploadSubmit handles file upload and creates new import session.
func (h *UIHandler) CustomerSavingProductsImportUploadSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/customer/saving-products/import", http.StatusSeeOther)
		return
	}

	if err := r.ParseMultipartForm(32 << 20); err != nil {
		h.redirectWithNotice(w, r, "/customer/saving-products/import", "error", i18n.T(langOf(r), "customer.saving.import.file_too_large_short"))
		return
	}

	file, fileHeader, err := r.FormFile("file")
	if err != nil {
		h.redirectWithNotice(w, r, "/customer/saving-products/import", "error", i18n.T(langOf(r), "customer.saving.import.select_file"))
		return
	}
	defer file.Close()

	if !SupportedUploadName(fileHeader.Filename) {
		h.redirectWithNotice(w, r, "/customer/saving-products/import", "error", unsupportedUploadMsg(langOf(r)))
		return
	}

	fileBytes, err := io.ReadAll(file)
	if err != nil || len(fileBytes) == 0 {
		h.redirectWithNotice(w, r, "/customer/saving-products/import", "error", i18n.T(langOf(r), "customer.saving.import.file_empty"))
		return
	}

	rawRows, err := sheet.ReadRows(fileBytes, fileHeader.Filename)
	if err != nil || len(rawRows) < 2 {
		h.log.WarnContext(ctx, "failed to parse spreadsheet", "error", err, "filename", fileHeader.Filename)
		h.redirectWithNotice(w, r, "/customer/saving-products/import", "error", i18n.T(langOf(r), "customer.saving.import.parse_error_short"))
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
		h.redirectWithNotice(w, r, "/customer/saving-products/import", "error", i18n.T(langOf(r), "customer.saving.import.no_data_rows"))
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

	session := globalSavingImportSessionStore.NewSession(actor.OrganizationID, actor.UserID, fileHeader.Filename, len(dataRows))
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

	http.Redirect(w, r, fmt.Sprintf("/customer/saving-products/import/%s", session.ID), http.StatusSeeOther)
}

// CustomerSavingProductsImportSessionPage renders the session at its current wizard phase.
func (h *UIHandler) CustomerSavingProductsImportSessionPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/customer/saving-products/import", http.StatusSeeOther)
		return
	}

	sessionID := chi.URLParam(r, "id")
	session, ok := globalSavingImportSessionStore.GetSession(sessionID, actor.OrganizationID)
	if !ok {
		h.redirectWithNotice(w, r, "/customer/saving-products/import", "error", i18n.T(langOf(r), "customer.saving.import.session_not_found"))
		return
	}

	matchFilter := strings.TrimSpace(r.URL.Query().Get("match"))
	sortBy := strings.TrimSpace(r.URL.Query().Get("sort"))
	sortOrder := strings.TrimSpace(r.URL.Query().Get("order"))
	search := strings.TrimSpace(r.URL.Query().Get("q"))

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit < 1 {
		limit = 25
	}

	filter := SavingRowFilter{
		Search:      search,
		MatchFilter: matchFilter,
		SortBy:      sortBy,
		SortOrder:   sortOrder,
		Page:        page,
		Limit:       limit,
	}

	rows, total := globalSavingImportSessionStore.FilterItems(session, filter)

	noticeType := r.URL.Query().Get("notice_type")
	noticeMsg := r.URL.Query().Get("notice")

	view := pages.SavingImportView{
		AIAvailable:         h.matchEnhancer != nil,
		AIUnavailableReason: savingAIUnavailableReason(h.matchEnhancer),
		Audience:            "customer",
		BaseURL:             "/customer/saving-products",
		ImportURL:           "/customer/saving-products/import",
		Session:             session,
		Filter:              filter,
		Rows:                rows,
		RowTotal:            total,
		NoticeType:          noticeType,
		NoticeMsg:           noticeMsg,
	}

	h.renderPage(ctx, w, "render customer saving import session page", pages.SavingImportPage(view, lang, dir))
}

// CustomerSavingProductsSampleXLSX streams download of a clean Excel template.
func (h *UIHandler) CustomerSavingProductsSampleXLSX(w http.ResponseWriter, r *http.Request) {
	lang, _ := h.localeAndDir(r)
	f := excelize.NewFile()
	sheet := "Saving Products Sample"
	f.SetSheetName("Sheet1", sheet)
	_ = f.SetSheetView(sheet, 0, &excelize.ViewOptions{
		RightToLeft: func(b bool) *bool { return &b }(true),
	})

	headers := []string{
		i18n.T(lang, "customer.saving.sample_col_name"),
		i18n.T(lang, "customer.saving.sample_col_sku"),
		i18n.T(lang, "customer.saving.sample_col_qty"),
		i18n.T(lang, "customer.saving.sample_col_price"),
	}
	for i, hName := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		_ = f.SetCellValue(sheet, cell, hName)
	}

	samples := [][]any{
		{"بانادول اكسترا 500 مجم أقراص", "PAN-EXT-24", 50, 48.50},
		{"كونجستال أقراص للبرد", "CONG-TAB-20", 30, 29.00},
		{"أوجمنتين 1 جم أقراص 14 قرص", "AUG-1G-14", 20, 110.00},
		{"أنتينال 200 مجم كبسول", "ANT-200-24", 40, 32.00},
	}

	for rIdx, row := range samples {
		for cIdx, val := range row {
			cell, _ := excelize.CoordinatesToCellName(cIdx+1, rIdx+2)
			_ = f.SetCellValue(sheet, cell, val)
		}
	}

	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", "attachment; filename=\"saving_products_sample.xlsx\"")
	_ = f.Write(w)
}

// CustomerSavingProductsSampleCSV streams download of a clean CSV template.
func (h *UIHandler) CustomerSavingProductsSampleCSV(w http.ResponseWriter, r *http.Request) {
	lang, _ := h.localeAndDir(r)
	csvContent := "\xEF\xBB\xBF" + i18n.T(lang, "customer.saving.sample_col_name") + "," +
		i18n.T(lang, "customer.saving.sample_col_sku") + "," +
		i18n.T(lang, "customer.saving.sample_col_qty") + "," +
		i18n.T(lang, "customer.saving.sample_col_price") + "\n" +
		"بانادول اكسترا 500 مجم أقراص,PAN-EXT-24,50,48.50\n" +
		"كونجستال أقراص للبرد,CONG-TAB-20,30,29.00\n" +
		"أوجمنتين 1 جم أقراص 14 قرص,AUG-1G-14,20,110.00\n" +
		"أنتينال 200 مجم كبسول,ANT-200-24,40,32.00\n"

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=\"saving_products_sample.csv\"")
	_, _ = w.Write([]byte(csvContent))
}
