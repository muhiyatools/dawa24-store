package ui

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/xuri/excelize/v2"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/spreadsheet"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// AdminProductImagesImportPage renders the image recovery main page with dropzone & past sessions.
func (h *UIHandler) AdminProductImagesImportPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/admin/products/images/import", http.StatusSeeOther)
		return
	}

	sessions := globalAdminImageImportSessionStore.ListSessions()
	noticeType := r.URL.Query().Get("notice_type")
	noticeMsg := r.URL.Query().Get("notice")

	view := pages.AdminProductImagesImportView{
		Session:    nil,
		Sessions:   sessions,
		NoticeType: noticeType,
		NoticeMsg:  noticeMsg,
		Actor:      actor,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminProductImagesImportPage(view, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin product images import page", "error", err)
	}
}

// AdminProductImagesUploadSubmit handles spreadsheet file upload and opens a new image recovery session.
func (h *UIHandler) AdminProductImagesUploadSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/admin/products/images/import", http.StatusSeeOther)
		return
	}

	if err := r.ParseMultipartForm(MaxUploadBytes); err != nil {
		h.redirectWithNotice(w, r, "/admin/products/images/import", "error", "حجم الملف كبير جداً أو تعذر قراءته.")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		h.redirectWithNotice(w, r, "/admin/products/images/import", "error", "يرجى اختيار ملف Excel أو CSV صالح.")
		return
	}
	defer file.Close()

	ext := strings.ToLower(filepath.Ext(header.Filename))
	if ext != ".xlsx" && ext != ".xls" && ext != ".csv" {
		h.redirectWithNotice(w, r, "/admin/products/images/import", "error", "صيغة الملف غير مدعومة. يرجى رفع ملف بصيغة .xlsx أو .xls أو .csv.")
		return
	}

	data, err := io.ReadAll(file)
	if err != nil || len(data) == 0 {
		h.redirectWithNotice(w, r, "/admin/products/images/import", "error", "الملف المرفوع فارغ أو تالف.")
		return
	}

	rawRows, err := spreadsheet.ReadRows(data)
	if err != nil || len(rawRows) < 2 {
		h.redirectWithNotice(w, r, "/admin/products/images/import", "error", "لم يتم العثور على بيانات صالحة في الملف المرفوع.")
		return
	}

	headers := rawRows[0]
	dataRows := rawRows[1:]

	skuCol, urlCol := detectImageImportColumns(headers)

	sampleCount := len(dataRows)
	if sampleCount > 5 {
		sampleCount = 5
	}
	sampleRows := make([][]string, sampleCount)
	copy(sampleRows, dataRows[:sampleCount])

	session := globalAdminImageImportSessionStore.NewSession(actor.OrganizationID, actor.UserID, header.Filename, len(dataRows))
	session.Headers = headers
	session.DetectedSKUCol = skuCol
	session.DetectedURLCol = urlCol
	session.SampleRows = sampleRows
	session.RawDataRows = dataRows

	http.Redirect(w, r, fmt.Sprintf("/admin/products/images/import/%s", session.ID), http.StatusSeeOther)
}

func detectImageImportColumns(headers []string) (skuCol, urlCol int) {
	skuCol = -1
	urlCol = -1

	for idx, rawH := range headers {
		h := strings.ToLower(strings.TrimSpace(rawH))
		if skuCol == -1 && (strings.Contains(h, "sku") || strings.Contains(h, "كود") || strings.Contains(h, "باركود") || strings.Contains(h, "barcode") || strings.Contains(h, "رمز") || strings.Contains(h, "code")) {
			skuCol = idx
		}
		if urlCol == -1 && (strings.Contains(h, "url") || strings.Contains(h, "image") || strings.Contains(h, "صورة") || strings.Contains(h, "link") || strings.Contains(h, "رابط") || strings.Contains(h, "photo") || strings.Contains(h, "img")) {
			urlCol = idx
		}
	}

	if skuCol == -1 && len(headers) > 0 {
		skuCol = 0
	}
	if urlCol == -1 && len(headers) > 1 {
		urlCol = 1
	}

	return skuCol, urlCol
}

// AdminProductImagesSessionPage displays the wizard stage for mapping, progress, or results review.
func (h *UIHandler) AdminProductImagesSessionPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/admin/products/images/import", http.StatusSeeOther)
		return
	}

	sessionID := chi.URLParam(r, "id")
	session, ok := globalAdminImageImportSessionStore.GetSession(sessionID)
	if !ok {
		h.redirectWithNotice(w, r, "/admin/products/images/import", "error", "جلسة استرداد الصور غير موجودة أو انتهت صلاحيتها.")
		return
	}

	noticeType := r.URL.Query().Get("notice_type")
	noticeMsg := r.URL.Query().Get("notice")

	view := pages.AdminProductImagesImportView{
		Session:    session,
		Sessions:   globalAdminImageImportSessionStore.ListSessions(),
		NoticeType: noticeType,
		NoticeMsg:  noticeMsg,
		Actor:      actor,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminProductImagesImportPage(view, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin product images session page", "error", err)
	}
}

// AdminProductImagesMappingSubmit confirms column selection and launches image downloading in the background.
func (h *UIHandler) AdminProductImagesMappingSubmit(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "id")
	session, ok := globalAdminImageImportSessionStore.GetSession(sessionID)
	if !ok {
		h.redirectWithNotice(w, r, "/admin/products/images/import", "error", "جلسة استرداد الصور غير موجودة أو انتهت صلاحيتها.")
		return
	}

	if err := r.ParseForm(); err != nil {
		h.redirectWithNotice(w, r, fmt.Sprintf("/admin/products/images/import/%s", sessionID), "error", "تعذر قراءة النموذج.")
		return
	}

	skuCol, _ := strconv.Atoi(r.PostFormValue("sku_col"))
	urlCol, _ := strconv.Atoi(r.PostFormValue("url_col"))

	if skuCol < 0 || urlCol < 0 || skuCol == urlCol {
		h.redirectWithNotice(w, r, fmt.Sprintf("/admin/products/images/import/%s", sessionID), "error", "يرجى تحديد عمود كود الصنف (SKU) وعمود رابط الصورة (Image URL) بشكل صحيح ومستقل.")
		return
	}

	session.DetectedSKUCol = skuCol
	session.DetectedURLCol = urlCol

	go func() {
		bgCtx := context.Background()
		globalAdminImageImportSessionStore.ProcessImageImport(bgCtx, sessionID, skuCol, urlCol, h.catSvc, h.storage)
	}()

	http.Redirect(w, r, fmt.Sprintf("/admin/products/images/import/%s", sessionID), http.StatusSeeOther)
}

// AdminProductImagesProgressJSON returns the live execution state and stats for real-time progress bar.
func (h *UIHandler) AdminProductImagesProgressJSON(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	sessionID := chi.URLParam(r, "id")
	session, ok := globalAdminImageImportSessionStore.GetSession(sessionID)
	if !ok {
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": "الجلسة غير موجودة"})
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]any{
		"success":        true,
		"phase":          session.Phase,
		"progress":       session.Progress,
		"progress_note":  session.ProgressNote,
		"total_rows":     session.TotalRows,
		"success_rows":   session.SuccessRows,
		"not_found_rows": session.NotFoundRows,
		"error_rows":     session.ErrorRows,
	})
}

// AdminProductImagesCancelSubmit cancels or deletes an import session.
func (h *UIHandler) AdminProductImagesCancelSubmit(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "id")
	globalAdminImageImportSessionStore.DeleteSession(sessionID)
	h.redirectWithNotice(w, r, "/admin/products/images/import", "success", "تم حذف جلسة استرداد الصور بنجاح.")
}

// AdminProductImagesSampleXLSX streams download of a clean Excel template.
func (h *UIHandler) AdminProductImagesSampleXLSX(w http.ResponseWriter, r *http.Request) {
	f := excelize.NewFile()
	sheet := "Sheet1"
	f.SetSheetName("Sheet1", sheet)

	headers := []string{"كود الصنف (SKU)", "رابط صورة المنتج (Image URL)"}
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		_ = f.SetCellValue(sheet, cell, h)
	}

	rows := [][]string{
		{"PAN-500-01", "https://example.com/images/panadol-extra.jpg"},
		{"CAT-050-02", "https://example.com/images/cataflam-50mg.jpg"},
		{"AUG-100-03", "https://example.com/images/augmentin-1g.jpg"},
	}
	for rIdx, row := range rows {
		for cIdx, val := range row {
			cell, _ := excelize.CoordinatesToCellName(cIdx+1, rIdx+2)
			_ = f.SetCellValue(sheet, cell, val)
		}
	}

	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", "attachment; filename=product_images_template.xlsx")
	_ = f.Write(w)
}

// AdminProductImagesSampleCSV streams download of a clean CSV template.
func (h *UIHandler) AdminProductImagesSampleCSV(w http.ResponseWriter, r *http.Request) {
	var buf bytes.Buffer
	buf.WriteString("\xEF\xBB\xBF") // UTF-8 BOM
	cw := csv.NewWriter(&buf)
	_ = cw.Write([]string{"كود الصنف (SKU)", "رابط صورة المنتج (Image URL)"})
	_ = cw.Write([]string{"PAN-500-01", "https://example.com/images/panadol-extra.jpg"})
	_ = cw.Write([]string{"CAT-050-02", "https://example.com/images/cataflam-50mg.jpg"})
	_ = cw.Write([]string{"AUG-100-03", "https://example.com/images/augmentin-1g.jpg"})
	cw.Flush()

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=product_images_template.csv")
	_, _ = w.Write(buf.Bytes())
}
