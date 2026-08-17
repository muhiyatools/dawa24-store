package ui

import (
	"net/http"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// VendorDashboardPage renders the supplier dashboard.
func (h *UIHandler) VendorDashboardPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/dashboard", http.StatusSeeOther)
		return
	}

	data := pages.VendorDashboardData{}

	if h.catSvc != nil {
		// A COUNT, not the length of a page. Counting a page capped at 100 rows
		// reports the cap once a supplier passes it, and reads as a real figure.
		if n, err := h.catSvc.CountProductsByOrg(ctx, actor.OrganizationID, string(catalog.StatusActive)); err != nil {
			h.log.ErrorContext(ctx, "vendor dashboard: count products", "error", err)
		} else {
			data.ActiveProducts = n
		}
	}

	if h.commSvc != nil {
		// The total is counted; the panel below shows only the newest ten, so
		// the list stays a page while the figure stays a figure.
		if n, err := h.commSvc.CountVendorShipmentsByStatus(ctx, actor.OrganizationID,
			[]string{string(commerce.StatusPending), string(commerce.StatusConfirmed)}); err != nil {
			h.log.ErrorContext(ctx, "vendor dashboard: count shipments", "error", err)
		} else {
			data.PendingShipments = n
		}
		if shipments, err := h.commSvc.ListVendorShipments(ctx, actor.OrganizationID, 10, 0); err == nil {
			for _, sh := range shipments {
				if sh.Status == commerce.StatusPending || sh.Status == commerce.StatusConfirmed {
					data.Shipments = append(data.Shipments, sh)
				}
			}
		}
		if total, err := h.commSvc.MonthSalesByVendor(ctx, actor.OrganizationID); err == nil {
			data.MonthSales = total
		}
		if quotes, err := h.commSvc.ListQuoteRequests(ctx, actor.OrganizationID, true, 100, 0); err == nil {
			for _, q := range quotes {
				if q.Status == commerce.QuotePending {
					data.UnreadQuotes++
				}
			}
		}
	}

	if h.invSvc != nil {
		if low, err := h.invSvc.ListLowStock(ctx, 10, 0); err == nil {
			data.LowStockCount = len(low)
			data.LowStock = low
		}
	}

	if h.promoSvc != nil {
		if offers, err := h.promoSvc.ListActiveOffers(ctx, 10, 0); err == nil {
			data.Offers = offers
		}
	}

	if h.billSvc != nil {
		if wallet, err := h.billSvc.GetWallet(ctx, actor.UserID, "EGP"); err == nil && wallet != nil {
			data.WalletBalance = wallet.Balance
			data.HasWallet = true
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.VendorDashboard(lang, dir, data).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render vendor dashboard", "error", err)
	}
}

// PharmacyDashboardPage renders the pharmacy buyer dashboard.
func (h *UIHandler) PharmacyDashboardPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/pharmacy/dashboard", http.StatusSeeOther)
		return
	}

	data := pages.PharmacyDashboardData{}

	if h.commSvc != nil {
		if orders, err := h.commSvc.ListCustomerOrders(ctx, actor.UserID, 100, 0); err == nil {
			for _, o := range orders {
				if o.Status != commerce.StatusDelivered && o.Status != commerce.StatusCancelled && o.Status != commerce.StatusRefunded {
					data.OpenOrders++
				}
				if len(data.Orders) < 10 {
					data.Orders = append(data.Orders, o)
				}
			}
		}
		if total, err := h.commSvc.MonthSpendByCustomer(ctx, actor.UserID); err == nil {
			data.MonthSpend = total
		}
	}

	if h.idSvc != nil {
		if favs, err := h.idSvc.ListFavorites(ctx, actor.UserID); err == nil {
			data.Favorites = len(favs)
		}
	}

	if h.promoSvc != nil {
		if offers, err := h.promoSvc.ListActiveOffers(ctx, 10, 0); err == nil {
			data.ActiveOffers = len(offers)
			data.Offers = offers
		}
	}

	if h.billSvc != nil {
		if wallet, err := h.billSvc.GetWallet(ctx, actor.UserID, "EGP"); err == nil && wallet != nil {
			data.WalletBalance = wallet.Balance
			data.HasWallet = true
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.PharmacyDashboard(lang, dir, data, actor.Permissions).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render pharmacy dashboard", "error", err)
	}
}

// OnboardingPendingPage renders the approval gate for pending/rejected orgs.
func (h *UIHandler) OnboardingPendingPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	rejected := r.URL.Query().Get("rejected") == "1"

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.OnboardingPending(lang, dir, rejected).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render onboarding pending", "error", err)
	}
}
