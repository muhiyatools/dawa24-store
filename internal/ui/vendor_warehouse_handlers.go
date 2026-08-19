package ui

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/inventory"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// VendorWarehousesPage renders the vendor's permanent and branch storage warehouses.
func (h *UIHandler) VendorWarehousesPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/warehouses", http.StatusSeeOther)
		return
	}

	var warehouses []*inventory.Warehouse
	if h.invSvc != nil {
		warehouses, _ = h.invSvc.ListWarehouses(ctx)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.VendorWarehousesPage(warehouses, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render vendor warehouses page", "error", err)
	}
}

// VendorWarehouseDetailPage renders single warehouse details and current stock rows.
func (h *UIHandler) VendorWarehouseDetailPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/warehouses", http.StatusSeeOther)
		return
	}

	idStr := chi.URLParam(r, "id")
	whID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || whID <= 0 {
		http.Redirect(w, r, "/vendor/warehouses", http.StatusSeeOther)
		return
	}

	var wh *inventory.Warehouse
	var stocks []*inventory.Stock
	if h.invSvc != nil {
		whs, _ := h.invSvc.ListWarehouses(ctx)
		for _, item := range whs {
			if item.ID == whID {
				wh = item
				break
			}
		}
		stocks, _ = h.invSvc.ListStocksByWarehouse(ctx, whID)
	}

	if wh == nil {
		http.Redirect(w, r, "/vendor/warehouses", http.StatusSeeOther)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.VendorWarehouseDetailPage(wh, stocks, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render vendor warehouse detail", "error", err)
	}
}
