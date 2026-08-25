package ui

import (
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
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/shared/spreadsheet"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// VendorSavingProductsPage renders the vendor's saving products directory.
func (h *UIHandler) VendorSavingProductsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/saving-products", http.StatusSeeOther)
		return
	}

	search := strings.TrimSpace(r.URL.Query().Get("search"))
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
			h.log.ErrorContext(ctx, "list saving products enriched error", "error", err, "org_id", actor.OrganizationID)
		}
	}

	if stats == nil {
		stats = &catalog.SavingProductStats{}
	}

	noticeType := r.URL.Query().Get("notice_type")
	noticeMsg := r.URL.Query().Get("notice")

	pageData := pages.VendorSavingPageData{
		Items:        items,
		Stats:        stats,
		SearchQuery:  search,
		FilterStatus: filter,
		NoticeType:   noticeType,
		NoticeMsg:    noticeMsg,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.VendorSavingProductsPage(pageData, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render vendor saving products", "error", err)
	}
}

// VendorSavingProductCreateSubmit handles manual creation of a saving product.
func (h *UIHandler) VendorSavingProductCreateSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/saving-products", http.StatusSeeOther)
		return
	}

	if err := r.ParseForm(); err != nil {
		h.redirectWithNotice(w, r, "/vendor/saving-products", "error", "بيانات النموذج غير صالحة.")
		return
	}

	nameProduct := strings.TrimSpace(r.FormValue("name_product"))
	if nameProduct == "" {
		h.redirectWithNotice(w, r, "/vendor/saving-products", "error", "اسم المنتج مطلوب.")
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
			h.redirectWithNotice(w, r, "/vendor/saving-products", "error", h.safeMessage(err, langOf(r)))
			return
		}
	}

	h.redirectWithNotice(w, r, "/vendor/saving-products", "success", "تمت إضافة منتج المساعدة بنجاح.")
}

// VendorSavingProductUpdateSubmit handles updating an existing saving product.
func (h *UIHandler) VendorSavingProductUpdateSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/saving-products", http.StatusSeeOther)
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/vendor/saving-products", "error", "معرف منتج غير صالح.")
		return
	}

	if err := r.ParseForm(); err != nil {
		h.redirectWithNotice(w, r, "/vendor/saving-products", "error", "بيانات النموذج غير صالحة.")
		return
	}

	nameProduct := strings.TrimSpace(r.FormValue("name_product"))
	if nameProduct == "" {
		h.redirectWithNotice(w, r, "/vendor/saving-products", "error", "اسم المنتج مطلوب.")
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
			h.redirectWithNotice(w, r, "/vendor/saving-products", "error", h.safeMessage(err, langOf(r)))
			return
		}
	}

	h.redirectWithNotice(w, r, "/vendor/saving-products", "success", "تم تحديث بيانات منتج المساعدة وتعيين الربط بنجاح.")
}

// VendorSavingProductDeleteSubmit deletes a saving product record.
func (h *UIHandler) VendorSavingProductDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/saving-products", http.StatusSeeOther)
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/vendor/saving-products", "error", "معرف منتج غير صالح.")
		return
	}

	if h.catSvc != nil {
		if err := h.catSvc.DeleteSavingProduct(ctx, id, actor.OrganizationID); err != nil {
			h.redirectWithNotice(w, r, "/vendor/saving-products", "error", h.safeMessage(err, langOf(r)))
			return
		}
	}

	h.redirectWithNotice(w, r, "/vendor/saving-products", "success", "تم حذف المنتج من قائمة المساعدة بنجاح.")
}

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

	ext := strings.ToLower(filepath.Ext(header.Filename))
	if ext != ".xlsx" && ext != ".xls" && ext != ".csv" {
		h.redirectWithNotice(w, r, "/vendor/saving-products", "error", "صيغة الملف غير مدعومة. يرجى رفع ملف بصيغة xlsx أو xls أو csv.")
		return
	}

	fileBytes, err := io.ReadAll(file)
	if err != nil || len(fileBytes) == 0 {
		h.redirectWithNotice(w, r, "/vendor/saving-products", "error", "الملف فارغ أو تعذر قراءته.")
		return
	}

	rawRows, err := spreadsheet.ReadRows(fileBytes)
	if err != nil || len(rawRows) < 2 {
		h.log.WarnContext(ctx, "failed to parse spreadsheet", "error", err, "filename", header.Filename)
		h.redirectWithNotice(w, r, "/vendor/saving-products", "error", "تعذر قراءة ملف البيانات المرفوع أو أن الملف لا يحتوي على صفوف بيانات صالحة.")
		return
	}

	// 1. Detect column mapping from header row
	headers := rawRows[0]
	nameCol := -1
	skuCol := -1
	qtyCol := -1
	priceCol := -1
	productIDCol := -1

	for idx, hName := range headers {
		norm := strings.TrimSpace(strings.ToLower(hName))
		norm = strings.ReplaceAll(norm, "_", "")
		norm = strings.ReplaceAll(norm, "-", "")
		norm = strings.ReplaceAll(norm, " ", "")

		if strings.Contains(norm, "productid") || strings.Contains(norm, "معرفالمنتج") || strings.Contains(norm, "رقمصنف") || strings.Contains(norm, "رقممنتج") || norm == "id" || norm == "pid" {
			if productIDCol == -1 {
				productIDCol = idx
			}
		} else if strings.Contains(norm, "name") || strings.Contains(norm, "اسم") || strings.Contains(norm, "صنف") || strings.Contains(norm, "منتج") || strings.Contains(norm, "item") {
			if nameCol == -1 {
				nameCol = idx
			}
		} else if strings.Contains(norm, "sku") || strings.Contains(norm, "كود") || strings.Contains(norm, "رمز") || strings.Contains(norm, "barcode") || strings.Contains(norm, "باركود") {
			if skuCol == -1 {
				skuCol = idx
			}
		} else if strings.Contains(norm, "qty") || strings.Contains(norm, "quantity") || strings.Contains(norm, "كمية") || strings.Contains(norm, "الكمية") || strings.Contains(norm, "stock") {
			if qtyCol == -1 {
				qtyCol = idx
			}
		} else if strings.Contains(norm, "price") || strings.Contains(norm, "سعر") || strings.Contains(norm, "السعر") || strings.Contains(norm, "cost") {
			if priceCol == -1 {
				priceCol = idx
			}
		}
	}

	// Positional fallback if names not detected
	if nameCol == -1 {
		if len(headers) >= 1 {
			nameCol = 0
		}
	}
	if skuCol == -1 && len(headers) >= 2 {
		skuCol = 1
	}
	if qtyCol == -1 && len(headers) >= 3 {
		qtyCol = 2
	}
	if priceCol == -1 && len(headers) >= 4 {
		priceCol = 3
	}

	// 2. Pre-cache catalog lookup map for instant ultra-fast smart matching
	skuToProductID := make(map[string]int64)
	nameToProductID := make(map[string]int64)

	if h.catSvc != nil {
		allCatalog, _ := h.catSvc.Search(ctx, catalog.SearchParams{Limit: 2000})
		for _, p := range allCatalog {
			if p.SKU != "" {
				skuToProductID[strings.ToLower(strings.TrimSpace(p.SKU))] = p.ID
			}
			arName := strings.ToLower(strings.TrimSpace(p.Name.Get(i18n.AR)))
			enName := strings.ToLower(strings.TrimSpace(p.Name.Get(i18n.EN)))
			if arName != "" {
				nameToProductID[arName] = p.ID
			}
			if enName != "" {
				nameToProductID[enName] = p.ID
			}
		}
	}

	// 3. Process each data row
	var parsedItems []*catalog.SavingProduct
	var matchedCount int
	var unlinkedCount int

	for i := 1; i < len(rawRows); i++ {
		row := rawRows[i]
		if len(row) == 0 {
			continue
		}

		var name string
		if nameCol >= 0 && nameCol < len(row) {
			name = strings.TrimSpace(row[nameCol])
		}
		if name == "" {
			continue
		}

		var sku string
		if skuCol >= 0 && skuCol < len(row) {
			sku = strings.TrimSpace(row[skuCol])
		}

		var qty float64
		if qtyCol >= 0 && qtyCol < len(row) {
			qStr := strings.TrimSpace(row[qtyCol])
			qStr = strings.ReplaceAll(qStr, ",", "")
			qty, _ = strconv.ParseFloat(qStr, 64)
		}

		var price money.Amount
		if priceCol >= 0 && priceCol < len(row) {
			pStr := strings.TrimSpace(row[priceCol])
			pStr = strings.ReplaceAll(pStr, ",", "")
			pStr = strings.ReplaceAll(pStr, "EGP", "")
			pStr = strings.ReplaceAll(pStr, "ج.م", "")
			price, _ = money.Parse(strings.TrimSpace(pStr))
		}

		// Determine product_id linkage
		var productID *int64
		if productIDCol >= 0 && productIDCol < len(row) {
			if pid, err := strconv.ParseInt(strings.TrimSpace(row[productIDCol]), 10, 64); err == nil && pid > 0 {
				productID = &pid
			}
		}

		// If no explicit product ID, attempt smart auto-match
		if productID == nil {
			if sku != "" {
				if pid, ok := skuToProductID[strings.ToLower(sku)]; ok {
					productID = &pid
				}
			}
			if productID == nil {
				cleanName := strings.ToLower(name)
				if pid, ok := nameToProductID[cleanName]; ok {
					productID = &pid
				}
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

// VendorSavingProductsExport streams an Excel spreadsheet of all saving products.
func (h *UIHandler) VendorSavingProductsExport(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/saving-products", http.StatusSeeOther)
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

	// Set RTL
	_ = f.SetSheetView(sheet, 0, &excelize.ViewOptions{
		RightToLeft: func(b bool) *bool { return &b }(true),
	})

	headers := []string{
		"معرف الصنف (ID)",
		"اسم صنف المنظمة",
		"كود SKU",
		"الكمية",
		"سعر الوحدة (ج.م)",
		"القيمة الإجمالية (ج.م)",
		"معرف صنف الكتالوج (ProductID)",
		"اسم الصنف المرتبط بالكتالوج",
		"عدد المنظمات الموفرة",
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

	filename := fmt.Sprintf("saving_products_org_%d.xlsx", actor.OrganizationID)
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	_ = f.Write(w)
}

// VendorSavingProductProvidersJSON returns JSON array of suppliers selling a master product.
func (h *UIHandler) VendorSavingProductProvidersJSON(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_, ok := authctx.From(ctx)
	if !ok {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	productID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || productID <= 0 {
		http.Error(w, `{"error":"invalid product id"}`, http.StatusBadRequest)
		return
	}

	if h.catSvc == nil {
		http.Error(w, `{"error":"service unavailable"}`, http.StatusServiceUnavailable)
		return
	}

	providers, err := h.catSvc.GetProductProviders(ctx, productID)
	if err != nil {
		h.log.ErrorContext(ctx, "get product providers error", "error", err, "product_id", productID)
		providers = []*catalog.ProductProviderInfo{}
	}

	type jsonProvider struct {
		OrgID              int64  `json:"org_id"`
		OrgName            string `json:"org_name"`
		VariantName        string `json:"variant_name"`
		SKU                string `json:"sku"`
		Unit               string `json:"unit"`
		Price              string `json:"price"`
		Discount           string `json:"discount"`
		PriceAfterDiscount string `json:"price_after_discount"`
		StockQuantity      int    `json:"stock_quantity"`
		BranchName         string `json:"branch_name"`
	}

	var res []jsonProvider
	for _, p := range providers {
		res = append(res, jsonProvider{
			OrgID:              p.OrgID,
			OrgName:            p.OrgName.Get(i18n.AR),
			VariantName:        p.VariantName.Get(i18n.AR),
			SKU:                p.SKU,
			Unit:               p.Unit,
			Price:              p.Price.String(),
			Discount:           p.Discount.String(),
			PriceAfterDiscount: p.PriceAfterDiscount.String(),
			StockQuantity:      p.StockQuantity,
			BranchName:         p.BranchName.Get(i18n.AR),
		})
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(res)
}

// VendorSavingProductSearchJSON returns JSON autocomplete list of catalog products for linking.
func (h *UIHandler) VendorSavingProductSearchJSON(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_, ok := authctx.From(ctx)
	if !ok {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(query) < 2 {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode([]any{})
		return
	}

	type searchResult struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		SKU  string `json:"sku"`
	}

	var results []searchResult
	if h.catSvc != nil {
		products, err := h.catSvc.Search(ctx, catalog.SearchParams{
			Query: query,
			Limit: 15,
		})
		if err == nil {
			for _, p := range products {
				name := p.Name.Get(i18n.AR)
				if name == "" {
					name = p.Name.Get(i18n.EN)
				}
				results = append(results, searchResult{
					ID:   fmt.Sprintf("%d", p.ID),
					Name: name,
					SKU:  p.SKU,
				})
			}
		}
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(results)
}

// VendorSavingProductsAlias redirects misspelled route /vendor/saveing-products.
func (h *UIHandler) VendorSavingProductsAlias(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/vendor/saving-products", http.StatusMovedPermanently)
}
