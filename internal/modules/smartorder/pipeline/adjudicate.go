package pipeline

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/smartorder"
)

// AI adjudication — the last tier, and the smallest.
//
// Everything the deterministic engine settled is already gone by the time this
// runs. What reaches here is the long tail: abbreviations, misspellings, brand
// against generic. Three rules govern it, and all three exist because the
// obvious implementation is ruinous at ten thousand rows:
//
//   - **Never per row.** Lines are batched, and each batch carries its own
//     shortlist. A model asked to resolve one row at a time turns a three-minute
//     run into an hour and a weekly budget into an afternoon.
//   - **Never with database access.** The batch carries everything the decision
//     needs. A model that can query the catalogue will, repeatedly.
//   - **Never twice.** Every decision is cached on the text plus the exact
//     candidate set, so the second import of a recurring file asks almost
//     nothing.
//
// And one that exists because of what this system orders: a result naming a
// product that was not among the candidates is rejected outright.

// PromptVersion changes whenever the adjudication prompt changes. It is part of
// the cache key, so a prompt change orphans old decisions instead of silently
// reusing answers to a different question.
const PromptVersion = "sm-v1"

// Ceilings bound the work a single run may do.
//
// The Gateway enforces its own budget, but hitting that mid-run surfaces to the
// buyer as an opaque failure. Stopping first, and saying so, is the difference
// between "the system degraded and told me" and "the system broke".
const (
	MaxItemsPerRequest   = 25
	MaxCandidatesPerItem = 5
	MaxRequestsPerRun    = 40
	MaxConcurrent        = 3
	MaxWallClock         = 90 * time.Second
)

// Adjudicator resolves residual lines by choosing among supplied candidates.
type Adjudicator interface {
	Adjudicate(ctx context.Context, items []AdjudicationItem) ([]AdjudicationResult, error)
}

// AdjudicationItem is one line and its shortlist. Nothing else is sent.
type AdjudicationItem struct {
	LineID     int64
	RawText    string
	NormText   string
	Candidates []CandidateSummary
}

// CandidateSummary is a catalogue product as the adjudicator sees it.
type CandidateSummary struct {
	ProductID     int64
	NameAR        string
	NameEN        string
	Scientific    string
	DosageForm    string
	Concentration string
	Manufacturer  string
}

// AdjudicationResult is one decision. A nil ProductID means "none of these",
// which is a legitimate and useful answer.
type AdjudicationResult struct {
	LineID     int64
	ProductID  *int64
	Confidence float64
	Reason     string
}

// Adjudication runs the AI tier under its ceilings.
type Adjudication struct {
	repo  smartorder.Repository
	ai    Adjudicator
	now   func() time.Time
	Stats AdjudicationStats
}

// AdjudicationStats is what the run records about this tier.
type AdjudicationStats struct {
	Requests    int
	CacheHits   int
	Adjudicated int
	Rejected    int
	CeilingHit  bool
}

// NewAdjudication constructs the AI tier.
func NewAdjudication(repo smartorder.Repository, ai Adjudicator) *Adjudication {
	return &Adjudication{repo: repo, ai: ai, now: time.Now}
}

// Run resolves what it can and leaves the rest as the deterministic engine found
// it.
//
// It never returns an error that fails the run. Every failure path — gateway
// down, budget exhausted, malformed response — degrades to the deterministic
// outcome, because a pharmacy must be able to order when the AI is unavailable.
func (a *Adjudication) Run(ctx context.Context, residual []Residual) {
	if a.ai == nil || len(residual) == 0 {
		return
	}

	deadline := a.now().Add(MaxWallClock)

	// Cache first, so a cached line never enters a request.
	items := a.applyCache(ctx, residual)
	if len(items) == 0 {
		return
	}

	var toSave []smartorder.CachedDecision
	byLine := indexResiduals(residual)

	for start := 0; start < len(items); start += MaxItemsPerRequest {
		if a.Stats.Requests >= MaxRequestsPerRun || a.now().After(deadline) {
			// Everything left keeps its deterministic outcome and is reported
			// as unresolved. The buyer is not made to wait on a budget.
			a.Stats.CeilingHit = true
			break
		}
		end := start + MaxItemsPerRequest
		if end > len(items) {
			end = len(items)
		}
		batch := items[start:end]

		results, err := a.adjudicateWithBisection(ctx, batch)
		if err != nil {
			// A whole batch failing is not a run failure.
			continue
		}
		for _, res := range results {
			r, ok := byLine[res.LineID]
			if !ok {
				a.Stats.Rejected++
				continue
			}
			if !a.accept(r, res) {
				a.Stats.Rejected++
				continue
			}
			a.Stats.Adjudicated++
			toSave = append(toSave, smartorder.CachedDecision{
				Key:             decisionKey(r),
				NormName:        r.Line.NormName,
				ChosenProductID: res.ProductID,
				Confidence:      res.Confidence,
				Reason:          res.Reason,
				PromptVersion:   PromptVersion,
			})
		}
	}

	if len(toSave) > 0 {
		// A cache write failing must not fail the run: the decisions were still
		// applied, they will simply be paid for again next time.
		_ = a.repo.SaveDecisions(ctx, toSave)
		a.recordAliases(ctx, toSave)
	}
}

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
	a.Stats.Requests++
	results, err := a.ai.Adjudicate(ctx, batch)
	if err == nil {
		return results, nil
	}
	if len(batch) < 2 || a.Stats.Requests+2 > MaxRequestsPerRun {
		return nil, err
	}

	mid := len(batch) / 2
	var combined []AdjudicationResult
	for _, half := range [][]AdjudicationItem{batch[:mid], batch[mid:]} {
		a.Stats.Requests++
		if res, err := a.ai.Adjudicate(ctx, half); err == nil {
			combined = append(combined, res...)
		}
	}
	if len(combined) == 0 {
		return nil, err
	}
	return combined, nil
}

// accept validates a result before it is allowed to change anything.
//
// FR-020: a product that was not among the candidates supplied for that line is
// rejected and the line keeps its deterministic outcome. This is the guard that
// stops a hallucinated product id becoming an order.
func (a *Adjudication) accept(r Residual, res AdjudicationResult) bool {
	if res.Confidence < 0 || res.Confidence > 1 {
		return false
	}
	if res.ProductID == nil {
		return true // "none of these" is a valid answer; nothing changes
	}
	if !inCandidates(r, *res.ProductID) {
		return false
	}
	setMatch(r.Line, *res.ProductID, smartorder.MethodAI, res.Confidence)
	return true
}

func inCandidates(r Residual, productID int64) bool {
	for _, c := range r.Candidates {
		if c.ProductID == productID {
			return true
		}
	}
	return false
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

// recordAliases stores each AI decision as an *untrusted* alias.
//
// It is written with source 'ai_confirmed', which the deterministic alias tier
// deliberately excludes. The row exists so that a buyer accepting the match can
// promote it, and so an operator can see what the model has been deciding — not
// so the next import trusts it. One confident mistake propagating silently to
// every pharmacy is precisely the failure this guards against.
func (a *Adjudication) recordAliases(ctx context.Context, decisions []smartorder.CachedDecision) {
	for _, d := range decisions {
		if d.ChosenProductID == nil || d.NormName == "" {
			continue
		}
		_ = a.repo.SaveAlias(ctx, *d.ChosenProductID, d.NormName, "ai_confirmed", d.Confidence)
	}
}
