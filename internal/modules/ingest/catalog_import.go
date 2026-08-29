package ingest

import (
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/inventory"
	"github.com/muhiya/dawa24-store/internal/shared/productmatch"
	"github.com/muhiya/dawa24-store/internal/shared/sheet"
)

// The vendor catalogue import.
//
// Seven stages, and the vendor is in charge of the middle three:
//
//	تحليل الملف → ربط الأعمدة → التحقق → إعدادات الاستيراد → التأكيد → المعالجة → النتائج
//
// Nothing is written to the catalogue before the confirmation. That is the
// whole design: a supplier price list changes what nine thousand pharmacies
// pay, and an importer that decides on their behalf and reports afterwards is
// asking them to approve something they cannot see.

// Phase is where an import has reached. It is the vendor's own progress through
// the wizard, not a job status: 'mapping' is waiting for them, 'processing' is
// waiting for us.
type Phase string

const (
	// PhaseMapping is analysed and awaiting the vendor's column review.
	PhaseMapping Phase = "mapping"
	// PhaseSettings has a confirmed mapping and is awaiting the import rules.
	PhaseSettings Phase = "settings"
	// PhaseReview stages parsed rows for the vendor to review, edit, and manually match.
	PhaseReview Phase = "review"
	// PhaseConfirm has everything and is showing the vendor what will happen.
	PhaseConfirm Phase = "confirm"
	// PhaseProcessing is running.
	PhaseProcessing Phase = "processing"
	// PhaseCompleted finished; the results screen has the account.
	PhaseCompleted Phase = "completed"
	// PhaseFailed stopped on an error and wrote nothing further.
	PhaseFailed Phase = "failed"
	// PhaseCancelled was discarded by the vendor.
	PhaseCancelled Phase = "cancelled"
)

// Label renders a phase in Arabic.
func (p Phase) Label() string {
	switch p {
	case PhaseMapping:
		return "مراجعة ربط الأعمدة"
	case PhaseSettings:
		return "إعدادات الاستيراد"
	case PhaseReview:
		return "مراجعة الأصناف والمطابقة"
	case PhaseConfirm:
		return "بانتظار التأكيد"
	case PhaseProcessing:
		return "جارٍ التنفيذ"
	case PhaseCompleted:
		return "مكتمل"
	case PhaseFailed:
		return "فشل"
	default:
		return string(p)
	}
}

// Open reports whether the vendor can still act on the import.
func (p Phase) Open() bool {
	return p == PhaseMapping || p == PhaseSettings || p == PhaseReview || p == PhaseConfirm
}

// Terminal reports whether the import has finished, one way or another.
func (p Phase) Terminal() bool {
	return p == PhaseCompleted || p == PhaseFailed || p == PhaseCancelled
}

// Mode is the reconciliation strategy applied to the vendor's own catalogue.
type Mode string

const (
	// ModeUpsert updates the variants the file matches and adds the rest.
	ModeUpsert Mode = "update_and_add"
	// ModeAddOnly adds what is new and leaves existing variants untouched.
	ModeAddOnly Mode = "add_new_only"
	// ModeUpdateOnly refreshes what already exists and adds nothing, for a price
	// revision that must not widen the catalogue.
	ModeUpdateOnly Mode = "update_existing_only"
	// ModeReplace additionally deactivates every variant the file does not
	// mention. It is what a vendor means by "this file is my whole catalogue
	// now", and it is the only mode that can remove an offer from sale, so the
	// screen demands a second confirmation for it.
	ModeReplace Mode = "replace_catalog"
)

// ModeOption describes one strategy for the settings screen.
type ModeOption struct {
	Mode        Mode
	Icon        string
	Title       string
	Description string
	// Destructive marks a mode that can take products off sale.
	Destructive bool
}

// ModeOptions are the four strategies, in the order they are offered.
var ModeOptions = []ModeOption{
	{
		Mode:        ModeUpsert,
		Icon:        "⚡",
		Title:       "تحديث الأصناف الحالية وإضافة الجديدة",
		Description: "الخيار الافتراضي؛ يحدّث أسعار وأرصدة الأصناف المطابقة ويضيف أي صنف جديد في الملف.",
	},
	{
		Mode:        ModeAddOnly,
		Icon:        "➕",
		Title:       "إضافة الأصناف الجديدة فقط",
		Description: "يضيف ما ليس لديك ولا يغيّر سعر أو رصيد أي صنف موجود.",
	},
	{
		Mode:        ModeUpdateOnly,
		Icon:        "🔄",
		Title:       "تحديث الأصناف الموجودة فقط",
		Description: "يحدّث الأصناف المطابقة فقط، ولا يضيف أي صنف جديد إلى كتالوجك.",
	},
	{
		Mode:        ModeReplace,
		Icon:        "🗂️",
		Title:       "اعتبار الملف هو الكتالوج الكامل",
		Description: "يحدّث ويضيف، ثم يوقف عرض كل صنف لديك غير موجود في هذا الملف.",
		Destructive: true,
	},
}

// ParseMode maps a submitted value onto a mode, defaulting to the safe one.
func ParseMode(raw string) Mode {
	switch Mode(raw) {
	case ModeAddOnly:
		return ModeAddOnly
	case ModeUpdateOnly:
		return ModeUpdateOnly
	case ModeReplace:
		return ModeReplace
	default:
		return ModeUpsert
	}
}

// Label renders a mode in Arabic.
func (m Mode) Label() string {
	for _, o := range ModeOptions {
		if o.Mode == m {
			return o.Title
		}
	}
	return string(m)
}

// Destructive reports whether the mode can take products off sale.
func (m Mode) Destructive() bool { return m == ModeReplace }

// A vendor's file can no longer introduce a product to the shared catalogue.
//
// It used to, as `UnmatchedCreate`: a row the catalogue did not recognise was
// registered as a new master product and the vendor's variant was linked to it.
// The intent was that a new supplier's first upload should not be mostly
// refused. The result was that the shared catalogue became whatever any
// supplier happened to type — thousands of near-duplicate entries, each one a
// product no other vendor's row could ever match, which is the opposite of what
// a shared catalogue is for.
//
// The catalogue is now the administrator's alone. A vendor's import matches
// against it and prices what it finds; what it cannot match is reported back to
// the vendor, with the candidates the engine considered, so they can correct
// their file or ask for the product to be added. Nothing is invented.

// Settings are the rules the vendor sets before processing starts.
//
// Every default is the conservative reading. An importer whose defaults widen
// the blast radius — treating a blank cell as zero stock, accepting a weak
// match, overwriting balances a price list said nothing about — is one whose
// first run has to be undone by hand.
type Settings struct {
	WarehouseID int64  `json:"warehouse_id"`
	BranchID    *int64 `json:"branch_id,omitempty"`

	Mode      Mode                `json:"mode"`
	StockMode inventory.StockMode `json:"stock_mode"`

	// Duplicates decides what a repeated identity inside one file means.
	Duplicates productmatch.DuplicatePolicy `json:"duplicates"`

	// MinMatchScore is the similarity at or above which a match is applied
	// without asking. Below it the row is recorded for review and, depending on
	// the unmatched policy, either skipped or given a new catalogue product.
	MinMatchScore float64 `json:"min_match_score"`
	// UseAI lets a model settle the rows the deterministic engine could not.
	//
	// It is a tier, not a mode: everything the exact and similarity tiers
	// resolved is already decided before this runs, and the model only ever
	// chooses among candidates the engine retrieved. That is what keeps it
	// cheap — a nine-thousand-row file reaches it with tens of rows, not nine
	// thousand — and what makes switching it off change how much is matched
	// rather than whether the import works.
	UseAI bool `json:"use_ai"`
	// TrustSupplierCode lets the vendor's own item code match the shared
	// catalogue's. Off by default: a vendor's "951" is their internal numbering.
	//
	// It has no effect unless a كود صنف column was mapped in step one. The two
	// are set in different steps, so a vendor can switch this on and then go
	// back and unmap the column; the run resolves both together rather than
	// trusting the toggle alone.
	TrustSupplierCode bool
	// TrustBarcode lets the file's barcode settle a match on its own.
	//
	// Off by default, and it used not to be a choice: the barcode tier ran on
	// every import, ahead of the name and the dose, and any eight-digit value
	// with a single catalogue hit won outright. A vendor whose "barcode" column
	// holds their own warehouse numbering got confident links to unrelated
	// medicines with nothing in the review screen to mark them as doubtful.
	TrustBarcode bool
	// CodeIsCatalogCode says the mapped code column holds دواء 24's own codes
	// rather than the vendor's internal numbering. Only then is a code hit
	// accepted without the name agreeing too.
	CodeIsCatalogCode bool `json:"trust_supplier_code"`

	BlankQuantityIsZero bool `json:"blank_quantity_is_zero"`
	InferDosageForm     bool `json:"infer_dosage_form"`
	InferConcentration  bool `json:"infer_concentration"`
	RejectExpired       bool `json:"reject_expired"`

	DefaultQuantity     int  `json:"default_quantity,omitempty"`
	DefaultMinOrderQty  int  `json:"default_min_order_qty"`
	DefaultMinThreshold int  `json:"default_min_threshold"`
	MarkNegotiable      bool `json:"mark_negotiable"`
	// PublishImmediately puts imported variants on sale at once. Off means they
	// are created inactive for the vendor to review in their own catalogue.
	//
	// It governs the vendor's own variants and nothing else. It used to decide
	// the status of master products the import created too; imports no longer
	// create any.
	PublishImmediately bool `json:"publish_immediately"`
	// RecordRows keeps a per-row outcome ledger. On by default; a vendor
	// importing a hundred thousand rows may turn it off.
	RecordRows bool `json:"record_rows"`
}

// DefaultSettings are what the settings screen starts on.
func DefaultSettings() Settings {
	return Settings{
		Mode:                ModeUpsert,
		StockMode:           inventory.StockReplace,
		Duplicates:          productmatch.DuplicateLastWins,
		MinMatchScore:       0.30,
		UseAI:               true,
		BlankQuantityIsZero: false,
		InferDosageForm:     false,
		InferConcentration:  false,
		RejectExpired:       false,
		DefaultMinOrderQty:  1,
		DefaultMinThreshold: 0,
		PublishImmediately:  false,
		RecordRows:          true,
	}
}

// Normalize fills in anything a submitted form left blank or out of range.
func (s Settings) Normalize() Settings {
	if s.Mode == "" {
		s.Mode = ModeUpsert
	}
	if s.StockMode == "" {
		s.StockMode = inventory.StockReplace
	}
	if s.Duplicates == "" {
		s.Duplicates = productmatch.DuplicateLastWins
	}
	if s.MinMatchScore <= 0 {
		s.MinMatchScore = 0.30
	} else {
		s.MinMatchScore = min(max(s.MinMatchScore, 0.05), 1)
	}
	if s.DefaultQuantity < 0 {
		s.DefaultQuantity = 0
	}
	if s.DefaultMinOrderQty <= 0 {
		s.DefaultMinOrderQty = 1
	}
	if s.DefaultMinThreshold < 0 {
		s.DefaultMinThreshold = 0
	}
	return s
}

// Session is one vendor catalogue import.
type Session struct {
	ID             int64  `json:"id"`
	PublicID       string `json:"public_id"`
	OrganizationID int64  `json:"organization_id"`
	CreatedBy      *int64 `json:"created_by,omitempty"`

	Filename      string `json:"filename"`
	FileSizeBytes int64  `json:"file_size_bytes"`

	Phase    Phase        `json:"phase"`
	Source   sheet.Source `json:"source"`
	Settings Settings     `json:"settings"`
	// Overrides are the vendor's column corrections, keyed by zero-based column
	// index. They are the only part of the analysis that is persisted: the rest
	// is re-derived from the file, which is safe because the engine is
	// deterministic and means a stored mapping can never drift from the one the
	// processing run actually uses.
	Overrides map[int]productmatch.Field `json:"overrides,omitempty"`
	// Mapping is the confirmed reading, stored once the vendor accepts it, for
	// the results screen and the audit trail.
	Mapping *MappingSnapshot `json:"mapping,omitempty"`

	Stats    productmatch.Stats   `json:"stats"`
	Findings []productmatch.Issue `json:"findings,omitempty"`
	// AI is what the enhancement stage did, and it is the vendor's answer to
	// "was the smart matching worth switching on?". Zero on an import that
	// never ran it.
	AI AIStats `json:"ai"`

	TotalRows     int `json:"total_rows"`
	InsertedRows  int `json:"inserted_rows"`
	UpdatedRows   int `json:"updated_rows"`
	SkippedRows   int `json:"skipped_rows"`
	ErrorRows     int `json:"error_rows"`
	MatchedRows   int `json:"matched_rows"`
	ReviewRows    int `json:"review_rows"`
	UnmatchedRows int `json:"unmatched_rows"`
	// CreatedProducts is history only. Imports no longer add anything to the
	// shared catalogue, so it is zero for every run from now on; older runs keep
	// the count they actually made.
	CreatedProducts int `json:"created_products"`

	ProgressPercent int    `json:"progress_percent"`
	ProgressNote    string `json:"progress_note"`
	ErrorMessage    string `json:"error_message,omitempty"`

	StartedAt   *time.Time `json:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	ExpiresAt   time.Time  `json:"expires_at"`
}

// Affected is how many of the vendor's variants the run touched.
func (s *Session) Affected() int { return s.InsertedRows + s.UpdatedRows }

// RowOutcome is what the run did with one spreadsheet row.
type RowOutcome struct {
	ID                 int64                         `json:"id"`
	SourceRow          int                           `json:"source_row"`
	Outcome            string                        `json:"outcome"`
	MatchLevel         string                        `json:"match_level"`
	MatchScore         float64                       `json:"match_score"`
	ProductID          *int64                        `json:"product_id,omitempty"`
	MatchedProductName string                        `json:"matched_product_name,omitempty"`
	MatchedProductSKU  string                        `json:"matched_product_sku,omitempty"`
	VariantID          *int64                        `json:"variant_id,omitempty"`
	DisplayName        string                        `json:"display_name"`
	SourceCode         string                        `json:"source_code"`
	CustomVariantName  string                        `json:"custom_variant_name,omitempty"`
	IsExcluded         bool                          `json:"is_excluded"`
	IsManuallyMatched  bool                          `json:"is_manually_matched"`
	Payload            *productmatch.Row             `json:"payload,omitempty"`
	Candidates         []productmatch.MatchCandidate `json:"candidates,omitempty"`
	Issues             []productmatch.Issue          `json:"issues,omitempty"`
	Message            string                        `json:"message"`
}

// EffectiveVariantName returns the custom variant name if set, or original row name.
func (r *RowOutcome) EffectiveVariantName() string {
	if r.CustomVariantName != "" {
		return r.CustomVariantName
	}
	if r.Payload != nil && r.Payload.Name != "" {
		return r.Payload.Name
	}
	return r.DisplayName
}

// MatchedCatalogName returns the name of the matched master product, or top candidate.
func (r *RowOutcome) MatchedCatalogName() string {
	if r.MatchedProductName != "" {
		return r.MatchedProductName
	}
	if len(r.Candidates) > 0 && r.Candidates[0].Name != "" {
		return r.Candidates[0].Name
	}
	return ""
}

// MatchedCatalogSKU returns the SKU/barcode of the matched master product.
func (r *RowOutcome) MatchedCatalogSKU() string {
	if r.MatchedProductSKU != "" {
		return r.MatchedProductSKU
	}
	return ""
}

// Outcome values recorded against a row.
const (
	OutcomeStaged   = "staged"
	OutcomeInserted = "inserted"
	OutcomeUpdated  = "updated"
	OutcomeSkipped  = "skipped"
	OutcomeError    = "error"
)

// RowFilter narrows the results table.
type RowFilter struct {
	Outcome    string
	MatchLevel string
	Search     string
	SortBy     string
	SortOrder  string
	Limit      int
	Offset     int
}

// identifierChoices is what the vendor switched on, in the shared engine's
// vocabulary. The engine drops any choice whose column was never mapped.
func (s Settings) identifierChoices() productmatch.IdentifierChoices {
	return productmatch.IdentifierChoices{
		ByCode:            s.TrustSupplierCode,
		ByBarcode:         s.TrustBarcode,
		CodeIsCatalogCode: s.CodeIsCatalogCode,
	}
}
