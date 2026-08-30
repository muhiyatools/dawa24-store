package ui

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/xuri/excelize/v2"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
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

	h.renderPage(ctx, w, "render customer saving products", pages.CustomerSavingProductsPage(pageData, lang, dir))
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

// SavingProductsPreviewResponse represents the preview payload returned to UI.
type SavingProductsPreviewResponse struct {
	Success    bool               `json:"success"`
	Error      string             `json:"error,omitempty"`
	Headers    []string           `json:"headers"`
	Detected   SavingDetectedCols `json:"detected"`
	SampleRows [][]string         `json:"sample_rows"`
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
