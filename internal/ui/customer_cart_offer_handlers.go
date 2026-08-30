package ui

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// AddOfferToCartSubmit adds an entire special offer bundle to the cart for a pharmacy.
func (h *UIHandler) AddOfferToCartSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
		if h.isHTMX(r) {
			w.Header().Set("HX-Redirect", "/auth/login?redirect=/offers")
			w.Header().Set("HX-Trigger", fmt.Sprintf(`{"showToast":{"message":%q,"type":"error"}}`, i18n.T(langOf(r), "customer.offer.login_required")))
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		http.Redirect(w, r, "/auth/login?redirect=/offers", http.StatusSeeOther)
		return
	}

	if !actor.IsCustomer() {
		if h.isHTMX(r) {
			w.Header().Set("HX-Trigger", fmt.Sprintf(`{"showToast":{"message":%q,"type":"error"}}`, i18n.T(langOf(r), "customer.offer.buy_pharmacy_only")))
			w.WriteHeader(http.StatusForbidden)
			return
		}
		h.redirectWithNotice(w, r, "/offers", "error", i18n.T(langOf(r), "customer.offer.buy_pharmacy_only_notice"))
		return
	}

	userID := actor.UserID
	if h.commSvc == nil || h.promoSvc == nil {
		if h.isHTMX(r) {
			w.Header().Set("HX-Trigger", fmt.Sprintf(`{"showToast":{"message":%q,"type":"error"}}`, i18n.T(langOf(r), "customer.cart.service_unavailable")))
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		http.Redirect(w, r, "/cart", http.StatusSeeOther)
		return
	}

	offerID, _ := strconv.ParseInt(r.PostFormValue("offer_id"), 10, 64)
	if offerID <= 0 {
		h.redirectWithNotice(w, r, "/offers", "error", i18n.T(langOf(r), "customer.offer.invalid_id"))
		return
	}

	bundleMultiplier, _ := strconv.Atoi(r.PostFormValue("quantity"))
	if bundleMultiplier <= 0 {
		bundleMultiplier, _ = strconv.Atoi(r.PostFormValue("qty"))
	}
	if bundleMultiplier <= 0 {
		bundleMultiplier = 1
	}

	sp, err := h.promoSvc.GetSpecialOffer(ctx, offerID)

	// The base offer is the authority on price and ownership. The special-offer
	// view adds the manifest -- the products the buyer receives -- which is
	// display information, not something to charge for line by line.
	baseOffer, baseErr := h.promoSvc.GetOffer(ctx, offerID)

	if (err != nil || sp == nil) && (baseErr != nil || baseOffer == nil) {
		h.redirectWithNotice(w, r, "/offers", "error", i18n.T(langOf(r), "customer.offer.not_found"))
		return
	}

	// Whichever view resolved, reduce both to the three facts a cart line needs.
	var (
		orgID      int64
		unitPrice  money.Amount
		offerTitle string
	)
	switch {
	case sp != nil:
		orgID = sp.OrganizationID
		unitPrice = sp.TotalPrice
		offerTitle = sp.Title.Get(i18n.ParseLang(langOf(r)))
	default:
		orgID = baseOffer.OrganizationID
		offerTitle = baseOffer.Title.Get(i18n.ParseLang(langOf(r)))
	}

	// An offer priced at zero is a configuration mistake, not a free item.
	// Charging nothing for it would be worse than refusing.
	if !unitPrice.IsPositive() {
		h.log.WarnContext(ctx, "offer has no price and cannot be sold",
			"offer_id", offerID, "organization_id", orgID)
		h.offerAddFailed(w, r, offerID, "customer.offer.unpriced")
		return
	}

	item := &commerce.CartItem{
		OrganizationID: orgID,
		Quantity:       bundleMultiplier,
		UnitPrice:      unitPrice,
		OfferID:        &offerID,
	}

	if _, aErr := h.commSvc.AddToCart(ctx, userID, item); aErr != nil {
		h.log.ErrorContext(ctx, "add offer to cart",
			"error", aErr, "offer_id", offerID, "user_id", userID)
		h.offerAddFailed(w, r, offerID, "customer.offer.add_failed")
		return
	}
	_ = offerTitle

	// Record offer conversion / click
	_ = h.promoSvc.RecordOfferClick(ctx, offerID)

	if h.isHTMX(r) {
		cart, _ := h.commSvc.GetCart(ctx, userID)
		totalCount := 0
		if cart != nil {
			for _, ci := range cart.Items {
				totalCount += ci.Quantity
			}
		}
		w.Header().Set("HX-Trigger", fmt.Sprintf(`{"showToast":{"message":%q,"type":"success"},"cartUpdated":{"count":%d}}`, i18n.T(langOf(r), "customer.offer.add_success"), totalCount))
		w.WriteHeader(http.StatusOK)
		return
	}

	h.redirectWithNotice(w, r, "/cart", "success", i18n.T(langOf(r), "customer.offer.add_success"))
}

// assertCartLineAvailable runs the availability rule and, when it refuses,
// redirects with the reason. It returns false when the caller must stop.
//
// Every buying surface goes through here so the rules cannot drift: the cart,
// the quantity controls and checkout all ask the same question.
func (h *UIHandler) assertCartLineAvailable(
	w http.ResponseWriter, r *http.Request, actor authctx.Actor,
	variantID, vendorOrgID int64, qty int, back string,
) bool {
	ctx := r.Context()

	branchID := h.pharmacyBranchID(ctx, &actor)

	res, err := h.commSvc.CheckAvailability(ctx, commerce.AvailabilityRequest{
		VariantID:        variantID,
		VendorOrgID:      vendorOrgID,
		CustomerOrgID:    actor.OrganizationID,
		CustomerBranchID: branchID,
		Quantity:         qty,
		When:             time.Now(),
	})
	if err != nil {
		// A failed check is not permission to buy.
		h.log.ErrorContext(ctx, "availability check failed", "error", err,
			"variant", variantID, "vendor", vendorOrgID, "branch", branchID)
		if h.isHTMX(r) {
			w.Header().Set("HX-Trigger", fmt.Sprintf(`{"showToast":{"message":%q,"type":"error"}}`, i18n.T(langOf(r), "customer.cart.availability_check_failed")))
			w.WriteHeader(http.StatusBadRequest)
			return false
		}
		h.redirectWithNotice(w, r, back, "error",
			i18n.T(langOf(r), "customer.cart.availability_check_failed"))
		return false
	}
	if !res.Allowed {
		h.log.InfoContext(ctx, "cart line refused", "reason", res.Reason,
			"variant", variantID, "vendor", vendorOrgID, "branch", branchID, "qty", qty)
		if h.isHTMX(r) {
			if back == "/cart" {
				cart, _ := h.commSvc.GetCart(ctx, actor.UserID)
				lang, _ := h.localeAndDir(r)
				w.Header().Set("HX-Trigger", fmt.Sprintf(`{"showToast":{"message":%q,"type":"error"}}`, res.MessageAr))
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				_ = pages.CustomerCartContent(cart, lang).Render(ctx, w)
				return false
			}
			w.Header().Set("HX-Trigger", fmt.Sprintf(`{"showToast":{"message":%q,"type":"error"}}`, res.MessageAr))
			w.WriteHeader(http.StatusBadRequest)
			return false
		}
		h.redirectWithNotice(w, r, back, "error", res.MessageAr)
		return false
	}
	return true
}

// offerAddFailed reports why an offer could not be added, over HTMX or a
// redirect, so the two paths cannot drift apart.
func (h *UIHandler) offerAddFailed(w http.ResponseWriter, r *http.Request, offerID int64, key string) {
	msg := i18n.T(langOf(r), key)
	if h.isHTMX(r) {
		w.Header().Set("HX-Trigger", fmt.Sprintf(`{"showToast":{"message":%q,"type":"error"}}`, msg))
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	h.redirectWithNotice(w, r, fmt.Sprintf("/offers/%d", offerID), "error", msg)
}
