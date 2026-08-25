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
//
// env carries batch-prefetched org/branch/stock/promo lookups; build it once
// per page with buildOfferEnv so rendering a page costs constant queries
// instead of a few per variant.
func (h *UIHandler) offersForProduct(ctx context.Context, product *catalog.Product, variants []*catalog.ProductVariant, env *offerEnv) []pages.SupplierOffer {
	if product == nil {
		return nil
	}
	if env == nil {
		env = h.buildOfferEnv(ctx, []int64{product.ID}, map[int64][]*catalog.ProductVariant{
			product.ID: variants,
		})
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

		orgn := env.org(v.OrganizationID)
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

		// Resolve actual stock from inventory.stocks (prefetched).
		stockQty := env.stockQty(v.ID)
		if stockQty == 0 && v.StockQty > 0 {
			stockQty = v.StockQty
		}

		branchNameStr := ""
		if v.BranchID != nil && *v.BranchID > 0 {
			if b := env.branch(*v.BranchID); b != nil {
				if b.Name["ar"] != "" {
					branchNameStr = b.Name["ar"]
				} else {
					branchNameStr = b.Name["en"]
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
			BranchName:       branchNameStr,
			IsCovered:        isCovered,
			CanAddToCart:     canAddToCart,
			CoverageReason:   covReason,
			IsNegotiable:     v.IsNegotiable,
		}

		seenSuppliers[v.OrganizationID] = len(offers)
		offers = append(offers, off)
	}

	// 2. Check for promotional discounts from promo module (prefetched)
	for _, row := range env.offersFor(product.ID) {
		if row == nil || row.Offer == nil || row.Product == nil {
			continue
		}

		listPrice := product.EffectivePrice()
		price, bd := promo.EffectivePrice(listPrice, row.Product, row.Offer)

		if idx, found := seenSuppliers[row.Offer.OrganizationID]; found {
			// Apply promo discount to existing supplier offer
			offers[idx].OfferID = row.Offer.ID
			offers[idx].Price = price
			offers[idx].OldPrice = bd.ListPrice
			offers[idx].DiscountAmount = bd.DiscountAmount
			offers[idx].DiscountBPS = bd.DiscountBPS
		} else {
			orgn := env.org(row.Offer.OrganizationID)
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
