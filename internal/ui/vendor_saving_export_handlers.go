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
)

// VendorSavingProductsExport streams an Excel spreadsheet of all saving products.
func (h *UIHandler) VendorSavingProductsExport(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, _ := h.localeAndDir(r)
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
