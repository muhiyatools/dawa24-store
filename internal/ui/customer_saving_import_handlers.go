package ui

import (
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/xuri/excelize/v2"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
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

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.SavingImportPage(view, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render customer saving import page", "error", err)
	}
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
		h.redirectWithNotice(w, r, "/customer/saving-products/import", "error", "الملف المرفوع كبير جداً أو غير صالح.")
		return
	}

	file, fileHeader, err := r.FormFile("file")
	if err != nil {
		h.redirectWithNotice(w, r, "/customer/saving-products/import", "error", "يرجى اختيار ملف Excel أو CSV صالح.")
		return
	}
	defer file.Close()

	ext := strings.ToLower(filepath.Ext(fileHeader.Filename))
	if ext != ".xlsx" && ext != ".xls" && ext != ".csv" {
		h.redirectWithNotice(w, r, "/customer/saving-products/import", "error", "صيغة الملف غير مدعومة. يرجى رفع ملف بصيغة xlsx أو xls أو csv.")
		return
	}

	fileBytes, err := io.ReadAll(file)
	if err != nil || len(fileBytes) == 0 {
		h.redirectWithNotice(w, r, "/customer/saving-products/import", "error", "الملف فارغ أو تعذر قراءته.")
		return
	}

	rawRows, err := sheet.ReadRows(fileBytes, fileHeader.Filename)
	if err != nil || len(rawRows) < 2 {
		h.log.WarnContext(ctx, "failed to parse spreadsheet", "error", err, "filename", fileHeader.Filename)
		h.redirectWithNotice(w, r, "/customer/saving-products/import", "error", "تعذر قراءة ملف البيانات المرفوع أو أن الملف لا يحتوي على صفوف بيانات.")
		return
	}

	headers := rawRows[0]
	var sampleRows [][]string
	if len(rawRows) > 1 {
		limit := 4
		if len(rawRows)-1 < limit {
			limit = len(rawRows) - 1
		}
		sampleRows = rawRows[1 : 1+limit]
	}

	nameCol, skuCol, qtyCol, priceCol, productIDCol := detectSavingProductColumns(
		headers,
		sampleRows,
		"", "", "", "", "",
	)

	session := globalSavingImportSessionStore.NewSession(actor.OrganizationID, actor.UserID, fileHeader.Filename, len(rawRows)-1)
	session.Phase = SavingPhaseMapping
	session.Headers = headers
	session.SampleRows = sampleRows
	session.RawDataRows = rawRows[1:]
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
		h.redirectWithNotice(w, r, "/customer/saving-products/import", "error", "جلسة الاستيراد غير موجودة أو انتهت صلاحيتها.")
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

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.SavingImportPage(view, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render customer saving import session page", "error", err)
	}
}

// CustomerSavingProductsSampleXLSX streams download of a clean Excel template.
func (h *UIHandler) CustomerSavingProductsSampleXLSX(w http.ResponseWriter, r *http.Request) {
	f := excelize.NewFile()
	sheet := "Saving Products Sample"
	f.SetSheetName("Sheet1", sheet)
	_ = f.SetSheetView(sheet, 0, &excelize.ViewOptions{
		RightToLeft: func(b bool) *bool { return &b }(true),
	})

	headers := []string{"اسم الصنف", "كود الصنف / SKU", "الكمية", "سعر الجمهور (ج.م)"}
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
	csvContent := "\xEF\xBB\xBFاسم الصنف,كود الصنف / SKU,الكمية,سعر الجمهور (ج.م)\n" +
		"بانادول اكسترا 500 مجم أقراص,PAN-EXT-24,50,48.50\n" +
		"كونجستال أقراص للبرد,CONG-TAB-20,30,29.00\n" +
		"أوجمنتين 1 جم أقراص 14 قرص,AUG-1G-14,20,110.00\n" +
		"أنتينال 200 مجم كبسول,ANT-200-24,40,32.00\n"

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=\"saving_products_sample.csv\"")
	_, _ = w.Write([]byte(csvContent))
}
