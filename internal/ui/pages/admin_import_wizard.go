package pages

import "github.com/muhiya/dawa24-store/internal/shared/i18n"
import (
	"fmt"
	"strings"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
)

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
			Title:       i18n.T("ar", "wizard.opt.auto_create_mfr_title"),
			Description: i18n.T("ar", "wizard.opt.auto_create_mfr_desc"),
			Checked:     opts.AutoCreateBrands,
		},
		{
			Name: "assign_category", Icon: "🗂️",
			Title:       i18n.T("ar", "wizard.opt.assign_cat_title"),
			Description: i18n.T("ar", "wizard.opt.assign_cat_desc"),
			Checked:     opts.AssignCategory,
		},
		{
			Name: "auto_create_categories", Icon: "➕",
			Title:       i18n.T("ar", "wizard.opt.auto_create_cats_title"),
			Description: i18n.T("ar", "wizard.opt.auto_create_cats_desc"),
			Checked:     opts.AutoCreateCategories,
		},
		{
			Name: "assign_dosage_form", Icon: "💊",
			Title:       i18n.T("ar", "wizard.opt.assign_dosage_title"),
			Description: i18n.T("ar", "wizard.opt.assign_dosage_desc"),
			Checked:     opts.AssignDosageForm,
		},
		{
			Name: "assign_scientific_name", Icon: "🧪",
			Title:       i18n.T("ar", "wizard.opt.assign_scientific_title"),
			Description: i18n.T("ar", "wizard.opt.assign_scientific_desc"),
			Checked:     opts.AssignScientificName,
		},
	}

	// One line, not four paragraphs. The description used to explain the
	// request batching, the candidate shortlisting and the per-request row
	// count — accurate, and nothing an administrator deciding whether to tick a
	// box needs to read. It is a feature, not a whitepaper.
	ai := ImportToggle{
		Name: "use_ai", Icon: "🤖",
		Title:   i18n.T("ar", "wizard.opt.use_ai_title"),
		Checked: opts.UseAI && aiAvailable,
	}
	if !aiAvailable {
		ai.Disabled = true
		ai.Checked = false
		ai.Note = i18n.T("ar", "wizard.opt.use_ai_note_unavailable")
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
			parts = append(parts, fmt.Sprintf(i18n.TDefault("w4_ui.s_32"), v.Session.SheetName))
		}
	case "csv":
		parts = append(parts, fmt.Sprintf(i18n.TDefault("w4_ui.s_33"), delimiterLabel(v.Session.Delimiter)))
	}
	if v.Session.BlockCount > 1 {
		parts = append(parts, fmt.Sprintf(i18n.TDefault("w4_ui.d_186"), v.Session.BlockCount))
	}
	parts = append(parts, fmt.Sprintf(i18n.TDefault("w4_ui.d_187"), v.Session.TotalRows))
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
	return fmt.Sprintf(i18n.TDefault("w4_ui.s_d_43"), v.Session.AINote, v.Session.AICalls)
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
		return i18n.TDefault("w4_ui.s_188_188")
	case catalog.SessionProcessing:
		return i18n.TDefault("w4_ui.s_189_189")
	case catalog.SessionReady:
		return i18n.TDefault("w4_ui.s_190_190")
	case catalog.SessionCommitted:
		return i18n.TDefault("w4_ui.s_191_191")
	case catalog.SessionCancelled:
		return i18n.TDefault("w4_ui.s_192_192")
	case catalog.SessionFailed:
		return i18n.TDefault("w4_ui.s_193_193")
	default:
		return string(status)
	}
}
