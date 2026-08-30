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

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/shared/sheet"
)

// VendorSavingProductsImportSubmit handles Excel/CSV bulk import with intelligent auto-matching.
func (h *UIHandler) VendorSavingProductsImportSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/saving-products", http.StatusSeeOther)
		return
	}

	if err := r.ParseMultipartForm(MaxUploadBytes); err != nil {
		h.redirectWithNotice(w, r, "/vendor/saving-products", "error", "تعذر قراءة الملف المرفوع.")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		h.redirectWithNotice(w, r, "/vendor/saving-products", "error", "يرجى اختيار ملف Excel أو CSV.")
		return
	}
	defer file.Close()

	if !SupportedUploadName(header.Filename) {
		h.redirectWithNotice(w, r, "/vendor/saving-products", "error", unsupportedUploadMessage)
		return
	}

	fileBytes, err := io.ReadAll(file)
	if err != nil || len(fileBytes) == 0 {
		h.redirectWithNotice(w, r, "/vendor/saving-products", "error", "الملف فارغ أو تعذر قراءته.")
		return
	}

	rawRows, err := sheet.ReadRows(fileBytes, header.Filename)
	if err != nil || len(rawRows) < 2 {
		h.log.WarnContext(ctx, "failed to parse spreadsheet", "error", err, "filename", header.Filename)
		h.redirectWithNotice(w, r, "/vendor/saving-products", "error", "تعذر قراءة ملف البيانات المرفوع أو أن الملف لا يحتوي على صفوف بيانات صالحة.")
		return
	}

	// 1. Detect column mapping from header row & optional user overrides
	headers := rawRows[0]
	colNameOverride := strings.TrimSpace(r.FormValue("col_name"))
	colSKUOverride := strings.TrimSpace(r.FormValue("col_sku"))
	colQtyOverride := strings.TrimSpace(r.FormValue("col_qty"))
	colPriceOverride := strings.TrimSpace(r.FormValue("col_price"))
	colProductIDOverride := strings.TrimSpace(r.FormValue("col_product_id"))

	sampleRows := rawRows[1:]
	if len(sampleRows) > 10 {
		sampleRows = sampleRows[:10]
	}

	nameCol, skuCol, qtyCol, priceCol, productIDCol := detectSavingProductColumns(
		headers,
		sampleRows,
		colNameOverride,
		colSKUOverride,
		colQtyOverride,
		colPriceOverride,
		colProductIDOverride,
	)

	matchStrategy := MatchStrategy(strings.TrimSpace(r.FormValue("match_strategy")))
	if matchStrategy == "" {
		matchStrategy = StrategySmartAuto
	}

	var matchEngine *SavingProductMatchEngine
	if h.catSvc != nil {
		if catalogSources, err := h.catSvc.ListMatchProducts(ctx); err == nil && len(catalogSources) > 0 {
			matchEngine = NewSavingProductMatchEngine(catalogSources)
		}
	}

	// 3. Process each data row
	var parsedItems []*catalog.SavingProduct
	var matchedCount int
	var unlinkedCount int

	for i := 1; i < len(rawRows); i++ {
		row := rawRows[i]
		if len(row) == 0 || IsAllEmptyRow(row) || IsSummaryOrTotalRow(row) {
			continue
		}

		var name string
		if nameCol >= 0 && nameCol < len(row) {
			name = strings.TrimSpace(row[nameCol])
		}

		var sku string
		if skuCol >= 0 && skuCol < len(row) {
			sku = strings.TrimSpace(row[skuCol])
		}

		// Double-check row-level swap: if name looks like a pure numeric SKU/barcode (>= 4 digits) and sku looks like Arabic descriptive drug name, swap them
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
		if qtyCol >= 0 && qtyCol < len(row) {
			qty, _ = ParseFlexibleQuantity(row[qtyCol])
		}

		var price money.Amount
		if priceCol >= 0 && priceCol < len(row) {
			price, _ = ParseFlexibleMoney(row[priceCol])
		}

		// Determine product_id linkage
		var productID *int64
		if productIDCol >= 0 && productIDCol < len(row) {
			if pid, err := strconv.ParseInt(strings.TrimSpace(row[productIDCol]), 10, 64); err == nil && pid > 0 {
				productID = &pid
			}
		}

		if matchEngine != nil {
			matchRes := matchEngine.Match(matchStrategy, productID, sku, name)
			if matchRes.ProductID != nil {
				productID = matchRes.ProductID
			}
		}

		if productID != nil {
			matchedCount++
		} else {
			unlinkedCount++
		}

		parsedItems = append(parsedItems, &catalog.SavingProduct{
			OrganizationID: actor.OrganizationID,
			UserID:         &actor.UserID,
			ProductID:      productID,
			NameProduct:    name,
			SKU:            sku,
			Quantity:       qty,
			Price:          price,
		})
	}

	if len(parsedItems) == 0 {
		h.redirectWithNotice(w, r, "/vendor/saving-products", "error", "لم يتم العثور على أي منتجات صالحة للاستيراد في الملف.")
		return
	}

	added, updated, err := h.catSvc.BatchUpsertSavingProducts(ctx, actor.OrganizationID, &actor.UserID, parsedItems)
	if err != nil {
		h.log.ErrorContext(ctx, "failed to batch upsert saving products", "error", err, "org_id", actor.OrganizationID)
		h.redirectWithNotice(w, r, "/vendor/saving-products", "error", "حدث خطأ أثناء حفظ المنتجات: "+h.safeMessage(err, langOf(r)))
		return
	}

	successMsg := fmt.Sprintf("تم استيراد ومعالجة %d منتج بنجاح (جديد: %d، تم تحديثه: %d). تم ربط %d صنف بالكتالوج، و %d صنف غير مرتبطة.", len(parsedItems), added, updated, matchedCount, unlinkedCount)
	h.redirectWithNotice(w, r, "/vendor/saving-products", "success", successMsg)
}

// VendorSavingProductsPreviewColumnsJSON reads uploaded spreadsheet and returns headers and detected columns.
func (h *UIHandler) VendorSavingProductsPreviewColumnsJSON(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")

	if err := r.ParseMultipartForm(10 << 20); err != nil {
		_ = json.NewEncoder(w).Encode(SavingProductsPreviewResponse{Success: false, Error: "الملف كبير جداً أو غير صالح"})
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		_ = json.NewEncoder(w).Encode(SavingProductsPreviewResponse{Success: false, Error: "يرجى اختيار ملف Excel أو CSV صالح"})
		return
	}
	defer file.Close()

	fileBytes, err := io.ReadAll(file)
	if err != nil || len(fileBytes) == 0 {
		_ = json.NewEncoder(w).Encode(SavingProductsPreviewResponse{Success: false, Error: "تعذر قراءة محتوى الملف"})
		return
	}

	rawRows, err := sheet.ReadRows(fileBytes, header.Filename)
	if err != nil || len(rawRows) == 0 {
		_ = json.NewEncoder(w).Encode(SavingProductsPreviewResponse{Success: false, Error: "تعذر قراءة أوراق العمل أو الجداول داخل الملف"})
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

	nameCol, skuCol, qtyCol, priceCol, productIDCol := detectSavingProductColumns(headers, sampleRows, "", "", "", "", "")

	_ = json.NewEncoder(w).Encode(SavingProductsPreviewResponse{
		Success: true,
		Headers: headers,
		Detected: SavingDetectedCols{
			NameCol:      nameCol,
			SKUCol:       skuCol,
			QtyCol:       qtyCol,
			PriceCol:     priceCol,
			ProductIDCol: productIDCol,
		},
		SampleRows: sampleRows,
	})
}

// VendorSavingProductsImportStartJSON initiates asynchronous processing of an uploaded file for vendor.
func (h *UIHandler) VendorSavingProductsImportStartJSON(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": "غير مصرح"})
		return
	}

	if err := r.ParseMultipartForm(32 << 20); err != nil {
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": "الملف كبير جداً أو غير صالح"})
		return
	}

	file, fileHeader, err := r.FormFile("file")
	if err != nil {
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": "يرجى اختيار ملف Excel أو CSV صالح"})
		return
	}
	defer file.Close()

	fileBytes, err := io.ReadAll(file)
	if err != nil || len(fileBytes) == 0 {
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": "تعذر قراءة محتوى الملف"})
		return
	}

	rawRows, err := sheet.ReadRows(fileBytes, fileHeader.Filename)
	if err != nil || len(rawRows) <= 1 {
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": "الملف فارغ أو لا يحتوي على صفوف بيانات"})
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

	matchStrategy := MatchStrategy(strings.TrimSpace(r.FormValue("match_strategy")))
	if matchStrategy == "" {
		matchStrategy = StrategySmartAuto
	}

	// Read before the goroutine and passed in, not captured: the background
	// worker must run under the choice the user made on the request that
	// started it, not whatever the session happens to hold later.
	useAI := r.FormValue("use_ai") == "1" || r.FormValue("use_ai") == "on"

	session := globalSavingImportSessionStore.NewSession(actor.OrganizationID, actor.UserID, fileHeader.Filename, len(rawRows)-1)

	// Launch async background processing
	go func(sessID string, orgID, userID int64, dataRows [][]string, nCol, sCol, qCol, pCol, pidCol int, strat MatchStrategy, aiOn bool) {
		bgCtx := context.Background()

		globalSavingImportSessionStore.UpdateProgress(sessID, 15, "تحميل وفهرسة كتالوج الأدوية المعتمد", 0)

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

		globalSavingImportSessionStore.UpdateProgress(sessID, 30, "جاري المطابقة الذكية للأصناف وتجهيز المسودة", 0)

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
				res := matchEngine.Match(strat, productID, sku, name)
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
				globalSavingImportSessionStore.UpdateProgress(sessID, pct, fmt.Sprintf("تمت معالجة %d من أصل %d صنف", i+1, total), i+1)
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
	}(session.ID, actor.OrganizationID, actor.UserID, rawRows[1:], nameCol, skuCol, qtyCol, priceCol, productIDCol, matchStrategy, useAI)

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
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": "غير مصرح"})
		return
	}

	sessionID := chi.URLParam(r, "id")
	session, ok := globalSavingImportSessionStore.GetSession(sessionID, actor.OrganizationID)
	if !ok {
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": "جلسة الاستيراد غير موجودة أو انتهت صلاحيتها"})
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
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": "غير مصرح"})
		return
	}

	sessionID := chi.URLParam(r, "id")
	added, updated, err := globalSavingImportSessionStore.CommitSession(ctx, sessionID, actor.OrganizationID, actor.UserID, h.catSvc)
	if err != nil {
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": "فشل الحفظ: " + h.safeMessage(err, langOf(r))})
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]any{
		"success": true,
		"added":   added,
		"updated": updated,
		"message": fmt.Sprintf("تم استيراد وحفظ %d منتج بنجاح (جديد: %d، تم تحديثه: %d).", added+updated, added, updated),
	})
}

// VendorSavingProductsImportCancelJSON discards and clears the staged session.
func (h *UIHandler) VendorSavingProductsImportCancelJSON(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": "غير مصرح"})
		return
	}

	sessionID := chi.URLParam(r, "id")
	cancelled := globalSavingImportSessionStore.CancelSession(sessionID, actor.OrganizationID)
	_ = json.NewEncoder(w).Encode(map[string]any{"success": cancelled})
}
