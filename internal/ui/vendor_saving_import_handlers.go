package ui

import (
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/sheet"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// VendorSavingProductsImportPage renders the sessions list and file dropzone for vendor.
func (h *UIHandler) VendorSavingProductsImportPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/saving-products/import", http.StatusSeeOther)
		return
	}

	sessions := globalSavingImportSessionStore.ListSessions(actor.OrganizationID)
	noticeType := r.URL.Query().Get("notice_type")
	noticeMsg := r.URL.Query().Get("notice")

	view := pages.SavingImportView{
		AIAvailable:         h.matchEnhancer != nil,
		AIUnavailableReason: savingAIUnavailableReason(h.matchEnhancer),
		Audience:            "vendor",
		BaseURL:             "/vendor/saving-products",
		ImportURL:           "/vendor/saving-products/import",
		Sessions:            sessions,
		NoticeType:          noticeType,
		NoticeMsg:           noticeMsg,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.SavingImportPage(view, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render vendor saving import page", "error", err)
	}
}

// VendorSavingProductsImportUploadSubmit handles file upload and creates new import session for vendor.
func (h *UIHandler) VendorSavingProductsImportUploadSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/saving-products/import", http.StatusSeeOther)
		return
	}

	if err := r.ParseMultipartForm(32 << 20); err != nil {
		h.redirectWithNotice(w, r, "/vendor/saving-products/import", "error", "الملف المرفوع كبير جداً أو غير صالح.")
		return
	}

	file, fileHeader, err := r.FormFile("file")
	if err != nil {
		h.redirectWithNotice(w, r, "/vendor/saving-products/import", "error", "يرجى اختيار ملف Excel أو CSV صالح.")
		return
	}
	defer file.Close()

	ext := strings.ToLower(filepath.Ext(fileHeader.Filename))
	if ext != ".xlsx" && ext != ".xls" && ext != ".csv" {
		h.redirectWithNotice(w, r, "/vendor/saving-products/import", "error", "صيغة الملف غير مدعومة. يرجى رفع ملف بصيغة xlsx أو xls أو csv.")
		return
	}

	fileBytes, err := io.ReadAll(file)
	if err != nil || len(fileBytes) == 0 {
		h.redirectWithNotice(w, r, "/vendor/saving-products/import", "error", "الملف فارغ أو تعذر قراءته.")
		return
	}

	rawRows, err := sheet.ReadRows(fileBytes, fileHeader.Filename)
	if err != nil || len(rawRows) < 2 {
		h.log.WarnContext(ctx, "failed to parse spreadsheet", "error", err, "filename", fileHeader.Filename)
		h.redirectWithNotice(w, r, "/vendor/saving-products/import", "error", "تعذر قراءة ملف البيانات المرفوع أو أن الملف لا يحتوي على صفوف بيانات.")
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

	http.Redirect(w, r, fmt.Sprintf("/vendor/saving-products/import/%s", session.ID), http.StatusSeeOther)
}

// VendorSavingProductsImportSessionPage renders the session at its current wizard phase for vendor.
func (h *UIHandler) VendorSavingProductsImportSessionPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/saving-products/import", http.StatusSeeOther)
		return
	}

	sessionID := chi.URLParam(r, "id")
	session, ok := globalSavingImportSessionStore.GetSession(sessionID, actor.OrganizationID)
	if !ok {
		h.redirectWithNotice(w, r, "/vendor/saving-products/import", "error", "جلسة الاستيراد غير موجودة أو انتهت صلاحيتها.")
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
		Audience:            "vendor",
		BaseURL:             "/vendor/saving-products",
		ImportURL:           "/vendor/saving-products/import",
		Session:             session,
		Filter:              filter,
		Rows:                rows,
		RowTotal:            total,
		NoticeType:          noticeType,
		NoticeMsg:           noticeMsg,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.SavingImportPage(view, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render vendor saving import session page", "error", err)
	}
}

// VendorSavingProductsSampleXLSX streams download of a clean Excel template.
func (h *UIHandler) VendorSavingProductsSampleXLSX(w http.ResponseWriter, r *http.Request) {
	h.CustomerSavingProductsSampleXLSX(w, r)
}

// VendorSavingProductsSampleCSV streams download of a clean CSV template.
func (h *UIHandler) VendorSavingProductsSampleCSV(w http.ResponseWriter, r *http.Request) {
	h.CustomerSavingProductsSampleCSV(w, r)
}
