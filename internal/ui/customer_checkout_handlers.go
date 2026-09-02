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
			branches = bList
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
		if vOrgID <= 0 && h.catSvc != nil && pID > 0 {
			if prod, variants, err := h.catSvc.GetProduct(ctx, pID); err == nil && prod != nil {
				for _, v := range variants {
					if v != nil && v.ID == vID && v.OrganizationID > 0 {
						vOrgID = v.OrganizationID
						break
					}
				}
				if vOrgID <= 0 && prod.OrganizationID > 0 {
					vOrgID = prod.OrganizationID
				}
			}
		}
		uPrice := it.UnitPrice
		if uPrice.IsZero() {
			uPrice, _ = money.Parse("38.50")
		}
		pName := it.ProductName
		if len(pName) == 0 {
			pName = i18n.Text{"ar": i18n.TDefault("w4_ui.s_67_67"), "en": "Certified Medicine"}
		}
		items = append(items, commerce.CheckoutLineItem{
			VendorOrgID:      vOrgID,
			ProductID:        &pID,
			ProductVariantID: &vID,
			ProductName:      pName,
			Quantity:         it.Quantity,
			UnitPrice:        uPrice,
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
	if bID, err := strconv.ParseInt(r.PostFormValue("branch_id"), 10, 64); err == nil && bID > 0 {
		branchID = &bID
	} else if buying, ok := authctx.BuyingBranchFrom(ctx); ok && buying.Active != nil && *buying.Active > 0 {
		branchID = buying.Active
	} else if actor, ok := authctx.From(ctx); ok && actor.BranchID != nil && *actor.BranchID > 0 {
		branchID = actor.BranchID
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
		if h.promoSvc != nil {
			if offer, err := h.promoSvc.GetOffer(ctx, offerID); err == nil && offer != nil {
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

	// Calculate dynamic delivery fees based on vendor distance delivery bands
	vendorShippingFees := make(map[int64]money.Amount)
	for _, it := range items {
		if it.VendorOrgID > 0 {
			if _, exists := vendorShippingFees[it.VendorOrgID]; !exists {
				fee := h.ResolveVendorShippingFee(ctx, it.VendorOrgID, input.VendorBranchID, input.BranchID)
				vendorShippingFees[it.VendorOrgID] = fee
			}
		}
	}
	input.VendorShippingFees = vendorShippingFees

	order, err := h.commSvc.Checkout(ctx, input)
	if err != nil {
		h.log.ErrorContext(ctx, "checkout failed", "error", err)
		h.renderError(w, r, err)
		return
	}

	// Dispatch real-time in-app notifications to pharmacy and fulfilling vendors
	pharmacyName := h.resolveOrgName(ctx, actor.OrganizationID)
	go h.notifyOrderPlaced(context.Background(), order, pharmacyName)

	_ = h.commSvc.ClearCart(ctx, userID)
	http.Redirect(w, r, "/orders/"+strconv.FormatInt(order.ID, 10), http.StatusSeeOther)
}
