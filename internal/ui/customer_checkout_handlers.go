package ui

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/muhiya/dawa24-store/internal/modules/billing"
	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
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

	if !actor.IsBuyer() {
		h.redirectWithNotice(w, r, "/catalog", "error", i18n.T(lang, "checkout.pharmacy_only"))
		return
	}

	userID := actor.UserID

	if h.commSvc == nil {
		h.renderPage(ctx, w, "render checkout page", pages.CustomerCheckout(nil, nil, nil, nil, lang, dir))
		return
	}

	cart, err := h.commSvc.GetCart(ctx, userID, buyerOrgID(ctx))
	if err != nil {
		h.renderError(w, r, err)
		return
	}

	var branches []*org.Branch
	if h.orgSvc != nil && actor.OrganizationID > 0 {
		if bList, err := h.orgSvc.ListBranches(ctx, actor.OrganizationID); err != nil {
			h.log.WarnContext(ctx, "checkout: list customer branches", "error", err)
		} else {
			branches = filterCheckoutBranches(bList, actor)
		}
	}

	var checkoutMethods []*billing.PlatformPaymentMethod
	var walletCheckoutAllowed bool
	if h.billSvc != nil {
		if pms, err := h.billSvc.ListPlatformPaymentMethods(ctx, true); err == nil {
			for _, pm := range pms {
				if pm != nil && pm.IsActive && pm.IsCheckoutEnabled {
					checkoutMethods = append(checkoutMethods, pm)
					if pm.ID == "wallet" {
						walletCheckoutAllowed = true
					}
				}
			}
		}
	}

	var wallet *billing.Wallet
	if walletCheckoutAllowed && h.billSvc != nil && actor.OrganizationID > 0 {
		walletUserID, _ := resolveTenantUserIDs(ctx, h, actor)
		if wItem, err := h.billSvc.GetWallet(ctx, walletUserID, "EGP"); err == nil {
			wallet = wItem
		}
	}

	h.renderPage(ctx, w, "render checkout page", pages.CustomerCheckout(cart, branches, wallet, checkoutMethods, lang, dir))
}

func (h *UIHandler) CheckoutSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, _ := authctx.From(ctx)
	if _, err := authctx.UserID(ctx); err != nil {
		http.Redirect(w, r, "/auth/login?redirect=/checkout", http.StatusSeeOther)
		return
	}
	lang := langOf(r)

	plan, failure := h.planCheckout(ctx, actor, lang, checkoutRequest{
		FormBranchID:  r.PostFormValue("branch_id"),
		PaymentMethod: strings.TrimSpace(r.PostFormValue("payment_method")),
		Notes:         r.PostFormValue("notes"),
	})
	if failure == nil {
		var order *commerce.Order
		order, failure = h.placeCheckout(ctx, actor, lang, plan)
		if failure == nil {
			http.Redirect(w, r, "/orders/"+strconv.FormatInt(order.ID, 10), http.StatusSeeOther)
			return
		}
	}
	switch {
	case failure.Err != nil:
		h.renderError(w, r, failure.Err)
	case failure.Message != "":
		h.redirectWithNotice(w, r, failure.Back, "error", failure.Message)
	default:
		http.Redirect(w, r, failure.Back, http.StatusSeeOther)
	}
}
