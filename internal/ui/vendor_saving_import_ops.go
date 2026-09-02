package ui

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// VendorSavingProductsImportMapSubmit processes column mapping and performs matching for vendor.
func (h *UIHandler) VendorSavingProductsImportMapSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/saving-products/import", http.StatusSeeOther)
		return
	}

	sessionID := chi.URLParam(r, "id")
	session, ok := globalSavingImportSessionStore.GetSession(sessionID, actor.OrganizationID)
	if !ok {
		h.redirectWithNotice(w, r, "/vendor/saving-products/import", "error", i18n.T(langOf(r), "saving.import.session_not_found"))
		return
	}

	if err := r.ParseForm(); err != nil {
		h.redirectWithNotice(w, r, fmt.Sprintf("/vendor/saving-products/import/%s", sessionID), "error", i18n.T(langOf(r), "common.invalid_form_data"))
		return
	}

	colName := strings.TrimSpace(r.FormValue("col_name"))
	colSKU := strings.TrimSpace(r.FormValue("col_sku"))
	colQty := strings.TrimSpace(r.FormValue("col_qty"))
	colPrice := strings.TrimSpace(r.FormValue("col_price"))
	matchChoice := ParseMatchChoice(r)
	useAI := ParseUseAI(r)

	nCol, sCol, qCol, pCol := -1, -1, -1, -1
	for idx, hName := range session.Headers {
		if hName == colName {
			nCol = idx
		}
		if hName == colSKU {
			sCol = idx
		}
		if hName == colQty {
			qCol = idx
		}
		if hName == colPrice {
			pCol = idx
		}
	}

	var matchEngine *SavingProductMatchEngine
	if h.catSvc != nil {
		if catalogSources, err := h.catSvc.ListMatchProducts(ctx); err == nil && len(catalogSources) > 0 {
			matchEngine = NewSavingProductMatchEngine(catalogSources)
		}
	}

	stagedItems := make([]*StagedSavingItem, 0, len(session.RawDataRows))
	matchedCount := 0
	unlinkedCount := 0
	var totalQty float64
	var totalValMinor int64

	for _, row := range session.RawDataRows {
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

		if isAllDigitsOrCode(name) && len(name) >= 4 && isDescriptiveArabicText(sku) {
			name, sku = sku, name
		}
		if name == "" && sku != "" {
			name = sku
		}
		if name == "" {
			continue
		}

		var qty float64 = 1.0
		if qCol >= 0 && qCol < len(row) {
			if parsedQ, ok := ParseFlexibleQuantity(row[qCol]); ok && parsedQ > 0 {
				qty = parsedQ
			}
		}

		var price money.Amount
		if pCol >= 0 && pCol < len(row) {
			price, _ = ParseFlexibleMoney(row[pCol])
		}

		var productID *int64
		matchType := "unlinked"
		confidence := 0.0
		masterName := ""
		masterSKU := ""

		if matchEngine != nil {
			res := matchEngine.MatchUnified(matchChoice, nil, sku, name)
			if res.ProductID != nil {
				productID = res.ProductID
				matchType = res.MatchType
				confidence = res.Confidence
				masterName, masterSKU = matchEngine.Describe(*productID)
			}
		}

		if productID != nil {
			matchedCount++
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
	}

	// The residue goes to the shared AI stage — the same prompt, the same
	// ceilings and the same decision cache the vendor import and the smart order
	// use. A row it cannot verify against the catalogue's own record keeps the
	// deterministic outcome, and the whole stage is a no-op when the Gateway is
	// unwired.
	if n := h.enhanceSaving(ctx, useAI, matchEngine, stagedItems); n > 0 {
		matchedCount += n
		unlinkedCount -= n
	}

	globalSavingImportSessionStore.CompleteProcessing(
		session.ID,
		stagedItems,
		matchedCount,
		unlinkedCount,
		totalQty,
		money.FromMinor(totalValMinor),
	)
	session.Phase = SavingPhaseReview

	http.Redirect(w, r, fmt.Sprintf("/vendor/saving-products/import/%s", sessionID), http.StatusSeeOther)
}

// VendorSavingProductsImportItemUpdateSubmit updates staged item details for vendor.
func (h *UIHandler) VendorSavingProductsImportItemUpdateSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	sessionID := chi.URLParam(r, "id")
	itemIndex, _ := strconv.Atoi(chi.URLParam(r, "itemIndex"))

	if err := r.ParseForm(); err == nil {
		name := strings.TrimSpace(r.FormValue("name"))
		var pricePtr *money.Amount
		if pStr := strings.TrimSpace(r.FormValue("price")); pStr != "" {
			if amt, err := money.Parse(pStr); err == nil {
				pricePtr = &amt
			}
		}
		var qtyPtr *float64
		if qStr := strings.TrimSpace(r.FormValue("quantity")); qStr != "" {
			if q, err := strconv.ParseFloat(qStr, 64); err == nil && q > 0 {
				qtyPtr = &q
			}
		}
		_ = globalSavingImportSessionStore.UpdateStagedItem(sessionID, actor.OrganizationID, itemIndex, name, pricePtr, qtyPtr, nil)
	}

	redirectURI := fmt.Sprintf("/vendor/saving-products/import/%s?%s", sessionID, r.URL.RawQuery)
	http.Redirect(w, r, redirectURI, http.StatusSeeOther)
}

// VendorSavingProductsImportItemMatchSubmit handles manual link or unlink for vendor.
func (h *UIHandler) VendorSavingProductsImportItemMatchSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	sessionID := chi.URLParam(r, "id")
	itemIndex, _ := strconv.Atoi(chi.URLParam(r, "itemIndex"))

	if err := r.ParseForm(); err == nil {
		productID, _ := strconv.ParseInt(r.FormValue("product_id"), 10, 64)
		masterName := strings.TrimSpace(r.FormValue("master_name"))
		masterSKU := strings.TrimSpace(r.FormValue("master_sku"))
		_ = globalSavingImportSessionStore.AssignStagedItemMatch(sessionID, actor.OrganizationID, itemIndex, productID, masterName, masterSKU)
	}

	redirectURI := fmt.Sprintf("/vendor/saving-products/import/%s?%s", sessionID, r.URL.RawQuery)
	http.Redirect(w, r, redirectURI, http.StatusSeeOther)
}

// VendorSavingProductsImportItemToggleSubmit flips included checkbox for vendor.
func (h *UIHandler) VendorSavingProductsImportItemToggleSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	sessionID := chi.URLParam(r, "id")
	itemIndex, _ := strconv.Atoi(chi.URLParam(r, "itemIndex"))

	_, _ = globalSavingImportSessionStore.ToggleStagedItem(sessionID, actor.OrganizationID, itemIndex)

	redirectURI := fmt.Sprintf("/vendor/saving-products/import/%s?%s", sessionID, r.URL.RawQuery)
	http.Redirect(w, r, redirectURI, http.StatusSeeOther)
}

// VendorSavingProductsImportCommitSubmit commits staged items into catalog.saving_products for vendor.
func (h *UIHandler) VendorSavingProductsImportCommitSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	sessionID := chi.URLParam(r, "id")
	added, updated, err := globalSavingImportSessionStore.CommitSession(ctx, sessionID, actor.OrganizationID, actor.UserID, h.catSvc)
	if err != nil {
		h.redirectWithNotice(w, r, fmt.Sprintf("/vendor/saving-products/import/%s", sessionID), "error", i18n.T(langOf(r), "saving.import.save_failed_prefix")+h.safeMessage(err, langOf(r)))
		return
	}

	session, ok := globalSavingImportSessionStore.GetSession(sessionID, actor.OrganizationID)
	if ok && session != nil {
		session.Phase = SavingPhaseCompleted
		session.InsertedCount = added
		session.UpdatedCount = updated
	}

	http.Redirect(w, r, fmt.Sprintf("/vendor/saving-products/import/%s", sessionID), http.StatusSeeOther)
}

// VendorSavingProductsImportCancelSubmit cancels and cleans up a session for vendor.
func (h *UIHandler) VendorSavingProductsImportCancelSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	sessionID := chi.URLParam(r, "id")
	globalSavingImportSessionStore.CancelSession(sessionID, actor.OrganizationID)

	h.redirectWithNotice(w, r, "/vendor/saving-products/import", "info", i18n.T(langOf(r), "saving.import.cancelled_success"))
}
