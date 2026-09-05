package ui

import (
	"context"
	"math"
	"math/rand"
	"sort"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/modules/promo"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

func (h *UIHandler) buildCatalogVariantCards(
	ctx context.Context,
	filtered []*catalog.Product,
	productIDs []int64,
	variantsByProduct map[int64][]*catalog.ProductVariant,
	brandMap map[int64]*catalog.Brand,
	env *offerEnv,
	lang string,
	inStock, hasDiscount bool,
	minPrice, maxPrice *int64,
	sortBy string,
) []*pages.SupplierVariantCard {
	var variantCards []*pages.SupplierVariantCard

	favMap := make(map[int64]bool)
	if actor, ok := authctx.From(ctx); ok && h.idSvc != nil {
		if ids, err := h.idSvc.ListFavorites(ctx, actor.UserID); err == nil {
			for _, id := range ids {
				favMap[id] = true
			}
		}
	}

	for _, p := range filtered {
		variants := variantsByProduct[p.ID]

		var pBrandID *int64
		var pBrandName string
		var pBrandLogo string
		if p.BrandID != nil {
			pBrandID = p.BrandID
			if b, found := brandMap[*p.BrandID]; found && b != nil {
				pBrandName = b.Name.Get(i18n.AR)
				if pBrandName == "" {
					pBrandName = b.Name.Get(i18n.EN)
				}
				pBrandLogo = b.Image
			}
		}
		if pBrandName == "" && p.ManufacturingCompanies != "" {
			pBrandName = p.ManufacturingCompanies
		}

		offers := h.offersForProduct(ctx, p, variants, env, lang)

		if len(offers) > 0 {
			for _, off := range offers {
				// Only display available-to-order variants: must have stock and be orderable
				if off.AvailableStock <= 0 {
					continue
				}
				if off.VariantID <= 0 {
					continue
				}
				if hasDiscount && off.DiscountBPS <= 0 {
					continue
				}
				if minPrice != nil && off.Price.Minor() < *minPrice {
					continue
				}
				if maxPrice != nil && off.Price.Minor() > *maxPrice {
					continue
				}

				// Find variant unit name
				varUnitName := ""
				varSKU := ""
				for _, v := range variants {
					if v != nil && v.ID == off.VariantID {
						varUnitName = v.Name["ar"]
						if varUnitName == "" {
							varUnitName = v.Name["en"]
						}
						varSKU = v.SKU
						break
					}
				}

				discPct := int(off.DiscountBPS / 100)

				variantCards = append(variantCards, &pages.SupplierVariantCard{
					VariantID:       off.VariantID,
					ProductID:       p.ID,
					ProductNameAr:   p.Name.Get(i18n.AR),
					ProductNameEn:   p.Name.Get(i18n.EN),
					ProductImage:    p.Image,
					DosageForm:      p.DosageForm,
					Manufacturer:    p.ManufacturingCompanies,
					BrandID:         pBrandID,
					BrandName:       pBrandName,
					BrandLogo:       pBrandLogo,
					ScientificName:  p.ScientificName,
					PublicPrice:     p.Price,
					Price:           off.Price,
					OriginalPrice:   off.OldPrice,
					DiscountPercent: discPct,
					AvailableStock:  off.AvailableStock,
					MinOrderQty:     off.MinOrderQty,
					ExpiryDate:      off.ExpiryDate,
					IsCovered:       off.IsCovered,
					CoverageReason:  off.CoverageReason,
					CanAddToCart:    off.CanAddToCart,
					IsNegotiable:    off.IsNegotiable,
					VariantName:     varUnitName,
					SKU:             varSKU,
					SupplierID:      off.SupplierID,
					SupplierName:    off.SupplierName,
					SupplierRating:  off.SupplierRating,
					IsVerified:      off.IsVerified,
					BranchName:      off.BranchName,
					CityName:        off.CityName,
					GovernorateName: off.GovernorateName,
					DistanceKM:      off.DistanceKM,
					DistanceText:    off.DistanceText,
					IsFavorite:      favMap[p.ID],
				})
			}
		}
	}

	// Sponsorship ranking
	sponsoredRankings := make(map[int64]*promo.RankedSponsorship)
	if h.promoSvc != nil {
		var allIDs []int64
		for _, pID := range productIDs {
			if pID > 0 {
				allIDs = append(allIDs, pID)
			}
		}
		for _, vc := range variantCards {
			if vc != nil && vc.VariantID > 0 {
				allIDs = append(allIDs, vc.VariantID)
			}
		}
		if len(allIDs) > 0 {
			rankings, err := h.promoSvc.RankedSponsorshipsForProducts(ctx, allIDs)
			if err == nil {
				for _, rs := range rankings {
					if rs != nil {
						sponsoredRankings[rs.ItemID] = rs
					}
				}
			}
		}
	}
	for _, vc := range variantCards {
		if vc != nil {
			if rs, ok := sponsoredRankings[vc.ProductID]; ok && rs != nil {
				vc.IsSponsored = true
				vc.SponsoredTier = rs.TierLevel
				vc.TieBreaker = rand.Int63()
			} else if rs, ok := sponsoredRankings[vc.VariantID]; ok && rs != nil {
				vc.IsSponsored = true
				vc.SponsoredTier = rs.TierLevel
				vc.TieBreaker = rand.Int63()
			}
		}
	}

	// 3. Prioritize variant cards:
	// - First: Sponsored products always at the absolute top of all normal products, ordered by package tier level (Diamond > Platinum > Gold, etc.) and random tie-breaker for same tier.
	// - Second: Actionable and orderable products (CanAddToCart && AvailableStock > 0 && IsCovered) ahead of unavailable products.
	// - Third: User sort preference (price_asc, price_desc, discount, name, newest, nearby).
	sort.SliceStable(variantCards, func(i, j int) bool {
		// 1. Sponsored vs Non-Sponsored
		if variantCards[i].IsSponsored != variantCards[j].IsSponsored {
			return variantCards[i].IsSponsored
		}
		if variantCards[i].IsSponsored && variantCards[j].IsSponsored {
			// Higher tier package first (Level 5 > 4 > 3 etc.)
			if variantCards[i].SponsoredTier != variantCards[j].SponsoredTier {
				return variantCards[i].SponsoredTier > variantCards[j].SponsoredTier
			}
			// Same tier package -> Random tie breaker
			if variantCards[i].TieBreaker != variantCards[j].TieBreaker {
				return variantCards[i].TieBreaker < variantCards[j].TieBreaker
			}
		}

		// 2. Orderable & in stock
		aOrderable := variantCards[i].CanAddToCart && variantCards[i].AvailableStock > 0 && variantCards[i].IsCovered
		bOrderable := variantCards[j].CanAddToCart && variantCards[j].AvailableStock > 0 && variantCards[j].IsCovered
		if aOrderable != bOrderable {
			return aOrderable
		}

		aInStock := variantCards[i].AvailableStock > 0
		bInStock := variantCards[j].AvailableStock > 0
		if aInStock != bInStock {
			return aInStock
		}

		// 3. User sort or default
		switch sortBy {
		case "price_asc":
			if variantCards[i].Price.Minor() != variantCards[j].Price.Minor() {
				return variantCards[i].Price.Minor() < variantCards[j].Price.Minor()
			}
		case "price_desc":
			if variantCards[i].Price.Minor() != variantCards[j].Price.Minor() {
				return variantCards[i].Price.Minor() > variantCards[j].Price.Minor()
			}
		case "discount", "discount_desc":
			if variantCards[i].DiscountPercent != variantCards[j].DiscountPercent {
				return variantCards[i].DiscountPercent > variantCards[j].DiscountPercent
			}
		case "name":
			return variantCards[i].ProductNameAr < variantCards[j].ProductNameAr
		case "newest":
			return variantCards[i].ProductID > variantCards[j].ProductID
		default:
			// Default / "nearby": Nearest vendor first!
			if variantCards[i].DistanceKM > 0 && variantCards[j].DistanceKM > 0 && math.Abs(variantCards[i].DistanceKM-variantCards[j].DistanceKM) > 1.0 {
				return variantCards[i].DistanceKM < variantCards[j].DistanceKM
			}
			if variantCards[i].DistanceKM > 0 && variantCards[j].DistanceKM <= 0 {
				return true
			}
			if variantCards[i].DistanceKM <= 0 && variantCards[j].DistanceKM > 0 {
				return false
			}
			if variantCards[i].DiscountPercent != variantCards[j].DiscountPercent {
				return variantCards[i].DiscountPercent > variantCards[j].DiscountPercent
			}
		}

		return variantCards[i].ProductID < variantCards[j].ProductID
	})

	return variantCards
}
