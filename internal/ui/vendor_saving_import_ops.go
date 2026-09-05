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

	// Detached. This used to run inline, inside this POST.
	//
	// It loaded the ENTIRE catalogue — up to a hundred and fifty thousand
	// products — built a match index over it, scored every row of the file
	// against that index, and then called the AI stage over whatever was left.
	// On a real price list that is minutes of work, and it happened with the
	// browser holding the connection open and nothing on screen: no bar, no
	// percentage, no way to tell a slow run from a dead one. Worse, this route
	// is not in httpx.longRunningPrefixes, so the request deadline cut it off
	// before it could finish and the user got an error page after the wait.
	//
	// startSavingImportRun is the path the JSON flow already used: a durable
	// platform.import_runs row, an in-memory session mirrored from it, and a
	// goroutine that outlives the request and reports progress as it goes. The
	// wizard redirects onto the run's id and renders savingProcessingStage,
	// which streams that progress through the same bar every other import uses.
	publicID, _, startErr := h.startSavingImportRun(
		ctx, actor, session.Filename,
		append([][]string{session.Headers}, session.RawDataRows...),
		session.Headers, session.SampleRows,
		nCol, sCol, qCol, pCol, -1,
		matchChoice, useAI, langOf(r), "vendor",
	)
	if startErr != nil {
		h.redirectWithNotice(w, r, fmt.Sprintf("/vendor/saving-products/import/%s", sessionID), "error", h.safeMessage(startErr, langOf(r)))
		return
	}

	http.Redirect(w, r, fmt.Sprintf("/vendor/saving-products/import/%s", publicID), http.StatusSeeOther)
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

	noticeMsg := "تم إلغاء ربط الصنف (أصبح غير مرتبط)."
	if err := r.ParseForm(); err == nil {
		productID, _ := strconv.ParseInt(r.FormValue("product_id"), 10, 64)
		masterName := strings.TrimSpace(r.FormValue("master_name"))
		masterSKU := strings.TrimSpace(r.FormValue("master_sku"))
		_ = globalSavingImportSessionStore.AssignStagedItemMatch(sessionID, actor.OrganizationID, itemIndex, productID, masterName, masterSKU)
		if productID > 0 {
			noticeMsg = fmt.Sprintf("تم ربط الصنف «%s» بالكتالوج المركزي.", masterName)
		}
	}

	redirectURI := fmt.Sprintf("/vendor/saving-products/import/%s?%s", sessionID, r.URL.RawQuery)
	h.redirectWithNotice(w, r, redirectURI, "success", noticeMsg)
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
