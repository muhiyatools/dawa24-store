package catalog

import (
	"strings"

	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// The file as the importer read it, kept as data rather than recomputed.
//
// Every screen in the wizard needs to say the same three things: which row held
// the column titles, which column the importer decided is which field, and what
// the values in that column actually look like. Deriving that meant decoding the
// whole workbook again on every page render — a second and a half of CPU and a
// 32 MB read off the session row to draw a table of twenty lines. So it is
// computed once, when the file is analysed or the mapping corrected, and stored.

// FileColumn is one column of the uploaded sheet, described for a human.
type FileColumn struct {
	// Index is one-based, matching the column number Excel shows.
	Index int `json:"index"`
	// Letter is the spreadsheet label for Index: A, B, ... AA.
	Letter string `json:"letter"`
	// Header is the column's title as the file wrote it, empty when the file
	// has no header row.
	Header string `json:"header,omitempty"`
	// Field is the product field bound to this column, empty when unbound.
	Field string `json:"field,omitempty"`
	// Confidence is how the binding was reached: certain, likely, or a guess.
	Confidence string `json:"confidence,omitempty"`
	// Samples are the first few distinct values found in the column. They are
	// what lets an admin recognise a mis-mapped column at a glance: a price
	// bound to a column of dates is obvious in the sample and invisible in the
	// header.
	Samples []string `json:"samples,omitempty"`
}

// SampleText renders the samples as one line for a table cell.
func (c FileColumn) SampleText() string { return strings.Join(c.Samples, " · ") }

// FieldBinding is one product field and the column it currently reads from.
type FieldBinding struct {
	Field string `json:"field"`
	Label string `json:"label"`
	// Column is one-based; zero means the field is not read at all.
	Column     int    `json:"column"`
	Header     string `json:"header,omitempty"`
	Confidence string `json:"confidence,omitempty"`
}

// FileStructure is the whole reading: the sheet, its shape, and its columns.
type FileStructure struct {
	Sheet         string   `json:"sheet,omitempty"`
	Format        string   `json:"format,omitempty"`
	Delimiter     string   `json:"delimiter,omitempty"`
	SheetsSkipped []string `json:"sheets_skipped,omitempty"`
	TotalRows     int      `json:"total_rows"`
	Width         int      `json:"width"`
	// HeaderRow is one-based, or zero when the sheet carries no header and the
	// columns are read by position.
	HeaderRow  int          `json:"header_row"`
	FirstRow   int          `json:"first_row"`
	LastRow    int          `json:"last_row"`
	BlockCount int          `json:"block_count"`
	Positional bool         `json:"positional"`
	Columns    []FileColumn `json:"columns,omitempty"`
	// Fields lists every product field the importer can read, whether or not
	// this file bound one, so the mapping screen can offer all of them.
	Fields []FieldBinding `json:"fields,omitempty"`
}

// IsEmpty reports whether nothing has been analysed yet.
func (s FileStructure) IsEmpty() bool { return len(s.Columns) == 0 && s.TotalRows == 0 }

// MappedFields is how many product fields this reading binds to a column.
func (s FileStructure) MappedFields() int {
	n := 0
	for _, f := range s.Fields {
		if f.Column > 0 {
			n++
		}
	}
	return n
}

// MissingCritical names the fields an import really wants and this reading did
// not find. A file with no name column imports nothing usable, and the admin
// should be told before the run rather than after it.
func (s FileStructure) MissingCritical() []string {
	bound := map[string]bool{}
	for _, f := range s.Fields {
		if f.Column > 0 {
			bound[f.Field] = true
		}
	}

	var out []string
	if !bound[FieldNameAR] && !bound[FieldNameEN] {
		out = append(out, FieldLabels[FieldNameAR])
	}
	if !bound[FieldSKU] && !bound[FieldBarcode] {
		out = append(out, FieldLabels[FieldSKU])
	}
	return out
}

// mappableFields are the product fields the mapping screen offers, in the order
// an admin looks for them: what the product is, how it is identified, what it
// costs, then everything else. It is MappableFields plus the English
// description, which the wizard can bind and a model is not asked to.
var mappableFields = append(append([]string{}, MappableFields...), FieldDescriptionEN)

// sampleDepth is how many data rows are scanned for column samples, and
// sampleCount how many distinct values are kept. Three values out of the first
// forty rows is enough to recognise a column and cheap on a sheet of any size.
const (
	sampleDepth = 40
	sampleCount = 3
)

// BuildFileStructure describes a decoded sheet under a resolved layout.
func BuildFileStructure(data *SheetData, layout SheetLayout) FileStructure {
	out := FileStructure{Positional: layout.Positional, BlockCount: len(layout.Blocks)}
	if data == nil {
		return out
	}

	out.Sheet, out.Format, out.Delimiter = data.Sheet, data.Format, data.Delimiter
	out.SheetsSkipped = data.SheetsSkipped
	out.TotalRows, out.Width = len(data.Rows), data.Width

	if len(layout.HeaderRows) > 0 && !layout.Positional {
		out.HeaderRow = layout.HeaderRows[0] + 1
	}
	if len(layout.Blocks) > 0 {
		out.FirstRow = layout.Blocks[0].FirstRow + 1
		out.LastRow = layout.Blocks[len(layout.Blocks)-1].LastRow + 1
	}

	plan := layout.Primary
	fieldByColumn := make(map[int]string, len(plan.Columns))
	for field, idx := range plan.Columns {
		fieldByColumn[idx] = field
	}
	confidence := make(map[string]string, len(plan.Bindings))
	headers := make(map[string]string, len(plan.Bindings))
	for _, b := range plan.Bindings {
		confidence[b.Field] = ConfidenceOf(b.Score)
		headers[b.Field] = b.Header
	}

	header := headerRowCells(data, out.HeaderRow)
	samples := columnSamples(data, layout)

	for i := range data.Width {
		col := FileColumn{Index: i + 1, Letter: ColumnLetter(i)}
		if i < len(header) {
			col.Header = CleanCellString(header[i])
		}
		if i < len(samples) {
			col.Samples = samples[i]
		}
		if field, bound := fieldByColumn[i]; bound {
			col.Field, col.Confidence = field, confidence[field]
		}
		out.Columns = append(out.Columns, col)
	}

	for _, field := range mappableFields {
		binding := FieldBinding{
			Field: field, Label: FieldLabels[field],
			Confidence: confidence[field], Header: headers[field],
		}
		if idx, bound := plan.Columns[field]; bound {
			binding.Column = idx + 1
		}
		out.Fields = append(out.Fields, binding)
	}
	return out
}

// headerRowCells returns the title row's cells, or nil when the sheet has none.
func headerRowCells(data *SheetData, oneBasedHeader int) []string {
	idx := oneBasedHeader - 1
	if idx < 0 || idx >= len(data.Rows) {
		return nil
	}
	return data.Rows[idx]
}

// columnSamples collects the first few distinct values in each column, taken
// from the data rows rather than from the whole sheet, so a repeated header row
// does not become the sample.
func columnSamples(data *SheetData, layout SheetLayout) [][]string {
	out := make([][]string, data.Width)
	seen := make([]map[string]bool, data.Width)
	for i := range seen {
		seen[i] = map[string]bool{}
	}

	start := 0
	if len(layout.Blocks) > 0 {
		start = layout.Blocks[0].FirstRow
	}
	end := min(start+sampleDepth, len(data.Rows))

	for r := start; r < end; r++ {
		for c, cell := range data.Rows[r] {
			if c >= data.Width || len(out[c]) >= sampleCount {
				continue
			}
			clean := CleanCellString(cell)
			if clean == "" || seen[c][clean] {
				continue
			}
			seen[c][clean] = true
			out[c] = append(out[c], truncateSample(clean))
		}
	}
	return out
}

// truncateSample keeps a sample readable inside a table cell.
func truncateSample(s string) string {
	const limit = 42
	runes := []rune(s)
	if len(runes) <= limit {
		return s
	}
	return string(runes[:limit]) + "…"
}

// ConfidenceOf renders a binding score as a word the admin can act on.
func ConfidenceOf(score int) string {
	switch {
	case score >= scoreExact:
		return "مؤكد"
	case score >= scoreStrong:
		return "مرجّح"
	default:
		return "تخميني"
	}
}

// ColumnLetter renders a zero-based column index as its spreadsheet label.
func ColumnLetter(idx int) string {
	if idx < 0 {
		return ""
	}
	var out []byte
	for n := idx; ; n = n/26 - 1 {
		out = append([]byte{byte('A' + n%26)}, out...)
		if n < 26 {
			break
		}
	}
	return string(out)
}

// OverridesFromColumns turns the mapping screen's submitted bindings into the
// overrides the parser reads.
//
// A field the admin left unselected is recorded as IgnoreColumn rather than
// omitted. The screen shows every field with an explicit choice, so "none" is a
// decision; omitting it would let header detection re-bind the field on the
// next run behind the admin's back.
func OverridesFromColumns(columns map[string]int) map[string]int {
	if len(columns) == 0 {
		return nil
	}
	out := make(map[string]int, len(columns))
	for field, column := range columns {
		if column > 0 {
			out[field] = column
			continue
		}
		out[field] = IgnoreColumn
	}
	return out
}

// ProductFieldValue renders one field of a parsed product as text, for the
// mapping screen's preview table.
//
// The preview shows the product the way the importer built it, not the raw
// cell: an admin checking a mapping needs to see that "12.50 ج.م" became a
// price of 12.50 and that a name column really did produce names.
func ProductFieldValue(p *Product, field string) string {
	if p == nil {
		return ""
	}
	switch field {
	case FieldNameAR:
		return p.Name.Get(i18n.AR)
	case FieldNameEN:
		return p.Name.Get(i18n.EN)
	case FieldSKU:
		return p.SKU
	case FieldBarcode:
		return p.Barcode
	case FieldPrice:
		return p.Price.String()
	case FieldPublicPrice:
		return p.OldPrice.String()
	case FieldDiscount:
		return p.Discount.String()
	case FieldManufacturer:
		return p.ManufacturingCompanies
	case FieldCategory:
		return p.SourceCategory
	case FieldDosageForm:
		return p.DosageForm
	case FieldConcentration:
		return p.Concentration
	case FieldGenericName:
		return p.ScientificName
	case FieldActive:
		return p.Active
	case FieldUnit:
		return p.Unit
	case FieldDescriptionAR:
		return p.Description.Get(i18n.AR)
	case FieldDescriptionEN:
		return p.Description.Get(i18n.EN)
	case FieldStatus:
		return string(p.Status)
	default:
		return ""
	}
}
