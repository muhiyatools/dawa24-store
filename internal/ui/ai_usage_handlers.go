package ui

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/muhiya/dawa24-store/internal/platform/aiusage"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/gateway"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

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

	h.renderPage(ctx, w, "render ai consumption logs", pages.AIConsumptionLogsPage(pageData, lang, dir))
}

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
