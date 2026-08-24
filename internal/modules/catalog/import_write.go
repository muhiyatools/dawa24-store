package catalog

import "fmt"

// Result types for the bulk write, shared between the repository that produces
// them and the admin screen that renders them.

// WriteAction records what happened to one product in the catalogue.
type WriteAction string

const (
	// ActionInserted means a new catalogue row was created.
	ActionInserted WriteAction = "inserted"
	// ActionUpdated means an existing row was matched and refreshed.
	ActionUpdated WriteAction = "updated"
)

// MatchReason records how an incoming row was tied to an existing product. It
// is shown in the report because "matched by name" deserves more scrutiny from
// the admin than "matched by barcode".
type MatchReason string

const (
	MatchNone    MatchReason = ""
	MatchSKU     MatchReason = "sku"
	MatchBarcode MatchReason = "barcode"
	MatchName    MatchReason = "name"
)

// MatchLabels renders a match reason in the admin's language.
var MatchLabels = map[MatchReason]string{
	MatchSKU:     "مطابقة بكود الصنف",
	MatchBarcode: "مطابقة بالباركود",
	MatchName:    "مطابقة باسم الصنف",
}

// WriteFailure identifies one product the database refused, by its position in
// the batch the caller submitted, so the caller can name the spreadsheet row.
type WriteFailure struct {
	Index  int    `json:"index"`
	Name   string `json:"name"`
	SKU    string `json:"sku,omitempty"`
	Reason string `json:"reason"`
}

// BulkWriteResult reports what a bulk import did to the catalogue.
type BulkWriteResult struct {
	Inserted int
	Updated  int
	// BrandsCreated counts manufacturers registered as new brands along the way.
	BrandsCreated int
	// Matches maps a batch index to how that product was matched, for the rows
	// that updated an existing product rather than inserting a new one.
	Matches map[int]MatchReason
	// Failures lists the rows the database refused. When it is non-empty the
	// whole transaction was rolled back and nothing was written.
	Failures []WriteFailure
}

// Total is the number of catalogue rows the import touched.
func (r BulkWriteResult) Total() int { return r.Inserted + r.Updated }

// Error renders the failures as a single message. It is deliberately concrete —
// the row, the product, and what PostgreSQL objected to — because the previous
// importer surfaced "bulk batch close failed" with no indication of which of
// nine thousand rows was at fault.
func (r BulkWriteResult) Error() string {
	if len(r.Failures) == 0 {
		return ""
	}
	msg := fmt.Sprintf("تعذر حفظ %d صنف", len(r.Failures))
	shown := r.Failures
	if len(shown) > 3 {
		shown = shown[:3]
	}
	for _, f := range shown {
		msg += fmt.Sprintf("؛ «%s»: %s", f.Name, f.Reason)
	}
	if len(r.Failures) > len(shown) {
		msg += fmt.Sprintf(" (و%d حالات أخرى)", len(r.Failures)-len(shown))
	}
	return msg
}

// ExistingMatch ties an incoming row to a product already in the catalogue.
type ExistingMatch struct {
	ProductID int64       `json:"product_id"`
	Reason    MatchReason `json:"reason"`
}
