package ui

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

func (h *UIHandler) CustomerCheckoutPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/checkout", http.StatusSeeOther)
		return
	}

	if !actor.IsCustomer() {
		h.redirectWithNotice(w, r, "/catalog", "error", i18n.T(lang, "checkout.pharmacy_only"))
		return
	}

	userID := actor.UserID

	if h.commSvc == nil {
		h.renderPage(ctx, w, "render checkout page", pages.CustomerCheckout(nil, nil, lang, dir))
		return
	}

	cart, err := h.commSvc.GetCart(ctx, userID)
	if err != nil {
		h.renderError(w, r, err)
		return
	}

	var branches []*org.Branch
	if h.orgSvc != nil && actor.OrganizationID > 0 {
		if bList, err := h.orgSvc.ListBranches(ctx, actor.OrganizationID); err != nil {
			h.log.WarnContext(ctx, "checkout: list customer branches", "error", err)
		} else {
			if actor.BranchID != nil && *actor.BranchID > 0 {
				for _, b := range bList {
					if b != nil && b.ID == *actor.BranchID {
						branches = append(branches, b)
						break
					}
				}
			} else {
				branches = bList
			}
		}
	}

	h.renderPage(ctx, w, "render checkout page", pages.CustomerCheckout(cart, branches, lang, dir))
}

func (h *UIHandler) CheckoutSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, _ := authctx.From(ctx)
	userID, err := authctx.UserID(ctx)
	if err != nil {
		http.Redirect(w, r, "/auth/login?redirect=/checkout", http.StatusSeeOther)
		return
	}

	if h.commSvc == nil {
		http.Redirect(w, r, "/orders", http.StatusSeeOther)
		return
	}

	cart, err := h.commSvc.GetCart(ctx, userID)
	if err != nil || cart == nil || len(cart.Items) == 0 {
		http.Redirect(w, r, "/cart", http.StatusSeeOther)
		return
	}

	var items []commerce.CheckoutLineItem
	var offerID int64
	for _, it := range cart.Items {
		pID := it.ProductID
		vID := it.ProductVariantID
		vOrgID := it.OrganizationID
		var listPrice money.Amount
		var discAmount money.Amount
		var costDiscPct float64

		if h.catSvc != nil && pID > 0 {
			if prod, variants, err := h.catSvc.GetProduct(ctx, pID); err == nil && prod != nil {
				if prod.Price.IsPositive() {
					listPrice = prod.Price
				}
				for _, v := range variants {
					if v != nil && v.ID == vID {
						if v.OrganizationID > 0 && vOrgID <= 0 {
							vOrgID = v.OrganizationID
						}
						if v.Price.IsPositive() {
							listPrice = v.Price
						}
						if v.CostDiscountPercentage > 0 {
							costDiscPct = v.CostDiscountPercentage
						} else if v.Discount.IsPositive() && listPrice.IsPositive() {
							costDiscPct = (float64(v.Discount.Minor()) / float64(listPrice.Minor())) * 100.0
						}
						break
					}
				}
				if vOrgID <= 0 && prod.OrganizationID > 0 {
					vOrgID = prod.OrganizationID
				}
			}
		}
		// Offer bundle lines carry no product reference: their vendor is the
		// offer's own organization. The cart read already resolves this via
		// the special_offers join; this is the second net for rows read
		// before that fix or with a stale join.
		if vOrgID <= 0 && it.OfferID != nil && *it.OfferID > 0 && h.promoSvc != nil {
			if spo, serr := h.promoSvc.GetSpecialOffer(ctx, *it.OfferID); serr == nil && spo != nil && spo.OrganizationID > 0 {
				vOrgID = spo.OrganizationID
				if spo.DiscountPercentage > 0 {
					costDiscPct = spo.DiscountPercentage
				}
			} else if offer, oerr := h.promoSvc.GetOffer(ctx, *it.OfferID); oerr == nil && offer != nil && offer.OrganizationID > 0 {
				vOrgID = offer.OrganizationID
				if offer.DiscountValue.IsPositive() {
					costDiscPct = float64(offer.DiscountValue.Minor()) / 100.0
				}
			}
		}
		uPrice := it.UnitPrice
		if uPrice.IsZero() {
			uPrice, _ = money.Parse("38.50")
		}
		if listPrice.IsZero() || listPrice.Minor() < uPrice.Minor() {
			listPrice = uPrice
		}
		if listPrice.Minor() > uPrice.Minor() {
			unitDisc := listPrice.Minor() - uPrice.Minor()
			discAmount = money.FromMinor(unitDisc * int64(it.Quantity))
			if costDiscPct <= 0 && listPrice.Minor() > 0 {
				costDiscPct = (float64(unitDisc) / float64(listPrice.Minor())) * 100.0
			}
		}
		pName := it.ProductName
		if len(pName) == 0 {
			pName = i18n.Text{"ar": i18n.TDefault("w4_ui.s_67_67"), "en": "Certified Medicine"}
		}
		var pIDPtr *int64
		if pID > 0 {
			pIDPtr = &pID
		}
		var vIDPtr *int64
		if vID > 0 {
			vIDPtr = &vID
		}
		items = append(items, commerce.CheckoutLineItem{
			VendorOrgID:            vOrgID,
			ProductID:              pIDPtr,
			ProductVariantID:       vIDPtr,
			ProductName:            pName,
			OfferProductID:         it.OfferID,
			Quantity:               it.Quantity,
			UnitPrice:              uPrice,
			ListPrice:              listPrice,
			OriginalPrice:          listPrice,
			DiscountAmount:         discAmount,
			CostDiscountPercentage: costDiscPct,
		})
		// One offer per order (main_orders parity). If the cart mixes offers,
		// the order degrades to a legacy non-offer order — the cart-per-offer
		// UI is Phase 5.
		if it.OfferID != nil {
			if offerID == 0 {
				offerID = *it.OfferID
			} else if offerID != *it.OfferID {
				offerID = 0
			}
		}
	}

	paymentMethod := "cod"

	var branchID *int64
	if actor, ok := authctx.From(ctx); ok && actor.BranchID != nil && *actor.BranchID > 0 {
		branchID = actor.BranchID
	} else if bID, err := strconv.ParseInt(r.PostFormValue("branch_id"), 10, 64); err == nil && bID > 0 {
		branchID = &bID
	} else if buying, ok := authctx.BuyingBranchFrom(ctx); ok && buying.Active != nil && *buying.Active > 0 {
		branchID = buying.Active
	}

	var targetBranchID int64
	if branchID != nil {
		targetBranchID = *branchID
	} else if actor, ok := authctx.From(ctx); ok {
		targetBranchID = h.pharmacyBranchID(ctx, &actor)
		if targetBranchID > 0 {
			branchID = &targetBranchID
		}
	}

	if targetBranchID <= 0 {
		h.redirectWithNotice(w, r, "/checkout", "error", "يرجى تحديد فرع صيدلية للاستلام أولاً للتمكن من إتمام الطلب")
		return
	}

	if actor, ok := authctx.From(ctx); ok && targetBranchID > 0 {
		for _, it := range cart.Items {
			// Lines with no variant (e.g. bundled offers) are validated at offer level
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
				CustomerBranchID: targetBranchID,
				Quantity:         it.Quantity,
				When:             time.Now(),
			})
			if err == nil && !res.Allowed {
				covReason := res.MessageAr
				if langOf(r) == "en" && res.MessageEn != "" {
					covReason = res.MessageEn
				}
				h.redirectWithNotice(w, r, "/checkout", "error", fmt.Sprintf(i18n.T(langOf(r), "checkout.branch_out_of_coverage_format"), covReason))
				return
			}
		}
	}

	input := commerce.CheckoutInput{
		CustomerID:    userID,
		BranchID:      branchID,
		PaymentMethod: paymentMethod,
		Notes:         r.PostFormValue("notes"),
		Items:         items,
	}
	if actor, ok := authctx.From(ctx); ok && actor.OrganizationID > 0 {
		input.CustomerOrgID = actor.OrganizationID
	}
	if offerID > 0 {
		input.OfferID = offerID
		// The offer is the authority for the minimum order amount and the
		// fulfilling vendor branch; the buying branch comes from the shell
		// selector, validated against the actor's own branches.
		//
		// Cart bundle lines reference promo.special_offers rows, so the base
		// promo.offers lookup alone misses them (and silently left the
		// minimum/branch unset). Resolve the special offer first.
		if h.promoSvc != nil {
			if spo, serr := h.promoSvc.GetSpecialOffer(ctx, offerID); serr == nil && spo != nil {
				if msg := validateSpecialOfferForCheckout(spo); msg != "" {
					h.redirectWithNotice(w, r, "/cart", "error", msg)
					return
				}
				input.MinOrderAmount = spo.MinOrderAmount
				if spo.BranchID != nil && *spo.BranchID > 0 {
					input.VendorBranchID = spo.BranchID
				}
			} else if offer, oerr := h.promoSvc.GetOffer(ctx, offerID); oerr == nil && offer != nil {
				input.MinOrderAmount = offer.MinOrderAmount
				if offer.BranchID != nil && *offer.BranchID > 0 {
					input.VendorBranchID = offer.BranchID
				}
			}
		}
	}

	if input.BranchID == nil {
		if buying, ok := authctx.BuyingBranchFrom(ctx); ok && buying.Active != nil {
			input.BranchID = buying.Active
		}
	}

	// Ensure vendor branch is resolved if vendor has branches
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

	// One quote per vendor, each measured from that vendor's own warehouse to
	// the pharmacy's branch. This used to pass input.VendorBranchID — a single
	// branch resolved from the FIRST vendor in the cart — into every call, so a
	// pharmacy buying from three suppliers paid three deliveries all priced as
	// if they shipped from the same place. See org/delivery_service.go.
	//
	// The same loop resolves each vendor's own fulfilling branch. Stamping every
	// shipment with input.VendorBranchID — one branch, taken from the first
	// vendor in the cart — told supplier B's parcel it shipped from supplier A's
	// warehouse.
	vendorShippingFees := make(map[int64]money.Amount)
	vendorBranches := make(map[int64]*int64)
	for _, it := range items {
		if it.VendorOrgID <= 0 {
			continue
		}
		if _, exists := vendorShippingFees[it.VendorOrgID]; exists {
			continue
		}
		vendorShippingFees[it.VendorOrgID] =
			h.QuoteVendorDelivery(ctx, it.VendorOrgID, input.BranchID).Fee
		vendorBranches[it.VendorOrgID] = h.vendorFulfillingBranch(ctx, it.VendorOrgID)
	}
	input.VendorShippingFees = vendorShippingFees
	input.VendorBranchIDs = vendorBranches

	order, err := h.commSvc.Checkout(ctx, input)
	if err != nil {
		h.log.ErrorContext(ctx, "checkout failed", "error", err)
		// Validation failures carry stable codes: surface a specific Arabic
		// message instead of the generic "بيانات الطلب غير صالحة" envelope
		// so the pharmacy knows what to fix (offer minimum, stock, ...).
		if msg, ok := checkoutValidationMessage(langOf(r), err); ok {
			h.redirectWithNotice(w, r, "/checkout", "error", msg)
			return
		}
		h.renderError(w, r, err)
		return
	}

	// Dispatch real-time in-app notifications to pharmacy and fulfilling vendors
	pharmacyName := h.resolveOrgName(ctx, actor.OrganizationID)
	go h.notifyOrderPlaced(context.Background(), order, pharmacyName)

	_ = h.commSvc.ClearCart(ctx, userID)
	http.Redirect(w, r, "/orders/"+strconv.FormatInt(order.ID, 10), http.StatusSeeOther)
}

// vendorFulfillingBranch picks the branch a vendor ships from: their main
// branch, or their first if none is marked main. nil means the vendor has no
// branches and the order-level branch stands.
func (h *UIHandler) vendorFulfillingBranch(ctx context.Context, vendorOrgID int64) *int64 {
	if h.orgSvc == nil || vendorOrgID <= 0 {
		return nil
	}
	branches, err := h.orgSvc.ListBranches(ctx, vendorOrgID)
	if err != nil || len(branches) == 0 {
		return nil
	}
	for _, b := range branches {
		if b.IsMain {
			id := b.ID
			return &id
		}
	}
	id := branches[0].ID
	return &id
}
