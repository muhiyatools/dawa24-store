package ingest

// Applying an answer, and refusing one.
//
// This is the file that decides what the model is allowed to change. Its bias
// is stated once and applies throughout: a wrong confident match ties a
// vendor's price and stock to the wrong medicine — a pharmacy then orders it
// believing the catalogue — while a refused match merely leaves a row on the
// review screen for a person to look at. So every guard here fails toward the
// deterministic outcome.
//
// Three of them, in order of how much damage they prevent:
//
//  1. The product must be one the model was actually shown. A product id that
//     was not in the window is a hallucination, and this is what stops it
//     becoming a live price.
//  2. The confidence must clear the floor. The prompt asks for abstention below
//     it; this enforces it, because an instruction is not a guarantee.
//  3. The product must survive productmatch.IdentityConflict — the strength,
//     the line-extension word, the dosage form and the shared distinctive word
//     all re-checked against the catalogue's own record. The model is
//     instructed at length on every one of these and is usually right, but
//     "usually" is not a standard that should decide what a pharmacy receives.
//
// Nothing here writes to the database. The stage hands back accepted matches
// and the caller applies them to the staged rows in one statement, which is
// what keeps a nine-thousand-row import to a fixed number of round trips.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strconv"
	"strings"

	"github.com/muhiya/dawa24-store/internal/shared/productmatch"
)

// aiReason renders the model's short justification for the review screen.
const aiReasonPrefix = "مطابقة بالذكاء الاصطناعي: "

// apply validates a batch's answers and records the ones that survive.
//
// An answer the guards refuse is deliberately NOT written to the decision
// cache. Caching it would save a request next time and would also freeze a
// judgement the guards may later make differently — the modifier vocabulary
// grows, the catalogue gains a product — and a cached wrong premise is worse
// than a repeated question. Only what the model actually decided is remembered.
func (e *Enhancement) apply(b plannedBatch, outcomes []EnhanceOutcome) []CachedDecision {
	saved := make([]CachedDecision, 0, len(outcomes))

	e.mu.Lock()
	defer e.mu.Unlock()

	for _, out := range outcomes {
		r, ok := b.refs[out.Ref]
		if !ok {
			e.Stats.Rejected++
			continue
		}
		rows := len(r.sourceRows)

		if out.Confidence < 0 || out.Confidence > 1 {
			e.Stats.Rejected += rows
			continue
		}

		decision := CachedDecision{
			Key:           decisionKey(r),
			NormName:      r.normName,
			Confidence:    out.Confidence,
			Reason:        out.Reason,
			PromptVersion: PromptVersion,
		}

		switch {
		case out.ProductID == nil || *out.ProductID <= 0:
			// "None of these" is a real answer and worth remembering: it stops
			// the next upload of the same price list asking again.
			e.Stats.Abstained += rows

		case !inWindow(b.window, *out.ProductID):
			// A product the model was not shown. This is the guard that stops a
			// hallucinated id becoming a live price.
			e.Stats.Rejected += rows
			continue

		case out.Confidence < MinApplyConfidence:
			// Recorded as an abstention, which is what it is: the model said it
			// was not sure enough, and the rows keep their deterministic
			// outcome.
			e.Stats.Abstained += rows

		default:
			if c := e.index.IdentityConflict(r.row, *out.ProductID); !c.None() {
				e.refuse(c, r, *out.ProductID, rows)
				continue
			}
			decision.ChosenProductID = out.ProductID
			e.resolve(r, *out.ProductID, out.Confidence, out.Reason)
		}

		saved = append(saved, decision)
	}
	return saved
}

// resolve records an accepted answer on the row that asked.
//
// Unlike the deterministic path it does not refuse a lower score, because that
// is exactly the case this stage exists for: a row the scorer guessed at 0.42
// being settled by a considered answer at 0.88 — or by a *different* product at
// 0.85, which is still better than a guess the engine itself would not stand
// behind. Only rows the engine left unsettled ever reach here, so a confident
// deterministic result is never in reach.
func (e *Enhancement) resolve(r *openRow, productID int64, confidence float64, reason string) {
	if r.answer != nil {
		return
	}
	r.answer = &aiAnswer{
		ProductID: productID,
		Score:     confidence,
		Reason:    aiReason(reason),
	}
	e.Stats.Improved += len(r.sourceRows)
}

func aiReason(reason string) string {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return "مطابقة بالذكاء الاصطناعي بين المرشحين"
	}
	return aiReasonPrefix + reason
}

// refuse records a rejected decision and says why.
//
// The reason is logged rather than shown: a vendor does not need to know that
// the model proposed something and was overruled, but an operator asking "is
// the AI stage helping or fighting the matcher?" needs the answer, and it is
// not recoverable after the fact.
//
// It touches Stats directly rather than through count(), because every caller
// already holds e.mu.
func (e *Enhancement) refuse(c productmatch.MatchConflict, r *openRow, productID int64, rows int) {
	e.Stats.Rejected += rows
	if e.Stats.RefusedBy == nil {
		e.Stats.RefusedBy = map[string]int{}
	}
	e.Stats.RefusedBy[c.Kind]++
	if e.log != nil {
		e.log.Debug("AI match refused by the identity guard",
			"source_row", r.firstRow(), "text", r.row.DisplayName(),
			"product_id", productID, "conflict", c.Kind, "detail", c.Detail)
	}
}

func inWindow(window map[int64]struct{}, id int64) bool {
	_, ok := window[id]
	return ok
}

// applyCache resolves what is already known and returns the remainder.
//
// This is the cheapest tier there is and the reason a vendor's weekly re-upload
// costs almost nothing: the cache is shared across every feature that asks this
// question, so an answer bought once is reused wherever the same product is
// written the same way against the same shortlist.
func (e *Enhancement) applyCache(ctx context.Context, rows []*openRow) []*openRow {
	if e.memory == nil {
		e.Stats.Reviewed = countRows(rows)
		return rows
	}

	keys := make([]string, 0, len(rows))
	for _, r := range rows {
		keys = append(keys, decisionKey(r))
	}
	cached, err := e.memory.LookupDecisions(ctx, keys)
	if err != nil {
		cached = nil // a cache miss is never fatal
	}

	pending := make([]*openRow, 0, len(rows))
	for _, r := range rows {
		d, ok := cached[decisionKey(r)]
		if !ok {
			pending = append(pending, r)
			continue
		}
		e.Stats.CacheHits += len(r.sourceRows)
		if d.ChosenProductID == nil || d.Confidence < MinApplyConfidence {
			continue
		}
		// The guard runs on a remembered answer too. The catalogue moves
		// between imports and a decision that was sound in March can conflict
		// in June.
		if c := e.index.IdentityConflict(r.row, *d.ChosenProductID); !c.None() {
			e.refuse(c, r, *d.ChosenProductID, len(r.sourceRows))
			continue
		}
		e.resolve(r, *d.ChosenProductID, d.Confidence, d.Reason)
	}
	e.Stats.Reviewed = countRows(pending)
	return pending
}

// countRows totals the spreadsheet rows behind a set of questions.
func countRows(rows []*openRow) int {
	n := 0
	for _, r := range rows {
		n += len(r.sourceRows)
	}
	return n
}

// flush persists the decisions and the aliases they imply.
func (e *Enhancement) flush(ctx context.Context, decisions []CachedDecision) {
	if e.memory == nil || len(decisions) == 0 {
		return
	}
	// A cache write failing must not fail the run: the decisions were still
	// applied, they will simply be paid for again next time.
	_ = e.memory.SaveDecisions(ctx, decisions)

	// Each accepted match is also recorded as an *untrusted* alias, source
	// 'ai_confirmed', which the deterministic alias tier deliberately excludes.
	// The row exists so a person accepting the match can promote it and so an
	// operator can see what the model has been deciding — not so the next
	// import trusts it. One confident mistake propagating silently to every
	// vendor is precisely what this guards against.
	for _, d := range decisions {
		if d.ChosenProductID == nil || d.NormName == "" {
			continue
		}
		_ = e.memory.SaveAlias(ctx, *d.ChosenProductID, d.NormName, "ai_confirmed", d.Confidence)
	}
}

// decisionKey identifies the exact question being asked.
//
// The retrieved candidate ids are part of it and are sorted first, so an answer
// is only reused when the same options were on the table; a decision made
// against a different shortlist answers a question nobody asked. PromptVersion
// is included so a prompt change invalidates cleanly rather than silently.
//
// It is byte-identical to the smart order's key for the same question, which is
// what lets the two features share one cache.
func decisionKey(r *openRow) string {
	ids := make([]int64, 0, len(r.candidates))
	for _, c := range r.candidates {
		ids = append(ids, c.ProductID)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })

	var b strings.Builder
	b.WriteString(r.normName)
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
