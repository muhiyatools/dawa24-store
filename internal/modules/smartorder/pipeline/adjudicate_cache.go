package pipeline

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/muhiya/dawa24-store/internal/modules/smartorder"
)

// The decision cache, and the request it stops from being made.
//
// Every adjudication is remembered against the exact question it answered: the
// normalised text, the exact candidate set, and the prompt version. A pharmacy
// that imports the same list every Sunday pays for it once. Reusing a decision
// made against a *different* shortlist would be answering a question nobody
// asked, which is why the candidate ids are part of the key.

// applyCache resolves what the cache already knows and returns the remainder.
func (a *Adjudication) applyCache(ctx context.Context, residual []Residual) []AdjudicationItem {
	keys := make([]string, 0, len(residual))
	for _, r := range residual {
		keys = append(keys, decisionKey(r))
	}
	cached, err := a.repo.LookupDecisions(ctx, keys)
	if err != nil {
		cached = nil // a cache miss is never fatal
	}

	items := make([]AdjudicationItem, 0, len(residual))
	for _, r := range residual {
		if d, ok := cached[decisionKey(r)]; ok {
			a.Stats.CacheHits++
			if d.ChosenProductID != nil && inCandidates(r, *d.ChosenProductID) {
				setMatch(r.Line, *d.ChosenProductID, smartorder.MethodAI, d.Confidence)
			}
			continue
		}
		if len(r.Candidates) == 0 {
			// Nothing to choose between. Sending it would ask the model to
			// invent a product, which is exactly what must not happen.
			continue
		}
		items = append(items, buildItem(r))
	}
	return items
}

// adjudicateWithBisection retries a failed batch once at half size.
//
// A batch usually fails because one item confused the model or the response
// exceeded a limit. Halving isolates the problem and salvages the other half.
func (a *Adjudication) adjudicateWithBisection(ctx context.Context, batch []AdjudicationItem) ([]AdjudicationResult, error) {
	// Batches run concurrently, so the request counter is atomic. It is
	// telemetry rather than a gate — the budget was already spent in plan —
	// but a torn count is a lie in the run's own record of what it did.
	atomic.AddInt64(&a.requests, 1)
	results, err := a.ai.Adjudicate(ctx, batch)
	if err == nil {
		return results, nil
	}
	if len(batch) < 2 || int(atomic.LoadInt64(&a.requests))+2 > MaxRequestsPerRun {
		return nil, err
	}

	mid := len(batch) / 2
	var combined []AdjudicationResult
	for _, half := range [][]AdjudicationItem{batch[:mid], batch[mid:]} {
		atomic.AddInt64(&a.requests, 1)
		if res, err := a.ai.Adjudicate(ctx, half); err == nil {
			combined = append(combined, res...)
		}
	}
	if len(combined) == 0 {
		return nil, err
	}
	return combined, nil
}

func buildItem(r Residual) AdjudicationItem {
	candidates := r.Candidates
	if len(candidates) > MaxCandidatesPerItem {
		candidates = candidates[:MaxCandidatesPerItem]
	}
	summaries := make([]CandidateSummary, 0, len(candidates))
	for _, c := range candidates {
		summaries = append(summaries, CandidateSummary{
			ProductID:     c.ProductID,
			NameAR:        c.Name,
			Scientific:    c.Scientific,
			DosageForm:    c.DosageForm,
			Concentration: c.Concentration,
			Manufacturer:  c.Manufacturer,
		})
	}
	return AdjudicationItem{
		LineID:     r.Line.ID,
		RawText:    r.Line.RawName,
		NormText:   r.Line.NormName,
		Candidates: summaries,
	}
}

// decisionKey identifies the exact question being asked.
//
// The candidate ids are part of the key and are sorted first, so a cached answer
// is only reused when the same shortlist is on the table. Reusing a decision
// made against a different candidate set would be answering a question nobody
// asked. PromptVersion is included so a prompt change invalidates cleanly.
func decisionKey(r Residual) string {
	ids := make([]int64, 0, len(r.Candidates))
	for _, c := range r.Candidates {
		ids = append(ids, c.ProductID)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })

	var b strings.Builder
	b.WriteString(r.Line.NormName)
	b.WriteByte('\x1f')
	for _, id := range ids {
		b.WriteString(strconv.FormatInt(id, 10))
		b.WriteByte(',')
	}
	b.WriteByte('\x1f')
	b.WriteString(PromptVersion)

	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:])
}

func indexResiduals(residual []Residual) map[int64]Residual {
	out := make(map[int64]Residual, len(residual))
	for _, r := range residual {
		out[r.Line.ID] = r
	}
	return out
}
