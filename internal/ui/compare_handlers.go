package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/go-chi/chi/v5"
	"github.com/xuri/excelize/v2"

	"github.com/muhiya/dawa24-store/internal/modules/billing"
	"github.com/muhiya/dawa24-store/internal/modules/compare"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/features"
	"github.com/muhiya/dawa24-store/internal/shared/sheet"
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
	if actor, ok := authctx.From(ctx); ok && actor.IsCustomer() {
		h.redirectWithNotice(w, r, "/customer/dashboard", "error", "هذه الصفحة مخصصة لحسابات الموردين فقط.")
		return
	}
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
	if actor.IsCustomer() {
		h.redirectWithNotice(w, r, "/customer/dashboard", "error", "هذه الصفحة مخصصة لحسابات الموردين فقط.")
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
	if actor.IsCustomer() {
		h.redirectWithNotice(w, r, "/customer/dashboard", "error", "أداة مقارنة الخصومات مخصصة لحسابات الموردين فقط.")
		return
	}
	if !actor.IsStaff && h.billSvc != nil {
		allowed, err := h.billSvc.CheckOrgEntitlement(ctx, actor.OrganizationID, actor.UserID, billing.FeatureCompareTool)
		if err != nil || !allowed {
			h.redirectWithNotice(w, r, "/vendor/subscription?upgrade=pro", "error", "يتطلب استخدام أداة مقارنة الخصومات ترقية باقة اشتراك المنشأة لتشمل هذه الميزة.")
			return
		}
	}

	noticeType := r.URL.Query().Get("notice")
	noticeMsg := r.URL.Query().Get("msg")

	// Subscription Feature Gate Check
	if !actor.IsStaff && h.billSvc != nil {
		allowed, err := h.billSvc.CheckOrgEntitlement(ctx, actor.OrganizationID, actor.UserID, billing.FeatureCompareTool)
		if err != nil || !allowed {
			plans, _ := h.billSvc.ListPlans(ctx)
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			if err := pages.SubscriptionGatePage(lang, dir, pages.SubscriptionGateProps{
				FeatureKey:   billing.FeatureCompareTool,
				FeatureTitle: "أداة مقارنة الخصومات الخاصة (Private Comparison Tool)",
				FeatureDesc:  "تتيح لك هذه الأداة رفع كشوف أسعار وخصومات الموردين وتحليل الفروقات واختيار أفضل العروض الدوائية لصيدليتك تلقائياً.",
				FeatureIcon:  "📊",
				Plans:        plans,
				Actor:        actor,
			}).Render(ctx, w); err != nil {
				h.log.ErrorContext(ctx, "render subscription gate page", "error", err)
			}
			return
		}
	}

	var files []*compare.CompareFile
	if h.compareSvc != nil {
		var orgPtr *int64
		if actor.OrganizationID > 0 {
			orgPtr = &actor.OrganizationID
		}
		files, _ = h.compareSvc.ListFiles(ctx, actor.UserID, orgPtr, nil)
		if len(files) == 0 && (actor.IsPlatformAdmin() || actor.IsStaff) {
			files, _ = h.compareSvc.ListAllFiles(ctx, "", nil)
		}
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

// CompareUploadSubmit handles uploading one or multiple supplier spreadsheet files and automatically parses rows in parallel.
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

	// 128 MB max memory for multi-file batch uploads
	if err := r.ParseMultipartForm(128 << 20); err != nil {
		h.redirectWithNotice(w, r, "/compare/tool", "error", "تعذر قراءة الملفات المرفوعة، يرجى التأكد من حجم الملفات.")
		return
	}

	var fileHeaders []*multipart.FileHeader
	if fhs, ok := r.MultipartForm.File["compare_files"]; ok && len(fhs) > 0 {
		fileHeaders = fhs
	} else if fhs, ok := r.MultipartForm.File["compare_file"]; ok && len(fhs) > 0 {
		fileHeaders = fhs
	}

	if len(fileHeaders) == 0 {
		h.redirectWithNotice(w, r, "/compare/tool", "error", "يرجى اختيار ملف Excel أو CSV واحد على الأقل.")
		return
	}

	var orgPtr *int64
	if actor.OrganizationID > 0 {
		orgPtr = &actor.OrganizationID
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
		ext := strings.ToLower(filepath.Ext(header.Filename))
		if ext != ".xlsx" && ext != ".xls" && ext != ".csv" {
			errorFiles = append(errorFiles, header.Filename+" (صيغة غير مدعومة)")
			continue
		}

		file, err := header.Open()
		if err != nil {
			errorFiles = append(errorFiles, header.Filename+" (تعذر الفتح)")
			continue
		}

		fileBytes, err := io.ReadAll(file)
		file.Close()
		if err != nil || len(fileBytes) == 0 {
			errorFiles = append(errorFiles, header.Filename+" (ملف فارغ أو تعذر قراءته)")
			continue
		}

		supplierName := strings.TrimSpace(r.FormValue("supplier_name"))
		if supplierName == "" || len(fileHeaders) > 1 {
			supplierName = strings.TrimSpace(strings.TrimSuffix(header.Filename, ext))
			supplierName = strings.ReplaceAll(supplierName, "_", " ")
			supplierName = strings.ReplaceAll(supplierName, "-", " ")
		}
		if supplierName == "" {
			supplierName = header.Filename
		}

		localURL, localErr := saveUploadedBytes(fileBytes, header.Filename, "compare")
		if localErr != nil {
			h.log.ErrorContext(ctx, "failed to save uploaded compare file to disk", "error", localErr, "file", header.Filename)
			errorFiles = append(errorFiles, header.Filename+" (تعذر الحفظ)")
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

	// 2. Process valid files with bounded parallel concurrency (up to 6 parallel workers)
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
						res.errFile = itm.filename + " (" + h.safeMessage(err, langOf(r)) + ")"
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

	// 3. Aggregate results
	var processedCount int
	var totalRows int
	var allArchived []string
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
		errMsg := "تعذر معالجة أي من الملفات المرفوعة: " + strings.Join(errorFiles, "، ")
		h.redirectWithNotice(w, r, "/compare/tool", "error", errMsg)
		return
	}

	msg := fmt.Sprintf("تم رفع ومعالجة %d كشوف موردين بنجاح (إجمالي %d صنف جاهزة للمقارنة).", processedCount, totalRows)
	firstID := uploadedIDs[0]
	queueStr := strings.Join(uploadedIDs, ",")
	redirectURL := fmt.Sprintf("/compare/tool?setup_queue=%s&setup_file=%s&setup_step=1&setup_total=%d&notice=success&msg=%s", url.QueryEscape(queueStr), firstID, len(uploadedIDs), url.QueryEscape(msg))
	if len(errorFiles) > 0 {
		redirectURL += "&warning=" + url.QueryEscape("تعذر رفع: "+strings.Join(errorFiles, "، "))
	}
	http.Redirect(w, r, redirectURL, http.StatusSeeOther)
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

// CompareFileMappingModal renders the interactive modal HTML fragment for column mapping and setup mode.
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

	// Parse optional setup mode query parameters
	isSetup := r.URL.Query().Get("setup") == "1" || r.URL.Query().Get("setup_queue") != "" || r.URL.Query().Get("queue") != ""
	queueParam := strings.TrimSpace(r.URL.Query().Get("setup_queue"))
	if queueParam == "" {
		queueParam = strings.TrimSpace(r.URL.Query().Get("queue"))
	}
	step, _ := strconv.Atoi(r.URL.Query().Get("setup_step"))
	if step <= 0 {
		step, _ = strconv.Atoi(r.URL.Query().Get("step"))
	}
	if step <= 0 {
		step = 1
	}
	total, _ := strconv.Atoi(r.URL.Query().Get("setup_total"))
	if total <= 0 {
		total, _ = strconv.Atoi(r.URL.Query().Get("total"))
	}

	var nextFileID int64
	var remainingQueue string
	if queueParam != "" {
		idParts := strings.Split(queueParam, ",")
		var cleanedParts []string
		foundCurrent := false
		for _, part := range idParts {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			partID, _ := strconv.ParseInt(part, 10, 64)
			if partID == id {
				foundCurrent = true
				continue
			}
			if foundCurrent {
				if nextFileID == 0 {
					nextFileID = partID
				}
				cleanedParts = append(cleanedParts, part)
			}
		}
		if total <= 0 {
			total = len(idParts)
		}
		if len(cleanedParts) > 0 {
			remainingQueue = strings.Join(cleanedParts, ",")
		}
	}
	if total <= 0 {
		total = 1
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.CompareFileMappingModal(file, headers, preview, detectedMapping, isSetup, step, total, remainingQueue, nextFileID).Render(ctx, w); err != nil {
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

	isSetup := r.URL.Query().Get("setup") == "1" || r.URL.Query().Get("setup_queue") != ""
	queueParam := strings.TrimSpace(r.URL.Query().Get("setup_queue"))
	step, _ := strconv.Atoi(r.URL.Query().Get("setup_step"))
	if step <= 0 {
		step = 1
	}
	total, _ := strconv.Atoi(r.URL.Query().Get("setup_total"))
	if total <= 0 {
		total = 1
	}

	var nextFileID int64
	var remainingQueue string
	if queueParam != "" {
		idParts := strings.Split(queueParam, ",")
		var cleanedParts []string
		foundCurrent := false
		for _, part := range idParts {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			partID, _ := strconv.ParseInt(part, 10, 64)
			if partID == id {
				foundCurrent = true
				continue
			}
			if foundCurrent {
				if nextFileID == 0 {
					nextFileID = partID
				}
				cleanedParts = append(cleanedParts, part)
			}
		}
		if len(cleanedParts) > 0 {
			remainingQueue = strings.Join(cleanedParts, ",")
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.CompareFileMappingPage(lang, dir, file, headers, preview, detectedMapping, isSetup, step, total, remainingQueue, nextFileID).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render compare file mapping", "error", err)
	}
}

// loadFileHeadersAndPreview extracts headers and sample preview rows from local disk or database.
func (h *UIHandler) loadFileHeadersAndPreview(ctx context.Context, file *compare.CompareFile) ([]string, [][]string) {
	var headers []string
	var preview [][]string

	if file != nil && file.StorageKey != "" {
		cleanKey := strings.TrimPrefix(filepath.FromSlash(file.StorageKey), string(filepath.Separator))
		candidates := []string{
			file.StorageKey,
			filepath.Join("data", cleanKey),
			filepath.Join(UploadBaseDir, "compare", filepath.Base(file.StorageKey)),
			filepath.Join("data", "uploads", "compare", filepath.Base(file.StorageKey)),
			filepath.Join(UploadBaseDir, "compare", filepath.Base(file.OriginalFilename)),
			filepath.Join("data", "uploads", "compare", filepath.Base(file.OriginalFilename)),
			"data" + file.StorageKey,
		}
		for _, cand := range candidates {
			if f, err := os.Open(cand); err == nil {
				headers, preview, _ = h.parseFilePreview(f, file.OriginalFilename)
				f.Close()
				if len(headers) > 0 {
					break
				}
			}
		}
	}

	// Fallback to already extracted rows in DB if disk read didn't populate headers
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
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, nil, err
	}
	allRows, err := sheet.ReadRows(data, filename)
	if err != nil || len(allRows) == 0 {
		return nil, nil, fmt.Errorf("empty or unparseable spreadsheet: %w", err)
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
		if r.Header.Get("Accept") == "application/json" {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		http.Redirect(w, r, "/auth/login?redirect=/compare/tool", http.StatusSeeOther)
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		if r.Header.Get("Accept") == "application/json" {
			http.Error(w, `{"error":"invalid file id"}`, http.StatusBadRequest)
			return
		}
		h.redirectWithNotice(w, r, "/compare/tool", "error", "معرف ملف غير صالح.")
		return
	}

	if h.compareSvc != nil {
		file, err := h.compareSvc.GetFile(ctx, id)
		if err != nil || !h.checkFileOwnership(actor, file) {
			if r.Header.Get("Accept") == "application/json" {
				http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
				return
			}
			h.redirectWithNotice(w, r, "/compare/tool", "error", "غير مصرح لك بتعديل هذا الملف.")
			return
		}

		// Update supplier name if modified in setup wizard
		if newSupplierName := strings.TrimSpace(r.FormValue("supplier_name")); newSupplierName != "" && newSupplierName != file.SupplierName {
			_ = h.compareSvc.RenameFile(ctx, id, newSupplierName)
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
			if r.Header.Get("Accept") == "application/json" {
				http.Error(w, `{"error":"`+h.safeMessage(err, langOf(r))+`"}`, http.StatusInternalServerError)
				return
			}
			h.redirectWithNotice(w, r, "/compare/tool", "error", h.safeMessage(err, langOf(r)))
			return
		}
	}

	queue := strings.TrimSpace(r.FormValue("setup_queue"))
	if queue == "" {
		queue = strings.TrimSpace(r.FormValue("queue"))
	}
	step, _ := strconv.Atoi(r.FormValue("step"))
	total, _ := strconv.Atoi(r.FormValue("total"))

	var nextFileID int64
	var nextQueue string
	if queue != "" {
		parts := strings.Split(queue, ",")
		if len(parts) > 0 {
			nextFileID, _ = strconv.ParseInt(strings.TrimSpace(parts[0]), 10, 64)
			if len(parts) > 1 {
				nextQueue = strings.Join(parts[1:], ",")
			}
		}
	}

	if r.Header.Get("Accept") == "application/json" {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success":         true,
			"next_file_id":    nextFileID,
			"remaining_queue": nextQueue,
			"step":            step + 1,
			"total":           total,
		})
		return
	}

	if nextFileID > 0 {
		redirectURL := fmt.Sprintf("/compare/tool?setup_file=%d&setup_queue=%s&setup_step=%d&setup_total=%d", nextFileID, url.QueryEscape(nextQueue), step+1, total)
		http.Redirect(w, r, redirectURL, http.StatusSeeOther)
		return
	}

	h.redirectWithNotice(w, r, "/compare/tool", "success", "تم حفظ وتطبيق ضبط أعمدة كشف المورد بنجاح.")
}

// CompareFileSkipSubmit handles skipping an uploaded file in setup mode.
func (h *UIHandler) CompareFileSkipSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
		if r.Header.Get("Accept") == "application/json" {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		http.Redirect(w, r, "/auth/login?redirect=/compare/tool", http.StatusSeeOther)
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		if r.Header.Get("Accept") == "application/json" {
			http.Error(w, `{"error":"invalid id"}`, http.StatusBadRequest)
			return
		}
		h.redirectWithNotice(w, r, "/compare/tool", "error", "معرف ملف غير صالح.")
		return
	}
	if h.compareSvc != nil {
		file, err := h.compareSvc.GetFile(ctx, id)
		if err == nil && h.checkFileOwnership(actor, file) {
			_ = h.compareSvc.DeleteFile(ctx, id)
		}
	}

	queue := strings.TrimSpace(r.FormValue("setup_queue"))
	if queue == "" {
		queue = strings.TrimSpace(r.FormValue("queue"))
	}
	step, _ := strconv.Atoi(r.FormValue("step"))
	total, _ := strconv.Atoi(r.FormValue("total"))

	var nextFileID int64
	var nextQueue string
	if queue != "" {
		parts := strings.Split(queue, ",")
		if len(parts) > 0 {
			nextFileID, _ = strconv.ParseInt(strings.TrimSpace(parts[0]), 10, 64)
			if len(parts) > 1 {
				nextQueue = strings.Join(parts[1:], ",")
			}
		}
	}

	if r.Header.Get("Accept") == "application/json" {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success":         true,
			"skipped_id":      id,
			"next_file_id":    nextFileID,
			"remaining_queue": nextQueue,
			"step":            step + 1,
			"total":           total,
		})
		return
	}

	if nextFileID > 0 {
		redirectURL := fmt.Sprintf("/compare/tool?setup_file=%d&setup_queue=%s&setup_step=%d&setup_total=%d", nextFileID, url.QueryEscape(nextQueue), step+1, total)
		http.Redirect(w, r, redirectURL, http.StatusSeeOther)
		return
	}

	h.redirectWithNotice(w, r, "/compare/tool", "success", "تم تخطي الملف بنجاح.")
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

// CompareQuickSearch handles GET /compare/search?q=... and /api/v1/compare/search?q=...
func (h *UIHandler) CompareQuickSearch(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor := authctx.FromContext(ctx)

	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(query) < 2 {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"query":         query,
			"total_matches": 0,
			"items":         []any{},
		})
		return
	}

	var orgPtr *int64
	if actor.OrganizationID > 0 {
		orgPtr = &actor.OrganizationID
	}

	if h.compareSvc == nil {
		http.Error(w, `{"error":"service unavailable"}`, http.StatusServiceUnavailable)
		return
	}

	results, err := h.compareSvc.SearchAcrossSuppliersAndCatalog(ctx, actor.UserID, orgPtr, query)
	if err != nil {
		h.log.ErrorContext(ctx, "compare quick search error", "error", err, "query", query)
		results = &compare.CompareSearchResults{
			Query: query,
			Items: []*compare.CompareSearchResultItem{},
		}
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(results)
}
