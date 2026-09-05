package ui

import (
	"context"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/modules/promo"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
)

// pharmacyCustomerBranch resolves the active pharmacy branch the customer is shopping for.
func (h *UIHandler) pharmacyCustomerBranch(ctx context.Context, actor *authctx.Actor) *org.Branch {
	if h.orgSvc == nil || actor == nil || actor.OrganizationID <= 0 {
		return nil
	}
	branchID := h.pharmacyBranchID(ctx, actor)
	if branchID <= 0 {
		return nil
	}
	branch, _ := h.orgSvc.GetBranch(ctx, branchID)
	return branch
}

// checkOfferCoverage evaluates whether a given offer covers the customer's pharmacy branch.
//
// Rules:
// 1. If the offer has specific location coverage rules in promo.offer_location_covers:
//   - The pharmacy branch is covered if its city_id matches any active location's city_id,
//     OR its coordinates fall within any active location's radius_meters.
//
// 2. If the offer has NO specific location rules:
//   - It falls back to the vendor's standard branch / weekly delivery coverage.
func (h *UIHandler) checkOfferCoverage(ctx context.Context, offer *promo.SpecialOffer, branch *org.Branch) (covered bool, reason string) {
	if offer == nil {
		return false, "بيانات العرض غير متوفرة"
	}
	if branch == nil {
		return false, "يرجى تحديد فرع صيدلية للاستلام أولاً"
	}

	// 1. Check offer-specific locations
	locations := offer.Locations
	if len(locations) == 0 && h.promoSvc != nil && offer.ID > 0 {
		if locs, err := h.promoSvc.ListSpecialOfferLocations(ctx, offer.ID); err == nil {
			locations = locs
		}
	}

	if len(locations) > 0 {
		hasActiveLocations := false
		for _, loc := range locations {
			if loc == nil || loc.Status != "active" {
				continue
			}
			hasActiveLocations = true

			// Check A: Direct City Match
			if loc.CityID != nil && branch.CityID != nil && *loc.CityID == *branch.CityID {
				return true, "مشمول بالتغطية في مدينتك"
			}

			// Check B: Spatial Distance Match
			if loc.Latitude != 0 && loc.Longitude != 0 && branch.Latitude != nil && branch.Longitude != nil &&
				(*branch.Latitude != 0 || *branch.Longitude != 0) {
				distKM := calculateHaversineKM(*branch.Latitude, *branch.Longitude, loc.Latitude, loc.Longitude)
				radiusKM := float64(loc.Radius) / 1000.0
				if radiusKM <= 0 {
					radiusKM = 1.0
				}
				if distKM <= radiusKM {
					return true, "مشمول بنطاق التغطية الجغرافي للعرض"
				}
			}
		}

		if hasActiveLocations {
			return false, "فرع الصيدلية خارج النطاق الجغرافي المحدد لهذا العرض"
		}
	}

	// 2. Fallback: No specific offer locations defined -> check vendor branch / weekly coverage
	if h.commSvc != nil && offer.OrganizationID > 0 {
		res, err := h.commSvc.CheckAvailability(ctx, commerce.AvailabilityRequest{
			VendorOrgID:      offer.OrganizationID,
			CustomerOrgID:    branch.OrganizationID,
			CustomerBranchID: branch.ID,
			Quantity:         1,
			When:             time.Now(),
		})
		if err == nil {
			if res.Allowed || res.Reason == commerce.ReasonOutOfStock || res.Reason == commerce.ReasonBelowMinimum {
				return true, "مشمول بجدول التوريد والتوصيل الأسبوعي للمورد"
			}
			if res.Reason == commerce.ReasonNotCovered || res.Reason == commerce.ReasonBranchNoLocation {
				return false, "فرع الصيدلية خارج نطاق تغطية المورد"
			}
		}
	}

	return true, "مشمول بالتغطية"
}
