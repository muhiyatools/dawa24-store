package ingest

// Committing: writing what the vendor reviewed and accepted.
//
// Nothing here matches anything. Every row arriving at this stage already
// carries the catalogue product it resolved to — deterministically, by the AI
// stage, or by the vendor's own hand on the review screen — and this file's
// only job is to turn that into the vendor's variants and warehouse balances
// in as few statements as possible.

import (
	"context"
	"fmt"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/modules/inventory"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

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
			i18n.TDefault("w4_mod.w4str_190_190"))
	}
	if session.Settings.WarehouseID <= 0 {
		return nil, apperr.Validation("import.warehouse_required",
			i18n.TDefault("w4_mod.w4str_191_191"), nil)
	}
	if !s.runs.claim(publicID) {
		return nil, apperr.Conflict("import.running", i18n.TDefault("w4_mod.w4str_192_192"))
	}
	defer s.runs.release(publicID)

	stagedRows, err := s.imports.StagedRowsForCommit(ctx, session.ID)
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
					Message:   i18n.TDefault("w4_mod.s_379_379"),
				})
				continue
			}

			pID := *sr.ProductID
			vID, _ := variantsIdx.resolve(sr.Payload, pID)

			// Apply ModeAddOnly: existing variants must NOT be updated
			if session.Settings.Mode == ModeAddOnly && vID > 0 {
				skippedCount++
				committedOutcomes = append(committedOutcomes, RowOutcome{
					ID:        sr.ID,
					Outcome:   OutcomeSkipped,
					VariantID: &vID,
					Message:   i18n.TDefault("w4_mod.w4str_209_209"),
				})
				continue
			}

			// Apply ModeUpdateOnly: new variants must NOT be added
			if session.Settings.Mode == ModeUpdateOnly && vID == 0 {
				skippedCount++
				committedOutcomes = append(committedOutcomes, RowOutcome{
					ID:        sr.ID,
					Outcome:   OutcomeSkipped,
					VariantID: nil,
					Message:   i18n.TDefault("w4_mod.w4str_208_208"),
				})
				continue
			}

			varName := sr.EffectiveVariantName()
			status := catalog.StatusActive

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
			}
			if !sr.Payload.CostPrice.IsZero() {
				cp := sr.Payload.CostPrice
				v.CostPrice = &cp
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
					Message:   i18n.TDefault("w4_mod.s_380_380"),
				})

				if s.inventory != nil && session.Settings.WarehouseID > 0 && sr.row.Payload != nil {
					hasQty := sr.row.Payload.HasQuantity
					qty := sr.row.Payload.Quantity
					if !hasQty && !wasUpdate && session.Settings.BlankQuantityIsZero {
						hasQty = true
						qty = 0
					}
					// Only queue if stock mode is not StockKeep OR if it's a new variant needing initial warehouse stock
					if session.Settings.StockMode != inventory.StockKeep || !wasUpdate || hasQty {
						stockRows = append(stockRows, inventory.StockWriteRow{
							Ref:         ref,
							HasQuantity: hasQty,
							Stock: &inventory.Stock{
								OrganizationID:   session.OrganizationID,
								WarehouseID:      session.Settings.WarehouseID,
								ProductID:        *sr.row.ProductID,
								ProductVariantID: newVID,
								Quantity:         qty,
								MinThreshold:     sr.row.Payload.MinThreshold,
							},
						})
					}
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
				stockRes, err := s.inventory.BulkWriteStocks(ctx, session.Settings.StockMode, stockRows)
				if err != nil {
					s.log.ErrorContext(ctx, "bulk write stocks failed", "import", session.PublicID, "error", err)
				} else if len(stockRes.Failures) > 0 {
					for _, f := range stockRes.Failures {
						s.log.WarnContext(ctx, "stock write row failure", "import", session.PublicID, "ref", f.Ref, "message", f.Message)
					}
				}
			}
		}
	}

	// In ModeReplace: retire any active variants belonging to this vendor that were not touched by this import
	if session.Settings.Mode == ModeReplace && errorCount == 0 && session.ErrorRows == 0 {
		retired, err := s.catalog.DeactivateVariantsExcept(ctx, session.OrganizationID, touchedVariants)
		if err != nil {
			s.log.WarnContext(ctx, "replace-mode variant retirement failed", "import", session.PublicID, "error", err)
		} else if retired > 0 {
			s.log.InfoContext(ctx, "replace-mode variants retired", "import", session.PublicID, "retired", retired)
		}
	}

	if len(committedOutcomes) > 0 {
		_ = s.imports.UpdateCommittedRows(ctx, session.ID, committedOutcomes)
	}

	session.InsertedRows = insertedCount
	session.UpdatedRows = updatedCount
	session.SkippedRows = skippedCount + held
	session.ErrorRows = errorCount
	session.Phase = PhaseCompleted
	if err := s.imports.Finish(ctx, session); err != nil {
		return nil, err
	}

	return session, nil
}
