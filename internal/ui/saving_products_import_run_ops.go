package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/importjobs"
	"github.com/muhiya/dawa24-store/internal/platform/importrun"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// startSavingImportRun coordinates asynchronous processing of saving products imports.
// It creates a durable platform.import_runs entry, mirrors it to the in-memory
// store for transitional UI compatibility, and starts background processing.
func (h *UIHandler) startSavingImportRun(
	ctx context.Context,
	actor authctx.Actor,
	filename string,
	rawRows [][]string,
	headers []string,
	sampleRows [][]string,
	nCol, sCol, qCol, pCol, pidCol int,
	choice MatchChoice,
	useAI bool,
	lang string,
	audience string,
) (string, int, error) {
	totalRows := len(rawRows) - 1
	if totalRows <= 0 {
		return "", 0, fmt.Errorf("%s", i18n.T(lang, "customer.saving.import.file_empty_no_rows"))
	}

	dataRows := rawRows[1:]

	payload := importjobs.SavingPayload{
		Headers:      headers,
		SampleRows:   sampleRows,
		RawDataRows:  dataRows,
		NameCol:      nCol,
		SKUCol:       sCol,
		QtyCol:       qCol,
		PriceCol:     pCol,
		ProductIDCol: pidCol,
		MatchChoice:  choice,
		UseAI:        useAI,
		Lang:         lang,
	}
	payloadBytes, _ := json.Marshal(payload)

	var runID int64
	var publicID string

	if h.importRunRepo != nil {
		run := &importrun.Run{
			OrganizationID: actor.OrganizationID,
			UserID:         actor.UserID,
			Kind:           importrun.KindSavingProducts,
			Audience:       audience,
			Filename:       filename,
			State:          importrun.StateProcessing,
			Phase:          i18n.T(lang, "customer.saving.import.progress_loading_catalog"),
			Percent:        5,
			TotalRows:      totalRows,
			ProcessedRows:  0,
			Payload:        payloadBytes,
		}
		if err := h.importRunRepo.CreateRun(ctx, run); err == nil {
			runID = run.ID
			publicID = run.PublicID
		}
	}

	if publicID != "" {
		globalSavingImportSessionStore.NewSessionWithID(publicID, actor.OrganizationID, actor.UserID, filename, totalRows)
	} else {
		memSession := globalSavingImportSessionStore.NewSession(actor.OrganizationID, actor.UserID, filename, totalRows)
		publicID = memSession.ID
	}

	// Launch async background processing.
	go func(runID int64, sessID string, orgID, userID int64, rows [][]string, nC, sC, qC, pC, pidC int, ch MatchChoice, aiOn bool, l string) {
		bgCtx := context.Background()

		phaseLoading := i18n.T(l, "customer.saving.import.progress_loading_catalog")
		globalSavingImportSessionStore.UpdateProgress(sessID, 15, phaseLoading, 0)
		if h.importRunRepo != nil && runID > 0 {
			_ = h.importRunRepo.UpdateProgress(bgCtx, runID, phaseLoading, 15, 0)
		}

		var matchEngine *SavingProductMatchEngine
		if h.catSvc != nil {
			if catalogSources, err := h.catSvc.ListMatchProducts(bgCtx); err == nil && len(catalogSources) > 0 {
				matchEngine = NewSavingProductMatchEngine(catalogSources)
			}
		}

		total := len(rows)
		stagedItems := make([]*StagedSavingItem, 0, total)
		var dbRows []importrun.Row
		matchedCount := 0
		unlinkedCount := 0
		var totalQty float64
		var totalValMinor int64

		phaseMatching := i18n.T(l, "customer.saving.import.progress_matching")
		globalSavingImportSessionStore.UpdateProgress(sessID, 30, phaseMatching, 0)
		if h.importRunRepo != nil && runID > 0 {
			_ = h.importRunRepo.UpdateProgress(bgCtx, runID, phaseMatching, 30, 0)
		}

		for i, row := range rows {
			if len(row) == 0 || IsAllEmptyRow(row) || IsSummaryOrTotalRow(row) {
				continue
			}

			var name string
			if nC >= 0 && nC < len(row) {
				name = strings.TrimSpace(row[nC])
			}
			var sku string
			if sC >= 0 && sC < len(row) {
				sku = strings.TrimSpace(row[sC])
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
			if qC >= 0 && qC < len(row) {
				if parsedQ, ok := ParseFlexibleQuantity(row[qC]); ok && parsedQ > 0 {
					qty = parsedQ
				}
			}

			var price money.Amount
			if pC >= 0 && pC < len(row) {
				price, _ = ParseFlexibleMoney(row[pC])
			}

			var productID *int64
			if pidC >= 0 && pidC < len(row) {
				if pid, err := strconv.ParseInt(strings.TrimSpace(row[pidC]), 10, 64); err == nil && pid > 0 {
					productID = &pid
				}
			}

			matchType := "unlinked"
			confidence := 0.0
			if matchEngine != nil {
				res := matchEngine.MatchUnified(ch, productID, sku, name)
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

			item := &StagedSavingItem{
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
			}
			stagedItems = append(stagedItems, item)

			if h.importRunRepo != nil && runID > 0 {
				dataBytes, _ := json.Marshal(item)
				dbRows = append(dbRows, importrun.Row{
					RunID:            runID,
					RowNumber:        len(stagedItems),
					Data:             dataBytes,
					Included:         true,
					MatchedProductID: productID,
				})
			}

			if i%100 == 0 || i == total-1 {
				pct := 30 + int(float64(i+1)/float64(total)*65)
				if pct > 98 {
					pct = 98
				}
				pMsg := fmt.Sprintf(i18n.T(l, "customer.saving.import.progress_processed"), i+1, total)
				globalSavingImportSessionStore.UpdateProgress(sessID, pct, pMsg, i+1)
				if h.importRunRepo != nil && runID > 0 {
					_ = h.importRunRepo.UpdateProgress(bgCtx, runID, pMsg, pct, i+1)
				}
			}
		}

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

		if h.importRunRepo != nil && runID > 0 {
			_ = h.importRunRepo.InsertRows(bgCtx, runID, dbRows)
			resCounters := map[string]any{
				"matched_rows":   matchedCount,
				"unlinked_rows":  unlinkedCount,
				"total_quantity": totalQty,
				"total_minor":    totalValMinor,
			}
			resBytes, _ := json.Marshal(resCounters)
			_ = h.importRunRepo.SetResult(bgCtx, runID, resBytes)
			_ = h.importRunRepo.UpdateProgress(bgCtx, runID, "اكتملت المعالجة", 100, total)
			_ = h.importRunRepo.TransitionState(bgCtx, runID, importrun.StateReady)
		}
	}(runID, publicID, actor.OrganizationID, actor.UserID, dataRows, nCol, sCol, qCol, pCol, pidCol, choice, useAI, lang)

	return publicID, totalRows, nil
}

// commitSavingImportRun commits staged saving items, attempting the in-memory
// session first and falling back to platform.import_run_rows if the session expired
// or the process restarted.
func (h *UIHandler) commitSavingImportRun(
	ctx context.Context,
	sessionID string,
	orgID, userID int64,
	catSvc *catalog.Service,
) (added int, updated int, err error) {
	// 1. Try in-memory session first.
	added, updated, err = globalSavingImportSessionStore.CommitSession(ctx, sessionID, orgID, userID, catSvc)
	if err == nil {
		if h.importRunRepo != nil {
			if run, rErr := h.importRunRepo.GetRunByPublicID(ctx, sessionID, orgID); rErr == nil && run != nil {
				_ = h.importRunRepo.TransitionState(ctx, run.ID, importrun.StateCommitted)
			}
		}
		return added, updated, nil
	}

	// 2. Transitional fallback to database rows if in-memory session was lost.
	if h.importRunRepo != nil {
		run, rErr := h.importRunRepo.GetRunByPublicID(ctx, sessionID, orgID)
		if rErr == nil && run != nil {
			rows, total, lErr := h.importRunRepo.ListRows(ctx, run.ID, true, 50000, 0)
			if lErr == nil && total > 0 {
				itemsToCommit := make([]*catalog.SavingProduct, 0, len(rows))
				for _, r := range rows {
					var item StagedSavingItem
					if jErr := json.Unmarshal(r.Data, &item); jErr == nil && item.Included {
						itemsToCommit = append(itemsToCommit, &catalog.SavingProduct{
							OrganizationID: orgID,
							UserID:         &userID,
							ProductID:      item.ProductID,
							NameProduct:    item.NameProduct,
							SKU:            item.SKU,
							Quantity:       item.Quantity,
							Price:          item.Price,
						})
					}
				}
				if len(itemsToCommit) > 0 && catSvc != nil {
					a, u, cErr := catSvc.BatchUpsertSavingProducts(ctx, orgID, &userID, itemsToCommit)
					if cErr == nil {
						_ = h.importRunRepo.TransitionState(ctx, run.ID, importrun.StateCommitted)
						return a, u, nil
					}
					return 0, 0, cErr
				}
			}
		}
	}

	return 0, 0, err
}
