package ui

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/modules/inventory"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/pagination"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// VendorInventoryPage renders the inventory stock view with search, warehouse filtering, and pagination.
func (h *UIHandler) VendorInventoryPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/inventory", http.StatusSeeOther)
		return
	}

	page := pagination.PageNumber(r)
	limit := pagination.RowsPerPage(r)
	offset := (page - 1) * limit

	q := strings.TrimSpace(r.URL.Query().Get("q"))
	whID, _ := strconv.ParseInt(r.URL.Query().Get("warehouse_id"), 10, 64)

	var pagedStocks []*inventory.Stock
	var total int
	var warehouses []*inventory.Warehouse
	if h.invSvc != nil {
		pagedStocks, total, _ = h.invSvc.ListStocksByOrgWithTotal(ctx, actor.OrganizationID, whID, q, limit, offset)
		warehouses, _ = h.invSvc.ListWarehouses(ctx)
	}

	var variants []*catalog.ProductVariant
	if h.catSvc != nil && len(pagedStocks) > 0 {
		var productIDs []int64
		for _, s := range pagedStocks {
			if s != nil {
				productIDs = append(productIDs, s.ProductID)
			}
		}
		if len(productIDs) > 0 {
			vMap, _ := h.catSvc.ListVariantsByProducts(ctx, productIDs)
			for _, vList := range vMap {
				variants = append(variants, vList...)
			}
		}
	}

	totalPages := (total + limit - 1) / limit
	if totalPages < 1 {
		totalPages = 1
	}

	data := pages.VendorInventoryData{
		Stocks:      pagedStocks,
		Warehouses:  warehouses,
		Variants:    variants,
		Total:       total,
		Page:        page,
		PerPage:     limit,
		TotalPages:  totalPages,
		Query:       q,
		WarehouseID: whID,
		NoticeType:  r.URL.Query().Get("notice_type"),
		NoticeMsg:   r.URL.Query().Get("notice"),
	}

	h.renderPage(ctx, w, "render vendor inventory page", pages.VendorInventory(data, lang, dir, h.isHTMX(r)))
}

// VendorTransfersPage renders warehouse inventory transfers.
func (h *UIHandler) VendorTransfersPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	if _, ok := database.TenantFrom(ctx); !ok {
		if actor, ok := authctx.From(ctx); ok && actor.OrganizationID > 0 {
			ctx = database.WithTenant(ctx, actor.OrganizationID)
		}
	}

	page := pagination.PageNumber(r)
	limit := pagination.RowsPerPage(r)
	offset := (page - 1) * limit

	if h.invSvc == nil {
		h.renderPage(ctx, w, "render vendor transfers fallback", pages.VendorTransfers(pages.VendorTransfersData{}, lang, dir, h.isHTMX(r)))
		return
	}

	transfers, totalCount, err := h.invSvc.ListTransfersWithTotal(ctx, "", limit, offset)
	if err != nil {
		h.renderError(w, r, err)
		return
	}

	data := pages.VendorTransfersData{
		Transfers:  transfers,
		Page:       page,
		PerPage:    limit,
		TotalCount: totalCount,
	}

	h.renderPage(ctx, w, "render vendor transfers page", pages.VendorTransfers(data, lang, dir, h.isHTMX(r)))
}

// VendorStockAdjustSubmit adjusts a stock level with a reason.
func (h *UIHandler) VendorStockAdjustSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/inventory", http.StatusSeeOther)
		return
	}

	stockID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	delta, _ := strconv.Atoi(r.PostFormValue("delta"))
	if h.invSvc != nil && stockID > 0 && delta != 0 {
		_, _ = h.invSvc.AdjustStock(ctx, inventory.AdjustStockInput{
			StockID: stockID,
			Delta:   delta,
			Type:    inventory.MovementAdjustment,
			Details: r.PostFormValue("reason"),
			UserID:  &actor.UserID,
		})
	}
	http.Redirect(w, r, "/vendor/inventory", http.StatusSeeOther)
}

// recordInitialStock writes a variant's opening quantity into inventory.stocks.
//
// inventory.stocks.warehouse_id is NOT NULL, so this needs somewhere to put it.
// A supplier with no warehouse yet gets a clear message rather than a number
// that disappears.
func (h *UIHandler) recordInitialStock(ctx context.Context, orgID int64, v *catalog.ProductVariant, qty int) error {
	if h.invSvc == nil {
		return fmt.Errorf("inventory service unavailable")
	}
	warehouses, err := h.invSvc.ListWarehouses(ctx)
	if err != nil {
		return err
	}
	var warehouseID int64
	// Match warehouse associated with the variant's branch
	if v.BranchID != nil && *v.BranchID > 0 {
		for _, wh := range warehouses {
			if wh.OrganizationID == orgID && wh.BranchID != nil && *wh.BranchID == *v.BranchID && wh.IsActive {
				warehouseID = wh.ID
				break
			}
		}
	}
	// Fallback to any active warehouse of the vendor
	if warehouseID == 0 {
		for _, wh := range warehouses {
			if wh.OrganizationID == orgID && wh.IsActive {
				warehouseID = wh.ID
				break
			}
		}
	}
	// If no warehouse exists, auto-create a real warehouse linked to the vendor's branch
	if warehouseID == 0 {
		whName := i18n.T("ar", "vendor.ingest.main_warehouse")
		if v.BranchID != nil && h.orgSvc != nil {
			if b, err := h.orgSvc.GetBranch(ctx, *v.BranchID); err == nil && b != nil {
				whName = i18n.T("ar", "vendor.inventory.warehouse_prefix") + b.Name.Get(i18n.AR)
			}
		}
		newWh := &inventory.Warehouse{
			OrganizationID: orgID,
			BranchID:       v.BranchID,
			Name:           whName,
			Code:           "WH-MAIN",
			IsActive:       true,
		}
		createdWh, err := h.invSvc.CreateWarehouse(ctx, newWh)
		if err == nil && createdWh != nil {
			warehouseID = createdWh.ID
		}
	}
	if warehouseID == 0 {
		return fmt.Errorf("organization %d has no warehouse to hold stock", orgID)
	}
	return h.invSvc.SetStock(ctx, &inventory.Stock{
		OrganizationID:   orgID,
		WarehouseID:      warehouseID,
		ProductID:        v.ProductID,
		ProductVariantID: v.ID,
		Quantity:         qty,
	})
}
