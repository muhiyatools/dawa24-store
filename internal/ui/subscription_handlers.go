package ui

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/billing"
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
		PlanName:         i18n.T(lang, "sub.default_plan_name"),
		PlanSlug:         "basic",
		Status:           i18n.T(lang, "sub.status_active"),
		ExpiresAt:        i18n.T(lang, "sub.auto_renews"),
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
			subView.Status = i18n.T(lang, "sub.status_active")
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

	h.renderPage(ctx, w, "render subscription page", pages.TenantSubscriptionPage(data, orgType, lang, dir))
}

// OnboardingPendingPage renders the approval gate for pending/rejected orgs.
func (h *UIHandler) OnboardingPendingPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if actor, ok := authctx.From(ctx); ok {
		if actor.IsStaff {
			http.Redirect(w, r, "/admin/dashboard", http.StatusSeeOther)
			return
		}
		if actor.OrgStatus == "approved" || actor.OrgStatus == "active" || actor.OrgStatus == "verified" {
			http.Redirect(w, r, landingPathForActor(actor), http.StatusSeeOther)
			return
		}
	}
	lang, dir := h.localeAndDir(r)
	rejected := r.URL.Query().Get("rejected") == "1" || r.URL.Query().Get("state") == "rejected"

	h.renderPage(ctx, w, "render onboarding pending", pages.OnboardingPending(lang, dir, rejected))
}
