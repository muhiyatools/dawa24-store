package smartorder

import (
	"context"

	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// CorrectMatch applies a buyer's correction, updates candidates and selections, and remembers it.
func (s *Service) CorrectMatch(ctx context.Context, orgID, lineID, productID int64) (*Line, error) {
	line, err := s.repo.GetLine(ctx, orgID, lineID)
	if err != nil {
		return nil, err
	}

	if productID <= 0 {
		line.MatchedProductID = nil
		line.MatchMethod = MethodNone
		line.MatchConfidence = 0.0
		line.CorrectedByUser = true
		line.Outcome = OutcomeUnmatched
		line.OutcomeReason = ""
		if err := s.repo.UpdateLines(ctx, []*Line{line}); err != nil {
			return nil, err
		}
		_ = s.repo.DeleteSelection(ctx, orgID, lineID)
		_ = s.repo.ReplaceCandidates(ctx, lineID, nil)
		s.recomputeRunStats(ctx, orgID, line.RunID)
		return line, nil
	}

	line.MatchedProductID = &productID
	line.MatchMethod = MethodManual
	line.MatchConfidence = 1.0
	line.CorrectedByUser = true

	// Load offers for this newly matched product and evaluate candidates
	offers, err := s.repo.LoadOffers(ctx, orgID, []int64{productID})
	if err == nil && len(offers) > 0 {
		candidates := make([]Candidate, 0, len(offers))
		for _, o := range offers {
			p := money.FromMinor(o.PriceMinor)
			discount := p.ApplyPercent(o.DiscountBps)
			net, _ := p.Sub(discount)
			eligible, reason := Evaluate(OfferCheck{
				BuyerOrgID:             orgID,
				VendorOrgID:            o.VendorOrgID,
				ProductActive:          o.ProductActive && o.VendorActive,
				InstitutionallyVisible: true,
				Covered:                true,
				StockQty:               o.StockQty,
				RequestedQty:           line.EffectiveQty,
				MinOrderQty:            o.MinOrderQty,
			})
			candidates = append(candidates, Candidate{
				LineID:           lineID,
				OrganizationID:   orgID,
				VendorOrgID:      o.VendorOrgID,
				VariantID:        o.VariantID,
				BranchID:         o.BranchID,
				Price:            p,
				DiscountBps:      o.DiscountBps,
				NetUnitPrice:     net,
				Unit:             o.Unit,
				MinOrderQty:      o.MinOrderQty,
				StockQty:         o.StockQty,
				IsFollowed:       o.IsFollowed,
				Eligible:         eligible,
				IneligibleReason: reason,
			})
		}
		_ = s.repo.ReplaceCandidates(ctx, lineID, candidates)
		outcome, reason := OutcomeFor(true, line.EffectiveQty, candidates)
		line.Outcome = outcome
		line.OutcomeReason = string(reason)

		// Pick supplier if candidates written and config available
		cfg, _ := s.repo.GetConfig(ctx, line.RunID)
		writtenCandidates, _ := s.repo.ListCandidates(ctx, orgID, lineID)
		if sel, ok := Select(cfg, line.ID, writtenCandidates); ok {
			for _, c := range writtenCandidates {
				if c.ID == sel.CandidateID {
					sel.LineNet, _ = LineNet(c.NetUnitPrice, line.EffectiveQty)
					break
				}
			}
			_ = s.repo.UpsertSelections(ctx, []*Selection{sel})
		}
	} else {
		line.Outcome = OutcomeNoSupplier
		line.OutcomeReason = string(ReasonInactive)
		_ = s.repo.DeleteSelection(ctx, orgID, lineID)
		_ = s.repo.ReplaceCandidates(ctx, lineID, nil)
	}

	if err := s.repo.UpdateLines(ctx, []*Line{line}); err != nil {
		return nil, err
	}
	if err := s.repo.SaveLearnedMapping(ctx, orgID, line.RawName, productID); err != nil {
		s.log.WarnContext(ctx, "could not remember match correction",
			"line_id", lineID, "error", err)
	}

	source := "manual"
	if line.MatchMethod == MethodAI {
		source = "ai_confirmed"
	}
	if err := s.repo.SaveAlias(ctx, productID, line.NormName, source, 1.0); err != nil {
		s.log.WarnContext(ctx, "could not record product alias",
			"line_id", lineID, "error", err)
	}

	s.recomputeRunStats(ctx, orgID, line.RunID)
	return line, nil
}

func (s *Service) recomputeRunStats(ctx context.Context, orgID, runID int64) {
	lines, _, err := s.repo.ListLines(ctx, runID, LineFilter{All: true})
	if err != nil {
		return
	}
	var stats Stats
	stats.TotalRows = len(lines)
	for _, l := range lines {
		switch l.Outcome {
		case OutcomeOrdered:
			stats.MatchedRows++
		case OutcomeUnmatched:
			stats.UnmatchedRows++
		case OutcomeNoSupplier:
			stats.NoSupplierRows++
		case OutcomeCoverageBlocked:
			stats.CoverageBlockedRows++
		case OutcomeInstitutionalBlocked:
			stats.InstitutionalBlockedRows++
		case OutcomeBelowMinQty:
			stats.BelowMinQtyRows++
		default:
			if !l.Matched() {
				stats.UnmatchedRows++
			}
		}
	}
	run, err := s.repo.GetRunByID(ctx, orgID, runID)
	if err == nil && run != nil {
		run.Stats = stats
		_ = s.repo.UpdateRunStats(ctx, run)
	}
}
