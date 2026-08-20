package ui

import (
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// VendorInstitutionalWorkPage renders the institutional service enrollments and memberships of the vendor.
func (h *UIHandler) VendorInstitutionalWorkPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/institutional-work", http.StatusSeeOther)
		return
	}

	var works []*org.InstitutionalWork
	if h.orgSvc != nil {
		works, _ = h.orgSvc.ListInstitutionalWorks(ctx, true)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.VendorInstitutionalWorkPage(works, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render vendor institutional work", "error", err)
	}
}

// VendorPharmacyCoveragePage renders which pharmacies fall inside this vendor's branch coverage schedules.
func (h *UIHandler) VendorPharmacyCoveragePage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/pharmacy-coverage", http.StatusSeeOther)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.VendorPharmacyCoveragePage(lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render vendor pharmacy coverage", "error", err)
	}
}

// VendorPharmacyCoverageDetailPage renders single pharmacy coverage detail.
func (h *UIHandler) VendorPharmacyCoverageDetailPage(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	http.Redirect(w, r, fmt.Sprintf("/vendor/pharmacy-coverage?id=%s", idStr), http.StatusSeeOther)
}
