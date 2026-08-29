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
	HasAIUsage         bool
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
	ActiveProducts     int
	PendingShipments   int
	MonthSales         money.Amount
	WalletBalance      money.Amount
	HasWallet          bool
	LowStockCount      int
	Shipments          []*commerce.OrderShipment
	LowStock           []*inventory.Stock
	Offers             []*promo.Offer
	UnreadQuotes       int
	PendingDocRequests []*attachments.DocumentRequest
	Subscription       *OrgSubscriptionView
}

// AIPercentage returns the percentage of AI usage (0 - 100).
func (v *OrgSubscriptionView) AIPercentage() int {
	if v == nil || v.AIBudgetLimitUSD <= 0 {
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

// FormatRelativeResetTime converts a reset timestamp into friendly countdown text (e.g. "متبقي 14 يوماً", "متبقي 8 ساعات").
func FormatRelativeResetTime(rawTime string) string {
	cleaned := strings.TrimSpace(rawTime)
	if cleaned == "" {
		return "تجديد شهري دوري"
	}
	layouts := []string{
		"2006-01-02 15:04",
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05Z07:00",
		"2006-01-02T15:04:05Z",
		"2006-01-02",
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
		return "تجديد شهري دوري"
	}
	now := time.Now()
	diff := resetTime.Sub(now)
	if diff <= 0 {
		return "اليوم (جاري التجديد)"
	}
	hours := int(diff.Hours())
	days := hours / 24
	if days > 1 {
		return fmt.Sprintf("متبقي %d يوماً", days)
	} else if days == 1 {
		return "متبقي يوم واحد"
	} else if hours > 1 {
		return fmt.Sprintf("متبقي %d ساعة", hours)
	} else if hours == 1 {
		return "متبقي ساعة واحدة"
	}
	mins := int(diff.Minutes())
	if mins > 1 {
		return fmt.Sprintf("متبقي %d دقيقة", mins)
	}
	return "أقل من دقيقة"
}

// AIResetText returns a user-friendly string indicating when the quota resets.
func (v *OrgSubscriptionView) AIResetText() string {
	if v == nil || v.AIBudgetResetTime == "" {
		return "تجديد دوري مستمر"
	}
	return FormatRelativeResetTime(v.AIBudgetResetTime)
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
}

// ResetCountdown returns friendly relative text for quota renewal.
func (d *AIConsumptionLogsPageData) ResetCountdown() string {
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

// FormatSmartOrderStatusLabel returns the localized Arabic label for Smart Order RunStatus.
func FormatSmartOrderStatusLabel(status smartorder.RunStatus) string {
	switch status {
	case smartorder.StatusPlaced, smartorder.StatusCompleted, smartorder.StatusFinalizing:
		return "مكتمل ومعتمد"
	case smartorder.StatusProcessing, smartorder.StatusQueued:
		return "جاري المعالجة"
	case smartorder.StatusMapping:
		return "بانتظار تعيين الأعمدة"
	case smartorder.StatusDraft:
		return "مسودة"
	case smartorder.StatusStale:
		return "بحاجة لإعادة تشغيل"
	case smartorder.StatusFailed:
		return "تعذّر الاكتمال"
	default:
		return string(status)
	}
}

// FormatSmartOrderStatusTone returns the badge tone for Smart Order RunStatus.
func FormatSmartOrderStatusTone(status smartorder.RunStatus) string {
	switch status {
	case smartorder.StatusPlaced, smartorder.StatusCompleted:
		return "emerald"
	case smartorder.StatusProcessing, smartorder.StatusQueued, smartorder.StatusFinalizing:
		return "amber"
	case smartorder.StatusMapping, smartorder.StatusDraft:
		return "sky"
	case smartorder.StatusStale, smartorder.StatusFailed:
		return "rose"
	default:
		return "slate"
	}
}

// FormatTxTypeLabel returns the Arabic translation for a wallet transaction type.
func FormatTxTypeLabel(t billing.TransactionType) string {
	switch t {
	case billing.TxDeposit:
		return "إيداع رصيد"
	case billing.TxWithdrawal:
		return "سحب / استرداد"
	case billing.TxPurchase:
		return "سداد طلبية"
	case billing.TxRefund:
		return "استرجاع مالي"
	case billing.TxBonus:
		return "مكافأة / بونص"
	case billing.TxPenalty:
		return "خصم إداري"
	case billing.TxTransferIn:
		return "تحويل وارد"
	case billing.TxTransferOut:
		return "تحويل صادر"
	case billing.TxAdjustment:
		return "تسوية رصيد"
	default:
		return string(t)
	}
}

// CoveredPharmacyItem represents one pharmacy branch covered by the vendor's distribution network.
type CoveredPharmacyItem struct {
	PharmacyID         int64
	PharmacyName       string
	PharmacyTradeName  string
	BranchID           int64
	BranchName         string
	Address            string
	Phone              string
	CityID             *int64
	CityName           string
	CoveringBranchID   int64
	CoveringBranchName string
	DistanceMeters     int
	DistanceKM         float64
	CoveredDays        []int
	CoveredDaysLabels  []string
	TimeWindow         string
	IsCoveredToday     bool
	MatchReason        string
}

// VendorPharmacyCoverageData encapsulates all data for the /vendor/pharmacy-coverage page.
type VendorPharmacyCoverageData struct {
	Pharmacies        []CoveredPharmacyItem
	TotalPharmacies   int
	CoveredTodayCount int
	CoveredCities     []string
	CoveredBranches   []string
	FilterDay         string
	FilterBranch      string
	FilterCity        string
	SearchQuery       string
}
