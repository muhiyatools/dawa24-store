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
		if products, err := h.catSvc.Search(ctx, catalog.SearchParams{
			OrganizationID: &actor.OrganizationID, Limit: 100,
		}); err == nil {
			for _, p := range products {
				if p.Status == catalog.StatusActive {
					data.ActiveProducts++
				}
			}
		}
	}

	if h.commSvc != nil {
		if shipments, err := h.commSvc.ListVendorShipments(ctx, actor.OrganizationID, 100, 0); err == nil {
			for _, s := range shipments {
				if s.Status == commerce.StatusPending || s.Status == commerce.StatusConfirmed {
					data.PendingShipments++
					if len(data.Shipments) < 10 {
						data.Shipments = append(data.Shipments, s)
					}
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
