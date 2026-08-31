package pages

import (
	"github.com/muhiya/dawa24-store/internal/modules/billing"
	"github.com/muhiya/dawa24-store/internal/modules/smartorder"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

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

// FormatTxTypeLabel returns the localized label for a wallet transaction type.
func FormatTxTypeLabel(t billing.TransactionType) string {
	switch t {
	case billing.TxDeposit:
		return i18n.T("ar", "tx.deposit")
	case billing.TxWithdrawal:
		return i18n.T("ar", "tx.withdrawal")
	case billing.TxPurchase:
		return i18n.T("ar", "tx.purchase")
	case billing.TxRefund:
		return i18n.T("ar", "tx.refund")
	case billing.TxBonus:
		return i18n.T("ar", "tx.bonus")
	case billing.TxPenalty:
		return i18n.T("ar", "tx.penalty")
	case billing.TxTransferIn:
		return i18n.T("ar", "tx.transfer_in")
	case billing.TxTransferOut:
		return i18n.T("ar", "tx.transfer_out")
	case billing.TxAdjustment:
		return i18n.T("ar", "tx.adjustment")
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
