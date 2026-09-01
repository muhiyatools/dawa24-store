package pages

import "github.com/muhiya/dawa24-store/internal/shared/i18n"

// One wizard, four faces.
//
// Every import in this platform is the same journey — choose a file, confirm how
// it will be read, choose how it will be applied, review what it produced, and
// see what happened — and each of the four had invented its own vocabulary for
// it. The smart order had five steps under `.so-*` classes, the administrator's
// import four under `.wiz-*`, the vendor's five read off a session phase, and
// the saving list four as a string enum. Same journey, four mental models, and a
// user who had learned one had learned one.
//
// The steps below are canonical and their numbers are fixed. A system that does
// not need a step renders it greyed rather than renumbering around it, because
// the number is what the user remembers: "the settings are on step 3" has to be
// true everywhere, including where step 3 is not offered.

// Step is one stage of an import, numbered identically in every wizard.
type Step int

const (
	// StepFile is the upload: drop zone, recent sessions, sample download.
	StepFile Step = 1
	// StepColumns is how the file will be read — the mapping grid, the
	// per-field confidence, the preview, and any structural findings.
	StepColumns Step = 2
	// StepSettings is how it will be applied: the mode and the toggles.
	StepSettings Step = 3
	// StepReview is what it produced, row by row, before anything is written.
	StepReview Step = 4
	// StepResults is what happened.
	StepResults Step = 5
	// StepOrder is the smart order's own tail — choosing suppliers and
	// confirming the basket. No other wizard has it.
	StepOrder Step = 6
)

// stepTitles are the canonical labels. They are the user's names for these
// stages, not the code's: a vendor and a pharmacy are doing the same thing on
// step 2 and should read the same word for it.
var stepTitles = map[Step]struct{ icon, title string }{
	StepFile:     {"📤", i18n.T("ar", "wizard.step.file")},
	StepColumns:  {"🔗", i18n.T("ar", "wizard.step.columns")},
	StepSettings: {"⚙️", i18n.T("ar", "wizard.step.settings")},
	StepReview:   {"📋", i18n.T("ar", "wizard.step.review")},
	StepResults:  {"📊", i18n.TDefault("w4_ui.s_196_196")},
	StepOrder:    {"🛒", i18n.TDefault("w4_ui.s_197_197")},
}

// WizardStep is one node on the rail.
type WizardStep struct {
	Step   Step
	Number int
	Icon   string
	Title  string
	Active bool
	Done   bool
	// Skipped marks a step this wizard does not offer. It is rendered, greyed,
	// so the numbering stays true across all four.
	Skipped bool
}

// WizardRailFor builds the rail for one wizard.
//
// used is the steps this system actually offers; anything in the canonical set
// and absent from it renders as skipped. current is where the user is now.
func WizardRailFor(current Step, used ...Step) []WizardStep {
	offers := make(map[Step]bool, len(used))
	highest := StepFile
	for _, s := range used {
		offers[s] = true
		if s > highest {
			highest = s
		}
	}

	out := make([]WizardStep, 0, int(highest))
	for s := StepFile; s <= highest; s++ {
		meta := stepTitles[s]
		out = append(out, WizardStep{
			Step:    s,
			Number:  int(s),
			Icon:    meta.icon,
			Title:   meta.title,
			Active:  s == current,
			Done:    s < current && offers[s],
			Skipped: !offers[s],
		})
	}
	return out
}

// ImportRail is the administrator's catalogue import: no settings step of its
// own, because its strategy and switches share the column screen.
func ImportRail(current Step) []WizardStep {
	return WizardRailFor(current, StepFile, StepColumns, StepReview, StepResults)
}

// VendorRail is the supplier's catalogue import, the widest of the four.
func VendorRail(current Step) []WizardStep {
	return WizardRailFor(current, StepFile, StepColumns, StepSettings, StepReview, StepResults)
}

// SavingRail is a private reference list: no settings, because there is nothing
// to decide beyond the mapping.
func SavingRail(current Step) []WizardStep {
	return WizardRailFor(current, StepFile, StepColumns, StepReview, StepResults)
}

// OrderRail is the smart order, which continues past the results into choosing
// suppliers and confirming a basket.
func OrderRail(current Step) []WizardStep {
	return WizardRailFor(current,
		StepFile, StepColumns, StepSettings, StepReview, StepResults, StepOrder)
}

// ToggleGroup buckets the switches on a settings screen.
//
// The four groups, in this order, everywhere. A vendor who has learned that the
// AI card is the last thing on the screen should find it there in every wizard
// that has one, and a group a system does not use is omitted rather than
// reordered around.
type ToggleGroup string

const (
	// GroupMode is what the import does to what is already there.
	GroupMode ToggleGroup = "mode"
	// GroupMatching is how rows are tied to the shared catalogue.
	GroupMatching ToggleGroup = "matching"
	// GroupEnrichment is what gets filled in that the file did not say.
	GroupEnrichment ToggleGroup = "enrichment"
	// GroupAI is always last, always one card, always with its availability
	// reason when the Gateway is down.
	GroupAI ToggleGroup = "ai"
)

// ToggleGroupOrder is the order the settings screen renders the groups in.
var ToggleGroupOrder = []ToggleGroup{GroupMode, GroupMatching, GroupEnrichment, GroupAI}

// Label renders a group heading.
func (g ToggleGroup) Label() string {
	switch g {
	case GroupMode:
		return i18n.TDefault("w4_ui.s_198_198")
	case GroupMatching:
		return i18n.TDefault("w4_ui.s_199_199")
	case GroupEnrichment:
		return i18n.TDefault("w4_ui.s_200_200")
	default:
		return i18n.TDefault("w4_ui.s_201_201")
	}
}

// WizardToggle is one switch, in the shape every settings screen renders.
type WizardToggle struct {
	Group       ToggleGroup
	Name        string
	Icon        string
	Title       string
	Description string
	Checked     bool
	// Disabled marks a switch that cannot be used right now, with Note saying
	// why. Rendering a toggle that ticks and then does nothing is worse than
	// rendering one that explains itself.
	Disabled bool
	Note     string
}

// TogglesInGroup filters a toggle list, preserving the caller's order within
// the group.
func TogglesInGroup(all []WizardToggle, g ToggleGroup) []WizardToggle {
	out := make([]WizardToggle, 0, len(all))
	for _, t := range all {
		if t.Group == g {
			out = append(out, t)
		}
	}
	return out
}

// AIToggle builds the one AI card, worded the same in every wizard.
//
// The copy is the vendor import's, which is the only one of the four that told
// the user what the feature costs and what it will not do. It is the shared copy
// now, because the promise has to be the same wherever it is made.
func AIToggle(name string, checked, available bool, unavailableReason string) WizardToggle {
	t := WizardToggle{
		Group:       GroupAI,
		Name:        name,
		Icon:        "🤖",
		Title:       "تفعيل الذكاء الاصطناعي",
		Description: "يُستخدم الذكاء الاصطناعي فقط لمساعدة الاستيراد في الحالات التي لم يتم حسمها بالطرق الأساسية. اختياري، والاستيراد يعمل بالكامل بدونه.",
		Checked:     checked && available,
		Note:        "",
	}
	if !available {
		t.Disabled = true
		t.Checked = false
		t.Note = unavailableReason
		if t.Note == "" {
			t.Note = i18n.TDefault("w4_ui.w4str_50_50")
		}
	}
	return t
}
