// Storefront offer presentation helpers (Rebuild V2 §3.1).
package ui

import (
	"context"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/modules/promo"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// offersForProduct turns the approved promo offers selling the product into
// storefront rows. Every price passes through promo.EffectivePrice.
func (h *UIHandler) offersForProduct(ctx context.Context, product *catalog.Product, variants []*catalog.ProductVariant) []pages.SupplierOffer {
	if h.promoSvc == nil || h.orgSvc == nil || product == nil {
		return nil
	}

	rows, err := h.promoSvc.ListOffersForProduct(ctx, product.ID)
	if err != nil {
		h.log.WarnContext(ctx, "load offers for product", "product_id", product.ID, "error", err)
		return nil
	}

	listPrice := product.EffectivePrice()
	if len(variants) > 0 && variants[0].Price.IsPositive() {
		listPrice = variants[0].Price
	}

	offers := make([]pages.SupplierOffer, 0, len(rows))
	seen := make(map[int64]bool, len(rows))
	for _, row := range rows {
		if row == nil || row.Offer == nil || row.Product == nil || seen[row.Offer.ID] {
			continue
		}
		seen[row.Offer.ID] = true

		price, bd := promo.EffectivePrice(listPrice, row.Product, row.Offer)

		offer := pages.SupplierOffer{
			OfferID:         row.Offer.ID,
			SupplierID:      row.Offer.OrganizationID,
			Price:           price,
			OldPrice:        bd.ListPrice,
			DiscountAmount:  bd.DiscountAmount,
			DiscountBPS:     bd.DiscountBPS,
			MinOrderQty:     row.Product.CustomQty,
		}

		if orgn, err := h.orgSvc.GetOrganization(ctx, row.Offer.OrganizationID); err == nil && orgn != nil {
			offer.SupplierName = orgName(orgn)
			offer.IsVerified = orgn.Status == org.StatusApproved
		}
		if offer.SupplierName == "" {
			offer.SupplierName = "مورد معتمد"
		}

		offers = append(offers, offer)
	}
	return offers
}

// orgName prefers the Arabic trade name, then the English one, then the
// registered legal name.
func orgName(o *org.Organization) string {
	if o == nil {
		return ""
	}
	if o.TradeName["ar"] != "" {
		return o.TradeName["ar"]
	}
	if o.TradeName["en"] != "" {
		return o.TradeName["en"]
	}
	return o.LegalName
}

// pharmacyBranchCoords resolves the branch the actor is buying for: the
// branch chosen in the shell selector, else the member-bound branch, else
// the main branch, else the first active one. Returns false when the pharmacy
// has no branch with coordinates. Coordinates come from the database branch
// record, never from the request (Rebuild V2 §3.2).
func (h *UIHandler) pharmacyBranchCoords(ctx context.Context, actor *authctx.Actor) (lat, lng float64, ok bool) {
	if h.orgSvc == nil || actor == nil || actor.OrganizationID <= 0 {
		return 0, 0, false
	}

	var branch *org.Branch
	if selection, has := authctx.BuyingBranchFrom(ctx); has && selection.Active != nil && *selection.Active > 0 {
		if b, err := h.orgSvc.GetBranch(ctx, *selection.Active); err == nil && b != nil {
			branch = b
		}
	}
	if branch == nil && actor.BranchID != nil && *actor.BranchID > 0 {
		if b, err := h.orgSvc.GetBranch(ctx, *actor.BranchID); err == nil {
			branch = b
		}
	}
	if branch == nil {
		branches, err := h.orgSvc.ListBranches(ctx, actor.OrganizationID)
		if err != nil {
			return 0, 0, false
		}
		for _, b := range branches {
			if b == nil {
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
	if branch == nil || branch.Latitude == nil || branch.Longitude == nil {
		return 0, 0, false
	}
	return *branch.Latitude, *branch.Longitude, true
}

// visibleOffersForActor lists the offers reachable from the pharmacy branch
// the actor is buying for; empty when no branch coordinates exist.
func (h *UIHandler) visibleOffersForActor(ctx context.Context, actor *authctx.Actor, limit int) []*promo.VisibleOffer {
	if h.promoSvc == nil {
		return nil
	}
	lat, lng, ok := h.pharmacyBranchCoords(ctx, actor)
	if !ok {
		return nil
	}
	offers, err := h.promoSvc.ListOffersVisibleTo(ctx, lat, lng, int(time.Now().Weekday()), limit, 0)
	if err != nil {
		h.log.WarnContext(ctx, "load visible offers", "error", err)
		return nil
	}
	return offers
}