package ui

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/xuri/excelize/v2"

	"github.com/muhiya/dawa24-store/internal/modules/compare"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

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

	h.renderPage(ctx, w, "render temp warehouses", pages.AdminTempWarehousesPage(data, lang, dir))
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
