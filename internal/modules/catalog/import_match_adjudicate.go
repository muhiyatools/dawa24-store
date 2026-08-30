package catalog

// Asking the model about the rows similarity could not settle.
//
// It runs last, over the residue the exact and similarity tiers left, in
// batches, and it applies only answers that name a candidate the model was
// actually shown. Everything it decides is filed in the shared decision cache,
// so an administrator re-uploading the same registry extract next month pays
// for the rows that changed and nothing else.

import (
	"context"

	"github.com/muhiya/dawa24-store/internal/shared/matchflow"
)

// adjudicateMatches asks the model about the rows similarity could not settle,
// in batches, and applies only the answers that name a candidate it was given.
func (s *Service) adjudicateMatches(
	ctx context.Context,
	session *ImportSession,
	prods []*Product,
	matches map[int]ExistingMatch,
	forAI []pendingMatch,
	stats *MatchStats,
) {
	items := make([]MatchAdjudicationItem, 0, len(forAI))
	byRef := make(map[int64]pendingMatch, len(forAI))
	for _, p := range forAI {
		ref := int64(p.index)
		byRef[ref] = p
		items = append(items, MatchAdjudicationItem{
			Ref:        ref,
			Text:       adjudicationText(prods[p.index]),
			Candidates: summarizeCandidates(p.candidates),
		})
	}

	decided := map[int64]bool{}
	var remembered []matchflow.Remembered
	for start := 0; start < len(items); start += aiBatchSize {
		if stats.AIRequests >= maxAIAdjudicationBatches {
			stats.CeilingHit = true
			return
		}
		end := start + aiBatchSize
		if end > len(items) {
			end = len(items)
		}

		stats.AIRequests++
		session.AICalls++
		req := MatchAdjudicationRequest{
			Items:          items[start:end],
			OrganizationID: session.OrganizationID,
		}
		if session.CreatedBy != nil {
			req.UserID = *session.CreatedBy
		}
		results, err := s.adjudicator.AdjudicateMatches(ctx, req)
		if err != nil {
			// One failed batch is not a failed import: those rows keep the
			// deterministic outcome, which is "this is a new product".
			session.AIFallback = true
			s.log.WarnContext(ctx, "match adjudication batch failed; deterministic outcome stands",
				"session", session.PublicID, "items", end-start, "error", err)
			continue
		}
		for _, res := range results {
			p, known := byRef[res.Ref]
			if !known || decided[res.Ref] {
				continue
			}
			decided[res.Ref] = true

			// Remembered before it is judged, and remembered whatever it said:
			// a confident "none of these" is as reusable as a confident match,
			// and it is what stops the next upload asking again. Hesitant
			// answers are not remembered at all — see
			// matchflow.MinMemoryConfidence.
			if res.Confidence >= matchflow.MinMemoryConfidence &&
				res.Confidence <= 1 && res.Confidence >= 0 {
				remembered = append(remembered, matchflow.Remembered{
					Key:             matchQuestionKey(matchRowNorm(prods[p.index]), p.candidates),
					NormName:        matchRowNorm(prods[p.index]),
					ChosenProductID: res.ProductID,
					Confidence:      res.Confidence,
					Reason:          res.Reason,
					PromptVersion:   matchflow.PromptVersion,
				})
			}

			if res.ProductID == nil || res.Confidence < aiFloor {
				continue
			}
			// A product that was not on the shortlist for that row is rejected
			// outright. This is the guard that stops an invented id becoming an
			// update to a real catalogue entry.
			if !inShortlist(p.candidates, *res.ProductID) {
				continue
			}
			matches[p.index] = ExistingMatch{ProductID: *res.ProductID, Reason: MatchAI}
			stats.AI++
			session.AIApplied++
		}
	}
	s.rememberMatchDecisions(ctx, remembered)
}
