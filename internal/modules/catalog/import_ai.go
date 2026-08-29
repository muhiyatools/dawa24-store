package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// AI's role in the import: three small translation jobs, nothing else.
//
// The previous design asked a model about every product, which on a
// fifty-thousand-row file is thousands of requests, minutes of waiting, and a
// per-row failure mode. None of that bought anything the importer could not do
// itself, because the questions repeat: a file has a handful of distinct
// category words and a handful of distinct pharmaceutical forms, and asking
// about each of the fifty thousand rows that use them asks the same question
// over and over.
//
// So AI answers each distinct question once:
//
//	1. Which spreadsheet column is which product field?
//	2. Which existing category does each distinct category word mean?
//	3. Which existing pharmaceutical form does each distinct form word mean?
//
// Three requests for any file of any size. Everything after that — every row,
// every lookup, every insert — is the ordinary importer, which is fast,
// predictable, and works unchanged when AI is switched off or unreachable.

// TaxonomyOption is one value the catalogue already has, with its id.
type TaxonomyOption struct {
	ID   int64  `json:"id,omitempty"`
	Name string `json:"name"`
}

// EnrichVocabulary is what the platform already knows: the closed sets a
// mapping request translates onto, and the manufacturer index the importer
// reuses so it never creates a brand twice.
type EnrichVocabulary struct {
	Categories  []TaxonomyOption
	Brands      []TaxonomyOption
	DosageForms []string
}

// AIMapper performs the three mapping calls. It is a port so the catalogue
// depends on the capability rather than on a transport.
type AIMapper interface {
	// MapColumns names the product field each spreadsheet column holds.
	MapColumns(ctx context.Context, req ColumnMapRequest) (ColumnMapResult, error)
	// MapValues translates distinct source values onto existing catalogue
	// values, for one taxonomy at a time.
	MapValues(ctx context.Context, req ValueMapRequest) (ValueMapResult, error)
	// Available reports whether the Gateway can be called at all.
	Available(ctx context.Context) bool
}

// ---------------------------------------------------------------------------
// Request 1: columns
// ---------------------------------------------------------------------------

// ColumnMapRequest is the header row plus a few data rows.
//
// A handful of rows is all a model needs and all it should be paid for: the
// header names the column and the sample shows what actually sits under it,
// which is what settles an ambiguous heading like "الكود" holding barcodes.
type ColumnMapRequest struct {
	Headers []string   `json:"headers"`
	Sample  [][]string `json:"sample_rows"`
	// Fields are the product fields available to map onto, sent so the model
	// chooses from a closed set instead of inventing names.
	Fields []FieldOption `json:"target_fields"`

	OrganizationID int64 `json:"-"`
	UserID         int64 `json:"-"`
}

// FieldOption is one product field the model may assign a column to.
type FieldOption struct {
	Field string `json:"field"`
	Label string `json:"label"`
}

// ColumnAssignment is one column the model recognised.
type ColumnAssignment struct {
	// Column is one-based, as an admin counts columns in Excel.
	Column     int     `json:"column"`
	Field      string  `json:"field"`
	Confidence float64 `json:"confidence,omitempty"`
}

// ColumnMapResult is the model's reading of the sheet.
type ColumnMapResult struct {
	Columns []ColumnAssignment `json:"columns"`
}

// sampleRowsForAI is how many data rows go with the header. Enough to show what
// a column holds; few enough that the request stays small on any file.
const sampleRowsForAI = 8

// columnMapPrompt is versioned here, beside the code that parses its answer.
const columnMapPrompt = `You map spreadsheet columns to database fields for an Egyptian pharmaceutical catalogue.

You are given the header row, a few sample data rows, and the list of database fields. For each column you recognise, return its 1-based column number and the field name it holds.

Rules:
- Use ONLY field names from target_fields. Never invent one.
- Judge by the sample values, not only the header. A column headed "الكود" holding 13-digit numbers is barcode, not sku.
- Assign each field to at most one column, and each column to at most one field.
- OMIT columns you do not recognise. A missing mapping is safe; a wrong one corrupts every row.
- Arabic headers are common: "اسم الصنف"=name_ar, "الشركة المصنعة"=manufacturer, "سعر البيع"=price, "سعر الجمهور"=public_price, "الشكل الصيدلي"=dosage_form, "التصنيف"/"الفئة"=category, "الاسم العلمي"=generic_name, "التركيز"=concentration, "الوحدة"=unit.
- Distinguish price kinds: selling price=price, public/consumer price=public_price, cost/purchase price=cost_price.

Respond with ONLY JSON: {"columns":[{"column":1,"field":"name_ar","confidence":0.95}]}`

// ColumnMapPrompt is the instruction the column-mapping capability runs under.
func ColumnMapPrompt() string { return columnMapPrompt }

// ColumnMapSchema constrains the model to the shape the parser expects.
func ColumnMapSchema() map[string]any {
	return map[string]any{
		"name":   "column_map",
		"strict": false,
		"schema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"columns": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"column":     map[string]any{"type": "integer"},
							"field":      map[string]any{"type": "string"},
							"confidence": map[string]any{"type": "number"},
						},
						"required": []string{"column", "field"},
					},
				},
			},
			"required": []string{"columns"},
		},
	}
}

// MappableFields are the product fields a model may assign a column to.
var MappableFields = []string{
	FieldNameAR, FieldNameEN, FieldSKU, FieldBarcode,
	FieldPrice, FieldPublicPrice, FieldCostPrice, FieldDiscount,
	FieldManufacturer, FieldCategory, FieldDosageForm, FieldConcentration,
	FieldGenericName, FieldActive, FieldUnit, FieldQuantity,
	FieldDescriptionAR, FieldStatus,
}

// BuildColumnMapRequest assembles the one request that reads a sheet's shape.
func BuildColumnMapRequest(data *SheetData, layout SheetLayout) ColumnMapRequest {
	req := ColumnMapRequest{Fields: make([]FieldOption, 0, len(MappableFields))}
	for _, field := range MappableFields {
		req.Fields = append(req.Fields, FieldOption{Field: field, Label: FieldLabels[field]})
	}
	if data == nil || len(data.Rows) == 0 {
		return req
	}

	headerRow, firstData := -1, 0
	if len(layout.HeaderRows) > 0 {
		headerRow = layout.HeaderRows[0]
		firstData = headerRow + 1
	}
	if headerRow >= 0 && headerRow < len(data.Rows) {
		req.Headers = cleanRow(data.Rows[headerRow])
	}

	for i := firstData; i < len(data.Rows) && len(req.Sample) < sampleRowsForAI; i++ {
		if isBlankRow(data.Rows[i]) {
			continue
		}
		req.Sample = append(req.Sample, cleanRow(data.Rows[i]))
	}
	return req
}

func cleanRow(row []string) []string {
	out := make([]string, len(row))
	for i, cell := range row {
		// Long free text tells the model nothing extra and costs tokens on
		// every sample row.
		out[i] = truncateRunes(CleanCellString(cell), 60)
	}
	return out
}

func truncateRunes(s string, limit int) string {
	r := []rune(s)
	if len(r) <= limit {
		return s
	}
	return string(r[:limit]) + "…"
}

// ApplyColumnMap turns the model's reading into layout overrides.
//
// It is deliberately conservative. A field the model names that the importer
// does not know is dropped; a column outside the sheet is dropped; and a field
// the deterministic mapper already bound with certainty is left alone, because
// an exact header match is better evidence than a model's opinion of it.
func ApplyColumnMap(result ColumnMapResult, plan ColumnPlan, width int) LayoutOverrides {
	known := make(map[string]bool, len(MappableFields))
	for _, field := range MappableFields {
		known[field] = true
	}

	certain := map[string]bool{}
	for _, binding := range plan.Bindings {
		if binding.Score >= scoreCertain {
			certain[binding.Field] = true
		}
	}

	overrides := LayoutOverrides{Columns: map[string]int{}}
	takenColumn := map[int]bool{}
	for _, assignment := range result.Columns {
		field := strings.TrimSpace(assignment.Field)
		if !known[field] || certain[field] {
			continue
		}
		if assignment.Column < 1 || (width > 0 && assignment.Column > width) {
			continue
		}
		if takenColumn[assignment.Column] {
			continue
		}
		if _, already := overrides.Columns[field]; already {
			continue
		}
		overrides.Columns[field] = assignment.Column
		takenColumn[assignment.Column] = true
	}

	if len(overrides.Columns) == 0 {
		overrides.Columns = nil
	}
	return overrides
}

// DecodeColumnMap reads the model's answer, tolerating markdown fences.
func DecodeColumnMap(content string) (ColumnMapResult, error) {
	var out ColumnMapResult
	if err := decodeJSON(content, &out); err != nil {
		return ColumnMapResult{}, fmt.Errorf("catalog: decode column map: %w", err)
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// Requests 2 and 3: taxonomy values
// ---------------------------------------------------------------------------

// ValueMapKind names which taxonomy a value-mapping request is about.
type ValueMapKind string

const (
	// ValueMapCategory maps the file's category words onto فئات المنتجات.
	ValueMapCategory ValueMapKind = "category"
	// ValueMapDosageForm maps the file's form words onto الأشكال الصيدلية.
	ValueMapDosageForm ValueMapKind = "dosage_form"
)

// ValueMapRequest asks the model to translate a file's distinct values onto the
// values the catalogue already uses.
//
// Distinct is the whole point: a fifty-thousand-row file has perhaps twenty
// category words in it, so this is one small request no matter how large the
// file is.
type ValueMapRequest struct {
	Kind ValueMapKind `json:"kind"`
	// Sources are the distinct values found in the file.
	Sources []string `json:"source_values"`
	// Targets are what the catalogue already has.
	Targets []string `json:"existing_values"`

	OrganizationID int64 `json:"-"`
	UserID         int64 `json:"-"`
}

// ValueMatch is one translation.
type ValueMatch struct {
	Source string `json:"source"`
	// Target is the existing value it means, or empty when none fits.
	Target     string  `json:"target"`
	Confidence float64 `json:"confidence,omitempty"`
}

// ValueMapResult is the model's translation table.
type ValueMapResult struct {
	Matches []ValueMatch `json:"matches"`
}

// maxDistinctValues bounds one value-mapping request. A file with more distinct
// category words than this has a mis-mapped column, and the answer to that is
// the review screen rather than a larger prompt.
const maxDistinctValues = 300

// minMapConfidence is the floor below which a translation is discarded. A wrong
// category reads as fact downstream; an unmapped one reads as "not classified".
const minMapConfidence = 0.6

const valueMapPrompt = `You translate values from a supplier's spreadsheet into the values an Egyptian pharmaceutical catalogue already uses.

You are given source_values (distinct values found in the uploaded file) and existing_values (what the catalogue already has). For each source value, return the existing value it means.

Rules:
- target MUST be copied EXACTLY from existing_values, character for character. Never invent, translate, reword, or reformat it.
- Return target as "" when no existing value means the same thing. An empty answer is correct and expected; a wrong one mislabels every product that uses that value.
- Match meaning, not spelling: Arabic and English names for the same thing match, singular and plural match, and abbreviations match their full form.
- Different strengths, forms, or audiences are NOT the same value.
- Return one entry per source value, with source copied exactly.
- confidence is 0.0 to 1.0.

Respond with ONLY JSON: {"matches":[{"source":"...","target":"...","confidence":0.9}]}`

// ValueMapPrompt is the instruction the value-mapping capability runs under.
func ValueMapPrompt() string { return valueMapPrompt }

// ValueMapSchema constrains the model to the shape the parser expects.
func ValueMapSchema() map[string]any {
	return map[string]any{
		"name":   "value_map",
		"strict": false,
		"schema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"matches": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"source":     map[string]any{"type": "string"},
							"target":     map[string]any{"type": "string"},
							"confidence": map[string]any{"type": "number"},
						},
						"required": []string{"source", "target"},
					},
				},
			},
			"required": []string{"matches"},
		},
	}
}

// DistinctValues collects the distinct non-empty values a field holds across
// the parsed products, in first-seen order.
//
// Folding is what keeps the request small: "أقراص", "اقراص" and "Aqras " are
// one question, and the answer applies to every row that used any of them.
func DistinctValues(prods []*Product, read func(*Product) string) []string {
	seen := map[string]bool{}
	var out []string

	for _, p := range prods {
		if p == nil {
			continue
		}
		value := CleanCellString(read(p))
		if value == "" {
			continue
		}
		key := NormalizeKey(value)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, value)
		if len(out) >= maxDistinctValues {
			break
		}
	}
	return out
}

// ValueMapping is a resolved translation table, keyed by folded source value so
// every spelling of a value finds it.
type ValueMapping struct {
	// resolved maps a folded source value to the exact existing value.
	resolved map[string]string
	// unmatched lists source values with no existing equivalent, in the file's
	// own spelling, for the caller to create when its toggle allows.
	unmatched []string
}

// Lookup returns the existing value a source value means.
func (m ValueMapping) Lookup(source string) (string, bool) {
	if m.resolved == nil {
		return "", false
	}
	target, ok := m.resolved[NormalizeKey(source)]
	return target, ok
}

// Unmatched lists the values nothing existing covers.
func (m ValueMapping) Unmatched() []string { return m.unmatched }

// Matched is how many distinct values were translated.
func (m ValueMapping) Matched() int { return len(m.resolved) }

// BuildValueMapping combines exact folding with the model's answer.
//
// Exact folding runs first and wins: if the file says "اقراص" and the catalogue
// has "أقراص", those are the same string once folded and no model is needed or
// trusted to say so. The model only settles what folding cannot.
func BuildValueMapping(sources, targets []string, result ValueMapResult) ValueMapping {
	byFolded := make(map[string]string, len(targets))
	for _, target := range targets {
		if key := NormalizeKey(target); key != "" {
			if _, taken := byFolded[key]; !taken {
				byFolded[key] = target
			}
		}
	}

	mapping := ValueMapping{resolved: map[string]string{}}
	fromModel := map[string]ValueMatch{}
	for _, match := range result.Matches {
		if key := NormalizeKey(match.Source); key != "" {
			fromModel[key] = match
		}
	}

	for _, source := range sources {
		key := NormalizeKey(source)
		if key == "" {
			continue
		}
		if exact, ok := byFolded[key]; ok {
			mapping.resolved[key] = exact
			continue
		}

		match, asked := fromModel[key]
		target := strings.TrimSpace(match.Target)
		// The model must name an existing value exactly. Anything else — a
		// reworded label, an invented one — is discarded rather than written.
		if asked && target != "" &&
			(match.Confidence == 0 || match.Confidence >= minMapConfidence) {
			if exact, known := byFolded[NormalizeKey(target)]; known {
				mapping.resolved[key] = exact
				continue
			}
		}
		mapping.unmatched = append(mapping.unmatched, source)
	}

	sort.Strings(mapping.unmatched)
	return mapping
}

// DecodeValueMap reads the model's answer, tolerating markdown fences.
func DecodeValueMap(content string) (ValueMapResult, error) {
	var out ValueMapResult
	if err := decodeJSON(content, &out); err != nil {
		return ValueMapResult{}, fmt.Errorf("catalog: decode value map: %w", err)
	}
	return out, nil
}

// EncodeJSON renders a request as the model's user message.
func EncodeJSON(v any) (string, error) {
	body, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("catalog: encode ai request: %w", err)
	}
	return string(body), nil
}

// decodeJSON parses a model answer, stripping the markdown fences models wrap
// JSON in often enough that tolerating them is part of parsing.
func decodeJSON(content string, into any) error {
	clean := strings.TrimSpace(content)
	clean = strings.TrimPrefix(clean, "```json")
	clean = strings.TrimPrefix(clean, "```")
	clean = strings.TrimSuffix(clean, "```")
	return json.Unmarshal([]byte(strings.TrimSpace(clean)), into)
}

// NeedsColumnHelp reports whether header detection left enough doubt to be
// worth an AI request. Exported so the decision is testable on its own.
func NeedsColumnHelp(plan ColumnPlan) bool { return needsColumnHelp(plan) }
