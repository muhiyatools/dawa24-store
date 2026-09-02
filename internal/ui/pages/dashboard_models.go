package pages

import (
	"fmt"
	"strings"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/attachments"
	"github.com/muhiya/dawa24-store/internal/modules/billing"
	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/modules/inventory"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/modules/promo"
	"github.com/muhiya/dawa24-store/internal/modules/smartorder"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// OrgSubscriptionView encapsulates subscription and AI Gateway usage details for tenant dashboards.
type OrgSubscriptionView struct {
	HasSubscription    bool
	PlanName           string
	PlanSlug           string
	Status             string
	ExpiresAt          string
	MaxLoginSessions   int
	MaxDevices         int
	AIPlanID           string
	AIUserID           string
	AIVirtualKeyMasked string
	IsDefaultPlan      bool
	AIRequestsCount    int
	AITokensUsed       int
	AIBudgetLimitUSD   float64
	AIBudgetSpentUSD   float64
	AIBudgetName       string
	AIBudgetResetTime  string
	// HasAIUsage reports whether the Gateway actually answered with this
	// tenant's consumption.
	//
	// It used to be assigned true unconditionally at the end of the loader,
	// after every branch, so a screen with no data at all still claimed to be
	// showing usage. A card that cannot say what a tenant has spent must say
	// so; inventing a number is worse than an empty state, because a pharmacy
	// makes decisions about its plan from this figure.
	HasAIUsage bool
	// HasAIBudget reports whether the Gateway publishes a spending ceiling for
	// this tenant's plan. When false there is no percentage to draw: the limit
	// was previously invented from the plan slug ($15 / $50 / $200), numbers
	// that existed nowhere but one switch statement.
	HasAIBudget bool
}

// TenantSubscriptionPageData represents the full subscription management view model.
type TenantSubscriptionPageData struct {
	Subscription  *OrgSubscriptionView
	Plans         []*billing.Plan
	CurrentPlanID int64
	WalletBalance money.Amount
	AutoRenew     bool
	BillingCycle  string
	NoticeType    string
	NoticeMsg     string
}

// VendorDashboardData is the supplier dashboard view model.
type VendorDashboardData struct {
	ActiveProducts        int
	PendingShipments      int
	DeliveredShipments    int
	TotalShipmentsCount   int
	PendingOrdersTotal    money.Amount
	MonthSales            money.Amount
	MonthNetProfit        money.Amount
	MonthProfitMargin     float64
	MonthCOGS             money.Amount
	WalletBalance         money.Amount
	HasWallet             bool
	LowStockCount         int
	ActiveWarehousesCount int
	Shipments             []*commerce.OrderShipment
	LowStock              []*inventory.Stock
	Offers                []*promo.Offer
	UnreadQuotes          int
	PendingDocRequests    []*attachments.DocumentRequest
	Subscription          *OrgSubscriptionView
}

// AIPercentage returns how much of the published budget has been spent, 0-100.
//
// Callers must check HasAIBudget first: a zero here means "no ceiling is
// known", not "nothing has been spent", and rendering it as a full-width empty
// progress bar tells the reader the opposite of the truth.
func (v *OrgSubscriptionView) AIPercentage() int {
	if v == nil || !v.HasAIBudget || v.AIBudgetLimitUSD <= 0 {
		return 0
	}
	pct := int((v.AIBudgetSpentUSD / v.AIBudgetLimitUSD) * 100)
	if pct < 0 {
		return 0
	}
	if pct > 100 {
		return 100
	}
	return pct
}

// FormatRelativeResetTime converts a reset timestamp into friendly countdown text.
func FormatRelativeResetTime(rawTime string) string {
	cleaned := strings.TrimSpace(rawTime)
	if cleaned == "" {
		return i18n.T("ar", "ai.periodic_renewal")
	}
	layouts := []string{
		"2006-01-02 03:04 PM",
		"2006-01-02 03:04:05 PM",
		"2006-01-02 15:04",
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05Z07:00",
		"2006-01-02T15:04:05Z",
		"2006-01-02",
		"03:04 PM 02-01-2006",
		"02-01-2006 03:04 PM",
		"15:04 02-01-2006",
		"02-01-2006 15:04",
		"02-01-2006",
	}
	var resetTime time.Time
	for _, l := range layouts {
		if t, err := time.Parse(l, cleaned); err == nil {
			resetTime = t
			break
		}
	}
	if resetTime.IsZero() {
		return i18n.T("ar", "ai.periodic_renewal")
	}
	now := time.Now()
	diff := resetTime.Sub(now)
	if diff <= 0 {
		return i18n.T("ar", "ai.today_renewing")
	}
	hours := int(diff.Hours())
	days := hours / 24
	if days > 1 {
		return fmt.Sprintf(i18n.T("ar", "ai.remaining_days_format"), days)
	} else if days == 1 {
		return i18n.T("ar", "ai.remaining_one_day")
	} else if hours > 1 {
		return fmt.Sprintf(i18n.T("ar", "ai.remaining_hours_format"), hours)
	} else if hours == 1 {
		return i18n.T("ar", "ai.remaining_one_hour")
	}
	mins := int(diff.Minutes())
	if mins > 1 {
		return fmt.Sprintf(i18n.T("ar", "ai.remaining_mins_format"), mins)
	}
	return i18n.T("ar", "ai.remaining_less_min")
}

// AIResetText says when the Gateway's budget window reopens.
func (v *OrgSubscriptionView) AIResetText() string {
	if v == nil || v.AIBudgetResetTime == "" {
		return i18n.T("ar", "ai.unspecified")
	}
	return FormatRelativeResetTime(v.AIBudgetResetTime)
}

// AIUsageText renders the spend against the ceiling, or says which of the two
// is unknown.
func (v *OrgSubscriptionView) AIUsageText() string {
	if v == nil || !v.HasAIUsage {
		return i18n.T("ar", "ai.no_usage_data")
	}
	if !v.HasAIBudget {
		return fmt.Sprintf(i18n.T("ar", "ai.consumed_no_ceiling_format"), v.AIBudgetSpentUSD)
	}
	return fmt.Sprintf(i18n.T("ar", "ai.consumed_of_total_format"), v.AIBudgetSpentUSD, v.AIBudgetLimitUSD)
}

// AILogItemView represents one unified, high-density AI request log entry.
type AILogItemView struct {
	ID            string
	Timestamp     string
	TimeFormatted string
	FeatureName   string
	FeatureKey    string
	ModelAlias    string
	ModelTier     string
	InputTokens   int
	OutputTokens  int
	TotalTokens   int
	CostUSD       float64
	DurationMs    int64
	Status        string // "success", "cached", "failed", "completed"
	StatusLabel   string
	SourceContext string
	// CostKnown and DurationKnown separate a real zero from an absent value.
	//
	// Rows drawn from the assistant's own message table used to synthesise both
	// — a cost from invented per-token rates and a flat 280 ms latency — which
	// put fabricated figures in the same column as the Gateway's measured ones,
	// indistinguishable to the reader.
	CostKnown     bool
	DurationKnown bool
}

// CostText renders the billed cost, or says it is not published.
func (l *AILogItemView) CostText() string {
	if l == nil || !l.CostKnown {
		return "—"
	}
	return fmt.Sprintf("%.6f$", l.CostUSD)
}

// DurationText renders the measured latency, or says it is not published.
func (l *AILogItemView) DurationText() string {
	if l == nil || !l.DurationKnown {
		return "—"
	}
	return fmt.Sprintf("%d ms", l.DurationMs)
}

// ModelText renders the model that served the request.
func (l *AILogItemView) ModelText() string {
	if l == nil || strings.TrimSpace(l.ModelAlias) == "" {
		return i18n.T("ar", "ai.unspecified")
	}
	return l.ModelAlias
}

// AIConsumptionLogsPageData represents the full view model for the AI Consumption Logs page.
type AIConsumptionLogsPageData struct {
	Logs              []*AILogItemView
	TotalRequests     int
	TotalTokens       int
	TotalCostUSD      float64
	ActiveBudgetLimit float64
	ActiveBudgetSpent float64
	UsagePercentage   int
	AIUserID          string
	PlanName          string
	PlanSlug          string
	AIPlanID          string
	ResetTime         string
	IsVendor          bool
	IsCustomer        bool
	FeatureBreakdown  map[string]int
	HasBudget         bool
	CostIsComplete    bool
	Page              int
	PerPage           int
	TotalCount        int
}

// TotalCostText renders the window's spend, marked as a floor when some of the
// requests in it carried no published price.
func (d *AIConsumptionLogsPageData) TotalCostText() string {
	if d == nil {
		return "—"
	}
	if d.TotalRequests == 0 {
		return i18n.T("ar", "ai.no_operations")
	}
	if !d.CostIsComplete {
		return fmt.Sprintf(i18n.T("ar", "ai.at_least_cost_format"), d.TotalCostUSD)
	}
	return fmt.Sprintf("%.4f$", d.TotalCostUSD)
}

// QuotaText renders the quota headline, or says the Gateway published none.
func (d *AIConsumptionLogsPageData) QuotaText() string {
	if d == nil || !d.HasBudget {
		return i18n.T("ar", "ai.no_ceiling_published")
	}
	return fmt.Sprintf("%d%%", d.UsagePercentage)
}

// ResetCountdown returns friendly relative text for quota renewal.
func (d *AIConsumptionLogsPageData) ResetCountdown() string {
	if d == nil || strings.TrimSpace(d.ResetTime) == "" {
		return i18n.T("ar", "ai.unspecified")
	}
	return FormatRelativeResetTime(d.ResetTime)
}

// PharmacyDashboardData is the pharmacy dashboard view model.
type PharmacyDashboardData struct {
	// Top 8 KPI Metrics
	TotalOrders       int
	CompletedOrders   int
	ActiveOrders      int
	CancelledOrders   int
	TotalSpend        money.Amount
	MonthSpend        money.Amount
	WalletBalance     money.Amount
	HasWallet         bool
	TotalOrderedItems int
	SmartOrdersCount  int

	// Pharmacy & Branch Context
	CustomerOrgName  string
	ActiveBranchID   int64
	ActiveBranchName string
	TotalBranches    int
	ActiveBranches   int
	Branches         []*org.Branch

	// Recent Orders Table
	Orders []*commerce.Order

	// Wallet Details
	PendingDepositsTotal money.Amount
	PendingDepositsCount int
	RecentTransactions   []*billing.WalletTransaction

	// Smart Orders Section
	SmartOrdersTotal       int
	SmartOrdersProcessing  int
	SmartOrdersCompleted   int
	SmartOrdersNeedsReview int
	RecentSmartOrders      []*smartorder.Run

	// Legacy & Supplemental
	Favorites          int
	ActiveOffers       int
	Offers             []*promo.Offer
	PendingDocRequests []*attachments.DocumentRequest
	Subscription       *OrgSubscriptionView
}

// FormatSmartOrderStatusLabel returns the localized label for Smart Order RunStatus.
func FormatSmartOrderStatusLabel(status smartorder.RunStatus) string {
	switch status {
	case smartorder.StatusPlaced, smartorder.StatusCompleted, smartorder.StatusFinalizing:
		return i18n.T("ar", "smartorder.status_placed")
	case smartorder.StatusProcessing, smartorder.StatusQueued:
		return i18n.T("ar", "smartorder.status_processing")
	case smartorder.StatusMapping:
		return i18n.T("ar", "smartorder.status_mapping")
	case smartorder.StatusDraft:
		return i18n.T("ar", "smartorder.status_draft")
	case smartorder.StatusStale:
		return i18n.T("ar", "smartorder.status_stale")
	case smartorder.StatusFailed:
		return i18n.T("ar", "smartorder.status_failed")
	default:
		return string(status)
	}
}
