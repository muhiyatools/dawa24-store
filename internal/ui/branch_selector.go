// Buying-branch selector for the customer shell (Rebuild V2 §3.2).
//
// The dropdown belongs in the shell, not on a page: changing the branch
// changes what the whole catalogue shows. The option list always comes from
// the database; the cookie only carries the chosen branch id, and every
// handler resolves coordinates from the branch record — never from the
// request — so a forged cookie cannot inject coordinates.
package ui

import (
	"net/http"

	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
)

// buyingBranchCookie persists the chosen branch per browser.
const buyingBranchCookie = "dawa24_buying_branch"

// BuyingBranchSelector binds the customer's branch options and the active
// choice into the request context. Non-customer actors pass through untouched.
func (h *UIHandler) BuyingBranchSelector(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authctx.From(r.Context())
		if !ok || !actor.IsCustomer() || h.orgSvc == nil {
			next.ServeHTTP(w, r)
			return
		}

		options := h.customerBranchOptions(r, actor)
		if len(options) == 0 {
			next.ServeHTTP(w, r)
			return
		}

		selection := authctx.BuyingBranch{Branches: options}
		if active := h.validatedCookieBranch(r, actor, options); active != nil {
			selection.Active = active
			actor.BranchID = active
		} else if len(options) > 0 {
			selection.Active = &options[0].ID
			actor.BranchID = &options[0].ID
		}

		ctx := authctx.WithBuyingBranch(r.Context(), selection)
		ctx = authctx.WithActor(ctx, actor)
		next.ServeHTTP(w, r.WithContext(ctx))

	})
}

// customerBranchOptions lists the actor's active branches for the selector.
func (h *UIHandler) customerBranchOptions(r *http.Request, actor authctx.Actor) []authctx.BranchOption {
	branches, err := h.orgSvc.ListBranches(r.Context(), actor.OrganizationID)
	if err != nil {
		return nil
	}
	options := make([]authctx.BranchOption, 0, len(branches))
	for _, b := range branches {
		if b == nil || b.Status != "active" {
			continue
		}
		options = append(options, authctx.BranchOption{ID: b.ID, Name: branchName(b)})
	}
	return options
}

// validatedCookieBranch accepts the cookie value only when it names one of
// the actor's own branches.
func (h *UIHandler) validatedCookieBranch(r *http.Request, actor authctx.Actor, options []authctx.BranchOption) *int64 {
	cookie, err := r.Cookie(buyingBranchCookie)
	id := parseBranchID(cookie)
	if err != nil || id <= 0 {
		return nil
	}
	branch, err := h.orgSvc.GetBranch(r.Context(), id)
	if err != nil || branch == nil || branch.OrganizationID != actor.OrganizationID {
		return nil
	}
	for _, o := range options {
		if o.ID == id {
			return &id
		}
	}
	return nil
}

// SetBuyingBranchSubmit persists the chosen branch for this browser and
// returns to the previous page, which re-renders with the new selection.
func (h *UIHandler) SetBuyingBranchSubmit(w http.ResponseWriter, r *http.Request) {
	actor, ok := authctx.From(r.Context())
	if !ok || !actor.IsCustomer() || h.orgSvc == nil {
		http.Redirect(w, r, "/catalog", http.StatusSeeOther)
		return
	}

	id := parseBranchID(&http.Cookie{Value: r.PostFormValue("branch_id")})
	if id <= 0 {
		http.Redirect(w, r, "/customer/dashboard", http.StatusSeeOther)
		return
	}

	branch, err := h.orgSvc.GetBranch(r.Context(), id)
	if err != nil || branch == nil || branch.OrganizationID != actor.OrganizationID {
		http.Redirect(w, r, "/customer/dashboard", http.StatusSeeOther)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     buyingBranchCookie,
		Value:    r.PostFormValue("branch_id"),
		Path:     "/",
		MaxAge:   60 * 60 * 24 * 30,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	target := r.PostFormValue("redirect_to")
	if target == "" {
		target = r.Referer()
	}
	if target == "" {
		target = "/catalog"
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
}

// parseBranchID decodes a plain-decimal cookie value, or 0 on garbage.
func parseBranchID(c *http.Cookie) int64 {
	if c == nil || c.Value == "" {
		return 0
	}
	var id int64
	for _, ch := range c.Value {
		if ch < '0' || ch > '9' {
			return 0
		}
		id = id*10 + int64(ch-'0')
	}
	if id <= 0 {
		return 0
	}
	return id
}

// branchName prefers the Arabic branch name, then the English one.
func branchName(b *org.Branch) string {
	if b == nil {
		return ""
	}
	if b.Name["ar"] != "" {
		return b.Name["ar"]
	}
	if b.Name["en"] != "" {
		return b.Name["en"]
	}
	return "فرع " + b.Code
}
