package ui

import (
	"net/http"
	"net/url"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// Soft-delete recovery. These screens previously rendered a hardcoded model
// list with invented row counts, and restore/purge logged a line and told the
// administrator it had worked. Everything here now goes through
// platform_admin's trash service, which discovers soft-deletable tables from
// information_schema and counts rows with real queries.

// AdminTrashListPage lists every soft-deletable table with its live counts.
func (h *UIHandler) AdminTrashListPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	if h.adminSvc == nil {
		h.redirectWithNotice(w, r, "/admin/dashboard", "error", "خدمة إدارة المنظومة غير متاحة.")
		return
	}
	models, err := h.adminSvc.ListTrashModels(ctx)
	if err != nil {
		h.log.ErrorContext(ctx, "list trash models", "error", err)
		h.renderError(w, r, err)
		return
	}

	entries := make([]pages.ModelMetaEntry, 0, len(models))
	for _, m := range models {
		entries = append(entries, pages.ModelMetaEntry{
			Key:          m.Key,
			NameAr:       m.NameAr,
			NameEn:       m.NameEn,
			SchemaTable:  m.Key,
			TotalCount:   int(m.TotalCount),
			TrashedCount: int(m.TrashedRows),
		})
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminTrashListPage(entries, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin trash list", "error", err)
	}
}

// AdminTrashListModelPage lists the deleted rows of one table.
func (h *UIHandler) AdminTrashListModelPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	modelKey := chi.URLParam(r, "model")

	if h.adminSvc == nil {
		h.redirectWithNotice(w, r, "/admin/dashboard", "error", "خدمة إدارة المنظومة غير متاحة.")
		return
	}
	rows, err := h.adminSvc.ListTrashedRows(ctx, modelKey, 100, 0)
	if err != nil {
		h.log.ErrorContext(ctx, "list trashed rows", "error", err, "model", modelKey)
		h.renderError(w, r, err)
		return
	}

	items := make([]pages.TrashRowView, 0, len(rows))
	for _, row := range rows {
		items = append(items, pages.TrashRowView{ID: row.ID, Label: row.Label, DeletedAt: row.DeletedAt})
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminTrashListModelPage(modelKey, items, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin trash list model", "error", err)
	}
}

// AdminTrashRestoreSubmit clears deleted_at on one row.
func (h *UIHandler) AdminTrashRestoreSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	modelKey := chi.URLParam(r, "model")
	back := "/admin/trash-list/" + url.PathEscape(modelKey)

	rowID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || rowID <= 0 {
		h.redirectWithNotice(w, r, back, "error", "معرف السجل غير صالح.")
		return
	}
	if h.adminSvc == nil {
		h.redirectWithNotice(w, r, back, "error", "خدمة إدارة المنظومة غير متاحة.")
		return
	}

	actor, _ := authctx.From(ctx)
	if err := h.adminSvc.RestoreTrashedRow(ctx, modelKey, rowID, actor.UserID); err != nil {
		h.log.ErrorContext(ctx, "restore trashed row", "error", err, "model", modelKey, "id", rowID)
		h.redirectWithNotice(w, r, back, "error", h.safeMessage(err, langOf(r)))
		return
	}
	h.redirectWithNotice(w, r, back, "success", "تم استرجاع السجل بنجاح.")
}

// AdminTrashPurgeSubmit permanently removes one row. Irreversible, so the
// service records the row's contents in the audit log before deleting it.
func (h *UIHandler) AdminTrashPurgeSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	modelKey := chi.URLParam(r, "model")
	back := "/admin/trash-list/" + url.PathEscape(modelKey)

	rowID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || rowID <= 0 {
		h.redirectWithNotice(w, r, back, "error", "معرف السجل غير صالح.")
		return
	}
	if h.adminSvc == nil {
		h.redirectWithNotice(w, r, back, "error", "خدمة إدارة المنظومة غير متاحة.")
		return
	}

	actor, _ := authctx.From(ctx)
	if err := h.adminSvc.PurgeTrashedRow(ctx, modelKey, rowID, actor.UserID); err != nil {
		h.log.ErrorContext(ctx, "purge trashed row", "error", err, "model", modelKey, "id", rowID)
		h.redirectWithNotice(w, r, back, "error", h.safeMessage(err, langOf(r)))
		return
	}
	h.redirectWithNotice(w, r, back, "success", "تم الحذف النهائي للسجل.")
}
