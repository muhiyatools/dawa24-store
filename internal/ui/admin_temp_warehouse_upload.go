package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/compare"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/filesecurity"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/shared/productmatch"
	"github.com/muhiya/dawa24-store/internal/shared/sheet"
)

func cleanSupplierNameFromFilename(filename string, lang ...string) string {
	base := filepath.Base(filename)
	ext := filepath.Ext(base)
	name := strings.TrimSuffix(base, ext)
	name = strings.ReplaceAll(name, "_", " ")
	name = strings.ReplaceAll(name, "-", " ")
	name = strings.Join(strings.Fields(name), " ")
	if name == "" {
		l := "ar"
		if len(lang) > 0 && lang[0] != "" {
			l = lang[0]
		}
		name = i18n.T(l, "admin.temp_warehouse.default_name_prefix") + time.Now().Format("2006-01-02 03:04 PM")
	}
	return name
}

func resolveStoragePath(storageKey, category string) string {
	cleanKey := strings.TrimPrefix(filepath.FromSlash(storageKey), string(filepath.Separator))
	candidates := []string{
		storageKey,
		filepath.Join(UploadBaseDir, category, filepath.Base(storageKey)),
		filepath.Join(UploadBaseDir, filepath.FromSlash(strings.TrimPrefix(storageKey, "/uploads/"))),
		filepath.Join("data", "uploads", category, filepath.Base(storageKey)),
		filepath.Join("data", cleanKey),
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return ""
}

// processSingleTempWarehouseFile handles parsing and inserting a single file in memory.
func (h *UIHandler) processSingleTempWarehouseFile(
	ctx context.Context,
	fh *multipart.FileHeader,
	defaultSupplierName string,
	customCode, customName, customPrice, customDiscount string,
	userID int64,
	orgID *int64,
) tempWarehouseUploadResult {
	file, err := fh.Open()
	if err != nil {
		return tempWarehouseUploadResult{Filename: fh.Filename, Success: false, Error: fmt.Sprintf(i18n.T("ar", "admin.temp_warehouse.open_failed_format"), err.Error())}
	}
	defer file.Close()

	fileBytes, err := io.ReadAll(file)
	if err != nil {
		return tempWarehouseUploadResult{Filename: fh.Filename, Success: false, Error: fmt.Sprintf(i18n.T("ar", "admin.temp_warehouse.read_failed_format"), err.Error())}
	}

	if err := filesecurity.ValidateSpreadsheetSecurity(fileBytes, fh.Filename); err != nil {
		return tempWarehouseUploadResult{Filename: fh.Filename, Success: false, Error: filesecurity.SecurityErrorMessage}
	}

	rawRows, err := sheet.ReadRows(fileBytes, fh.Filename)
	if err != nil || len(rawRows) < 2 {
		msg := i18n.T("ar", "admin.temp_warehouse.insufficient_rows")
		if err != nil && strings.Contains(err.Error(), filesecurity.SecurityErrorMessage) {
			msg = filesecurity.SecurityErrorMessage
		}
		return tempWarehouseUploadResult{Filename: fh.Filename, Success: false, Error: msg}
	}

	// Row zero is not reliably the header row.
	//
	// This used to read `rawRows[0]` and hand it straight to the column
	// detector. Warehouse exports routinely carry a title band above the
	// header — the warehouse's name, an export date, a blank spacer — so the
	// detector was matching against those and binding the code column to
	// whatever happened to sit in that position. The live data shows the
	// result: rows whose "code" is 24.5 or 21, which are prices, and a
	// discount column that stayed at zero because it was never found.
	//
	// AnalyzeLayout is what every other importer in the platform uses to find
	// the header row and the first data row, and this one was the outlier.
	layout, _ := productmatch.AnalyzeLayout(rawRows)
	headers := layout.Headers
	dataStart := layout.FirstDataRow
	if layout.HeaderRow < 0 || len(headers) == 0 {
		headers = rawRows[0]
		dataStart = 1
	}
	if dataStart < 1 || dataStart > len(rawRows) {
		dataStart = 1
	}
	dataRows := rawRows[dataStart:]

	codeCol, nameCol, priceCol, discountCol := detectTempWarehouseCols(headers, customCode, customName, customPrice, customDiscount)

	supplierName := defaultSupplierName
	if supplierName == "" {
		supplierName = cleanSupplierNameFromFilename(fh.Filename)
	}

	// Persist uploaded bytes to disk so mapping can be readjusted anytime
	localURL, _ := saveUploadedBytes(fileBytes, fh.Filename, "temp_warehouses")
	storageKey := localURL
	if storageKey == "" {
		storageKey = fmt.Sprintf("temp_warehouses/%d_%s", time.Now().UnixNano(), filepath.Base(fh.Filename))
	}

	compareFile := &compare.CompareFile{
		UserID:           userID,
		OrganizationID:   orgID,
		SupplierName:     supplierName,
		OriginalFilename: fh.Filename,
		StorageKey:       storageKey,
		MIMEType:         "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		SizeBytes:        int64(len(fileBytes)),
		Status:           compare.FileReady,
		IsTempWarehouse:  true,
		MappingConfig: compare.MappingConfig{
			CodeCol:     &codeCol,
			NameCol:     &nameCol,
			PriceCol:    &priceCol,
			DiscountCol: &discountCol,
		},
	}

	if err := h.compareSvc.CreateFile(database.AsSystem(ctx), compareFile); err != nil {
		return tempWarehouseUploadResult{Filename: fh.Filename, SupplierName: supplierName, Success: false, Error: fmt.Sprintf(i18n.T("ar", "admin.temp_warehouse.create_record_failed_format"), err.Error())}
	}

	fileRows := make([]*compare.CompareFileRow, 0, len(dataRows))
	for idx, row := range dataRows {
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
			OrganizationID:     orgID,
			RowNumber:          dataStart + idx + 1,
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
			h.log.ErrorContext(ctx, "insert warehouse file rows", "error", err, "file_id", compareFile.ID)
		}
	}

	compareFile.RowCount = len(fileRows)
	_ = h.compareSvc.UpdateFile(database.AsSystem(ctx), compareFile)

	return tempWarehouseUploadResult{
		ID:           compareFile.ID,
		Filename:     fh.Filename,
		SupplierName: supplierName,
		RowCount:     len(fileRows),
		Success:      true,
	}
}

// AdminTempWarehouseUploadSubmit handles bulk and single file upload (optimized for 60-100+ files in parallel).
func (h *UIHandler) AdminTempWarehouseUploadSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)

	// 500MB max limit to comfortably allow 60-100+ bulk files
	if err := r.ParseMultipartForm(500 << 20); err != nil {
		if isJSONOrAJAX(r) {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": i18n.T(lang, "admin.temp_warehouse.upload_limit_exceeded")})
			return
		}
		h.redirectWithNotice(w, r, "/admin/user/temparte-warehouses", "error", i18n.T(lang, "admin.temp_warehouse.upload_too_large"))
		return
	}

	// Gather all files from multipart form
	var fileHeaders []*multipart.FileHeader
	if r.MultipartForm != nil && r.MultipartForm.File != nil {
		if files, ok := r.MultipartForm.File["files"]; ok && len(files) > 0 {
			fileHeaders = append(fileHeaders, files...)
		}
		if file, ok := r.MultipartForm.File["file"]; ok && len(file) > 0 {
			fileHeaders = append(fileHeaders, file...)
		}
		for k, fl := range r.MultipartForm.File {
			if k != "files" && k != "file" {
				fileHeaders = append(fileHeaders, fl...)
			}
		}
	}

	if len(fileHeaders) == 0 {
		if isJSONOrAJAX(r) {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": i18n.T(lang, "admin.temp_warehouse.select_files")})
			return
		}
		h.redirectWithNotice(w, r, "/admin/user/temparte-warehouses", "error", i18n.T(lang, "admin.temp_warehouse.select_files_notice"))
		return
	}

	baseSupplierName := strings.TrimSpace(r.FormValue("supplier_name"))
	customCode := r.FormValue("col_code")
	customName := r.FormValue("col_name")
	customPrice := r.FormValue("col_price")
	customDiscount := r.FormValue("col_discount")

	if len(fileHeaders) == 1 && baseSupplierName == "" {
		baseSupplierName = cleanSupplierNameFromFilename(fileHeaders[0].Filename)
	}

	// Determine actor user ID / fallback
	var userID int64 = 41
	var orgID *int64
	if actor, ok := authctx.From(ctx); ok {
		if actor.UserID > 0 {
			userID = actor.UserID
		}
		if actor.OrgID > 0 {
			orgID = &actor.OrgID
		}
	}

	// High-speed parallel worker pool: concurrency bounded by CPU cores (e.g. 8-16 workers)
	numWorkers := 8
	if n := runtime.NumCPU() * 2; n > numWorkers {
		numWorkers = n
	}
	if numWorkers > 16 {
		numWorkers = 16
	}
	if numWorkers > len(fileHeaders) {
		numWorkers = len(fileHeaders)
	}

	results := make([]tempWarehouseUploadResult, len(fileHeaders))
	sem := make(chan struct{}, numWorkers)
	var wg sync.WaitGroup

	for i, fh := range fileHeaders {
		wg.Add(1)
		go func(idx int, header *multipart.FileHeader) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			suppName := ""
			if len(fileHeaders) == 1 {
				suppName = baseSupplierName
			} else {
				fileClean := cleanSupplierNameFromFilename(header.Filename)
				if baseSupplierName != "" {
					suppName = baseSupplierName + " - " + fileClean
				} else {
					suppName = fileClean
				}
			}

			res := h.processSingleTempWarehouseFile(
				ctx,
				header,
				suppName,
				customCode,
				customName,
				customPrice,
				customDiscount,
				userID,
				orgID,
			)
			results[idx] = res
		}(i, fh)
	}

	wg.Wait()

	// Aggregate results
	successCount := 0
	failCount := 0
	totalRows := int64(0)
	var errorMessages []string
	var uploadedIDs []string

	for _, res := range results {
		if res.Success {
			successCount++
			totalRows += int64(res.RowCount)
			if res.ID > 0 {
				uploadedIDs = append(uploadedIDs, strconv.FormatInt(res.ID, 10))
			}
		} else {
			failCount++
			if res.Error != "" {
				errorMessages = append(errorMessages, fmt.Sprintf("%s: %s", res.Filename, res.Error))
			}
		}
	}

	if isJSONOrAJAX(r) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success":          successCount > 0,
			"total_files":      len(fileHeaders),
			"successful_files": successCount,
			"failed_files":     failCount,
			"total_items":      totalRows,
			"uploaded_ids":     uploadedIDs,
			"setup_queue":      strings.Join(uploadedIDs, ","),
			"results":          results,
			"errors":           errorMessages,
			"message":          fmt.Sprintf(i18n.T(lang, "admin.temp_warehouse.upload_success_message"), successCount, len(fileHeaders), totalRows),
		})
		return
	}

	if successCount == 0 && failCount > 0 {
		h.redirectWithNotice(w, r, "/admin/user/temparte-warehouses", "error", i18n.T(lang, "admin.temp_warehouse.all_files_failed_prefix")+strings.Join(errorMessages, " | "))
		return
	}

	successMsg := fmt.Sprintf(i18n.T(lang, "admin.temp_warehouse.success_summary"), successCount, len(fileHeaders), totalRows)
	if failCount > 0 {
		successMsg += fmt.Sprintf(i18n.T(lang, "admin.temp_warehouse.fail_count_suffix"), failCount)
	}
	h.redirectWithNotice(w, r, "/admin/user/temparte-warehouses", "success", successMsg)
}
