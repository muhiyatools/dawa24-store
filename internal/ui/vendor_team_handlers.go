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
	h.handleTeamImportPage(w, r, "vendor")
}

// VendorTeamImportUploadSubmit handles file upload and creates new employee import session for vendor.
func (h *UIHandler) VendorTeamImportUploadSubmit(w http.ResponseWriter, r *http.Request) {
	h.handleTeamImportUploadSubmit(w, r, "vendor")
}

// VendorTeamImportSampleDownload downloads the sample spreadsheet template for vendor team.
func (h *UIHandler) VendorTeamImportSampleDownload(w http.ResponseWriter, r *http.Request) {
	h.handleTeamImportSampleDownload(w, "vendor", "dawa24_vendor_team_sample.xlsx")
}

// VendorTeamImportSessionPage renders current session stage for vendor.
func (h *UIHandler) VendorTeamImportSessionPage(w http.ResponseWriter, r *http.Request) {
	h.handleTeamImportSessionPage(w, r, "vendor")
}

// VendorTeamImportMapSubmit saves user column mappings and role mappings, then builds review rows.
func (h *UIHandler) VendorTeamImportMapSubmit(w http.ResponseWriter, r *http.Request) {
	h.handleTeamImportMapSubmit(w, r, "vendor")
}

// VendorTeamImportCommitSubmit creates user accounts and links employees to vendor org.
func (h *UIHandler) VendorTeamImportCommitSubmit(w http.ResponseWriter, r *http.Request) {
	h.handleTeamImportCommitSubmit(w, r, "vendor", "org_employee")
}

// VendorTeamImportCancelSubmit cancels and deletes an import session.
func (h *UIHandler) VendorTeamImportCancelSubmit(w http.ResponseWriter, r *http.Request) {
	h.handleTeamImportCancelSubmit(w, r, "vendor")
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

	h.renderPage(ctx, w, "render vendor team fast add", pages.VendorTeamFastAddPage(lang, dir))
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

	h.renderPage(ctx, w, "render vendor team user detail", pages.VendorTeamUserDetailPage(empID, lang, dir))
}

// VendorTeamUserInfoPage renders employee audit information.
func (h *UIHandler) VendorTeamUserInfoPage(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	http.Redirect(w, r, fmt.Sprintf("/vendor/team/%s", idStr), http.StatusSeeOther)
}


