package ui

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

func (h *UIHandler) CustomerCartPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/cart", http.StatusSeeOther)
		return
	}

	if !actor.IsCustomer() {
		h.redirectWithNotice(w, r, "/catalog", "error", "عذراً، الشراء وسلة الطلبات متاحة حصرياً للصيدليات المرخصة.")
		return
	}

	if h.commSvc == nil {
		h.renderPage(ctx, w, "render cart page", pages.CustomerCart(nil, lang, dir, h.isHTMX(r)))
		return
	}

	cart, err := h.commSvc.GetCart(ctx, actor.UserID)
	if err != nil {
		h.renderError(w, r, err)
		return
	}

	if cart != nil && len(cart.Items) > 0 {
		branchID := h.pharmacyBranchID(ctx, &actor)
		for _, it := range cart.Items {
			it.IsCovered = true
			if branchID > 0 && it.ProductVariantID > 0 && it.OrganizationID > 0 {
				res, err := h.commSvc.CheckAvailability(ctx, commerce.AvailabilityRequest{
					VariantID:        it.ProductVariantID,
					VendorOrgID:      it.OrganizationID,
					CustomerOrgID:    actor.OrganizationID,
					CustomerBranchID: branchID,
					Quantity:         it.Quantity,
					When:             time.Now(),
				})
				if err == nil {
					if !res.Allowed {
						if res.Reason == commerce.ReasonNotCovered || res.Reason == commerce.ReasonBranchNoLocation {
							it.IsCovered = false
							it.CoverageReason = "خارج نطاق التغطية للفرع المحدد"
						} else if res.Reason == commerce.ReasonOutOfStock || res.Reason == commerce.ReasonInsufficientStock {
							it.CoverageReason = "نفد المخزون لدى المورد"
						}
					}
				}
			}
		}
	}

	h.renderPage(ctx, w, "render cart page", pages.CustomerCart(cart, lang, dir, h.isHTMX(r)))
}

func (h *UIHandler) AddToCartSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
		if h.isHTMX(r) {
			w.Header().Set("HX-Redirect", "/auth/login?redirect=/cart")
			w.Header().Set("HX-Trigger", `{"showToast":{"message":"يرجى تسجيل الدخول كصيدلية مرخصة للشراء","type":"error"}}`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		http.Redirect(w, r, "/auth/login?redirect=/cart", http.StatusSeeOther)
		return
	}

	if !actor.IsCustomer() {
		if h.isHTMX(r) {
			w.Header().Set("HX-Trigger", `{"showToast":{"message":"عذراً، إضافة الأدوية وسلة المشتريات متاحة حصرياً لحسابات الصيدليات المرخصة","type":"error"}}`)
			w.WriteHeader(http.StatusForbidden)
			return
		}
		h.redirectWithNotice(w, r, "/catalog", "error", "عذراً، إضافة الأدوية وطلب التوريد متاح حصرياً للصيدليات المرخصة.")
		return
	}

	userID := actor.UserID
	if h.commSvc == nil {
		if h.isHTMX(r) {
			w.Header().Set("HX-Trigger", `{"showToast":{"message":"خدمة السلة غير متوفرة حالياً","type":"error"}}`)
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		http.Redirect(w, r, "/cart", http.StatusSeeOther)
		return
	}

	variantID, _ := strconv.ParseInt(r.PostFormValue("variant_id"), 10, 64)
	productID, _ := strconv.ParseInt(r.PostFormValue("product_id"), 10, 64)
	vendorOrgID, _ := strconv.ParseInt(r.PostFormValue("vendor_org_id"), 10, 64)
	if vendorOrgID <= 0 {
		vendorOrgID, _ = strconv.ParseInt(r.PostFormValue("organization_id"), 10, 64)
	}

	qty, _ := strconv.Atoi(r.PostFormValue("quantity"))
	if qty <= 0 {
		qty, _ = strconv.Atoi(r.PostFormValue("qty"))
	}
	if qty <= 0 {
		qty = 1
	}

	back := strings.TrimSpace(r.PostFormValue("return_to"))
	if back == "" {
		back = r.Header.Get("Referer")
	}
	if back == "" {
		back = "/cart"
	}

	// Auto-resolve missing product/vendor info from variant
	if h.catSvc != nil && variantID > 0 && (productID <= 0 || vendorOrgID <= 0) {
		if v, err := h.catSvc.GetVariant(database.AsSystem(ctx), variantID); err == nil && v != nil {
			if productID <= 0 {
				productID = v.ProductID
			}
			if vendorOrgID <= 0 {
				vendorOrgID = v.OrganizationID
			}
		}
	}

	// Stock, supplier approval, branch ownership and weekly coverage are all
	// decided by commerce.CheckAvailability. Nothing here defaults a missing
	// supplier or quietly reduces the quantity the pharmacy asked for.
	if !h.assertCartLineAvailable(w, r, actor, variantID, vendorOrgID, qty, back) {
		return
	}

	item := &commerce.CartItem{
		ProductID:        productID,
		ProductVariantID: variantID,
		OrganizationID:   vendorOrgID,
		Quantity:         qty,
	}

	// Keep the offer identity and custom offer price
	if offerID, err := strconv.ParseInt(r.PostFormValue("offer_id"), 10, 64); err == nil && offerID > 0 {
		item.OfferID = &offerID
	}
	if offerPriceStr := strings.TrimSpace(r.PostFormValue("offer_price")); offerPriceStr != "" {
		if amt, err := money.Parse(offerPriceStr); err == nil && amt.IsPositive() {
			item.UnitPrice = amt
		}
	} else if customPriceStr := strings.TrimSpace(r.PostFormValue("custom_price")); customPriceStr != "" {
		if amt, err := money.Parse(customPriceStr); err == nil && amt.IsPositive() {
			item.UnitPrice = amt
		}
	}

	// Authoritative catalog price lookup if unit price is not set
	if item.UnitPrice.IsZero() && h.catSvc != nil {
		if variantID > 0 {
			if v, err := h.catSvc.GetVariant(database.AsSystem(ctx), variantID); err == nil && v != nil && !v.Price.IsZero() {
				item.UnitPrice = v.Price
			}
		}
		if item.UnitPrice.IsZero() && productID > 0 {
			if prod, _, err := h.catSvc.GetProduct(database.AsSystem(ctx), productID); err == nil && prod != nil {
				item.ProductName = prod.Name
				item.UnitPrice = prod.EffectivePrice()
			}
		}
	}

	if _, err := h.commSvc.AddToCart(ctx, userID, item); err != nil {
		h.log.ErrorContext(ctx, "add to cart", "error", err,
			"user", userID, "variant", variantID, "vendor", vendorOrgID)
		if h.isHTMX(r) {
			w.Header().Set("HX-Trigger", fmt.Sprintf(`{"showToast":{"message":%q,"type":"error"}}`, h.safeMessage(err, langOf(r))))
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		h.redirectWithNotice(w, r, back, "error", h.safeMessage(err, langOf(r)))
		return
	}

	if h.isHTMX(r) {
		cart, _ := h.commSvc.GetCart(ctx, userID)
		itemCount := 0
		if cart != nil {
			for _, ci := range cart.Items {
				itemCount += ci.Quantity
			}
		}
		w.Header().Set("HX-Trigger", fmt.Sprintf(`{"showToast":{"message":"تمت إضافة الصنف إلى سلة المشتريات بنجاح","type":"success"},"cartUpdated":{"count":%d}}`, itemCount))
		w.WriteHeader(http.StatusOK)
		return
	}

	if returnTo := strings.TrimSpace(r.PostFormValue("return_to")); returnTo != "" {
		h.redirectWithNotice(w, r, returnTo, "success", "تمت إضافة الصنف إلى سلة المشتريات بنجاح.")
		return
	}

	http.Redirect(w, r, "/cart", http.StatusSeeOther)
}

func (h *UIHandler) RemoveFromCartSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID, err := authctx.UserID(ctx)
	if err != nil {
		http.Redirect(w, r, "/auth/login?redirect=/cart", http.StatusSeeOther)
		return
	}

	if h.commSvc == nil {
		http.Redirect(w, r, "/cart", http.StatusSeeOther)
		return
	}

	variantID, _ := strconv.ParseInt(r.PostFormValue("variant_id"), 10, 64)
	_, _ = h.commSvc.RemoveFromCart(ctx, userID, variantID)

	if h.isHTMX(r) {
		cart, _ := h.commSvc.GetCart(ctx, userID)
		lang, _ := h.localeAndDir(r)
		h.renderPage(ctx, w, "render customer cart content", pages.CustomerCartContent(cart, lang))
		return
	}

	http.Redirect(w, r, "/cart", http.StatusSeeOther)
}

func (h *UIHandler) UpdateCartQuantitySubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID, err := authctx.UserID(ctx)
	if err != nil {
		http.Redirect(w, r, "/auth/login?redirect=/cart", http.StatusSeeOther)
		return
	}

	if h.commSvc == nil {
		http.Redirect(w, r, "/cart", http.StatusSeeOther)
		return
	}

	variantID, _ := strconv.ParseInt(r.PostFormValue("variant_id"), 10, 64)
	qty, _ := strconv.Atoi(r.PostFormValue("quantity"))
	if qty < 0 {
		qty = 0
	}

	// Raising a quantity is a purchase decision and gets the same check as
	// adding the line. The client's "+" button is a hint; this is the rule.
	// Quantity 0 means "remove", which needs no availability check.
	if qty > 0 {
		actor, ok := authctx.From(ctx)
		if !ok {
			http.Redirect(w, r, "/auth/login?redirect=/cart", http.StatusSeeOther)
			return
		}
		vendorOrgID, _ := strconv.ParseInt(r.PostFormValue("vendor_org_id"), 10, 64)
		if vendorOrgID <= 0 {
			// The cart row knows its supplier even when the form omits it.
			if line, err := h.commSvc.GetCartLine(ctx, userID, variantID); err == nil && line != nil {
				vendorOrgID = line.OrganizationID
			}
		}
		if !h.assertCartLineAvailable(w, r, actor, variantID, vendorOrgID, qty, "/cart") {
			return
		}
	}

	if _, err := h.commSvc.SetCartQuantity(ctx, userID, variantID, qty); err != nil {
		h.log.ErrorContext(ctx, "set cart quantity", "error", err,
			"user", userID, "variant", variantID, "qty", qty)
		h.redirectWithNotice(w, r, "/cart", "error", h.safeMessage(err, langOf(r)))
		return
	}

	if h.isHTMX(r) {
		cart, _ := h.commSvc.GetCart(ctx, userID)
		lang, _ := h.localeAndDir(r)
		h.renderPage(ctx, w, "render customer cart content", pages.CustomerCartContent(cart, lang))
		return
	}

	http.Redirect(w, r, "/cart", http.StatusSeeOther)
}
