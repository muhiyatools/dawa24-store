package ui

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/sheet"
)

// VendorSavingProductsImportStartJSON initiates asynchronous processing of an uploaded file for vendor.
func (h *UIHandler) VendorSavingProductsImportStartJSON(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": i18n.T(langOf(r), "common.unauthorized")})
		return
	}

	if err := r.ParseMultipartForm(32 << 20); err != nil {
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": i18n.T(langOf(r), "customer.saving.import.file_too_large")})
		return
	}

	file, fileHeader, err := r.FormFile("file")
	if err != nil {
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": i18n.T(langOf(r), "customer.saving.import.select_valid_file")})
		return
	}
	defer file.Close()

	fileBytes, err := io.ReadAll(file)
	if err != nil || len(fileBytes) == 0 {
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": i18n.T(langOf(r), "customer.saving.import.read_content_error")})
		return
	}

	rawRows, err := sheet.ReadRows(fileBytes, fileHeader.Filename)
	if err != nil || len(rawRows) <= 1 {
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": i18n.T(langOf(r), "customer.saving.import.file_empty_no_rows")})
		return
	}

	headers := rawRows[0]
	var sampleRows [][]string
	if len(rawRows) > 1 {
		limit := 4
		if len(rawRows)-1 < limit {
			limit = len(rawRows) - 1
		}
		sampleRows = rawRows[1 : 1+limit]
	}

	nameCol, skuCol, qtyCol, priceCol, productIDCol := detectSavingProductColumns(
		headers,
		sampleRows,
		r.FormValue("col_name"),
		r.FormValue("col_sku"),
		r.FormValue("col_qty"),
		r.FormValue("col_price"),
		r.FormValue("col_product_id"),
	)

	// The unified matching choice: name first, AI second, identifier tiers only
	// where the user switched them on. Read before the goroutine and passed in,
	// not captured, so the background worker runs under the choice made on the
	// request that started it rather than whatever the session holds later.
	matchChoice := ParseMatchChoice(r)
	useAI := ParseUseAI(r)

	sessionID, totalRows, err := h.startSavingImportRun(
		ctx,
		actor,
		fileHeader.Filename,
		rawRows,
		headers,
		sampleRows,
		nameCol, skuCol, qtyCol, priceCol, productIDCol,
		matchChoice,
		useAI,
		langOf(r),
		"vendor",
	)
	if err != nil {
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": err.Error()})
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]any{
		"success":    true,
		"session_id": sessionID,
		"total_rows": totalRows,
	})
}

// VendorSavingProductsImportProgressJSON returns the live state and staged items for review for vendor.
func (h *UIHandler) VendorSavingProductsImportProgressJSON(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": i18n.T(langOf(r), "common.unauthorized")})
		return
	}

	sessionID := chi.URLParam(r, "id")
	session, ok := globalSavingImportSessionStore.GetSession(sessionID, actor.OrganizationID)
	if !ok {
		// Durable DB fallback for sessions surviving server restarts.
		if h.importRunRepo != nil {
			run, err := h.importRunRepo.GetRunByPublicID(ctx, sessionID, actor.OrganizationID)
			if err == nil && run != nil {
				h.respondWithRunProgress(w, r, run)
				return
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": i18n.T(langOf(r), "customer.saving.import.session_not_found")})
		return
	}

	session.Success = true
	_ = json.NewEncoder(w).Encode(session)
}

// VendorSavingProductsImportCommitJSON commits the staged session into catalog.saving_products.
func (h *UIHandler) VendorSavingProductsImportCommitJSON(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": i18n.T(langOf(r), "common.unauthorized")})
		return
	}

	sessionID := chi.URLParam(r, "id")
	added, updated, err := h.commitSavingImportRun(ctx, sessionID, actor.OrganizationID, actor.UserID, h.catSvc)
	if err != nil {
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": fmt.Sprintf(i18n.T(langOf(r), "customer.saving.import.commit_error"), h.safeMessage(err, langOf(r)))})
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]any{
		"success": true,
		"added":   added,
		"updated": updated,
		"message": fmt.Sprintf(i18n.T(langOf(r), "customer.saving.import.commit_success"), added+updated, added, updated),
	})
}

// VendorSavingProductsImportCancelJSON discards and clears the staged session.
func (h *UIHandler) VendorSavingProductsImportCancelJSON(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": i18n.T(langOf(r), "common.unauthorized")})
		return
	}

	sessionID := chi.URLParam(r, "id")
	cancelled := globalSavingImportSessionStore.CancelSession(sessionID, actor.OrganizationID)
	_ = json.NewEncoder(w).Encode(map[string]any{"success": cancelled})
}
