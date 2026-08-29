package ui

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// CustomerSavingProductsImportMapSubmit processes column mapping and performs matching.
func (h *UIHandler) CustomerSavingProductsImportMapSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/customer/saving-products/import", http.StatusSeeOther)
		return
	}

	sessionID := chi.URLParam(r, "id")
	session, ok := globalSavingImportSessionStore.GetSession(sessionID, actor.OrganizationID)
	if !ok {
		h.redirectWithNotice(w, r, "/customer/saving-products/import", "error", "جلسة الاستيراد غير موجودة أو انتهت صلاحيتها.")
		return
	}

	if err := r.ParseForm(); err != nil {
		h.redirectWithNotice(w, r, fmt.Sprintf("/customer/saving-products/import/%s", sessionID), "error", "بيانات غير صالحة.")
		return
	}

	colName := strings.TrimSpace(r.FormValue("col_name"))
	colSKU := strings.TrimSpace(r.FormValue("col_sku"))
	colQty := strings.TrimSpace(r.FormValue("col_qty"))
	colPrice := strings.TrimSpace(r.FormValue("col_price"))
	stratStr := strings.TrimSpace(r.FormValue("match_strategy"))
	if stratStr == "" {
		stratStr = string(StrategySmartAuto)
	}
	strat := MatchStrategy(stratStr)

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
	if strat != "none" && h.catSvc != nil {
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

		if matchEngine != nil && strat != "none" {
			res := matchEngine.Match(strat, nil, sku, name)
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
	if n := enhanceSavingItems(ctx, h.matchEnhancer, matchEngine, stagedItems, h.log); n > 0 {
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

	http.Redirect(w, r, fmt.Sprintf("/customer/saving-products/import/%s", sessionID), http.StatusSeeOther)
}

// CustomerSavingProductsImportItemUpdateSubmit updates staged item details directly from table row.
func (h *UIHandler) CustomerSavingProductsImportItemUpdateSubmit(w http.ResponseWriter, r *http.Request) {
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

	redirectURI := fmt.Sprintf("/customer/saving-products/import/%s?%s", sessionID, r.URL.RawQuery)
	http.Redirect(w, r, redirectURI, http.StatusSeeOther)
}

// CustomerSavingProductsImportItemMatchSubmit handles manual link or unlink of catalog product.
func (h *UIHandler) CustomerSavingProductsImportItemMatchSubmit(w http.ResponseWriter, r *http.Request) {
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

	redirectURI := fmt.Sprintf("/customer/saving-products/import/%s?%s", sessionID, r.URL.RawQuery)
	http.Redirect(w, r, redirectURI, http.StatusSeeOther)
}

// CustomerSavingProductsImportItemToggleSubmit flips included checkbox.
func (h *UIHandler) CustomerSavingProductsImportItemToggleSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	sessionID := chi.URLParam(r, "id")
	itemIndex, _ := strconv.Atoi(chi.URLParam(r, "itemIndex"))

	_, _ = globalSavingImportSessionStore.ToggleStagedItem(sessionID, actor.OrganizationID, itemIndex)

	redirectURI := fmt.Sprintf("/customer/saving-products/import/%s?%s", sessionID, r.URL.RawQuery)
	http.Redirect(w, r, redirectURI, http.StatusSeeOther)
}

// CustomerSavingProductsImportCommitSubmit commits staged items into catalog.saving_products.
func (h *UIHandler) CustomerSavingProductsImportCommitSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	sessionID := chi.URLParam(r, "id")
	added, updated, err := globalSavingImportSessionStore.CommitSession(ctx, sessionID, actor.OrganizationID, actor.UserID, h.catSvc)
	if err != nil {
		h.redirectWithNotice(w, r, fmt.Sprintf("/customer/saving-products/import/%s", sessionID), "error", "فشل الحفظ: "+h.safeMessage(err, langOf(r)))
		return
	}

	session, ok := globalSavingImportSessionStore.GetSession(sessionID, actor.OrganizationID)
	if ok && session != nil {
		session.Phase = SavingPhaseCompleted
		session.InsertedCount = added
		session.UpdatedCount = updated
	}

	http.Redirect(w, r, fmt.Sprintf("/customer/saving-products/import/%s", sessionID), http.StatusSeeOther)
}

// CustomerSavingProductsImportCancelSubmit cancels and cleans up a session.
func (h *UIHandler) CustomerSavingProductsImportCancelSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	sessionID := chi.URLParam(r, "id")
	globalSavingImportSessionStore.CancelSession(sessionID, actor.OrganizationID)

	h.redirectWithNotice(w, r, "/customer/saving-products/import", "info", "تم إلغاء جلسة الاستيراد بنجاح.")
}
