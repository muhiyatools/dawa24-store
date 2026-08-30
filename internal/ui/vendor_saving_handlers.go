package ui

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/money"
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

	if h.catSvc != nil {
		it, st, err := h.catSvc.ListSavingProductsEnriched(ctx, actor.OrganizationID, search, filter, 500, 0)
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
		h.redirectWithNotice(w, r, "/vendor/saving-products", "error", "بيانات النموذج غير صالحة.")
		return
	}

	nameProduct := strings.TrimSpace(r.FormValue("name_product"))
	if nameProduct == "" {
		h.redirectWithNotice(w, r, "/vendor/saving-products", "error", "اسم المنتج مطلوب.")
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

	h.redirectWithNotice(w, r, "/vendor/saving-products", "success", "تمت إضافة منتج المساعدة بنجاح.")
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
		h.redirectWithNotice(w, r, "/vendor/saving-products", "error", "معرف منتج غير صالح.")
		return
	}

	if err := r.ParseForm(); err != nil {
		h.redirectWithNotice(w, r, "/vendor/saving-products", "error", "بيانات النموذج غير صالحة.")
		return
	}

	nameProduct := strings.TrimSpace(r.FormValue("name_product"))
	if nameProduct == "" {
		h.redirectWithNotice(w, r, "/vendor/saving-products", "error", "اسم المنتج مطلوب.")
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

	h.redirectWithNotice(w, r, "/vendor/saving-products", "success", "تم تحديث بيانات منتج المساعدة وتعيين الربط بنجاح.")
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
		h.redirectWithNotice(w, r, "/vendor/saving-products", "error", "معرف منتج غير صالح.")
		return
	}

	if h.catSvc != nil {
		if err := h.catSvc.DeleteSavingProduct(ctx, id, actor.OrganizationID); err != nil {
			h.redirectWithNotice(w, r, "/vendor/saving-products", "error", h.safeMessage(err, langOf(r)))
			return
		}
	}

	h.redirectWithNotice(w, r, "/vendor/saving-products", "success", "تم حذف المنتج من قائمة المساعدة بنجاح.")
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
			h.redirectWithNotice(w, r, "/vendor/saving-products", "error", "حدث خطأ أثناء حذف الأصناف.")
			return
		}
	}

	h.redirectWithNotice(w, r, "/vendor/saving-products", "success", "تم حذف جميع منتجات التوفير بنجاح.")
}

// VendorSavingProductsAlias redirects misspelled route /vendor/saveing-products.
func (h *UIHandler) VendorSavingProductsAlias(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/vendor/saving-products", http.StatusMovedPermanently)
}
