package pipeline

// Applying an answer, and refusing one.
//
// This is the file that decides what the model is allowed to change. Its bias is
// stated once and applies throughout: a wrong confident match ships the wrong
// medicine to a patient and gives nobody a signal that anything went wrong,
// while a refused match merely leaves a line for a human to look at. So every
// guard here fails toward the deterministic outcome.
//
// Three of them, in order of how much damage they prevent:
//
//  1. The product must be one the model was actually shown. A product id that
//     was not in the window is a hallucination, and this is what stops it
//     becoming an order.
//  2. The confidence must clear the floor. The prompt asks for abstention below
//     it; this enforces it, because an instruction is not a guarantee.
//  3. The strength must not conflict with the catalogue's own record. The model
//     is instructed at length that strength is decisive and is usually right —
//     but "usually" is not a standard that should decide which medicine a
//     pharmacy receives.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strconv"
	"strings"

	"github.com/muhiya/dawa24-store/internal/modules/smartorder"
)

// apply validates a batch's answers and writes the ones that survive.
//
// Three guards, in order of how much damage they prevent:
//
//  1. The product must be one the model was actually shown. This is what stops a
//     hallucinated id becoming an order.
//  2. The confidence must clear the floor. Below it the answer is remembered but
//     not applied, so the same question is not paid for twice.
//  3. The strength must not conflict. The model is instructed on this and is
//     usually right, but "usually" is not a standard that should govern which
//     medicine a pharmacy receives, so it is re-checked deterministically
//     against the catalogue's own record.
func (e *Enhancement) apply(b plannedBatch, outcomes []EnhanceOutcome) []smartorder.CachedDecision {
	saved := make([]smartorder.CachedDecision, 0, len(outcomes))

	e.mu.Lock()
	defer e.mu.Unlock()

	for _, out := range outcomes {
		group, ok := b.refs[out.Ref]
		if !ok || len(group) == 0 {
			e.Stats.Rejected++
			continue
		}
		lead := group[0]

		if out.Confidence < 0 || out.Confidence > 1 {
			e.Stats.Rejected += len(group)
			continue
		}

		decision := smartorder.CachedDecision{
			Key:           decisionKey(lead),
			NormName:      lead.Line.NormName,
			Confidence:    out.Confidence,
			Reason:        out.Reason,
			PromptVersion: PromptVersion,
		}

		switch {
		case out.ProductID == nil || *out.ProductID <= 0:
			// "None of these" is a real answer and worth remembering: it stops
			// the next import of the same file asking again.
			e.Stats.Abstained += len(group)

		case !inWindow(b.window, *out.ProductID):
			// A product the model was not shown. This is the guard that stops a
			// hallucinated id becoming an order.
			e.Stats.Rejected += len(group)
			continue

		case out.Confidence < MinApplyConfidence:
			// Recorded as an abstention, which is what it is: the model said it
			// was not sure enough, and the lines keep their deterministic
			// outcome.
			e.Stats.Abstained += len(group)

		case e.index.StrengthConflict(lead.Row, *out.ProductID):
			e.Stats.Rejected += len(group)
			continue

		default:
			decision.ChosenProductID = out.ProductID
			for _, r := range group {
				before := r.Line.MatchedProductID
				forceMatch(r.Line, *out.ProductID, smartorder.MethodAI, out.Confidence)
				if before == nil || *before != *out.ProductID {
					e.Stats.Improved++
				} else {
					e.Stats.Confirmed++
				}
			}
		}

		saved = append(saved, decision)
	}
	return saved
}

func inWindow(window map[int64]struct{}, id int64) bool {
	_, ok := window[id]
	return ok
}

// forceMatch records an AI resolution.
//
// Unlike setMatch it does not refuse a lower confidence, because that is exactly
// the case this stage exists for: a line the scorer guessed at 0.42 being
// replaced by a considered answer at 0.88 — or by a *different* product at 0.75,
// which is still better than a guess the engine itself would not stand behind.
// It is only ever called on lines below Cutoff, so a confident deterministic
// result is never in reach (FR-018).
func forceMatch(l *smartorder.Line, productID int64, method smartorder.MatchMethod, confidence float64) {
	l.MatchedProductID = &productID
	l.MatchMethod = method
	l.MatchConfidence = confidence
}

// flush persists the decisions and the aliases they imply.
func (e *Enhancement) flush(ctx context.Context, decisions []smartorder.CachedDecision) {
	if len(decisions) == 0 {
		return
	}
	// A cache write failing must not fail the run: the decisions were still
	// applied, they will simply be paid for again next time.
	_ = e.repo.SaveDecisions(ctx, decisions)

	// Each accepted match is also recorded as an *untrusted* alias, source
	// 'ai_confirmed', which the deterministic alias tier deliberately excludes.
	// The row exists so a buyer accepting the match can promote it and so an
	// operator can see what the model has been deciding — not so the next
	// import trusts it. One confident mistake propagating silently to every
	// pharmacy is precisely what this guards against.
	for _, d := range decisions {
		if d.ChosenProductID == nil || d.NormName == "" {
			continue
		}
		_ = e.repo.SaveAlias(ctx, *d.ChosenProductID, d.NormName, "ai_confirmed", d.Confidence)
	}
}

// applyCache resolves what is already known and returns the remainder.
func (e *Enhancement) applyCache(ctx context.Context, reviews []Review) ([]Review, []smartorder.CachedDecision) {
	keys := make([]string, 0, len(reviews))
	for _, r := range reviews {
		keys = append(keys, decisionKey(r))
	}
	cached, err := e.repo.LookupDecisions(ctx, keys)
	if err != nil {
		cached = nil // a cache miss is never fatal
	}

	pending := make([]Review, 0, len(reviews))
	for _, r := range reviews {
		d, ok := cached[decisionKey(r)]
		if !ok {
			pending = append(pending, r)
			continue
		}
		e.Stats.CacheHits++
		if d.ChosenProductID == nil || d.Confidence < MinApplyConfidence {
			continue
		}
		if e.index.StrengthConflict(r.Row, *d.ChosenProductID) {
			continue
		}
		before := r.Line.MatchedProductID
		forceMatch(r.Line, *d.ChosenProductID, smartorder.MethodAI, d.Confidence)
		if before == nil || *before != *d.ChosenProductID {
			e.Stats.Improved++
		} else {
			e.Stats.Confirmed++
		}
	}
	e.Stats.Reviewed = len(pending)
	return pending, nil
}

// decisionKey identifies the exact question being asked.
//
// The retrieved candidate ids are part of it and are sorted first, so an answer
// is only reused when the same options were on the table; a decision made
// against a different shortlist answers a question nobody asked. PromptVersion
// is included so a prompt change invalidates cleanly rather than silently.
func decisionKey(r Review) string {
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
