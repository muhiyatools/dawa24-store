package ui

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

var systemModelEntries = []pages.ModelMetaEntry{
	{Key: "products", NameAr: "المنتجات والأدوية", NameEn: "Products", SchemaTable: "catalog.products"},
	{Key: "organizations", NameAr: "المنشآت والشركات", NameEn: "Organizations", SchemaTable: "org.organizations"},
	{Key: "users", NameAr: "المستخدمين والعملاء", NameEn: "Users", SchemaTable: "identity.users"},
	{Key: "branches", NameAr: "الفروع والمستودعات", NameEn: "Branches", SchemaTable: "org.branches"},
	{Key: "orders", NameAr: "الطلبات والمبيعات", NameEn: "Orders", SchemaTable: "commerce.orders"},
	{Key: "invoices", NameAr: "الفواتير الضريبية", NameEn: "Invoices", SchemaTable: "billing.invoices"},
}

// AdminDeletesListsPage renders overview of all system models and their soft-deletable status.
func (h *UIHandler) AdminDeletesListsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminDeletesListsPage(systemModelEntries, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin deletes lists", "error", err)
	}
}

// AdminDeletesListModelPage renders rows for a specific model.
func (h *UIHandler) AdminDeletesListModelPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	modelKey := chi.URLParam(r, "model")

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminDeletesListModelPage(modelKey, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin deletes list model", "error", err)
	}
}

// AdminDeletesListShowPage renders specific record detail.
func (h *UIHandler) AdminDeletesListShowPage(w http.ResponseWriter, r *http.Request) {
	modelKey := chi.URLParam(r, "model")
	http.Redirect(w, r, fmt.Sprintf("/admin/deletes-lists/%s", modelKey), http.StatusSeeOther)
}

// AdminTrashListPage renders deleted rows directory across models.
func (h *UIHandler) AdminTrashListPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminTrashListPage(systemModelEntries, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin trash list", "error", err)
	}
}

// AdminTrashListModelPage renders trashed rows for a specific model with restore/purge actions.
func (h *UIHandler) AdminTrashListModelPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	modelKey := chi.URLParam(r, "model")

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminTrashListModelPage(modelKey, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin trash list model", "error", err)
	}
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

	h.log.WarnContext(r.Context(), "admin restore requested but registry not connected", "model", modelKey, "id", rowID)
	h.redirectWithNotice(w, r, fmt.Sprintf("/admin/trash-list/%s", modelKey), "error", "خاصية استرجاع السجلات قيد التحديث.")
}

// AdminTrashPurgeSubmit permanently hard-deletes a record.
func (h *UIHandler) AdminTrashPurgeSubmit(w http.ResponseWriter, r *http.Request) {
	modelKey := chi.URLParam(r, "model")
	idStr := chi.URLParam(r, "id")
	rowID, _ := strconv.ParseInt(idStr, 10, 64)

	h.log.WarnContext(r.Context(), "admin purge requested but registry not connected", "model", modelKey, "id", rowID)
	h.redirectWithNotice(w, r, fmt.Sprintf("/admin/trash-list/%s", modelKey), "error", "خاصية الحذف النهائي قيد التحديث.")
}
