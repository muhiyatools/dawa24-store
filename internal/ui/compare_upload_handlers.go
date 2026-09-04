package ui

import (
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/xuri/excelize/v2"

	"github.com/muhiya/dawa24-store/internal/modules/compare"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// CompareSampleDownload generates and streams a realistic Egyptian pharmaceutical pricing & discount template file (.xlsx).
func (h *UIHandler) CompareSampleDownload(w http.ResponseWriter, r *http.Request) {
	f := excelize.NewFile()
	defer f.Close()

	sheetName := i18n.T("ar", "excel.sheet.price_list")
	f.SetSheetName("Sheet1", sheetName)

	// Set right-to-left layout for Arabic
	_ = f.SetSheetView(sheetName, 0, &excelize.ViewOptions{
		RightToLeft: func() *bool { b := true; return &b }(),
	})

	// Header style
	headerStyle, _ := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{
			Bold:  true,
			Color: "#FFFFFF",
			Size:  11,
		},
		Fill: excelize.Fill{
			Type:    "pattern",
			Color:   []string{"#0F172A"},
			Pattern: 1,
		},
		Alignment: &excelize.Alignment{
			Horizontal: "center",
			Vertical:   "center",
		},
	})

	headers := []string{i18n.TDefault("w4_ui.s_52_52"), i18n.TDefault("w4_ui.s_53_53"), i18n.TDefault("w4_ui.s_54_54"), i18n.TDefault("w4_ui.s_55_55"), i18n.TDefault("w4_ui.s_56_56")}
	for i, hName := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		_ = f.SetCellValue(sheetName, cell, hName)
		_ = f.SetCellStyle(sheetName, cell, cell, headerStyle)
	}

	// 10 realistic pharmaceutical sample records
	samples := [][]any{
		{"1001", i18n.TDefault("w4_ui.24_57"), 45.00, 18.5, i18n.T("ar", "sample.notes.large_stock")},
		{"1002", i18n.TDefault("w4_ui.1_14_58"), 135.00, 12.0, i18n.T("ar", "sample.notes.extra_discount")},
		{"1003", i18n.TDefault("w4_ui.20_59"), 31.00, 20.0, i18n.T("ar", "sample.notes.seasonal_offer")},
		{"1004", i18n.TDefault("w4_ui.50_20_60"), 58.50, 15.0, i18n.T("ar", "sample.notes.fresh_expiry")},
		{"1005", i18n.TDefault("w4_ui.20_61"), 22.00, 25.0, i18n.T("ar", "sample.notes.highest_discount")},
		{"1006", i18n.TDefault("w4_ui.400_30_62"), 48.00, 14.5, i18n.T("ar", "sample.notes.fast_delivery")},
		{"1007", i18n.TDefault("w4_ui.20_63"), 35.00, 16.0, i18n.TDefault("w4_ui.2027_17")},
		{"1008", i18n.TDefault("w4_ui.500_20_64"), 18.00, 22.5, i18n.T("ar", "sample.notes.pharmacy_offer")},
		{"1009", i18n.TDefault("w4_ui.500_3_65"), 52.00, 15.0, i18n.T("ar", "sample.notes.factory_direct")},
		{"1010", i18n.TDefault("w4_ui.20_66"), 28.00, 19.0, i18n.T("ar", "sample.notes.free_shipping")},
	}

	for rowIdx, row := range samples {
		for colIdx, val := range row {
			cell, _ := excelize.CoordinatesToCellName(colIdx+1, rowIdx+2)
			_ = f.SetCellValue(sheetName, cell, val)
		}
	}

	_ = f.SetColWidth(sheetName, "A", "A", 18)
	_ = f.SetColWidth(sheetName, "B", "B", 46)
	_ = f.SetColWidth(sheetName, "C", "C", 20)
	_ = f.SetColWidth(sheetName, "D", "D", 22)
	_ = f.SetColWidth(sheetName, "E", "E", 32)

	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", "attachment; filename=\"dawa24_supplier_template.xlsx\"")
	_ = f.Write(w)
}

// CompareUploadSubmit handles uploading one or multiple supplier spreadsheet files and automatically parses rows in parallel.
func (h *UIHandler) CompareUploadSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/compare/tool", http.StatusSeeOther)
		return
	}

	if h.compareSvc == nil {
		h.redirectWithNotice(w, r, "/compare/tool", "error", i18n.T(lang, "common.compare_service_unavailable"))
		return
	}

	// 128 MB max memory for multi-file batch uploads
	if err := r.ParseMultipartForm(128 << 20); err != nil {
		h.redirectWithNotice(w, r, "/compare/tool", "error", i18n.T(lang, "compare.upload.read_failed"))
		return
	}

	var fileHeaders []*multipart.FileHeader
	if fhs, ok := r.MultipartForm.File["compare_files"]; ok && len(fhs) > 0 {
		fileHeaders = fhs
	} else if fhs, ok := r.MultipartForm.File["compare_file"]; ok && len(fhs) > 0 {
		fileHeaders = fhs
	}

	if len(fileHeaders) == 0 {
		h.redirectWithNotice(w, r, "/compare/tool", "error", i18n.T(lang, "compare.upload.choose_file"))
		return
	}

	var orgPtr *int64
	if actor.OrganizationID > 0 {
		orgPtr = &actor.OrganizationID
	}

	// Enforce compare files quota based on user's active subscription plan
	maxAllowedFiles := 10
	if h.billSvc != nil {
		if plan, err := h.billSvc.GetEffectivePlan(ctx, actor.UserID, orgPtr); err == nil && plan != nil {
			maxAllowedFiles = plan.GetMaxCompareFiles()
		}
	}

	if maxAllowedFiles > 0 {
		activeFiles, err := h.compareSvc.ListFiles(ctx, actor.UserID, orgPtr, nil)
		if err == nil {
			activeCount := 0
			for _, f := range activeFiles {
				if f.Status != compare.FileArchived && f.DeletedAt == nil && !f.IsTempWarehouse {
					activeCount++
				}
			}
			if activeCount >= maxAllowedFiles {
				h.redirectWithNotice(w, r, "/compare/tool", "error", fmt.Sprintf(i18n.T(lang, "compare.upload.quota_exceeded"), maxAllowedFiles))
				return
			}
			if activeCount+len(fileHeaders) > maxAllowedFiles {
				remaining := maxAllowedFiles - activeCount
				h.redirectWithNotice(w, r, "/compare/tool", "error", fmt.Sprintf(i18n.T(lang, "compare.upload.quota_overflow"), len(fileHeaders), remaining, maxAllowedFiles))
				return
			}
		}
	}

	type fileItem struct {
		index        int
		filename     string
		supplierName string
		contentType  string
		size         int64
		fileBytes    []byte
		localURL     string
	}

	type fileResult struct {
		index    int
		file     *compare.CompareFile
		archived []string
		err      error
		errFile  string
	}

	// 1. Read and validate all uploaded file payloads into memory/disk
	var validItems []fileItem
	var errorFiles []string

	for idx, header := range fileHeaders {
		if !SupportedUploadName(header.Filename) {
			errorFiles = append(errorFiles, header.Filename+" ("+i18n.T(lang, "compare.upload.unsupported_format")+")")
			continue
		}

		file, err := header.Open()
		if err != nil {
			errorFiles = append(errorFiles, header.Filename+" ("+i18n.T(lang, "compare.upload.open_failed")+")")
			continue
		}

		fileBytes, err := io.ReadAll(file)
		file.Close()
		if err != nil || len(fileBytes) == 0 {
			errorFiles = append(errorFiles, header.Filename+" ("+i18n.T(lang, "compare.upload.empty_or_unread")+")")
			continue
		}

		supplierName := strings.TrimSpace(r.FormValue("supplier_name"))
		if supplierName == "" || len(fileHeaders) > 1 {
			supplierName = strings.TrimSpace(strings.TrimSuffix(header.Filename,
				filepath.Ext(header.Filename)))
			supplierName = strings.ReplaceAll(supplierName, "_", " ")
			supplierName = strings.ReplaceAll(supplierName, "-", " ")
		}
		if supplierName == "" {
			supplierName = header.Filename
		}

		localURL, localErr := saveUploadedBytes(fileBytes, header.Filename, "compare")
		if localErr != nil {
			h.log.ErrorContext(ctx, "failed to save uploaded compare file to disk", "error", localErr, "file", header.Filename)
			errorFiles = append(errorFiles, header.Filename+" ("+i18n.T(lang, "compare.upload.save_failed")+")")
			continue
		}

		validItems = append(validItems, fileItem{
			index:        idx,
			filename:     header.Filename,
			supplierName: supplierName,
			contentType:  header.Header.Get("Content-Type"),
			size:         header.Size,
			fileBytes:    fileBytes,
			localURL:     localURL,
		})
	}

	// 2. Reserve quota for the whole batch, once.
	//
	// The per-file upload path also makes room for itself, and used to be the
	// only thing that did. With six workers running it at the same time, each
	// read the same active-file count, each concluded the quota was full, and
	// each archived "everything past the limit" — by then including the files
	// its siblings had just created. A batch of eight arrived and two survived.
	// Reserving here means the eviction decision is made once, against the real
	// batch size, before any of the batch exists.
	batchArchived, roomErr := h.compareSvc.MakeRoomForFiles(ctx, actor.UserID, orgPtr, len(validItems))
	if roomErr != nil {
		h.redirectWithNotice(w, r, "/compare/tool", "error", h.safeMessage(roomErr, lang))
		return
	}

	// 3. Process valid files with bounded parallel concurrency.
	results := make([]fileResult, len(validItems))
	if len(validItems) > 0 {
		numWorkers := 6
		if len(validItems) < numWorkers {
			numWorkers = len(validItems)
		}

		itemChan := make(chan fileItem, len(validItems))
		var wg sync.WaitGroup

		for w := 0; w < numWorkers; w++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for itm := range itemChan {
					uploadedFile, archived, err := h.compareSvc.UploadAndProcessCompareFile(
						ctx, actor.UserID, orgPtr, itm.supplierName, itm.filename,
						itm.contentType, itm.size, itm.localURL, itm.fileBytes,
					)
					res := fileResult{
						index:    itm.index,
						file:     uploadedFile,
						archived: archived,
						err:      err,
					}
					if err != nil {
						res.errFile = itm.filename + " (" + h.safeMessage(err, lang) + ")"
					}
					results[itm.index] = res
				}
			}()
		}

		for i, itm := range validItems {
			itm.index = i
			itemChan <- itm
		}
		close(itemChan)
		wg.Wait()
	}

	// 4. Aggregate results
	var processedCount int
	var totalRows int
	allArchived := batchArchived
	var uploadedIDs []string

	for _, res := range results {
		if res.err != nil {
			if res.errFile != "" {
				errorFiles = append(errorFiles, res.errFile)
			}
			continue
		}
		if res.file != nil {
			processedCount++
			totalRows += res.file.RowCount
			allArchived = append(allArchived, res.archived...)
			uploadedIDs = append(uploadedIDs, strconv.FormatInt(res.file.ID, 10))
		}
	}

	if processedCount == 0 {
		errMsg := i18n.T(lang, "compare.upload.none_processed_prefix") + strings.Join(errorFiles, ", ")
		h.redirectWithNotice(w, r, "/compare/tool", "error", errMsg)
		return
	}

	msg := fmt.Sprintf(i18n.T(lang, "compare.upload.success_summary"), processedCount, totalRows)
	firstID := uploadedIDs[0]
	queueStr := strings.Join(uploadedIDs, ",")
	redirectURL := fmt.Sprintf("/compare/tool?setup_queue=%s&setup_file=%s&setup_step=1&setup_total=%d&notice=success&msg=%s", url.QueryEscape(queueStr), firstID, len(uploadedIDs), url.QueryEscape(msg))
	if len(errorFiles) > 0 {
		redirectURL += "&warning=" + url.QueryEscape(i18n.T(lang, "compare.upload.warning_failed_prefix")+strings.Join(errorFiles, ", "))
	}
	http.Redirect(w, r, redirectURL, http.StatusSeeOther)
}
