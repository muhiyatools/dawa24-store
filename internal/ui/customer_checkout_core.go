package ui

import (
	"context"
	"fmt"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// Placing an order from the cart, in two steps.
//
// planCheckout reads and validates everything — the cart, the payment method,
// the receiving branch, offer coverage, availability of every line, offer
// minimums, delivery quotes — and changes nothing. placeCheckout commits it.
//
// The checkout screen runs both in one request. The assistant runs planCheckout
// to render the confirmation card, and both again when the person confirms, so
// the order it places is the order the screen would have placed, validated
// against the data as it is at that moment.

// checkoutRequest is what the buyer chose on the checkout screen.
type checkoutRequest struct {
	// FormBranchID is the legacy form value; the buying-branch selection in the
	// context is authoritative when present.
	FormBranchID  string
	PaymentMethod string
	Notes         string
}

// checkoutFailure says where to send the buyer and why. A failure with neither
// Message nor Err is a silent redirect (an empty cart).
type checkoutFailure struct {
	Back    string
	Message string
	Err     error
}

// checkoutPlan is a validated checkout, ready to place.
type checkoutPlan struct {
	Cart     *commerce.Cart
	Items    []commerce.CheckoutLineItem
	Input    commerce.CheckoutInput
	BranchID int64
}

func (h *UIHandler) planCheckout(ctx context.Context, actor authctx.Actor, lang string, req checkoutRequest) (*checkoutPlan, *checkoutFailure) {
	if actor.UserID <= 0 || h.commSvc == nil {
		return nil, &checkoutFailure{Back: "/orders"}
	}
	cart, err := h.commSvc.GetCart(ctx, actor.UserID, buyerOrgID(ctx))
	if err != nil || cart == nil || len(cart.Items) == 0 {
		return nil, &checkoutFailure{Back: "/cart"}
	}

	items, offerID := h.prepareCheckoutItems(ctx, cart)

	paymentMethod := req.PaymentMethod
	if paymentMethod == "" {
		paymentMethod = "cod"
	}
	// The payment method must be active and enabled for checkout by the platform.
	if h.billSvc != nil {
		pm, err := h.billSvc.GetPlatformPaymentMethod(ctx, paymentMethod)
		if err == nil && pm != nil {
			if !pm.IsActive || !pm.IsCheckoutEnabled {
				return nil, &checkoutFailure{Back: "/checkout", Message: "طريقة الدفع المحددة غير متاحة حالياً عند طلب الشراء."}
			}
		} else if paymentMethod == "wallet" {
			return nil, &checkoutFailure{Back: "/checkout", Message: "طريقة الدفع عبر المحفظة غير متاحة حالياً عند طلب الشراء."}
		}
	}

	branchID := h.resolveCheckoutBranch(ctx, actor, req.FormBranchID)
	var targetBranchID int64
	if branchID != nil {
		targetBranchID = *branchID
	}
	if targetBranchID <= 0 {
		return nil, &checkoutFailure{Back: "/checkout", Message: i18n.T(lang, "buying.select_branch_first")}
	}

	if f := h.checkCartOfferCoverage(ctx, lang, cart, targetBranchID); f != nil {
		return nil, f
	}
	if f := h.checkCartAvailability(ctx, actor, lang, cart, targetBranchID); f != nil {
		return nil, f
	}

	input := commerce.CheckoutInput{
		CustomerID:    actor.UserID,
		BranchID:      branchID,
		PaymentMethod: paymentMethod,
		PaymentStatus: commerce.PaymentUnpaid,
		Notes:         req.Notes,
		Items:         items,
	}
	if actor.OrganizationID > 0 {
		input.CustomerOrgID = actor.OrganizationID
		input.CustomerOrgType = actor.OrgType
	}
	if offerID > 0 {
		if f := h.applyCheckoutOffer(ctx, offerID, &input); f != nil {
			return nil, f
		}
	}
	if input.BranchID == nil {
		if buying, ok := authctx.BuyingBranchFrom(ctx); ok && buying.Active != nil {
			input.BranchID = buying.Active
		}
	}
	h.resolveCheckoutVendors(ctx, items, &input)

	return &checkoutPlan{Cart: cart, Items: items, Input: input, BranchID: targetBranchID}, nil
}

// checkCartOfferCoverage re-runs the offer coverage rule for bundle lines,
// which carry no variant for the ordinary availability check to inspect.
func (h *UIHandler) checkCartOfferCoverage(ctx context.Context, lang string, cart *commerce.Cart, branchID int64) *checkoutFailure {
	if h.promoSvc == nil {
		return nil
	}
	checked := make(map[int64]bool)
	for _, it := range cart.Items {
		if it.OfferID == nil || *it.OfferID <= 0 || checked[*it.OfferID] {
			continue
		}
		checked[*it.OfferID] = true
		sp, err := h.promoSvc.GetSpecialOffer(ctx, *it.OfferID)
		if err != nil || sp == nil {
			continue
		}
		targetBranch, _ := h.orgSvc.GetBranch(ctx, branchID)
		if covered, reason := h.checkOfferCoverage(ctx, sp, targetBranch); !covered {
			if reason == "" {
				reason = i18n.T(lang, "offers.cov_reason_verify_failed")
			}
			return &checkoutFailure{Back: "/checkout", Message: reason}
		}
	}
	return nil
}

// checkCartAvailability applies commerce.CheckAvailability to every variant
// line for the receiving branch.
func (h *UIHandler) checkCartAvailability(ctx context.Context, actor authctx.Actor, lang string, cart *commerce.Cart, branchID int64) *checkoutFailure {
	for _, it := range cart.Items {
		// Lines with no variant (bundled offers) are validated at offer level.
		if it.ProductVariantID <= 0 {
			continue
		}
		vOrgID := it.OrganizationID
		if vOrgID <= 0 && h.catSvc != nil {
			if v, err := h.catSvc.GetVariant(database.AsSystem(ctx), it.ProductVariantID); err == nil && v != nil && v.OrganizationID > 0 {
				vOrgID = v.OrganizationID
			}
		}
		if vOrgID <= 0 {
			continue
		}
		res, err := h.commSvc.CheckAvailability(ctx, commerce.AvailabilityRequest{
			VariantID:        it.ProductVariantID,
			VendorOrgID:      vOrgID,
			CustomerOrgID:    actor.OrganizationID,
			CustomerBranchID: branchID,
			Quantity:         it.Quantity,
			When:             time.Now(),
		})
		if err == nil && !res.Allowed {
			return &checkoutFailure{Back: "/checkout",
				Message: fmt.Sprintf(i18n.T(lang, "checkout.branch_out_of_coverage_format"), res.Message(lang))}
		}
	}
	return nil
}

// applyCheckoutOffer makes the offer the authority for the minimum order
// amount and the fulfilling vendor branch.
//
// Cart bundle lines reference promo.special_offers rows, so the base
// promo.offers lookup alone misses them. The special offer is resolved first.
func (h *UIHandler) applyCheckoutOffer(ctx context.Context, offerID int64, input *commerce.CheckoutInput) *checkoutFailure {
	input.OfferID = offerID
	if h.promoSvc == nil {
		return nil
	}
	if spo, err := h.promoSvc.GetSpecialOffer(ctx, offerID); err == nil && spo != nil {
		if msg := validateSpecialOfferForCheckout(spo); msg != "" {
			return &checkoutFailure{Back: "/cart", Message: msg}
		}
		input.MinOrderAmount = spo.MinOrderAmount
		if spo.BranchID != nil && *spo.BranchID > 0 {
			input.VendorBranchID = spo.BranchID
		}
	} else if offer, err := h.promoSvc.GetOffer(ctx, offerID); err == nil && offer != nil {
		input.MinOrderAmount = offer.MinOrderAmount
		if offer.BranchID != nil && *offer.BranchID > 0 {
			input.VendorBranchID = offer.BranchID
		}
	}
	return nil
}

// resolveCheckoutVendors fills the fulfilling branch and the delivery quote
// per vendor. One quote per vendor, each measured from that vendor's own
// warehouse to the buyer's branch — a single branch taken from the first
// vendor in the cart used to price every supplier's delivery.
func (h *UIHandler) resolveCheckoutVendors(ctx context.Context, items []commerce.CheckoutLineItem, input *commerce.CheckoutInput) {
	if input.VendorBranchID == nil && len(items) > 0 && items[0].VendorOrgID > 0 && h.orgSvc != nil {
		if vBranches, err := h.orgSvc.ListBranches(ctx, items[0].VendorOrgID); err == nil && len(vBranches) > 0 {
			for _, vb := range vBranches {
				if vb.IsMain {
					input.VendorBranchID = &vb.ID
					break
				}
			}
			if input.VendorBranchID == nil {
				input.VendorBranchID = &vBranches[0].ID
			}
		}
	}
	fees := make(map[int64]money.Amount)
	branches := make(map[int64]*int64)
	for _, it := range items {
		if it.VendorOrgID <= 0 {
			continue
		}
		if _, exists := fees[it.VendorOrgID]; exists {
			continue
		}
		fees[it.VendorOrgID] = h.QuoteVendorDelivery(ctx, it.VendorOrgID, input.BranchID).Fee
		branches[it.VendorOrgID] = h.vendorFulfillingBranch(ctx, it.VendorOrgID)
	}
	input.VendorShippingFees = fees
	input.VendorBranchIDs = branches
}

// placeCheckout commits a plan: the wallet debit when paying by wallet, the
// order, the notifications, and the emptied cart. A failed order refunds the
// debit.
func (h *UIHandler) placeCheckout(ctx context.Context, actor authctx.Actor, lang string, plan *checkoutPlan) (*commerce.Order, *checkoutFailure) {
	input := plan.Input
	var walletUserID int64
	var goodsAmount money.Amount
	if input.PaymentMethod == "wallet" {
		goodsAmount = computeCheckoutGoodsAmount(plan.Items)
		var err error
		walletUserID, err = h.processWalletPayment(ctx, actor, goodsAmount)
		if err != nil {
			h.log.WarnContext(ctx, "checkout wallet payment rejected", "error", err)
			return nil, &checkoutFailure{Back: "/checkout", Message: h.safeMessage(err, lang)}
		}
		input.PaymentStatus = commerce.PaymentPaid
	}

	order, err := h.commSvc.Checkout(ctx, input)
	if err != nil {
		if walletUserID > 0 && goodsAmount.IsPositive() && h.billSvc != nil {
			_, _ = h.billSvc.Deposit(ctx, walletUserID, "EGP", goodsAmount, "refund", nil, "استرداد قيمة مشتريات لتعذر إتمام الطلب")
		}
		h.log.ErrorContext(ctx, "checkout failed", "error", err)
		// Validation failures carry stable codes: a specific Arabic message
		// tells the pharmacy what to fix (offer minimum, stock, ...).
		if msg, ok := checkoutValidationMessage(lang, err); ok {
			return nil, &checkoutFailure{Back: "/checkout", Message: msg}
		}
		return nil, &checkoutFailure{Back: "/checkout", Err: err}
	}

	pharmacyName := h.resolveOrgName(ctx, actor.OrganizationID)
	go h.notifyOrderPlaced(context.Background(), order, pharmacyName)

	_ = h.commSvc.ClearCart(ctx, actor.UserID)
	return order, nil
}
