package ui

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/muhiya/dawa24-store/internal/modules/promo"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// enrichAds populates SupplierName, SupplierLogo, PublicPrice, DiscountPercent, and SupplyPrice on Ads.
func (h *UIHandler) enrichAds(ctx context.Context, ads []*promo.Ad) {
	if len(ads) == 0 {
		return
	}

	sysCtx := database.AsSystem(ctx)

	for _, ad := range ads {
		if ad == nil {
			continue
		}

		// 1. Supplier Name & Logo
		if ad.OrganizationID != nil && *ad.OrganizationID > 0 && h.orgSvc != nil {
			if o, err := h.orgSvc.GetOrganization(sysCtx, *ad.OrganizationID); err == nil && o != nil {
				name := o.TradeName.Get("ar")
				if name == "" {
					name = o.LegalName
				}
				ad.SupplierName = name
			}
		}

		// 2. Pricing details (Public price, Discount, Net supply price)
		var variantID int64
		if ad.ClickTargetID != nil && *ad.ClickTargetID > 0 {
			variantID = *ad.ClickTargetID
		} else if strings.Contains(ad.TargetURL, "variant_id=") {
			parts := strings.Split(ad.TargetURL, "variant_id=")
			if len(parts) == 2 {
				if id, err := strconv.ParseInt(parts[1], 10, 64); err == nil {
					variantID = id
				}
			}
		} else if strings.Contains(ad.TargetURL, "/catalog/") {
			parts := strings.Split(ad.TargetURL, "/catalog/")
			if len(parts) == 2 {
				cleanID := strings.TrimSpace(parts[1])
				if id, err := strconv.ParseInt(cleanID, 10, 64); err == nil {
					variantID = id
				}
			}
		}

		if variantID > 0 && h.catSvc != nil {
			if variant, err := h.catSvc.GetVariant(sysCtx, variantID); err == nil && variant != nil {
				pubMinor := variant.Price.Minor()
				if pubMinor > 0 {
					ad.PublicPrice = fmt.Sprintf("%.2f ج.م", float64(pubMinor)/100.0)

					discountPct := 15.0
					if variant.CostDiscountPercentage > 0 {
						discountPct = variant.CostDiscountPercentage
					} else if variant.Discount.Minor() > 0 {
						discountPct = (float64(variant.Discount.Minor()) / float64(pubMinor)) * 100.0
					}

					netMinor := int64(float64(pubMinor) * (1.0 - (discountPct / 100.0)))
					if netMinor <= 0 {
						netMinor = pubMinor
					}

					ad.DiscountPercent = fmt.Sprintf("%.0f%%", discountPct)
					ad.SupplyPrice = fmt.Sprintf("%.2f ج.م", float64(netMinor)/100.0)
				}
			}
		}

		// Fallback pricing if not tied to variant
		if ad.PublicPrice == "" {
			ad.PublicPrice = "سعر رسمي معتمد"
		}
		if ad.DiscountPercent == "" {
			ad.DiscountPercent = "خصم تجاري خاص"
		}
		if ad.SupplyPrice == "" {
			ad.SupplyPrice = "أفضل سعر توريد"
		}
	}
}
