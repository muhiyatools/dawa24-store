package ui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/compare"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// CompareFileMappingModal renders the interactive modal HTML fragment for column mapping and setup mode.
func (h *UIHandler) CompareFileMappingModal(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, i18n.T(lang, "compare.file.invalid_id"), http.StatusBadRequest)
		return
	}

	var file *compare.CompareFile
	if h.compareSvc != nil {
		file, err = h.compareSvc.GetFile(ctx, id)
		if err != nil {
			http.Error(w, i18n.T(lang, "compare.file.not_found"), http.StatusNotFound)
			return
		}
	}

	if !h.checkFileOwnership(actor, file) {
		http.Error(w, i18n.T(lang, "compare.file.access_forbidden"), http.StatusForbidden)
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

	h.renderPage(ctx, w, "render compare file mapping modal", pages.CompareFileMappingModal(file, headers, preview, detectedMapping, isSetup, step, total, remainingQueue, nextFileID))
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
		h.redirectWithNotice(w, r, "/compare/tool", "error", i18n.T(lang, "compare.file.invalid_id"))
		return
	}

	var file *compare.CompareFile
	if h.compareSvc != nil {
		file, err = h.compareSvc.GetFile(ctx, id)
		if err != nil {
			h.redirectWithNotice(w, r, "/compare/tool", "error", h.safeMessage(err, lang))
			return
		}
	}

	if !h.checkFileOwnership(actor, file) {
		h.redirectWithNotice(w, r, "/compare/tool", "error", i18n.T(lang, "compare.file.access_forbidden"))
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

	h.renderPage(ctx, w, "render compare file mapping", pages.CompareFileMappingPage(lang, dir, file, headers, preview, detectedMapping, isSetup, step, total, remainingQueue, nextFileID))
}

// CompareFileMappingSubmit persists user-confirmed column mapping for a spreadsheet.
func (h *UIHandler) CompareFileMappingSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
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
		h.redirectWithNotice(w, r, "/compare/tool", "error", i18n.T(lang, "compare.file.invalid_id"))
		return
	}

	if h.compareSvc != nil {
		file, err := h.compareSvc.GetFile(ctx, id)
		if err != nil || !h.checkFileOwnership(actor, file) {
			if r.Header.Get("Accept") == "application/json" {
				http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
				return
			}
			h.redirectWithNotice(w, r, "/compare/tool", "error", i18n.T(lang, "compare.file.edit_forbidden"))
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
				http.Error(w, `{"error":"`+h.safeMessage(err, lang)+`"}`, http.StatusInternalServerError)
				return
			}
			h.redirectWithNotice(w, r, "/compare/tool", "error", h.safeMessage(err, lang))
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

	h.redirectWithNotice(w, r, "/compare/tool", "success", i18n.T(lang, "compare.mapping.saved_success"))
}
