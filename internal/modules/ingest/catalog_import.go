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
	case PhaseConfirm:
		return "بانتظار التأكيد"
	case PhaseProcessing:
		return "جارٍ التنفيذ"
	case PhaseCompleted:
		return "مكتمل"
	case PhaseFailed:
		return "فشل"
	default:
		return "ملغي"
	}
}

// Open reports whether the vendor can still act on the import.
func (p Phase) Open() bool {
	return p == PhaseMapping || p == PhaseSettings || p == PhaseConfirm
}

// Terminal reports whether the import has finished, one way or another.
func (p Phase) Terminal() bool {
	return p == PhaseCompleted || p == PhaseFailed || p == PhaseCancelled
}

// Mode is the reconciliation strategy applied to the vendor's own catalogue.
type Mode string

const (
	// ModeUpsert updates the variants the file matches and adds the rest. It is
	// the default because it is what uploading an updated price list means.
	ModeUpsert Mode = "update_and_add"
	// ModeAddOnly adds what is new and leaves every existing variant untouched,
	// for a vendor extending their range without republishing their prices.
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

// UnmatchedPolicy decides what happens to a row the shared catalogue does not
// recognise.
type UnmatchedPolicy string

const (
	// UnmatchedCreate registers the product in the shared catalogue as pending
	// review and links the vendor's variant to it. This is the default: a
	// supplier's file is not wrong because the catalogue has not caught up, and
	// refusing those rows means refusing most of a new supplier's first upload.
	UnmatchedCreate UnmatchedPolicy = "create"
	// UnmatchedSkip leaves the row out entirely and lists it in the results for
	// the vendor to resolve by hand.
	UnmatchedSkip UnmatchedPolicy = "skip"
)

// Label renders the policy in Arabic.
func (p UnmatchedPolicy) Label() string {
	if p == UnmatchedSkip {
		return "تخطي الأصناف غير المطابقة وعرضها في التقرير"
	}
	return "إضافة الأصناف غير المطابقة إلى الكتالوج المركزي بانتظار الاعتماد"
}

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
	Unmatched UnmatchedPolicy     `json:"unmatched"`

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
	TrustSupplierCode bool `json:"trust_supplier_code"`

	BlankQuantityIsZero bool `json:"blank_quantity_is_zero"`
	InferDosageForm     bool `json:"infer_dosage_form"`
	InferConcentration  bool `json:"infer_concentration"`
	RejectExpired       bool `json:"reject_expired"`

	DefaultMinOrderQty  int  `json:"default_min_order_qty"`
	DefaultMinThreshold int  `json:"default_min_threshold"`
	MarkNegotiable      bool `json:"mark_negotiable"`
	// PublishImmediately puts imported variants on sale at once. Off means they
	// are created inactive for the vendor to review in their own catalogue.
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
		Unmatched:           UnmatchedCreate,
		Duplicates:          productmatch.DuplicateLastWins,
		MinMatchScore:       0.78,
		UseAI:               true,
		BlankQuantityIsZero: false,
		InferDosageForm:     true,
		InferConcentration:  true,
		RejectExpired:       true,
		DefaultMinOrderQty:  1,
		DefaultMinThreshold: 0,
		PublishImmediately:  true,
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
	if s.Unmatched == "" {
		s.Unmatched = UnmatchedCreate
	}
	if s.Duplicates == "" {
		s.Duplicates = productmatch.DuplicateLastWins
	}
	// Below about half, similarity stops meaning anything: two pharmaceutical
	// names share that much by sharing a manufacturer's house style.
	if s.MinMatchScore < 0.5 || s.MinMatchScore > 1 {
		s.MinMatchScore = 0.78
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

	TotalRows       int `json:"total_rows"`
	InsertedRows    int `json:"inserted_rows"`
	UpdatedRows     int `json:"updated_rows"`
	SkippedRows     int `json:"skipped_rows"`
	ErrorRows       int `json:"error_rows"`
	MatchedRows     int `json:"matched_rows"`
	ReviewRows      int `json:"review_rows"`
	UnmatchedRows   int `json:"unmatched_rows"`
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

// MappingSnapshot is the confirmed column reading, flattened for storage.
type MappingSnapshot struct {
	HeaderRow    int                     `json:"header_row"`
	FirstDataRow int                     `json:"first_data_row"`
	Headers      []string                `json:"headers"`
	Columns      []MappedColumn          `json:"columns"`
	Conflicts    []productmatch.Conflict `json:"conflicts,omitempty"`
	Notes        []productmatch.Note     `json:"notes,omitempty"`
}

// MappedColumn is one column as the vendor confirmed it.
type MappedColumn struct {
	Index      int                     `json:"index"`
	Header     string                  `json:"header"`
	Field      productmatch.Field      `json:"field,omitempty"`
	Label      string                  `json:"label,omitempty"`
	Confidence productmatch.Confidence `json:"confidence,omitempty"`
	Source     productmatch.Source     `json:"source,omitempty"`
	Score      float64                 `json:"score"`
	Why        []string                `json:"why,omitempty"`
	Ignored    bool                    `json:"ignored"`
}

// SnapshotMapping flattens a resolved mapping for storage.
func SnapshotMapping(layout productmatch.Layout, m *productmatch.Mapping) *MappingSnapshot {
	if m == nil {
		return nil
	}
	snap := &MappingSnapshot{
		HeaderRow:    layout.HeaderRow,
		FirstDataRow: layout.FirstDataRow,
		Headers:      layout.Headers,
		Conflicts:    m.Conflicts,
		Notes:        m.Notes,
	}
	for _, c := range m.Columns {
		col := MappedColumn{
			Index:      c.Index,
			Header:     c.Header,
			Field:      c.Field,
			Confidence: c.Confidence,
			Source:     c.Source,
			Score:      c.Score,
			Why:        c.Why,
			Ignored:    c.Ignored,
		}
		if c.Field != "" {
			col.Label = c.Field.Label()
		}
		snap.Columns = append(snap.Columns, col)
	}
	return snap
}

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
	Payload            *productmatch.Row             `json:"payload,omitempty"`
	Candidates         []productmatch.MatchCandidate `json:"candidates,omitempty"`
	Issues             []productmatch.Issue          `json:"issues,omitempty"`
	Message            string                        `json:"message"`
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
	Limit      int
	Offset     int
}
