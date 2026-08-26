package pipeline

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
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
	repo smartorder.Repository
	ai   Adjudicator
	now  func() time.Time
	// requests counts calls actually issued. It is separate from Stats because
	// batches run concurrently and this one field is written from several
	// goroutines; Run folds it back into Stats before returning.
	requests int64
	Stats    AdjudicationStats
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

	// The wall clock is enforced on the calls themselves, not merely checked
	// between them. Batches run concurrently now, so a check in the dispatch
	// loop would pass for every batch and then let three slow requests run past
	// the limit unbounded; a deadline on the context cuts them off, and a cut
	// off batch degrades to its deterministic outcome like any other failure.
	adjCtx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()

	// Cache first, so a cached line never enters a request.
	items := a.applyCache(ctx, residual)
	if len(items) == 0 {
		return
	}

	var toSave []smartorder.CachedDecision
	byLine := indexResiduals(residual)

	// Batches run MaxConcurrent at a time.
	//
	// They were sequential, and on a file that reaches the AI tier with a few
	// hundred rows that mattered: at a couple of seconds a request, the ninety
	// second wall clock expired around the thirtieth batch and everything after
	// it was reported as "limit reached" — a ceiling the run hit by waiting
	// rather than by spending. Nothing here shares state across batches except
	// the counters and the result slice, both of which are taken under the
	// mutex, so the concurrency is bounded and the ordering of results does not
	// matter.
	batches := a.plan(items, deadline)
	var (
		mu sync.Mutex
		wg sync.WaitGroup
	)
	slots := make(chan struct{}, MaxConcurrent)

	for _, batch := range batches {
		wg.Add(1)
		slots <- struct{}{}
		go func(batch []AdjudicationItem) {
			defer wg.Done()
			defer func() { <-slots }()

			// A panic inside one batch must not take the run — and with it the
			// buyer's whole import — down with it.
			defer func() {
				if rec := recover(); rec != nil {
					mu.Lock()
					a.Stats.Rejected += len(batch)
					mu.Unlock()
				}
			}()

			results, err := a.adjudicateWithBisection(adjCtx, batch)
			if err != nil {
				// A whole batch failing is not a run failure: those lines keep
				// the deterministic outcome they already have. A deadline
				// expiry arrives here too, and is reported as a ceiling.
				if errors.Is(err, context.DeadlineExceeded) {
					mu.Lock()
					a.Stats.CeilingHit = true
					mu.Unlock()
				}
				return
			}

			mu.Lock()
			defer mu.Unlock()
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
		}(batch)
	}
	wg.Wait()

	a.Stats.Requests = int(atomic.LoadInt64(&a.requests))

	if len(toSave) > 0 {
		// A cache write failing must not fail the run: the decisions were still
		// applied, they will simply be paid for again next time.
		_ = a.repo.SaveDecisions(ctx, toSave)
		a.recordAliases(ctx, toSave)
	}
}

// plan slices the items into batches, up to the run's request budget.
//
// The budget is spent here rather than inside the loop that issues the calls,
// so concurrent workers cannot race past it: whatever this returns is exactly
// what will be asked, and everything it leaves out keeps its deterministic
// outcome and is reported honestly as unresolved.
func (a *Adjudication) plan(items []AdjudicationItem, deadline time.Time) [][]AdjudicationItem {
	var batches [][]AdjudicationItem
	for start := 0; start < len(items); start += MaxItemsPerRequest {
		// Reserve two requests of headroom for bisection, so a batch that fails
		// can still be retried at half size within the budget.
		if len(batches)+2 > MaxRequestsPerRun || a.now().After(deadline) {
			a.Stats.CeilingHit = true
			break
		}
		end := start + MaxItemsPerRequest
		if end > len(items) {
			end = len(items)
		}
		batches = append(batches, items[start:end])
	}
	return batches
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
