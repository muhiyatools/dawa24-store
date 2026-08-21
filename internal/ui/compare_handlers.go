package ui

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/xuri/excelize/v2"

	"github.com/muhiya/dawa24-store/internal/modules/billing"
	"github.com/muhiya/dawa24-store/internal/modules/compare"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/features"
	"github.com/muhiya/dawa24-store/internal/platform/storage"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

func (h *UIHandler) checkFileOwnership(actor authctx.Actor, file *compare.CompareFile) bool {
	if file == nil {
		return false
	}
	if file.UserID == actor.UserID {
		return true
	}
	if file.OrganizationID != nil && actor.OrganizationID > 0 && *file.OrganizationID == actor.OrganizationID {
		return true
	}
	return false
}

// ComparePlansPage renders the pricing page for the discount-comparison plans.
func (h *UIHandler) ComparePlansPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !features.Enabled(ctx, "compare.enabled") {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	lang, dir := h.localeAndDir(r)

	var viewPlans []*billing.Plan
	if h.compareSvc != nil {
		cPlans, err := h.compareSvc.ListPlans(ctx, true)
		if err == nil {
			for _, cp := range cPlans {
				viewPlans = append(viewPlans, &billing.Plan{
					ID:          cp.ID,
					Name:        cp.Name,
					Slug:        cp.Slug,
					Description: cp.Description,
					PriceMonth:  cp.PriceMonthly,
					PriceYear:   cp.PriceYearly,
					IsActive:    cp.IsActive,
				})
			}
		}
	} else if h.billSvc != nil {
		viewPlans, _ = h.billSvc.ListPlans(ctx)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.ComparePlansPage(lang, dir, viewPlans).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render compare plans", "error", err)
	}
}

// CompareSubscribeSubmit subscribes the caller to a compare plan.
func (h *UIHandler) CompareSubscribeSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/compare", http.StatusSeeOther)
		return
	}

	slug := r.URL.Query().Get("plan")
	if slug == "" {
		h.redirectWithNotice(w, r, "/compare", "error", "تعذر الاشتراك.")
		return
	}

	var orgPtr *int64
	if actor.OrganizationID > 0 {
		orgPtr = &actor.OrganizationID
	}

	if h.compareSvc != nil {
		if _, err := h.compareSvc.SubscribeDirectly(ctx, slug, orgPtr, actor.UserID, "monthly"); err != nil {
			h.redirectWithNotice(w, r, "/compare", "error", h.safeMessage(err, langOf(r)))
			return
		}
	} else if h.billSvc != nil {
		if _, err := h.billSvc.Subscribe(ctx, actor.UserID, orgPtr, slug, "compare", nil); err != nil {
			h.redirectWithNotice(w, r, "/compare", "error", h.safeMessage(err, langOf(r)))
			return
		}
	}
	h.redirectWithNotice(w, r, "/compare/tool", "success", "تم تفعيل اشتراكك بنجاح في محرك المقارنة.")
}

// CompareToolPage renders the 3-column comparison workspace.
func (h *UIHandler) CompareToolPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/compare/tool", http.StatusSeeOther)
		return
	}

	noticeType := r.URL.Query().Get("notice")
	noticeMsg := r.URL.Query().Get("msg")

	var files []*compare.CompareFile
	if h.compareSvc != nil {
		var orgPtr *int64
		if actor.OrganizationID > 0 {
			orgPtr = &actor.OrganizationID
		}
		files, _ = h.compareSvc.ListFiles(ctx, actor.UserID, orgPtr, nil)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.CompareToolPage(lang, dir, files, noticeType, noticeMsg).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render compare tool", "error", err)
	}
}

// CompareSampleDownload generates and streams a realistic Egyptian pharmaceutical pricing & discount template file (.xlsx).
func (h *UIHandler) CompareSampleDownload(w http.ResponseWriter, r *http.Request) {
	f := excelize.NewFile()
	defer f.Close()

	sheetName := "كشف أسعار المورد"
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

	headers := []string{"كود الصنف", "اسم الصنف", "السعر", "الخصم", "ملاحظات"}
	for i, hName := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		_ = f.SetCellValue(sheetName, cell, hName)
		_ = f.SetCellStyle(sheetName, cell, cell, headerStyle)
	}

	// 10 realistic pharmaceutical sample records
	samples := [][]any{
		{"1001", "بانادول اكسترا 24 قرص", 45.00, 18.5, "متوفر كميات كبيرة"},
		{"1002", "اوجمنتين 1 جم 14 قرص", 135.00, 12.0, "خصم إضافي للطلبيات الكبيرة"},
		{"1003", "كونجستال 20 قرص", 31.00, 20.0, "عرض موسمي حصري"},
		{"1004", "كتافلام 50 مجم 20 قرص", 58.50, 15.0, "تاريخ صلاحية حديث"},
		{"1005", "انتوسيد 20 قرص", 22.00, 25.0, "أعلى خصم بالسوق"},
		{"1006", "بروفين 400 مجم 30 قرص", 48.00, 14.5, "تسليم فوري ومباشر"},
		{"1007", "اومفيل 20 كبسولة", 35.00, 16.0, "صلاحية 2027"},
		{"1008", "سيتال 500 مجم 20 قرص", 18.00, 22.5, "عرض خاص للصيدليات"},
		{"1009", "ازيثرودوز 500 مجم 3 كبسولات", 52.00, 15.0, "توريد مباشر من المصنع"},
		{"1010", "كولد اند فلو 20 قرص", 28.00, 19.0, "شحن مجاني للطلبات الكبيرة"},
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

// CompareUploadSubmit handles uploading a supplier spreadsheet file and automatically parses rows.
func (h *UIHandler) CompareUploadSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/compare/tool", http.StatusSeeOther)
		return
	}

	if h.compareSvc == nil {
		h.redirectWithNotice(w, r, "/compare/tool", "error", "خدمة المقارنة غير متاحة حالياً.")
		return
	}

	if err := r.ParseMultipartForm(MaxUploadBytes); err != nil {
		h.redirectWithNotice(w, r, "/compare/tool", "error", "تعذر قراءة الملف المرفوع.")
		return
	}

	file, header, err := r.FormFile("compare_file")
	if err != nil {
		h.redirectWithNotice(w, r, "/compare/tool", "error", "يرجى اختيار ملف Excel أو CSV.")
		return
	}
	defer file.Close()

	ext := strings.ToLower(filepath.Ext(header.Filename))
	if ext != ".xlsx" && ext != ".xls" && ext != ".csv" {
		h.redirectWithNotice(w, r, "/compare/tool", "error", "صيغة الملف غير مدعومة. يرجى رفع ملف بصيغة xlsx أو xls أو csv.")
		return
	}

	fileBytes, err := io.ReadAll(file)
	if err != nil {
		h.redirectWithNotice(w, r, "/compare/tool", "error", "تعذر قراءة محتوى الملف.")
		return
	}

	supplierName := strings.TrimSpace(r.FormValue("supplier_name"))
	if supplierName == "" {
		supplierName = strings.TrimSuffix(header.Filename, ext)
	}

	// 1. Upload to MinIO object storage if configured
	var storageKey string
	if actor.OrganizationID > 0 {
		storageKey = storage.KeyFor(actor.OrganizationID, fmt.Sprintf("compare/%d_%s", time.Now().Unix(), filepath.Base(header.Filename)))
	} else {
		storageKey = fmt.Sprintf("users/%d/compare/%d_%s", actor.UserID, time.Now().Unix(), filepath.Base(header.Filename))
	}
	if h.storage != nil {
		if err := h.storage.Put(ctx, storageKey, bytes.NewReader(fileBytes), int64(len(fileBytes)), header.Header.Get("Content-Type")); err != nil {
			h.log.WarnContext(ctx, "minio put upload warning", "error", err, "key", storageKey)
		}
	}

	// 2. Also write to local storage as guaranteed disk fallback
	localDiskPath := filepath.Join("data", filepath.FromSlash(storageKey))
	_ = os.MkdirAll(filepath.Dir(localDiskPath), 0755)
	_ = os.WriteFile(localDiskPath, fileBytes, 0644)

	localURL, localErr := saveUploadedBytes(fileBytes, header.Filename, "compare")
	if localErr != nil {
		h.log.WarnContext(ctx, "local disk save warning", "error", localErr)
	}
	if storageKey == "" || h.storage == nil {
		storageKey = localURL
	}

	var orgPtr *int64
	if actor.OrganizationID > 0 {
		orgPtr = &actor.OrganizationID
	}

	uploadedFile, archived, err := h.compareSvc.UploadAndProcessCompareFile(
		ctx, actor.UserID, orgPtr, supplierName, header.Filename,
		header.Header.Get("Content-Type"), header.Size, storageKey, fileBytes,
	)
	if err != nil {
		h.log.ErrorContext(ctx, "failed to upload and process compare file", "error", err, "supplier", supplierName)
		h.redirectWithNotice(w, r, "/compare/tool", "error", h.safeMessage(err, langOf(r)))
		return
	}

	h.log.InfoContext(ctx, "compare file uploaded and processed successfully", "file_id", uploadedFile.ID, "rows", uploadedFile.RowCount, "supplier", uploadedFile.SupplierName)

	msg := fmt.Sprintf("تم رفع ومعالجة كشف المورد '%s' بنجاح (تم استخراج %d صنف جاهزة للمقارنة).", uploadedFile.SupplierName, uploadedFile.RowCount)
	if len(archived) > 0 {
		msg += " تنبيه: لقد تجاوزت الحد الأقصى للملفات النشطة، تم نقل المورد الأقدم (" + strings.Join(archived, "، ") + ") إلى الأرشيف."
	}
	h.redirectWithNotice(w, r, "/compare/tool", "success", msg)
}

// CompareFileRenameSubmit handles renaming a supplier file label.
func (h *UIHandler) CompareFileRenameSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/compare/tool", http.StatusSeeOther)
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/compare/tool", "error", "معرف ملف غير صالح.")
		return
	}

	if h.compareSvc != nil {
		file, err := h.compareSvc.GetFile(ctx, id)
		if err != nil || !h.checkFileOwnership(actor, file) {
			h.redirectWithNotice(w, r, "/compare/tool", "error", "غير مصرح لك بتعديل هذا الملف.")
			return
		}

		newName := strings.TrimSpace(r.FormValue("supplier_name"))
		if newName == "" {
			h.redirectWithNotice(w, r, "/compare/tool", "error", "اسم المورد لا يمكن أن يكون فارغاً.")
			return
		}
		if err := h.compareSvc.RenameFile(ctx, id, newName); err != nil {
			h.redirectWithNotice(w, r, "/compare/tool", "error", h.safeMessage(err, langOf(r)))
			return
		}
	}
	h.redirectWithNotice(w, r, "/compare/tool", "success", "تم تغيير اسم المورد بنجاح.")
}

// CompareFileMappingModal renders the interactive modal HTML fragment for column mapping.
func (h *UIHandler) CompareFileMappingModal(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "معرف ملف غير صالح.", http.StatusBadRequest)
		return
	}

	var file *compare.CompareFile
	if h.compareSvc != nil {
		file, err = h.compareSvc.GetFile(ctx, id)
		if err != nil {
			http.Error(w, "الملف غير موجود.", http.StatusNotFound)
			return
		}
	}

	if !h.checkFileOwnership(actor, file) {
		http.Error(w, "غير مصرح لك بالوصول لهذا الملف.", http.StatusForbidden)
		return
	}

	headers, preview := h.loadFileHeadersAndPreview(ctx, file)

	fieldMapping, scores, confidence := compare.DetectColumnsWithConfidence(headers)
	colMapping := make(map[compare.TargetField]*int)
	for colIdx, field := range fieldMapping {
		idx := colIdx
		colMapping[field] = &idx
	}

	detectedMapping := &compare.ColumnDetection{
		NameCol:     colMapping[compare.FieldProductName],
		PriceCol:    colMapping[compare.FieldPrice],
		DiscountCol: colMapping[compare.FieldDiscount],
		CodeCol:     colMapping[compare.FieldSKU],
		Confidence:  confidence,
		FieldScores: scores,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.CompareFileMappingModal(file, headers, preview, detectedMapping).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render compare file mapping modal", "error", err)
	}
}

// CompareFileMappingPage shows the column mapping page or modal.
func (h *UIHandler) CompareFileMappingPage(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("HX-Request") == "true" || r.URL.Query().Get("modal") == "1" {
		h.CompareFileMappingModal(w, r)
		return
	}

	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/compare/tool", http.StatusSeeOther)
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/compare/tool", "error", "معرف ملف غير صالح.")
		return
	}

	var file *compare.CompareFile
	if h.compareSvc != nil {
		file, err = h.compareSvc.GetFile(ctx, id)
		if err != nil {
			h.redirectWithNotice(w, r, "/compare/tool", "error", h.safeMessage(err, langOf(r)))
			return
		}
	}

	if !h.checkFileOwnership(actor, file) {
		h.redirectWithNotice(w, r, "/compare/tool", "error", "غير مصرح لك بالوصول لهذا الملف.")
		return
	}

	headers, preview := h.loadFileHeadersAndPreview(ctx, file)

	fieldMapping, scores, confidence := compare.DetectColumnsWithConfidence(headers)
	colMapping := make(map[compare.TargetField]*int)
	for colIdx, field := range fieldMapping {
		idx := colIdx
		colMapping[field] = &idx
	}

	detectedMapping := &compare.ColumnDetection{
		NameCol:     colMapping[compare.FieldProductName],
		PriceCol:    colMapping[compare.FieldPrice],
		DiscountCol: colMapping[compare.FieldDiscount],
		CodeCol:     colMapping[compare.FieldSKU],
		Confidence:  confidence,
		FieldScores: scores,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.CompareFileMappingPage(lang, dir, file, headers, preview, detectedMapping).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render compare file mapping", "error", err)
	}
}

// loadFileHeadersAndPreview extracts headers and sample preview rows from storage, local disk, or database.
func (h *UIHandler) loadFileHeadersAndPreview(ctx context.Context, file *compare.CompareFile) ([]string, [][]string) {
	var headers []string
	var preview [][]string

	if file != nil && file.StorageKey != "" {
		if h.storage != nil && !strings.HasPrefix(file.StorageKey, "/") && !strings.HasPrefix(file.StorageKey, "data/") {
			reader, _, err := h.storage.Get(ctx, file.StorageKey)
			if err == nil {
				headers, preview, _ = h.parseFilePreview(reader, file.OriginalFilename)
				reader.Close()
			}
		}
		if len(headers) == 0 {
			localPath := file.StorageKey
			if strings.HasPrefix(localPath, "/uploads/") {
				localPath = "data" + localPath
			}
			f, err := os.Open(localPath)
			if err == nil {
				headers, preview, _ = h.parseFilePreview(f, file.OriginalFilename)
				f.Close()
			}
		}
	}

	// Fallback to already extracted rows in DB if storage read didn't populate headers
	if len(headers) == 0 && file != nil && h.compareSvc != nil {
		if dbRows, _ := h.compareSvc.ListFileRows(ctx, file.ID, 5, 0); len(dbRows) > 0 {
			headers = []string{"كود الصنف", "اسم الصنف", "السعر", "الخصم"}
			for _, dr := range dbRows {
				preview = append(preview, []string{
					dr.SKU, dr.RawName, dr.Price.String(), fmt.Sprintf("%.1f%%", dr.Discount),
				})
			}
		}
	}

	if len(headers) == 0 {
		headers = []string{"كود الصنف", "اسم الصنف", "السعر", "الخصم", "ملاحظات"}
	}

	return headers, preview
}

// parseFilePreview reads the first few rows of a spreadsheet for mapping preview.
func (h *UIHandler) parseFilePreview(reader io.Reader, filename string) ([]string, [][]string, error) {
	lower := strings.ToLower(filename)
	if strings.HasSuffix(lower, ".xlsx") || strings.HasSuffix(lower, ".xls") {
		return h.parseXLSXPreview(reader)
	}
	if strings.HasSuffix(lower, ".csv") {
		return h.parseCSVPreview(reader)
	}
	return nil, nil, fmt.Errorf("unsupported file format")
}

func (h *UIHandler) parseCSVPreview(reader io.Reader) ([]string, [][]string, error) {
	csvReader := csv.NewReader(reader)
	csvReader.FieldsPerRecord = -1
	csvReader.TrimLeadingSpace = true

	allRows, err := csvReader.ReadAll()
	if err != nil {
		return nil, nil, err
	}
	if len(allRows) == 0 {
		return nil, nil, fmt.Errorf("empty csv")
	}

	headerRowIdx, _, _ := compare.FindBestHeaderRow(allRows)
	headers := allRows[headerRowIdx]

	var preview [][]string
	for i := headerRowIdx + 1; i < len(allRows) && len(preview) < 5; i++ {
		if len(allRows[i]) > 0 {
			preview = append(preview, allRows[i])
		}
	}
	return headers, preview, nil
}

func (h *UIHandler) parseXLSXPreview(reader io.Reader) ([]string, [][]string, error) {
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, nil, err
	}

	f, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()

	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		return nil, nil, fmt.Errorf("no sheets")
	}

	allRows, err := f.GetRows(sheets[0])
	if err != nil {
		return nil, nil, err
	}
	if len(allRows) == 0 {
		return nil, nil, fmt.Errorf("empty sheet")
	}

	headerRowIdx, _, _ := compare.FindBestHeaderRow(allRows)
	headers := allRows[headerRowIdx]

	var preview [][]string
	for i := headerRowIdx + 1; i < len(allRows) && len(preview) < 5; i++ {
		if len(allRows[i]) > 0 {
			preview = append(preview, allRows[i])
		}
	}

	return headers, preview, nil
}

// CompareFileArchiveSubmit handles manually archiving a file.
func (h *UIHandler) CompareFileArchiveSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/compare/tool", http.StatusSeeOther)
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/compare/tool", "error", "معرف ملف غير صالح.")
		return
	}
	if h.compareSvc != nil {
		file, err := h.compareSvc.GetFile(ctx, id)
		if err != nil || !h.checkFileOwnership(actor, file) {
			h.redirectWithNotice(w, r, "/compare/tool", "error", "غير مصرح لك بتعديل هذا الملف.")
			return
		}
		if err := h.compareSvc.ArchiveFile(ctx, id, "أرشفة يدوية من قبل المستخدم"); err != nil {
			h.redirectWithNotice(w, r, "/compare/tool", "error", h.safeMessage(err, langOf(r)))
			return
		}
	}
	h.redirectWithNotice(w, r, "/compare/tool", "success", "تم نقل الملف إلى الأرشيف.")
}

// CompareFileUnarchiveSubmit handles restoring an archived file.
func (h *UIHandler) CompareFileUnarchiveSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/compare/tool", http.StatusSeeOther)
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/compare/tool", "error", "معرف ملف غير صالح.")
		return
	}
	if h.compareSvc != nil {
		file, err := h.compareSvc.GetFile(ctx, id)
		if err != nil || !h.checkFileOwnership(actor, file) {
			h.redirectWithNotice(w, r, "/compare/tool", "error", "غير مصرح لك بتعديل هذا الملف.")
			return
		}
		if err := h.compareSvc.UnarchiveFile(ctx, id); err != nil {
			h.redirectWithNotice(w, r, "/compare/tool", "error", h.safeMessage(err, langOf(r)))
			return
		}
	}
	h.redirectWithNotice(w, r, "/compare/tool", "success", "تم استعادة الملف من الأرشيف بنجاح.")
}

// CompareFileDeleteSubmit handles soft-deleting a file.
func (h *UIHandler) CompareFileDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/compare/tool", http.StatusSeeOther)
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/compare/tool", "error", "معرف ملف غير صالح.")
		return
	}
	if h.compareSvc != nil {
		file, err := h.compareSvc.GetFile(ctx, id)
		if err != nil || !h.checkFileOwnership(actor, file) {
			h.redirectWithNotice(w, r, "/compare/tool", "error", "غير مصرح لك بحذف هذا الملف.")
			return
		}
		if err := h.compareSvc.DeleteFile(ctx, id); err != nil {
			h.redirectWithNotice(w, r, "/compare/tool", "error", h.safeMessage(err, langOf(r)))
			return
		}
	}
	h.redirectWithNotice(w, r, "/compare/tool", "success", "تم حذف الملف بنجاح.")
}

// CompareFileMappingSubmit persists user-confirmed column mapping for a spreadsheet.
func (h *UIHandler) CompareFileMappingSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/compare/tool", http.StatusSeeOther)
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/compare/tool", "error", "معرف ملف غير صالح.")
		return
	}

	if h.compareSvc != nil {
		file, err := h.compareSvc.GetFile(ctx, id)
		if err != nil || !h.checkFileOwnership(actor, file) {
			h.redirectWithNotice(w, r, "/compare/tool", "error", "غير مصرح لك بتعديل هذا الملف.")
			return
		}

		var config compare.MappingConfig
		if nameStr := r.FormValue("name_col"); nameStr != "" {
			if idx, err := strconv.Atoi(nameStr); err == nil && idx >= 0 {
				config.NameCol = &idx
			}
		}
		if priceStr := r.FormValue("price_col"); priceStr != "" {
			if idx, err := strconv.Atoi(priceStr); err == nil && idx >= 0 {
				config.PriceCol = &idx
			}
		}
		if discStr := r.FormValue("discount_col"); discStr != "" {
			if idx, err := strconv.Atoi(discStr); err == nil && idx >= 0 {
				config.DiscountCol = &idx
			}
		}
		if codeStr := r.FormValue("code_col"); codeStr != "" {
			if idx, err := strconv.Atoi(codeStr); err == nil && idx >= 0 {
				config.CodeCol = &idx
			}
		}

		if err := h.compareSvc.SaveFileMapping(ctx, id, config); err != nil {
			h.redirectWithNotice(w, r, "/compare/tool", "error", h.safeMessage(err, langOf(r)))
			return
		}
	}

	h.redirectWithNotice(w, r, "/compare/tool", "success", "تم حفظ وتطبيق تعيين الأعمدة بنجاح.")
}

// CompareRowManualMatchSubmit allows users to manually link an uploaded row to a master product.
func (h *UIHandler) CompareRowManualMatchSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/compare/tool", http.StatusSeeOther)
		return
	}

	rowID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || rowID <= 0 {
		h.redirectWithNotice(w, r, "/compare/tool", "error", "معرف سطر غير صالح.")
		return
	}

	productID, err := strconv.ParseInt(r.FormValue("product_id"), 10, 64)
	if err != nil || productID <= 0 {
		h.redirectWithNotice(w, r, "/compare/tool", "error", "يرجى اختيار صنف صحيح للربط.")
		return
	}

	rawName := strings.TrimSpace(r.FormValue("raw_name"))

	var orgPtr *int64
	if actor.OrganizationID > 0 {
		orgPtr = &actor.OrganizationID
	}

	if h.compareSvc != nil {
		if err := h.compareSvc.SaveManualCorrection(ctx, orgPtr, rowID, rawName, productID); err != nil {
			h.redirectWithNotice(w, r, "/compare/tool", "error", h.safeMessage(err, langOf(r)))
			return
		}
	}

	h.redirectWithNotice(w, r, "/compare/tool", "success", "تم حفظ وتثبيت المطابقة بنجاح.")
}
