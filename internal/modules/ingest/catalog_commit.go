package ingest

// Committing: writing what the vendor reviewed and accepted.
//
// Nothing here matches anything. Every row arriving at this stage already
// carries the catalogue product it resolved to — deterministically, by the AI
// stage, or by the vendor's own hand on the review screen — and this file's
// only job is to turn that into the vendor's variants and warehouse balances
// in as few statements as possible.
//
// What it does have to get right is the settings. Every one of them is a
// promise the vendor made a decision on two screens ago, and a commit that
// quietly ignores one of them is worse than a commit that refuses: the vendor
// believes their catalogue is in the state they asked for. So each setting is
// applied at exactly one place below, and each place says which setting it is.

import (
	"context"
	"fmt"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/modules/inventory"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// commitBatchSize is how many of the vendor's rows are written per round trip.
const commitBatchSize = 250

// commitRun carries the state of one commit.
type commitRun struct {
	svc      *Service
	session  *Session
	settings Settings
	variants *variantIndex

	inserted int
	updated  int
	skipped  int
	errors   int
	// stocked counts the warehouse balances actually written, which is the
	// number a vendor checks when they ask "did the quantities land?".
	stocked int

	// touched are the variants this run wrote, and the seed of the keep-list
	// for the mode that declares the file to be the whole catalogue.
	touched []int64
	// retired is how many of the vendor's other variants that mode took off
	// sale, which is the number the results screen leads with.
	retired  int
	outcomes []RowOutcome
}

// CommitImport writes every reviewed, non-excluded staged row to the vendor's
// catalogue and to the warehouse they chose.
func (s *Service) CommitImport(ctx context.Context, publicID string) (*Session, error) {
	session, err := s.prepareCommit(ctx, publicID)
	if err != nil {
		return nil, err
	}
	if !s.runs.claim(publicID) {
		return nil, apperr.Conflict("import.running", i18n.TDefault("w4_mod.w4str_192_192"))
	}
	defer s.runs.release(publicID)
	return s.commit(ctx, session)
}

// ConfirmImport executes the final commit of reviewed staged rows.
func (s *Service) ConfirmImport(ctx context.Context, publicID string) (*Session, error) {
	return s.CommitImport(ctx, publicID)
}

// prepareCommit loads an import and refuses one that cannot be committed.
func (s *Service) prepareCommit(ctx context.Context, publicID string) (*Session, error) {
	if s.imports == nil || s.catalog == nil {
		return nil, ErrImportStoreUnavailable
	}
	session, err := s.LoadImport(ctx, publicID)
	if err != nil {
		return nil, err
	}
	if session.Phase == PhaseProcessing || s.runs.running(publicID) {
		return nil, apperr.Conflict("import.running", i18n.TDefault("w4_mod.w4str_190_190"))
	}
	if session.Settings.WarehouseID <= 0 {
		return nil, apperr.Validation("import.warehouse_required",
			i18n.TDefault("w4_mod.w4str_191_191"), nil)
	}
	return session, nil
}

// commit performs the write. The caller owns the run claim.
func (s *Service) commit(ctx context.Context, session *Session) (*Session, error) {
	settings := session.Settings.Normalize()

	staged, err := s.imports.StagedRowsForCommit(ctx, session.ID)
	if err != nil {
		return nil, fmt.Errorf("load staged rows for commit: %w", err)
	}

	// Rows the vendor never confirmed are not written, and the count of them is
	// carried into the outcome rather than left out of it. A commit that writes
	// four hundred rows out of nine thousand and reports only the four hundred
	// is telling the truth about what it did and nothing about what it did not.
	_, held, err := s.imports.PendingRowIDs(ctx, session.ID, 1)
	if err != nil {
		s.log.WarnContext(ctx, "pending row count unavailable at commit",
			"import", session.PublicID, "error", err)
	}

	run, err := s.newCommitRun(ctx, session, settings)
	if err != nil {
		return nil, err
	}

	for start := 0; start < len(staged); start += commitBatchSize {
		end := min(start+commitBatchSize, len(staged))
		if err := run.writeBatch(ctx, staged[start:end]); err != nil {
			return nil, err
		}
		s.note(ctx, session, commitPercent(end, len(staged)),
			fmt.Sprintf(i18n.TDefault("w4_mod.d_384"), end))
	}

	retirement := run.retireAbsent(ctx)

	if len(run.outcomes) > 0 {
		if err := s.imports.UpdateCommittedRows(ctx, session.ID, run.outcomes); err != nil {
			s.log.WarnContext(ctx, "commit row ledger not recorded",
				"import", session.PublicID, "error", err)
		}
	}

	session.InsertedRows = run.inserted
	session.UpdatedRows = run.updated
	session.SkippedRows = run.skipped + held
	session.ErrorRows = run.errors
	session.ErrorMessage = commitNotice(run, retirement)
	session.Phase = PhaseCompleted
	if err := s.imports.Finish(ctx, session); err != nil {
		return nil, err
	}

	s.log.InfoContext(ctx, "vendor catalogue import committed",
		"import", session.PublicID, "inserted", run.inserted, "updated", run.updated,
		"skipped", session.SkippedRows, "errors", run.errors, "balances", run.stocked)
	return session, nil
}

// newCommitRun loads the one index a commit needs: the vendor's own variants,
// told which warehouse and branch this import is writing to.
func (s *Service) newCommitRun(
	ctx context.Context, session *Session, settings Settings,
) (*commitRun, error) {
	keys, err := s.catalog.ListVariantKeys(ctx, session.OrganizationID)
	if err != nil {
		return nil, fmt.Errorf("load existing variants: %w", err)
	}
	var inWarehouse map[int64]bool
	if s.inventory != nil && settings.WarehouseID > 0 {
		inWarehouse, err = s.inventory.VariantIDsInWarehouse(ctx, settings.WarehouseID)
		if err != nil {
			// Not fatal. Without it the index falls back to refusing an
			// ambiguous product rather than choosing wrongly, which is the
			// behaviour that existed before this lookup did.
			s.log.WarnContext(ctx, "warehouse variant set unavailable",
				"import", session.PublicID, "warehouse", settings.WarehouseID, "error", err)
			inWarehouse = nil
		}
	}
	return &commitRun{
		svc:      s,
		session:  session,
		settings: settings,
		variants: newVariantIndex(keys, inWarehouse, settings.BranchID),
	}, nil
}

// plannedRow is one staged row that survived the import mode, paired with the
// variant it will be written as.
type plannedRow struct {
	row     *RowOutcome
	variant *catalog.ProductVariant
	// existingID is the vendor's variant this row resolved onto before the
	// write, and zero for a row that will create one.
	existingID int64
}

// writeBatch decides, writes and records one batch of staged rows.
func (c *commitRun) writeBatch(ctx context.Context, chunk []*RowOutcome) error {
	planned := c.plan(chunk)

	// planned is indexed by the row's position in the chunk and holds nil where
	// the import mode declined the row, so that a reference surviving the round
	// trip still points at the row it came from.
	writes := make([]catalog.VariantWriteRow, 0, len(planned))
	for ref, p := range planned {
		if p == nil {
			continue
		}
		writes = append(writes, catalog.VariantWriteRow{Ref: ref, Variant: p.variant})
	}
	if len(writes) == 0 {
		return nil
	}

	result, err := c.svc.catalog.BulkWriteVariants(ctx, c.session.OrganizationID, writes)
	if err != nil {
		return fmt.Errorf("bulk write variants: %w", err)
	}

	stocks, stockRefs := c.applyVariantResult(planned, result)
	c.writeStocks(ctx, stocks, stockRefs)
	return nil
}

// plan resolves each row onto a variant and applies the import mode, recording
// the rows it declines. The result is indexed by the row's position in the
// chunk, which is the reference carried through the write and back.
func (c *commitRun) plan(chunk []*RowOutcome) []*plannedRow {
	planned := make([]*plannedRow, len(chunk))
	for ref, sr := range chunk {
		planned[ref] = c.decide(sr)
	}
	return planned
}

// applyVariantResult reads the write's outcome onto the ledger and builds the
// balances for everything that landed.
//
// It walks the plan in the chunk's own order rather than the result map's. Two
// runs of the same file must produce the same ledger, and a map's iteration
// order is deliberately not the same twice.
func (c *commitRun) applyVariantResult(
	planned []*plannedRow, result catalog.VariantWriteResult,
) ([]inventory.StockWriteRow, map[int]*RowOutcome) {
	failed := make(map[int]string, len(result.Failures))
	for _, failure := range result.Failures {
		failed[failure.Ref] = failure.Message
	}

	stocks := make([]inventory.StockWriteRow, 0, len(planned))
	stockRefs := make(map[int]*RowOutcome, len(planned))

	for ref, p := range planned {
		if p == nil {
			continue
		}
		// The variant we were trying to write is recorded even on failure. A
		// ledger row that says "error" with no variant id makes a failed update
		// indistinguishable from a failed insert, which is exactly how nine
		// hundred refused updates were read as nine hundred refused inserts.
		if message, refused := failed[ref]; refused {
			c.errors++
			c.record(p.row, OutcomeError, nullableVariant(p.existingID), message)
			continue
		}
		id := result.IDs[ref]
		if id <= 0 {
			c.errors++
			c.record(p.row, OutcomeError, nullableVariant(p.existingID),
				i18n.TDefault("w4_mod.w4str_125_125"))
			continue
		}

		// The port says whether it created a row, because it is the only party
		// that knows: a row sent as an update whose variant has since been
		// deleted is inserted instead. Falling back on what we asked for keeps
		// a port that reports nothing from mislabelling every insert.
		if result.InsertedRefs[ref] || p.existingID == 0 {
			c.inserted++
			c.record(p.row, OutcomeInserted, &id, i18n.TDefault("w4_mod.s_380_380"))
		} else {
			c.updated++
			c.record(p.row, OutcomeUpdated, &id, i18n.TDefault("w4_mod.s_380_380"))
		}

		c.variants.remember(p.row.Payload, productOf(p.row), id)
		c.touched = append(c.touched, id)

		if stock := c.stockFor(p.row, id); stock != nil {
			stock.Ref = ref
			stocks = append(stocks, *stock)
			stockRefs[ref] = p.row
		}
	}
	return stocks, stockRefs
}

// decide applies the import mode and builds the variant to write, or records
// why the row is not being written at all.
func (c *commitRun) decide(sr *RowOutcome) *plannedRow {
	productID := productOf(sr)
	if productID <= 0 {
		c.skipped++
		c.record(sr, OutcomeSkipped, nil, i18n.TDefault("w4_mod.s_379_379"))
		return nil
	}

	variantID, _ := c.variants.resolve(sr.Payload, productID, sr.VariantID)

	// The import mode, applied at the one place it can be applied: after the
	// row is known to be about a catalogue product and after we know whether
	// the vendor already has a variant of it.
	switch c.settings.Mode {
	case ModeAddOnly:
		if variantID > 0 {
			c.skipped++
			c.record(sr, OutcomeSkipped, &variantID, i18n.TDefault("w4_mod.w4str_209_209"))
			return nil
		}
	case ModeUpdateOnly:
		if variantID == 0 {
			c.skipped++
			c.record(sr, OutcomeSkipped, nil, i18n.TDefault("w4_mod.w4str_208_208"))
			return nil
		}
	}

	return &plannedRow{
		row:        sr,
		variant:    c.buildVariant(sr, productID, variantID),
		existingID: variantID,
	}
}

// buildVariant renders a reviewed row as the vendor's own catalogue entry.
func (c *commitRun) buildVariant(sr *RowOutcome, productID, variantID int64) *catalog.ProductVariant {
	v := &catalog.ProductVariant{
		ID:             variantID,
		OrganizationID: c.session.OrganizationID,
		ProductID:      productID,
		Name:           i18n.New(sr.EffectiveVariantName(), ""),
		SKU:            sr.SourceCode,
		BranchID:       c.settings.BranchID,
		IsNegotiable:   c.settings.MarkNegotiable,
		Status:         c.statusFor(variantID),
		VariantType:    "standard",
		MinOrderQty:    c.settings.DefaultMinOrderQty,
	}
	if row := sr.Payload; row != nil {
		v.Barcode = row.Barcode
		v.Unit = row.Unit
		v.Image = row.Image
		v.BatchNumber = row.BatchNumber
		v.ExpiryDate = row.ExpiryDate
		if row.MinOrderQty > 0 {
			v.MinOrderQty = row.MinOrderQty
		}
		if !row.CostPrice.IsZero() {
			cost := row.CostPrice
			v.CostPrice = &cost
		}
		if row.Negotiable != nil {
			v.IsNegotiable = *row.Negotiable
		}
		v.Price, v.Discount = listAndDiscount(row)
	}
	return v
}

// statusFor applies "publish immediately".
//
// It governs new variants only, and the empty string on an update means "leave
// the status alone". That asymmetry is the setting's actual meaning: it says
// what a freshly imported offer starts as, and a vendor who leaves it off on a
// routine price refresh is not asking for their whole live catalogue to be
// delisted.
func (c *commitRun) statusFor(variantID int64) catalog.ProductStatus {
	if variantID > 0 {
		return ""
	}
	if c.settings.PublishImmediately {
		return catalog.StatusActive
	}
	return catalog.StatusInactive
}

// stockFor turns a written row into the warehouse balance it implies, or nil
// where there is nothing to write.
//
// "Nothing to write" is a narrow case on purpose. A variant that has just been
// created deserves a balance row even at zero, because a vendor looking at
// their inventory screen for a product they just imported and finding no line
// at all concludes the import failed.
func (c *commitRun) stockFor(sr *RowOutcome, variantID int64) *inventory.StockWriteRow {
	if c.svc.inventory == nil || c.settings.WarehouseID <= 0 || variantID <= 0 {
		return nil
	}
	row := sr.Payload
	if row == nil {
		return nil
	}

	hasQuantity := row.HasQuantity
	quantity := row.Quantity
	// "A blank quantity cell means zero" is the vendor's statement that this
	// file is a full stocktake rather than a price list. It applies to the
	// variants they already have as much as to the ones being created — a
	// stocktake that silently leaves last month's balance on every item it
	// omits is not a stocktake.
	if !hasQuantity && c.settings.BlankQuantityIsZero {
		hasQuantity = true
		quantity = 0
	}

	return &inventory.StockWriteRow{
		HasQuantity: hasQuantity,
		Stock: &inventory.Stock{
			OrganizationID:   c.session.OrganizationID,
			WarehouseID:      c.settings.WarehouseID,
			ProductID:        productOf(sr),
			ProductVariantID: variantID,
			Quantity:         quantity,
			MinThreshold:     stockThreshold(row.MinThreshold, c.settings.DefaultMinThreshold),
		},
	}
}

// writeStocks writes one batch of balances and reports each failure against the
// row that caused it.
//
// A balance that would not write is not a failed row: the variant landed and is
// on sale. But it is also not silence — the previous version logged it to the
// server and told the vendor their import succeeded, which is precisely the
// "the quantities never arrived and nothing said why" complaint.
func (c *commitRun) writeStocks(
	ctx context.Context, rows []inventory.StockWriteRow, byRef map[int]*RowOutcome,
) {
	if len(rows) == 0 {
		return
	}
	result, err := c.svc.inventory.BulkWriteStocks(ctx, c.settings.StockMode, rows)
	if err != nil {
		c.svc.log.ErrorContext(ctx, "bulk write stocks failed",
			"import", c.session.PublicID, "error", err)
		for _, row := range rows {
			if sr, ok := byRef[row.Ref]; ok {
				c.amend(sr, i18n.TDefault("w4_mod.w4str_214_214")+
					i18n.TDefault("w4_mod.w4str_226_226"))
			}
		}
		return
	}
	c.stocked += result.Written
	for _, failure := range result.Failures {
		if sr, ok := byRef[failure.Ref]; ok {
			c.amend(sr, i18n.TDefault("w4_mod.w4str_214_214")+failure.Message)
		}
	}
}

// retireAbsent takes off sale every variant the file does not mention, for the
// mode that declares the file to be the whole catalogue. It returns what the
// vendor should be told about it.
//
// "Does not mention" is the whole correction here. The keep-list used to hold
// only the variants this run WROTE, and the mode was then switched off entirely
// whenever any row was held or errored — which on a real price list is always,
// because a file that matches every row has never once been uploaded. So a
// vendor who chose "this file is my whole catalogue" got an import that
// retired nothing, said so in a field the results screen did not lead with, and
// left their catalogue exactly as it was. That is the complaint this method
// exists to answer.
//
// The caution behind the old guard was right, and it is kept in a sharper form.
// A row the vendor left in the review queue still MENTIONS its product, so the
// variant it refers to is protected: an unsettled match is a reason not to
// write a price, not a reason to delist a product the file plainly lists. What
// is retired is what the file is silent about, which is exactly what the vendor
// was promised.
//
// The one refusal left is a run that wrote nothing at all. Retiring a whole
// catalogue on the strength of a commit that failed is not a mode, it is an
// outage.
func (c *commitRun) retireAbsent(ctx context.Context) string {
	if c.settings.Mode != ModeReplace {
		return ""
	}
	if len(c.touched) == 0 {
		return i18n.TDefault("ingest.commit.retire_skipped_empty")
	}

	retired, err := c.svc.catalog.RetireVariantsExcept(
		ctx, c.session.OrganizationID, c.keepList(ctx))
	if err != nil {
		c.svc.log.WarnContext(ctx, "replace-mode variant retirement failed",
			"import", c.session.PublicID, "error", err)
		return i18n.TDefault("ingest.commit.retire_failed")
	}
	c.retired = len(retired)
	if c.retired == 0 {
		return ""
	}
	c.svc.log.InfoContext(ctx, "replace-mode variants retired",
		"import", c.session.PublicID, "retired", c.retired)
	c.zeroBalances(ctx, retired)
	return fmt.Sprintf(i18n.TDefault("ingest.commit.retired_format"), c.retired)
}

// keepList is every variant that survives a replace: the ones this run wrote,
// plus the ones any included row of the file refers to.
func (c *commitRun) keepList(ctx context.Context) []int64 {
	keep := make(map[int64]struct{}, len(c.touched)*2)
	for _, id := range c.touched {
		keep[id] = struct{}{}
	}

	mentions, err := c.svc.imports.MentionedRows(ctx, c.session.ID)
	if err != nil {
		// Without the mentions the keep-list is the written rows alone, which
		// is the old, over-eager behaviour. Refusing to retire is the safe
		// reading of a lookup that failed.
		c.svc.log.WarnContext(ctx, "replace-mode mentions unavailable",
			"import", c.session.PublicID, "error", err)
		return nil
	}
	for _, m := range mentions {
		if id := c.variants.mentioned(m); id > 0 {
			keep[id] = struct{}{}
		}
		for _, id := range c.variants.mentionedProduct(m.ProductID) {
			keep[id] = struct{}{}
		}
	}

	out := make([]int64, 0, len(keep))
	for id := range keep {
		out = append(out, id)
	}
	return out
}

// zeroBalances empties the retired variants' stock in the warehouse this import
// wrote to.
//
// Only there, and only for variants that already held a balance in it: an
// import writing to one warehouse has said nothing about the vendor's others,
// and creating a zero row for a variant that never had one would fill the
// inventory screen with lines for products the vendor has just delisted.
func (c *commitRun) zeroBalances(ctx context.Context, retired []catalog.RetiredVariant) {
	if c.svc.inventory == nil || c.settings.WarehouseID <= 0 {
		return
	}
	rows := make([]inventory.StockWriteRow, 0, len(retired))
	for _, v := range retired {
		if !c.variants.inWarehouse[v.ID] {
			continue
		}
		rows = append(rows, inventory.StockWriteRow{
			HasQuantity: true,
			Stock: &inventory.Stock{
				OrganizationID:   c.session.OrganizationID,
				WarehouseID:      c.settings.WarehouseID,
				ProductID:        v.ProductID,
				ProductVariantID: v.ID,
				Quantity:         0,
			},
		})
	}
	if len(rows) == 0 {
		return
	}
	if _, err := c.svc.inventory.BulkWriteStocks(ctx, inventory.StockReplace, rows); err != nil {
		c.svc.log.WarnContext(ctx, "replace-mode balance clearing failed",
			"import", c.session.PublicID, "variants", len(rows), "error", err)
	}
}

// record appends one row's outcome to the ledger the results screen reads.
func (c *commitRun) record(sr *RowOutcome, outcome string, variantID *int64, message string) {
	if !c.settings.RecordRows {
		return
	}
	c.outcomes = append(c.outcomes, RowOutcome{
		ID:        sr.ID,
		Outcome:   outcome,
		VariantID: variantID,
		Message:   message,
	})
}

// amend adds a note to the last outcome recorded for a row, which is the one
// its variant write produced.
func (c *commitRun) amend(sr *RowOutcome, note string) {
	for i := len(c.outcomes) - 1; i >= 0; i-- {
		if c.outcomes[i].ID == sr.ID {
			c.outcomes[i].Message = appendMessage(c.outcomes[i].Message, note)
			return
		}
	}
}

// commitNotice is the one sentence the results screen leads with when something
// about the run needs saying.
func commitNotice(run *commitRun, retirement string) string {
	if run.errors > 0 {
		return appendMessage(
			fmt.Sprintf(i18n.TDefault("ingest.commit.row_errors_format"), run.errors), retirement)
	}
	return retirement
}

// commitPercent maps rows written onto the progress bar, leaving the last few
// per cent for the retirement pass and the ledger write.
func commitPercent(done, total int) int {
	if total <= 0 {
		return 95
	}
	return min(5+done*90/total, 95)
}

// stockThreshold prefers the file's alert level and falls back to the vendor's
// default for this import.
func stockThreshold(row, fallback int) int {
	if row > 0 {
		return row
	}
	if fallback > 0 {
		return fallback
	}
	return 0
}

// productOf reads the catalogue product a staged row resolved to.
func productOf(sr *RowOutcome) int64 {
	if sr == nil || sr.ProductID == nil {
		return 0
	}
	return *sr.ProductID
}

// nullableVariant renders a variant id for the ledger, or nil for a row that
// never had one.
func nullableVariant(id int64) *int64 {
	if id <= 0 {
		return nil
	}
	return &id
}
