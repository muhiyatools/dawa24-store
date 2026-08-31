package compare

// The compare tool's AI matching tier.
//
// It is the stage the vendor import and the smart order run, reached through
// the same port with the same request shape, the same catalogue window, the
// same ceilings and the same decision cache. Nothing about the question differs
// — "which product in this window is the medicine this line names, or none of
// them?" — so nothing about the prompt does either.
//
// Every failure here is silent by design. The user keeps a completely,
// deterministically matched file, which is a usable result they can correct by
// hand; an import that fails because a model was unavailable is not.

import (
	"context"

	"github.com/muhiya/dawa24-store/internal/shared/matchflow"
	"github.com/muhiya/dawa24-store/internal/shared/productmatch"
)

// enhanceMatches asks the model about the rows the engine left open and applies
// the answers that survive every guard.
func (s *Service) enhanceMatches(
	ctx context.Context,
	index *productmatch.Index,
	pending []*openRow,
	updates *[]RowMatch,
	stats *MatchStats,
) {
	recall := productmatch.DefaultRecallOptions()
	recall.Limit = matchCeilings.RecallLimit

	// Retrieval first, and the plausibility gate with it. A row whose best
	// retrieved candidate is a coincidence has no answer for a model to choose
	// between, and i18n.TDefault("w4_mod.s_364_364") is already the honest outcome — on a live file
	// that is most of the residue, and sending it anyway is what used to spend
	// a run's whole budget to improve a handful of rows.
	askable := make([]*openRow, 0, len(pending))
	for _, p := range pending {
		p.candidates = index.Recall(p.query, recall)
		for _, c := range p.candidates {
			if c.Score >= matchCeilings.MinPlausible {
				askable = append(askable, p)
				break
			}
		}
	}
	if len(askable) == 0 {
		return
	}

	askable = s.applyMatchCache(ctx, index, askable, updates, stats)

	for start := 0; start < len(askable); start += matchCeilings.MaxItemsPerRequest {
		if stats.Requests >= matchCeilings.MaxRequestsPerRun {
			stats.CeilingHit = true
			return
		}
		end := start + matchCeilings.MaxItemsPerRequest
		if end > len(askable) {
			end = len(askable)
		}
		batch := askable[start:end]

		req, window := buildMatchBatch(index, batch)
		stats.Requests++
		decisions, err := s.enhancer.Enhance(ctx, req)
		if err != nil {
			// The deterministic outcome stands for this batch.
			s.log.WarnContext(ctx, "compare AI matching batch failed; deterministic outcome stands",
				"items", len(batch), "error", err)
			continue
		}

		var remembered []matchflow.Remembered
		for _, d := range decisions {
			if d.Ref < 0 || d.Ref >= len(batch) {
				continue
			}
			p := batch[d.Ref]
			if d.Confidence < 0 || d.Confidence > 1 {
				continue
			}
			if d.Confidence >= matchflow.MinMemoryConfidence {
				remembered = append(remembered, matchflow.Remembered{
					Key:             matchKeyOf(p),
					NormName:        productmatch.NormalizeText(p.row.RawName),
					ChosenProductID: d.ProductID,
					Confidence:      d.Confidence,
					Reason:          d.Reason,
					PromptVersion:   matchflow.PromptVersion,
				})
			}
			if !acceptable(index, p, window, d.ProductID, d.Confidence) {
				continue
			}
			applyAIMatch(p, *d.ProductID, d.Confidence, updates, stats)
		}
		s.rememberMatches(ctx, remembered)
	}
}

// applyMatchCache resolves what the shared cache already knows and returns the
// rows still worth asking about.
func (s *Service) applyMatchCache(
	ctx context.Context,
	index *productmatch.Index,
	rows []*openRow,
	updates *[]RowMatch,
	stats *MatchStats,
) []*openRow {
	if s.memory == nil {
		return rows
	}
	keys := make([]string, 0, len(rows))
	for _, p := range rows {
		keys = append(keys, matchKeyOf(p))
	}
	known := matchflow.Recall(ctx, s.memory, keys)
	if len(known) == 0 {
		return rows
	}

	pending := make([]*openRow, 0, len(rows))
	for _, p := range rows {
		d, ok := known[matchKeyOf(p)]
		if !ok {
			pending = append(pending, p)
			continue
		}
		stats.CacheHits++
		if d.ChosenProductID == nil || d.Confidence < matchCeilings.MinApplyConfidence {
			continue
		}
		// The guard runs on a remembered answer too: the catalogue moves
		// between runs and a decision that was sound in March can conflict in
		// June.
		if !index.IdentityConflict(p.query, *d.ChosenProductID).None() {
			continue
		}
		applyAIMatch(p, *d.ChosenProductID, d.Confidence, updates, stats)
	}
	return pending
}

// acceptable applies the three guards an answer must survive: it must name a
// product the model was actually shown, it must clear the confidence floor, and
// the catalogue's own record must agree that it could be this product.
func acceptable(
	index *productmatch.Index, p *openRow,
	window map[int64]struct{}, productID *int64, confidence float64,
) bool {
	if productID == nil || *productID <= 0 {
		return false
	}
	if _, shown := window[*productID]; !shown {
		return false
	}
	if confidence < matchCeilings.MinApplyConfidence {
		return false
	}
	return index.IdentityConflict(p.query, *productID).None()
}

func applyAIMatch(p *openRow, productID int64, confidence float64, updates *[]RowMatch, stats *MatchStats) {
	id := productID
	*updates = append(*updates, RowMatch{
		RowID: p.row.ID, ProductID: &id,
		Method: MatchMethodAI, Confidence: confidence * 100,
	})
	stats.AI++
	if stats.Review > 0 {
		stats.Review--
	} else if stats.Unmatched > 0 {
		stats.Unmatched--
	}
}

// buildMatchBatch renders one request: a de-duplicated catalogue window every
// item may be answered from, and the items themselves.
func buildMatchBatch(index *productmatch.Index, rows []*openRow) (matchflow.Batch, map[int64]struct{}) {
	batch := matchflow.Batch{
		Feature: matchflow.FeatureCompareTool,
		Items:   make([]matchflow.Item, 0, len(rows)),
	}
	window := make(map[int64]struct{}, len(rows)*4)
	for ref, p := range rows {
		item := matchflow.Item{Ref: ref, Text: p.row.RawName, SKU: p.row.SKU}
		for _, c := range p.candidates {
			item.Options = append(item.Options, c.ProductID)
			if _, seen := window[c.ProductID]; seen {
				continue
			}
			window[c.ProductID] = struct{}{}
			if m, ok := index.Lookup(c.ProductID); ok {
				batch.Catalog = append(batch.Catalog, matchflow.CatalogEntry{
					ProductID: m.ID, NameAR: m.NameAR, NameEN: m.NameEN,
					Scientific: m.Scientific, DosageForm: m.DosageForm,
					Concentration: m.Concentration, Manufacturer: m.Manufacturer,
				})
			}
		}
		batch.Items = append(batch.Items, item)
	}
	return batch, window
}

// rememberMatches files the run's decisions in the shared cache.
func (s *Service) rememberMatches(ctx context.Context, decisions []matchflow.Remembered) {
	if s.memory == nil || len(decisions) == 0 {
		return
	}
	_ = s.memory.Save(ctx, decisions)
	for _, d := range decisions {
		if d.ChosenProductID == nil || d.NormName == "" {
			continue
		}
		_ = s.memory.SaveAlias(ctx, *d.ChosenProductID, d.NormName, "ai_confirmed", d.Confidence)
	}
}

// matchKeyOf identifies the exact question one row asks: this text, against
// these candidates, under this prompt. Byte-identical to the key every other
// import tool computes, which is what makes the cache one cache.
func matchKeyOf(p *openRow) string {
	ids := make([]int64, 0, len(p.candidates))
	for _, c := range p.candidates {
		ids = append(ids, c.ProductID)
	}
	return matchflow.DecisionKey(productmatch.NormalizeText(p.row.RawName), ids)
}
