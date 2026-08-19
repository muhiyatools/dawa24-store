package ui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/ingest"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// VendorIngestPage renders the primary catalog upload and sync dashboard.
func (h *UIHandler) VendorIngestPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/ingest", http.StatusSeeOther)
		return
	}

	var sessions []*ingest.ImportSession
	if h.ingSvc != nil && actor.OrganizationID > 0 {
		sList, err := h.ingSvc.ListSessions(ctx, actor.OrganizationID, 20, 0)
		if err == nil {
			sessions = sList
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.VendorIngestPage(sessions, nil, nil, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render vendor ingest page", "error", err)
	}
}

// VendorIngestSessionPage loads an ongoing or completed session to resume at the exact step.
func (h *UIHandler) VendorIngestSessionPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/ingest", http.StatusSeeOther)
		return
	}

	idStr := chi.URLParam(r, "sessionID")
	sessionID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || sessionID <= 0 {
		http.Redirect(w, r, "/vendor/ingest", http.StatusSeeOther)
		return
	}

	var session *ingest.ImportSession
	var rows []*ingest.ImportRow
	if h.ingSvc != nil {
		sItem, err := h.ingSvc.GetSessionProgress(ctx, sessionID)
		if err == nil && sItem != nil {
			session = sItem
			rList, _ := h.ingSvc.ListImportRows(ctx, sessionID, "", 50, 0)
			rows = rList
		}
	}

	if session == nil {
		h.redirectWithNotice(w, r, "/vendor/ingest", "error", "جلسة الاستيراد غير موجودة.")
		return
	}

	var sessions []*ingest.ImportSession
	if h.ingSvc != nil && actor.OrganizationID > 0 {
		sessions, _ = h.ingSvc.ListSessions(ctx, actor.OrganizationID, 20, 0)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.VendorIngestPage(sessions, session, rows, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render vendor ingest session page", "error", err)
	}
}

// VendorIngestUploadSubmit handles the initial file upload and initiates streaming ingestion.
func (h *UIHandler) VendorIngestUploadSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/ingest", http.StatusSeeOther)
		return
	}

	// Limit upload size to 50MB
	r.Body = http.MaxBytesReader(w, r.Body, 50<<20)
	if err := r.ParseMultipartForm(50 << 20); err != nil {
		h.redirectWithNotice(w, r, "/vendor/ingest", "error", "حجم الملف كبير جداً (الحد الأقصى 50 ميجابايت).")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		h.redirectWithNotice(w, r, "/vendor/ingest", "error", "يرجى اختيار ملف صالح للاستيراد (.xlsx أو .csv).")
		return
	}
	defer file.Close()

	if h.ingSvc != nil {
		// Register upload
		fileUpload := &ingest.FileUpload{
			OrganizationID: actor.OrganizationID,
			UserID:         actor.UserID,
			Filename:       header.Filename,
			StorageKey:     fmt.Sprintf("orgs/%d/uploads/%s", actor.OrganizationID, header.Filename),
			FileSizeBytes:  header.Size,
			MimeType:       "application/octet-stream",
		}
		createdUpload, err := h.ingSvc.RegisterUpload(ctx, fileUpload)
		if err != nil {
			h.redirectWithNotice(w, r, "/vendor/ingest", "error", h.safeMessage(err, langOf(r)))
			return
		}

		// Start session
		session, err := h.ingSvc.StartSession(ctx, createdUpload.ID, []string{"product_name", "price", "quantity", "barcode", "sku"}, 0.85)
		if err != nil {
			h.redirectWithNotice(w, r, "/vendor/ingest", "error", h.safeMessage(err, langOf(r)))
			return
		}

		// Stream spreadsheet rows in 500-row chunks
		_, err = h.ingSvc.ProcessSpreadsheetStream(ctx, session.ID, file, header.Filename, "product_name", nil)
		if err != nil {
			h.log.WarnContext(ctx, "stream rows warning", "session_id", session.ID, "error", err)
		}

		// If called via AJAX/JSON
		if r.Header.Get("Accept") == "application/json" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"session_id": session.ID,
				"status":     session.Status,
				"redirect":   fmt.Sprintf("/vendor/ingest/%d", session.ID),
			})
			return
		}

		http.Redirect(w, r, fmt.Sprintf("/vendor/ingest/%d", session.ID), http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/vendor/ingest", http.StatusSeeOther)
}

// VendorIngestMappingSubmit updates column mapping for the active session.
func (h *UIHandler) VendorIngestMappingSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	idStr := chi.URLParam(r, "id")
	sessionID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || sessionID <= 0 {
		http.Redirect(w, r, "/vendor/ingest", http.StatusSeeOther)
		return
	}

	_ = r.ParseForm()
	mapping := make(map[string]string)
	for k, v := range r.PostForm {
		if len(v) > 0 && v[0] != "" {
			mapping[k] = v[0]
		}
	}

	if h.ingSvc != nil {
		_ = h.ingSvc.UpdateColumnMapping(ctx, sessionID, mapping)
	}

	http.Redirect(w, r, fmt.Sprintf("/vendor/ingest/%d", sessionID), http.StatusSeeOther)
}

// VendorIngestRowsPartial returns rows HTML for review.
func (h *UIHandler) VendorIngestRowsPartial(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	idStr := chi.URLParam(r, "id")
	sessionID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || sessionID <= 0 {
		http.Error(w, "invalid session", http.StatusBadRequest)
		return
	}

	var rows []*ingest.ImportRow
	if h.ingSvc != nil {
		rList, _ := h.ingSvc.ListImportRows(ctx, sessionID, "", 50, 0)
		rows = rList
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"rows": rows, "count": len(rows)})
}

// VendorIngestRowUpdateSubmit overrides or fixes a staged row match.
func (h *UIHandler) VendorIngestRowUpdateSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	idStr := chi.URLParam(r, "id")
	sessionID, _ := strconv.ParseInt(idStr, 10, 64)
	rowIDStr := chi.URLParam(r, "rid")
	rowID, _ := strconv.ParseInt(rowIDStr, 10, 64)

	productIDStr := r.FormValue("product_id")
	productID, _ := strconv.ParseInt(productIDStr, 10, 64)

	if h.ingSvc != nil && rowID > 0 && productID > 0 {
		_ = h.ingSvc.OverrideRowMatch(ctx, rowID, productID)
	}

	http.Redirect(w, r, fmt.Sprintf("/vendor/ingest/%d", sessionID), http.StatusSeeOther)
}

// VendorIngestCommitSubmit commits the session and activates inventory in catalog.
func (h *UIHandler) VendorIngestCommitSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	idStr := chi.URLParam(r, "id")
	sessionID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || sessionID <= 0 {
		http.Redirect(w, r, "/vendor/ingest", http.StatusSeeOther)
		return
	}

	if h.ingSvc != nil {
		if err := h.ingSvc.CommitSession(ctx, sessionID); err != nil {
			h.redirectWithNotice(w, r, fmt.Sprintf("/vendor/ingest/%d", sessionID), "error", h.safeMessage(err, langOf(r)))
			return
		}
	}

	if r.Header.Get("Accept") == "application/json" {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "completed"})
		return
	}

	http.Redirect(w, r, fmt.Sprintf("/vendor/ingest/%d", sessionID), http.StatusSeeOther)
}

// VendorIngestCancelSubmit cancels the session.
func (h *UIHandler) VendorIngestCancelSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	idStr := chi.URLParam(r, "id")
	sessionID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || sessionID <= 0 {
		http.Redirect(w, r, "/vendor/ingest", http.StatusSeeOther)
		return
	}

	if h.ingSvc != nil {
		_ = h.ingSvc.CancelSession(ctx, sessionID)
	}

	http.Redirect(w, r, "/vendor/ingest", http.StatusSeeOther)
}
