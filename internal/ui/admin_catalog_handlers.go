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
	"github.com/muhiya/dawa24-store/internal/shared/pagination"
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

	page := pagination.PageNumber(r)
	limit := pagination.RowsPerPage(r)
	offset := (page - 1) * limit

	data := pages.AdminProductChildrenData{
		SearchQuery:  search,
		StatusFilter: status,
		Page:         page,
		PerPage:      limit,
	}

	sysCtx := database.AsSystem(ctx)

	if h.catSvc != nil {
		params := catalog.VariantSearchParams{
			Query:  search,
			Status: status,
			Limit:  limit,
			Offset: offset,
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

	limit := pagination.RowsPerPage(r)
	page := pagination.PageNumber(r)
	offset := (page - 1) * limit

	var stocks []*inventory.Stock
	var total int
	if h.invSvc != nil {
		stocks, total, _ = h.invSvc.ListLowStockWithTotal(database.AsSystem(ctx), limit, offset)
	}

	h.renderPage(ctx, w, "render admin stocks page", pages.AdminStocksPage(stocks, lang, dir, page, limit, total))
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
