package catalog

import (
	"sort"

	"github.com/muhiya/dawa24-store/internal/shared/productmatch"
	"github.com/muhiya/dawa24-store/internal/shared/sheet"
)

// Column identification for spreadsheet import.
//
// The previous mapper walked the header left to right and gave a column to the
// first field whose keyword it contained, first match winning. That is why a
// column headed "الباركود الدولي" was bound to sku: the sku rule tested for
// "كود", which is a substring of "باركود", and it ran before the barcode rule.
// The real barcode column was then unavailable and the real item-code column,
// further right, was never looked at.
//
// The fix is to stop deciding per column and start deciding globally. Every
// (field, column) pair is scored, and the assignment is made best-score-first
// with each column claimed once, so a strong signal anywhere in the header
// outranks a weak coincidence earlier in it.

// Import field names. These are the keys of the binding map that row parsing
// reads, kept as constants so a typo cannot silently produce an unmapped field.
const (
	FieldNameAR        = "name_ar"
	FieldNameEN        = "name_en"
	FieldSKU           = "sku"
	FieldBarcode       = "barcode"
	FieldPrice         = "price"
	FieldPublicPrice   = "public_price"
	FieldCostPrice     = "cost_price"
	FieldDiscount      = "discount"
	FieldGenericName   = "generic_name"
	FieldActive        = "active_ingredient"
	FieldDosageForm    = "dosage_form"
	FieldConcentration = "concentration"
	FieldUnit          = "unit"
	FieldManufacturer  = "manufacturer"
	FieldDescriptionAR = "description_ar"
	FieldDescriptionEN = "description_en"
	FieldQuantity      = "quantity"
	FieldStatus        = "status"
	FieldCategory      = "category"
)

// FieldLabels are the Arabic names shown in the import report, so an admin
// reading "لم يتم العثور على عمود: السعر" knows which column to add.
var FieldLabels = map[string]string{
	FieldNameAR:        "اسم الصنف بالعربي",
	FieldNameEN:        "اسم الصنف بالإنجليزي",
	FieldSKU:           "كود الصنف",
	FieldBarcode:       "الباركود",
	FieldPrice:         "السعر",
	FieldPublicPrice:   "سعر الجمهور",
	FieldCostPrice:     "سعر التكلفة",
	FieldDiscount:      "الخصم",
	FieldGenericName:   "الاسم العلمي",
	FieldActive:        "المادة الفعالة",
	FieldDosageForm:    "الشكل الصيدلي",
	FieldConcentration: "التركيز",
	FieldUnit:          "الوحدة",
	FieldManufacturer:  "الشركة المصنعة",
	FieldDescriptionAR: "الوصف بالعربي",
	FieldDescriptionEN: "الوصف بالإنجليزي",
	FieldQuantity:      "الكمية",
	FieldStatus:        "الحالة",
	FieldCategory:      "فئة المنتج",
}

// Match strengths. The gaps are wide on purpose: an exact header match must beat
// any number of partial ones, and a "strong" phrase (a full multi-word term the
// field owns) must beat a "weak" one (a single word several fields share).
// scoreCertain is the reported score at or above which a binding is treated as
// settled rather than as a guess worth a second look.
//
// It tracks productmatch.ConfidenceCertain, which is where the shared resolver
// stops hedging. The previous constant was 100 and meant "the header is exactly
// this field's name"; a resolver that also weighs the values reaches certainty
// by more routes than that, and demanding a perfect header again would have
// quietly reclassified every value-corroborated binding as uncertain.
const scoreCertain = 82

// scoreLikely is where the shared resolver stops guessing, on the same scale.
const scoreLikely = 62

// catalogFields translates between this module's field names and the shared
// engine's.
//
// The two vocabularies exist for different reasons and both are right.
// productmatch names what a *spreadsheet cell* can be saying; this module names
// what a *column of catalog.products* is called. They are one-to-one for
// everything an administrator can import, so the translation is a table rather
// than a decision.
var catalogFields = map[productmatch.Field]string{
	productmatch.FieldName:             FieldNameAR,
	productmatch.FieldNameEN:           FieldNameEN,
	productmatch.FieldSKU:              FieldSKU,
	productmatch.FieldBarcode:          FieldBarcode,
	productmatch.FieldPrice:            FieldPrice,
	productmatch.FieldPublicPrice:      FieldPublicPrice,
	productmatch.FieldCostPrice:        FieldCostPrice,
	productmatch.FieldDiscountPct:      FieldDiscount,
	productmatch.FieldScientific:       FieldGenericName,
	productmatch.FieldActiveIngredient: FieldActive,
	productmatch.FieldDosageForm:       FieldDosageForm,
	productmatch.FieldConcentration:    FieldConcentration,
	productmatch.FieldUnit:             FieldUnit,
	productmatch.FieldManufacturer:     FieldManufacturer,
	productmatch.FieldDescription:      FieldDescriptionAR,
	productmatch.FieldDescriptionEN:    FieldDescriptionEN,
	productmatch.FieldQuantity:         FieldQuantity,
	productmatch.FieldStatus:           FieldStatus,
	productmatch.FieldCategory:         FieldCategory,
}

// ColumnBinding records one resolved field-to-column decision, so the import
// report can show the admin exactly how their file was read.
type ColumnBinding struct {
	Field  string `json:"field"`
	Label  string `json:"label"`
	Header string `json:"header"`
	Index  int    `json:"index"`
	Score  int    `json:"score"`
}

// ColumnPlan is the outcome of reading a header row.
type ColumnPlan struct {
	// Columns maps a field constant to its zero-based column index.
	Columns map[string]int
	// Bindings lists the same decisions with their evidence, ordered by column.
	Bindings []ColumnBinding
	// Unmapped holds header labels that matched no field. They are shown to the
	// admin rather than dropped silently: an unmapped "سعر الجمهور" is the
	// difference between a correct catalogue and a wrong one.
	Unmapped []string
	// Positional is true when no usable header was found and the plan falls back
	// to column order. Callers warn loudly in that case.
	Positional bool
}

// Has reports whether a field was bound to a column.
func (p ColumnPlan) Has(field string) bool {
	_, ok := p.Columns[field]
	return ok
}

// PlanColumns resolves a header row into a field-to-column plan.
//
// The scoring, the synonym table and the global assignment all used to live in
// this file — a second implementation of productmatch's resolver, carrying the
// same explanatory comments about the same real files. Two copies of "which
// column is the price" drift, and the drift is invisible until two features
// disagree about one spreadsheet. This is now the shared resolver, narrowed to
// the fields an administrator can write into catalog.products.
//
// sample is the data rows beneath the header, and passing them is what buys the
// second witness this module never had: the previous mapper read headers only,
// so a file whose column titles were wrong, missing or generic could not be
// read at all, and a column of GS1 check digits under a header nobody wrote was
// simply unmapped. Callers that genuinely have no rows may pass nil.
func PlanColumns(headerRow []string, sample [][]string) ColumnPlan {
	plan := ColumnPlan{Columns: map[string]int{}}
	if len(headerRow) == 0 {
		return plan
	}

	mapping := productmatch.ResolveWith(
		headerRow, profileColumns(headerRow, sample), nil, productmatch.CatalogFields)

	for _, c := range mapping.Columns {
		field, known := catalogFields[c.Field]
		if !known {
			if CleanCellString(c.Header) != "" {
				plan.Unmapped = append(plan.Unmapped, CleanCellString(c.Header))
			}
			continue
		}
		plan.Columns[field] = c.Index
		plan.Bindings = append(plan.Bindings, ColumnBinding{
			Field:  field,
			Label:  FieldLabels[field],
			Header: CleanCellString(c.Header),
			Index:  c.Index,
			// The shared resolver scores 0..1; this module has always reported
			// 0..100 and the review screen renders it that way. The bands below
			// are the resolver's own confidence thresholds rescaled, so
			// "مؤكد" means here exactly what it means on the vendor's screen.
			Score: int(c.Score * 100),
		})
	}

	sort.Slice(plan.Bindings, func(i, j int) bool { return plan.Bindings[i].Index < plan.Bindings[j].Index })
	return plan
}

// profileColumns measures the sample rows so the resolver has value evidence.
//
// A nil or empty sample yields sealed empty profiles rather than nil ones, so
// the resolver runs on header evidence alone instead of panicking on a missing
// column.
func profileColumns(headerRow []string, sample [][]string) []*sheet.ColumnProfile {
	profiles := make([]*sheet.ColumnProfile, len(headerRow))
	for i, h := range headerRow {
		profiles[i] = sheet.NewColumnProfile(i, h)
	}
	for _, row := range sample {
		for i := range profiles {
			if i < len(row) {
				profiles[i].Observe(row[i])
			} else {
				profiles[i].Observe("")
			}
		}
	}
	for _, p := range profiles {
		p.Seal()
	}
	return profiles
}

// MapHeaderColumns resolves a header row to a field-to-index map.
//
// Retained as the narrow entry point used by callers that only need the
// mapping; PlanColumns carries the evidence the import report renders.
func MapHeaderColumns(headerRow []string, sample [][]string) map[string]int {
	return PlanColumns(headerRow, sample).Columns
}
