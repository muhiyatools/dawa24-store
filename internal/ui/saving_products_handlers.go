package ui

import (
	"encoding/json"
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
	"github.com/muhiya/dawa24-store/internal/ui/components"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// SavingProductsPreviewResponse represents the preview payload returned to UI.
type SavingProductsPreviewResponse struct {
	Success    bool               `json:"success"`
	Error      string             `json:"error,omitempty"`
	Headers    []string           `json:"headers"`
	Detected   SavingDetectedCols `json:"detected"`
	SampleRows [][]string         `json:"sample_rows"`
}

// handleSavingProductsPage renders the live price-delta tracking list for customer or vendor.
func (h *UIHandler) handleSavingProductsPage(w http.ResponseWriter, r *http.Request, audience string) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, fmt.Sprintf("/auth/login?redirect=/%s/saving-products", audience), http.StatusSeeOther)
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

	limit := h.pageLimit(r)
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	offset := (page - 1) * limit

	if h.catSvc != nil {
		it, st, err := h.catSvc.ListSavingProductsEnriched(ctx, actor.OrganizationID, search, filter, limit, offset)
		if err == nil {
			items = it
			stats = st
		} else {
			h.log.ErrorContext(ctx, fmt.Sprintf("%s list saving products enriched error", audience), "error", err, "org_id", actor.OrganizationID)
		}
	}

	if stats == nil {
		stats = &catalog.SavingProductStats{}
	}

	noticeType := r.URL.Query().Get("notice_type")
	noticeMsg := r.URL.Query().Get("notice")

	baseURL := fmt.Sprintf("/%s/saving-products", audience)
	paginationProps := components.PaginationProps{
		CurrentPage: page,
		PageSize:    limit,
		TotalCount:  stats.CountAll,
		BaseURL:     baseURL,
		QueryValues: r.URL.Query(),
	}

	if audience == "vendor" {
		pageData := pages.VendorSavingPageData{
			Items:        items,
			Stats:        stats,
			SearchQuery:  search,
			FilterStatus: filter,
			NoticeType:   noticeType,
			NoticeMsg:    noticeMsg,
			Pagination:   paginationProps,
		}
		h.renderPage(ctx, w, "render vendor saving products", pages.VendorSavingProductsPage(pageData, lang, dir))
		return
	}

	pageData := pages.CustomerSavingPageData{
		Items:        items,
		Stats:        stats,
		SearchQuery:  search,
		FilterStatus: filter,
		NoticeType:   noticeType,
		NoticeMsg:    noticeMsg,
		Pagination:   paginationProps,
	}
	h.renderPage(ctx, w, "render customer saving products", pages.CustomerSavingProductsPage(pageData, lang, dir))
}

// handleSavingProductCreateSubmit handles manual creation of a saving product for customer or vendor.
func (h *UIHandler) handleSavingProductCreateSubmit(w http.ResponseWriter, r *http.Request, audience string) {
	ctx := r.Context()
	targetURL := fmt.Sprintf("/%s/saving-products", audience)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, fmt.Sprintf("/auth/login?redirect=%s", targetURL), http.StatusSeeOther)
		return
	}

	var formInvalidMsg string
	if audience == "vendor" {
		formInvalidMsg = i18n.T(langOf(r), "common.form_invalid")
	} else {
		formInvalidMsg = i18n.T(langOf(r), "customer.saving.form_invalid")
	}

	if err := r.ParseForm(); err != nil {
		h.redirectWithNotice(w, r, targetURL, "error", formInvalidMsg)
		return
	}

	nameProduct := strings.TrimSpace(r.FormValue("name_product"))
	if nameProduct == "" {
		h.redirectWithNotice(w, r, targetURL, "error", i18n.T(langOf(r), "customer.saving.name_required"))
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
			h.log.ErrorContext(ctx, fmt.Sprintf("create %s saving product error", audience), "error", err)
			var errMsg string
			if audience == "customer" {
				errMsg = fmt.Sprintf(i18n.T(langOf(r), "customer.saving.create_error"), h.safeMessage(err, langOf(r)))
			} else {
				errMsg = h.safeMessage(err, langOf(r))
			}
			h.redirectWithNotice(w, r, targetURL, "error", errMsg)
			return
		}
	}

	var successMsg string
	if audience == "vendor" {
		successMsg = i18n.T(langOf(r), "vendor.saving.create_success")
	} else {
		successMsg = i18n.T(langOf(r), "customer.saving.create_success")
	}
	h.redirectWithNotice(w, r, targetURL, "success", successMsg)
}

// handleSavingProductUpdateSubmit handles updating an existing saving product for customer or vendor.
func (h *UIHandler) handleSavingProductUpdateSubmit(w http.ResponseWriter, r *http.Request, audience string) {
	ctx := r.Context()
	targetURL := fmt.Sprintf("/%s/saving-products", audience)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, fmt.Sprintf("/auth/login?redirect=%s", targetURL), http.StatusSeeOther)
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		var idErrMsg string
		if audience == "vendor" {
			idErrMsg = i18n.T(langOf(r), "vendor.saving.invalid_product_id")
		} else {
			idErrMsg = i18n.T(langOf(r), "customer.saving.invalid_id")
		}
		h.redirectWithNotice(w, r, targetURL, "error", idErrMsg)
		return
	}

	if err := r.ParseForm(); err != nil {
		var formErrMsg string
		if audience == "vendor" {
			formErrMsg = i18n.T(langOf(r), "common.form_invalid")
		} else {
			formErrMsg = i18n.T(langOf(r), "customer.saving.form_invalid")
		}
		h.redirectWithNotice(w, r, targetURL, "error", formErrMsg)
		return
	}

	nameProduct := strings.TrimSpace(r.FormValue("name_product"))
	if nameProduct == "" {
		h.redirectWithNotice(w, r, targetURL, "error", i18n.T(langOf(r), "customer.saving.name_required"))
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
			h.log.ErrorContext(ctx, fmt.Sprintf("update %s saving product error", audience), "error", err)
			var errMsg string
			if audience == "customer" {
				errMsg = fmt.Sprintf(i18n.T(langOf(r), "customer.saving.update_error"), h.safeMessage(err, langOf(r)))
			} else {
				errMsg = h.safeMessage(err, langOf(r))
			}
			h.redirectWithNotice(w, r, targetURL, "error", errMsg)
			return
		}
	}

	var successMsg string
	if audience == "vendor" {
		successMsg = i18n.T(langOf(r), "vendor.saving.update_success")
	} else {
		successMsg = i18n.T(langOf(r), "customer.saving.update_success")
	}
	h.redirectWithNotice(w, r, targetURL, "success", successMsg)
}

// handleSavingProductDeleteSubmit deletes a saving product record for customer or vendor.
func (h *UIHandler) handleSavingProductDeleteSubmit(w http.ResponseWriter, r *http.Request, audience string) {
	ctx := r.Context()
	targetURL := fmt.Sprintf("/%s/saving-products", audience)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, fmt.Sprintf("/auth/login?redirect=%s", targetURL), http.StatusSeeOther)
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		var idErrMsg string
		if audience == "vendor" {
			idErrMsg = i18n.T(langOf(r), "vendor.saving.invalid_product_id")
		} else {
			idErrMsg = i18n.T(langOf(r), "customer.saving.invalid_id")
		}
		h.redirectWithNotice(w, r, targetURL, "error", idErrMsg)
		return
	}

	if h.catSvc != nil {
		if err := h.catSvc.DeleteSavingProduct(ctx, id, actor.OrganizationID); err != nil {
			h.log.ErrorContext(ctx, fmt.Sprintf("delete %s saving product error", audience), "error", err)
			var errMsg string
			if audience == "customer" {
				errMsg = i18n.T(langOf(r), "customer.saving.delete_error")
			} else {
				errMsg = h.safeMessage(err, langOf(r))
			}
			h.redirectWithNotice(w, r, targetURL, "error", errMsg)
			return
		}
	}

	var successMsg string
	if audience == "vendor" {
		successMsg = i18n.T(langOf(r), "vendor.saving.delete_success")
	} else {
		successMsg = i18n.T(langOf(r), "customer.saving.delete_success")
	}
	h.redirectWithNotice(w, r, targetURL, "success", successMsg)
}

// handleSavingProductDeleteAllSubmit deletes all saving products for the organization.
func (h *UIHandler) handleSavingProductDeleteAllSubmit(w http.ResponseWriter, r *http.Request, audience string) {
	ctx := r.Context()
	targetURL := fmt.Sprintf("/%s/saving-products", audience)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, fmt.Sprintf("/auth/login?redirect=%s", targetURL), http.StatusSeeOther)
		return
	}

	if h.catSvc != nil {
		if err := h.catSvc.DeleteAllSavingProducts(ctx, actor.OrganizationID); err != nil {
			h.log.ErrorContext(ctx, fmt.Sprintf("delete all %s saving products error", audience), "error", err)
			h.redirectWithNotice(w, r, targetURL, "error", i18n.T(langOf(r), "customer.saving.delete_all_error"))
			return
		}
	}

	var successMsg string
	if audience == "vendor" {
		successMsg = i18n.T(langOf(r), "vendor.saving.delete_all_success")
	} else {
		successMsg = i18n.T(langOf(r), "customer.saving.delete_all_success")
	}
	h.redirectWithNotice(w, r, targetURL, "success", successMsg)
}

// handleSavingProductsExport streams an Excel spreadsheet of all saving products for customer or vendor.
func (h *UIHandler) handleSavingProductsExport(w http.ResponseWriter, r *http.Request, audience string) {
	ctx := r.Context()
	lang, _ := h.localeAndDir(r)
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, fmt.Sprintf("/auth/login?redirect=/%s/saving-products", audience), http.StatusSeeOther)
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

	var headers []string
	var filename string
	if audience == "vendor" {
		filename = fmt.Sprintf("saving_products_org_%d.xlsx", actor.OrganizationID)
		headers = []string{
			i18n.T(lang, "customer.saving.export_col_id"),
			i18n.T(lang, "vendor.saving.export_col_name"),
			i18n.T(lang, "customer.saving.export_col_sku"),
			i18n.T(lang, "customer.saving.export_col_qty"),
			i18n.T(lang, "vendor.saving.export_col_price"),
			i18n.T(lang, "customer.saving.export_col_total"),
			i18n.T(lang, "customer.saving.export_col_product_id"),
			i18n.T(lang, "customer.saving.export_col_linked_name"),
			i18n.T(lang, "vendor.saving.export_col_orgs"),
		}
	} else {
		filename = fmt.Sprintf("pharmacy_saving_products_%d.xlsx", actor.OrganizationID)
		headers = []string{
			i18n.T(lang, "customer.saving.export_col_id"),
			i18n.T(lang, "customer.saving.export_col_name"),
			i18n.T(lang, "customer.saving.export_col_sku"),
			i18n.T(lang, "customer.saving.export_col_qty"),
			i18n.T(lang, "customer.saving.export_col_price"),
			i18n.T(lang, "customer.saving.export_col_total"),
			i18n.T(lang, "customer.saving.export_col_product_id"),
			i18n.T(lang, "customer.saving.export_col_linked_name"),
			i18n.T(lang, "customer.saving.export_col_suppliers"),
		}
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
			_ = f.SetCellValue(sheet, fmt.Sprintf("H%d", rNum), it.LinkedProductName.Get(i18n.ParseLang(lang)))
			_ = f.SetCellValue(sheet, fmt.Sprintf("I%d", rNum), it.ProvidingOrgsCount)
		} else {
			_ = f.SetCellValue(sheet, fmt.Sprintf("G%d", rNum), "")
			_ = f.SetCellValue(sheet, fmt.Sprintf("H%d", rNum), i18n.T(lang, "customer.saving.not_linked"))
			_ = f.SetCellValue(sheet, fmt.Sprintf("I%d", rNum), 0)
		}
	}

	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	_ = f.Write(w)
}

// SavingProductProvidersJSON returns JSON array of suppliers selling a master product.
func (h *UIHandler) SavingProductProvidersJSON(w http.ResponseWriter, r *http.Request) {
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
			OrgName:            p.OrgName.Get(i18n.ParseLang(langOf(r))),
			VariantName:        p.VariantName.Get(i18n.ParseLang(langOf(r))),
			SKU:                p.SKU,
			Unit:               p.Unit,
			Price:              p.Price.String(),
			Discount:           p.Discount.String(),
			PriceAfterDiscount: p.PriceAfterDiscount.String(),
			StockQuantity:      p.StockQuantity,
			BranchName:         p.BranchName.Get(i18n.ParseLang(langOf(r))),
		})
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(res)
}

// SavingProductSearchJSON returns JSON autocomplete list of catalog products for linking.
func (h *UIHandler) SavingProductSearchJSON(w http.ResponseWriter, r *http.Request) {
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
			Query:     query,
			FirstWord: catalog.FirstWordOf(query),
			Limit:     20,
		})
		if err == nil {
			for _, p := range products {
				name := p.Name.Get(i18n.ParseLang(langOf(r)))
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

// Customer Saving Products Handlers (public delegates)

// CustomerSavingProductsPage renders the customer's live price-delta tracking list.
func (h *UIHandler) CustomerSavingProductsPage(w http.ResponseWriter, r *http.Request) {
	h.handleSavingProductsPage(w, r, "customer")
}

// CustomerSavingProductCreateSubmit handles manual creation of a pharmacy saving product.
func (h *UIHandler) CustomerSavingProductCreateSubmit(w http.ResponseWriter, r *http.Request) {
	h.handleSavingProductCreateSubmit(w, r, "customer")
}

// CustomerSavingProductUpdateSubmit handles updating an existing pharmacy saving product.
func (h *UIHandler) CustomerSavingProductUpdateSubmit(w http.ResponseWriter, r *http.Request) {
	h.handleSavingProductUpdateSubmit(w, r, "customer")
}

// CustomerSavingProductDeleteSubmit deletes a saving product record for the pharmacy.
func (h *UIHandler) CustomerSavingProductDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	h.handleSavingProductDeleteSubmit(w, r, "customer")
}

// CustomerSavingProductsDeleteAllSubmit deletes all saving products for the customer org.
func (h *UIHandler) CustomerSavingProductsDeleteAllSubmit(w http.ResponseWriter, r *http.Request) {
	h.handleSavingProductDeleteAllSubmit(w, r, "customer")
}

// CustomerSavingProductsExport streams an Excel spreadsheet of all saving products for the pharmacy.
func (h *UIHandler) CustomerSavingProductsExport(w http.ResponseWriter, r *http.Request) {
	h.handleSavingProductsExport(w, r, "customer")
}

// CustomerSavingProductProvidersJSON delegates to SavingProductProvidersJSON logic.
func (h *UIHandler) CustomerSavingProductProvidersJSON(w http.ResponseWriter, r *http.Request) {
	h.SavingProductProvidersJSON(w, r)
}

// CustomerSavingProductSearchJSON delegates to SavingProductSearchJSON logic.
func (h *UIHandler) CustomerSavingProductSearchJSON(w http.ResponseWriter, r *http.Request) {
	h.SavingProductSearchJSON(w, r)
}

// CustomerSavingProductDetailPage renders single saving product delta details.
func (h *UIHandler) CustomerSavingProductDetailPage(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/customer/saving-products", http.StatusSeeOther)
}

// CustomerSavingProductsAlias redirects misspelled route /customer/saveing-products.
func (h *UIHandler) CustomerSavingProductsAlias(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/customer/saving-products", http.StatusMovedPermanently)
}

// Vendor Saving Products Handlers (public delegates)

// VendorSavingProductsPage renders the vendor's saving products directory.
func (h *UIHandler) VendorSavingProductsPage(w http.ResponseWriter, r *http.Request) {
	h.handleSavingProductsPage(w, r, "vendor")
}

// VendorSavingProductCreateSubmit handles manual creation of a saving product for vendor.
func (h *UIHandler) VendorSavingProductCreateSubmit(w http.ResponseWriter, r *http.Request) {
	h.handleSavingProductCreateSubmit(w, r, "vendor")
}

// VendorSavingProductUpdateSubmit handles updating an existing saving product for vendor.
func (h *UIHandler) VendorSavingProductUpdateSubmit(w http.ResponseWriter, r *http.Request) {
	h.handleSavingProductUpdateSubmit(w, r, "vendor")
}

// VendorSavingProductDeleteSubmit deletes a saving product record for vendor.
func (h *UIHandler) VendorSavingProductDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	h.handleSavingProductDeleteSubmit(w, r, "vendor")
}

// VendorSavingProductsDeleteAllSubmit deletes all saving products for the vendor org.
func (h *UIHandler) VendorSavingProductsDeleteAllSubmit(w http.ResponseWriter, r *http.Request) {
	h.handleSavingProductDeleteAllSubmit(w, r, "vendor")
}

// VendorSavingProductsExport streams an Excel spreadsheet of all saving products for vendor.
func (h *UIHandler) VendorSavingProductsExport(w http.ResponseWriter, r *http.Request) {
	h.handleSavingProductsExport(w, r, "vendor")
}

// VendorSavingProductProvidersJSON delegates to SavingProductProvidersJSON logic.
func (h *UIHandler) VendorSavingProductProvidersJSON(w http.ResponseWriter, r *http.Request) {
	h.SavingProductProvidersJSON(w, r)
}

// VendorSavingProductSearchJSON delegates to SavingProductSearchJSON logic.
func (h *UIHandler) VendorSavingProductSearchJSON(w http.ResponseWriter, r *http.Request) {
	h.SavingProductSearchJSON(w, r)
}

// VendorSavingProductsAlias redirects misspelled route /vendor/saveing-products.
func (h *UIHandler) VendorSavingProductsAlias(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/vendor/saving-products", http.StatusMovedPermanently)
}
