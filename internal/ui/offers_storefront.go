// Storefront offer presentation helpers (Rebuild V2 §3.1).
package ui

import (
	"context"
	"sort"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/modules/promo"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// offersForProduct turns the approved vendor variants and promo offers selling the product into
// storefront rows. Every price passes through promo.EffectivePrice when discounts apply.
func (h *UIHandler) offersForProduct(ctx context.Context, product *catalog.Product, variants []*catalog.ProductVariant) []pages.SupplierOffer {
	if product == nil {
		return nil
	}

	offers := make([]pages.SupplierOffer, 0, len(variants)+2)
	seenSuppliers := make(map[int64]int) // supplierID -> index in offers

	actor, hasActor := authctx.From(ctx)
	isPharmacy := hasActor && actor.IsCustomer()
	customerBranchID := int64(0)
	if isPharmacy {
		customerBranchID = h.pharmacyBranchID(ctx, &actor)
	}

	// 1. Process all direct vendor supply variants
	for _, v := range variants {
		if v == nil || v.OrganizationID <= 0 {
			continue
		}

		// Verify supplier organization
		var orgn *org.Organization
		if h.orgSvc != nil {
			orgn, _ = h.orgSvc.GetOrganization(ctx, v.OrganizationID)
		}
		supplierName := orgName(orgn)
		if supplierName == "" {
			supplierName = "مورد معتمد"
		}

		minQty := v.MinOrderQty
		if minQty <= 0 {
			minQty = 1
		}

		expiryStr := ""
		if v.ExpiryDate != nil {
			expiryStr = v.ExpiryDate.Format("2006-01-02")
		}

		price := v.Price
		oldPrice := v.Price
		discountAmt := money.Zero
		var discountBPS int64 = 0

		if v.Discount.IsPositive() && v.Price.IsPositive() && v.Price.Minor() > v.Discount.Minor() {
			oldPrice = v.Price
			price = money.FromMinor(v.Price.Minor() - v.Discount.Minor())
			discountAmt = v.Discount
			discountBPS = int64(v.Discount.Minor()) * 10000 / int64(v.Price.Minor())
		} else if product != nil && product.Price.IsPositive() && v.Price.IsPositive() && product.Price.Minor() > v.Price.Minor() {
			oldPrice = product.Price
			price = v.Price
			diff := product.Price.Minor() - v.Price.Minor()
			discountAmt = money.FromMinor(diff)
			discountBPS = diff * 10000 / int64(product.Price.Minor())
		} else if !price.IsPositive() && product != nil && product.EffectivePrice().IsPositive() {
			price = product.EffectivePrice()
			oldPrice = price
		}

		// Resolve actual stock from inventory.stocks
		stockQty := 0
		if h.invSvc != nil {
			if s, err := h.invSvc.AvailableQuantity(ctx, v.ID); err == nil && s > 0 {
				stockQty = s
			}
		}
		if stockQty == 0 && v.StockQty > 0 {
			stockQty = v.StockQty
		}

		branchName := ""
		if v.BranchID != nil && *v.BranchID > 0 && h.orgSvc != nil {
			if b, err := h.orgSvc.GetBranch(ctx, *v.BranchID); err == nil && b != nil {
				if b.Name["ar"] != "" {
					branchName = b.Name["ar"]
				} else {
					branchName = b.Name["en"]
				}
			}
		}

		isCovered := false
		canAddToCart := false
		covReason := ""

		if isPharmacy {
			if h.commSvc != nil && customerBranchID > 0 {
				res, err := h.commSvc.CheckAvailability(ctx, commerce.AvailabilityRequest{
					VariantID:        v.ID,
					VendorOrgID:      v.OrganizationID,
					CustomerOrgID:    actor.OrganizationID,
					CustomerBranchID: customerBranchID,
					Quantity:         minQty,
					When:             time.Now(),
				})
				if err == nil {
					if res.Allowed {
						isCovered = true
						canAddToCart = (stockQty > 0)
					} else {
						covReason = res.MessageAr
						if res.Reason == commerce.ReasonNotCovered || res.Reason == commerce.ReasonBranchNoLocation {
							isCovered = false
							canAddToCart = false
						} else if res.Reason == commerce.ReasonOutOfStock || res.Reason == commerce.ReasonInsufficientStock {
							isCovered = true
							canAddToCart = false
						} else if res.Reason == commerce.ReasonBelowMinimum {
							isCovered = true
							canAddToCart = (stockQty > 0)
						} else {
							isCovered = false
							canAddToCart = false
						}
					}
				} else {
					isCovered = false
					covReason = "تعذر التحقق من التغطية الجغرافية"
					canAddToCart = false
				}
			} else if customerBranchID <= 0 {
				isCovered = false
				covReason = "يرجى تحديد فرع الاستلام للتحقق من التغطية"
				canAddToCart = false
			}
		} else {
			isCovered = true
			if stockQty > 0 {
				canAddToCart = true
			} else {
				covReason = "نفد المخزون لدى المورد"
			}
		}

		off := pages.SupplierOffer{
			VariantID:        v.ID,
			SupplierID:       v.OrganizationID,
			SupplierName:     supplierName,
			IsVerified:       orgn == nil || orgn.Status == org.StatusApproved,
			Price:            price,
			OldPrice:         oldPrice,
			DiscountAmount:   discountAmt,
			DiscountBPS:      discountBPS,
			AvailableStock:   stockQty,
			MinOrderQty:      minQty,
			BatchNumber:      v.BatchNumber,
			ExpiryDate:       expiryStr,
			DeliveryEstimate: "توصيل خلال 24 ساعة",
			ColdChain:        true,
			BranchName:       branchName,
			IsCovered:        isCovered,
			CanAddToCart:     canAddToCart,
			CoverageReason:   covReason,
			IsNegotiable:     v.IsNegotiable,
		}

		seenSuppliers[v.OrganizationID] = len(offers)
		offers = append(offers, off)
	}

	// 2. Check for promotional discounts from promo module
	if h.promoSvc != nil {
		rows, err := h.promoSvc.ListOffersForProduct(ctx, product.ID)
		if err != nil {
			h.log.WarnContext(ctx, "offers storefront: list offers for product", "product_id", product.ID, "error", err)
		} else {
			listPrice := product.EffectivePrice()
			for _, row := range rows {
				if row == nil || row.Offer == nil || row.Product == nil {
					continue
				}

				price, bd := promo.EffectivePrice(listPrice, row.Product, row.Offer)

				if idx, found := seenSuppliers[row.Offer.OrganizationID]; found {
					// Apply promo discount to existing supplier offer
					offers[idx].OfferID = row.Offer.ID
					offers[idx].Price = price
					offers[idx].OldPrice = bd.ListPrice
					offers[idx].DiscountAmount = bd.DiscountAmount
					offers[idx].DiscountBPS = bd.DiscountBPS
				} else {
					var orgn *org.Organization
					if h.orgSvc != nil {
						orgn, _ = h.orgSvc.GetOrganization(ctx, row.Offer.OrganizationID)
					}
					sName := orgName(orgn)
					if sName == "" {
						sName = "مورد معتمد"
					}

					newOffer := pages.SupplierOffer{
						OfferID:          row.Offer.ID,
						SupplierID:       row.Offer.OrganizationID,
						SupplierName:     sName,
						IsVerified:       orgn == nil || orgn.Status == org.StatusApproved,
						Price:            price,
						OldPrice:         bd.ListPrice,
						DiscountAmount:   bd.DiscountAmount,
						DiscountBPS:      bd.DiscountBPS,
						MinOrderQty:      row.Product.CustomQty,
						DeliveryEstimate: "توصيل خلال 24 ساعة",
						ColdChain:        true,
						IsCovered:        true,
						CanAddToCart:     true,
					}
					if newOffer.MinOrderQty <= 0 {
						newOffer.MinOrderQty = 1
					}
					seenSuppliers[row.Offer.OrganizationID] = len(offers)
					offers = append(offers, newOffer)
				}
			}
		}
	}

	// Sort offers: actionable & covered first, then lowest price
	sort.SliceStable(offers, func(i, j int) bool {
		if offers[i].CanAddToCart != offers[j].CanAddToCart {
			return offers[i].CanAddToCart
		}
		if offers[i].IsCovered != offers[j].IsCovered {
			return offers[i].IsCovered
		}
		return offers[i].Price.Minor() < offers[j].Price.Minor()
	})

	return offers
}

// pharmacyBranchID resolves the branch the actor is buying for: the
// branch chosen in the shell selector, else the member-bound branch, else
// the main branch, else the first active one.
func (h *UIHandler) pharmacyBranchID(ctx context.Context, actor *authctx.Actor) int64 {
	if h.orgSvc == nil || actor == nil || actor.OrganizationID <= 0 {
		return 0
	}
	if selection, has := authctx.BuyingBranchFrom(ctx); has && selection.Active != nil && *selection.Active > 0 {
		return *selection.Active
	}
	if actor.BranchID != nil && *actor.BranchID > 0 {
		return *actor.BranchID
	}
	branches, err := h.orgSvc.ListBranches(ctx, actor.OrganizationID)
	if err != nil {
		return 0
	}
	for _, b := range branches {
		if b == nil || b.Status != "active" {
			continue
		}
		if b.IsMain {
			return b.ID
		}
	}
	for _, b := range branches {
		if b != nil && b.Status == "active" {
			return b.ID
		}
	}
	return 0
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
		if b, err := h.orgSvc.GetBranch(ctx, *selection.Active); err != nil {
			h.log.DebugContext(ctx, "pharmacy branch coords: get branch selection optional", "branch_id", *selection.Active, "error", err)
		} else if b != nil {
			branch = b
		}
	}
	if branch == nil && actor.BranchID != nil && *actor.BranchID > 0 {
		if b, err := h.orgSvc.GetBranch(ctx, *actor.BranchID); err != nil {
			h.log.DebugContext(ctx, "pharmacy branch coords: get actor branch optional", "branch_id", *actor.BranchID, "error", err)
		} else if b != nil {
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
