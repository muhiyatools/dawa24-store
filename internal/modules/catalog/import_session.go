package catalog

import (
	"time"

	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// The staged import.
//
// An import used to be one request: upload, parse, write, hope. For a
// nine-thousand-row catalogue that is a decision the admin makes blind. The flow
// is now three steps — analyse the file, review what will happen, then commit —
// with the middle step held in catalog.import_sessions so the admin can read a
// full account of every row before anything touches the catalogue.

// ImportMode is the reconciliation strategy applied at commit.
//
// The names match ingest.ImportMode deliberately. A vendor uploading a price
// list and an admin uploading the master catalogue are choosing between the same
// four behaviours, and one vocabulary across both screens is worth more than the
// module independence a second set of names would buy.
type ImportMode string

const (
	// ModeAddNewOnly inserts products the catalogue does not have and leaves
	// every existing row untouched.
	ModeAddNewOnly ImportMode = "add_new_only"
	// ModeUpdateExistingOnly refreshes matched products and inserts nothing.
	ModeUpdateExistingOnly ImportMode = "update_existing_only"
	// ModeClearAndAdd retires the whole catalogue, then inserts the file.
	ModeClearAndAdd ImportMode = "clear_and_add"
	// ModeUpdateAndAdd is the default: update what matches, insert what does not.
	ModeUpdateAndAdd ImportMode = "update_and_add"
)

// ImportModeOption describes one mode for the chooser on the upload screen.
type ImportModeOption struct {
	Mode        ImportMode
	Icon        string
	Title       string
	Description string
	// Destructive marks a mode that removes existing catalogue rows, so the UI
	// can demand a deliberate second confirmation for it.
	Destructive bool
}

// ImportModeOptions are the four strategies, in the order they are offered.
var ImportModeOptions = []ImportModeOption{
	{
		Mode:        ModeUpdateAndAdd,
		Icon:        "⚡",
		Title:       "تحديث المنتجات الحالية وإضافة الجديدة",
		Description: "الخيار الافتراضي؛ يحدّث أسعار وبيانات الأصناف المتطابقة، ويضيف أي صنف جديد غير مسجل.",
	},
	{
		Mode:        ModeAddNewOnly,
		Icon:        "➕",
		Title:       "إضافة المنتجات الجديدة فقط",
		Description: "يضيف الأصناف الجديدة فقط، ولا يعدّل أي منتج موجود مسبقاً في الكتالوج.",
	},
	{
		Mode:        ModeUpdateExistingOnly,
		Icon:        "🔄",
		Title:       "تحديث المنتجات الموجودة فقط",
		Description: "يحدّث تفاصيل وأسعار الأصناف الموجودة بالفعل، ولا يضيف أي صنف جديد.",
	},
	{
		Mode:        ModeClearAndAdd,
		Icon:        "🗑️",
		Title:       "أرشفة الكتالوج الحالي ثم الإضافة",
		Description: "ينقل جميع الأصناف الحالية إلى الأرشيف (حذف قابل للاسترجاع)، ثم يستورد الملف كأصناف جديدة.",
		Destructive: true,
	},
}

// ParseMode maps a submitted value onto a mode, defaulting to the safe one.
func ParseMode(raw string) ImportMode {
	switch ImportMode(raw) {
	case ModeAddNewOnly:
		return ModeAddNewOnly
	case ModeUpdateExistingOnly:
		return ModeUpdateExistingOnly
	case ModeClearAndAdd:
		return ModeClearAndAdd
	default:
		return ModeUpdateAndAdd
	}
}

// Label renders a mode in the admin's language.
func (m ImportMode) Label() string {
	for _, o := range ImportModeOptions {
		if o.Mode == m {
			return o.Title
		}
	}
	return string(m)
}

// IsDestructive reports whether the mode removes existing catalogue rows.
func (m ImportMode) IsDestructive() bool { return m == ModeClearAndAdd }

// ImportOptions are the enrichment switches the admin sets before processing.
//
// Each one is off-by-default where it writes something the file did not say.
// An importer that invents a category for nine thousand products because a
// checkbox defaulted to on is worse than one that leaves the column empty.
type ImportOptions struct {
	// AutoCreateBrands registers a manufacturer named in the file as a brand
	// when the catalogue has no matching one.
	AutoCreateBrands bool `json:"auto_create_brands"`
	// AssignCategory fills catalog.products.category_id.
	AssignCategory bool `json:"assign_category"`
	// AssignDosageForm fills the pharmaceutical form.
	AssignDosageForm bool `json:"assign_dosage_form"`
	// AssignScientificName fills the generic name.
	AssignScientificName bool `json:"assign_scientific_name"`
	// UseAI routes the fields above through the Gateway for rows the
	// deterministic rules could not resolve.
	UseAI bool `json:"use_ai"`
	// DefaultCategoryID is applied to every product that ends without one,
	// including when AI is off. Zero leaves the column null.
	DefaultCategoryID int64 `json:"default_category_id,omitempty"`
}

// DefaultImportOptions are what the upload screen starts on: infer the form
// from the product name, which is deterministic and safe, and nothing else.
func DefaultImportOptions() ImportOptions {
	return ImportOptions{AssignDosageForm: true}
}

// WantsEnrichment reports whether any field-filling switch is on.
func (o ImportOptions) WantsEnrichment() bool {
	return o.AssignCategory || o.AssignDosageForm || o.AssignScientificName
}

// SessionStatus tracks where a staged import has reached.
type SessionStatus string

const (
	// SessionDraft is analysed and awaiting the admin's review.
	SessionDraft SessionStatus = "draft"
	// SessionReady has been processed and is awaiting confirmation.
	SessionReady SessionStatus = "ready"
	// SessionCommitted has been written to the catalogue.
	SessionCommitted SessionStatus = "committed"
	// SessionCancelled was discarded by the admin.
	SessionCancelled SessionStatus = "cancelled"
	// SessionFailed hit an error during commit and wrote nothing.
	SessionFailed SessionStatus = "failed"
)

// RowAction is what committing will do with one staged row.
type RowAction string

const (
	// ActionInsert creates a new catalogue product.
	ActionInsert RowAction = "insert"
	// ActionUpdate refreshes a matched product.
	ActionUpdate RowAction = "update"
	// ActionSkip leaves the catalogue untouched for this row, because the
	// chosen mode excludes it or the admin deselected it.
	ActionSkip RowAction = "skip"
)

// Label renders an action in the admin's language.
func (a RowAction) Label() string {
	switch a {
	case ActionInsert:
		return "إضافة"
	case ActionUpdate:
		return "تحديث"
	default:
		return "تخطي"
	}
}

// ImportSession is one staged import awaiting review or already committed.
type ImportSession struct {
	ID             int64           `json:"id"`
	PublicID       string          `json:"public_id"`
	OrganizationID int64           `json:"organization_id"`
	CreatedBy      *int64          `json:"created_by,omitempty"`
	Filename       string          `json:"filename"`
	FileSizeBytes  int64           `json:"file_size_bytes"`
	SourceFormat   string          `json:"source_format"`
	SheetName      string          `json:"sheet_name"`
	Delimiter      string          `json:"delimiter"`
	Status         SessionStatus   `json:"status"`
	Mode           ImportMode      `json:"import_mode"`
	Options        ImportOptions   `json:"options"`
	Overrides      LayoutOverrides `json:"layout_overrides"`

	TotalRows   int `json:"total_rows"`
	ParsedRows  int `json:"parsed_rows"`
	InsertRows  int `json:"insert_rows"`
	UpdateRows  int `json:"update_rows"`
	SkipRows    int `json:"skip_rows"`
	ErrorRows   int `json:"error_rows"`
	WarningRows int `json:"warning_rows"`
	BlockCount  int `json:"block_count"`

	// NewBrands and NewCategories are the taxonomy rows this import would
	// create. They are proposals until the admin confirms; nothing is written
	// during analysis.
	NewBrands []string `json:"new_brands,omitempty"`

	AICalls    int    `json:"ai_calls"`
	AIApplied  int    `json:"ai_applied"`
	AINote     string `json:"ai_note,omitempty"`
	AIFallback bool   `json:"ai_fallback"`

	ErrorMessage string     `json:"error_message,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	CommittedAt  *time.Time `json:"committed_at,omitempty"`
	ExpiresAt    time.Time  `json:"expires_at"`
}

// IsReviewable reports whether the admin can still act on this session.
func (s *ImportSession) IsReviewable() bool {
	return s.Status == SessionDraft || s.Status == SessionReady
}

// Affected is how many catalogue rows committing would touch.
func (s *ImportSession) Affected() int { return s.InsertRows + s.UpdateRows }

// AIChange records one field an AI enrichment pass filled, so the preview can
// show the admin exactly what the model decided and why.
type AIChange struct {
	Field  string `json:"field"`
	Label  string `json:"label"`
	Value  string `json:"value"`
	Reason string `json:"reason,omitempty"`
}

// StagingRow is one parsed product held for review.
type StagingRow struct {
	ID        int64     `json:"id"`
	SessionID int64     `json:"session_id"`
	SourceRow int       `json:"source_row"`
	Block     int       `json:"block"`
	Action    RowAction `json:"action"`
	// Included is the admin's per-row switch. A deselected row is skipped at
	// commit whatever its action says.
	Included         bool        `json:"included"`
	MatchedProductID *int64      `json:"matched_product_id,omitempty"`
	MatchReason      MatchReason `json:"match_reason,omitempty"`
	Product          *Product    `json:"product"`
	Issues           []RowIssue  `json:"issues,omitempty"`
	AIChanges        []AIChange  `json:"ai_changes,omitempty"`
}

// DisplayName is the product name shown in the review table.
func (r *StagingRow) DisplayName() string {
	if r.Product == nil {
		return ""
	}
	if name := r.Product.Name.Get(i18n.AR); name != "" {
		return name
	}
	return r.Product.Name.Get(i18n.EN)
}

// HasErrors reports whether this row carries a blocking finding.
func (r *StagingRow) HasErrors() bool {
	for _, issue := range r.Issues {
		if issue.Severity == SeverityError {
			return true
		}
	}
	return false
}

// StagingFilter narrows the review table.
type StagingFilter struct {
	// Action limits to one action; empty means all.
	Action RowAction
	// OnlyIssues limits to rows carrying a warning or an error.
	OnlyIssues bool
	// OnlyAI limits to rows an AI pass changed.
	OnlyAI bool
	// Search matches the product name, SKU, or barcode.
	Search string
	Limit  int
	Offset int
}

// StagingCounts is the tally the confirmation screen promises.
type StagingCounts struct {
	Insert    int `json:"insert"`
	Update    int `json:"update"`
	Skip      int `json:"skip"`
	Errors    int `json:"errors"`
	Warnings  int `json:"warnings"`
	AIChanged int `json:"ai_changed"`
	Total     int `json:"total"`
}

// Affected is how many catalogue rows the commit would touch.
func (c StagingCounts) Affected() int { return c.Insert + c.Update }
