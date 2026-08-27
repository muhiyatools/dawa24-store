package pages

import (
	"fmt"

	"github.com/muhiya/dawa24-store/internal/modules/ingest"
	"github.com/muhiya/dawa24-store/internal/modules/inventory"
	"github.com/muhiya/dawa24-store/internal/shared/productmatch"
)

// The vendor catalogue import screen.
//
// One page, seven stages, one view model. The stage is read off the session
// rather than off the URL, so a vendor who reloads, or returns to a link from
// yesterday, lands exactly where they left off instead of at the start.

// VendorImportView is everything the import screen renders.
type VendorImportView struct {
	Session  *ingest.Session
	Analysis *productmatch.Analysis

	Warehouses []*inventory.Warehouse
	Recent     []*ingest.Session

	// Rows and its companions back the results table.
	Rows      []*ingest.RowOutcome
	RowTotal  int
	RowCounts map[string]int
	Filter    ingest.RowFilter
	Page      int
	PerPage   int

	// AIAvailable says whether the platform can actually run the AI tier. The
	// switch is rendered disabled with a reason when it cannot, rather than
	// offering a toggle that ticks and then does nothing.
	AIAvailable bool
	// AIUnavailableReason is what to tell the vendor when it cannot.
	AIUnavailableReason string

	NoticeType    string
	NoticeMessage string
	// Fatal is a message that replaces the whole stage, for a file that could
	// not be read at all.
	Fatal string
}

// Phase is the stage the screen should render.
func (v VendorImportView) Phase() ingest.Phase {
	if v.Session == nil {
		return ""
	}
	return v.Session.Phase
}

// Started reports whether an import is open at all, as opposed to the upload
// screen.
func (v VendorImportView) Started() bool { return v.Session != nil }

// VendorImportStep is one node on the progress rail.
type VendorImportStep struct {
	Number int
	Title  string
	Icon   string
	Active bool
	Done   bool
}

// VendorImportSteps renders the rail for the current phase.
func VendorImportSteps(phase ingest.Phase) []VendorImportStep {
	steps := []VendorImportStep{
		{Number: 1, Title: "رفع الملف", Icon: "📤"},
		{Number: 2, Title: "ربط الأعمدة والتحقق", Icon: "🔗"},
		{Number: 3, Title: "إعدادات الاستيراد", Icon: "⚙️"},
		{Number: 4, Title: "مراجعة الأصناف والمطابقة", Icon: "📋"},
		{Number: 5, Title: "النتائج", Icon: "📊"},
	}
	current := 1
	switch phase {
	case ingest.PhaseMapping:
		current = 2
	case ingest.PhaseSettings:
		current = 3
	case ingest.PhaseReview, ingest.PhaseConfirm, ingest.PhaseProcessing:
		current = 4
	case ingest.PhaseCompleted, ingest.PhaseFailed, ingest.PhaseCancelled:
		current = 5
	}
	for i := range steps {
		steps[i].Active = steps[i].Number == current
		steps[i].Done = steps[i].Number < current
	}
	return steps
}

// MappedColumns are the analysed columns, or the stored snapshot once the run
// has finished and the file may already have been reaped.
func (v VendorImportView) MappedColumns() []*productmatch.Column {
	if v.Analysis != nil && v.Analysis.Mapping != nil {
		return v.Analysis.Mapping.Columns
	}
	return nil
}

// CoreFieldStatus tracks the mapping state of critical and required catalog & inventory fields.
type CoreFieldStatus struct {
	Field       productmatch.Field
	Label       string
	Description string
	Required    bool
	ColumnIndex int
	ColumnName  string
}

// CoreFieldsStatus inspects all mapped columns and reports status for every essential field.
func (v VendorImportView) CoreFieldsStatus() []CoreFieldStatus {
	defs := []struct {
		f   productmatch.Field
		l   string
		d   string
		req bool
	}{
		{productmatch.FieldName, "اسم الصنف", "الاسم التجاري أو الوصف في الملف", true},
		{productmatch.FieldPrice, "سعر البيع", "سعر بيع الصنف", true},
		{productmatch.FieldQuantity, "الكمية (رصيد المخزون)", "الكمية المتوفرة لتسجيلها في المستودع المحدد", true},
		{productmatch.FieldSKU, "كود الصنف (SKU)", "رمز الصنف الداخلي لديك", false},
		{productmatch.FieldBarcode, "الباركود الدولي", "رقم الباركود للمطابقة التلقائية الفورية", false},
		{productmatch.FieldExpiryDate, "تاريخ الصلاحية", "تاريخ انتهاء صلاحية التشغيلة", false},
		{productmatch.FieldDiscountPct, "نسبة الخصم %", "نسبة الخصم الممنوحة على الصنف", false},
	}

	cols := v.MappedColumns()
	res := make([]CoreFieldStatus, 0, len(defs))
	for _, d := range defs {
		st := CoreFieldStatus{
			Field:       d.f,
			Label:       d.l,
			Description: d.d,
			Required:    d.req,
			ColumnIndex: -1,
			ColumnName:  "",
		}
		for _, c := range cols {
			if c.Field == d.f && !c.Ignored {
				st.ColumnIndex = c.Index
				if c.Header != "" {
					st.ColumnName = c.Header
				} else {
					st.ColumnName = fmt.Sprintf("العمود %d", c.Index+1)
				}
				break
			}
		}
		res = append(res, st)
	}
	return res
}

// FieldChoices lists the fields a column may be bound to, grouped for the
// dropdown.
func FieldChoices() []productmatch.Spec { return productmatch.Specs }

// FieldGroups lists the review screen's sections in order.
func FieldGroups() []productmatch.Group { return productmatch.Groups }

// ConfidenceTone maps a confidence onto the badge palette.
func ConfidenceTone(c productmatch.Confidence) string {
	switch c {
	case productmatch.ConfidenceCertain:
		return "badge-emerald"
	case productmatch.ConfidenceHigh:
		return "badge-sky"
	case productmatch.ConfidenceMedium:
		return "badge-amber"
	case productmatch.ConfidenceLow:
		return "badge-rose"
	}
	return "badge-slate"
}

// SeverityTone maps a finding's severity onto the badge palette.
func SeverityTone(s productmatch.Severity) string {
	switch s {
	case productmatch.SeverityError:
		return "badge-rose"
	case productmatch.SeverityWarning:
		return "badge-amber"
	}
	return "badge-slate"
}

// OutcomeTone maps a row outcome onto the badge palette.
func OutcomeTone(outcome string) string {
	switch outcome {
	case ingest.OutcomeInserted:
		return "badge-emerald"
	case ingest.OutcomeUpdated:
		return "badge-sky"
	case ingest.OutcomeError:
		return "badge-rose"
	}
	return "badge-slate"
}

// OutcomeLabel renders a row outcome in Arabic.
func OutcomeLabel(outcome string) string {
	switch outcome {
	case ingest.OutcomeInserted:
		return "تمت الإضافة"
	case ingest.OutcomeUpdated:
		return "تم التحديث"
	case ingest.OutcomeError:
		return "خطأ"
	}
	return "تم التخطي"
}

// MatchLevelLabel renders a match level in Arabic.
func MatchLevelLabel(level string) string {
	return productmatch.MatchLevel(level).Label()
}

// MatchLevelBadgeClass maps a match level to a badge tone.
func MatchLevelBadgeClass(level string) string {
	switch productmatch.MatchLevel(level) {
	case productmatch.MatchBarcode, productmatch.MatchCode, productmatch.MatchExact, productmatch.MatchStrong:
		return "badge-emerald"
	case productmatch.MatchReview:
		return "badge-amber"
	case productmatch.MatchAmbiguous:
		return "badge-purple"
	default:
		return "badge-slate"
	}
}

// PercentText renders a 0..1 score as a percentage.
func PercentText(score float64) string {
	return fmt.Sprintf("%d%%", int(score*100+0.5))
}

// FileSizeText renders a byte count the way a vendor reads it.
func FileSizeText(bytes int64) string {
	switch {
	case bytes >= 1<<20:
		return fmt.Sprintf("%.1f ميجابايت", float64(bytes)/float64(1<<20))
	case bytes >= 1<<10:
		return fmt.Sprintf("%.0f كيلوبايت", float64(bytes)/float64(1<<10))
	}
	return fmt.Sprintf("%d بايت", bytes)
}

// ImportModeOptions are the reconciliation strategies for the settings screen.
func ImportModeOptions() []ingest.ModeOption { return ingest.ModeOptions }

// StockModeOptions are the three readings of an imported quantity.
func StockModeOptions() []inventory.StockMode {
	return []inventory.StockMode{inventory.StockReplace, inventory.StockAdd, inventory.StockKeep}
}

// importRowsURL links one filter tab of the results table.
func importRowsURL(publicID, outcome string) string {
	if outcome == "" {
		return "/vendor/ingest/" + publicID
	}
	return "/vendor/ingest/" + publicID + "?outcome=" + outcome
}

// importWarehouseName names the chosen warehouse on the confirmation screen.
func importWarehouseName(view VendorImportView) string {
	if view.Session == nil {
		return ""
	}
	for _, wh := range view.Warehouses {
		if wh.ID == view.Session.Settings.WarehouseID {
			return wh.Name
		}
	}
	return "—"
}
