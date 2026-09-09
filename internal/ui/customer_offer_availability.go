package ui

import (
	"context"
	"fmt"
	"net/http"

	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/modules/promo"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// checkSpecialOfferAvailability applies the same variant rule as ordinary cart
// lines to every product in a bundle. Offer-location coverage remains an
// additional promotion rule, but it cannot bypass branch ownership, stock,
// institutional connectivity, weekly coverage, or quota.
func (h *UIHandler) checkSpecialOfferAvailability(
	ctx context.Context, actor authctx.Actor, offer *promo.SpecialOffer, branchID int64, multiplier int,
) (commerce.AvailabilityResult, error) {
	if offer == nil || len(offer.Products) == 0 {
		return commerce.AvailabilityResult{Allowed: true}, nil
	}
	if multiplier <= 0 {
		multiplier = 1
	}
	for _, product := range offer.Products {
		if product == nil || product.VariantID <= 0 {
			continue
		}
		quantity := product.Quantity
		if quantity <= 0 {
			quantity = 1
		}
		result, err := h.commSvc.CheckAvailability(ctx, commerce.AvailabilityRequest{
			VariantID:        product.VariantID,
			VendorOrgID:      offer.OrganizationID,
			CustomerOrgID:    actor.OrganizationID,
			CustomerBranchID: branchID,
			Quantity:         quantity * multiplier,
		})
		if err != nil || !result.Allowed {
			return result, err
		}
	}
	return commerce.AvailabilityResult{Allowed: true}, nil
}

func (h *UIHandler) rejectOfferAvailability(
	w http.ResponseWriter, r *http.Request, offerID int64, result commerce.AvailabilityResult, err error,
) {
	message := result.MessageAr
	if message == "" {
		message = i18n.T(langOf(r), "offers.cov_reason_verify_failed")
	}
	if err != nil {
		message = i18n.T(langOf(r), "offers.cov_reason_verify_failed")
	}
	if h.isHTMX(r) {
		w.Header().Set("HX-Trigger", fmt.Sprintf(`{"showToast":{"message":%q,"type":"error"}}`, message))
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	h.redirectWithNotice(w, r, fmt.Sprintf("/offers/%d", offerID), "error", message)
}
