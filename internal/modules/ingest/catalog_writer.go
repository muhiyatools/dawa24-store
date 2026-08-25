package ingest

import (
	"context"
	"fmt"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/modules/ingest/engine"
	"github.com/muhiya/dawa24-store/internal/modules/inventory"
)

// Writing one batch.
//
// Everything about a batch is decided before anything is written, and then
// written in four calls: create the catalogue products the file introduced,
// write the variants, write the balances, record what happened. Deciding first
// is what makes the four calls batchable; batching is what makes a
// nine-thousand-row file take seconds rather than minutes.

// counters accumulate across the whole run.
type counters struct {
	inserted  int
	updated   int
	skipped   int
	errors    int
	matched   int
	review    int
	unmatched int
	created   int
}

// decision is what the run resolved for one row, before any write.
type decision struct {
	row   *engine.Row
	match engine.MatchResult
	// productID is the catalogue product the variant will point at, once any
	// newly created product has an id.
	productID int64
	// variantID is non-zero when an existing variant of the vendor's is being
	// updated rather than a new one inserted.
	variantID int64
	// newProduct is set when the row introduced a product the catalogue lacks.
	newProduct *catalog.Product
	outcome    string
	message    string
	// ref is this decision's index within the batch, used to tie a write
	// failure back to it.
	ref int
}

// importWriter carries the state of one processing run.
type importWriter struct {
	svc      *Service
	session  *Session
	settings Settings
	index    *engine.Index
	variants *variantIndex
	match    engine.MatchOptions

	counts    counters
	touched   []int64
	processed int
}

// write handles one batch of parsed rows.
func (w *importWriter) write(ctx context.Context, batch []*engine.Row) error {
	decisions := w.decide(batch)
	if err := w.createProducts(ctx, decisions); err != nil {
		return err
	}
	if err := w.writeVariants(ctx, decisions); err != nil {
		return err
	}
	if err := w.writeStocks(ctx, decisions); err != nil {
		return err
	}
	if err := w.record(ctx, decisions); err != nil {
		return err
	}

	w.processed += len(batch)
	w.reportProgress(ctx)
	return nil
}

// decide resolves every row in the batch without writing anything.
func (w *importWriter) decide(batch []*engine.Row) []*decision {
	out := make([]*decision, 0, len(batch))
	for i, row := range batch {
		d := &decision{row: row, ref: i}
		out = append(out, d)

		d.match = w.index.Match(row, w.match)
		w.countMatch(d.match)

		switch {
		case d.match.Matched() && d.match.Level.Settled():
			d.productID = d.match.ProductID
		case w.settings.Unmatched == UnmatchedSkip:
			d.outcome = OutcomeSkipped
			d.message = w.unmatchedMessage(d.match)
			w.counts.skipped++
			continue
		default:
			// The catalogue does not carry this product yet, or carries it too
			// ambiguously to pick. Either way the vendor stocks it, so it is
			// registered as pending rather than dropped.
			d.newProduct = buildProduct(row)
		}

		d.variantID, _ = w.variants.resolve(row, d.productID)
		if !w.allowed(d.variantID) {
			d.outcome = OutcomeSkipped
			d.message = w.modeMessage(d.variantID)
			w.counts.skipped++
			continue
		}
	}
	return out
}

// countMatch tallies how the shared catalogue answered.
func (w *importWriter) countMatch(m engine.MatchResult) {
	switch {
	case m.Level.Settled():
		w.counts.matched++
	case m.Level == engine.MatchReview || m.Level == engine.MatchAmbiguous:
		w.counts.review++
	default:
		w.counts.unmatched++
	}
}

// allowed applies the import mode to one row.
func (w *importWriter) allowed(variantID int64) bool {
	switch w.settings.Mode {
	case ModeAddOnly:
		return variantID == 0
	case ModeUpdateOnly:
		return variantID != 0
	default:
		return true
	}
}

func (w *importWriter) modeMessage(variantID int64) string {
	if variantID == 0 {
		return "تم التخطي: الصنف غير موجود لديك، والوضع الحالي يحدّث الموجود فقط."
	}
	return "تم التخطي: الصنف موجود لديك بالفعل، والوضع الحالي يضيف الجديد فقط."
}

func (w *importWriter) unmatchedMessage(m engine.MatchResult) string {
	if m.Level == engine.MatchAmbiguous {
		return "تم التخطي: أكثر من صنف في الكتالوج المركزي يطابق هذا السطر بنفس الدرجة."
	}
	return "تم التخطي: لا يوجد صنف مطابق في الكتالوج المركزي."
}

// createProducts registers the catalogue products this batch introduced.
func (w *importWriter) createProducts(ctx context.Context, decisions []*decision) error {
	var pending []*catalog.Product
	var owners []*decision
	for _, d := range decisions {
		if d.outcome != "" || d.newProduct == nil {
			continue
		}
		pending = append(pending, d.newProduct)
		owners = append(owners, d)
	}
	if len(pending) == 0 {
		return nil
	}

	ids, err := w.svc.catalog.CreateImportProducts(ctx, pending)
	if err != nil {
		return fmt.Errorf("create catalogue products: %w", err)
	}
	for i, d := range owners {
		if i >= len(ids) || ids[i] <= 0 {
			d.outcome = OutcomeError
			d.message = "تعذر تسجيل الصنف في الكتالوج المركزي."
			w.counts.errors++
			continue
		}
		d.productID = ids[i]
		w.counts.created++
	}
	return nil
}

// writeVariants writes the vendor's own catalogue rows.
func (w *importWriter) writeVariants(ctx context.Context, decisions []*decision) error {
	rows := make([]catalog.VariantWriteRow, 0, len(decisions))
	byRef := make(map[int]*decision, len(decisions))
	for _, d := range decisions {
		if d.outcome != "" {
			continue
		}
		variant := w.buildVariant(d)
		rows = append(rows, catalog.VariantWriteRow{Ref: d.ref, Variant: variant})
		byRef[d.ref] = d
	}
	if len(rows) == 0 {
		return nil
	}

	result, err := w.svc.catalog.BulkWriteVariants(ctx, w.session.OrganizationID, rows)
	if err != nil {
		return fmt.Errorf("write variants: %w", err)
	}
	for _, f := range result.Failures {
		if d, ok := byRef[f.Ref]; ok {
			d.outcome = OutcomeError
			d.message = f.Message
			w.counts.errors++
		}
	}
	for ref, id := range result.IDs {
		d, ok := byRef[ref]
		if !ok || d.outcome != "" || id <= 0 {
			continue
		}
		wasUpdate := d.variantID > 0
		d.variantID = id
		w.variants.remember(d.row, d.productID, id)
		w.touched = append(w.touched, id)
		if wasUpdate {
			d.outcome = OutcomeUpdated
			w.counts.updated++
		} else {
			d.outcome = OutcomeInserted
			w.counts.inserted++
		}
	}
	return nil
}

// writeStocks writes the warehouse balances for everything that landed.
func (w *importWriter) writeStocks(ctx context.Context, decisions []*decision) error {
	if w.svc.inventory == nil || w.settings.WarehouseID <= 0 ||
		w.settings.StockMode == inventory.StockKeep {
		return nil
	}

	rows := make([]inventory.StockWriteRow, 0, len(decisions))
	byRef := make(map[int]*decision, len(decisions))
	for _, d := range decisions {
		if d.variantID <= 0 || d.outcome == OutcomeError || d.outcome == OutcomeSkipped {
			continue
		}
		if !d.row.HasQuantity && d.row.MinThreshold == 0 {
			// The file said nothing about this row's stock and nothing about its
			// threshold either. Writing a balance row here would create one at
			// zero for a product the vendor may hold plenty of.
			continue
		}
		rows = append(rows, inventory.StockWriteRow{
			Ref:         d.ref,
			HasQuantity: d.row.HasQuantity,
			Stock: &inventory.Stock{
				OrganizationID:   w.session.OrganizationID,
				WarehouseID:      w.settings.WarehouseID,
				ProductID:        d.productID,
				ProductVariantID: d.variantID,
				Quantity:         d.row.Quantity,
				MinThreshold:     d.row.MinThreshold,
			},
		})
		byRef[d.ref] = d
	}
	if len(rows) == 0 {
		return nil
	}

	result, err := w.svc.inventory.BulkWriteStocks(ctx, w.settings.StockMode, rows)
	if err != nil {
		return fmt.Errorf("write stocks: %w", err)
	}
	for _, f := range result.Failures {
		if d, ok := byRef[f.Ref]; ok {
			// The variant landed; only its balance did not. That is a warning on
			// a written row, not a failed row, and saying otherwise would have
			// the results screen contradict the catalogue.
			d.message = appendMessage(d.message, "تعذر تحديث الرصيد: "+f.Message)
			d.row.Issues = append(d.row.Issues, engine.Issue{
				Row:      d.row.Number,
				Field:    engine.FieldQuantity,
				Severity: engine.SeverityWarning,
				Message:  f.Message,
			})
		}
	}
	return nil
}

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
