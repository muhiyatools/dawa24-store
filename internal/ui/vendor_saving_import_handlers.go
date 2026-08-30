package ui

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/productmatch"
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

	h.renderPage(ctx, w, "render vendor saving import page", pages.SavingImportPage(view, lang, dir))
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
		h.redirectWithNotice(w, r, "/vendor/saving-products/import", "error", i18n.T(langOf(r), "customer.saving.import.file_too_large_short"))
		return
	}

	file, fileHeader, err := r.FormFile("file")
	if err != nil {
		h.redirectWithNotice(w, r, "/vendor/saving-products/import", "error", i18n.T(langOf(r), "customer.saving.import.select_file"))
		return
	}
	defer file.Close()

	if !SupportedUploadName(fileHeader.Filename) {
		h.redirectWithNotice(w, r, "/vendor/saving-products/import", "error", unsupportedUploadMsg(langOf(r)))
		return
	}

	fileBytes, err := io.ReadAll(file)
	if err != nil || len(fileBytes) == 0 {
		h.redirectWithNotice(w, r, "/vendor/saving-products/import", "error", i18n.T(langOf(r), "customer.saving.import.file_empty"))
		return
	}

	rawRows, err := sheet.ReadRows(fileBytes, fileHeader.Filename)
	if err != nil || len(rawRows) < 2 {
		h.log.WarnContext(ctx, "failed to parse spreadsheet", "error", err, "filename", fileHeader.Filename)
		h.redirectWithNotice(w, r, "/vendor/saving-products/import", "error", i18n.T(langOf(r), "customer.saving.import.parse_error_short"))
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
		h.redirectWithNotice(w, r, "/vendor/saving-products/import", "error", i18n.T(langOf(r), "customer.saving.import.no_data_rows"))
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
		h.redirectWithNotice(w, r, "/vendor/saving-products/import", "error", i18n.T(langOf(r), "customer.saving.import.session_not_found"))
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

	h.renderPage(ctx, w, "render vendor saving import session page", pages.SavingImportPage(view, lang, dir))
}

// VendorSavingProductsSampleXLSX streams download of a clean Excel template.
func (h *UIHandler) VendorSavingProductsSampleXLSX(w http.ResponseWriter, r *http.Request) {
	h.CustomerSavingProductsSampleXLSX(w, r)
}

// VendorSavingProductsSampleCSV streams download of a clean CSV template.
func (h *UIHandler) VendorSavingProductsSampleCSV(w http.ResponseWriter, r *http.Request) {
	h.CustomerSavingProductsSampleCSV(w, r)
}
