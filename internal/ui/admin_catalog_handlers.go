package ui

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/modules/inventory"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui/components"
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
	var variants []*catalog.ProductVariant
	if h.catSvc != nil {
		prod, variants, _ = h.catSvc.GetProduct(database.AsSystem(ctx), prodID)
	}

	if prod == nil {
		h.redirectWithNotice(w, r, "/admin/products", "error", i18n.T(lang, "admin.products.not_found"))
		return
	}

	h.renderPage(ctx, w, "render admin product detail", pages.AdminProductDetailPage(prod, variants, lang, dir))
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

	h.renderPage(ctx, w, "render product children", pages.AdminProductChildrenPage(data, lang, dir))
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
	h.redirectWithNotice(w, r, "/admin/product-child", "success", i18n.T(langOf(r), "admin.catalog.variant_status_updated_success"))
}

// AdminStocksPage renders inventory stocks across all warehouses.
func (h *UIHandler) AdminStocksPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	var stocks []*inventory.Stock
	if h.invSvc != nil {
		stocks, _ = h.invSvc.ListLowStock(database.AsSystem(ctx), 100, 0)
	}

	h.renderPage(ctx, w, "render admin stocks page", pages.AdminStocksPage(stocks, lang, dir))
}

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

	limit := h.pageLimit(r)
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	offset := (page - 1) * limit

	if h.catSvc != nil {
		var err error
		items, stats, err = h.catSvc.ListAllSavingProductsAdmin(database.AsSystem(ctx), userID, orgID, search, filter, limit, offset)
		if err != nil {
			h.log.ErrorContext(ctx, "admin list saving products", "error", err)
		}
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
		Pagination: components.PaginationProps{
			CurrentPage: page,
			PageSize:    limit,
			TotalCount:  stats.TotalProducts,
			BaseURL:     "/admin/saving-products",
			QueryValues: r.URL.Query(),
		},
	}

	h.renderPage(ctx, w, "render saving products", pages.AdminSavingProductsPage(data, lang, dir))
}

// AdminProductsDeleteAllSubmit removes all master products and variants (Super Admin).
func (h *UIHandler) AdminProductsDeleteAllSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	if h.catSvc == nil {
		h.redirectWithNotice(w, r, "/admin/products", "error", i18n.T(lang, "admin.import.service_unavailable"))
		return
	}
	count, err := h.catSvc.DeleteAllProducts(database.AsSystem(ctx))
	if err != nil {
		h.log.ErrorContext(ctx, "delete all master products error", "error", err)
		h.redirectWithNotice(w, r, "/admin/products", "error", fmt.Sprintf(i18n.T(lang, "admin.catalog.delete_all_failed_format"), h.safeMessage(err, lang)))
		return
	}
	h.redirectWithNotice(w, r, "/admin/products", "success", fmt.Sprintf(i18n.T(lang, "admin.catalog.deleted_all_success_format"), count))
}
