package ui

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/inventory"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// AdminWarehousesPage renders warehouse registry for fulfillment network.
func (h *UIHandler) AdminWarehousesPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	var warehouses []*inventory.Warehouse
	if h.invSvc != nil {
		warehouses, _ = h.invSvc.ListWarehouses(database.AsSystem(ctx))
	}

	var orgs []*org.Organization
	if h.orgSvc != nil {
		orgs, _ = h.orgSvc.ListOrganizations(database.AsSystem(ctx), nil, nil, 500, 0)
	}
	orgMap := make(map[int64]string)
	for _, o := range orgs {
		if o != nil {
			orgMap[o.ID] = o.LegalName
		}
	}

	var rows []*pages.AdminWarehouseRowView
	for _, wh := range warehouses {
		if wh != nil {
			rows = append(rows, &pages.AdminWarehouseRowView{
				Warehouse: wh,
				OrgName:   orgMap[wh.OrganizationID],
			})
		}
	}

	h.renderPage(ctx, w, "render admin warehouses page", pages.AdminWarehousesPage(rows, lang, dir))
}

// AdminWarehouseDetailPage renders full warehouse detail and searchable/paginated stocks page.
func (h *UIHandler) AdminWarehouseDetailPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	idStr := chi.URLParam(r, "id")
	whID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || whID <= 0 {
		http.Redirect(w, r, "/admin/warehouses", http.StatusSeeOther)
		return
	}

	var wh *inventory.Warehouse
	if h.invSvc != nil {
		whs, _ := h.invSvc.ListWarehouses(database.AsSystem(ctx))
		for _, item := range whs {
			if item != nil && item.ID == whID {
				wh = item
				break
			}
		}
	}

	if wh == nil {
		http.Redirect(w, r, "/admin/warehouses", http.StatusSeeOther)
		return
	}

	var orgName string
	if h.orgSvc != nil && wh.OrganizationID > 0 {
		if o, err := h.orgSvc.GetOrganization(database.AsSystem(ctx), wh.OrganizationID); err == nil && o != nil {
			orgName = o.LegalName
		}
	}

	var allStocks []*inventory.DetailedWarehouseStockView
	if h.invSvc != nil {
		allStocks, _ = h.invSvc.ListDetailedStocksByWarehouse(database.AsSystem(ctx), whID)
	}
	if allStocks == nil {
		allStocks = []*inventory.DetailedWarehouseStockView{}
	}

	// Calculate overall stats before filtering
	totalUnits := 0
	availableCount := 0
	lowStockCount := 0
	outOfStockCount := 0

	for _, s := range allStocks {
		if s == nil {
			continue
		}
		totalUnits += s.Quantity
		if s.Quantity > s.MinThreshold {
			availableCount++
		} else if s.Quantity > 0 {
			lowStockCount++
		} else {
			outOfStockCount++
		}
	}

	// Parse query filters
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	qLower := strings.ToLower(q)
	statusFilter := strings.TrimSpace(r.URL.Query().Get("status"))
	if statusFilter == "" {
		statusFilter = "all"
	}
	negotiableFilter := strings.TrimSpace(r.URL.Query().Get("negotiable"))
	if negotiableFilter == "" {
		negotiableFilter = "all"
	}

	var filtered []*inventory.DetailedWarehouseStockView
	for _, s := range allStocks {
		if s == nil {
			continue
		}
		// Search query filter
		if qLower != "" {
			match := strings.Contains(strings.ToLower(s.ProductName), qLower) ||
				strings.Contains(strings.ToLower(s.VariantName), qLower) ||
				strings.Contains(strings.ToLower(s.SKU), qLower) ||
				strings.Contains(strings.ToLower(s.Barcode), qLower) ||
				strings.Contains(strings.ToLower(s.BatchNumber), qLower)
			if !match {
				continue
			}
		}

		// Status filter
		switch statusFilter {
		case "available":
			if s.Quantity <= s.MinThreshold {
				continue
			}
		case "low":
			if s.Quantity <= 0 || s.Quantity > s.MinThreshold {
				continue
			}
		case "out":
			if s.Quantity > 0 {
				continue
			}
		}

		// Negotiable filter
		switch negotiableFilter {
		case "yes":
			if !s.IsNegotiable {
				continue
			}
		case "no":
			if s.IsNegotiable {
				continue
			}
		}

		filtered = append(filtered, s)
	}

	// Pagination
	page := 1
	if pStr := r.URL.Query().Get("page"); pStr != "" {
		if p, err := strconv.Atoi(pStr); err == nil && p > 0 {
			page = p
		}
	}

	limit := 25
	if lStr := r.URL.Query().Get("limit"); lStr != "" {
		if l, err := strconv.Atoi(lStr); err == nil && l > 0 {
			if l == 25 || l == 50 || l == 100 || l == 250 {
				limit = l
			}
		}
	}

	totalFiltered := len(filtered)
	totalPages := (totalFiltered + limit - 1) / limit
	if totalPages < 1 {
		totalPages = 1
	}
	if page > totalPages {
		page = totalPages
	}

	offset := (page - 1) * limit
	var paginatedItems []*inventory.DetailedWarehouseStockView
	if offset < totalFiltered {
		end := offset + limit
		if end > totalFiltered {
			end = totalFiltered
		}
		paginatedItems = filtered[offset:end]
	}

	data := pages.AdminWarehouseDetailView{
		Warehouse:        wh,
		OrgName:          orgName,
		Items:            paginatedItems,
		TotalItems:       len(allStocks),
		TotalUnits:       totalUnits,
		AvailableCount:   availableCount,
		LowStockCount:    lowStockCount,
		OutOfStockCount:  outOfStockCount,
		SearchQuery:      q,
		StatusFilter:     statusFilter,
		NegotiableFilter: negotiableFilter,
		CurrentPage:      page,
		PageSize:         limit,
		TotalCount:       totalFiltered,
		QueryValues:      r.URL.Query(),
	}

	h.renderPage(ctx, w, "render admin warehouse detail page", pages.AdminWarehouseDetailPage(data, lang, dir))
}

// AdminWarehouseStocksJSON provides detailed stock rows for interactive inspection.
func (h *UIHandler) AdminWarehouseStocksJSON(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	idStr := chi.URLParam(r, "id")
	whID, _ := strconv.ParseInt(idStr, 10, 64)

	var stocks []*inventory.DetailedWarehouseStockView
	if h.invSvc != nil && whID > 0 {
		stocks, _ = h.invSvc.ListDetailedStocksByWarehouse(database.AsSystem(ctx), whID)
	}

	if stocks == nil {
		stocks = []*inventory.DetailedWarehouseStockView{}
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(stocks)
}

type tempWarehouseUploadResult struct {
	ID           int64  `json:"id"`
	Filename     string `json:"filename"`
	SupplierName string `json:"supplier_name"`
	RowCount     int    `json:"row_count"`
	Success      bool   `json:"success"`
	Error        string `json:"error,omitempty"`
}
