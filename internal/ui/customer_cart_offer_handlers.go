package ui

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/modules/promo"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
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

	// The base offer is always resolved, not only when the special-offer lookup
	// fails. Its ProductIDs are the fallback when the special offer carries no
	// product rows of its own -- which is exactly the case that used to fall
	// between the two branches below and report "add failed" for every offer on
	// the platform.
	baseOffer, baseErr := h.promoSvc.GetOffer(ctx, offerID)

	if err != nil || sp == nil {
		if baseErr != nil || baseOffer == nil {
			h.redirectWithNotice(w, r, "/offers", "error", i18n.T(langOf(r), "customer.offer.not_found"))
			return
		}
		sp = &promo.SpecialOffer{
			ID:                 baseOffer.ID,
			OrganizationID:     baseOffer.OrganizationID,
			Title:              baseOffer.Title,
			DiscountPercentage: float64(baseOffer.DiscountValue.Minor()) / 100.0,
		}
	}

	addedItemsCount := 0
	if len(sp.Products) > 0 {
		for _, p := range sp.Products {
			if p == nil {
				continue
			}

			variantID := p.VariantID
			prodID := p.ProductID
			unitPrice := p.CustomPrice

			// 1. If variant ID is missing but we have prodID, resolve variant from catalog
			if variantID <= 0 && h.catSvc != nil && prodID > 0 {
				if _, vars, vErr := h.catSvc.GetProduct(database.AsSystem(ctx), prodID); vErr == nil && len(vars) > 0 {
					for _, v := range vars {
						if v.OrganizationID == sp.OrganizationID || variantID <= 0 {
							variantID = v.ID
							if unitPrice.IsZero() {
								unitPrice = v.Price
							}
							break
						}
					}
					if variantID <= 0 {
						variantID = vars[0].ID
						if unitPrice.IsZero() {
							unitPrice = vars[0].Price
						}
					}
				}
			}

			// 2. If variant ID is present, make sure we have prodID and fallback price
			if variantID > 0 && h.catSvc != nil {
				if v, vErr := h.catSvc.GetVariant(database.AsSystem(ctx), variantID); vErr == nil && v != nil {
					if prodID <= 0 {
						prodID = v.ProductID
					}
					if unitPrice.IsZero() {
						unitPrice = v.Price
					}
				}
			}

			// 3. If variant ID is still missing (e.g. only VariantName / SKU or product name exists)
			if variantID <= 0 && h.catSvc != nil && p.VariantName != "" {
				if prods, sErr := h.catSvc.Search(database.AsSystem(ctx), catalog.SearchParams{Query: p.VariantName, Limit: 5}); sErr == nil && len(prods) > 0 {
					pID := prods[0].ID
					if _, vars, vErr := h.catSvc.GetProduct(database.AsSystem(ctx), pID); vErr == nil && len(vars) > 0 {
						prodID = pID
						variantID = vars[0].ID
						if unitPrice.IsZero() {
							unitPrice = vars[0].Price
						}
					}
				}
			}

			if variantID <= 0 || prodID <= 0 {
				h.log.WarnContext(ctx, "skipping offer product due to unresolvable variant/product", "offer_id", offerID, "variant_id", variantID, "product_id", prodID, "variant_name", p.VariantName)
				continue
			}

			// 4. Calculate unit price
			if unitPrice.IsZero() && p.OriginalPrice.IsPositive() {
				if p.DiscountPercentage > 0 {
					discMinor := int64(float64(p.OriginalPrice.Minor()) * (p.DiscountPercentage / 100.0))
					unitPrice = money.FromMinor(p.OriginalPrice.Minor() - discMinor)
				} else if sp.DiscountPercentage > 0 {
					discMinor := int64(float64(p.OriginalPrice.Minor()) * (sp.DiscountPercentage / 100.0))
					unitPrice = money.FromMinor(p.OriginalPrice.Minor() - discMinor)
				} else if p.DiscountAmount.IsPositive() && p.DiscountAmount.Minor() < p.OriginalPrice.Minor() {
					unitPrice = money.FromMinor(p.OriginalPrice.Minor() - p.DiscountAmount.Minor())
				} else {
					unitPrice = p.OriginalPrice
				}
			}

			if unitPrice.IsZero() && sp.TotalPrice.IsPositive() && len(sp.Products) > 0 {
				unitPrice = money.FromMinor(sp.TotalPrice.Minor() / int64(len(sp.Products)))
			}

			if unitPrice.IsZero() {
				unitPrice = money.FromMinor(100) // 1 EGP fallback
			}

			qty := p.Quantity * bundleMultiplier
			if qty <= 0 {
				qty = bundleMultiplier
			}

			item := &commerce.CartItem{
				ProductID:        prodID,
				ProductVariantID: variantID,
				OrganizationID:   sp.OrganizationID,
				Quantity:         qty,
				UnitPrice:        unitPrice,
				OfferID:          &offerID,
			}

			if _, aErr := h.commSvc.AddToCart(ctx, userID, item); aErr == nil {
				addedItemsCount++
			} else {
				h.log.ErrorContext(ctx, "add offer item to cart error", "error", aErr, "variant_id", variantID, "product_id", prodID, "user_id", userID)
			}
		}
	} else if baseErr == nil && baseOffer != nil && len(baseOffer.ProductIDs) > 0 && h.catSvc != nil {
		for _, pID := range baseOffer.ProductIDs {
			if _, vars, vErr := h.catSvc.GetProduct(database.AsSystem(ctx), pID); vErr == nil && len(vars) > 0 {
				var targetVar *catalog.ProductVariant
				for _, v := range vars {
					if v.OrganizationID == baseOffer.OrganizationID {
						targetVar = v
						break
					}
				}
				if targetVar == nil {
					targetVar = vars[0]
				}
				if targetVar != nil {
					uPrice := targetVar.Price
					if sp.DiscountPercentage > 0 && uPrice.IsPositive() {
						discMinor := int64(float64(uPrice.Minor()) * (sp.DiscountPercentage / 100.0))
						uPrice = money.FromMinor(uPrice.Minor() - discMinor)
					}
					if uPrice.IsZero() {
						uPrice = money.FromMinor(100)
					}
					item := &commerce.CartItem{
						ProductID:        pID,
						ProductVariantID: targetVar.ID,
						OrganizationID:   baseOffer.OrganizationID,
						Quantity:         bundleMultiplier,
						UnitPrice:        uPrice,
						OfferID:          &offerID,
					}
					if _, aErr := h.commSvc.AddToCart(ctx, userID, item); aErr == nil {
						addedItemsCount++
					} else {
						h.log.ErrorContext(ctx, "add base offer item to cart error", "error", aErr, "variant_id", targetVar.ID, "product_id", pID, "user_id", userID)
					}
				}
			}
		}
	} else if h.catSvc != nil && sp.OrganizationID > 0 {
		// Fallback: If offer has no explicit products assigned, pick top vendor catalog products
		if vars, _, vErr := h.catSvc.ListVariantsByOrganization(ctx, sp.OrganizationID, catalog.VariantSearchParams{Limit: 5}); vErr == nil && len(vars) > 0 {
			for _, v := range vars {
				if v == nil || v.ID <= 0 {
					continue
				}
				uPrice := v.Price
				if sp.DiscountPercentage > 0 && uPrice.IsPositive() {
					discMinor := int64(float64(uPrice.Minor()) * (sp.DiscountPercentage / 100.0))
					uPrice = money.FromMinor(uPrice.Minor() - discMinor)
				}
				if uPrice.IsZero() {
					uPrice = money.FromMinor(100)
				}
				item := &commerce.CartItem{
					ProductID:        v.ProductID,
					ProductVariantID: v.ID,
					OrganizationID:   sp.OrganizationID,
					Quantity:         bundleMultiplier,
					UnitPrice:        uPrice,
					OfferID:          &offerID,
				}
				if _, aErr := h.commSvc.AddToCart(ctx, userID, item); aErr == nil {
					addedItemsCount++
				}
			}
		}
	}

	// Record offer conversion / click
	_ = h.promoSvc.RecordOfferClick(ctx, offerID)

	if addedItemsCount == 0 {
		// An offer with nothing in it is a supplier configuration problem, not
		// something the buyer can retry their way out of. It gets its own
		// message, and a log line loud enough to find the offer.
		reasonKey := "customer.offer.add_failed"
		emptyOffer := len(sp.Products) == 0 && (baseOffer == nil || len(baseOffer.ProductIDs) == 0)
		if emptyOffer {
			reasonKey = "customer.offer.no_products"
			h.log.WarnContext(ctx, "offer has no purchasable items",
				"offer_id", offerID, "organization_id", sp.OrganizationID)
		}

		if h.isHTMX(r) {
			w.Header().Set("HX-Trigger", fmt.Sprintf(`{"showToast":{"message":%q,"type":"error"}}`, i18n.T(langOf(r), reasonKey)))
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		h.redirectWithNotice(w, r, fmt.Sprintf("/offers/%d", offerID), "error", i18n.T(langOf(r), reasonKey))
		return
	}

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
