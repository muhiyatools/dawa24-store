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
)

// CompareFileSkipSubmit handles skipping an uploaded file in setup mode.
func (h *UIHandler) CompareFileSkipSubmit(w http.ResponseWriter, r *http.Request) {
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
			http.Error(w, `{"error":"invalid id"}`, http.StatusBadRequest)
			return
		}
		h.redirectWithNotice(w, r, "/compare/tool", "error", i18n.T(lang, "compare.file.invalid_id"))
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

	h.redirectWithNotice(w, r, "/compare/tool", "success", i18n.T(lang, "compare.mapping.skipped_success"))
}

// CompareRowManualMatchSubmit allows users to manually link an uploaded row to a master product.
func (h *UIHandler) CompareRowManualMatchSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/compare/tool", http.StatusSeeOther)
		return
	}

	rowID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || rowID <= 0 {
		h.redirectWithNotice(w, r, "/compare/tool", "error", i18n.T(lang, "compare.mapping.invalid_row_id"))
		return
	}

	productID, err := strconv.ParseInt(r.FormValue("product_id"), 10, 64)
	if err != nil || productID <= 0 {
		h.redirectWithNotice(w, r, "/compare/tool", "error", i18n.T(lang, "compare.mapping.valid_product_required"))
		return
	}

	rawName := strings.TrimSpace(r.FormValue("raw_name"))

	var orgPtr *int64
	if actor.OrganizationID > 0 {
		orgPtr = &actor.OrganizationID
	}

	if h.compareSvc != nil {
		if err := h.compareSvc.SaveManualCorrection(ctx, orgPtr, rowID, rawName, productID); err != nil {
			h.redirectWithNotice(w, r, "/compare/tool", "error", h.safeMessage(err, lang))
			return
		}
	}

	h.redirectWithNotice(w, r, "/compare/tool", "success", i18n.T(lang, "compare.mapping.match_confirmed_success"))
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
