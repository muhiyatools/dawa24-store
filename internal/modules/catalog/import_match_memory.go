package catalog

// The master-catalogue import's half of the shared decision cache.
//
// This importer was the last one paying for every question on every upload. An
// administrator re-uploads the same registry extract with a few hundred rows
// changed, and the adjudication tier asked the model about the whole residue
// again — the same rows, the same shortlists, the same prompt, the same answers
// the vendor import and the smart order had already bought and filed.
//
// It reads and writes catalog.match_decisions through the shared port, keyed by
// matchflow.DecisionKey, so the four import paths are now genuinely one cache
// rather than two tools sharing one and two tools ignoring it.
//
// The one asymmetry is deliberate: this importer applies at 0.90 where the
// others apply at 0.80, because a wrong match here overwrites the entry every
// pharmacy reads. A remembered answer is held to the same floor as a fresh one,
// so an answer bought by a vendor's import at 0.85 is READ here and refused
// here — which is correct, and is not the same as not reading it.

import (
	"context"

	"github.com/muhiya/dawa24-store/internal/shared/matchflow"
	"github.com/muhiya/dawa24-store/internal/shared/productmatch"
)

// SetMatchMemory attaches the shared decision cache. Nil is allowed and means
// every question is paid for again.
func (s *Service) SetMatchMemory(m matchflow.Memory) { s.matchMemory = m }

// matchQuestionKey identifies the question one pending row asks: this text,
// against these candidates, under this prompt.
func matchQuestionKey(normName string, candidates []productmatch.MatchCandidate) string {
	ids := make([]int64, 0, len(candidates))
	for _, c := range candidates {
		ids = append(ids, c.ProductID)
	}
	return matchflow.DecisionKey(normName, ids)
}

// applyMatchMemory settles what the cache already knows and returns the rows
// still worth asking about.
//
// Every guard a fresh answer passes, a remembered one passes too: the product
// must still be on this row's shortlist, the confidence must still clear this
// importer's floor, and the catalogue's own record must still agree. The
// catalogue moves between imports and a decision that was sound in March can
// conflict in June.
func (s *Service) applyMatchMemory(
	ctx context.Context,
	index *productmatch.Index,
	prods []*Product,
	matches map[int]ExistingMatch,
	pending []pendingMatch,
	stats *MatchStats,
) []pendingMatch {
	if s.matchMemory == nil || len(pending) == 0 {
		return pending
	}

	keys := make([]string, 0, len(pending))
	keyOf := make(map[int]string, len(pending))
	for _, p := range pending {
		key := matchQuestionKey(matchRowNorm(prods[p.index]), p.candidates)
		keyOf[p.index] = key
		keys = append(keys, key)
	}

	known := matchflow.Recall(ctx, s.matchMemory, keys)
	if len(known) == 0 {
		return pending
	}

	rest := make([]pendingMatch, 0, len(pending))
	for _, p := range pending {
		d, ok := known[keyOf[p.index]]
		if !ok {
			rest = append(rest, p)
			continue
		}
		stats.CacheHits++
		if d.ChosenProductID == nil || d.Confidence < aiFloor {
			// "None of these", or an answer too hesitant for this importer.
			// Either way the row keeps its deterministic outcome and is not
			// re-asked: the model was asked and declined.
			continue
		}
		id := *d.ChosenProductID
		if !inShortlist(p.candidates, id) {
			rest = append(rest, p)
			continue
		}
		if !index.IdentityConflict(matchRowFor(prods[p.index]), id).None() {
			continue
		}
		matches[p.index] = ExistingMatch{ProductID: id, Reason: MatchAI}
		stats.AI++
	}
	return rest
}

// rememberMatchDecisions files what the model decided, for every tool that asks
// the same question next.
//
// Only answers at or above matchflow.MinMemoryConfidence are written: the cache
// is shared and long-lived, and an entry made from a hesitant answer becomes a
// standing fact nobody is shown the provenance of.
func (s *Service) rememberMatchDecisions(ctx context.Context, decisions []matchflow.Remembered) {
	if s.matchMemory == nil || len(decisions) == 0 {
		return
	}
	// A cache write failing must not fail the import: the decisions were still
	// applied, they will simply be paid for again next time.
	_ = s.matchMemory.Save(ctx, decisions)
	for _, d := range decisions {
		if d.ChosenProductID == nil || d.NormName == "" {
			continue
		}
		_ = s.matchMemory.SaveAlias(ctx, *d.ChosenProductID, d.NormName, "ai_confirmed", d.Confidence)
	}
}

// matchRowNorm is the row's normalised name — the cache's alias key and part of
// its decision key. It is the same normaliser every other tool keys on, which
// is what makes the four caches one cache.
func matchRowNorm(p *Product) string {
	if p == nil {
		return ""
	}
	return productmatch.NormalizeText(matchRowFor(p).DisplayName())
}
