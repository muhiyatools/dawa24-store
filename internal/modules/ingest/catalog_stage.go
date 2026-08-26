package ingest

import (
	"context"
	"fmt"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/modules/inventory"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/productmatch"
	"github.com/muhiya/dawa24-store/internal/shared/sheet"
)

// StageImport parses all rows from the file, scores them against the central catalog,
// runs AI adjudication on ambiguous rows if enabled, and stages every row into
// ingest.catalog_import_rows without writing anything to live variants or stocks.
func (s *Service) StageImport(ctx context.Context, session *Session) error {
	analysis, err := s.analyse(ctx, session)
	if err != nil {
		return err
	}
	analysis.Complete()

	// Clear previous staging rows for this session
	if err := s.imports.ClearRows(ctx, session.ID); err != nil {
		s.log.WarnContext(ctx, "clear import rows failed", "error", err)
	}

	products, err := s.catalog.ListMatchProducts(database.AsSystem(ctx))
	if err != nil {
		return fmt.Errorf("load shared catalogue: %w", err)
	}
	master := make([]productmatch.MasterProduct, 0, len(products))
	for _, p := range products {
		master = append(master, productmatch.MasterProduct{
			ID: p.ID, NameAR: p.NameAR, NameEN: p.NameEN, SKU: p.SKU,
			Barcode: p.Barcode, Scientific: p.Scientific, DosageForm: p.DosageForm,
			Concentration: p.Concentration, Unit: p.Unit,
			Manufacturer: p.Manufacturer, PublicPrice: p.PublicPrice,
		})
	}

	matchOpts := productmatch.DefaultMatchOptions()
	matchOpts.MinStrong = session.Settings.MinMatchScore
	if matchOpts.MinStrong <= 0 {
		matchOpts.MinStrong = 0.30
	}
	matchOpts.MinReview = min(matchOpts.MinStrong*0.5, 0.20)
	matchOpts.TrustSupplierCode = session.Settings.TrustSupplierCode

	index := productmatch.NewIndex(master)

	content, err := s.imports.File(ctx, session.ID)
	if err != nil {
		return err
	}
	book, err := sheet.Open(content, session.Filename)
	if err != nil {
		return apperr.Validation("import.unreadable", err.Error(), nil)
	}
	defer func() { _ = book.Close() }()
	if session.Source.Sheet != "" {
		_ = book.Use(session.Source.Sheet)
	}

	opts := productmatch.DefaultProcessOptions()
	opts.Parse = parseOptionsFrom(session.Settings)
	opts.Duplicates = session.Settings.Duplicates
	opts.Vocabulary = s.vocabulary(ctx, session.OrganizationID)

	var matchedCount, reviewCount, unmatchedCount, errorCount int
	var aiBudgetVal aiBudget

	result, err := productmatch.Process(book, analysis.Layout, analysis.Mapping, opts,
		func(batch []*productmatch.Row) error {
			stagedRows := make([]RowOutcome, 0, len(batch))
			decisions := make([]*decision, 0, len(batch))

			for i, row := range batch {
				d := &decision{row: row, ref: i}
				decisions = append(decisions, d)
				d.match = index.Match(row, matchOpts)
				if d.match.Matched() && d.match.Level.Settled() {
					d.productID = d.match.ProductID
				}
			}

			// Run AI adjudication on difficult rows if enabled
			if session.Settings.UseAI && s.adjudicator != nil && !aiBudgetVal.spent() {
				wDummy := &importWriter{
					svc:      s,
					session:  session,
					settings: session.Settings,
					index:    index,
					match:    matchOpts,
					ai:       aiBudgetVal,
				}
				wDummy.adjudicate(ctx, decisions)
				aiBudgetVal = wDummy.ai
			}

			for _, d := range decisions {
				outcomeVal := OutcomeStaged
				var prodID *int64
				if d.match.ProductID > 0 {
					pID := d.match.ProductID
					prodID = &pID
				}
				if len(d.row.Issues) > 0 {
					for _, iss := range d.row.Issues {
						if iss.Severity == productmatch.SeverityError {
							outcomeVal = OutcomeError
							errorCount++
							break
						}
					}
				}

				switch {
				case d.match.Level.Settled():
					matchedCount++
				case d.match.Level == productmatch.MatchReview || d.match.Level == productmatch.MatchAmbiguous:
					reviewCount++
				default:
					unmatchedCount++
				}

				stagedRows = append(stagedRows, RowOutcome{
					SourceRow:         d.row.Number,
					Outcome:           outcomeVal,
					MatchLevel:        string(d.match.Level),
					MatchScore:        d.match.Score,
					ProductID:         prodID,
					DisplayName:       d.row.Name,
					SourceCode:        d.row.SKU,
					CustomVariantName: d.row.Name,
					IsExcluded:        false,
					IsManuallyMatched: false,
					Payload:           d.row,
					Candidates:        d.match.Candidates,
					Issues:            d.row.Issues,
					Message:           d.match.Reason,
				})
			}

			return s.imports.AppendRows(ctx, session.ID, session.OrganizationID, stagedRows)
		})
	if err != nil {
		return err
	}

	session.Stats = result.Stats
	session.Findings = result.Issues
	session.TotalRows = result.Stats.SheetRows
	session.MatchedRows = matchedCount
	session.ReviewRows = reviewCount
	session.UnmatchedRows = unmatchedCount
	session.ErrorRows = errorCount + result.Stats.Rejected
	session.Phase = PhaseReview

	return s.imports.SaveDraft(ctx, session)
}

// CommitImport writes all reviewed, non-excluded staged rows to catalog and inventory.
func (s *Service) CommitImport(ctx context.Context, publicID string) (*Session, error) {
	if s.imports == nil || s.catalog == nil {
		return nil, ErrImportStoreUnavailable
	}
	session, err := s.LoadImport(ctx, publicID)
	if err != nil {
		return nil, err
	}
	if session.Phase == PhaseProcessing || s.runs.running(publicID) {
		return nil, apperr.Conflict("import.running",
			"هذه العملية قيد التنفيذ بالفعل. يرجى انتظار انتهائها.")
	}
	if session.Settings.WarehouseID <= 0 {
		return nil, apperr.Validation("import.warehouse_required",
			"يجب اختيار المخزن قبل بدء الاستيراد.", nil)
	}
	if !s.runs.claim(publicID) {
		return nil, apperr.Conflict("import.running", "هذه العملية قيد التنفيذ بالفعل.")
	}
	defer s.runs.release(publicID)

	stagedRows, err := s.imports.StagedRowsForCommit(ctx, session.ID)
	if err != nil {
		return nil, fmt.Errorf("load staged rows for commit: %w", err)
	}

	keys, err := s.catalog.ListVariantKeys(ctx, session.OrganizationID)
	if err != nil {
		return nil, fmt.Errorf("load existing variants: %w", err)
	}
	variantsIdx := newVariantIndex(keys)

	var insertedCount, updatedCount, skippedCount, errorCount int
	var touchedVariants []int64
	var committedOutcomes []RowOutcome

	const batchSize = 250
	for i := 0; i < len(stagedRows); i += batchSize {
		end := min(i+batchSize, len(stagedRows))
		chunk := stagedRows[i:end]

		variantRows := make([]catalog.VariantWriteRow, 0, len(chunk))
		type stagedRef struct {
			row       *RowOutcome
			variantID int64
		}
		refMap := make(map[int]*stagedRef, len(chunk))

		for idx, sr := range chunk {
			if sr.ProductID == nil || *sr.ProductID <= 0 {
				skippedCount++
				committedOutcomes = append(committedOutcomes, RowOutcome{
					ID:        sr.ID,
					Outcome:   OutcomeSkipped,
					VariantID: nil,
					Message:   "لم يتم تحديد صنف مطابق",
				})
				continue
			}

			pID := *sr.ProductID
			vID, _ := variantsIdx.resolve(sr.Payload, pID)

			varName := sr.EffectiveVariantName()
			status := catalog.StatusActive
			if !session.Settings.PublishImmediately {
				status = catalog.StatusInactive
			}
			if sr.Payload != nil && sr.Payload.Status == "inactive" {
				status = catalog.StatusInactive
			}

			price, discount := listAndDiscount(sr.Payload)

			v := &catalog.ProductVariant{
				ID:             vID,
				OrganizationID: session.OrganizationID,
				ProductID:      pID,
				Name:           i18n.New(varName, ""),
				SKU:            sr.SourceCode,
				Barcode:        sr.Payload.Barcode,
				Unit:           sr.Payload.Unit,
				Image:          sr.Payload.Image,
				BatchNumber:    sr.Payload.BatchNumber,
				ExpiryDate:     sr.Payload.ExpiryDate,
				MinOrderQty:    sr.Payload.MinOrderQty,
				BranchID:       session.Settings.BranchID,
				IsNegotiable:   session.Settings.MarkNegotiable,
				Status:         status,
				VariantType:    "standard",
				Price:          price,
				Discount:       discount,
				CostPrice:      sr.Payload.CostPrice,
			}
			if sr.Payload.Negotiable != nil {
				v.IsNegotiable = *sr.Payload.Negotiable
			}

			refMap[idx] = &stagedRef{row: sr, variantID: vID}
			variantRows = append(variantRows, catalog.VariantWriteRow{
				Ref:     idx,
				Variant: v,
			})
		}

		if len(variantRows) > 0 {
			res, err := s.catalog.BulkWriteVariants(ctx, session.OrganizationID, variantRows)
			if err != nil {
				return nil, fmt.Errorf("bulk write variants: %w", err)
			}

			stockRows := make([]inventory.StockWriteRow, 0, len(variantRows))

			for ref, newVID := range res.IDs {
				sr, ok := refMap[ref]
				if !ok || newVID <= 0 {
					continue
				}
				wasUpdate := sr.variantID > 0
				sr.variantID = newVID
				variantsIdx.remember(sr.row.Payload, *sr.row.ProductID, newVID)
				touchedVariants = append(touchedVariants, newVID)

				outcome := OutcomeInserted
				if wasUpdate {
					outcome = OutcomeUpdated
					updatedCount++
				} else {
					insertedCount++
				}

				vidVal := newVID
				committedOutcomes = append(committedOutcomes, RowOutcome{
					ID:        sr.row.ID,
					Outcome:   outcome,
					VariantID: &vidVal,
					Message:   "تم بنجاح",
				})

				if s.inventory != nil && session.Settings.WarehouseID > 0 &&
					session.Settings.StockMode != inventory.StockKeep &&
					sr.row.Payload != nil && (sr.row.Payload.HasQuantity || sr.row.Payload.MinThreshold > 0) {
					stockRows = append(stockRows, inventory.StockWriteRow{
						Ref:         ref,
						HasQuantity: sr.row.Payload.HasQuantity,
						Stock: &inventory.Stock{
							OrganizationID:   session.OrganizationID,
							WarehouseID:      session.Settings.WarehouseID,
							ProductID:        *sr.row.ProductID,
							ProductVariantID: newVID,
							Quantity:         sr.row.Payload.Quantity,
							MinThreshold:     sr.row.Payload.MinThreshold,
						},
					})
				}
			}

			for _, fail := range res.Failures {
				if sr, ok := refMap[fail.Ref]; ok {
					errorCount++
					committedOutcomes = append(committedOutcomes, RowOutcome{
						ID:        sr.row.ID,
						Outcome:   OutcomeError,
						VariantID: nil,
						Message:   fail.Message,
					})
				}
			}

			if len(stockRows) > 0 {
				_, _ = s.inventory.BulkWriteStocks(ctx, session.Settings.StockMode, stockRows)
			}
		}
	}

	if len(committedOutcomes) > 0 {
		_ = s.imports.UpdateCommittedRows(ctx, session.ID, committedOutcomes)
	}

	session.InsertedRows = insertedCount
	session.UpdatedRows = updatedCount
	session.SkippedRows = skippedCount
	session.ErrorRows = errorCount
	session.Phase = PhaseCompleted
	if err := s.imports.Finish(ctx, session); err != nil {
		return nil, err
	}

	return session, nil
}