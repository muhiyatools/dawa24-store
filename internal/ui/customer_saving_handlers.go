package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/xuri/excelize/v2"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/modules/promo"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/shared/sheet"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// CustomerSavingProductsPage renders the customer's live price-delta tracking list.
func (h *UIHandler) CustomerSavingProductsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/customer/saving-products", http.StatusSeeOther)
		return
	}

	search := strings.TrimSpace(r.URL.Query().Get("search"))
	if search == "" {
		search = strings.TrimSpace(r.URL.Query().Get("q"))
	}
	filter := strings.TrimSpace(r.URL.Query().Get("filter"))
	if filter == "" {
		filter = "all"
	}

	var items []*catalog.SavingProductEnriched
	var stats *catalog.SavingProductStats

	if h.catSvc != nil {
		it, st, err := h.catSvc.ListSavingProductsEnriched(ctx, actor.OrganizationID, search, filter, 500, 0)
		if err == nil {
			items = it
			stats = st
		} else {
			h.log.ErrorContext(ctx, "customer list saving products enriched error", "error", err, "org_id", actor.OrganizationID)
		}
	}

	if stats == nil {
		stats = &catalog.SavingProductStats{}
	}

	noticeType := r.URL.Query().Get("notice_type")
	noticeMsg := r.URL.Query().Get("notice")

	pageData := pages.CustomerSavingPageData{
		Items:        items,
		Stats:        stats,
		SearchQuery:  search,
		FilterStatus: filter,
		NoticeType:   noticeType,
		NoticeMsg:    noticeMsg,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.CustomerSavingProductsPage(pageData, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render customer saving products", "error", err)
	}
}

// CustomerSavingProductCreateSubmit handles manual creation of a pharmacy saving product.
func (h *UIHandler) CustomerSavingProductCreateSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/customer/saving-products", http.StatusSeeOther)
		return
	}

	if err := r.ParseForm(); err != nil {
		h.redirectWithNotice(w, r, "/customer/saving-products", "error", "بيانات النموذج غير صالحة.")
		return
	}

	nameProduct := strings.TrimSpace(r.FormValue("name_product"))
	if nameProduct == "" {
		h.redirectWithNotice(w, r, "/customer/saving-products", "error", "اسم المنتج مطلوب.")
		return
	}

	sku := strings.TrimSpace(r.FormValue("sku"))
	qty, _ := strconv.ParseFloat(strings.TrimSpace(r.FormValue("qty")), 64)
	price, _ := money.Parse(strings.TrimSpace(r.FormValue("price")))

	var productID *int64
	if prodStr := strings.TrimSpace(r.FormValue("product_id")); prodStr != "" {
		if pid, err := strconv.ParseInt(prodStr, 10, 64); err == nil && pid > 0 {
			productID = &pid
		}
	}

	sp := &catalog.SavingProduct{
		OrganizationID: actor.OrganizationID,
		UserID:         &actor.UserID,
		ProductID:      productID,
		NameProduct:    nameProduct,
		SKU:            sku,
		Quantity:       qty,
		Price:          price,
	}

	if h.catSvc != nil {
		if err := h.catSvc.CreateSavingProduct(ctx, sp); err != nil {
			h.log.ErrorContext(ctx, "create customer saving product error", "error", err)
			h.redirectWithNotice(w, r, "/customer/saving-products", "error", "حدث خطأ أثناء حفظ المنتج: "+h.safeMessage(err, langOf(r)))
			return
		}
	}

	h.redirectWithNotice(w, r, "/customer/saving-products", "success", "تمت إضافة صنف التوفير بنجاح.")
}

// CustomerSavingProductUpdateSubmit handles updating an existing pharmacy saving product.
func (h *UIHandler) CustomerSavingProductUpdateSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/customer/saving-products", http.StatusSeeOther)
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/customer/saving-products", "error", "معرف الصنف غير صالح.")
		return
	}

	if err := r.ParseForm(); err != nil {
		h.redirectWithNotice(w, r, "/customer/saving-products", "error", "بيانات النموذج غير صالحة.")
		return
	}

	nameProduct := strings.TrimSpace(r.FormValue("name_product"))
	if nameProduct == "" {
		h.redirectWithNotice(w, r, "/customer/saving-products", "error", "اسم المنتج مطلوب.")
		return
	}

	sku := strings.TrimSpace(r.FormValue("sku"))
	qty, _ := strconv.ParseFloat(strings.TrimSpace(r.FormValue("qty")), 64)
	price, _ := money.Parse(strings.TrimSpace(r.FormValue("price")))

	var productID *int64
	if prodStr := strings.TrimSpace(r.FormValue("product_id")); prodStr != "" {
		if pid, err := strconv.ParseInt(prodStr, 10, 64); err == nil && pid > 0 {
			productID = &pid
		}
	}

	sp := &catalog.SavingProduct{
		ID:             id,
		OrganizationID: actor.OrganizationID,
		UserID:         &actor.UserID,
		ProductID:      productID,
		NameProduct:    nameProduct,
		SKU:            sku,
		Quantity:       qty,
		Price:          price,
	}

	if h.catSvc != nil {
		if err := h.catSvc.UpdateSavingProduct(ctx, sp); err != nil {
			h.log.ErrorContext(ctx, "update customer saving product error", "error", err)
			h.redirectWithNotice(w, r, "/customer/saving-products", "error", "حدث خطأ أثناء تعديل المنتج: "+h.safeMessage(err, langOf(r)))
			return
		}
	}

	h.redirectWithNotice(w, r, "/customer/saving-products", "success", "تم تحديث صنف التوفير بنجاح.")
}

// CustomerSavingProductDeleteSubmit deletes a saving product record for the pharmacy.
func (h *UIHandler) CustomerSavingProductDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/customer/saving-products", http.StatusSeeOther)
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/customer/saving-products", "error", "معرف الصنف غير صالح.")
		return
	}

	if h.catSvc != nil {
		if err := h.catSvc.DeleteSavingProduct(ctx, id, actor.OrganizationID); err != nil {
			h.log.ErrorContext(ctx, "delete customer saving product error", "error", err)
			h.redirectWithNotice(w, r, "/customer/saving-products", "error", "حدث خطأ أثناء حذف المنتج.")
			return
		}
	}

	h.redirectWithNotice(w, r, "/customer/saving-products", "success", "تم حذف الصنف من قائمة التوفير بنجاح.")
}

// CustomerSavingProductsDeleteAllSubmit deletes all saving products for the customer org.
func (h *UIHandler) CustomerSavingProductsDeleteAllSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/customer/saving-products", http.StatusSeeOther)
		return
	}

	if h.catSvc != nil {
		if err := h.catSvc.DeleteAllSavingProducts(ctx, actor.OrganizationID); err != nil {
			h.log.ErrorContext(ctx, "delete all customer saving products error", "error", err)
			h.redirectWithNotice(w, r, "/customer/saving-products", "error", "حدث خطأ أثناء حذف الأصناف.")
			return
		}
	}

	h.redirectWithNotice(w, r, "/customer/saving-products", "success", "تم حذف جميع أصناف التوفير بنجاح.")
}

// CustomerSavingProductsImportSubmit handles Excel/CSV bulk import of saving products for pharmacy.
func (h *UIHandler) CustomerSavingProductsImportSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/customer/saving-products", http.StatusSeeOther)
		return
	}

	if err := r.ParseMultipartForm(MaxUploadBytes); err != nil {
		h.redirectWithNotice(w, r, "/customer/saving-products", "error", "تعذر قراءة الملف المرفوع.")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		h.redirectWithNotice(w, r, "/customer/saving-products", "error", "يرجى اختيار ملف Excel أو CSV.")
		return
	}
	defer file.Close()

	ext := strings.ToLower(filepath.Ext(header.Filename))
	if ext != ".xlsx" && ext != ".xls" && ext != ".csv" {
		h.redirectWithNotice(w, r, "/customer/saving-products", "error", "صيغة الملف غير مدعومة. يرجى رفع ملف بصيغة xlsx أو xls أو csv.")
		return
	}

	fileBytes, err := io.ReadAll(file)
	if err != nil || len(fileBytes) == 0 {
		h.redirectWithNotice(w, r, "/customer/saving-products", "error", "الملف فارغ أو تعذر قراءته.")
		return
	}

	rawRows, err := sheet.ReadRows(fileBytes, header.Filename)
	if err != nil || len(rawRows) < 2 {
		h.log.WarnContext(ctx, "failed to parse spreadsheet", "error", err, "filename", header.Filename)
		h.redirectWithNotice(w, r, "/customer/saving-products", "error", "تعذر قراءة ملف البيانات المرفوع أو أن الملف لا يحتوي على صفوف بيانات صالحة.")
		return
	}

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
		h.redirectWithNotice(w, r, "/customer/saving-products", "error", "لم يتم العثور على أي منتجات صالحة للاستيراد في الملف.")
		return
	}

	added, updated, err := h.catSvc.BatchUpsertSavingProducts(ctx, actor.OrganizationID, &actor.UserID, parsedItems)
	if err != nil {
		h.log.ErrorContext(ctx, "customer failed to batch upsert saving products", "error", err, "org_id", actor.OrganizationID)
		h.redirectWithNotice(w, r, "/customer/saving-products", "error", "حدث خطأ أثناء حفظ المنتجات: "+h.safeMessage(err, langOf(r)))
		return
	}

	successMsg := fmt.Sprintf("تم استيراد ومعالجة %d منتج بنجاح (جديد: %d، تم تحديثه: %d). تم ربط %d صنف بالكتالوج، و %d صنف غير مرتبطة.", len(parsedItems), added, updated, matchedCount, unlinkedCount)
	h.redirectWithNotice(w, r, "/customer/saving-products", "success", successMsg)
}

// SavingProductsPreviewResponse represents the preview payload returned to UI.
type SavingProductsPreviewResponse struct {
	Success    bool               `json:"success"`
	Error      string             `json:"error,omitempty"`
	Headers    []string           `json:"headers"`
	Detected   SavingDetectedCols `json:"detected"`
	SampleRows [][]string         `json:"sample_rows"`
}

// CustomerSavingProductsPreviewColumnsJSON reads uploaded spreadsheet and returns headers and detected columns.
func (h *UIHandler) CustomerSavingProductsPreviewColumnsJSON(w http.ResponseWriter, r *http.Request) {
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

// CustomerSavingProductsImportStartJSON initiates asynchronous processing of an uploaded file.
func (h *UIHandler) CustomerSavingProductsImportStartJSON(w http.ResponseWriter, r *http.Request) {
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

// CustomerSavingProductsImportProgressJSON returns the live state and staged items for review.
func (h *UIHandler) CustomerSavingProductsImportProgressJSON(w http.ResponseWriter, r *http.Request) {
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

// CustomerSavingProductsImportCommitJSON commits the staged session into catalog.saving_products.
func (h *UIHandler) CustomerSavingProductsImportCommitJSON(w http.ResponseWriter, r *http.Request) {
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

// CustomerSavingProductsImportCancelJSON discards and clears the staged session.
func (h *UIHandler) CustomerSavingProductsImportCancelJSON(w http.ResponseWriter, r *http.Request) {
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

// CustomerSavingProductsExport streams an Excel spreadsheet of all saving products for the pharmacy.
func (h *UIHandler) CustomerSavingProductsExport(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/customer/saving-products", http.StatusSeeOther)
		return
	}

	var items []*catalog.SavingProductEnriched
	if h.catSvc != nil {
		it, _, err := h.catSvc.ListSavingProductsEnriched(ctx, actor.OrganizationID, "", "all", 2000, 0)
		if err == nil {
			items = it
		}
	}

	f := excelize.NewFile()
	sheet := "Saving Products"
	f.SetSheetName("Sheet1", sheet)

	_ = f.SetSheetView(sheet, 0, &excelize.ViewOptions{
		RightToLeft: func(b bool) *bool { return &b }(true),
	})

	headers := []string{
		"معرف الصنف (ID)",
		"اسم صنف الصيدلية",
		"كود SKU",
		"الكمية",
		"سعر الجمهور المسجل (ج.م)",
		"القيمة الإجمالية (ج.م)",
		"معرف صنف الكتالوج (ProductID)",
		"اسم الصنف المرتبط بالكتالوج",
		"عدد الموردين المتاحين",
	}

	for colIdx, header := range headers {
		cell, _ := excelize.CoordinatesToCellName(colIdx+1, 1)
		_ = f.SetCellValue(sheet, cell, header)
	}

	for rowIdx, it := range items {
		rNum := rowIdx + 2
		_ = f.SetCellValue(sheet, fmt.Sprintf("A%d", rNum), it.ID)
		_ = f.SetCellValue(sheet, fmt.Sprintf("B%d", rNum), it.NameProduct)
		_ = f.SetCellValue(sheet, fmt.Sprintf("C%d", rNum), it.SKU)
		_ = f.SetCellValue(sheet, fmt.Sprintf("D%d", rNum), it.Quantity)
		_ = f.SetCellValue(sheet, fmt.Sprintf("E%d", rNum), it.Price.String())
		_ = f.SetCellValue(sheet, fmt.Sprintf("F%d", rNum), it.TotalValue.String())

		if it.ProductID != nil && *it.ProductID > 0 {
			_ = f.SetCellValue(sheet, fmt.Sprintf("G%d", rNum), *it.ProductID)
			_ = f.SetCellValue(sheet, fmt.Sprintf("H%d", rNum), it.LinkedProductName.Get(i18n.AR))
			_ = f.SetCellValue(sheet, fmt.Sprintf("I%d", rNum), it.ProvidingOrgsCount)
		} else {
			_ = f.SetCellValue(sheet, fmt.Sprintf("G%d", rNum), "")
			_ = f.SetCellValue(sheet, fmt.Sprintf("H%d", rNum), "غير مرتبط")
			_ = f.SetCellValue(sheet, fmt.Sprintf("I%d", rNum), 0)
		}
	}

	filename := fmt.Sprintf("pharmacy_saving_products_%d.xlsx", actor.OrganizationID)
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	_ = f.Write(w)
}

// CustomerSavingProductProvidersJSON delegates to VendorSavingProductProvidersJSON logic.
func (h *UIHandler) CustomerSavingProductProvidersJSON(w http.ResponseWriter, r *http.Request) {
	h.VendorSavingProductProvidersJSON(w, r)
}

// CustomerSavingProductSearchJSON delegates to VendorSavingProductSearchJSON logic.
func (h *UIHandler) CustomerSavingProductSearchJSON(w http.ResponseWriter, r *http.Request) {
	h.VendorSavingProductSearchJSON(w, r)
}

// CustomerSavingProductDetailPage renders single saving product delta details.
func (h *UIHandler) CustomerSavingProductDetailPage(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/customer/saving-products", http.StatusSeeOther)
}

// CustomerSavingProductsAlias redirects misspelled route /customer/saveing-products.
func (h *UIHandler) CustomerSavingProductsAlias(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/customer/saving-products", http.StatusMovedPermanently)
}

// CustomerOfferOrdersPage renders offer-based orders placed by the pharmacy.
func (h *UIHandler) CustomerOfferOrdersPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/orders/offers", http.StatusSeeOther)
		return
	}

	var orders []*commerce.Order
	if h.commSvc != nil {
		orders, _ = h.commSvc.ListCustomerOrders(ctx, actor.OrganizationID, 100, 0)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.CustomerOfferOrdersPage(orders, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render customer offer orders", "error", err)
	}
}

// CustomerOfferOrderDetailPage renders single offer order details.
func (h *UIHandler) CustomerOfferOrderDetailPage(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	http.Redirect(w, r, fmt.Sprintf("/orders/%s", idStr), http.StatusSeeOther)
}

// CustomerOfferCheckoutPage renders dedicated checkout flow for a specific offer.
func (h *UIHandler) CustomerOfferCheckoutPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/checkout", http.StatusSeeOther)
		return
	}

	idStr := chi.URLParam(r, "id")
	offerID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || offerID <= 0 {
		http.Redirect(w, r, "/offers", http.StatusSeeOther)
		return
	}

	var offer *promo.Offer
	if h.promoSvc != nil {
		offer, _ = h.promoSvc.GetOffer(database.AsSystem(ctx), offerID)
	}

	if offer == nil {
		http.Redirect(w, r, "/offers", http.StatusSeeOther)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.CustomerOfferCheckoutPage(offer, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render offer checkout", "error", err)
	}
}

// CustomerAddOrderPage renders manual / quick order entry form.
func (h *UIHandler) CustomerAddOrderPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.CustomerAddOrderPage(lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render customer add order", "error", err)
	}
}

// CustomerProductsMainAlias redirects /customer/products/main/{id} to /catalog/{id}.
func (h *UIHandler) CustomerProductsMainAlias(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	http.Redirect(w, r, fmt.Sprintf("/catalog/%s", idStr), http.StatusMovedPermanently)
}

// GuestOrderTrackingPage renders unauthenticated public order tracking screen.
func (h *UIHandler) GuestOrderTrackingPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	orderNumber := r.URL.Query().Get("order_number")
	var order *commerce.Order
	if orderNumber != "" && h.commSvc != nil {
		order, _ = h.commSvc.GetOrderByNumber(database.AsSystem(ctx), orderNumber)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.GuestOrderTrackingPage(orderNumber, order, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render guest tracking", "error", err)
	}
}

func isAllDigitsOrCode(s string) bool {
	if s == "" {
		return false
	}
	digits := 0
	for _, r := range s {
		if r >= '0' && r <= '9' {
			digits++
		} else if r != '-' && r != '_' && r != '.' && r != '/' && r != ' ' {
			return false
		}
	}
	return digits > 0
}

func isDescriptiveArabicText(s string) bool {
	if len(s) < 3 {
		return false
	}
	hasArabic := false
	hasSpace := false
	for _, r := range s {
		if (r >= 0x0600 && r <= 0x06FF) || (r >= 0x0750 && r <= 0x077F) {
			hasArabic = true
		}
		if r == ' ' {
			hasSpace = true
		}
	}
	return hasArabic && hasSpace
}
