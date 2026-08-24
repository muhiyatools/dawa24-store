package ui

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/modules/inventory"
	"github.com/muhiya/dawa24-store/internal/modules/org"
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

	search := strings.TrimSpace(r.URL.Query().Get("q"))
	status := strings.TrimSpace(r.URL.Query().Get("status"))

	data := pages.AdminProductChildrenData{
		SearchQuery:  search,
		StatusFilter: status,
	}

	sysCtx := database.AsSystem(ctx)

	if h.catSvc != nil {
		params := catalog.VariantSearchParams{
			Query:  search,
			Status: status,
			Limit:  100,
			Offset: 0,
		}
		variants, total, err := h.catSvc.ListAllVariants(sysCtx, params)
		if err == nil {
			data.Total = total

			orgNames := make(map[int64]string)
			if h.orgSvc != nil {
				if orgs, err := h.orgSvc.ListOrganizations(sysCtx, nil, nil, 500, 0); err == nil {
					for _, o := range orgs {
						if o != nil {
							name := o.LegalName
							if name == "" {
								name = o.TradeName.Get("ar")
							}
							orgNames[o.ID] = name
						}
					}
				}
			}

			prodNames := make(map[int64]string)
			prodImages := make(map[int64]string)
			if masterProds, err := h.catSvc.Search(sysCtx, catalog.SearchParams{Limit: 1000}); err == nil {
				for _, p := range masterProds {
					if p != nil {
						prodNames[p.ID] = p.Name.Get("ar")
						if p.Image != "" {
							prodImages[p.ID] = p.Image
						}
					}
				}
			}

			for _, v := range variants {
				if v != nil {
					img := strings.TrimSpace(v.Image)
					isParentImg := false
					if img == "" {
						if parentImg, ok := prodImages[v.ProductID]; ok && parentImg != "" {
							img = parentImg
							isParentImg = true
						}
					}
					hasImg := img != ""
					data.Items = append(data.Items, pages.VendorVariantItem{
						Variant:        v,
						DisplayImage:   img,
						IsParentImage:  isParentImg,
						OrgName:        orgNames[v.OrganizationID],
						ParentProdName: prodNames[v.ProductID],
						HasImage:       hasImg,
					})
				}
			}
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminProductChildrenPage(data, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render product children", "error", err)
	}
}

// AdminProductChildStatusSubmit updates the active status of a vendor product variant.
func (h *UIHandler) AdminProductChildStatusSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err == nil && id > 0 && h.catSvc != nil {
		sysCtx := database.AsSystem(ctx)
		variant, err := h.catSvc.GetVariant(sysCtx, id)
		if err == nil && variant != nil {
			newStatus := r.URL.Query().Get("status")
			if newStatus == "" {
				newStatus = r.PostFormValue("status")
			}
			if newStatus == "" {
				if variant.Status == catalog.StatusActive {
					newStatus = "inactive"
				} else {
					newStatus = "active"
				}
			}
			variant.Status = catalog.ProductStatus(newStatus)
			_, _ = h.catSvc.UpdateVariant(sysCtx, id, variant)
		}
	}
	h.redirectWithNotice(w, r, "/admin/product-child", "success", "تم تحديث حالة صنف المورد بنجاح.")
}

// AdminAdvProductsPage is the legacy advanced-uploader route.
//
// The import wizard now covers what it offered — column mapping, a strategy, and
// a review before writing — so the two screens have merged and this stays only
// so bookmarks and the sidebar links that point here keep working.
func (h *UIHandler) AdminAdvProductsPage(w http.ResponseWriter, r *http.Request) {
	h.AdminProductsImportPage(w, r)
}

// AdminStocksPage renders inventory stocks across all warehouses.
func (h *UIHandler) AdminStocksPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	var stocks []*inventory.Stock
	if h.invSvc != nil {
		stocks, _ = h.invSvc.ListLowStock(database.AsSystem(ctx), 100, 0)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminStocksPage(stocks, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin stocks page", "error", err)
	}
}

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

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminWarehousesPage(rows, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin warehouses page", "error", err)
	}
}

// AdminWarehouseDetailPage redirects to the main warehouses hub.
func (h *UIHandler) AdminWarehouseDetailPage(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/admin/warehouses", http.StatusMovedPermanently)
}

// AdminWarehouseStocksJSON provides detailed stock rows for interactive modal inspection.
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
// AdminSavingProductsPage renders saving products (منتجات التوفير) across all users and organizations.
func (h *UIHandler) AdminSavingProductsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	var userID *int64
	var orgID *int64
	var selectedUserID, selectedOrgID int64

	if uStr := chi.URLParam(r, "userId"); uStr != "" {
		if uid, err := strconv.ParseInt(uStr, 10, 64); err == nil && uid > 0 {
			userID = &uid
			selectedUserID = uid
		}
	}
	if oStr := chi.URLParam(r, "organizationId"); oStr != "" {
		if oid, err := strconv.ParseInt(oStr, 10, 64); err == nil && oid > 0 {
			orgID = &oid
			selectedOrgID = oid
		}
	}

	if qUID := r.URL.Query().Get("user_id"); qUID != "" {
		if uid, err := strconv.ParseInt(qUID, 10, 64); err == nil && uid > 0 {
			userID = &uid
			selectedUserID = uid
		}
	}
	if qOID := r.URL.Query().Get("org_id"); qOID != "" {
		if oid, err := strconv.ParseInt(qOID, 10, 64); err == nil && oid > 0 {
			orgID = &oid
			selectedOrgID = oid
		}
	}

	search := r.URL.Query().Get("q")
	filter := r.URL.Query().Get("filter")
	if filter == "" {
		filter = "all"
	}

	var items []*catalog.SavingProductAdminView
	var stats *catalog.SavingProductAdminStats
	if h.catSvc != nil {
		items, stats, _ = h.catSvc.ListAllSavingProductsAdmin(database.AsSystem(ctx), userID, orgID, search, filter, 500, 0)
	}
	if stats == nil {
		stats = &catalog.SavingProductAdminStats{}
	}

	var orgOptions []*pages.SavingUserOrgOption
	if h.orgSvc != nil {
		if orgs, err := h.orgSvc.ListOrganizations(database.AsSystem(ctx), nil, nil, 200, 0); err == nil {
			for _, o := range orgs {
				name := o.LegalName
				if name == "" {
					name = o.TradeName.Get("ar")
				}
				orgOptions = append(orgOptions, &pages.SavingUserOrgOption{
					ID:   o.ID,
					Name: name,
					Type: string(o.Type),
				})
			}
		}
	}

	var userOptions []*pages.SavingUserOrgOption
	if h.idSvc != nil {
		if users, err := h.idSvc.AdminListUsers(database.AsSystem(ctx), "", ""); err == nil {
			for _, u := range users {
				name := u.Name.Get("ar")
				if name == "" {
					name = u.Name.Get("en")
				}
				if name == "" {
					name = u.Email
				}
				userOptions = append(userOptions, &pages.SavingUserOrgOption{
					ID:   u.ID,
					Name: name,
					Type: u.Email,
				})
			}
		}
	}

	data := pages.AdminSavingProductsData{
		Items:          items,
		Stats:          stats,
		Organizations:  orgOptions,
		Users:          userOptions,
		SelectedOrgID:  selectedOrgID,
		SelectedUserID: selectedUserID,
		SearchQuery:    search,
		ActiveFilter:   filter,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminSavingProductsPage(data, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render saving products", "error", err)
	}
}
