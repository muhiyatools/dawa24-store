package ui

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/compare"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

func (h *UIHandler) checkFileOwnership(actor authctx.Actor, file *compare.CompareFile) bool {
	if file == nil {
		return false
	}
	if file.UserID == actor.UserID {
		return true
	}
	if file.OrganizationID != nil && actor.OrganizationID > 0 && *file.OrganizationID == actor.OrganizationID {
		return true
	}
	return false
}

// CompareFileRenameSubmit handles renaming a supplier file label.
func (h *UIHandler) CompareFileRenameSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/compare/tool", http.StatusSeeOther)
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/compare/tool", "error", i18n.T(lang, "compare.file.invalid_id"))
		return
	}

	if h.compareSvc != nil {
		file, err := h.compareSvc.GetFile(ctx, id)
		if err != nil || !h.checkFileOwnership(actor, file) {
			h.redirectWithNotice(w, r, "/compare/tool", "error", i18n.T(lang, "compare.file.edit_forbidden"))
			return
		}

		newName := strings.TrimSpace(r.FormValue("supplier_name"))
		if newName == "" {
			h.redirectWithNotice(w, r, "/compare/tool", "error", i18n.T(lang, "compare.file.supplier_name_required"))
			return
		}
		if err := h.compareSvc.RenameFile(ctx, id, newName); err != nil {
			h.redirectWithNotice(w, r, "/compare/tool", "error", h.safeMessage(err, lang))
			return
		}
	}
	h.redirectWithNotice(w, r, "/compare/tool", "success", i18n.T(lang, "compare.file.renamed_success"))
}

// CompareFileArchiveSubmit handles manually archiving a file.
func (h *UIHandler) CompareFileArchiveSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/compare/tool", http.StatusSeeOther)
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/compare/tool", "error", i18n.T(lang, "compare.file.invalid_id"))
		return
	}
	if h.compareSvc != nil {
		file, err := h.compareSvc.GetFile(ctx, id)
		if err != nil || !h.checkFileOwnership(actor, file) {
			h.redirectWithNotice(w, r, "/compare/tool", "error", i18n.T(lang, "compare.file.edit_forbidden"))
			return
		}
		if err := h.compareSvc.ArchiveFile(ctx, id, i18n.T(lang, "compare.file.manual_archive_reason")); err != nil {
			h.redirectWithNotice(w, r, "/compare/tool", "error", h.safeMessage(err, lang))
			return
		}
	}
	h.redirectWithNotice(w, r, "/compare/tool", "success", i18n.T(lang, "compare.file.archived_success"))
}

// CompareFileUnarchiveSubmit handles restoring an archived file.
func (h *UIHandler) CompareFileUnarchiveSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/compare/tool", http.StatusSeeOther)
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/compare/tool", "error", i18n.T(lang, "compare.file.invalid_id"))
		return
	}
	if h.compareSvc != nil {
		file, err := h.compareSvc.GetFile(ctx, id)
		if err != nil || !h.checkFileOwnership(actor, file) {
			h.redirectWithNotice(w, r, "/compare/tool", "error", i18n.T(lang, "compare.file.edit_forbidden"))
			return
		}
		if err := h.compareSvc.UnarchiveFile(ctx, id); err != nil {
			h.redirectWithNotice(w, r, "/compare/tool", "error", h.safeMessage(err, lang))
			return
		}
	}
	h.redirectWithNotice(w, r, "/compare/tool", "success", i18n.T(lang, "compare.file.unarchived_success"))
}

// CompareFileDeleteSubmit handles soft-deleting a file.
func (h *UIHandler) CompareFileDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/compare/tool", http.StatusSeeOther)
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/compare/tool", "error", i18n.T(lang, "compare.file.invalid_id"))
		return
	}
	if h.compareSvc != nil {
		file, err := h.compareSvc.GetFile(ctx, id)
		if err != nil || !h.checkFileOwnership(actor, file) {
			h.redirectWithNotice(w, r, "/compare/tool", "error", i18n.T(lang, "compare.file.delete_forbidden"))
			return
		}
		if err := h.compareSvc.DeleteFile(ctx, id); err != nil {
			h.redirectWithNotice(w, r, "/compare/tool", "error", h.safeMessage(err, lang))
			return
		}
	}
	h.redirectWithNotice(w, r, "/compare/tool", "success", i18n.T(lang, "compare.file.deleted_success"))
}

// CompareFileVisibilitySubmit lets a vendor mark one of their uploaded compare
// files public or private. Public files surface on /market-discounts alongside
// the moderator temporary warehouses.
func (h *UIHandler) CompareFileVisibilitySubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/compare/tool", http.StatusSeeOther)
		return
	}
	if actor.OrganizationID == 0 {
		h.redirectWithNotice(w, r, "/compare/tool", "error", i18n.T(lang, "compare.file.visibility_forbidden"))
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/compare/tool", "error", i18n.T(lang, "compare.file.invalid_id"))
		return
	}
	visibility := strings.TrimSpace(r.FormValue("visibility"))
	if visibility != compare.VisibilityPublic && visibility != compare.VisibilityPrivate {
		h.redirectWithNotice(w, r, "/compare/tool", "error", i18n.T(lang, "compare.file.invalid_id"))
		return
	}
	if h.compareSvc != nil {
		file, err := h.compareSvc.GetFile(ctx, id)
		if err != nil || !h.checkFileOwnership(actor, file) {
			h.redirectWithNotice(w, r, "/compare/tool", "error", i18n.T(lang, "compare.file.edit_forbidden"))
			return
		}
		if err := h.compareSvc.SetFileVisibility(ctx, id, visibility); err != nil {
			h.redirectWithNotice(w, r, "/compare/tool", "error", h.safeMessage(err, lang))
			return
		}
	}
	if visibility == compare.VisibilityPublic {
		h.redirectWithNotice(w, r, "/compare/tool", "success", i18n.T(lang, "compare.file.visibility_public"))
	} else {
		h.redirectWithNotice(w, r, "/compare/tool", "success", i18n.T(lang, "compare.file.visibility_private"))
	}
}
