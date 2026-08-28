package ui

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/xuri/excelize/v2"

	"github.com/muhiya/dawa24-store/internal/modules/compare"
	"github.com/muhiya/dawa24-store/internal/modules/inventory"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/shared/spreadsheet"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// AdminWarehousesPage renders warehouse registry for fulfillment network.
func (h *UIHandler) AdminWarehousesPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	var warehouses []*inventory.Warehouse
	if h.invSvc != nil {
		warehouses, _ = h.invSvc.ListWarehouses(database.AsSystem(ctx))
	}

	var orgs []*org.Organization
	if h.orgSvc != nil {
		orgs, _ = h.orgSvc.ListOrganizations(database.AsSystem(ctx), nil, nil, 500, 0)
	}
	orgMap := make(map[int64]string)
	for _, o := range orgs {
		if o != nil {
			orgMap[o.ID] = o.LegalName
		}
	}

	var rows []*pages.AdminWarehouseRowView
	for _, wh := range warehouses {
		if wh != nil {
			rows = append(rows, &pages.AdminWarehouseRowView{
				Warehouse: wh,
				OrgName:   orgMap[wh.OrganizationID],
			})
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminWarehousesPage(rows, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin warehouses page", "error", err)
	}
}

// AdminWarehouseDetailPage renders full warehouse detail and searchable/paginated stocks page.
func (h *UIHandler) AdminWarehouseDetailPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	idStr := chi.URLParam(r, "id")
	whID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || whID <= 0 {
		http.Redirect(w, r, "/admin/warehouses", http.StatusSeeOther)
		return
	}

	var wh *inventory.Warehouse
	if h.invSvc != nil {
		whs, _ := h.invSvc.ListWarehouses(database.AsSystem(ctx))
		for _, item := range whs {
			if item != nil && item.ID == whID {
				wh = item
				break
			}
		}
	}

	if wh == nil {
		http.Redirect(w, r, "/admin/warehouses", http.StatusSeeOther)
		return
	}

	var orgName string
	if h.orgSvc != nil && wh.OrganizationID > 0 {
		if o, err := h.orgSvc.GetOrganization(database.AsSystem(ctx), wh.OrganizationID); err == nil && o != nil {
			orgName = o.LegalName
		}
	}

	var allStocks []*inventory.DetailedWarehouseStockView
	if h.invSvc != nil {
		allStocks, _ = h.invSvc.ListDetailedStocksByWarehouse(database.AsSystem(ctx), whID)
	}
	if allStocks == nil {
		allStocks = []*inventory.DetailedWarehouseStockView{}
	}

	// Calculate overall stats before filtering
	totalUnits := 0
	availableCount := 0
	lowStockCount := 0
	outOfStockCount := 0

	for _, s := range allStocks {
		if s == nil {
			continue
		}
		totalUnits += s.Quantity
		if s.Quantity > s.MinThreshold {
			availableCount++
		} else if s.Quantity > 0 {
			lowStockCount++
		} else {
			outOfStockCount++
		}
	}

	// Parse query filters
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	qLower := strings.ToLower(q)
	statusFilter := strings.TrimSpace(r.URL.Query().Get("status"))
	if statusFilter == "" {
		statusFilter = "all"
	}
	negotiableFilter := strings.TrimSpace(r.URL.Query().Get("negotiable"))
	if negotiableFilter == "" {
		negotiableFilter = "all"
	}

	var filtered []*inventory.DetailedWarehouseStockView
	for _, s := range allStocks {
		if s == nil {
			continue
		}
		// Search query filter
		if qLower != "" {
			match := strings.Contains(strings.ToLower(s.ProductName), qLower) ||
				strings.Contains(strings.ToLower(s.VariantName), qLower) ||
				strings.Contains(strings.ToLower(s.SKU), qLower) ||
				strings.Contains(strings.ToLower(s.Barcode), qLower) ||
				strings.Contains(strings.ToLower(s.BatchNumber), qLower)
			if !match {
				continue
			}
		}

		// Status filter
		switch statusFilter {
		case "available":
			if s.Quantity <= s.MinThreshold {
				continue
			}
		case "low":
			if s.Quantity <= 0 || s.Quantity > s.MinThreshold {
				continue
			}
		case "out":
			if s.Quantity > 0 {
				continue
			}
		}

		// Negotiable filter
		switch negotiableFilter {
		case "yes":
			if !s.IsNegotiable {
				continue
			}
		case "no":
			if s.IsNegotiable {
				continue
			}
		}

		filtered = append(filtered, s)
	}

	// Pagination
	page := 1
	if pStr := r.URL.Query().Get("page"); pStr != "" {
		if p, err := strconv.Atoi(pStr); err == nil && p > 0 {
			page = p
		}
	}

	limit := 25
	if lStr := r.URL.Query().Get("limit"); lStr != "" {
		if l, err := strconv.Atoi(lStr); err == nil && l > 0 {
			if l == 25 || l == 50 || l == 100 || l == 250 {
				limit = l
			}
		}
	}

	totalFiltered := len(filtered)
	totalPages := (totalFiltered + limit - 1) / limit
	if totalPages < 1 {
		totalPages = 1
	}
	if page > totalPages {
		page = totalPages
	}

	offset := (page - 1) * limit
	var paginatedItems []*inventory.DetailedWarehouseStockView
	if offset < totalFiltered {
		end := offset + limit
		if end > totalFiltered {
			end = totalFiltered
		}
		paginatedItems = filtered[offset:end]
	}

	data := pages.AdminWarehouseDetailView{
		Warehouse:        wh,
		OrgName:          orgName,
		Items:            paginatedItems,
		TotalItems:       len(allStocks),
		TotalUnits:       totalUnits,
		AvailableCount:   availableCount,
		LowStockCount:    lowStockCount,
		OutOfStockCount:  outOfStockCount,
		SearchQuery:      q,
		StatusFilter:     statusFilter,
		NegotiableFilter: negotiableFilter,
		CurrentPage:      page,
		PageSize:         limit,
		TotalCount:       totalFiltered,
		QueryValues:      r.URL.Query(),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminWarehouseDetailPage(data, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin warehouse detail page", "error", err)
	}
}

// AdminWarehouseStocksJSON provides detailed stock rows for interactive inspection.
func (h *UIHandler) AdminWarehouseStocksJSON(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	idStr := chi.URLParam(r, "id")
	whID, _ := strconv.ParseInt(idStr, 10, 64)

	var stocks []*inventory.DetailedWarehouseStockView
	if h.invSvc != nil && whID > 0 {
		stocks, _ = h.invSvc.ListDetailedStocksByWarehouse(database.AsSystem(ctx), whID)
	}

	if stocks == nil {
		stocks = []*inventory.DetailedWarehouseStockView{}
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(stocks)
}

// AdminTempWarehousesPage renders temporary warehouses staging directory with database integration.
func (h *UIHandler) AdminTempWarehousesPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	query := strings.TrimSpace(r.URL.Query().Get("q"))
	statusFilter := strings.TrimSpace(r.URL.Query().Get("status"))
	scope := strings.TrimSpace(r.URL.Query().Get("scope"))
	noticeMsg := strings.TrimSpace(r.URL.Query().Get("notice"))
	noticeType := strings.TrimSpace(r.URL.Query().Get("notice_type"))

	var statusPtr *compare.CompareFileStatus
	if statusFilter != "" {
		s := compare.CompareFileStatus(statusFilter)
		statusPtr = &s
	}

	var files []*compare.CompareFile
	var err error
	if h.compareSvc != nil {
		files, err = h.compareSvc.ListAllFiles(database.AsSystem(ctx), query, statusPtr)
		if err != nil {
			h.log.ErrorContext(ctx, "list all temp warehouse files", "error", err)
		}
	}

	var totalRows int64
	var activeCount int
	var archivedCount int
	var items []*pages.AdminTempWarehouseItem

	for _, f := range files {
		if f == nil {
			continue
		}
		totalRows += int64(f.RowCount)
		if f.Status == compare.FileReady {
			activeCount++
		} else if f.Status == compare.FileArchived {
			archivedCount++
		}

		items = append(items, &pages.AdminTempWarehouseItem{
			ID:               f.ID,
			SupplierName:     f.SupplierName,
			OriginalFilename: f.OriginalFilename,
			RowCount:         f.RowCount,
			SizeBytes:        f.SizeBytes,
			Status:           string(f.Status),
			CreatedBy:        &f.UserID,
			CreatedAt:        f.CreatedAt,
			ArchivedAt:       f.ArchivedAt,
		})
	}

	data := &pages.AdminTempWarehousesData{
		Items:         items,
		TotalCount:    len(items),
		TotalRows:     totalRows,
		ActiveCount:   activeCount,
		ArchivedCount: archivedCount,
		Query:         query,
		StatusFilter:  statusFilter,
		Scope:         scope,
		NoticeMsg:     noticeMsg,
		NoticeType:    noticeType,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminTempWarehousesPage(data, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render temp warehouses", "error", err)
	}
}

// AdminTempWarehouseItemsJSON returns paginated items for a warehouse in JSON for the items modal.
func (h *UIHandler) AdminTempWarehouseItemsJSON(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	idStr := chi.URLParam(r, "id")
	fileID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || fileID <= 0 {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": "Invalid warehouse ID"})
		return
	}

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page <= 0 {
		page = 1
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 100 {
		limit = 10
	}

	rows, total, err := h.compareSvc.GetFileRowsPaginated(database.AsSystem(ctx), fileID, page, limit)
	if err != nil {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": err.Error()})
		return
	}

	type jsonRow struct {
		ID                 int64  `json:"id"`
		SKU                string `json:"sku"`
		RawName            string `json:"raw_name"`
		Price              string `json:"price"`
		Discount           string `json:"discount"`
		PriceAfterDiscount string `json:"price_after_discount"`
	}

	resRows := make([]jsonRow, 0, len(rows))
	for _, r := range rows {
		if r == nil {
			continue
		}
		resRows = append(resRows, jsonRow{
			ID:                 r.ID,
			SKU:                r.SKU,
			RawName:            r.RawName,
			Price:              r.Price.String(),
			Discount:           fmt.Sprintf("%.2f", r.Discount),
			PriceAfterDiscount: r.PriceAfterDiscount.String(),
		})
	}

	totalPages := int(math.Ceil(float64(total) / float64(limit)))
	if totalPages < 1 {
		totalPages = 1
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success":     true,
		"items":       resRows,
		"total":       total,
		"page":        page,
		"limit":       limit,
		"total_pages": totalPages,
	})
}

// AdminTempWarehouseItemDeleteSubmit deletes a single product row from inside a warehouse.
func (h *UIHandler) AdminTempWarehouseItemDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	idStr := chi.URLParam(r, "id")
	rowID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || rowID <= 0 {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": "Invalid item ID"})
		return
	}

	if err := h.compareSvc.DeleteFileRow(database.AsSystem(ctx), rowID); err != nil {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]any{"success": true})
}

// AdminTempWarehouseDeleteSubmit deletes an entire temporary warehouse and all its rows.
func (h *UIHandler) AdminTempWarehouseDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	idStr := chi.URLParam(r, "id")
	fileID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || fileID <= 0 {
		h.redirectWithNotice(w, r, "/admin/user/temparte-warehouses", "error", "رقم المستودع غير صحيح.")
		return
	}

	if err := h.compareSvc.DeleteFile(database.AsSystem(ctx), fileID); err != nil {
		h.log.ErrorContext(ctx, "delete temp warehouse", "error", err, "file_id", fileID)
		h.redirectWithNotice(w, r, "/admin/user/temparte-warehouses", "error", "فشل حذف المستودع: "+err.Error())
		return
	}

	h.redirectWithNotice(w, r, "/admin/user/temparte-warehouses", "success", "تم حذف المستودع وكافة أصنافه بنجاح.")
}

// AdminTempWarehouseToggleArchiveSubmit toggles warehouse archive status.
func (h *UIHandler) AdminTempWarehouseToggleArchiveSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	idStr := chi.URLParam(r, "id")
	fileID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || fileID <= 0 {
		h.redirectWithNotice(w, r, "/admin/user/temparte-warehouses", "error", "رقم المستودع غير صحيح.")
		return
	}

	f, err := h.compareSvc.GetFile(database.AsSystem(ctx), fileID)
	if err != nil || f == nil {
		h.redirectWithNotice(w, r, "/admin/user/temparte-warehouses", "error", "لم يتم العثور على المستودع المطلوب.")
		return
	}

	if f.Status == compare.FileArchived {
		if err := h.compareSvc.UnarchiveFile(database.AsSystem(ctx), fileID); err != nil {
			h.redirectWithNotice(w, r, "/admin/user/temparte-warehouses", "error", "فشل تفعيل المستودع: "+err.Error())
			return
		}
		h.redirectWithNotice(w, r, "/admin/user/temparte-warehouses", "success", "تم تفعيل المستودع وإتاحته بالخصومات بنجاح.")
	} else {
		if err := h.compareSvc.ArchiveFile(database.AsSystem(ctx), fileID, "أرشفة يدوية من لوحة المشرف"); err != nil {
			h.redirectWithNotice(w, r, "/admin/user/temparte-warehouses", "error", "فشل أرشفة المستودع: "+err.Error())
			return
		}
		h.redirectWithNotice(w, r, "/admin/user/temparte-warehouses", "success", "تم أرشفة المستودع بنجاح.")
	}
}

// AdminTempWarehouseExportXLSX streams all warehouse rows as an Excel file.
func (h *UIHandler) AdminTempWarehouseExportXLSX(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	idStr := chi.URLParam(r, "id")
	fileID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || fileID <= 0 {
		http.Error(w, "Invalid warehouse ID", http.StatusBadRequest)
		return
	}

	fMeta, err := h.compareSvc.GetFile(database.AsSystem(ctx), fileID)
	if err != nil || fMeta == nil {
		http.Error(w, "Warehouse not found", http.StatusNotFound)
		return
	}

	rows, err := h.compareSvc.ListFileRows(database.AsSystem(ctx), fileID, 50000, 0)
	if err != nil {
		http.Error(w, "Failed to load warehouse items", http.StatusInternalServerError)
		return
	}

	f := excelize.NewFile()
	sheet := "Warehouse Items"
	f.SetSheetName("Sheet1", sheet)
	_ = f.SetSheetView(sheet, 0, &excelize.ViewOptions{
		RightToLeft: func(b bool) *bool { return &b }(true),
	})

	headers := []string{"كود الصنف (SKU)", "اسم المنتج / الصنف", "سعر الجمهور (ج.م)", "نسبة الخصم (%)", "السعر الإجمالي بعد الخصم (ج.م)"}
	for colIdx, header := range headers {
		cell, _ := excelize.CoordinatesToCellName(colIdx+1, 1)
		_ = f.SetCellValue(sheet, cell, header)
	}

	for rowIdx, row := range rows {
		if row == nil {
			continue
		}
		rNum := rowIdx + 2
		_ = f.SetCellValue(sheet, fmt.Sprintf("A%d", rNum), row.SKU)
		_ = f.SetCellValue(sheet, fmt.Sprintf("B%d", rNum), row.RawName)
		_ = f.SetCellValue(sheet, fmt.Sprintf("C%d", rNum), row.Price.String())
		_ = f.SetCellValue(sheet, fmt.Sprintf("D%d", rNum), fmt.Sprintf("%.2f%%", row.Discount))
		_ = f.SetCellValue(sheet, fmt.Sprintf("E%d", rNum), row.PriceAfterDiscount.String())
	}

	safeName := strings.ReplaceAll(fMeta.SupplierName, " ", "_")
	filename := fmt.Sprintf("warehouse_%s_%s.xlsx", safeName, time.Now().Format("20060102"))

	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	_ = f.Write(w)
}

// AdminTempWarehouseUploadSubmit handles file upload, auto column mapping, and item persistence.
func (h *UIHandler) AdminTempWarehouseUploadSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if err := r.ParseMultipartForm(50 << 20); err != nil { // 50MB max
		h.redirectWithNotice(w, r, "/admin/user/temparte-warehouses", "error", "حجم الملف المرفوع كبير جداً.")
		return
	}

	supplierName := strings.TrimSpace(r.FormValue("supplier_name"))
	if supplierName == "" {
		h.redirectWithNotice(w, r, "/admin/user/temparte-warehouses", "error", "يرجى كتابة اسم المورد أو المستودع.")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		h.redirectWithNotice(w, r, "/admin/user/temparte-warehouses", "error", "يرجى اختيار ملف صحيح للرفع.")
		return
	}
	defer file.Close()

	fileBytes, err := io.ReadAll(file)
	if err != nil {
		h.redirectWithNotice(w, r, "/admin/user/temparte-warehouses", "error", "فشل قراءة بيانات الملف.")
		return
	}

	rawRows, err := spreadsheet.ReadRows(fileBytes)
	if err != nil || len(rawRows) < 2 {
		h.redirectWithNotice(w, r, "/admin/user/temparte-warehouses", "error", "فشل تحليل ملف الإكسيل أو الملف لا يحتوي على صفوف بيانات كافية.")
		return
	}

	headers := rawRows[0]
	customCode := r.FormValue("col_code")
	customName := r.FormValue("col_name")
	customPrice := r.FormValue("col_price")
	customDiscount := r.FormValue("col_discount")

	codeCol, nameCol, priceCol, discountCol := detectTempWarehouseCols(headers, customCode, customName, customPrice, customDiscount)

	// Create warehouse file in database
	compareFile := &compare.CompareFile{
		SupplierName:     supplierName,
		OriginalFilename: header.Filename,
		SizeBytes:        int64(len(fileBytes)),
		Status:           compare.FileReady,
		MappingConfig: compare.MappingConfig{
			CodeCol:     &codeCol,
			NameCol:     &nameCol,
			PriceCol:    &priceCol,
			DiscountCol: &discountCol,
		},
	}

	if err := h.compareSvc.CreateFile(database.AsSystem(ctx), compareFile); err != nil {
		h.log.ErrorContext(ctx, "create temp warehouse file record", "error", err)
		h.redirectWithNotice(w, r, "/admin/user/temparte-warehouses", "error", "فشل حفظ بيانات المستودع في قاعدة البيانات: "+err.Error())
		return
	}

	// Parse file rows
	var fileRows []*compare.CompareFileRow
	for idx, row := range rawRows[1:] {
		if len(row) == 0 {
			continue
		}

		rawName := ""
		if nameCol >= 0 && nameCol < len(row) {
			rawName = strings.TrimSpace(row[nameCol])
		}
		if rawName == "" {
			continue
		}

		sku := ""
		if codeCol >= 0 && codeCol < len(row) {
			sku = strings.TrimSpace(row[codeCol])
		}

		priceMinor := int64(0)
		if priceCol >= 0 && priceCol < len(row) {
			if p, err := parsePriceFloat(row[priceCol]); err == nil && p > 0 {
				priceMinor = int64(math.Round(p * 100))
			}
		}

		discountPct := 0.0
		if discountCol >= 0 && discountCol < len(row) {
			if d, err := parsePriceFloat(row[discountCol]); err == nil && d >= 0 {
				discountPct = d
				if discountPct > 100 {
					discountPct = 100
				}
			}
		}

		priceMoney := money.FromMinor(priceMinor)
		priceAfterMinor := int64(math.Round(float64(priceMinor) * (1.0 - (discountPct / 100.0))))
		priceAfterMoney := money.FromMinor(priceAfterMinor)

		fileRows = append(fileRows, &compare.CompareFileRow{
			FileID:             compareFile.ID,
			RowNumber:          idx + 2,
			RawName:            rawName,
			NormalizedName:     strings.ToLower(rawName),
			SKU:                sku,
			Price:              priceMoney,
			Discount:           discountPct,
			PriceAfterDiscount: priceAfterMoney,
		})
	}

	if len(fileRows) > 0 {
		if err := h.compareSvc.InsertFileRows(database.AsSystem(ctx), fileRows); err != nil {
			h.log.ErrorContext(ctx, "insert warehouse file rows", "error", err)
		}
	}

	compareFile.RowCount = len(fileRows)
	_ = h.compareSvc.UpdateFile(database.AsSystem(ctx), compareFile)

	h.redirectWithNotice(w, r, "/admin/user/temparte-warehouses", "success", fmt.Sprintf("تم رفع المستودع [%s] بنجاح بإجمالي %d صنف متاح في خصومات السوق.", supplierName, len(fileRows)))
}

// detectTempWarehouseCols determines column indices based on header names or custom inputs.
func detectTempWarehouseCols(headers []string, customCode, customName, customPrice, customDiscount string) (codeCol, nameCol, priceCol, discountCol int) {
	codeCol, nameCol, priceCol, discountCol = -1, -1, -1, -1
	numCols := len(headers)

	if customCode != "" {
		if c, err := strconv.Atoi(customCode); err == nil && c >= 0 && c < numCols {
			codeCol = c
		}
	}
	if customName != "" {
		if c, err := strconv.Atoi(customName); err == nil && c >= 0 && c < numCols {
			nameCol = c
		}
	}
	if customPrice != "" {
		if c, err := strconv.Atoi(customPrice); err == nil && c >= 0 && c < numCols {
			priceCol = c
		}
	}
	if customDiscount != "" {
		if c, err := strconv.Atoi(customDiscount); err == nil && c >= 0 && c < numCols {
			discountCol = c
		}
	}

	for i, h := range headers {
		norm := strings.ToLower(strings.TrimSpace(h))
		norm = strings.ReplaceAll(norm, "_", "")
		norm = strings.ReplaceAll(norm, "-", "")
		norm = strings.ReplaceAll(norm, " ", "")

		if codeCol == -1 {
			if strings.Contains(norm, "كود") || strings.Contains(norm, "code") || strings.Contains(norm, "sku") ||
				strings.Contains(norm, "باركود") || strings.Contains(norm, "barcode") || strings.Contains(norm, "رقمصنف") ||
				strings.Contains(norm, "itemcode") {
				codeCol = i
			}
		}

		if nameCol == -1 {
			if strings.Contains(norm, "اسم") || strings.Contains(norm, "name") || strings.Contains(norm, "صنف") ||
				strings.Contains(norm, "منتج") || strings.Contains(norm, "item") || strings.Contains(norm, "product") {
				nameCol = i
			}
		}

		if priceCol == -1 {
			if (strings.Contains(norm, "سعر") || strings.Contains(norm, "price") || strings.Contains(norm, "جمهور") || strings.Contains(norm, "رسمي")) && !strings.Contains(norm, "خصم") && !strings.Contains(norm, "صافي") {
				priceCol = i
			}
		}

		if discountCol == -1 {
			if strings.Contains(norm, "خصم") || strings.Contains(norm, "discount") || strings.Contains(norm, "%") || strings.Contains(norm, "نسبة") {
				discountCol = i
			}
		}
	}

	if codeCol == -1 && numCols > 0 {
		codeCol = 0
	}
	if nameCol == -1 && numCols > 1 {
		nameCol = 1
	}
	if priceCol == -1 && numCols > 2 {
		priceCol = 2
	}
	if discountCol == -1 && numCols > 3 {
		discountCol = 3
	}

	return codeCol, nameCol, priceCol, discountCol
}

// parsePriceFloat parses a numeric string into float64, converting Arabic numerals and commas.
func parsePriceFloat(raw string) (float64, error) {
	s := strings.TrimSpace(raw)
	s = strings.ReplaceAll(s, "٠", "0")
	s = strings.ReplaceAll(s, "١", "1")
	s = strings.ReplaceAll(s, "٢", "2")
	s = strings.ReplaceAll(s, "٣", "3")
	s = strings.ReplaceAll(s, "٤", "4")
	s = strings.ReplaceAll(s, "٥", "5")
	s = strings.ReplaceAll(s, "٦", "6")
	s = strings.ReplaceAll(s, "٧", "7")
	s = strings.ReplaceAll(s, "٨", "8")
	s = strings.ReplaceAll(s, "٩", "9")
	s = strings.ReplaceAll(s, "٫", ".")
	s = strings.ReplaceAll(s, "%", "")
	s = strings.ReplaceAll(s, ",", ".")
	s = strings.TrimSpace(s)

	var numStr strings.Builder
	foundDigit := false
	for _, r := range s {
		if (r >= '0' && r <= '9') || r == '.' {
			numStr.WriteRune(r)
			foundDigit = true
		} else if foundDigit && r != ' ' {
			break
		}
	}
	if !foundDigit {
		return 0, fmt.Errorf("no digit found")
	}
	return strconv.ParseFloat(numStr.String(), 64)
}

