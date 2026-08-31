package ui

import (
	"context"
	"math"
	"sort"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
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
				if inStock && off.AvailableStock <= 0 {
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
					SupplierID:      off.SupplierID,
					SupplierName:    off.SupplierName,
					SupplierRating:  off.SupplierRating,
					IsVerified:      off.IsVerified,
					BranchName:      off.BranchName,
					CityName:        off.CityName,
					GovernorateName: off.GovernorateName,
					DistanceKM:      off.DistanceKM,
					DistanceText:    off.DistanceText,
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
				})
			}
		} else {
			// Master product placeholder when no active offer
			if !inStock && !hasDiscount {
				variantCards = append(variantCards, &pages.SupplierVariantCard{
					ProductID:      p.ID,
					ProductNameAr:  p.Name.Get(i18n.AR),
					ProductNameEn:  p.Name.Get(i18n.EN),
					ProductImage:   p.Image,
					DosageForm:     p.DosageForm,
					Manufacturer:   p.ManufacturingCompanies,
					BrandID:        pBrandID,
					BrandName:      pBrandName,
					BrandLogo:      pBrandLogo,
					ScientificName: p.ScientificName,
					PublicPrice:    p.Price,
					Price:          p.Price,
					SupplierName:   i18n.T(lang, "customer.catalog.custom_procurement_request"),
					IsVerified:     true,
					DistanceText:   "-",
					CanAddToCart:   false,
				})
			}
		}
	}

	// Sponsorship ranking
	sponsoredProductIDs := make(map[int64]bool)
	if h.promoSvc != nil && len(productIDs) > 0 {
		rankings, err := h.promoSvc.RankedSponsorshipsForProducts(ctx, productIDs)
		if err == nil {
			for _, rs := range rankings {
				if rs != nil {
					sponsoredProductIDs[rs.ItemID] = true
				}
			}
		}
	}
	for _, vc := range variantCards {
		if vc != nil && sponsoredProductIDs[vc.ProductID] {
			vc.IsSponsored = true
		}
	}

	// 3. Prioritize variant cards: Sponsored first, then In-Stock & Covered, then Proximity
	sort.SliceStable(variantCards, func(i, j int) bool {
		if variantCards[i].IsSponsored != variantCards[j].IsSponsored {
			return variantCards[i].IsSponsored
		}
		if variantCards[i].IsSponsored && variantCards[j].IsSponsored {
			return variantCards[i].ProductID < variantCards[j].ProductID
		}
		// Tier 1: Actionable (In-stock & Covered)
		if variantCards[i].CanAddToCart != variantCards[j].CanAddToCart {
			return variantCards[i].CanAddToCart
		}
		if (variantCards[i].AvailableStock > 0) != (variantCards[j].AvailableStock > 0) {
			return variantCards[i].AvailableStock > 0
		}
		if variantCards[i].IsCovered != variantCards[j].IsCovered {
			return variantCards[i].IsCovered
		}

		// Tier 2: User sort or Proximity to Client
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
