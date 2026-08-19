package ui

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/modules/inventory"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// AdminProductDetailPage renders detail view of a master catalog product.
func (h *UIHandler) AdminProductDetailPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	idStr := chi.URLParam(r, "id")
	prodID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || prodID <= 0 {
		http.Redirect(w, r, "/admin/products", http.StatusSeeOther)
		return
	}

	var prod *catalog.Product
	if h.catSvc != nil {
		prod, _, _ = h.catSvc.GetProduct(database.AsSystem(ctx), prodID)
	}

	if prod == nil {
		h.redirectWithNotice(w, r, "/admin/products", "error", "المنتج غير موجود.")
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminProductDetailPage(prod, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin product detail", "error", err)
	}
}

// AdminProductChildrenPage renders vendor-level variant listings and branch offers.
func (h *UIHandler) AdminProductChildrenPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	var prods []*catalog.Product
	if h.catSvc != nil {
		// AsSystem justified: platform admin browsing master catalog listings
		prods, _ = h.catSvc.ListProducts(database.AsSystem(ctx), "", 100, 0)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminProductChildrenPage(prods, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render product children", "error", err)
	}
}

// AdminAdvProductsPage renders advanced spreadsheet column mapping uploader.
func (h *UIHandler) AdminAdvProductsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = pages.AdminAdvProductsPage(lang, dir).Render(ctx, w)
}

// AdminApisProductsPage renders external API inventory connector.
func (h *UIHandler) AdminApisProductsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = pages.AdminApisProductsPage(lang, dir).Render(ctx, w)
}

// AdminStocksPage renders inventory stocks across all warehouses.
func (h *UIHandler) AdminStocksPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	var stocks []*inventory.Stock
	if h.invSvc != nil {
		// AsSystem justified: platform admin inspecting aggregate stocks
		stocks, _ = h.invSvc.ListStocksByWarehouse(database.AsSystem(ctx), 0)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminStocksPage(stocks, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin stocks", "error", err)
	}
}

// AdminWarehousesPage renders permanent warehouses list.
func (h *UIHandler) AdminWarehousesPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	var warehouses []*inventory.Warehouse
	if h.invSvc != nil {
		// AsSystem justified: platform admin inspecting all warehouses
		warehouses, _ = h.invSvc.ListWarehouses(database.AsSystem(ctx))
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminWarehousesPage(warehouses, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin warehouses", "error", err)
	}
}

// AdminWarehouseDetailPage renders warehouse details and stock levels.
func (h *UIHandler) AdminWarehouseDetailPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	idStr := chi.URLParam(r, "id")
	whID, _ := strconv.ParseInt(idStr, 10, 64)

	var wh *inventory.Warehouse
	var stocks []*inventory.Stock
	if h.invSvc != nil && whID > 0 {
		whs, _ := h.invSvc.ListWarehouses(database.AsSystem(ctx))
		for _, wCandidate := range whs {
			if wCandidate.ID == whID {
				wh = wCandidate
				break
			}
		}
		stocks, _ = h.invSvc.ListStocksByWarehouse(database.AsSystem(ctx), whID)
	}

	if wh == nil {
		http.Redirect(w, r, "/admin/warehouses", http.StatusSeeOther)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = pages.AdminWarehouseDetailPage(wh, stocks, lang, dir).Render(ctx, w)
}

// AdminTempWarehousesPage renders temporary warehouses staging directory.
func (h *UIHandler) AdminTempWarehousesPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminTempWarehousesPage(nil, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render temp warehouses", "error", err)
	}
}

// AdminSavingProductsPage renders saving products (منتجات التوفير).
func (h *UIHandler) AdminSavingProductsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminSavingProductsPage(nil, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render saving products", "error", err)
	}
}
