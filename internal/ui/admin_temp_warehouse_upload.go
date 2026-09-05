package ui

import (
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
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
	// The CONFIGURED directory first, then the compiled-in default.
	//
	// saveUploadedBytes writes to GetUploadBaseDir(), which honours UPLOAD_DIR
	// and DATA_DIR; this looked only under the UploadBaseDir constant and the
	// literal "data/uploads". On any deployment that sets either variable the
	// two disagreed, and a file that had just been written was reported as
	// missing — which is precisely the failure the detached staging pass
	// depends on not happening, because it re-reads what the request wrote.
	base := GetUploadBaseDir()
	candidates := []string{
		storageKey,
		filepath.Join(base, category, filepath.Base(storageKey)),
		filepath.Join(base, filepath.FromSlash(strings.TrimPrefix(storageKey, "/uploads/"))),
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

// AdminTempWarehouseUploadSubmit handles bulk and single file upload (optimized for 60-100+ files in parallel).
func (h *UIHandler) AdminTempWarehouseUploadSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)

	// 500MB max limit to comfortably allow 60-100+ bulk files
	if err := parseImportUpload(w, r); err != nil {
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

			// Registers the file and returns; the parse runs on a goroutine
			// that outlives this request. Reading a hundred workbooks inside
			// the POST is what made this endpoint unusable at the batch sizes
			// it was written for.
			results[idx] = h.registerTempWarehouseFile(
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
			// RowCount is zero here by design: the rows have not been read yet.
			// The screen polls /admin/user/temparte-warehouses/staging for the
			// real figures as each file finishes.
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
			"staging":          true,
			"message":          fmt.Sprintf(i18n.T(lang, "admin.temp_warehouse.staging_message"), successCount, len(fileHeaders)),
		})
		return
	}

	if successCount == 0 && failCount > 0 {
		h.redirectWithNotice(w, r, "/admin/user/temparte-warehouses", "error", i18n.T(lang, "admin.temp_warehouse.all_files_failed_prefix")+strings.Join(errorMessages, " | "))
		return
	}

	successMsg := fmt.Sprintf(i18n.T(lang, "admin.temp_warehouse.staging_message"), successCount, len(fileHeaders))
	if failCount > 0 {
		successMsg += fmt.Sprintf(i18n.T(lang, "admin.temp_warehouse.fail_count_suffix"), failCount)
	}
	h.redirectWithNotice(w, r, "/admin/user/temparte-warehouses", "success", successMsg)
}
