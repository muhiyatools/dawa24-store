package ui

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// VendorTeamImportPage renders bulk employee spreadsheet upload page.
func (h *UIHandler) VendorTeamImportPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/team/import", http.StatusSeeOther)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.VendorTeamImportPage(lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render vendor team import", "error", err)
	}
}

// VendorTeamFastAddPage renders fast add form for single employee account.
func (h *UIHandler) VendorTeamFastAddPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/team/fast-add", http.StatusSeeOther)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.VendorTeamFastAddPage(lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render vendor team fast add", "error", err)
	}
}

// VendorTeamUserDetailPage renders single employee profile and assigned permissions.
func (h *UIHandler) VendorTeamUserDetailPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	idStr := chi.URLParam(r, "id")
	empID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || empID <= 0 {
		http.Redirect(w, r, "/settings/employees", http.StatusSeeOther)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.VendorTeamUserDetailPage(empID, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render vendor team user detail", "error", err)
	}
}

// VendorTeamUserInfoPage renders employee audit information.
func (h *UIHandler) VendorTeamUserInfoPage(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	http.Redirect(w, r, fmt.Sprintf("/vendor/team/%s", idStr), http.StatusSeeOther)
}
