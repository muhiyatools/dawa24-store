package ui

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// SuppliersPage renders the public supplier directory.
func (h *UIHandler) SuppliersPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	data := pages.SupplierDirectoryData{}
	if h.orgSvc != nil {
		typ := org.TypeSupplier
		status := org.StatusApproved
		data.Suppliers, _ = h.orgSvc.ListOrganizations(ctx, &typ, &status, 50, 0)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.SuppliersDirectory(lang, dir, data).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render suppliers directory", "error", err)
	}
}

// SupplierProfilePage renders a supplier's public profile: catalogue, reviews
// and policies.
func (h *UIHandler) SupplierProfilePage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || h.orgSvc == nil {
		h.renderError(w, r, err)
		return
	}

	o, err := h.orgSvc.GetOrganization(ctx, id)
	if err != nil {
		h.renderError(w, r, err)
		return
	}
	// Only approved suppliers are publicly listed.
	if o.Status != org.StatusApproved {
		h.renderError(w, r, http.ErrNotSupported)
		return
	}

	data := pages.SupplierProfileData{Org: o}
	if h.catSvc != nil {
		data.Products, _ = h.catSvc.Search(ctx, catalog.SearchParams{OrganizationID: &id, Limit: 24})
	}
	if h.orgSvc != nil {
		data.Reviews, _ = h.orgSvc.ListReviews(ctx, id, 20, 0)
		data.Policies, _ = h.orgSvc.ListPolicies(ctx, id)
		if actor, ok := authctx.From(ctx); ok {
			data.IsFollowing, _ = h.orgSvc.IsFollowing(ctx, id, actor.UserID)
		}
	}
	data.ReviewCount = len(data.Reviews)
	if data.ReviewCount > 0 {
		var sum int
		for _, rv := range data.Reviews {
			sum += rv.Rating
		}
		data.Rating = float64(sum) / float64(data.ReviewCount)
	} else {
		data.Rating = 0
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.SupplierProfile(lang, dir, data).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render supplier profile", "error", err)
	}
}

// SupplierFollowSubmit toggles following for the signed-in user.
func (h *UIHandler) SupplierFollowSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID, err := authctx.UserID(ctx)
	if err != nil {
		http.Redirect(w, r, "/auth/login?redirect="+r.Referer(), http.StatusSeeOther)
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err == nil && h.orgSvc != nil {
		_, _ = h.orgSvc.ToggleFollow(ctx, id, userID)
	}

	back := r.Referer()
	if back == "" {
		back = "/suppliers"
	}
	http.Redirect(w, r, back, http.StatusSeeOther)
}
