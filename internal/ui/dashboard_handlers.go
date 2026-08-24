package ui

import (
	"context"
	"fmt"
	"net/http"

	"github.com/muhiya/dawa24-store/internal/modules/attachments"
	"github.com/muhiya/dawa24-store/internal/modules/billing"
	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/modules/promo"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

func (h *UIHandler) loadOrgSubscriptionView(ctx context.Context, actor authctx.Actor, lang string) *pages.OrgSubscriptionView {
	if actor.OrganizationID <= 0 && actor.UserID <= 0 {
		return nil
	}

	subView := &pages.OrgSubscriptionView{
		HasSubscription:  true,
		PlanName:         "الباقة الأساسية الافتراضية",
		PlanSlug:         "basic",
		Status:           "ساري وفعال",
		ExpiresAt:        "تجديد تلقائي مستمر",
		MaxLoginSessions: 3,
		MaxDevices:       3,
		AIPlanID:         "plan-dev",
		IsDefaultPlan:    true,
	}

	sysCtx := database.AsSystem(ctx)

	// Fetch Subscription & Plan
	if h.billSvc != nil {
		var sub *billing.Subscription
		if actor.OrganizationID > 0 {
			sub, _ = h.billSvc.GetActiveSubscriptionByOrg(sysCtx, actor.OrganizationID)
		}
		if sub == nil && actor.UserID > 0 {
			sub, _ = h.billSvc.GetActiveSubscription(sysCtx, actor.UserID)
		}

		if sub != nil {
			subView.Status = "ساري وفعال"
			if !sub.ExpiresAt.IsZero() {
				subView.ExpiresAt = sub.ExpiresAt.Format("2006-01-02")
			}
			if plan, err := h.billSvc.GetPlanByID(sysCtx, sub.PlanID); err == nil && plan != nil {
				subView.PlanName = plan.Name.Get(i18n.Lang(lang))
				subView.PlanSlug = plan.Slug
				subView.MaxLoginSessions = plan.MaxLoginSessions
				subView.MaxDevices = plan.MaxDevices
				subView.AIPlanID = plan.AIPlanID
				subView.IsDefaultPlan = plan.IsDefault
			}
		} else {
			if defPlan, err := h.billSvc.GetDefaultPlan(sysCtx); err == nil && defPlan != nil {
				subView.PlanName = defPlan.Name.Get(i18n.Lang(lang))
				subView.PlanSlug = defPlan.Slug
				subView.MaxLoginSessions = defPlan.MaxLoginSessions
				subView.MaxDevices = defPlan.MaxDevices
				subView.AIPlanID = defPlan.AIPlanID
				subView.IsDefaultPlan = true
			}
		}
	}

	// Fetch Org AI credentials
	if h.orgSvc != nil && actor.OrganizationID > 0 {
		if o, err := h.orgSvc.GetOrganization(sysCtx, actor.OrganizationID); err == nil && o != nil {
			subView.AIUserID = o.AIUserID
			if subView.AIUserID == "" {
				subView.AIUserID = fmt.Sprintf("org-%d", actor.OrganizationID)
			}
			if len(o.AIVirtualKey) > 8 {
				subView.AIVirtualKeyMasked = o.AIVirtualKey[:4] + "••••••••" + o.AIVirtualKey[len(o.AIVirtualKey)-4:]
			} else if o.AIVirtualKey != "" {
				subView.AIVirtualKeyMasked = "••••••••"
			}
		}
	}

	return subView
}

// VendorDashboardPage renders the supplier dashboard.
func (h *UIHandler) VendorDashboardPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/dashboard", http.StatusSeeOther)
		return
	}

	data := pages.VendorDashboardData{
		Subscription: h.loadOrgSubscriptionView(ctx, actor, lang),
	}

	if h.attSvc != nil && actor.OrganizationID > 0 {
		if reqs, err := h.attSvc.ListDocumentRequests(ctx, actor, &actor.OrganizationID); err == nil {
			for _, reqItem := range reqs {
				if reqItem != nil && reqItem.Status == attachments.DocReqPending {
					data.PendingDocRequests = append(data.PendingDocRequests, reqItem)
				}
			}
		}
	}

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
		if shipments, err := h.commSvc.ListVendorShipments(ctx, actor.OrganizationID, 10, 0); err != nil {
			h.log.WarnContext(ctx, "vendor dashboard: list shipments", "error", err)
		} else {
			for _, sh := range shipments {
				if sh.Status == commerce.StatusPending || sh.Status == commerce.StatusConfirmed {
					data.Shipments = append(data.Shipments, sh)
				}
			}
		}
		if total, err := h.commSvc.MonthSalesByVendor(ctx, actor.OrganizationID); err != nil {
			h.log.WarnContext(ctx, "vendor dashboard: month sales", "error", err)
		} else {
			data.MonthSales = total
		}
		if quotes, err := h.commSvc.ListQuoteRequests(ctx, actor.OrganizationID, true, 100, 0); err != nil {
			h.log.WarnContext(ctx, "vendor dashboard: list quote requests", "error", err)
		} else {
			for _, q := range quotes {
				if q.Status == commerce.QuotePending {
					data.UnreadQuotes++
				}
			}
		}
	}

	if h.invSvc != nil {
		if low, err := h.invSvc.ListLowStock(ctx, 10, 0); err != nil {
			h.log.WarnContext(ctx, "vendor dashboard: list low stock", "error", err)
		} else {
			data.LowStockCount = len(low)
			data.LowStock = low
		}
	}

	if h.promoSvc != nil {
		if offers, err := h.promoSvc.ListActiveOffers(ctx, 10, 0); err != nil {
			h.log.WarnContext(ctx, "vendor dashboard: list active offers", "error", err)
		} else {
			data.Offers = offers
		}
	}

	if h.billSvc != nil {
		if wallet, err := h.billSvc.GetWallet(ctx, actor.UserID, "EGP"); err != nil {
			// Wallet lookup error (e.g. no wallet created yet) is logged as debug
			h.log.DebugContext(ctx, "vendor dashboard: get wallet optional", "error", err)
		} else if wallet != nil {
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

	data := pages.PharmacyDashboardData{
		Subscription: h.loadOrgSubscriptionView(ctx, actor, lang),
	}

	if h.attSvc != nil && actor.OrganizationID > 0 {
		if reqs, err := h.attSvc.ListDocumentRequests(ctx, actor, &actor.OrganizationID); err == nil {
			for _, reqItem := range reqs {
				if reqItem != nil && reqItem.Status == attachments.DocReqPending {
					data.PendingDocRequests = append(data.PendingDocRequests, reqItem)
				}
			}
		}
	}

	if h.commSvc != nil {
		if orders, err := h.commSvc.ListCustomerOrders(ctx, actor.UserID, 100, 0); err != nil {
			h.log.WarnContext(ctx, "pharmacy dashboard: list orders", "error", err)
		} else {
			for _, o := range orders {
				if o.Status != commerce.StatusDelivered && o.Status != commerce.StatusCancelled && o.Status != commerce.StatusRefunded {
					data.OpenOrders++
				}
				if len(data.Orders) < 10 {
					data.Orders = append(data.Orders, o)
				}
			}
		}
		if total, err := h.commSvc.MonthSpendByCustomer(ctx, actor.UserID); err != nil {
			h.log.WarnContext(ctx, "pharmacy dashboard: month spend", "error", err)
		} else {
			data.MonthSpend = total
		}
	}

	if h.idSvc != nil {
		if favs, err := h.idSvc.ListFavorites(ctx, actor.UserID); err != nil {
			h.log.WarnContext(ctx, "pharmacy dashboard: list favorites", "error", err)
		} else {
			data.Favorites = len(favs)
		}
	}

	if h.promoSvc != nil {
		if visible := h.visibleOffersForActor(ctx, &actor, 10); len(visible) > 0 {
			data.Offers = make([]*promo.Offer, 0, len(visible))
			for _, v := range visible {
				if v.Offer != nil {
					data.Offers = append(data.Offers, v.Offer)
				}
			}
			data.ActiveOffers = len(data.Offers)
		}
	}

	if h.billSvc != nil {
		if wallet, err := h.billSvc.GetWallet(ctx, actor.UserID, "EGP"); err != nil {
			h.log.DebugContext(ctx, "pharmacy dashboard: get wallet optional", "error", err)
		} else if wallet != nil {
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
