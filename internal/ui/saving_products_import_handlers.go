package ui

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/xuri/excelize/v2"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/filesecurity"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/shared/pagination"
	"github.com/muhiya/dawa24-store/internal/shared/productmatch"
	"github.com/muhiya/dawa24-store/internal/shared/sheet"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// handleSavingProductsImportPage renders the sessions list and file dropzone for customer or vendor.
func (h *UIHandler) handleSavingProductsImportPage(w http.ResponseWriter, r *http.Request, audience string) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	targetImportURL := fmt.Sprintf("/%s/saving-products/import", audience)
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, fmt.Sprintf("/auth/login?redirect=%s", targetImportURL), http.StatusSeeOther)
		return
	}

	sessions := globalSavingImportSessionStore.ListSessions(actor.OrganizationID)
	noticeType := r.URL.Query().Get("notice_type")
	noticeMsg := r.URL.Query().Get("notice")

	view := pages.SavingImportView{
		AIAvailable:         h.matchEnhancer != nil,
		AIUnavailableReason: savingAIUnavailableReason(h.matchEnhancer, lang),
		Audience:            audience,
		BaseURL:             fmt.Sprintf("/%s/saving-products", audience),
		ImportURL:           targetImportURL,
		Sessions:            sessions,
		NoticeType:          noticeType,
		NoticeMsg:           noticeMsg,
	}

	h.renderPage(ctx, w, fmt.Sprintf("render %s saving import page", audience), pages.SavingImportPage(view, lang, dir))
}

// handleSavingProductsImportUploadSubmit handles file upload and creates new import session for customer or vendor.
func (h *UIHandler) handleSavingProductsImportUploadSubmit(w http.ResponseWriter, r *http.Request, audience string) {
	ctx := r.Context()
	targetImportURL := fmt.Sprintf("/%s/saving-products/import", audience)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, fmt.Sprintf("/auth/login?redirect=%s", targetImportURL), http.StatusSeeOther)
		return
	}

	if err := parseImportUpload(w, r); err != nil {
		h.redirectWithNotice(w, r, targetImportURL, "error", i18n.T(langOf(r), "customer.saving.import.file_too_large_short"))
		return
	}

	file, fileHeader, err := r.FormFile("file")
	if err != nil {
		h.redirectWithNotice(w, r, targetImportURL, "error", i18n.T(langOf(r), "customer.saving.import.select_file"))
		return
	}
	defer file.Close()

	if !SupportedUploadName(fileHeader.Filename) {
		h.redirectWithNotice(w, r, targetImportURL, "error", unsupportedUploadMsg(langOf(r)))
		return
	}

	fileBytes, err := io.ReadAll(file)
	if err != nil || len(fileBytes) == 0 {
		h.redirectWithNotice(w, r, targetImportURL, "error", i18n.T(langOf(r), "customer.saving.import.file_empty"))
		return
	}

	if err := filesecurity.ValidateSpreadsheetSecurity(fileBytes, fileHeader.Filename); err != nil {
		h.redirectWithNotice(w, r, targetImportURL, "error", filesecurity.SecurityErrorMessage)
		return
	}

	rawRows, err := sheet.ReadRows(fileBytes, fileHeader.Filename)
	if err != nil || len(rawRows) < 2 {
		h.log.WarnContext(ctx, "failed to parse spreadsheet", "error", err, "filename", fileHeader.Filename)
		msg := i18n.T(langOf(r), "customer.saving.import.parse_error_short")
		if err != nil && strings.Contains(err.Error(), filesecurity.SecurityErrorMessage) {
			msg = filesecurity.SecurityErrorMessage
		}
		h.redirectWithNotice(w, r, targetImportURL, "error", msg)
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
		h.redirectWithNotice(w, r, targetImportURL, "error", i18n.T(langOf(r), "customer.saving.import.no_data_rows"))
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

	http.Redirect(w, r, fmt.Sprintf("/%s/saving-products/import/%s", audience, session.ID), http.StatusSeeOther)
}

// handleSavingProductsImportSessionPage renders the session at its current wizard phase for customer or vendor.
func (h *UIHandler) handleSavingProductsImportSessionPage(w http.ResponseWriter, r *http.Request, audience string) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	targetImportURL := fmt.Sprintf("/%s/saving-products/import", audience)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, fmt.Sprintf("/auth/login?redirect=%s", targetImportURL), http.StatusSeeOther)
		return
	}

	sessionID := chi.URLParam(r, "id")
	session, ok := globalSavingImportSessionStore.GetSession(sessionID, actor.OrganizationID)
	if !ok {
		h.redirectWithNotice(w, r, targetImportURL, "error", i18n.T(langOf(r), "customer.saving.import.session_not_found"))
		return
	}

	matchFilter := strings.TrimSpace(r.URL.Query().Get("match"))
	sortBy := strings.TrimSpace(r.URL.Query().Get("sort"))
	sortOrder := strings.TrimSpace(r.URL.Query().Get("order"))
	search := strings.TrimSpace(r.URL.Query().Get("q"))

	page := pagination.PageNumber(r)
	limit := pagination.RowsPerPage(r)

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
		AIUnavailableReason: savingAIUnavailableReason(h.matchEnhancer, lang),
		Audience:            audience,
		BaseURL:             fmt.Sprintf("/%s/saving-products", audience),
		ImportURL:           targetImportURL,
		Session:             session,
		Filter:              filter,
		Rows:                rows,
		RowTotal:            total,
		NoticeType:          noticeType,
		NoticeMsg:           noticeMsg,
	}

	h.renderPage(ctx, w, fmt.Sprintf("render %s saving import session page", audience), pages.SavingImportPage(view, lang, dir))
}

// handleSavingProductsImportMapSubmit processes column mapping and performs matching for customer or vendor.
func (h *UIHandler) handleSavingProductsImportMapSubmit(w http.ResponseWriter, r *http.Request, audience string) {
	ctx := r.Context()
	lang := langOf(r)
	targetImportURL := fmt.Sprintf("/%s/saving-products/import", audience)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, fmt.Sprintf("/auth/login?redirect=%s", targetImportURL), http.StatusSeeOther)
		return
	}

	sessionID := chi.URLParam(r, "id")
	session, ok := globalSavingImportSessionStore.GetSession(sessionID, actor.OrganizationID)
	if !ok {
		h.redirectWithNotice(w, r, targetImportURL, "error", i18n.T(lang, "saving.import.session_not_found"))
		return
	}

	var invalidDataMsg string
	if audience == "vendor" {
		invalidDataMsg = i18n.T(lang, "common.invalid_form_data")
	} else {
		invalidDataMsg = i18n.T(lang, "validation.invalid_data")
	}

	if err := r.ParseForm(); err != nil {
		h.redirectWithNotice(w, r, fmt.Sprintf("/%s/saving-products/import/%s", audience, sessionID), "error", invalidDataMsg)
		return
	}

	colName := strings.TrimSpace(r.FormValue("col_name"))
	colSKU := strings.TrimSpace(r.FormValue("col_sku"))
	colQty := strings.TrimSpace(r.FormValue("col_qty"))
	colPrice := strings.TrimSpace(r.FormValue("col_price"))
	matchChoice := ParseMatchChoice(r)
	useAI := ParseUseAI(r)

	nCol, sCol, qCol, pCol := -1, -1, -1, -1
	for idx, hName := range session.Headers {
		if hName == colName {
			nCol = idx
		}
		if hName == colSKU {
			sCol = idx
		}
		if hName == colQty {
			qCol = idx
		}
		if hName == colPrice {
			pCol = idx
		}
	}

	publicID, _, startErr := h.startSavingImportRun(
		ctx, actor, session.Filename,
		append([][]string{session.Headers}, session.RawDataRows...),
		session.Headers, session.SampleRows,
		nCol, sCol, qCol, pCol, -1,
		matchChoice, useAI, langOf(r), audience,
	)
	if startErr != nil {
		h.redirectWithNotice(w, r, fmt.Sprintf("/%s/saving-products/import/%s", audience, sessionID), "error", h.safeMessage(startErr, langOf(r)))
		return
	}

	http.Redirect(w, r, fmt.Sprintf("/%s/saving-products/import/%s", audience, publicID), http.StatusSeeOther)
}

// handleSavingProductsImportItemUpdateSubmit updates staged item details for customer or vendor.
func (h *UIHandler) handleSavingProductsImportItemUpdateSubmit(w http.ResponseWriter, r *http.Request, audience string) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	sessionID := chi.URLParam(r, "id")
	itemIndex, _ := strconv.Atoi(chi.URLParam(r, "itemIndex"))

	if err := r.ParseForm(); err == nil {
		name := strings.TrimSpace(r.FormValue("name"))
		var pricePtr *money.Amount
		if pStr := strings.TrimSpace(r.FormValue("price")); pStr != "" {
			if amt, err := money.Parse(pStr); err == nil {
				pricePtr = &amt
			}
		}
		var qtyPtr *float64
		if qStr := strings.TrimSpace(r.FormValue("quantity")); qStr != "" {
			if q, err := strconv.ParseFloat(qStr, 64); err == nil && q > 0 {
				qtyPtr = &q
			}
		}
		_ = globalSavingImportSessionStore.UpdateStagedItem(sessionID, actor.OrganizationID, itemIndex, name, pricePtr, qtyPtr, nil)
	}

	redirectURI := fmt.Sprintf("/%s/saving-products/import/%s?%s", audience, sessionID, r.URL.RawQuery)
	http.Redirect(w, r, redirectURI, http.StatusSeeOther)
}

// handleSavingProductsImportItemMatchSubmit handles manual link or unlink for customer or vendor.
func (h *UIHandler) handleSavingProductsImportItemMatchSubmit(w http.ResponseWriter, r *http.Request, audience string) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	sessionID := chi.URLParam(r, "id")
	itemIndex, _ := strconv.Atoi(chi.URLParam(r, "itemIndex"))

	noticeMsg := "تم إلغاء ربط الصنف (أصبح غير مرتبط)."
	if err := r.ParseForm(); err == nil {
		productID, _ := strconv.ParseInt(r.FormValue("product_id"), 10, 64)
		masterName := strings.TrimSpace(r.FormValue("master_name"))
		masterSKU := strings.TrimSpace(r.FormValue("master_sku"))
		_ = globalSavingImportSessionStore.AssignStagedItemMatch(sessionID, actor.OrganizationID, itemIndex, productID, masterName, masterSKU)
		if productID > 0 {
			noticeMsg = fmt.Sprintf("تم ربط الصنف «%s» بالكتالوج المركزي.", masterName)
		}
	}

	redirectURI := fmt.Sprintf("/%s/saving-products/import/%s?%s", audience, sessionID, r.URL.RawQuery)
	h.redirectWithNotice(w, r, redirectURI, "success", noticeMsg)
}

// handleSavingProductsImportItemToggleSubmit flips included checkbox for customer or vendor.
func (h *UIHandler) handleSavingProductsImportItemToggleSubmit(w http.ResponseWriter, r *http.Request, audience string) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	sessionID := chi.URLParam(r, "id")
	itemIndex, _ := strconv.Atoi(chi.URLParam(r, "itemIndex"))

	_, _ = globalSavingImportSessionStore.ToggleStagedItem(sessionID, actor.OrganizationID, itemIndex)

	redirectURI := fmt.Sprintf("/%s/saving-products/import/%s?%s", audience, sessionID, r.URL.RawQuery)
	http.Redirect(w, r, redirectURI, http.StatusSeeOther)
}

// handleSavingProductsImportCommitSubmit commits staged items into catalog.saving_products for customer or vendor.
func (h *UIHandler) handleSavingProductsImportCommitSubmit(w http.ResponseWriter, r *http.Request, audience string) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	sessionID := chi.URLParam(r, "id")
	added, updated, err := globalSavingImportSessionStore.CommitSession(ctx, sessionID, actor.OrganizationID, actor.UserID, h.catSvc)
	if err != nil {
		h.redirectWithNotice(w, r, fmt.Sprintf("/%s/saving-products/import/%s", audience, sessionID), "error", i18n.T(langOf(r), "saving.import.save_failed_prefix")+h.safeMessage(err, langOf(r)))
		return
	}

	session, ok := globalSavingImportSessionStore.GetSession(sessionID, actor.OrganizationID)
	if ok && session != nil {
		session.Phase = SavingPhaseCompleted
		session.InsertedCount = added
		session.UpdatedCount = updated
	}

	http.Redirect(w, r, fmt.Sprintf("/%s/saving-products/import/%s", audience, sessionID), http.StatusSeeOther)
}

// handleSavingProductsImportCancelSubmit cancels and cleans up a session for customer or vendor.
func (h *UIHandler) handleSavingProductsImportCancelSubmit(w http.ResponseWriter, r *http.Request, audience string) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	sessionID := chi.URLParam(r, "id")
	globalSavingImportSessionStore.CancelSession(sessionID, actor.OrganizationID)

	h.redirectWithNotice(w, r, fmt.Sprintf("/%s/saving-products/import", audience), "info", i18n.T(langOf(r), "saving.import.cancelled_success"))
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
		{i18n.TDefault("w4_ui.500_69"), "PAN-EXT-24", 50, 48.50},
		{i18n.TDefault("w4_ui.s_70_70"), "CONG-TAB-20", 30, 29.00},
		{i18n.TDefault("w4_ui.1_14_71"), "AUG-1G-14", 20, 110.00},
		{i18n.TDefault("w4_ui.200_72"), "ANT-200-24", 40, 32.00},
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
		i18n.TDefault("w4_ui.500_pan_ext_24_50_48_50_18") +
		i18n.TDefault("w4_ui.cong_tab_20_30_29_00_n_19") +
		i18n.TDefault("w4_ui.1_14_aug_1g_14_20_110_00_20") +
		i18n.TDefault("w4_ui.200_ant_200_24_40_32_00_21")

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=\"saving_products_sample.csv\"")
	_, _ = w.Write([]byte(csvContent))
}

// VendorSavingProductsSampleXLSX streams download of a clean Excel template for vendor.
func (h *UIHandler) VendorSavingProductsSampleXLSX(w http.ResponseWriter, r *http.Request) {
	h.CustomerSavingProductsSampleXLSX(w, r)
}

// VendorSavingProductsSampleCSV streams download of a clean CSV template for vendor.
func (h *UIHandler) VendorSavingProductsSampleCSV(w http.ResponseWriter, r *http.Request) {
	h.CustomerSavingProductsSampleCSV(w, r)
}

// Batch & JSON Handlers

func (h *UIHandler) handleSavingProductsImportSubmit(w http.ResponseWriter, r *http.Request, audience string) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	targetURL := fmt.Sprintf("/%s/saving-products", audience)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, fmt.Sprintf("/auth/login?redirect=%s", targetURL), http.StatusSeeOther)
		return
	}

	if err := r.ParseMultipartForm(uploadMemoryBudget); err != nil {
		h.redirectWithNotice(w, r, targetURL, "error", i18n.T(langOf(r), "customer.saving.import.read_error"))
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		h.redirectWithNotice(w, r, targetURL, "error", i18n.T(langOf(r), "customer.saving.import.select_file"))
		return
	}
	defer file.Close()

	if !SupportedUploadName(header.Filename) {
		h.redirectWithNotice(w, r, targetURL, "error", unsupportedUploadMsg(langOf(r)))
		return
	}

	fileBytes, err := io.ReadAll(file)
	if err != nil || len(fileBytes) == 0 {
		h.redirectWithNotice(w, r, targetURL, "error", i18n.T(langOf(r), "customer.saving.import.file_empty"))
		return
	}

	if err := filesecurity.ValidateSpreadsheetSecurity(fileBytes, header.Filename); err != nil {
		h.redirectWithNotice(w, r, targetURL, "error", filesecurity.SecurityErrorMessage)
		return
	}

	rawRows, err := sheet.ReadRows(fileBytes, header.Filename)
	if err != nil || len(rawRows) < 2 {
		h.log.WarnContext(ctx, "failed to parse spreadsheet", "error", err, "filename", header.Filename)
		msg := i18n.T(langOf(r), "customer.saving.import.parse_error")
		if err != nil && strings.Contains(err.Error(), filesecurity.SecurityErrorMessage) {
			msg = filesecurity.SecurityErrorMessage
		}
		h.redirectWithNotice(w, r, targetURL, "error", msg)
		return
	}

	headers := rawRows[0]
	colNameOverride := strings.TrimSpace(r.FormValue("col_name"))
	colSKUOverride := strings.TrimSpace(r.FormValue("col_sku"))
	colQtyOverride := strings.TrimSpace(r.FormValue("col_qty"))
	colPriceOverride := strings.TrimSpace(r.FormValue("col_price"))
	colProductIDOverride := strings.TrimSpace(r.FormValue("col_product_id"))

	sampleRows := rawRows[1:]
	if len(sampleRows) > 10 {
		sampleRows = sampleRows[:10]
	}

	nameCol, skuCol, qtyCol, priceCol, productIDCol := detectSavingProductColumns(
		headers,
		sampleRows,
		colNameOverride,
		colSKUOverride,
		colQtyOverride,
		colPriceOverride,
		colProductIDOverride,
	)

	matchChoice := ParseMatchChoice(r)

	var matchEngine *SavingProductMatchEngine
	if h.catSvc != nil {
		if catalogSources, err := h.catSvc.ListMatchProducts(ctx); err == nil && len(catalogSources) > 0 {
			matchEngine = NewSavingProductMatchEngine(catalogSources)
		}
	}

	var parsedItems []*catalog.SavingProduct
	var matchedCount int
	var unlinkedCount int

	for i := 1; i < len(rawRows); i++ {
		row := rawRows[i]
		if len(row) == 0 || IsAllEmptyRow(row) || IsSummaryOrTotalRow(row) {
			continue
		}

		var name string
		if nameCol >= 0 && nameCol < len(row) {
			name = strings.TrimSpace(row[nameCol])
		}

		var sku string
		if skuCol >= 0 && skuCol < len(row) {
			sku = strings.TrimSpace(row[skuCol])
		}

		if isAllDigitsOrCode(name) && len(name) >= 4 && isDescriptiveArabicText(sku) {
			name, sku = sku, name
		}

		if name == "" && sku != "" {
			name = sku
		}
		if name == "" {
			continue
		}

		var qty float64
		if qtyCol >= 0 && qtyCol < len(row) {
			qty, _ = ParseFlexibleQuantity(row[qtyCol])
		}

		var price money.Amount
		if priceCol >= 0 && priceCol < len(row) {
			price, _ = ParseFlexibleMoney(row[priceCol])
		}

		var productID *int64
		if productIDCol >= 0 && productIDCol < len(row) {
			if pid, err := strconv.ParseInt(strings.TrimSpace(row[productIDCol]), 10, 64); err == nil && pid > 0 {
				productID = &pid
			}
		}

		if matchEngine != nil {
			matchRes := matchEngine.MatchUnified(matchChoice, productID, sku, name)
			if matchRes.ProductID != nil {
				productID = matchRes.ProductID
			}
		}

		if productID != nil {
			matchedCount++
		} else {
			unlinkedCount++
		}

		parsedItems = append(parsedItems, &catalog.SavingProduct{
			OrganizationID: actor.OrganizationID,
			UserID:         &actor.UserID,
			ProductID:      productID,
			NameProduct:    name,
			SKU:            sku,
			Quantity:       qty,
			Price:          price,
		})
	}

	if len(parsedItems) == 0 {
		h.redirectWithNotice(w, r, targetURL, "error", i18n.T(langOf(r), "customer.saving.import.no_valid_products"))
		return
	}

	added, updated, err := h.catSvc.BatchUpsertSavingProducts(ctx, actor.OrganizationID, &actor.UserID, parsedItems)
	if err != nil {
		h.log.ErrorContext(ctx, fmt.Sprintf("%s failed to batch upsert saving products", audience), "error", err, "org_id", actor.OrganizationID)
		h.redirectWithNotice(w, r, targetURL, "error", fmt.Sprintf(i18n.T(langOf(r), "customer.saving.import.save_error"), h.safeMessage(err, langOf(r))))
		return
	}

	successMsg := fmt.Sprintf(i18n.T(langOf(r), "customer.saving.import.success_summary"), len(parsedItems), added, updated, matchedCount, unlinkedCount)
	h.redirectWithNotice(w, r, targetURL, "success", successMsg)
}

func (h *UIHandler) handleSavingProductsPreviewColumnsJSON(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")

	if err := r.ParseMultipartForm(uploadMemoryBudget); err != nil {
		_ = json.NewEncoder(w).Encode(SavingProductsPreviewResponse{Success: false, Error: i18n.T(langOf(r), "customer.saving.import.file_too_large")})
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		_ = json.NewEncoder(w).Encode(SavingProductsPreviewResponse{Success: false, Error: i18n.T(langOf(r), "customer.saving.import.select_valid_file")})
		return
	}
	defer file.Close()

	fileBytes, err := io.ReadAll(file)
	if err != nil || len(fileBytes) == 0 {
		_ = json.NewEncoder(w).Encode(SavingProductsPreviewResponse{Success: false, Error: i18n.T(langOf(r), "customer.saving.import.read_content_error")})
		return
	}

	if err := filesecurity.ValidateSpreadsheetSecurity(fileBytes, header.Filename); err != nil {
		_ = json.NewEncoder(w).Encode(SavingProductsPreviewResponse{Success: false, Error: filesecurity.SecurityErrorMessage})
		return
	}

	rawRows, err := sheet.ReadRows(fileBytes, header.Filename)
	if err != nil || len(rawRows) == 0 {
		msg := i18n.T(langOf(r), "customer.saving.import.read_sheets_error")
		if err != nil && strings.Contains(err.Error(), filesecurity.SecurityErrorMessage) {
			msg = filesecurity.SecurityErrorMessage
		}
		_ = json.NewEncoder(w).Encode(SavingProductsPreviewResponse{Success: false, Error: msg})
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

	nameCol, skuCol, qtyCol, priceCol, productIDCol := detectSavingProductColumns(headers, sampleRows, "", "", "", "", "")

	_ = json.NewEncoder(w).Encode(SavingProductsPreviewResponse{
		Success: true,
		Headers: headers,
		Detected: SavingDetectedCols{
			NameCol:      nameCol,
			SKUCol:       skuCol,
			QtyCol:       qtyCol,
			PriceCol:     priceCol,
			ProductIDCol: productIDCol,
		},
		SampleRows: sampleRows,
	})
}

func (h *UIHandler) handleSavingProductsImportStartJSON(w http.ResponseWriter, r *http.Request, audience string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": i18n.T(langOf(r), "common.unauthorized")})
		return
	}

	if err := parseImportUpload(w, r); err != nil {
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": i18n.T(langOf(r), "customer.saving.import.file_too_large")})
		return
	}

	file, fileHeader, err := r.FormFile("file")
	if err != nil {
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": i18n.T(langOf(r), "customer.saving.import.select_valid_file")})
		return
	}
	defer file.Close()

	fileBytes, err := io.ReadAll(file)
	if err != nil || len(fileBytes) == 0 {
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": i18n.T(langOf(r), "customer.saving.import.read_content_error")})
		return
	}

	if err := filesecurity.ValidateSpreadsheetSecurity(fileBytes, fileHeader.Filename); err != nil {
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": filesecurity.SecurityErrorMessage})
		return
	}

	rawRows, err := sheet.ReadRows(fileBytes, fileHeader.Filename)
	if err != nil || len(rawRows) <= 1 {
		msg := i18n.T(langOf(r), "customer.saving.import.file_empty_no_rows")
		if err != nil && strings.Contains(err.Error(), filesecurity.SecurityErrorMessage) {
			msg = filesecurity.SecurityErrorMessage
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": msg})
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
		r.FormValue("col_name"),
		r.FormValue("col_sku"),
		r.FormValue("col_qty"),
		r.FormValue("col_price"),
		r.FormValue("col_product_id"),
	)

	matchChoice := ParseMatchChoice(r)
	useAI := ParseUseAI(r)

	sessionID, totalRows, err := h.startSavingImportRun(
		ctx,
		actor,
		fileHeader.Filename,
		rawRows,
		headers,
		sampleRows,
		nameCol, skuCol, qtyCol, priceCol, productIDCol,
		matchChoice,
		useAI,
		langOf(r),
		audience,
	)
	if err != nil {
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": err.Error()})
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]any{
		"success":    true,
		"session_id": sessionID,
		"total_rows": totalRows,
	})
}

func (h *UIHandler) handleSavingProductsImportProgressJSON(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": i18n.T(langOf(r), "common.unauthorized")})
		return
	}

	sessionID := chi.URLParam(r, "id")
	session, ok := globalSavingImportSessionStore.GetSession(sessionID, actor.OrganizationID)
	if !ok {
		if h.importRunRepo != nil {
			run, err := h.importRunRepo.GetRunByPublicID(ctx, sessionID, actor.OrganizationID)
			if err == nil && run != nil {
				h.respondWithRunProgress(w, r, run)
				return
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": i18n.T(langOf(r), "customer.saving.import.session_not_found")})
		return
	}

	session.Success = true
	_ = json.NewEncoder(w).Encode(session)
}

func (h *UIHandler) handleSavingProductsImportCommitJSON(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": i18n.T(langOf(r), "common.unauthorized")})
		return
	}

	sessionID := chi.URLParam(r, "id")
	added, updated, err := h.commitSavingImportRun(ctx, sessionID, actor.OrganizationID, actor.UserID, h.catSvc)
	if err != nil {
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": fmt.Sprintf(i18n.T(langOf(r), "customer.saving.import.commit_error"), h.safeMessage(err, langOf(r)))})
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]any{
		"success": true,
		"added":   added,
		"updated": updated,
		"message": fmt.Sprintf(i18n.T(langOf(r), "customer.saving.import.commit_success"), added+updated, added, updated),
	})
}

func (h *UIHandler) handleSavingProductsImportCancelJSON(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": i18n.T(langOf(r), "common.unauthorized")})
		return
	}

	sessionID := chi.URLParam(r, "id")
	cancelled := globalSavingImportSessionStore.CancelSession(sessionID, actor.OrganizationID)
	_ = json.NewEncoder(w).Encode(map[string]any{"success": cancelled})
}

// Customer Saving Products Import Handlers (public delegates)

func (h *UIHandler) CustomerSavingProductsImportPage(w http.ResponseWriter, r *http.Request) {
	h.handleSavingProductsImportPage(w, r, "customer")
}

func (h *UIHandler) CustomerSavingProductsImportUploadSubmit(w http.ResponseWriter, r *http.Request) {
	h.handleSavingProductsImportUploadSubmit(w, r, "customer")
}

func (h *UIHandler) CustomerSavingProductsImportSessionPage(w http.ResponseWriter, r *http.Request) {
	h.handleSavingProductsImportSessionPage(w, r, "customer")
}

func (h *UIHandler) CustomerSavingProductsImportMapSubmit(w http.ResponseWriter, r *http.Request) {
	h.handleSavingProductsImportMapSubmit(w, r, "customer")
}

func (h *UIHandler) CustomerSavingProductsImportItemUpdateSubmit(w http.ResponseWriter, r *http.Request) {
	h.handleSavingProductsImportItemUpdateSubmit(w, r, "customer")
}

func (h *UIHandler) CustomerSavingProductsImportItemMatchSubmit(w http.ResponseWriter, r *http.Request) {
	h.handleSavingProductsImportItemMatchSubmit(w, r, "customer")
}

func (h *UIHandler) CustomerSavingProductsImportItemToggleSubmit(w http.ResponseWriter, r *http.Request) {
	h.handleSavingProductsImportItemToggleSubmit(w, r, "customer")
}

func (h *UIHandler) CustomerSavingProductsImportCommitSubmit(w http.ResponseWriter, r *http.Request) {
	h.handleSavingProductsImportCommitSubmit(w, r, "customer")
}

func (h *UIHandler) CustomerSavingProductsImportCancelSubmit(w http.ResponseWriter, r *http.Request) {
	h.handleSavingProductsImportCancelSubmit(w, r, "customer")
}

func (h *UIHandler) CustomerSavingProductsImportSubmit(w http.ResponseWriter, r *http.Request) {
	h.handleSavingProductsImportSubmit(w, r, "customer")
}

func (h *UIHandler) CustomerSavingProductsPreviewColumnsJSON(w http.ResponseWriter, r *http.Request) {
	h.handleSavingProductsPreviewColumnsJSON(w, r)
}

func (h *UIHandler) CustomerSavingProductsImportStartJSON(w http.ResponseWriter, r *http.Request) {
	h.handleSavingProductsImportStartJSON(w, r, "customer")
}

func (h *UIHandler) CustomerSavingProductsImportProgressJSON(w http.ResponseWriter, r *http.Request) {
	h.handleSavingProductsImportProgressJSON(w, r)
}

func (h *UIHandler) CustomerSavingProductsImportCommitJSON(w http.ResponseWriter, r *http.Request) {
	h.handleSavingProductsImportCommitJSON(w, r)
}

func (h *UIHandler) CustomerSavingProductsImportCancelJSON(w http.ResponseWriter, r *http.Request) {
	h.handleSavingProductsImportCancelJSON(w, r)
}

// Vendor Saving Products Import Handlers (public delegates)

func (h *UIHandler) VendorSavingProductsImportPage(w http.ResponseWriter, r *http.Request) {
	h.handleSavingProductsImportPage(w, r, "vendor")
}

func (h *UIHandler) VendorSavingProductsImportUploadSubmit(w http.ResponseWriter, r *http.Request) {
	h.handleSavingProductsImportUploadSubmit(w, r, "vendor")
}

func (h *UIHandler) VendorSavingProductsImportSessionPage(w http.ResponseWriter, r *http.Request) {
	h.handleSavingProductsImportSessionPage(w, r, "vendor")
}

func (h *UIHandler) VendorSavingProductsImportMapSubmit(w http.ResponseWriter, r *http.Request) {
	h.handleSavingProductsImportMapSubmit(w, r, "vendor")
}

func (h *UIHandler) VendorSavingProductsImportItemUpdateSubmit(w http.ResponseWriter, r *http.Request) {
	h.handleSavingProductsImportItemUpdateSubmit(w, r, "vendor")
}

func (h *UIHandler) VendorSavingProductsImportItemMatchSubmit(w http.ResponseWriter, r *http.Request) {
	h.handleSavingProductsImportItemMatchSubmit(w, r, "vendor")
}

func (h *UIHandler) VendorSavingProductsImportItemToggleSubmit(w http.ResponseWriter, r *http.Request) {
	h.handleSavingProductsImportItemToggleSubmit(w, r, "vendor")
}

func (h *UIHandler) VendorSavingProductsImportCommitSubmit(w http.ResponseWriter, r *http.Request) {
	h.handleSavingProductsImportCommitSubmit(w, r, "vendor")
}

func (h *UIHandler) VendorSavingProductsImportCancelSubmit(w http.ResponseWriter, r *http.Request) {
	h.handleSavingProductsImportCancelSubmit(w, r, "vendor")
}

func (h *UIHandler) VendorSavingProductsImportSubmit(w http.ResponseWriter, r *http.Request) {
	h.handleSavingProductsImportSubmit(w, r, "vendor")
}

func (h *UIHandler) VendorSavingProductsPreviewColumnsJSON(w http.ResponseWriter, r *http.Request) {
	h.handleSavingProductsPreviewColumnsJSON(w, r)
}

func (h *UIHandler) VendorSavingProductsImportStartJSON(w http.ResponseWriter, r *http.Request) {
	h.handleSavingProductsImportStartJSON(w, r, "vendor")
}

func (h *UIHandler) VendorSavingProductsImportProgressJSON(w http.ResponseWriter, r *http.Request) {
	h.handleSavingProductsImportProgressJSON(w, r)
}

func (h *UIHandler) VendorSavingProductsImportCommitJSON(w http.ResponseWriter, r *http.Request) {
	h.handleSavingProductsImportCommitJSON(w, r)
}

func (h *UIHandler) VendorSavingProductsImportCancelJSON(w http.ResponseWriter, r *http.Request) {
	h.handleSavingProductsImportCancelJSON(w, r)
}

func isAllDigitsOrCode(s string) bool {
	if s == "" {
		return false
	}
	digits := 0
	for _, r := range s {
		if r >= '0' && r <= '9' {
			digits++
		} else if r != '-' && r != '_' && r != '.' && r != '/' && r != ' ' {
			return false
		}
	}
	return digits > 0
}

func isDescriptiveArabicText(s string) bool {
	if len(s) < 3 {
		return false
	}
	hasArabic := false
	hasSpace := false
	for _, r := range s {
		if (r >= 0x0600 && r <= 0x06FF) || (r >= 0x0750 && r <= 0x077F) {
			hasArabic = true
		}
		if r == ' ' {
			hasSpace = true
		}
	}
	return hasArabic && hasSpace
}
