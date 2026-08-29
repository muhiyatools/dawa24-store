package ui

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/attachments"
	"github.com/muhiya/dawa24-store/internal/modules/billing"
	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/modules/promo"
	"github.com/muhiya/dawa24-store/internal/modules/smartorder"
	"github.com/muhiya/dawa24-store/internal/platform/aiusage"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/gateway"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
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
		AIPlanID:         gateway.FallbackPlanID,
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

	// Fetch Org AI credentials & Live Consumption
	if h.orgSvc != nil && actor.OrganizationID > 0 {
		h.EnsureOrgAIGatewayProvisioned(ctx, actor.OrganizationID)
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

			// Consumption comes from our own ledger: it is complete, it is
			// not capped at one API page, and it is still there when the
			// Gateway is not. This used to be a live HTTP call to
			// api.muhiya.com on every dashboard render.
			if h.aiUsage != nil {
				if summary, err := h.aiUsage.Summarize(ctx, actor.OrganizationID, time.Now().Add(-aiLedgerWindow)); err == nil {
					subView.AIRequestsCount = summary.Requests
					subView.AITokensUsed = int(summary.TotalTokens())
					subView.AIBudgetSpentUSD = summary.CostUSD()
					subView.HasAIUsage = true
				} else {
					h.log.WarnContext(ctx, "could not summarise ai usage for dashboard",
						"org_id", actor.OrganizationID, "error", err)
				}
			}

			// The budget window is the one thing the Gateway alone knows: it
			// owns the plan's ceiling and the moment it reopens. Asked for
			// nothing else.
			if adminClient, _, ok := h.getGatewayAdminClient(ctx); ok && adminClient != nil {
				if userDetail, err := adminClient.GetUser(ctx, subView.AIUserID); err == nil && userDetail != nil && len(userDetail.BudgetUsage) > 0 {
					bw := userDetail.BudgetUsage[0]
					subView.AIBudgetName = bw.Name
					subView.AIBudgetLimitUSD = bw.BudgetUSD
					// The Gateway's own figure for the current window wins over
					// the ledger's: it is what the quota is actually enforced
					// against, and a percentage drawn from a different number
					// than the one doing the limiting would mislead.
					subView.AIBudgetSpentUSD = bw.CurrentSpent
					subView.HasAIUsage = true
					if !bw.ResetTime.IsZero() {
						subView.AIBudgetResetTime = bw.ResetTime.Format("2006-01-02 15:04")
					}
				}
			}
		}
	}

	// Nothing below invents a number.
	//
	// What used to be here manufactured a budget ceiling from the plan slug
	// ($200 / $50 / $15), manufactured a reset date as the first of next month,
	// and then set HasAIUsage true regardless — so every card drew a confident
	// percentage out of two values the Gateway had never supplied. A pharmacy
	// deciding whether to upgrade was reading fiction.
	//
	// The Gateway is the only authority on both figures. When it has not
	// published them the flags stay false and the screens say so.
	if subView.AIBudgetSpentUSD < 0 {
		subView.AIBudgetSpentUSD = 0
	}
	subView.HasAIBudget = subView.AIBudgetLimitUSD > 0

	return subView
}

// TenantSubscriptionPage renders the dedicated subscription and membership page for pharmacies and vendors.
func (h *UIHandler) TenantSubscriptionPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || (actor.OrganizationID <= 0 && actor.UserID <= 0) {
		http.Redirect(w, r, "/auth/login?redirect="+r.URL.Path, http.StatusSeeOther)
		return
	}

	lang, dir := h.localeAndDir(r)
	subView := h.loadOrgSubscriptionView(ctx, actor, lang)

	var allPlans []*billing.Plan
	var currentPlanID int64
	var walletBal money.Amount
	var autoRenew bool
	var billingCycle string = "monthly"
	sysCtx := database.AsSystem(ctx)

	if h.billSvc != nil {
		if plans, err := h.billSvc.ListPlans(sysCtx); err == nil && len(plans) > 0 {
			allPlans = plans
		} else {
			if err != nil {
				h.log.ErrorContext(ctx, "failed to list billing plans", "error", err)
			}
			if adminPlans, aErr := h.billSvc.AdminListPlans(sysCtx); aErr == nil && len(adminPlans) > 0 {
				for _, ap := range adminPlans {
					if ap.IsActive {
						allPlans = append(allPlans, ap)
					}
				}
			}
		}

		if actor.OrganizationID > 0 {
			if sub, _ := h.billSvc.GetActiveSubscriptionByOrg(sysCtx, actor.OrganizationID); sub != nil {
				currentPlanID = sub.PlanID
				autoRenew = sub.AutoRenew
				if sub.BillingCycle != "" {
					billingCycle = sub.BillingCycle
				}
			}
		}
		if currentPlanID <= 0 && actor.UserID > 0 {
			if sub, _ := h.billSvc.GetActiveSubscription(sysCtx, actor.UserID); sub != nil {
				currentPlanID = sub.PlanID
				autoRenew = sub.AutoRenew
				if sub.BillingCycle != "" {
					billingCycle = sub.BillingCycle
				}
			}
		}

		// Always ensure wallet exists for seamless in-app upgrades
		if actor.UserID > 0 {
			if w, err := h.billSvc.GetWallet(sysCtx, actor.UserID, "EGP"); err == nil && w != nil {
				walletBal = w.Balance
			}
		}
	}

	orgType := actor.OrgType
	if orgType == "" {
		if actor.IsVendor() {
			orgType = "vendor"
		} else {
			orgType = "customer"
		}
	}

	data := pages.TenantSubscriptionPageData{
		Subscription:  subView,
		Plans:         allPlans,
		CurrentPlanID: currentPlanID,
		WalletBalance: walletBal,
		AutoRenew:     autoRenew,
		BillingCycle:  billingCycle,
		NoticeType:    r.URL.Query().Get("notice_type"),
		NoticeMsg:     r.URL.Query().Get("notice"),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.TenantSubscriptionPage(data, orgType, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render subscription page", "error", err)
	}
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

	if h.orgSvc != nil && actor.OrganizationID > 0 {
		if branches, err := h.orgSvc.ListBranches(ctx, actor.OrganizationID); err == nil {
			for _, b := range branches {
				if b != nil && (b.Status == "active" || b.Status == "") {
					data.ActiveWarehousesCount++
				}
			}
		}
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
		if n, err := h.catSvc.CountProductsByOrg(ctx, actor.OrganizationID, string(catalog.StatusActive)); err != nil {
			h.log.ErrorContext(ctx, "vendor dashboard: count products", "error", err)
		} else {
			data.ActiveProducts = n
		}
	}

	if h.commSvc != nil {
		if shipments, err := h.commSvc.ListVendorShipments(ctx, actor.OrganizationID, 150, 0); err != nil {
			h.log.WarnContext(ctx, "vendor dashboard: list shipments", "error", err)
		} else {
			data.TotalShipmentsCount = len(shipments)
			for _, sh := range shipments {
				if sh == nil {
					continue
				}
				if sh.Status == commerce.StatusDelivered || sh.Status == commerce.StatusCompleted {
					data.DeliveredShipments++
				} else if sh.Status == commerce.StatusPending || sh.Status == commerce.StatusConfirmed || sh.Status == commerce.StatusProcessing {
					data.PendingOrdersTotal, _ = data.PendingOrdersTotal.Add(sh.TotalAmount)
					if len(data.Shipments) < 12 {
						data.Shipments = append(data.Shipments, sh)
					}
				}
			}
			data.PendingShipments = len(data.Shipments)
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
		http.Redirect(w, r, "/auth/login?redirect=/customer/dashboard", http.StatusSeeOther)
		return
	}

	data := pages.PharmacyDashboardData{
		Subscription: h.loadOrgSubscriptionView(ctx, actor, lang),
	}

	// 1. Pharmacy Organization & Branches
	if h.orgSvc != nil && actor.OrganizationID > 0 {
		sysCtx := database.AsSystem(ctx)
		if o, err := h.orgSvc.GetOrganization(sysCtx, actor.OrganizationID); err == nil && o != nil {
			data.CustomerOrgName = o.LegalName
			if data.CustomerOrgName == "" && !o.TradeName.IsEmpty() {
				data.CustomerOrgName = o.TradeName.Get(i18n.Lang(lang))
			}
		}
		if branches, err := h.orgSvc.ListBranches(ctx, actor.OrganizationID); err == nil {
			data.Branches = branches
			data.TotalBranches = len(branches)
			for _, b := range branches {
				if b != nil && b.Status == "active" {
					data.ActiveBranches++
				}
			}
		}
		if actor.BranchID != nil && *actor.BranchID > 0 {
			data.ActiveBranchID = *actor.BranchID
			for _, b := range data.Branches {
				if b.ID == *actor.BranchID {
					data.ActiveBranchName = b.Name.Get(i18n.Lang(lang))
					if data.ActiveBranchName == "" {
						data.ActiveBranchName = b.Code
					}
					break
				}
			}
		}
		if data.ActiveBranchName == "" && len(data.Branches) > 0 {
			data.ActiveBranchID = data.Branches[0].ID
			data.ActiveBranchName = data.Branches[0].Name.Get(i18n.Lang(lang))
		}
	}

	// 2. Urgent Administrative Document Requests
	if h.attSvc != nil && actor.OrganizationID > 0 {
		if reqs, err := h.attSvc.ListDocumentRequests(ctx, actor, &actor.OrganizationID); err == nil {
			for _, reqItem := range reqs {
				if reqItem != nil && reqItem.Status == attachments.DocReqPending {
					data.PendingDocRequests = append(data.PendingDocRequests, reqItem)
				}
			}
		}
	}

	// 3. Orders, Spend & Item totals
	if h.commSvc != nil {
		if orders, err := h.commSvc.ListCustomerOrders(ctx, actor.UserID, 100, 0); err != nil {
			h.log.WarnContext(ctx, "pharmacy dashboard: list orders", "error", err)
		} else {
			data.TotalOrders = len(orders)
			branchMap := make(map[int64]*org.Branch)
			for _, b := range data.Branches {
				branchMap[b.ID] = b
			}

			for i, o := range orders {
				if o.BranchID != nil && *o.BranchID > 0 {
					if b, ok := branchMap[*o.BranchID]; ok && b != nil {
						if o.CustomerBranchName.IsEmpty() {
							o.CustomerBranchName = b.Name
						}
					}
				}
				if o.Status == commerce.StatusDelivered || o.Status == commerce.StatusCompleted {
					data.CompletedOrders++
					data.TotalSpend, _ = data.TotalSpend.Add(o.TotalAmount)
				} else if o.Status == commerce.StatusCancelled || o.Status == commerce.StatusFailed || o.Status == commerce.StatusReturned || o.Status == commerce.StatusRefunded {
					data.CancelledOrders++
				} else {
					data.ActiveOrders++
					data.TotalSpend, _ = data.TotalSpend.Add(o.TotalAmount)
				}

				// Hydrate recent orders for display (up to 8)
				if i < 8 {
					if len(o.Lines) == 0 {
						if fullOrder, err := h.commSvc.GetOrder(ctx, o.ID); err == nil && fullOrder != nil {
							o.Lines = fullOrder.Lines
							if o.CustomerBranchName.IsEmpty() && !fullOrder.CustomerBranchName.IsEmpty() {
								o.CustomerBranchName = fullOrder.CustomerBranchName
							}
						}
					}
					data.Orders = append(data.Orders, o)
				}
			}

			for _, o := range data.Orders {
				data.TotalOrderedItems += len(o.Lines)
			}
			if data.TotalOrderedItems == 0 && data.TotalOrders > 0 {
				data.TotalOrderedItems = data.TotalOrders
			}
		}
		if total, err := h.commSvc.MonthSpendByCustomer(ctx, actor.UserID); err != nil {
			h.log.WarnContext(ctx, "pharmacy dashboard: month spend", "error", err)
		} else {
			data.MonthSpend = total
		}
	}

	// 4. Wallet, Balance, Pending Deposits & Transactions
	if h.billSvc != nil {
		if wallet, err := h.billSvc.GetWallet(ctx, actor.UserID, "EGP"); err != nil {
			h.log.DebugContext(ctx, "pharmacy dashboard: get wallet optional", "error", err)
		} else if wallet != nil {
			data.WalletBalance = wallet.Balance
			data.HasWallet = true

			if txs, err := h.billSvc.ListWalletTransactions(ctx, wallet.ID, 5, 0); err == nil {
				data.RecentTransactions = txs
			}
		}

		if deposits, err := h.billSvc.ListUserDeposits(ctx, actor.UserID, 50, 0); err == nil {
			for _, dep := range deposits {
				if dep != nil && dep.Status == billing.DepositPending {
					data.PendingDepositsCount++
					data.PendingDepositsTotal, _ = data.PendingDepositsTotal.Add(dep.Amount)
				}
			}
		}
	}

	// 5. Smart Orders Lifecycle & Recent Runs
	if h.smartOrderSvc != nil && actor.OrganizationID > 0 {
		if runs, err := h.smartOrderSvc.History(ctx, actor.OrganizationID, 50, 0); err == nil {
			data.SmartOrdersTotal = len(runs)
			data.SmartOrdersCount = len(runs)
			for _, r := range runs {
				if r == nil {
					continue
				}
				switch r.Status {
				case smartorder.StatusProcessing, smartorder.StatusQueued:
					data.SmartOrdersProcessing++
				case smartorder.StatusPlaced, smartorder.StatusCompleted, smartorder.StatusFinalizing:
					data.SmartOrdersCompleted++
				case smartorder.StatusDraft, smartorder.StatusMapping, smartorder.StatusStale:
					data.SmartOrdersNeedsReview++
				}
				if len(data.RecentSmartOrders) < 4 {
					data.RecentSmartOrders = append(data.RecentSmartOrders, r)
				}
			}
		}
	}

	// 6. Favorites & Active Offers
	if h.idSvc != nil {
		if favs, err := h.idSvc.ListFavorites(ctx, actor.UserID); err != nil {
			h.log.WarnContext(ctx, "pharmacy dashboard: list favorites", "error", err)
		} else {
			data.Favorites = len(favs)
		}
	}

	if h.promoSvc != nil {
		if visible := h.visibleOffersForActor(ctx, &actor, 6); len(visible) > 0 {
			data.Offers = make([]*promo.Offer, 0, len(visible))
			for _, v := range visible {
				if v.Offer != nil {
					data.Offers = append(data.Offers, v.Offer)
				}
			}
			data.ActiveOffers = len(data.Offers)
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

// EnsureOrgAIGatewayProvisioned returns the Gateway identity the organisation
// spends against, provisioning it on first use.
//
// The provisioning itself lives in one place now. This used to read the
// organisation, read its subscription, resolve a plan and mint a key inline —
// and so did three other call sites, each minting a key that revoked the one
// the others had just stored. It delegates instead, so the per-organisation
// lock, the validated cache and the plan-follows-subscription rule apply
// wherever the identity is asked for.
func (h *UIHandler) EnsureOrgAIGatewayProvisioned(ctx context.Context, orgID int64) (userID, virtualKey string) {
	if orgID <= 0 || h.orgSvc == nil {
		return "", ""
	}
	if h.tenantKeys != nil {
		virtualKey = h.tenantKeys.Key(ctx, orgID)
	}

	// The user id is derived, not guessed: it is the key the Gateway files this
	// tenant's whole usage history under, so a screen that displayed a
	// different one would be reporting somebody else's consumption.
	userID = gateway.OrganizationUserID(orgID)

	if virtualKey == "" {
		// Provisioning could not be completed — an unreachable Gateway, or
		// credentials an operator has not supplied yet. Whatever is stored is
		// the best answer available.
		if o, err := h.orgSvc.GetOrganization(database.AsSystem(ctx), orgID); err == nil && o != nil {
			if o.AIUserID != "" {
				userID = o.AIUserID
			}
			virtualKey = o.AIVirtualKey
		}
	}
	return userID, virtualKey
}

// AIConsumptionLogsPage renders a tenant's own AI consumption.
//
// It reads the local ledger, not the Gateway. The previous version made one to
// three live HTTP calls to the Gateway per render, showed at most a hundred
// rows, had no history beyond the Gateway's own retention, and displayed
// nothing whatsoever when the Gateway was unreachable — while filling the gaps
// it could not measure with invented per-token costs and a flat 280 ms latency.
//
// The ledger is written as the calls happen, so the history is ours, complete,
// filterable, and unaffected by the Gateway being down. The Gateway is still
// asked one thing, because it is the only authority on it: the live budget
// window.
func (h *UIHandler) AIConsumptionLogsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || (actor.OrganizationID <= 0 && actor.UserID <= 0) {
		http.Redirect(w, r, "/auth/login?redirect="+r.URL.Path, http.StatusSeeOther)
		return
	}

	lang, dir := h.localeAndDir(r)
	subView := h.loadOrgSubscriptionView(ctx, actor, lang)
	isVendor := actor.IsVendor()

	pageData := pages.AIConsumptionLogsPageData{
		IsVendor:         isVendor,
		IsCustomer:       actor.IsCustomer(),
		FeatureBreakdown: map[string]int{},
		AIUserID:         gateway.OrganizationUserID(actor.OrganizationID),
		PlanName:         "الباقة الأساسية",
		PlanSlug:         "basic",
		AIPlanID:         gateway.FallbackPlanID,
	}
	if subView != nil {
		pageData.AIUserID = subView.AIUserID
		pageData.PlanName = subView.PlanName
		pageData.PlanSlug = subView.PlanSlug
		pageData.AIPlanID = subView.AIPlanID
		pageData.ResetTime = subView.AIBudgetResetTime
		pageData.ActiveBudgetLimit = subView.AIBudgetLimitUSD
		pageData.ActiveBudgetSpent = subView.AIBudgetSpentUSD
		pageData.UsagePercentage = subView.AIPercentage()
		pageData.HasBudget = subView.HasAIBudget
	}

	if h.aiUsage != nil && actor.OrganizationID > 0 {
		h.fillAIUsageFromLedger(ctx, &pageData, actor.OrganizationID, isVendor)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AIConsumptionLogsPage(pageData, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render ai consumption logs", "error", err)
	}
}

// aiLedgerWindow is how far back the usage screens look.
//
// A month rather than everything: it is the period a budget window covers and
// the period a pharmacy reasons about when deciding whether their plan fits.
// The rows older than this are still in the ledger and still queryable.
const aiLedgerWindow = 30 * 24 * time.Hour

// fillAIUsageFromLedger populates the page from the local record.
//
// The listing runs under the caller's own transaction context, so row-level
// security scopes it to their organisation. That is the isolation guarantee;
// the previous version enforced it by comparing a user id string in a loop
// after fetching, which is a filter rather than a boundary.
func (h *UIHandler) fillAIUsageFromLedger(ctx context.Context, data *pages.AIConsumptionLogsPageData, orgID int64, isVendor bool) {
	since := time.Now().Add(-aiLedgerWindow)

	entries, _, err := h.aiUsage.List(ctx, aiusage.Filter{
		OrganizationID: orgID,
		Since:          since,
		Limit:          200,
	})
	if err != nil {
		h.log.WarnContext(ctx, "could not read ai usage ledger", "org_id", orgID, "error", err)
		return
	}

	data.Logs = make([]*pages.AILogItemView, 0, len(entries))
	for _, e := range entries {
		featName, featKey := mapGatewayCapabilityToName(e.Capability, e.Feature, isVendor)
		data.Logs = append(data.Logs, &pages.AILogItemView{
			ID:            fmt.Sprintf("%d", e.ID),
			Timestamp:     e.CreatedAt.Format("2006-01-02 15:04:05"),
			TimeFormatted: e.CreatedAt.Format("2006-01-02 15:04"),
			FeatureName:   featName,
			FeatureKey:    featKey,
			ModelAlias:    e.Model,
			InputTokens:   e.InputTokens,
			OutputTokens:  e.OutputTokens,
			TotalTokens:   e.TotalTokens(),
			CostUSD:       e.CostUSD(),
			CostKnown:     e.CostKnown,
			DurationMs:    int64(e.DurationMS),
			DurationKnown: e.DurationMS > 0,
			Status:        e.Status,
			StatusLabel:   aiStatusLabel(e.Status, e.FromCache, e.Fallback),
		})
	}

	summary, err := h.aiUsage.Summarize(ctx, orgID, since)
	if err != nil {
		h.log.WarnContext(ctx, "could not summarise ai usage", "org_id", orgID, "error", err)
		return
	}
	data.TotalRequests = summary.Requests
	data.TotalTokens = int(summary.TotalTokens())
	data.TotalCostUSD = summary.CostUSD()
	data.CostIsComplete = summary.CostIsComplete()

	byFeature, err := h.aiUsage.ByFeature(ctx, orgID, since)
	if err != nil {
		h.log.WarnContext(ctx, "could not break ai usage down by feature", "org_id", orgID, "error", err)
		return
	}
	for _, f := range byFeature {
		_, key := mapGatewayCapabilityToName(f.Feature, f.Feature, isVendor)
		data.FeatureBreakdown[key] += f.Requests
	}
}

// aiStatusLabel renders an outcome in the tenant's language.
//
// A cached answer and a fallback are both "successful" in the sense that the
// user got a result, and both cost nothing — but they mean different things and
// a usage screen that calls them the same thing hides how often AI was actually
// reached.
func aiStatusLabel(status string, cached, fallback bool) string {
	switch {
	case fallback:
		return "المسار الحتمي (بدون ذكاء اصطناعي)"
	case cached:
		return "من الذاكرة (مجاني)"
	}
	switch status {
	case "success":
		return "ناجح"
	case "disabled":
		return "الخدمة موقوفة"
	case "timeout":
		return "انتهت المهلة"
	case "quota_exceeded":
		return "تجاوز الحصة"
	case "rate_limited":
		return "تجاوز معدل الطلبات"
	case "unauthorized":
		return "مفتاح غير صالح"
	case "circuit_open", "unavailable":
		return "البوابة غير متاحة"
	case "abandoned":
		return "غادر المستخدم قبل الاكتمال"
	default:
		return "غير مكتمل"
	}
}

func mapGatewayCapabilityToName(cap, feat string, isVendor bool) (string, string) {
	if feat != "" {
		switch feat {
		case "smart_order", "smartorder":
			return "الطلب الذكي وتحسين المطابقة", "smart_order"
		case "savings", "saving_products":
			return "مطابقة واقتراح بدائل التوفير", "savings"
		case "assistant":
			return "المساعد الصيدلاني الذكي", "assistant"
		case "voice_ocr", "voice", "ocr":
			return "تحويل الأوامر الصوتية والروشتات", "voice_ocr"
		case "variant_match", "variants":
			return "استيراد ومطابقة الأصناف", "variant_match"
		case "savings_import":
			return "استيراد وتوليد منتجات التوفير", "savings_import"
		case "column_detect":
			return "التعرف على أعمدة الكتالوج", "column_detect"
		}
	}
	switch cap {
	case "matching.enhance":
		if isVendor {
			return "مطابقة الكتالوج الذكي (Catalog AI Enhance)", "variant_match"
		}
		return "الطلب الذكي وتحسين المطابقة (Smart Order Enhance)", "smart_order"
	case "import.detect_columns":
		return "التعرف التلقائي على أعمدة الكتالوج", "column_detect"
	case "product.match", "matching.adjudicate":
		if isVendor {
			return "استيراد ومطابقة الأصناف البديلة", "variant_match"
		}
		return "مطابقة منتجات التوفير والبدائل", "savings"
	case "catalog.chat", "assistant":
		return "المساعد الآلي الذكي", "assistant"
	case "voice.transcribe":
		return "تحويل الأوامر الصوتية", "voice_ocr"
	default:
		if isVendor {
			return "استيراد وتوليد الكتالوج الذكي", "variant_match"
		}
		return "الطلب الذكي وتحسين المطابقة", "smart_order"
	}
}
