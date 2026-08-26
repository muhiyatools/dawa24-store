package ingest

import (
	"context"
	"fmt"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/modules/inventory"
	"github.com/muhiya/dawa24-store/internal/shared/productmatch"
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
}

// decision is what the run resolved for one row, before any write.
type decision struct {
	row   *productmatch.Row
	match productmatch.MatchResult
	// productID is the catalogue product the variant will point at.
	productID int64
	// variantID is non-zero when an existing variant of the vendor's is being
	// updated rather than a new one inserted.
	variantID int64
	outcome   string
	message   string
	// ref is this decision's index within the batch, used to tie a write
	// failure back to it.
	ref int
	// bucket is which match counter this row was first counted under, so the AI
	// tier can move it without double-counting.
	bucket matchBucket
}

// importWriter carries the state of one processing run.
type importWriter struct {
	svc      *Service
	session  *Session
	settings Settings
	index    *productmatch.Index
	variants *variantIndex
	match    productmatch.MatchOptions

	counts    counters
	touched   []int64
	processed int
	// ai is the run's AI allowance, carried across batches so the budget is a
	// property of the import rather than of each five hundred rows.
	ai aiBudget
}

// write handles one batch of parsed rows.
func (w *importWriter) write(ctx context.Context, batch []*productmatch.Row) error {
	decisions := w.decide(batch)
	// The AI tier runs between deciding and writing, on the rows the
	// deterministic engine could not settle and only where the vendor asked for
	// it. A row it resolves stops being a new catalogue entry and becomes an
	// update to the existing one — which is the difference between a shared
	// catalogue and forty spellings of Panadol.
	w.adjudicate(ctx, decisions)
	w.settle(decisions)
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

// decide resolves every row in the batch to a catalogue product, without
// writing anything and without yet choosing a variant.
//
// Variant resolution is deliberately a separate pass. It depends on which
// catalogue product the row resolved to, and the AI tier runs in between and
// can change that answer — so resolving the variant here would tie half the
// rows to the wrong one whenever adjudication succeeded.
func (w *importWriter) decide(batch []*productmatch.Row) []*decision {
	out := make([]*decision, 0, len(batch))
	for i, row := range batch {
		d := &decision{row: row, ref: i}
		out = append(out, d)

		d.match = w.index.Match(row, w.match)
		d.bucket = bucketOf(d.match)
		w.count(d.bucket, 1)

		if d.match.Matched() && d.match.Level.Settled() {
			d.productID = d.match.ProductID
		}
	}
	return out
}

// settle decides each row's fate and resolves it onto one of the vendor's own
// variants. It runs after adjudication, so it sees the final catalogue product
// for every row.
//
// A row with no catalogue product is skipped and reported. It is never turned
// into a new master product: the shared catalogue is the administrator's, and a
// supplier's spelling is not an amendment to it.
func (w *importWriter) settle(decisions []*decision) {
	for _, d := range decisions {
		if d.outcome != "" {
			continue
		}
		if d.productID == 0 {
			d.outcome = OutcomeSkipped
			d.message = w.unmatchedMessage(d.match)
			w.counts.skipped++
			continue
		}

		d.variantID, _ = w.variants.resolve(d.row, d.productID)
		if !w.allowed(d.variantID) {
			d.outcome = OutcomeSkipped
			d.message = w.modeMessage(d.variantID)
			w.counts.skipped++
		}
	}
}

// matchBucket is which of the three match counters a row belongs to. It is kept
// on the decision so the AI tier can move a row from one to another without
// having to re-derive where it started.
type matchBucket int

const (
	bucketMatched matchBucket = iota
	bucketReview
	bucketUnmatched
)

func bucketOf(m productmatch.MatchResult) matchBucket {
	switch {
	case m.Level.Settled():
		return bucketMatched
	case m.Level == productmatch.MatchReview || m.Level == productmatch.MatchAmbiguous:
		return bucketReview
	default:
		return bucketUnmatched
	}
}

// count adjusts one of the match counters by delta.
func (w *importWriter) count(b matchBucket, delta int) {
	switch b {
	case bucketMatched:
		w.counts.matched += delta
	case bucketReview:
		w.counts.review += delta
	default:
		w.counts.unmatched += delta
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

// unmatchedMessage tells the vendor why a row was left out and what to do.
//
// "Skipped" on its own is not an answer: the vendor stocks the product and
// needs to know whether to fix their file or to ask for the catalogue to be
// extended. The two cases have different remedies, so they say different things.
func (w *importWriter) unmatchedMessage(m productmatch.MatchResult) string {
	if m.Level == productmatch.MatchAmbiguous {
		return "تم التخطي: أكثر من صنف في الكتالوج المعتمد يطابق هذا السطر بنفس الدرجة — " +
			"راجع المرشحين واختر الصنف الصحيح، أو أضف الباركود إلى ملفك ليُحسم تلقائياً."
	}
	return "تم التخطي: لا يوجد صنف مطابق في الكتالوج المعتمد. " +
		"تأكد من الاسم أو أضف الباركود، وإن كان الصنف غير موجود فاطلب من الإدارة إضافته."
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
			d.row.Issues = append(d.row.Issues, productmatch.Issue{
				Row:      d.row.Number,
				Field:    productmatch.FieldQuantity,
				Severity: productmatch.SeverityWarning,
				Message:  f.Message,
			})
		}
	}
	return nil
}
