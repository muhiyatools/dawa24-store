package ingest

import (
	"context"
	"fmt"
)

// What the run tells the vendor about itself.
//
// Two things, and both are reporting rather than importing: the per-row ledger
// that makes the results screen filterable, and the progress the screen polls
// while the run is going. Neither may ever fail the run — a vendor's catalogue
// matters more than the record of how it got there.

// record writes the per-row outcome ledger.
func (w *importWriter) record(ctx context.Context, decisions []*decision) error {
	if !w.settings.RecordRows {
		return nil
	}
	out := make([]RowOutcome, 0, len(decisions))
	for _, d := range decisions {
		outcome := d.outcome
		if outcome == "" {
			outcome = OutcomeSkipped
		}
		rec := RowOutcome{
			SourceRow:   d.row.Number,
			Outcome:     outcome,
			MatchLevel:  string(d.match.Level),
			MatchScore:  d.match.Score,
			DisplayName: d.row.DisplayName(),
			SourceCode:  d.row.SKU,
			Payload:     d.row,
			Issues:      d.row.Issues,
			Message:     appendMessage(d.message, d.match.Reason),
		}
		if d.productID > 0 {
			id := d.productID
			rec.ProductID = &id
			// The catalogue's own name for what this row was tied to. It comes
			// from the index already in memory, so it costs nothing, and without
			// it the results table can only show a number the vendor has no way
			// to check against what they uploaded.
			if p, found := w.index.Lookup(id); found {
				rec.MatchedProductName = w.index.Name(id)
				rec.MatchedProductSKU = p.SKU
				if rec.MatchedProductSKU == "" {
					rec.MatchedProductSKU = p.Barcode
				}
			}
		}
		if d.variantID > 0 {
			id := d.variantID
			rec.VariantID = &id
		}
		// The shortlist is only worth keeping where the vendor may act on it.
		if !d.match.Level.Settled() {
			rec.Candidates = d.match.Candidates
		}
		out = append(out, rec)
	}
	return w.svc.imports.AppendRows(ctx, w.session.ID, w.session.OrganizationID, out)
}

// reportProgress updates the session so the progress screen can move.
//
// Failing to write progress must never fail the run: the vendor's catalogue is
// more important than the bar that describes it.
func (w *importWriter) reportProgress(ctx context.Context) {
	total := w.session.TotalRows
	percent := 0
	if total > 0 {
		percent = w.processed * 100 / total
	}
	note := fmt.Sprintf("تمت معالجة %d صنف", w.processed)
	if err := w.svc.imports.Progress(ctx, w.session.ID, percent, note); err != nil {
		w.svc.log.WarnContext(ctx, "import progress not recorded",
			"import", w.session.PublicID, "error", err)
	}
}

func appendMessage(base, extra string) string {
	switch {
	case extra == "":
		return base
	case base == "":
		return extra
	default:
		return base + " — " + extra
	}
}
