package ui

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/shared/pagination"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// AdminOrgImportSavingSessionPage renders the session mapping/review page for the administrator.
func (h *UIHandler) AdminOrgImportSavingSessionPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	sessionID := chi.URLParam(r, "id")
	session, ok := globalSavingImportSessionStore.GetSessionForAdmin(sessionID)
	if !ok {
		h.redirectWithNotice(w, r, "/admin/organizations/import", "error", i18n.T(lang, "customer.saving.import.session_not_found"))
		return
	}

	matchFilter := strings.TrimSpace(r.URL.Query().Get("match"))
	sortBy := strings.TrimSpace(r.URL.Query().Get("sort"))
	sortOrder := strings.TrimSpace(r.URL.Query().Get("order"))
	search := strings.TrimSpace(r.URL.Query().Get("q"))

	page := pagination.PageNumber(r)
	limit := pagination.RowsPerPage(r)

	filter := SavingRowFilter{
		Search:      search,
		MatchFilter: matchFilter,
		SortBy:      sortBy,
		SortOrder:   sortOrder,
		Page:        page,
		Limit:       limit,
	}

	rows, total := globalSavingImportSessionStore.FilterItems(session, filter)

	targetOrgName := ""
	targetOrgType := ""
	if h.orgSvc != nil && session.OrgID > 0 {
		if targetOrg, err := h.orgSvc.GetOrganization(database.AsSystem(ctx), session.OrgID); err == nil && targetOrg != nil {
			if targetOrg.TradeName != nil && targetOrg.TradeName["ar"] != "" {
				targetOrgName = targetOrg.TradeName["ar"]
			} else {
				targetOrgName = targetOrg.LegalName
			}
			targetOrgType = string(targetOrg.Type)
		}
	}

	view := pages.SavingImportView{
		AIAvailable:         h.matchEnhancer != nil,
		AIUnavailableReason: savingAIUnavailableReason(h.matchEnhancer, lang),
		Audience:            "admin",
		TargetOrgName:       targetOrgName,
		TargetOrgType:       targetOrgType,
		BaseURL:             "/admin/organizations/import",
		ImportURL:           "/admin/organizations/import/saving",
		Session:             session,
		Filter:              filter,
		Rows:                rows,
		RowTotal:            total,
	}

	h.renderPage(ctx, w, "render admin org saving import session page", pages.SavingImportPage(view, lang, dir))
}

// AdminOrgImportSavingSessionMapSubmit processes column mapping for the admin session.
func (h *UIHandler) AdminOrgImportSavingSessionMapSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)

	sessionID := chi.URLParam(r, "id")
	session, ok := globalSavingImportSessionStore.GetSessionForAdmin(sessionID)
	if !ok {
		h.redirectWithNotice(w, r, "/admin/organizations/import", "error", i18n.T(lang, "saving.import.session_not_found"))
		return
	}

	if err := r.ParseForm(); err != nil {
		h.redirectWithNotice(w, r, fmt.Sprintf("/admin/organizations/import/saving/%s", sessionID), "error", i18n.T(lang, "validation.invalid_data"))
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

	if nCol == -1 {
		h.redirectWithNotice(w, r, fmt.Sprintf("/admin/organizations/import/saving/%s", sessionID), "error", i18n.T(lang, "customer.saving.import.missing_name_col"))
		return
	}

	var matchEngine *SavingProductMatchEngine
	if h.catSvc != nil {
		if catalogSources, err := h.catSvc.ListMatchProducts(database.AsSystem(ctx)); err == nil && len(catalogSources) > 0 {
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

	if n := h.enhanceSaving(database.AsSystem(ctx), useAI, matchEngine, stagedItems); n > 0 {
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

	http.Redirect(w, r, fmt.Sprintf("/admin/organizations/import/saving/%s", sessionID), http.StatusSeeOther)
}

// AdminOrgImportSavingSessionCommitSubmit saves staged saving products to the target organization.
func (h *UIHandler) AdminOrgImportSavingSessionCommitSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, _ := authctx.From(ctx)

	sessionID := chi.URLParam(r, "id")
	session, ok := globalSavingImportSessionStore.GetSessionForAdmin(sessionID)
	if !ok {
		h.redirectWithNotice(w, r, "/admin/organizations/import", "error", i18n.T(lang, "saving.import.session_not_found"))
		return
	}

	added, updated, err := globalSavingImportSessionStore.CommitSession(database.AsSystem(ctx), sessionID, session.OrgID, actor.UserID, h.catSvc)
	if err != nil {
		h.log.ErrorContext(ctx, "failed to commit saving products session", "error", err, "session_id", sessionID, "target_org_id", session.OrgID)
		h.redirectWithNotice(w, r, fmt.Sprintf("/admin/organizations/import/saving/%s", sessionID), "error", h.safeMessage(err, lang))
		return
	}

	h.log.InfoContext(ctx, "admin committed saving products for organization", "actor_id", actor.UserID, "target_org_id", session.OrgID, "added", added, "updated", updated)
	successMsg := fmt.Sprintf(i18n.T(lang, "customer.saving.import.commit_success"), added+updated, added, updated)
	h.redirectWithNotice(w, r, "/admin/organizations/import", "success", successMsg)
}

// AdminOrgImportSavingSessionCancelSubmit cancels an import session.
func (h *UIHandler) AdminOrgImportSavingSessionCancelSubmit(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "id")
	if session, ok := globalSavingImportSessionStore.GetSessionForAdmin(sessionID); ok {
		globalSavingImportSessionStore.CancelSession(sessionID, session.OrgID)
	}
	h.redirectWithNotice(w, r, "/admin/organizations/import", "info", i18n.T(langOf(r), "saving.import.cancelled_success"))
}