package ui

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/compare"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/filesecurity"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

type streamedFileItem struct {
	index        int
	filename     string
	supplierName string
	contentType  string
	size         int64
	scanned      filesecurity.Scanned
	storageKey   string
}

type streamedFileResult struct {
	index    int
	file     *compare.CompareFile
	archived []string
	err      error
	errFile  string
}

// AdminOrgImportTempWarehouseUploadSubmit handles streaming multi-file upload (up to 80 files)
// without buffering 500 MB into heap memory, staging each file for the target organization.
func (h *UIHandler) AdminOrgImportTempWarehouseUploadSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/admin/organizations/import", http.StatusSeeOther)
		return
	}

	if h.compareSvc == nil {
		h.redirectWithNotice(w, r, "/admin/organizations/import", "error", i18n.T(lang, "common.compare_service_unavailable"))
		return
	}

	// 1. Resolve Target Organization ID from URL parameter or form
	var targetOrgID int64
	if param := chi.URLParam(r, "orgID"); param != "" {
		targetOrgID, _ = strconv.ParseInt(param, 10, 64)
	}

	mr, err := r.MultipartReader()
	if err != nil {
		h.redirectWithNotice(w, r, "/admin/organizations/import", "error", i18n.T(lang, "compare.upload.read_failed"))
		return
	}

	var validItems []streamedFileItem
	var errorFiles []string
	formSupplierName := ""
	fileIndex := 0
	maxBatchLimit := 80

	// 2. Stream parts one-by-one from the multipart socket to disk
	for {
		part, pErr := mr.NextPart()
		if pErr == io.EOF {
			break
		}
		if pErr != nil {
			h.log.WarnContext(ctx, "multipart stream read interrupted", "error", pErr)
			break
		}

		formName := part.FormName()
		filename := part.FileName()

		if filename == "" {
			valBytes, _ := io.ReadAll(io.LimitReader(part, 1<<16))
			val := strings.TrimSpace(string(valBytes))
			if formName == "org_id" && targetOrgID <= 0 {
				targetOrgID, _ = strconv.ParseInt(val, 10, 64)
			} else if formName == "supplier_name" {
				formSupplierName = val
			}
			part.Close()
			continue
		}

		if len(validItems) >= maxBatchLimit {
			errorFiles = append(errorFiles, filename+" ("+i18n.T(lang, "admin.temp_warehouse.batch_limit_reached")+")")
			part.Close()
			continue
		}

		if !SupportedUploadName(filename) {
			errorFiles = append(errorFiles, filename+" ("+i18n.T(lang, "compare.upload.unsupported_format")+")")
			part.Close()
			continue
		}

		// Stream to temporary file on disk using a 32KB buffer
		tmpFile, tmpErr := os.CreateTemp("", "org_tw_*.tmp")
		if tmpErr != nil {
			errorFiles = append(errorFiles, filename+" ("+i18n.T(lang, "compare.upload.save_failed")+")")
			part.Close()
			continue
		}
		tmpPath := tmpFile.Name()

		written, cErr := io.Copy(tmpFile, part)
		tmpFile.Close()
		part.Close()

		if cErr != nil || written == 0 {
			_ = os.Remove(tmpPath)
			errorFiles = append(errorFiles, filename+" ("+i18n.T(lang, "compare.upload.empty_or_unread")+")")
			continue
		}

		fileBytes, readErr := os.ReadFile(tmpPath)
		_ = os.Remove(tmpPath)
		if readErr != nil || len(fileBytes) == 0 {
			errorFiles = append(errorFiles, filename+" ("+i18n.T(lang, "compare.upload.empty_or_unread")+")")
			continue
		}

		// Security inspection before final storage
		scanned, sErr := filesecurity.Scan(fileBytes, filename)
		if sErr != nil {
			errorFiles = append(errorFiles, filename+" ("+filesecurity.SecurityErrorMessage+")")
			continue
		}

		storageKey, saveErr := saveUploadedBytes(fileBytes, filename, "temp_warehouses")
		if saveErr != nil {
			h.log.ErrorContext(ctx, "failed to store temp warehouse file", "error", saveErr, "file", filename)
			errorFiles = append(errorFiles, filename+" ("+i18n.T(lang, "compare.upload.save_failed")+")")
			continue
		}

		suppName := formSupplierName
		if suppName == "" || fileIndex > 0 {
			cleanName := strings.TrimSuffix(filename, filepath.Ext(filename))
			cleanName = strings.ReplaceAll(cleanName, "_", " ")
			cleanName = strings.ReplaceAll(cleanName, "-", " ")
			suppName = strings.TrimSpace(cleanName)
		}
		if suppName == "" {
			suppName = filename
		}

		validItems = append(validItems, streamedFileItem{
			index:        fileIndex,
			filename:     filename,
			supplierName: suppName,
			contentType:  part.Header.Get("Content-Type"),
			size:         written,
			scanned:      scanned,
			storageKey:   storageKey,
		})
		fileIndex++
	}

	if targetOrgID <= 0 {
		h.redirectWithNotice(w, r, "/admin/organizations/import", "error", i18n.T(lang, "validation.invalid_id"))
		return
	}

	if len(validItems) == 0 {
		errMsg := i18n.T(lang, "compare.upload.none_processed_prefix")
		if len(errorFiles) > 0 {
			errMsg += " " + strings.Join(errorFiles, ", ")
		}
		h.redirectWithNotice(w, r, "/admin/organizations/import", "error", errMsg)
		return
	}

	// 3. Resolve target organization details under AsSystem
	sysCtx := database.AsSystem(ctx)
	var targetOrg *org.Organization
	if h.orgSvc != nil {
		targetOrg, _ = h.orgSvc.GetOrganization(sysCtx, targetOrgID)
	}
	orgName := fmt.Sprintf("منشأة #%d", targetOrgID)
	if targetOrg != nil {
		if targetOrg.TradeName != nil && targetOrg.TradeName["ar"] != "" {
			orgName = targetOrg.TradeName["ar"]
		} else if targetOrg.LegalName != "" {
			orgName = targetOrg.LegalName
		}
	}

	// 4. Process valid items in parallel with bounded worker pool (6 workers)
	numWorkers := 6
	if len(validItems) < numWorkers {
		numWorkers = len(validItems)
	}

	results := make([]streamedFileResult, len(validItems))
	itemChan := make(chan streamedFileItem, len(validItems))
	var wg sync.WaitGroup

	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				if rec := recover(); rec != nil {
					h.log.Error("temp warehouse upload worker panicked", "panic", rec)
				}
			}()
			for itm := range itemChan {
				// Stages the file row under AsSystem scoped to the target organization
				staged, stErr := h.compareSvc.RegisterAndStage(
					sysCtx, actor.UserID, &targetOrgID, itm.supplierName, itm.filename,
					itm.contentType, itm.size, itm.storageKey, itm.scanned,
				)
				res := streamedFileResult{index: itm.index, err: stErr}
				if staged != nil {
					res.file = staged.File
					res.archived = staged.Archived
				}
				if stErr != nil {
					res.errFile = itm.filename + " (" + h.safeMessage(stErr, lang) + ")"
				}
				results[itm.index] = res
			}
		}()
	}

	for _, item := range validItems {
		itemChan <- item
	}
	close(itemChan)
	wg.Wait()

	// 5. Calculate summary metrics and format success notice
	successCount := 0
	for _, res := range results {
		if res.err == nil && res.file != nil {
			successCount++
		} else if res.errFile != "" {
			errorFiles = append(errorFiles, res.errFile)
		}
	}

	h.log.InfoContext(ctx, "admin streamed multi-file temp warehouse upload completed",
		"actor_id", actor.UserID,
		"target_org_id", targetOrgID,
		"target_org_name", orgName,
		"success_count", successCount,
		"error_count", len(errorFiles),
	)

	if successCount == 0 {
		errMsg := i18n.T(lang, "compare.upload.none_processed_prefix") + " " + strings.Join(errorFiles, ", ")
		h.redirectWithNotice(w, r, "/admin/organizations/import", "error", errMsg)
		return
	}

	noticeMsg := fmt.Sprintf("تم بنجاح رفع وتجهيز %d ملف مستودع مؤقت لمنشأة (%s).", successCount, orgName)
	if len(errorFiles) > 0 {
		noticeMsg += fmt.Sprintf(" (تعذر تجهيز %d ملف)", len(errorFiles))
	}

	h.redirectWithNotice(w, r, fmt.Sprintf("/admin/organizations/import/%d/compare", targetOrgID), "success", noticeMsg)
}

// AdminOrgImportComparePage renders the 3-column Compare Tool workspace on behalf of the target organization.
func (h *UIHandler) AdminOrgImportComparePage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	orgID, _ := strconv.ParseInt(chi.URLParam(r, "orgID"), 10, 64)
	if orgID <= 0 {
		h.redirectWithNotice(w, r, "/admin/organizations/import", "error", i18n.T(lang, "validation.invalid_id"))
		return
	}

	sysCtx := database.WithTenant(database.AsSystem(ctx), orgID)
	var orgName string
	if h.orgSvc != nil {
		targetOrg, err := h.orgSvc.GetOrganization(sysCtx, orgID)
		if err != nil || targetOrg == nil {
			h.redirectWithNotice(w, r, "/admin/organizations/import", "error", "المنشأة المحددة غير موجودة")
			return
		}
		orgName, _ = h.resolveTargetOrgInfo(sysCtx, orgID)
	}

	var activeFiles []*compare.CompareFile
	if h.compareSvc != nil {
		activeFiles, _ = h.compareSvc.ListFiles(sysCtx, 0, &orgID, nil)
	}

	noticeType := r.URL.Query().Get("notice")
	noticeMsg := r.URL.Query().Get("msg")

	view := pages.CompareToolView{
		Lang:            lang,
		Dir:             dir,
		Files:           activeFiles,
		MaxAllowedFiles: 80,
		NoticeType:      noticeType,
		NoticeMsg:       noticeMsg,
		Audience:        "admin",
		TargetOrgID:     orgID,
		TargetOrgName:   orgName,
		UploadURL:       fmt.Sprintf("/admin/organizations/import/%d/temp-warehouse/upload", orgID),
	}

	h.renderPage(ctx, w, "render admin org compare tool", pages.CompareToolPage(view))
}

