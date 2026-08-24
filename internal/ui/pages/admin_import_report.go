package pages

import (
	"fmt"
	"strings"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// ImportReportView is everything the import result page renders.
//
// It is assembled here rather than in the template because deciding what an
// import meant — how many rows landed, which were merged, what was assumed —
// is logic, and logic in a .templ file cannot be tested.
type ImportReportView struct {
	// Title heads the page.
	Title string
	// Fatal, when set, is the single reason nothing was imported.
	Fatal string
	// FatalDetail is the technical detail under it, shown smaller.
	FatalDetail string
	// FileName is the uploaded file as the admin named it.
	FileName string
	// Source describes what was read: the worksheet or the CSV delimiter.
	Source string
	// Succeeded is true when rows actually reached the catalogue.
	Succeeded bool

	Inserted      int
	Updated       int
	BrandsCreated int
	RowsRead      int
	Parsed        int
	Skipped       int
	Merged        int
	Rejected      int

	// Bindings shows how each column of the file was interpreted.
	Bindings []ImportBindingRow
	// Unmapped lists column headings that matched no product field.
	Unmapped []string
	// SheetsSkipped names other worksheets that held data and were not read.
	SheetsSkipped []string
	// Errors and Warnings are the per-row findings, errors first.
	Errors   []catalog.RowIssue
	Warnings []catalog.RowIssue
	// TruncatedIssues is true when more findings existed than were retained.
	TruncatedIssues bool
}

// ImportBindingRow is one column-to-field decision as rendered.
type ImportBindingRow struct {
	Column     string
	Header     string
	Field      string
	Confidence string
}

// maxRenderedIssues bounds the tables on the page. A browser handed nine
// thousand table rows becomes unusable, and the counts above the table stay
// exact regardless.
const maxRenderedIssues = 100

// NewImportReportView summarises a parse into the page model.
func NewImportReportView(filename string, parsed *catalog.ParseResult) ImportReportView {
	stats := parsed.Stats
	view := ImportReportView{
		Title:         "نتيجة استيراد الأصناف",
		FileName:      filename,
		Source:        describeSource(stats),
		RowsRead:      stats.TotalRowsRead,
		Parsed:        stats.ValidProducts,
		Skipped:       stats.EmptyRows + stats.RepeatedHeader,
		Merged:        stats.DuplicateRows,
		Rejected:      stats.RejectedRows,
		Unmapped:      parsed.Plan.Unmapped,
		SheetsSkipped: parsed.SheetsSkipped,
	}

	for _, b := range parsed.Plan.Bindings {
		view.Bindings = append(view.Bindings, ImportBindingRow{
			Column:     columnLetter(b.Index),
			Header:     b.Header,
			Field:      catalog.FieldLabels[b.Field],
			Confidence: confidenceLabel(b.Score),
		})
	}
	if parsed.Plan.Positional {
		view.Bindings = append(view.Bindings, ImportBindingRow{
			Column: "—", Header: "لا يوجد صف عناوين", Field: "قراءة بترتيب الأعمدة", Confidence: "منخفضة",
		})
	}

	view.AddIssues(parsed.Issues)
	return view
}

// describeSource says which worksheet or delimiter the rows came from, so an
// admin whose workbook has five tabs can confirm the right one was read.
func describeSource(stats catalog.ImportStats) string {
	switch stats.Format {
	case "xlsx":
		if stats.SheetName != "" {
			return fmt.Sprintf("ملف Excel — ورقة العمل «%s»", stats.SheetName)
		}
		return "ملف Excel"
	case "csv":
		return fmt.Sprintf("ملف نصي — الفاصل «%s»", delimiterLabel(stats.Delimiter))
	default:
		return ""
	}
}

func delimiterLabel(d string) string {
	switch d {
	case "\t":
		return "Tab"
	case "":
		return ","
	default:
		return d
	}
}

// confidenceLabel translates a match score into something an admin can judge.
// A "منخفضة" binding is the one worth checking before trusting the import.
func confidenceLabel(score int) string {
	switch {
	case score >= 100:
		return "مؤكدة"
	case score >= 60:
		return "عالية"
	default:
		return "منخفضة"
	}
}

// columnLetter renders a zero-based column index the way Excel labels it, so a
// finding can be found in the file without counting columns by hand.
func columnLetter(idx int) string {
	if idx < 0 {
		return "—"
	}
	name := ""
	for n := idx; ; n = n/26 - 1 {
		name = string(rune('A'+n%26)) + name
		if n < 26 {
			break
		}
	}
	return name
}

// AddIssues files findings into the error and warning tables.
func (v *ImportReportView) AddIssues(issues []catalog.RowIssue) {
	for _, issue := range issues {
		if issue.Severity == catalog.SeverityError {
			if len(v.Errors) < maxRenderedIssues {
				v.Errors = append(v.Errors, issue)
			} else {
				v.TruncatedIssues = true
			}
			continue
		}
		if len(v.Warnings) < maxRenderedIssues {
			v.Warnings = append(v.Warnings, issue)
		} else {
			v.TruncatedIssues = true
		}
	}
}

// AddFailures turns database refusals into row-addressed errors.
//
// A failure carries its position in the submitted batch, not its spreadsheet
// row, so it is resolved back through the parsed products to the name the admin
// will recognise in their file.
func (v *ImportReportView) AddFailures(failures []catalog.WriteFailure, parsed *catalog.ParseResult) {
	for _, f := range failures {
		name := f.Name
		if name == "" && f.Index >= 0 && f.Index < len(parsed.Products) {
			name = parsed.Products[f.Index].Name.Get(i18n.AR)
		}
		v.Errors = append(v.Errors, catalog.RowIssue{
			Value:    name,
			Message:  f.Reason,
			Severity: catalog.SeverityError,
		})
	}
}

// ApplyWriteResult records what the catalogue write actually did.
func (v *ImportReportView) ApplyWriteResult(res catalog.BulkWriteResult) {
	v.Inserted = res.Inserted
	v.Updated = res.Updated
	v.BrandsCreated = res.BrandsCreated
	v.Succeeded = res.Total() > 0
	if v.Succeeded {
		v.Title = "تم استيراد الأصناف بنجاح"
	}
}

// Summary is the one-line result shown under the title.
func (v ImportReportView) Summary() string {
	if !v.Succeeded {
		return ""
	}
	parts := []string{}
	if v.Inserted > 0 {
		parts = append(parts, fmt.Sprintf("إضافة %d صنف جديد", v.Inserted))
	}
	if v.Updated > 0 {
		parts = append(parts, fmt.Sprintf("تحديث %d صنف موجود", v.Updated))
	}
	if v.BrandsCreated > 0 {
		parts = append(parts, fmt.Sprintf("تسجيل %d شركة مصنعة جديدة", v.BrandsCreated))
	}
	return "تم " + strings.Join(parts, "، ") + " في الكتالوج المعتمد."
}

// HasFindings reports whether either findings table has anything to show.
func (v ImportReportView) HasFindings() bool { return len(v.Errors) > 0 || len(v.Warnings) > 0 }

// IssueRowLabel renders the spreadsheet row a finding belongs to.
func IssueRowLabel(issue catalog.RowIssue) string {
	if issue.Row <= 0 {
		return "—"
	}
	return fmt.Sprintf("%d", issue.Row)
}
