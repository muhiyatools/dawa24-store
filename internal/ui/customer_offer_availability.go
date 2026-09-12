package ui

import (
	"context"
	"fmt"
	"net/http"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/modules/promo"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// checkSpecialOfferAvailability applies the same variant rule as ordinary cart
// lines to every product in a bundle. Offer-location coverage remains an
// additional promotion rule, but it cannot bypass branch ownership, stock,
// institutional connectivity, weekly coverage, or quota.
func (h *UIHandler) checkSpecialOfferAvailability(
	ctx context.Context, actor authctx.Actor, offer *promo.SpecialOffer, branchID int64, multiplier int,
) (commerce.AvailabilityResult, error) {
	if offer == nil || len(offer.Products) == 0 || h.commSvc == nil {
		return commerce.AvailabilityResult{Allowed: true}, nil
	}
	if multiplier <= 0 {
		multiplier = 1
	}
	for _, product := range offer.Products {
		if product == nil {
			continue
		}

		variantID := product.VariantID
		// If variant was deleted or missing, attempt to heal using vendor's active variant for this product
		if variantID <= 0 && product.ProductID > 0 && h.catSvc != nil {
			if vars, err := h.catSvc.ListVariantsByProduct(database.AsSystem(ctx), product.ProductID); err == nil {
				for _, v := range vars {
					if v != nil && v.OrganizationID == offer.OrganizationID && v.DeletedAt == nil && v.Status == catalog.StatusActive {
						variantID = v.ID
						product.VariantID = v.ID
						break
					}
				}
			}
		}

		if variantID <= 0 {
			prodName := product.VariantName
			if prodName == "" {
				prodName = fmt.Sprintf("#%d", product.ProductID)
			}
			return commerce.AvailabilityResult{
				Allowed:   false,
				Reason:    commerce.ReasonVariantInvalid,
				MessageAr: fmt.Sprintf("عذراً، الصنف (%s) المشمول في هذا العرض لم يعد متاحاً لدى المورد.", prodName),
				MessageEn: fmt.Sprintf("Item (%s) in this offer is no longer available from the supplier.", prodName),
			}, nil
		}

		quantity := product.Quantity
		if quantity <= 0 {
			quantity = 1
		}
		result, err := h.commSvc.CheckAvailability(ctx, commerce.AvailabilityRequest{
			VariantID:        variantID,
			VendorOrgID:      offer.OrganizationID,
			CustomerOrgID:    actor.OrganizationID,
			CustomerBranchID: branchID,
			Quantity:         quantity * multiplier,
		})
		if err != nil || !result.Allowed {
			if result.Reason == commerce.ReasonVariantInvalid || result.Reason == commerce.ReasonVariantInactive || result.Reason == commerce.ReasonOutOfStock || result.Reason == commerce.ReasonWrongVendor {
				prodName := product.VariantName
				if prodName == "" {
					prodName = fmt.Sprintf("#%d", variantID)
				}
				if result.Reason == commerce.ReasonOutOfStock {
					result.MessageAr = fmt.Sprintf("عذراً، نفدت كمية الصنف (%s) المشمول في هذا العرض لدى المورد.", prodName)
				} else {
					result.MessageAr = fmt.Sprintf("عذراً، الصنف (%s) المشمول في هذا العرض غير متاح حالياً لدى المورد.", prodName)
				}
			}
			return result, err
		}
	}
	return commerce.AvailabilityResult{Allowed: true}, nil
}

func (h *UIHandler) rejectOfferAvailability(
	w http.ResponseWriter, r *http.Request, offerID int64, result commerce.AvailabilityResult, err error,
) {
	message := result.Message(langOf(r))
	if message == "" || err != nil {
		message = i18n.T(langOf(r), "offers.cov_reason_verify_failed")
	}
	if h.isHTMX(r) {
		w.Header().Set("HX-Trigger", fmt.Sprintf(`{"showToast":{"message":%+q,"type":"error"}}`, message))
		w.WriteHeader(http.StatusUnprocessableEntity)
		return
	}
	h.redirectWithNotice(w, r, fmt.Sprintf("/offers/%d", offerID), "error", message)
}
