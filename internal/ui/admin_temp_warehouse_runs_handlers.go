package ui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/compare"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/importrun"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// resolveWarehouseRunBase extracts the current route prefix.
func resolveWarehouseRunBase(r *http.Request) string {
	path := r.URL.Path
	if strings.Contains(path, "/admin/my/") {
		return "/admin/my/temparte-warehouses"
	}
	if strings.Contains(path, "/admin/team/") {
		return "/admin/team/temparte-warehouses"
	}
	if strings.Contains(path, "/admin/admins/") {
		return "/admin/admins/temparte-warehouses"
	}
	if strings.Contains(path, "/admin/plan/") {
		return "/admin/plan/temparte-warehouses"
	}
	return "/admin/user/temparte-warehouses"
}

// AdminTempWarehouseRunDispatcher redirects a run root URL to its current active phase.
func (h *UIHandler) AdminTempWarehouseRunDispatcher(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	base := resolveWarehouseRunBase(r)
	runIDStr := chi.URLParam(r, "runID")

	run, err := h.ResolveTempWarehouseRun(ctx, runIDStr)
	if err != nil || run == nil {
		h.redirectWithNotice(w, r, base, "error", "تعذر العثور على جلسة الرفع المحددة")
		return
	}

	switch run.Phase {
	case PhaseReview:
		http.Redirect(w, r, fmt.Sprintf("%s/runs/%d/review", base, run.ID), http.StatusSeeOther)
	case PhaseStaging, PhaseCommitting:
		http.Redirect(w, r, fmt.Sprintf("%s/runs/%d/progress", base, run.ID), http.StatusSeeOther)
	case PhaseDone:
		h.redirectWithNotice(w, r, base, "success", "اكتملت جلسة الرفع بنجاح")
	default:
		http.Redirect(w, r, fmt.Sprintf("%s/runs/%d/mapping", base, run.ID), http.StatusSeeOther)
	}
}

// AdminTempWarehouseRunMappingPage renders the full-page column mapping wizard.
func (h *UIHandler) AdminTempWarehouseRunMappingPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	base := resolveWarehouseRunBase(r)
	runIDStr := chi.URLParam(r, "runID")

	run, err := h.ResolveTempWarehouseRun(ctx, runIDStr)
	if err != nil || run == nil {
		h.redirectWithNotice(w, r, base, "error", "تعذر العثور على جلسة الرفع")
		return
	}

	payload, err := DecodeTempWarehousePayload(run.Payload)
	if err != nil || len(payload.FileIDs) == 0 {
		h.redirectWithNotice(w, r, base, "error", "بيانات الجلسة غير صالحة")
		return
	}

	sysCtx := database.AsSystem(ctx)

	// Determine active file ID
	activeFileID := payload.CurrentFileID
	if qID, _ := strconv.ParseInt(r.URL.Query().Get("file_id"), 10, 64); qID > 0 {
		for _, fid := range payload.FileIDs {
			if fid == qID {
				activeFileID = qID
				break
			}
		}
	}
	if activeFileID <= 0 && len(payload.FileIDs) > 0 {
		activeFileID = payload.FileIDs[0]
	}

	var curFile *compare.CompareFile
	var curFileView *pages.TempWarehouseMappingFileView
	fileViews := make([]*pages.TempWarehouseMappingFileView, 0, len(payload.FileIDs))

	for idx, fid := range payload.FileIDs {
		f, fErr := h.compareSvc.GetFile(sysCtx, fid)
		if fErr != nil || f == nil {
			continue
		}
		if fid == activeFileID {
			curFile = f
			payload.Step = idx + 1
			payload.CurrentFileID = fid
		}

		isMapped := f.MappingConfig.NameCol != nil && *f.MappingConfig.NameCol >= 0
		codeCol, nameCol, priceCol, discountCol := -1, -1, -1, -1
		if f.MappingConfig.CodeCol != nil {
			codeCol = *f.MappingConfig.CodeCol
		}
		if f.MappingConfig.NameCol != nil {
			nameCol = *f.MappingConfig.NameCol
		}
		if f.MappingConfig.PriceCol != nil {
			priceCol = *f.MappingConfig.PriceCol
		}
		if f.MappingConfig.DiscountCol != nil {
			discountCol = *f.MappingConfig.DiscountCol
		}

		fv := &pages.TempWarehouseMappingFileView{
			ID:           f.ID,
			Filename:     f.OriginalFilename,
			SupplierName: f.SupplierName,
			RowCount:     f.RowCount,
			Status:       string(f.Status),
			IsMapped:     isMapped,
			CodeCol:      codeCol,
			NameCol:      nameCol,
			PriceCol:     priceCol,
			DiscountCol:  discountCol,
		}
		fileViews = append(fileViews, fv)
		if fid == activeFileID {
			curFileView = fv
		}
	}

	if curFile == nil && len(payload.FileIDs) > 0 {
		h.redirectWithNotice(w, r, base, "error", "تعذر العثور على ملف المستودع")
		return
	}

	headers, preview := h.loadFileHeadersAndPreview(sysCtx, curFile)
	curFileView.Headers = headers
	curFileView.Preview = preview

	if !curFileView.IsMapped && len(headers) > 0 {
		c, n, p, d := detectTempWarehouseCols(headers, "", "", "", "")
		if curFileView.CodeCol < 0 {
			curFileView.CodeCol = c
		}
		if curFileView.NameCol < 0 {
			curFileView.NameCol = n
		}
		if curFileView.PriceCol < 0 {
			curFileView.PriceCol = p
		}
		if curFileView.DiscountCol < 0 {
			curFileView.DiscountCol = d
		}
	}

	runView := &pages.TempWarehouseRunView{
		ID:            run.ID,
		PublicID:      run.PublicID,
		Filename:      run.Filename,
		State:         string(run.State),
		Phase:         run.Phase,
		Percent:       run.Percent,
		Step:          payload.Step,
		TotalFiles:    len(payload.FileIDs),
		CurrentFileID: activeFileID,
		BaseURL:       base,
		FileIDs:       payload.FileIDs,
		SupplierNames: payload.SupplierNames,
	}

	h.renderPage(ctx, w, "render run mapping page", pages.AdminTempWarehouseRunMappingPage(runView, curFileView, fileViews, lang, dir))
}

// AdminTempWarehouseRunMappingSubmit autosaves or commits column choices for a file in the run.
func (h *UIHandler) AdminTempWarehouseRunMappingSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	base := resolveWarehouseRunBase(r)
	runIDStr := chi.URLParam(r, "runID")

	run, err := h.ResolveTempWarehouseRun(ctx, runIDStr)
	if err != nil || run == nil {
		if isJSONOrAJAX(r) {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": "الجلسة غير موجودة"})
			return
		}
		h.redirectWithNotice(w, r, base, "error", "تعذر العثور على جلسة الرفع")
		return
	}

	if err := r.ParseForm(); err != nil {
		if isJSONOrAJAX(r) {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": "بيانات الطلب غير صالحة"})
			return
		}
		h.redirectWithNotice(w, r, fmt.Sprintf("%s/runs/%d/mapping", base, run.ID), "error", "خطأ في قراءة البيانات")
		return
	}

	fileID, _ := strconv.ParseInt(r.FormValue("file_id"), 10, 64)
	colName, _ := strconv.Atoi(r.FormValue("col_name"))
	colCode, _ := strconv.Atoi(r.FormValue("col_code"))
	colPrice, _ := strconv.Atoi(r.FormValue("col_price"))
	colDiscount, _ := strconv.Atoi(r.FormValue("col_discount"))
	isAutosave := r.FormValue("is_autosave") == "1" || isJSONOrAJAX(r)
	action := r.FormValue("action")

	cfg := compare.MappingConfig{}
	if colName >= 0 {
		cfg.NameCol = &colName
	}
	if colCode >= 0 {
		cfg.CodeCol = &colCode
	}
	if colPrice >= 0 {
		cfg.PriceCol = &colPrice
	}
	if colDiscount >= 0 {
		cfg.DiscountCol = &colDiscount
	}

	sysCtx := database.AsSystem(ctx)

	// Persist mapping on the compare file
	if fileID > 0 && h.compareSvc != nil {
		_ = h.compareSvc.UpdateFileMapping(sysCtx, fileID, cfg)
	}

	// Update payload in run
	payload, _ := DecodeTempWarehousePayload(run.Payload)
	if payload.Mappings == nil {
		payload.Mappings = make(map[int64]compare.MappingConfig)
	}
	payload.Mappings[fileID] = cfg
	payloadBytes, _ := json.Marshal(payload)
	_ = h.importRunRepo.UpdatePayload(sysCtx, run.ID, payloadBytes)

	if isAutosave {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "file_id": fileID})
		return
	}

	if action == "review" {
		_ = h.importRunRepo.UpdateRunProgress(sysCtx, run.ID, PhaseReview, 70, 0, 0)
		http.Redirect(w, r, fmt.Sprintf("%s/runs/%d/review", base, run.ID), http.StatusSeeOther)
		return
	}

	// Next file logic
	nextFileID := int64(0)
	for i, fid := range payload.FileIDs {
		if fid == fileID && i+1 < len(payload.FileIDs) {
			nextFileID = payload.FileIDs[i+1]
			break
		}
	}

	if nextFileID > 0 {
		payload.CurrentFileID = nextFileID
		pBytes, _ := json.Marshal(payload)
		_ = h.importRunRepo.UpdatePayload(sysCtx, run.ID, pBytes)
		http.Redirect(w, r, fmt.Sprintf("%s/runs/%d/mapping?file_id=%d", base, run.ID, nextFileID), http.StatusSeeOther)
		return
	}

	// Reached end of files -> redirect to review
	_ = h.importRunRepo.UpdateRunProgress(sysCtx, run.ID, PhaseReview, 80, 0, 0)
	h.redirectWithNotice(w, r, fmt.Sprintf("%s/runs/%d/review", base, run.ID), "success", i18n.T(lang, "admin.temp_wh.all_files_mapped_msg"))
}

// AdminTempWarehouseRunReviewPage renders the summary review page.
func (h *UIHandler) AdminTempWarehouseRunReviewPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	base := resolveWarehouseRunBase(r)
	runIDStr := chi.URLParam(r, "runID")

	run, err := h.ResolveTempWarehouseRun(ctx, runIDStr)
	if err != nil || run == nil {
		h.redirectWithNotice(w, r, base, "error", "تعذر العثور على جلسة الرفع")
		return
	}

	payload, _ := DecodeTempWarehousePayload(run.Payload)
	sysCtx := database.AsSystem(ctx)

	fileViews := make([]*pages.TempWarehouseMappingFileView, 0, len(payload.FileIDs))
	for _, fid := range payload.FileIDs {
		f, fErr := h.compareSvc.GetFile(sysCtx, fid)
		if fErr != nil || f == nil {
			continue
		}
		isMapped := f.MappingConfig.NameCol != nil && *f.MappingConfig.NameCol >= 0
		fileViews = append(fileViews, &pages.TempWarehouseMappingFileView{
			ID:           f.ID,
			Filename:     f.OriginalFilename,
			SupplierName: f.SupplierName,
			RowCount:     f.RowCount,
			Status:       string(f.Status),
			IsMapped:     isMapped,
		})
	}

	runView := &pages.TempWarehouseRunView{
		ID:            run.ID,
		PublicID:      run.PublicID,
		Filename:      run.Filename,
		State:         string(run.State),
		Phase:         PhaseReview,
		Percent:       80,
		TotalFiles:    len(payload.FileIDs),
		BaseURL:       base,
		FileIDs:       payload.FileIDs,
		SupplierNames: payload.SupplierNames,
	}

	h.renderPage(ctx, w, "render run review page", pages.AdminTempWarehouseRunReviewPage(runView, fileViews, lang, dir))
}

// AdminTempWarehouseRunCommitSubmit commits all files in the batch session.
func (h *UIHandler) AdminTempWarehouseRunCommitSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	base := resolveWarehouseRunBase(r)
	runIDStr := chi.URLParam(r, "runID")

	run, err := h.ResolveTempWarehouseRun(ctx, runIDStr)
	if err != nil || run == nil {
		h.redirectWithNotice(w, r, base, "error", "تعذر العثور على جلسة الرفع")
		return
	}

	payload, _ := DecodeTempWarehousePayload(run.Payload)
	sysCtx := database.AsSystem(ctx)

	// Activate and re-parse files
	totalProcessed := 0
	for _, fid := range payload.FileIDs {
		f, _ := h.compareSvc.GetFile(sysCtx, fid)
		if f != nil {
			_ = h.compareSvc.UpdateFileStatus(sysCtx, fid, compare.FileStatusActive)
			totalProcessed += f.RowCount
		}
	}

	// Update run to completed
	_ = h.importRunRepo.UpdateRunProgress(sysCtx, run.ID, PhaseDone, 100, totalProcessed, totalProcessed)
	_ = h.importRunRepo.MarkRunCommitted(sysCtx, run.ID)

	http.Redirect(w, r, fmt.Sprintf("%s/runs/%d/progress", base, run.ID), http.StatusSeeOther)
}

// AdminTempWarehouseRunProgressPage renders live progress or JSON status polling.
func (h *UIHandler) AdminTempWarehouseRunProgressPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	base := resolveWarehouseRunBase(r)
	runIDStr := chi.URLParam(r, "runID")

	run, err := h.ResolveTempWarehouseRun(ctx, runIDStr)
	if err != nil || run == nil {
		if isJSONOrAJAX(r) {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": "not found"})
			return
		}
		h.redirectWithNotice(w, r, base, "error", "تعذر العثور على جلسة الرفع")
		return
	}

	if r.URL.Query().Get("poll") == "1" || isJSONOrAJAX(r) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":      run.ID,
			"percent": run.Percent,
			"phase":   run.Phase,
			"state":   run.State,
			"done":    run.Phase == PhaseDone || run.Percent >= 100,
		})
		return
	}

	payload, _ := DecodeTempWarehousePayload(run.Payload)
	runView := &pages.TempWarehouseRunView{
		ID:         run.ID,
		PublicID:   run.PublicID,
		Filename:   run.Filename,
		State:      string(run.State),
		Phase:      run.Phase,
		Percent:    run.Percent,
		TotalFiles: len(payload.FileIDs),
		BaseURL:    base,
	}

	h.renderPage(ctx, w, "render run progress page", pages.AdminTempWarehouseRunProgressPage(runView, lang, dir))
}

// AdminTempWarehouseRunCancelSubmit cancels the run.
func (h *UIHandler) AdminTempWarehouseRunCancelSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	base := resolveWarehouseRunBase(r)
	runIDStr := chi.URLParam(r, "runID")

	run, err := h.ResolveTempWarehouseRun(ctx, runIDStr)
	if err != nil || run == nil {
		h.redirectWithNotice(w, r, base, "error", "تعذر العثور على جلسة الرفع")
		return
	}

	sysCtx := database.AsSystem(ctx)
	_ = h.importRunRepo.UpdateRunState(sysCtx, run.ID, importrun.StateCancelled, PhaseFailed)
	h.redirectWithNotice(w, r, base, "success", "تم إلغاء جلسة الرفع")
}
