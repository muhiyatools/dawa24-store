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

// handleSavingProductsImportItemUpdateSubmit updates staged item details for customer or vendor.
func (h *UIHandler) handleSavingProductsImportItemUpdateSubmit(w http.ResponseWriter, r *http.Request, audience string) {
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

	redirectURI := fmt.Sprintf("/%s/saving-products/import/%s?%s", audience, sessionID, r.URL.RawQuery)
	http.Redirect(w, r, redirectURI, http.StatusSeeOther)
}

// handleSavingProductsImportItemMatchSubmit handles manual link or unlink for customer or vendor.
func (h *UIHandler) handleSavingProductsImportItemMatchSubmit(w http.ResponseWriter, r *http.Request, audience string) {
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

	redirectURI := fmt.Sprintf("/%s/saving-products/import/%s?%s", audience, sessionID, r.URL.RawQuery)
	h.redirectWithNotice(w, r, redirectURI, "success", noticeMsg)
}

// handleSavingProductsImportItemToggleSubmit flips included checkbox for customer or vendor.
func (h *UIHandler) handleSavingProductsImportItemToggleSubmit(w http.ResponseWriter, r *http.Request, audience string) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	sessionID := chi.URLParam(r, "id")
	itemIndex, _ := strconv.Atoi(chi.URLParam(r, "itemIndex"))

	_, _ = globalSavingImportSessionStore.ToggleStagedItem(sessionID, actor.OrganizationID, itemIndex)

	redirectURI := fmt.Sprintf("/%s/saving-products/import/%s?%s", audience, sessionID, r.URL.RawQuery)
	http.Redirect(w, r, redirectURI, http.StatusSeeOther)
}

// handleSavingProductsImportCommitSubmit commits staged items into catalog.saving_products for customer or vendor.
func (h *UIHandler) handleSavingProductsImportCommitSubmit(w http.ResponseWriter, r *http.Request, audience string) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	sessionID := chi.URLParam(r, "id")
	added, updated, err := globalSavingImportSessionStore.CommitSession(ctx, sessionID, actor.OrganizationID, actor.UserID, h.catSvc)
	if err != nil {
		h.redirectWithNotice(w, r, fmt.Sprintf("/%s/saving-products/import/%s", audience, sessionID), "error", i18n.T(langOf(r), "saving.import.save_failed_prefix")+h.safeMessage(err, langOf(r)))
		return
	}

	session, ok := globalSavingImportSessionStore.GetSession(sessionID, actor.OrganizationID)
	if ok && session != nil {
		session.Phase = SavingPhaseCompleted
		session.InsertedCount = added
		session.UpdatedCount = updated
	}

	http.Redirect(w, r, fmt.Sprintf("/%s/saving-products/import/%s", audience, sessionID), http.StatusSeeOther)
}

// handleSavingProductsImportCancelSubmit cancels and cleans up a session for customer or vendor.
func (h *UIHandler) handleSavingProductsImportCancelSubmit(w http.ResponseWriter, r *http.Request, audience string) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	sessionID := chi.URLParam(r, "id")
	globalSavingImportSessionStore.CancelSession(sessionID, actor.OrganizationID)

	h.redirectWithNotice(w, r, fmt.Sprintf("/%s/saving-products/import", audience), "info", i18n.T(langOf(r), "saving.import.cancelled_success"))
}