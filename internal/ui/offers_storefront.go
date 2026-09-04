package ui

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/modules/promo"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

func calculateHaversineKM(lat1, lon1, lat2, lon2 float64) float64 {
	const earthRadiusKM = 6371.0
	dLat := (lat2 - lat1) * (math.Pi / 180.0)
	dLon := (lon2 - lon1) * (math.Pi / 180.0)
	rLat1 := lat1 * (math.Pi / 180.0)
	rLat2 := lat2 * (math.Pi / 180.0)

	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Sin(dLon/2)*math.Sin(dLon/2)*math.Cos(rLat1)*math.Cos(rLat2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	return earthRadiusKM * c
}

func formatDistanceKMText(km float64, lang string) string {
	if km <= 0 {
		return ""
	}
	if km < 1.0 {
		return i18n.T(lang, "offers.distance_less_1km")
	}
	if km < 100.0 {
		return fmt.Sprintf(i18n.T(lang, "offers.distance_km_format"), km)
	}
	return fmt.Sprintf(i18n.T(lang, "offers.distance_km_int_format"), int(km))
}

// offersForProduct turns the approved vendor variants and promo offers selling the product into
// storefront rows. Every price passes through promo.EffectivePrice when discounts apply.
//
// env carries batch-prefetched org/branch/stock/promo lookups; build it once
// per page with buildOfferEnv so rendering a page costs constant queries
// instead of a few per variant.
func (h *UIHandler) offersForProduct(ctx context.Context, product *catalog.Product, variants []*catalog.ProductVariant, env *offerEnv, langOptional ...string) []pages.SupplierOffer {
	if product == nil {
		return nil
	}
	lang := "ar"
	if len(langOptional) > 0 && langOptional[0] != "" {
		lang = langOptional[0]
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
	custLat, custLng, hasCustCoords := h.pharmacyBranchCoords(ctx, &actor)

	// 1. Process all direct vendor supply variants
	for _, v := range variants {
		if v == nil || v.OrganizationID <= 0 {
			continue
		}

		orgn := env.org(v.OrganizationID)
		supplierName := orgName(orgn)
		if supplierName == "" {
			supplierName = i18n.T(lang, "offers.default_supplier_name")
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

		if v.Discount.IsPositive() && v.Price.IsPositive() {
			oldPrice = v.Price
			// In catalog schema, v.Discount stores the discount percentage (e.g. 15.00 for 15%).
			discPct := float64(v.Discount.Minor()) / 100.0
			if discPct > 0 && discPct < 100 {
				discountBPS = int64(discPct * 100.0)
				discMinor := int64(float64(v.Price.Minor()) * (discPct / 100.0))
				discountAmt = money.FromMinor(discMinor)
				price = money.FromMinor(v.Price.Minor() - discMinor)
			}
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
		cityNameStr := ""
		var distKM float64 = 0
		var distText string = ""

		var venBranch *org.Branch
		if v.BranchID != nil && *v.BranchID > 0 {
			venBranch = env.branch(*v.BranchID)
			if venBranch != nil {
				if venBranch.Name["ar"] != "" {
					branchNameStr = venBranch.Name["ar"]
				} else {
					branchNameStr = venBranch.Name["en"]
				}
				if venBranch.Address != "" {
					cityNameStr = venBranch.Address
				}
			}
		}

		if hasCustCoords && venBranch != nil && venBranch.Latitude != nil && venBranch.Longitude != nil && (*venBranch.Latitude != 0 || *venBranch.Longitude != 0) {
			distKM = calculateHaversineKM(custLat, custLng, *venBranch.Latitude, *venBranch.Longitude)
			distText = formatDistanceKMText(distKM, lang)
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
						if res.Reason == commerce.ReasonNotCovered || res.Reason == commerce.ReasonBranchNoLocation || res.Reason == commerce.ReasonBranchNoInstitutionalWorks {
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
					covReason = i18n.T(lang, "offers.cov_reason_verify_failed")
					canAddToCart = false
				}
			} else if customerBranchID <= 0 {
				isCovered = false
				canAddToCart = false
				covReason = "يرجى تحديد فرع صيدلية للاستلام أولاً للتمكن من الطلب"
			}
		} else {
			isCovered = true
			if stockQty > 0 {
				canAddToCart = true
			} else {
				covReason = i18n.T(lang, "offers.cov_reason_out_of_stock")
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
			DeliveryEstimate: i18n.T(lang, "offers.delivery_estimate_24h"),
			ColdChain:        true,
			BranchName:       branchNameStr,
			CityName:         cityNameStr,
			DistanceKM:       distKM,
			DistanceText:     distText,
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
				sName = i18n.T(lang, "offers.default_supplier_name")
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
				DeliveryEstimate: i18n.T(lang, "offers.delivery_estimate_24h"),
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

	sortSupplierOffers(offers)

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
		lat, lng = 30.0444, 31.2357
	}
	offers, err := h.promoSvc.ListOffersVisibleTo(ctx, lat, lng, int(time.Now().Weekday()), limit, 0)
	if err != nil {
		h.log.WarnContext(ctx, "load visible offers", "error", err)
		return nil
	}
	return offers
}
