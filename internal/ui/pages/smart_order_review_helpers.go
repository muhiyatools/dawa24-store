package pages

import (
	"github.com/muhiya/dawa24-store/internal/modules/smartorder"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// SmartOrderReviewLine is one line as the review screen shows it.
type SmartOrderReviewLine struct {
	Line           *smartorder.Line
	VendorName     string
	AvailableStock int
	UnitPrice      money.Amount
	DiscountPct    float64
	LineNet        money.Amount
	DecidedBy      smartorder.DecidedBy
	SkippedName    string
	SkippedExcess  float64
	Alternatives   int
}

// SmartOrderReviewGroup is the lines going to one supplier.
type SmartOrderReviewGroup struct {
	VendorName string
	Lines      []SmartOrderReviewLine
	TotalCount int
	Subtotal   money.Amount
}

// SmartOrderReviewData is the review screen's payload.
type SmartOrderReviewData struct {
	Run        *smartorder.Run
	Groups     []SmartOrderReviewGroup
	AllGroups  []SmartOrderReviewGroup
	Excluded   []*smartorder.Line
	BranchName string
	Stale      []smartorder.StaleLine
	Error      string
	Page       int
	PerPage    int
	TotalLines int
}

func decidedLabel(d smartorder.DecidedBy) string {
	switch d {
	case smartorder.DecidedLowestPrice:
		return "أقل سعر"
	case smartorder.DecidedHighestDiscount:
		return "أعلى خصم"
	case smartorder.DecidedFollowedSuppliers:
		return "مورد مفضل"
	case smartorder.DecidedOnlyCandidate:
		return "المورد الوحيد"
	case smartorder.DecidedUser:
		return "اختيار يدوي"
	case smartorder.DecidedDefault:
		return "تلقائي"
	}
	return "—"
}

func totalOrderableItems(groups []SmartOrderReviewGroup) int {
	total := 0
	for _, g := range groups {
		total += len(g.Lines)
	}
	return total
}
