// Batch-prefetched lookup environment for storefront offer rendering.
//
// Rendering one catalog page used to issue per-variant GetOrganization,
// AvailableQuantity and GetBranch calls plus per-product promo lookups —
// roughly a thousand sequential round trips for a full page. The environment
// resolves every ID set once, up front; offer assembly then reads maps only.
package ui

import (
	"context"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/modules/promo"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
)

// offerEnv holds batch-prefetched lookups shared by every product on a page.
type offerEnv struct {
	orgs     map[int64]*org.Organization
	branches map[int64]*org.Branch
	stock    map[int64]int
	promo    map[int64][]*promo.OfferProductWithOffer
}

func (e *offerEnv) org(id int64) *org.Organization                    { return e.orgs[id] }
func (e *offerEnv) branch(id int64) *org.Branch                       { return e.branches[id] }
func (e *offerEnv) stockQty(id int64) int                             { return e.stock[id] }
func (e *offerEnv) offersFor(id int64) []*promo.OfferProductWithOffer { return e.promo[id] }

// buildOfferEnv prefetches organizations, branches, stock levels and running
// promo offers for the given products and their variants in five queries.
func (h *UIHandler) buildOfferEnv(ctx context.Context, productIDs []int64, variantsByProduct map[int64][]*catalog.ProductVariant) *offerEnv {
	env := &offerEnv{
		orgs:     make(map[int64]*org.Organization),
		branches: make(map[int64]*org.Branch),
		stock:    make(map[int64]int),
		promo:    make(map[int64][]*promo.OfferProductWithOffer),
	}
	if len(productIDs) == 0 {
		return env
	}

	orgIDSet := make(map[int64]struct{})
	branchIDSet := make(map[int64]struct{})
	variantIDs := make([]int64, 0, len(productIDs)*2)
	for _, variants := range variantsByProduct {
		for _, v := range variants {
			if v == nil {
				continue
			}
			if v.OrganizationID > 0 {
				orgIDSet[v.OrganizationID] = struct{}{}
			}
			if v.BranchID != nil && *v.BranchID > 0 {
				branchIDSet[*v.BranchID] = struct{}{}
			}
			if v.ID > 0 {
				variantIDs = append(variantIDs, v.ID)
			}
		}
	}

	if h.orgSvc != nil {
		if m, err := h.orgSvc.GetOrganizations(ctx, keysOf(orgIDSet)); err == nil && m != nil {
			env.orgs = m
		}
		if m, err := h.orgSvc.GetBranches(ctx, keysOf(branchIDSet)); err == nil && m != nil {
			env.branches = m
		}
	}
	if h.invSvc != nil {
		if m, err := h.invSvc.AvailableQuantities(ctx, variantIDs); err == nil && m != nil {
			env.stock = m
		}
	}
	if h.promoSvc != nil {
		if m, err := h.promoSvc.ListOffersForProducts(ctx, productIDs); err == nil && m != nil {
			env.promo = m
		}
	}
	return env
}

func keysOf(set map[int64]struct{}) []int64 {
	out := make([]int64, 0, len(set))
	for id := range set {
		out = append(out, id)
	}
	return out
}

// pharmacyBranchID resolves the branch the actor is buying for: the
// member-bound branch first (if assigned to a specific branch), else
// the branch chosen in the shell selector, else the main branch, else
// the first active one.
func (h *UIHandler) pharmacyBranchID(ctx context.Context, actor *authctx.Actor) int64 {
	if actor != nil && actor.BranchID != nil && *actor.BranchID > 0 {
		return *actor.BranchID
	}
	if selection, has := authctx.BuyingBranchFrom(ctx); has && selection.Active != nil && *selection.Active > 0 {
		return *selection.Active
	}
	if h.orgSvc == nil || actor == nil || actor.OrganizationID <= 0 {
		return 0
	}
	branches, err := h.orgSvc.ListBranches(ctx, actor.OrganizationID)
	if err != nil {
		return 0
	}
	for _, b := range branches {
		if b == nil || b.Status == "inactive" || b.Status == "suspended" {
			continue
		}
		if b.IsMain {
			return b.ID
		}
	}
	for _, b := range branches {
		if b != nil && b.Status != "inactive" && b.Status != "suspended" {
			return b.ID
		}
	}
	return 0
}

// pharmacyBranchCoords resolves the branch the actor is buying for: the
// member-bound branch first (if assigned to a specific branch), else
// the branch chosen in the shell selector, else the main branch, else
// the first active one. Returns false when the pharmacy
// has no branch with coordinates. Coordinates come from the database branch
// record, never from the request (Rebuild V2 §3.2).
func (h *UIHandler) pharmacyBranchCoords(ctx context.Context, actor *authctx.Actor) (lat, lng float64, ok bool) {
	if h.orgSvc == nil || actor == nil || actor.OrganizationID <= 0 {
		return 0, 0, false
	}

	var branch *org.Branch
	if actor.BranchID != nil && *actor.BranchID > 0 {
		if b, err := h.orgSvc.GetBranch(ctx, *actor.BranchID); err != nil {
			h.log.DebugContext(ctx, "pharmacy branch coords: get actor branch", "branch_id", *actor.BranchID, "error", err)
		} else if b != nil {
			branch = b
		}
	}
	if branch == nil {
		if selection, has := authctx.BuyingBranchFrom(ctx); has && selection.Active != nil && *selection.Active > 0 {
			if b, err := h.orgSvc.GetBranch(ctx, *selection.Active); err != nil {
				h.log.DebugContext(ctx, "pharmacy branch coords: get branch selection optional", "branch_id", *selection.Active, "error", err)
			} else if b != nil {
				branch = b
			}
		}
	}
	if branch == nil {
		branches, err := h.orgSvc.ListBranches(ctx, actor.OrganizationID)
		if err != nil {
			return 0, 0, false
		}
		for _, b := range branches {
			if b == nil || b.Status == "inactive" || b.Status == "suspended" {
				continue
			}
			if branch == nil {
				branch = b
			}
			if b.IsMain {
				branch = b
				break
			}
		}
	}
	if branch == nil {
		return 0, 0, false
	}
	if branch.Latitude != nil && branch.Longitude != nil && (*branch.Latitude != 0 || *branch.Longitude != 0) {
		return *branch.Latitude, *branch.Longitude, true
	}
	if branch.CityID != nil && *branch.CityID > 0 && h.adminSvc != nil {
		if city, err := h.adminSvc.GetCity(ctx, *branch.CityID); err == nil && city != nil {
			if city.Latitude != 0 || city.Longitude != 0 {
				return city.Latitude, city.Longitude, true
			}
		}
	}
	return 0, 0, false
}
