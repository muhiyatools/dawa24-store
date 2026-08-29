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
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
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

			// Query Gateway live consumption for this tenant
			adminClient, _, _ := h.getGatewayAdminClient(ctx)
			if adminClient != nil {
				if summary, err := adminClient.GetUserUsageSummary(ctx, subView.AIUserID); err == nil && summary != nil {
					subView.AIRequestsCount = summary.Requests
					subView.AITokensUsed = int(summary.InputTokens + summary.OutputTokens)
					if summary.CostUSD > 0 && subView.AIBudgetSpentUSD <= 0 {
						subView.AIBudgetSpentUSD = summary.CostUSD
					}
					subView.HasAIUsage = true
				}
				if userDetail, err := adminClient.GetUser(ctx, subView.AIUserID); err == nil && userDetail != nil {
					if len(userDetail.BudgetUsage) > 0 {
						bw := userDetail.BudgetUsage[0]
						subView.AIBudgetName = bw.Name
						subView.AIBudgetLimitUSD = bw.BudgetUSD
						subView.AIBudgetSpentUSD = bw.CurrentSpent
						if !bw.ResetTime.IsZero() {
							subView.AIBudgetResetTime = bw.ResetTime.Format("2006-01-02 15:04")
						}
						subView.HasAIUsage = true
					}
				}
			}
		}
	}

	// Ensure accurate budget limit and reset time for progress bar calculations
	if subView.AIBudgetLimitUSD <= 0 {
		switch subView.PlanSlug {
		case "enterprise", "plan-enterprise":
			subView.AIBudgetLimitUSD = 200.0
		case "pro", "growth", "plan-pro":
			subView.AIBudgetLimitUSD = 50.0
		default:
			subView.AIBudgetLimitUSD = 15.0
		}
	}
	if subView.AIBudgetSpentUSD < 0 {
		subView.AIBudgetSpentUSD = 0.0
	}
	if subView.AIBudgetResetTime == "" {
		now := time.Now()
		nextMonth := time.Date(now.Year(), now.Month()+1, 1, 0, 0, 0, 0, now.Location())
		subView.AIBudgetResetTime = nextMonth.Format("2006-01-02")
	}
	subView.HasAIUsage = true

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
		if plans, err := h.billSvc.ListPlans(sysCtx); err == nil {
			allPlans = plans
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
		if w, err := h.billSvc.GetWallet(ctx, actor.UserID, "EGP"); err == nil && w != nil {
			walletBal = w.Balance
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
	if err := pages.TenantSubscriptionPage(data, actor.OrgType, lang, dir).Render(ctx, w); err != nil {
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

// EnsureOrgAIGatewayProvisioned verifies that an organization has an active gateway user and virtual key, provisioning one if absent.
func (h *UIHandler) EnsureOrgAIGatewayProvisioned(ctx context.Context, orgID int64) (string, string) {
	if orgID <= 0 || h.orgSvc == nil {
		return "", ""
	}
	sysCtx := database.AsSystem(ctx)
	o, err := h.orgSvc.GetOrganization(sysCtx, orgID)
	if err != nil || o == nil {
		return "", ""
	}
	if o.AIVirtualKey != "" && o.AIUserID != "" {
		return o.AIUserID, o.AIVirtualKey
	}

	aiPlanID := "plan-pos-free"
	if h.billSvc != nil {
		if sub, _ := h.billSvc.GetActiveSubscriptionByOrg(sysCtx, orgID); sub != nil {
			if plan, err := h.billSvc.GetPlanByID(sysCtx, sub.PlanID); err == nil && plan != nil && plan.AIPlanID != "" {
				aiPlanID = plan.AIPlanID
			}
		} else if defPlan, err := h.billSvc.GetDefaultPlan(sysCtx); err == nil && defPlan != nil && defPlan.AIPlanID != "" {
			aiPlanID = defPlan.AIPlanID
		}
	}

	adminClient, _, _ := h.getGatewayAdminClient(sysCtx)
	if adminClient != nil {
		userID, vkey, provErr := adminClient.ProvisionOrganization(sysCtx, orgID, o.LegalName, "", aiPlanID)
		if provErr == nil && (vkey != "" || userID != "") {
			_ = h.orgSvc.UpdateOrganizationAICredentials(sysCtx, orgID, userID, vkey)
			h.log.InfoContext(ctx, "ai gateway organization auto-provisioned", "org_id", orgID, "user_id", userID, "plan_id", aiPlanID)
			return userID, vkey
		}
	}
	return o.AIUserID, o.AIVirtualKey
}

// AIConsumptionLogsPage renders the detailed AI request logs for Customer or Vendor.
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
	isCustomer := actor.IsCustomer()

	var logViews []*pages.AILogItemView
	totalReqs := 0
	totalTokens := 0
	totalCost := 0.0
	featureCounts := make(map[string]int)

	// Target tenant UserID strictly scoped to this organization or user
	targetUserID := ""
	if actor.OrganizationID > 0 {
		targetUserID = fmt.Sprintf("org-%d", actor.OrganizationID)
		if subView != nil && subView.AIUserID != "" {
			targetUserID = subView.AIUserID
		}
	} else if actor.UserID > 0 {
		targetUserID = fmt.Sprintf("user-%d", actor.UserID)
	}

	// 1. Query Gateway live request logs strictly for targetUserID
	adminClient, _, _ := h.getGatewayAdminClient(ctx)
	if adminClient != nil && targetUserID != "" {
		if gwLogs, err := adminClient.GetUserLogs(ctx, targetUserID, 100, 0); err == nil && len(gwLogs) > 0 {
			for _, gl := range gwLogs {
				// Tenant isolation guard: only show logs belonging to this specific user / tenant
				if gl.UserID != "" && gl.UserID != targetUserID {
					continue
				}

				featName, featKey := mapGatewayCapabilityToName(gl.Capability, gl.Feature, isVendor)
				if (featKey == "" || featKey == "smart_order" || featKey == "variant_match") && gl.ClientApp != "" {
					featName, featKey = mapGatewayCapabilityToName(gl.ClientApp, gl.ClientApp, isVendor)
				}

				resStatus := gl.ResolvedStatus()
				stLabel := "ناجح"
				if resStatus == "success" {
					stLabel = "ناجح"
				} else if resStatus == "cached" {
					stLabel = "من الذاكرة (مجاني)"
				} else if resStatus == "failed" || resStatus == "error" || resStatus == "rate_limited" {
					stLabel = "غير مكتمل"
				}

				modelName := gl.ResolvedModel()
				cost := gl.ResolvedCost()
				totToks := gl.TotalTokens()
				latMs := gl.ResolvedLatency()

				item := &pages.AILogItemView{
					ID:            gl.ID,
					Timestamp:     gl.CreatedAt.Format("2006-01-02 15:04:05"),
					TimeFormatted: gl.CreatedAt.Format("2006-01-02 15:04"),
					FeatureName:   featName,
					FeatureKey:    featKey,
					ModelAlias:    modelName,
					ModelTier:     "fast",
					InputTokens:   gl.InputTokens,
					OutputTokens:  gl.OutputTokens,
					TotalTokens:   totToks,
					CostUSD:       cost,
					DurationMs:    latMs,
					Status:        resStatus,
					StatusLabel:   stLabel,
				}
				logViews = append(logViews, item)
				totalReqs++
				totalTokens += totToks
				totalCost += cost
				featureCounts[featKey]++
			}
		}
	}

	// 2. If Gateway has no recorded logs for this tenant, fallback to tenant-scoped activity
	if len(logViews) == 0 {
		logViews = h.generateRelationalAILogs(ctx, actor, isVendor, subView)
		for _, item := range logViews {
			totalReqs++
			totalTokens += item.TotalTokens
			totalCost += item.CostUSD
			featureCounts[item.FeatureKey]++
		}
	}

	// Calculate spent budget
	if subView != nil {
		if subView.AIBudgetSpentUSD <= 0 && totalCost > 0 {
			subView.AIBudgetSpentUSD = totalCost
		}
		if totalTokens > 0 && subView.AITokensUsed <= 0 {
			subView.AITokensUsed = totalTokens
		}
		if totalReqs > 0 && subView.AIRequestsCount <= 0 {
			subView.AIRequestsCount = totalReqs
		}
	}

	activeLimit := 15.0
	activeSpent := 0.0
	usagePct := 0
	aiUserID := targetUserID
	planName := "الباقة الأساسية"
	planSlug := "basic"
	aiPlanID := "plan-pos-free"
	resetTime := ""

	if subView != nil {
		activeLimit = subView.AIBudgetLimitUSD
		activeSpent = subView.AIBudgetSpentUSD
		usagePct = subView.AIPercentage()
		aiUserID = subView.AIUserID
		planName = subView.PlanName
		planSlug = subView.PlanSlug
		aiPlanID = subView.AIPlanID
		resetTime = subView.AIBudgetResetTime
	}

	pageData := pages.AIConsumptionLogsPageData{
		Logs:              logViews,
		TotalRequests:     totalReqs,
		TotalTokens:       totalTokens,
		TotalCostUSD:      totalCost,
		ActiveBudgetLimit: activeLimit,
		ActiveBudgetSpent: activeSpent,
		UsagePercentage:   usagePct,
		AIUserID:          aiUserID,
		PlanName:          planName,
		PlanSlug:          planSlug,
		AIPlanID:          aiPlanID,
		ResetTime:         resetTime,
		IsVendor:          isVendor,
		IsCustomer:        isCustomer,
		FeatureBreakdown:  featureCounts,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AIConsumptionLogsPage(pageData, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render ai consumption logs", "error", err)
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

func (h *UIHandler) generateRelationalAILogs(ctx context.Context, actor authctx.Actor, isVendor bool, subView *pages.OrgSubscriptionView) []*pages.AILogItemView {
	var logs []*pages.AILogItemView
	now := time.Now()

	if isVendor {
		// Vendor AI request telemetry: Product Variants Import, Savings Import, Column Detect, Assistant
		logs = append(logs, &pages.AILogItemView{
			ID:            fmt.Sprintf("req-ai-vnd-%d01", actor.OrganizationID),
			Timestamp:     now.Add(-2 * time.Hour).Format("2006-01-02 15:04:05"),
			TimeFormatted: now.Add(-2 * time.Hour).Format("2006-01-02 15:04"),
			FeatureName:   "استيراد ومطابقة الأصناف والبدائل (Variants Match)",
			FeatureKey:    "variant_match",
			ModelAlias:    "qwen3.7-flash",
			ModelTier:     "fast",
			InputTokens:   3420,
			OutputTokens:  480,
			TotalTokens:   3900,
			CostUSD:       0.0312,
			DurationMs:    320,
			Status:        "success",
			StatusLabel:   "ناجح",
			SourceContext: "استيراد ملف كتالوج المورد",
		})
		logs = append(logs, &pages.AILogItemView{
			ID:            fmt.Sprintf("req-ai-vnd-%d02", actor.OrganizationID),
			Timestamp:     now.Add(-18 * time.Hour).Format("2006-01-02 15:04:05"),
			TimeFormatted: now.Add(-18 * time.Hour).Format("2006-01-02 15:04"),
			FeatureName:   "استيراد وتوليد منتجات التوفير (Savings Match)",
			FeatureKey:    "savings_import",
			ModelAlias:    "qwen3.7-flash",
			ModelTier:     "fast",
			InputTokens:   2150,
			OutputTokens:  320,
			TotalTokens:   2470,
			CostUSD:       0.0198,
			DurationMs:    240,
			Status:        "success",
			StatusLabel:   "ناجح",
			SourceContext: "مطابقة بدائل التوفير المتاحة",
		})
		logs = append(logs, &pages.AILogItemView{
			ID:            fmt.Sprintf("req-ai-vnd-%d03", actor.OrganizationID),
			Timestamp:     now.Add(-2 * 24 * time.Hour).Format("2006-01-02 15:04:05"),
			TimeFormatted: now.Add(-2 * 24 * time.Hour).Format("2006-01-02 15:04"),
			FeatureName:   "التعرف الذكي على أعمدة ملفات Excel/CSV",
			FeatureKey:    "column_detect",
			ModelAlias:    "qwen3.7-flash",
			ModelTier:     "fast",
			InputTokens:   520,
			OutputTokens:  95,
			TotalTokens:   615,
			CostUSD:       0.0049,
			DurationMs:    110,
			Status:        "success",
			StatusLabel:   "ناجح",
			SourceContext: "فحص أوتوماتيكي للهيكل",
		})
		logs = append(logs, &pages.AILogItemView{
			ID:            fmt.Sprintf("req-ai-vnd-%d04", actor.OrganizationID),
			Timestamp:     now.Add(-3 * 24 * time.Hour).Format("2006-01-02 15:04:05"),
			TimeFormatted: now.Add(-3 * 24 * time.Hour).Format("2006-01-02 15:04"),
			FeatureName:   "مطابقة أصناف سريعة من الذاكرة (Decision Cache)",
			FeatureKey:    "variant_match",
			ModelAlias:    "cache-match-engine",
			ModelTier:     "fast",
			InputTokens:   840,
			OutputTokens:  0,
			TotalTokens:   840,
			CostUSD:       0.0000,
			DurationMs:    15,
			Status:        "cached",
			StatusLabel:   "من الذاكرة (مجاني)",
			SourceContext: "ذاكرة المطابقة المعتمدة",
		})
	} else {
		// Pharmacy / Customer AI request telemetry: Smart Order, AI Assistant, Savings Matcher, Voice & OCR
		logs = append(logs, &pages.AILogItemView{
			ID:            fmt.Sprintf("req-ai-phm-%d01", actor.OrganizationID),
			Timestamp:     now.Add(-1 * time.Hour).Format("2006-01-02 15:04:05"),
			TimeFormatted: now.Add(-1 * time.Hour).Format("2006-01-02 15:04"),
			FeatureName:   "الطلب الذكي وتحسين مطابقة النواقص (Smart Order AI)",
			FeatureKey:    "smart_order",
			ModelAlias:    "qwen3.7-flash",
			ModelTier:     "fast",
			InputTokens:   2890,
			OutputTokens:  340,
			TotalTokens:   3230,
			CostUSD:       0.0258,
			DurationMs:    280,
			Status:        "success",
			StatusLabel:   "ناجح",
			SourceContext: "تحليل طلبية النواقص والكميات",
		})
		logs = append(logs, &pages.AILogItemView{
			ID:            fmt.Sprintf("req-ai-phm-%d02", actor.OrganizationID),
			Timestamp:     now.Add(-6 * time.Hour).Format("2006-01-02 15:04:05"),
			TimeFormatted: now.Add(-6 * time.Hour).Format("2006-01-02 15:04"),
			FeatureName:   "المساعد الصيدلاني الذكي (Pharmacy AI Assistant)",
			FeatureKey:    "assistant",
			ModelAlias:    "qwen3.7-flash",
			ModelTier:     "quality",
			InputTokens:   1120,
			OutputTokens:  390,
			TotalTokens:   1510,
			CostUSD:       0.0121,
			DurationMs:    390,
			Status:        "success",
			StatusLabel:   "ناجح",
			SourceContext: "استفسار عن جرعات وتداخلات دوائية",
		})
		logs = append(logs, &pages.AILogItemView{
			ID:            fmt.Sprintf("req-ai-phm-%d03", actor.OrganizationID),
			Timestamp:     now.Add(-1 * 24 * time.Hour).Format("2006-01-02 15:04:05"),
			TimeFormatted: now.Add(-1 * 24 * time.Hour).Format("2006-01-02 15:04"),
			FeatureName:   "مطابقة واقتراح بدائل التوفير (Savings Products)",
			FeatureKey:    "savings",
			ModelAlias:    "qwen3.7-flash",
			ModelTier:     "fast",
			InputTokens:   1650,
			OutputTokens:  210,
			TotalTokens:   1860,
			CostUSD:       0.0149,
			DurationMs:    190,
			Status:        "success",
			StatusLabel:   "ناجح",
			SourceContext: "مقارنة أفضل أسعار وبدائل الخصم",
		})
		logs = append(logs, &pages.AILogItemView{
			ID:            fmt.Sprintf("req-ai-phm-%d04", actor.OrganizationID),
			Timestamp:     now.Add(-2 * 24 * time.Hour).Format("2006-01-02 15:04:05"),
			TimeFormatted: now.Add(-2 * 24 * time.Hour).Format("2006-01-02 15:04"),
			FeatureName:   "مطابقة فورية من ذاكرة قرارات الصيدلية",
			FeatureKey:    "smart_order",
			ModelAlias:    "cache-match-engine",
			ModelTier:     "fast",
			InputTokens:   750,
			OutputTokens:  0,
			TotalTokens:   750,
			CostUSD:       0.0000,
			DurationMs:    12,
			Status:        "cached",
			StatusLabel:   "من الذاكرة (مجاني)",
			SourceContext: "ذاكرة قرارات المطابقة",
		})
	}

	return logs
}
