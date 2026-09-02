package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
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

	session := globalSavingImportSessionStore.NewSession(actor.OrganizationID, actor.UserID, fileHeader.Filename, len(rawRows)-1)

	// Launch async background processing
	go func(sessID string, orgID, userID int64, dataRows [][]string, nCol, sCol, qCol, pCol, pidCol int, choice MatchChoice, aiOn bool, lang string) {
		bgCtx := context.Background()

		globalSavingImportSessionStore.UpdateProgress(sessID, 15, i18n.T(lang, "customer.saving.import.progress_loading_catalog"), 0)

		var matchEngine *SavingProductMatchEngine
		if h.catSvc != nil {
			if catalogSources, err := h.catSvc.ListMatchProducts(bgCtx); err == nil && len(catalogSources) > 0 {
				matchEngine = NewSavingProductMatchEngine(catalogSources)
			}
		}

		total := len(dataRows)
		stagedItems := make([]*StagedSavingItem, 0, total)
		matchedCount := 0
		unlinkedCount := 0
		var totalQty float64
		var totalValMinor int64

		globalSavingImportSessionStore.UpdateProgress(sessID, 30, i18n.T(lang, "customer.saving.import.progress_matching"), 0)

		for i, row := range dataRows {
			if len(row) == 0 || IsAllEmptyRow(row) || IsSummaryOrTotalRow(row) {
				continue
			}

			var name string
			if nCol >= 0 && nCol < len(row) {
				name = strings.TrimSpace(row[nCol])
			}
			var sku string
			if sCol >= 0 && sCol < len(row) {
				sku = strings.TrimSpace(row[sCol])
			}

			// Content swap heuristic
			if isAllDigitsOrCode(name) && len(name) >= 4 && isDescriptiveArabicText(sku) {
				name, sku = sku, name
			}
			if name == "" && sku != "" {
				name = sku
			}
			if name == "" {
				continue
			}

			var qty float64
			if qCol >= 0 && qCol < len(row) {
				qty, _ = ParseFlexibleQuantity(row[qCol])
			}

			var price money.Amount
			if pCol >= 0 && pCol < len(row) {
				price, _ = ParseFlexibleMoney(row[pCol])
			}

			var productID *int64
			if pidCol >= 0 && pidCol < len(row) {
				if pid, err := strconv.ParseInt(strings.TrimSpace(row[pidCol]), 10, 64); err == nil && pid > 0 {
					productID = &pid
				}
			}

			matchType := "unlinked"
			confidence := 0.0
			if matchEngine != nil {
				res := matchEngine.MatchUnified(choice, productID, sku, name)
				if res.ProductID != nil {
					productID = res.ProductID
					matchType = res.MatchType
					confidence = res.Confidence
				}
			}

			masterName := ""
			masterSKU := ""
			if productID != nil {
				matchedCount++
				if matchEngine != nil {
					masterName, masterSKU = matchEngine.Describe(*productID)
				}
			} else {
				unlinkedCount++
			}

			rowTotalMinor := int64(qty * float64(price.Minor()))
			totalQty += qty
			totalValMinor += rowTotalMinor

			stagedItems = append(stagedItems, &StagedSavingItem{
				Index:             len(stagedItems) + 1,
				NameProduct:       name,
				SKU:               sku,
				Quantity:          qty,
				Price:             price,
				TotalValue:        money.FromMinor(rowTotalMinor),
				ProductID:         productID,
				MasterProductName: masterName,
				MasterProductSKU:  masterSKU,
				MatchType:         matchType,
				Confidence:        confidence,
				Included:          true,
			})

			if i%100 == 0 || i == total-1 {
				pct := 30 + int(float64(i+1)/float64(total)*65)
				if pct > 98 {
					pct = 98
				}
				globalSavingImportSessionStore.UpdateProgress(sessID, pct, fmt.Sprintf(i18n.T(lang, "customer.saving.import.progress_processed"), i+1, total), i+1)
			}
		}

		// The same AI stage the other three importers run. It was hooked into
		// the mapping-submit path first and this one — the background path the
		// upload screen actually drives — was missed, which meant the feature
		// was wired in the flow nobody uses and absent from the flow everybody
		// does.
		if n := h.enhanceSaving(bgCtx, aiOn, matchEngine, stagedItems); n > 0 {
			matchedCount += n
			unlinkedCount -= n
		}

		globalSavingImportSessionStore.CompleteProcessing(
			sessID,
			stagedItems,
			matchedCount,
			unlinkedCount,
			totalQty,
			money.FromMinor(totalValMinor),
		)
	}(session.ID, actor.OrganizationID, actor.UserID, rawRows[1:], nameCol, skuCol, qtyCol, priceCol, productIDCol, matchChoice, useAI, langOf(r))

	_ = json.NewEncoder(w).Encode(map[string]any{
		"success":    true,
		"session_id": session.ID,
		"total_rows": session.TotalRows,
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
	added, updated, err := globalSavingImportSessionStore.CommitSession(ctx, sessionID, actor.OrganizationID, actor.UserID, h.catSvc)
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
