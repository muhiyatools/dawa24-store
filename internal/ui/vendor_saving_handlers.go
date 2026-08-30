package ui

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/ui/components"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// VendorSavingProductsPage renders the vendor's saving products directory.
func (h *UIHandler) VendorSavingProductsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/saving-products", http.StatusSeeOther)
		return
	}

	search := strings.TrimSpace(r.URL.Query().Get("search"))
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
			h.log.ErrorContext(ctx, "list saving products enriched error", "error", err, "org_id", actor.OrganizationID)
		}
	}

	if stats == nil {
		stats = &catalog.SavingProductStats{}
	}

	noticeType := r.URL.Query().Get("notice_type")
	noticeMsg := r.URL.Query().Get("notice")

	pageData := pages.VendorSavingPageData{
		Items:        items,
		Stats:        stats,
		SearchQuery:  search,
		FilterStatus: filter,
		NoticeType:   noticeType,
		NoticeMsg:    noticeMsg,
		Pagination: components.PaginationProps{
			CurrentPage: page,
			PageSize:    limit,
			TotalCount:  stats.CountAll,
			BaseURL:     "/vendor/saving-products",
			QueryValues: r.URL.Query(),
		},
	}

	h.renderPage(ctx, w, "render vendor saving products", pages.VendorSavingProductsPage(pageData, lang, dir))
}

// VendorSavingProductCreateSubmit handles manual creation of a saving product.
func (h *UIHandler) VendorSavingProductCreateSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/saving-products", http.StatusSeeOther)
		return
	}

	if err := r.ParseForm(); err != nil {
		h.redirectWithNotice(w, r, "/vendor/saving-products", "error", i18n.T(langOf(r), "common.form_invalid"))
		return
	}

	nameProduct := strings.TrimSpace(r.FormValue("name_product"))
	if nameProduct == "" {
		h.redirectWithNotice(w, r, "/vendor/saving-products", "error", i18n.T(langOf(r), "customer.saving.name_required"))
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
			h.redirectWithNotice(w, r, "/vendor/saving-products", "error", h.safeMessage(err, langOf(r)))
			return
		}
	}

	h.redirectWithNotice(w, r, "/vendor/saving-products", "success", i18n.T(langOf(r), "vendor.saving.create_success"))
}

// VendorSavingProductUpdateSubmit handles updating an existing saving product.
func (h *UIHandler) VendorSavingProductUpdateSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/saving-products", http.StatusSeeOther)
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/vendor/saving-products", "error", i18n.T(langOf(r), "vendor.saving.invalid_product_id"))
		return
	}

	if err := r.ParseForm(); err != nil {
		h.redirectWithNotice(w, r, "/vendor/saving-products", "error", i18n.T(langOf(r), "common.form_invalid"))
		return
	}

	nameProduct := strings.TrimSpace(r.FormValue("name_product"))
	if nameProduct == "" {
		h.redirectWithNotice(w, r, "/vendor/saving-products", "error", i18n.T(langOf(r), "customer.saving.name_required"))
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
			h.redirectWithNotice(w, r, "/vendor/saving-products", "error", h.safeMessage(err, langOf(r)))
			return
		}
	}

	h.redirectWithNotice(w, r, "/vendor/saving-products", "success", i18n.T(langOf(r), "vendor.saving.update_success"))
}

// VendorSavingProductDeleteSubmit deletes a saving product record.
func (h *UIHandler) VendorSavingProductDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/saving-products", http.StatusSeeOther)
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/vendor/saving-products", "error", i18n.T(langOf(r), "vendor.saving.invalid_product_id"))
		return
	}

	if h.catSvc != nil {
		if err := h.catSvc.DeleteSavingProduct(ctx, id, actor.OrganizationID); err != nil {
			h.redirectWithNotice(w, r, "/vendor/saving-products", "error", h.safeMessage(err, langOf(r)))
			return
		}
	}

	h.redirectWithNotice(w, r, "/vendor/saving-products", "success", i18n.T(langOf(r), "vendor.saving.delete_success"))
}

// VendorSavingProductsDeleteAllSubmit deletes all saving products for the vendor org.
func (h *UIHandler) VendorSavingProductsDeleteAllSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/saving-products", http.StatusSeeOther)
		return
	}

	if h.catSvc != nil {
		if err := h.catSvc.DeleteAllSavingProducts(ctx, actor.OrganizationID); err != nil {
			h.log.ErrorContext(ctx, "delete all vendor saving products error", "error", err)
			h.redirectWithNotice(w, r, "/vendor/saving-products", "error", i18n.T(langOf(r), "customer.saving.delete_all_error"))
			return
		}
	}

	h.redirectWithNotice(w, r, "/vendor/saving-products", "success", i18n.T(langOf(r), "vendor.saving.delete_all_success"))
}

// VendorSavingProductsAlias redirects misspelled route /vendor/saveing-products.
func (h *UIHandler) VendorSavingProductsAlias(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/vendor/saving-products", http.StatusMovedPermanently)
}
