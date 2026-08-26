package pages

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/a-h/templ"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
)

// View models for the catalogue import wizard.
//
// The wizard is three screens — configure, review, confirm — and each of them
// needs the same underlying session rendered differently. Assembling that here
// rather than in the templates keeps the decisions testable and keeps the .templ
// files to markup.

// ImportStep identifies which screen is showing, for the progress rail.
type ImportStep int

const (
	// StepConfigure is the upload screen: choose a file.
	StepConfigure ImportStep = iota
	// StepMapping is the column review: how the file will be read, what it
	// yields, and the strategy to run it under. Nothing is staged until the
	// admin leaves this screen deliberately.
	StepMapping
	// StepReview is the staged result: counts, per-row table, confirmation.
	StepReview
	// StepDone is the committed summary.
	StepDone
)

// ImportStepInfo labels one node on the progress rail.
type ImportStepInfo struct {
	Step    ImportStep
	Number  int
	Icon    string
	Title   string
	Active  bool
	Done    bool
	Pending bool
}

// ImportSteps renders the rail for the step currently showing.
func ImportSteps(current ImportStep) []ImportStepInfo {
	labels := []struct {
		icon, title string
	}{
		{"📤", "رفع الملف"},
		{"🧭", "مراجعة الأعمدة"},
		{"🔍", "مراجعة النتائج"},
		{"✅", "الحفظ في الكتالوج"},
	}

	out := make([]ImportStepInfo, 0, len(labels))
	for i, l := range labels {
		step := ImportStep(i)
		out = append(out, ImportStepInfo{
			Step:    step,
			Number:  i + 1,
			Icon:    l.icon,
			Title:   l.title,
			Active:  step == current,
			Done:    step < current,
			Pending: step > current,
		})
	}
	return out
}

// ImportToggle is one switch on the configuration screen.
type ImportToggle struct {
	Name        string
	Icon        string
	Title       string
	Description string
	Checked     bool
	// Disabled marks a switch that cannot be used right now, with Note saying
	// why. The AI switch uses this when the Gateway is unreachable.
	Disabled bool
	Note     string
}

// ImportConfigureView is the upload screen.
type ImportConfigureView struct {
	Modes        []catalog.ImportModeOption
	SelectedMode catalog.ImportMode
	Toggles      []ImportToggle
	Categories   []catalog.TaxonomyOption
	AIAvailable  bool
	// Recent is the import history panel.
	Recent []*catalog.ImportSession
	// Fatal explains a rejected upload, if the admin has just had one.
	Fatal       string
	FatalDetail string
}

// NewImportConfigureView builds the upload screen from the platform's state.
func NewImportConfigureView(
	categories []catalog.TaxonomyOption, recent []*catalog.ImportSession, aiAvailable bool,
) ImportConfigureView {
	opts := catalog.DefaultImportOptions()
	return ImportConfigureView{
		Modes:        catalog.ImportModeOptions,
		SelectedMode: catalog.ModeUpdateAndAdd,
		Toggles:      importToggles(opts, aiAvailable),
		Categories:   categories,
		Recent:       recent,
		AIAvailable:  aiAvailable,
	}
}

// importToggles renders the enrichment switches in a fixed order.
func importToggles(opts catalog.ImportOptions, aiAvailable bool) []ImportToggle {
	toggles := []ImportToggle{
		{
			Name: "auto_create_brands", Icon: "🏭",
			Title:       "إنشاء الشركات المصنعة تلقائياً",
			Description: "تسجيل أي شركة مصنعة غير موجودة في قائمة الشركات وربط الأصناف بها.",
			Checked:     opts.AutoCreateBrands,
		},
		{
			Name: "assign_category", Icon: "🗂️",
			Title:       "تحديد فئة المنتج",
			Description: "ربط كل صنف بالفئة المناسبة من فئات المنصة الموجودة.",
			Checked:     opts.AssignCategory,
		},
		{
			Name: "auto_create_categories", Icon: "➕",
			Title:       "إنشاء الفئات غير الموجودة",
			Description: "إضافة الفئات المستوردة التي لا تطابق أي فئة حالية. الفئات الموجودة يُعاد استخدامها دائماً.",
			Checked:     opts.AutoCreateCategories,
		},
		{
			Name: "assign_dosage_form", Icon: "💊",
			Title:       "تحديد الشكل الصيدلي",
			Description: "استنتاج الشكل الصيدلي (أقراص، شراب، كريم…) من اسم الصنف.",
			Checked:     opts.AssignDosageForm,
		},
		{
			Name: "assign_scientific_name", Icon: "🧪",
			Title:       "تحديد الاسم العلمي",
			Description: "تحديد الاسم العلمي للمادة الفعالة لكل صنف دوائي.",
			Checked:     opts.AssignScientificName,
		},
	}

	ai := ImportToggle{
		Name: "use_ai", Icon: "🤖",
		Title: "مساعدة الذكاء الاصطناعي",
		Description: "ثلاثة طلبات ثابتة مهما كان حجم الملف — تحديد معنى الأعمدة، ومطابقة الفئات " +
			"والأشكال الصيدلية — ثم طلبات مجمّعة للأصناف التي لم يحسمها التطابق الحتمي وحده " +
			"(٢٥ صنفاً في الطلب الواحد، ومن بين مرشحين محدّدين فقط). لا يُعالَج أي صف على حدة، " +
			"وإيقافه لا يعطّل الاستيراد بل يقلّل نسبة المطابقة فقط.",
		Checked: opts.UseAI && aiAvailable,
		Note:    "اختياري — الاستيراد يعمل بالكامل بدون تفعيله.",
	}
	if !aiAvailable {
		ai.Disabled = true
		ai.Note = "خدمة الذكاء الاصطناعي غير متاحة حالياً."
	}
	return append(toggles, ai)
}

// ImportReviewView is the review and confirmation screen.
type ImportReviewView struct {
	Session *catalog.ImportSession
	Counts  catalog.StagingCounts
	Rows    []*catalog.StagingRow
	Total   int

	// Structure is how the file was read, as stored on the session.
	Structure catalog.FileStructure
	Bindings  []ImportBindingRow
	Unmapped  []string

	Modes       []catalog.ImportModeOption
	Toggles     []ImportToggle
	Categories  []catalog.TaxonomyOption
	AIAvailable bool

	// Filter is the review table's current state, for the toolbar.
	Filter catalog.StagingFilter
	Page   int
	Pages  int

	Notice      string
	NoticeKind  string
	FatalDetail string

	// Progress is the live state of a background preparation run. While one is
	// in flight the review tables are meaningless — they still hold the previous
	// run's rows — so the page shows the progress panel instead.
	Progress catalog.ImportProgress
	Working  bool
}

// ProgressPercent is how far the run has got, or -1 when the phase carries no
// count and the bar should read as indeterminate.
func (v ImportReviewView) ProgressPercent() int { return v.Progress.Percent() }

// ProgressPhases lists the stages with the current one marked, so the admin can
// see what is happening rather than watching an unlabelled bar.
func (v ImportReviewView) ProgressPhases() []ImportPhaseInfo {
	phases := []catalog.ImportPhase{
		catalog.ImportPhaseReading,
		catalog.ImportPhaseParsing,
		catalog.ImportPhaseMapping,
		catalog.ImportPhaseMatching,
		catalog.ImportPhaseStaging,
	}

	current := -1
	for i, phase := range phases {
		if phase == v.Progress.Phase {
			current = i
		}
	}

	out := make([]ImportPhaseInfo, 0, len(phases))
	for i, phase := range phases {
		out = append(out, ImportPhaseInfo{
			Label:  phase.Label(),
			Active: i == current,
			Done:   current > i || v.Progress.Phase == catalog.ImportPhaseDone,
		})
	}
	return out
}

// ImportPhaseInfo is one stage on the progress panel.
type ImportPhaseInfo struct {
	Label  string
	Active bool
	Done   bool
}

// NewImportReviewView assembles the review screen.
func NewImportReviewView(
	session *catalog.ImportSession, counts catalog.StagingCounts,
	rows []*catalog.StagingRow, total int, filter catalog.StagingFilter,
	categories []catalog.TaxonomyOption, aiAvailable bool,
) ImportReviewView {
	view := ImportReviewView{
		Session:     session,
		Counts:      counts,
		Rows:        rows,
		Total:       total,
		Modes:       catalog.ImportModeOptions,
		Toggles:     importToggles(session.Options, aiAvailable),
		Categories:  categories,
		AIAvailable: aiAvailable,
		Filter:      filter,
	}

	limit := filter.Limit
	if limit <= 0 {
		limit = 100
	}
	view.Page = filter.Offset/limit + 1
	view.Pages = max((total+limit-1)/limit, 1)
	return view
}

// SetStructure fills the review screen's read-only account of how the file was
// interpreted, from the description stored on the session.
//
// Read-only on purpose. Correcting a mapping belongs to step two, and having
// those controls here is what made the review page re-read and re-decode a
// 32 MB workbook on every render, every filter change and every row toggle.
func (v *ImportReviewView) SetStructure(structure catalog.FileStructure) {
	v.Structure = structure
	for _, col := range structure.Columns {
		if col.Field == "" {
			if col.Header != "" {
				v.Unmapped = append(v.Unmapped, col.Header)
			}
			continue
		}
		v.Bindings = append(v.Bindings, ImportBindingRow{
			Column:     col.Letter,
			Header:     col.Header,
			Field:      catalog.FieldLabels[col.Field],
			Confidence: col.Confidence,
			Sample:     col.SampleText(),
		})
	}
}

// CanCommit reports whether there is anything selected to write.
func (v ImportReviewView) CanCommit() bool {
	return v.Session != nil && v.Session.IsReviewable() && v.Counts.Affected() > 0
}

// ModeIsDestructive reports whether the chosen strategy archives the catalogue,
// which the confirm button warns about.
func (v ImportReviewView) ModeIsDestructive() bool {
	return v.Session != nil && v.Session.Mode.IsDestructive()
}

// SourceSummary describes what was read, in one line.
func (v ImportReviewView) SourceSummary() string {
	if v.Session == nil {
		return ""
	}
	parts := []string{v.Session.Filename}
	switch v.Session.SourceFormat {
	case "xlsx":
		if v.Session.SheetName != "" {
			parts = append(parts, fmt.Sprintf("ورقة «%s»", v.Session.SheetName))
		}
	case "csv":
		parts = append(parts, fmt.Sprintf("فاصل «%s»", delimiterLabel(v.Session.Delimiter)))
	}
	if v.Session.BlockCount > 1 {
		parts = append(parts, fmt.Sprintf("%d كتلة بيانات", v.Session.BlockCount))
	}
	parts = append(parts, fmt.Sprintf("%d صف", v.Session.TotalRows))
	return strings.Join(parts, " · ")
}

// EnrichmentSummary is what the taxonomy passes resolved on this run.
//
// The category and form passes run whether or not AI is on — exact folding
// against the catalogue's own vocabulary is the first tier and usually the only
// one — so this is not an AI report. It was rendered under a robot badge
// regardless, which told an administrator who had deliberately left AI off that
// a model had been consulted.
func (v ImportReviewView) EnrichmentSummary() string {
	if v.Session == nil || v.Session.AINote == "" {
		return ""
	}
	if v.Session.AICalls == 0 {
		return v.Session.AINote
	}
	return fmt.Sprintf("%s (%d طلب ذكاء اصطناعي)", v.Session.AINote, v.Session.AICalls)
}

// UsedAI reports whether a model was actually asked anything, which is what the
// robot badge should mean.
func (v ImportReviewView) UsedAI() bool { return v.Session != nil && v.Session.AICalls > 0 }

// MatchRate is the share of the file that resolved to a product the catalogue
// already holds, rendered as a percentage.
//
// It is the number an admin watches to know whether an import is about to
// update the catalogue or to duplicate it: everything that did not match is
// staged as a new product, so a low rate on a file of familiar medicines means
// the matching is failing, not that the catalogue is missing them.
func (v ImportReviewView) MatchRate() string {
	if v.Session == nil {
		return "—"
	}
	considered := v.Counts.Insert + v.Counts.Update
	if considered == 0 {
		return "—"
	}
	return fmt.Sprintf("%d%%", v.Counts.Update*100/considered)
}

// RowActionBadge picks the badge colour for a staged action.// RowActionBadge picks the badge colour for a staged action.
func RowActionBadge(action catalog.RowAction) string {
	switch action {
	case catalog.ActionInsert:
		return "badge-emerald"
	case catalog.ActionUpdate:
		return "badge-sky"
	default:
		return "badge-slate"
	}
}

// SessionStatusLabel renders a session state in the admin's language.
func SessionStatusLabel(status catalog.SessionStatus) string {
	switch status {
	case catalog.SessionDraft:
		return "بانتظار مراجعة الأعمدة"
	case catalog.SessionProcessing:
		return "جارٍ المعالجة"
	case catalog.SessionReady:
		return "بانتظار التأكيد"
	case catalog.SessionCommitted:
		return "تم الحفظ"
	case catalog.SessionCancelled:
		return "ملغاة"
	case catalog.SessionFailed:
		return "فشلت"
	default:
		return string(status)
	}
}

// SessionStatusBadge picks the badge colour for a session state.
func SessionStatusBadge(status catalog.SessionStatus) string {
	switch status {
	case catalog.SessionCommitted:
		return "badge-emerald"
	case catalog.SessionProcessing:
		return "badge-sky"
	case catalog.SessionReady:
		return "badge-amber"
	case catalog.SessionFailed:
		return "badge-rose"
	default:
		return "badge-slate"
	}
}

// ProductSummaryLine renders a staged product's details for the review table.
func ProductSummaryLine(row *catalog.StagingRow) string {
	if row == nil {
		return ""
	}
	return catalog.SummarizeProduct(row.Product)
}

// FormatCount renders a number with thousands separators, which matters when
// the figure on screen is 8,790 rather than 12.
func FormatCount(n int) string {
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	var b strings.Builder
	for i, digit := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(digit)
	}
	return b.String()
}

// URL helpers for the review screen.
//
// templ.SafeURL is the escape hatch that says "this string is a trusted URL".
// Every one of these is built from a session's own UUID and fixed path
// segments, never from anything a file or a query string supplied.

// importAction builds a POST target on the session.
func importAction(view ImportReviewView, verb string) templ.SafeURL {
	return templ.SafeURL(fmt.Sprintf("/admin/products/import/%s/%s", view.Session.PublicID, verb))
}

// importRowAction builds the include/exclude target for one staged row.
func importRowAction(view ImportReviewView, row *catalog.StagingRow) templ.SafeURL {
	return templ.SafeURL(fmt.Sprintf("/admin/products/import/%s/rows/%d",
		view.Session.PublicID, row.ID))
}

// includedToggleValue is what the row's button submits: the opposite of where
// the row is now, so one click flips it.
func includedToggleValue(row *catalog.StagingRow) string {
	if row.Included {
		return "0"
	}
	return "1"
}

// importPageURL keeps the active filter while moving through pages.
func importPageURL(view ImportReviewView, page int) templ.SafeURL {
	return templ.SafeURL("/admin/products/import/" + view.Session.PublicID +
		"?" + importQuery(view, view.currentFilterKey(), page).Encode())
}

// importFilterURL switches the active filter and returns to the first page.
func importFilterURL(view ImportReviewView, key string) templ.SafeURL {
	return templ.SafeURL("/admin/products/import/" + view.Session.PublicID +
		"?" + importQuery(view, key, 1).Encode())
}

func importQuery(view ImportReviewView, filterKey string, page int) url.Values {
	q := url.Values{}
	if filterKey != "" {
		q.Set("filter", filterKey)
	}
	if view.Filter.Search != "" {
		q.Set("q", view.Filter.Search)
	}
	if page > 1 {
		q.Set("page", fmt.Sprintf("%d", page))
	}
	return q
}

// currentFilterKey renders the active filter back into its query value.
func (v ImportReviewView) currentFilterKey() string {
	switch {
	case v.Filter.OnlyIssues:
		return "issues"
	case v.Filter.OnlyAI:
		return "ai"
	case v.Filter.Action != "":
		return string(v.Filter.Action)
	default:
		return ""
	}
}

// FilterIsActive reports whether a chip is the one currently applied.
func (v ImportReviewView) FilterIsActive(key string) bool {
	return v.currentFilterKey() == key
}

// ParseStagingFilter reads the review table's controls out of a query string.
func ParseStagingFilter(values url.Values, pageSize int) catalog.StagingFilter {
	filter := catalog.StagingFilter{
		Search: strings.TrimSpace(values.Get("q")),
		Limit:  pageSize,
	}

	switch values.Get("filter") {
	case string(catalog.ActionInsert):
		filter.Action = catalog.ActionInsert
	case string(catalog.ActionUpdate):
		filter.Action = catalog.ActionUpdate
	case string(catalog.ActionSkip):
		filter.Action = catalog.ActionSkip
	case "issues":
		filter.OnlyIssues = true
	case "ai":
		filter.OnlyAI = true
	}

	if page, err := strconv.Atoi(values.Get("page")); err == nil && page > 1 {
		filter.Offset = (page - 1) * pageSize
	}
	return filter
}

// SetRows fills the review table after the rows have been read.
//
// Paging is computed here rather than at construction because the rows are
// fetched only for a session that has finished preparing; one in flight is
// rendered without them.
func (v *ImportReviewView) SetRows(rows []*catalog.StagingRow, total int, counts catalog.StagingCounts) {
	v.Rows, v.Total, v.Counts = rows, total, counts

	limit := v.Filter.Limit
	if limit <= 0 {
		limit = 100
	}
	v.Page = v.Filter.Offset/limit + 1
	v.Pages = max((total+limit-1)/limit, 1)
}
