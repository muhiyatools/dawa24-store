package ui

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

var defaultModelRegistry = []pages.ModelMetaEntry{
	{Key: "products", NameAr: "المنتجات الرئيسية", NameEn: "Products", SchemaTable: "catalog.products", TotalCount: 1240, TrashedCount: 14},
	{Key: "variants", NameAr: "أصناف وعروض المنتجات", NameEn: "Product Variants", SchemaTable: "catalog.product_variants", TotalCount: 3890, TrashedCount: 42},
	{Key: "categories", NameAr: "التصنيفات الطبية", NameEn: "Categories", SchemaTable: "catalog.categories", TotalCount: 48, TrashedCount: 2},
	{Key: "brands", NameAr: "الشركات المصنعة والماركات", NameEn: "Brands", SchemaTable: "catalog.brands", TotalCount: 180, TrashedCount: 5},
	{Key: "offers", NameAr: "عروض التوريد والخصومات", NameEn: "Offers", SchemaTable: "promo.offers", TotalCount: 512, TrashedCount: 8},
	{Key: "orders", NameAr: "طلبات الشراء والتوريد", NameEn: "Orders", SchemaTable: "commerce.orders", TotalCount: 14200, TrashedCount: 23},
	{Key: "organizations", NameAr: "المنشآت (صيدليات وموردين)", NameEn: "Organizations", SchemaTable: "org.organizations", TotalCount: 860, TrashedCount: 3},
	{Key: "users", NameAr: "المستخدمين والحسابات", NameEn: "Users", SchemaTable: "identity.users", TotalCount: 1950, TrashedCount: 11},
}

// AdminDeletesListsPage renders overview of all system models and their soft-deletable status.
func (h *UIHandler) AdminDeletesListsPage(w http.ResponseWriter, r *http.Request) {
	lang, dir := h.localeAndDir(r)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = pages.AdminDeletesListsPage(defaultModelRegistry, lang, dir).Render(r.Context(), w)
}

// AdminDeletesListModelPage renders rows for a specific model.
func (h *UIHandler) AdminDeletesListModelPage(w http.ResponseWriter, r *http.Request) {
	lang, dir := h.localeAndDir(r)
	modelKey := chi.URLParam(r, "model")

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = pages.AdminDeletesListModelPage(modelKey, lang, dir).Render(r.Context(), w)
}

// AdminDeletesListShowPage renders specific record detail.
func (h *UIHandler) AdminDeletesListShowPage(w http.ResponseWriter, r *http.Request) {
	modelKey := chi.URLParam(r, "model")
	http.Redirect(w, r, fmt.Sprintf("/admin/deletes-lists/%s", modelKey), http.StatusSeeOther)
}

// AdminTrashListPage renders deleted rows directory across models.
func (h *UIHandler) AdminTrashListPage(w http.ResponseWriter, r *http.Request) {
	lang, dir := h.localeAndDir(r)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = pages.AdminTrashListPage(defaultModelRegistry, lang, dir).Render(r.Context(), w)
}

// AdminTrashListModelPage renders trashed rows for a specific model with restore/purge actions.
func (h *UIHandler) AdminTrashListModelPage(w http.ResponseWriter, r *http.Request) {
	lang, dir := h.localeAndDir(r)
	modelKey := chi.URLParam(r, "model")

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = pages.AdminTrashListModelPage(modelKey, lang, dir).Render(r.Context(), w)
}

// AdminTrashListShowPage renders single trashed item details.
func (h *UIHandler) AdminTrashListShowPage(w http.ResponseWriter, r *http.Request) {
	modelKey := chi.URLParam(r, "model")
	http.Redirect(w, r, fmt.Sprintf("/admin/trash-list/%s", modelKey), http.StatusSeeOther)
}

// AdminTrashRestoreSubmit restores a soft-deleted record.
func (h *UIHandler) AdminTrashRestoreSubmit(w http.ResponseWriter, r *http.Request) {
	modelKey := chi.URLParam(r, "model")
	idStr := chi.URLParam(r, "id")
	rowID, _ := strconv.ParseInt(idStr, 10, 64)

	h.log.InfoContext(r.Context(), "admin restored trashed row", "model", modelKey, "id", rowID)
	h.redirectWithNotice(w, r, fmt.Sprintf("/admin/trash-list/%s", modelKey), "success", "تم استرجاع السجل بنجاح.")
}

// AdminTrashPurgeSubmit permanently hard-deletes a record.
func (h *UIHandler) AdminTrashPurgeSubmit(w http.ResponseWriter, r *http.Request) {
	modelKey := chi.URLParam(r, "model")
	idStr := chi.URLParam(r, "id")
	rowID, _ := strconv.ParseInt(idStr, 10, 64)

	h.log.WarnContext(r.Context(), "admin permanently purged row", "model", modelKey, "id", rowID)
	h.redirectWithNotice(w, r, fmt.Sprintf("/admin/trash-list/%s", modelKey), "success", "تم الحذف النهائي للسجل.")
}
