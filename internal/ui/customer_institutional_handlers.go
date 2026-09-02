package ui

import (
	"net/http"

	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// CustomerInstitutionalWorkPage renders the institutional agreements and network memberships for the customer/pharmacy.
func (h *UIHandler) CustomerInstitutionalWorkPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok || !actor.IsCustomer() {
		http.Redirect(w, r, "/auth/login?redirect=/customer/institutional-work", http.StatusSeeOther)
		return
	}

	var works []*org.InstitutionalWork
	if h.orgSvc != nil {
		works, _ = h.orgSvc.ListInstitutionalWorks(ctx, true)
	}

	h.renderPage(ctx, w, "render customer institutional work", pages.CustomerInstitutionalWorkPage(works, lang, dir))
}