package ingest

// The AI enhancement stage of the vendor catalogue import.
//
// It is the same stage the smart order pipeline runs, asking the same question
// through the same prompt, and that is deliberate rather than convenient. Both
// features have one job here — "which product in this window is the medicine
// this line names, or none of them?" — and a second prompt would drift from the
// first the day either was tuned. Sharing it also shares the decision cache:
// an answer paid for by a pharmacy's order is free to the vendor whose file
// asks the same question, and the reverse.
//
// What it replaces, and why the old shape was expensive:
//
//   - It sent twenty-five rows per request with every row's candidates repeated
//     inside it. Supplier files are long and repetitive — the same twenty
//     antihypertensives are retrieved for every antihypertensive row — so that
//     repetition was most of the payload. Candidates are now de-duplicated into
//     ONE catalogue window that every item in the request references by id.
//   - It ran inside the row-streaming callback, once per five hundred rows, so
//     nothing could be shared or de-duplicated across the file. It now runs once
//     over the whole residue, which is what makes the window and the collapsing
//     of duplicate rows actually pay.
//   - It never consulted a cache. Vendors re-upload the same price list weekly
//     with a dozen rows changed; every re-upload was paid for in full.
//   - It skipped any row the scorer found nothing for — on live files most of
//     the residue, and exactly the rows that need a second opinion most.
//     Retrieval is now a separate, recall-tuned pass.
//   - It applied any answer above 0.70 that named an offered id. A wrong
//     confident match ties a vendor's price to the wrong medicine, so an answer
//     is now re-checked against the catalogue's own record before it is applied.

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/muhiya/dawa24-store/internal/shared/matchflow"
	"github.com/muhiya/dawa24-store/internal/shared/productmatch"
)

// PromptVersion is the version of the question being asked, and it is part of
// the decision-cache key.
//
// It is the shared one. It used to be declared here, and it had drifted from
// the copy the other importer declared — so the two filed answers to the same
// question under different keys and neither ever read the other's. A build gate
// now fails if the literal is restated outside internal/shared/matchflow.
const PromptVersion = matchflow.PromptVersion

// ceilings are what one run may spend, from the shared table.
//
// They used to be a const block here and another in the other importer, with
// every number documented as measured and no two of them agreeing.
var ceilings = matchflow.For(matchflow.ProfileVendor)

// Enhancer resolves a batch of unsettled rows against a catalogue window.
//
// Declared here, in the module's own vocabulary, so this package never imports
// the gateway and every test in it runs without one.
type Enhancer interface {
	Enhance(ctx context.Context, batch EnhanceBatch) ([]EnhanceOutcome, error)
}

// EnhanceBatch is one request: a de-duplicated catalogue window and the rows to
// resolve against it.
type EnhanceBatch = matchflow.Batch

// WindowProduct is one catalogue product offered to the model.
type WindowProduct = matchflow.CatalogEntry

// ReviewLine is one row needing a second opinion, as the model sees it.
type ReviewLine = matchflow.Item

// EnhanceOutcome is one decision, keyed by the request-local ref.
type EnhanceOutcome = matchflow.Decision

// CachedDecision is one remembered answer, shared with every other feature that
// asks this question.
type CachedDecision struct {
	Key             string
	NormName        string
	ChosenProductID *int64
	Confidence      float64
	Reason          string
	PromptVersion   string
}

// MatchMemory is the decision cache and the alias ledger.
//
// A nil implementation is allowed throughout: the stage then pays for every
// question, which is slower and dearer but never wrong.
type MatchMemory interface {
	LookupDecisions(ctx context.Context, keys []string) (map[string]CachedDecision, error)
	SaveDecisions(ctx context.Context, decisions []CachedDecision) error
	SaveAlias(ctx context.Context, productID int64, alias, source string, confidence float64) error
}

// openRow is ONE QUESTION, not one spreadsheet row.
//
// Supplier files repeat themselves — the same product listed per warehouse, per
// batch, per pack — and every repetition asks the identical question. They are
// collapsed onto one openRow before retrieval runs, so a file that names
// i18n.TDefault("w4_mod.s_381_381") forty times retrieves once, asks once, and pays once. The
// rows that asked are carried in SourceRows and all receive the answer.
type openRow struct {
	// row is the parsed spreadsheet row this question was built from, which the
	// identity guard re-reads.
	row *productmatch.Row
	// sourceRows are the row numbers as the vendor sees them in Excel's gutter.
	// They are how an accepted answer finds its staged rows again, and they are
	// never sent to a model.
	sourceRows []int
	// normName is the row's normalised name: the dedup key, the cache's alias
	// key, and part of the decision key.
	normName string
	// guess and score are the deterministic engine's own best attempt, offered
	// to the model as a suggestion it may confirm or overrule.
	guess      *int64
	score      float64
	candidates []productmatch.MatchCandidate
	// answer is set when a decision survived every guard.
	answer *aiAnswer
}

// aiAnswer is one accepted decision, before it is spread over the rows that
// asked for it.
type aiAnswer struct {
	ProductID int64
	Score     float64
	Reason    string
}

// AIMatch is an accepted answer bound to one staged row.
type AIMatch struct {
	SourceRow int
	ProductID int64
	Score     float64
	Reason    string
}

// AIStats is what the run records about this stage, and what the review screen
// shows the vendor.
//
// Improved answers the only question a vendor actually has — "did this do
// anything for me?" — and counts rows whose match changed, not rows the model
// replied about.
type AIStats struct {
	Reviewed   int            `json:"reviewed"`
	CacheHits  int            `json:"cache_hits"`
	Requests   int            `json:"requests"`
	Improved   int            `json:"improved"`
	Abstained  int            `json:"abstained"`
	Rejected   int            `json:"rejected"`
	RefusedBy  map[string]int `json:"refused_by,omitempty"`
	CeilingHit bool           `json:"ceiling_hit"`
	// Skipped counts rows retrieval could find no plausible candidate for, so
	// they were never sent. It is reported rather than hidden: a vendor looking
	// at a low match rate needs to know the difference between "the model was
	// asked and could not answer" and "the catalogue does not carry this".
	Skipped int `json:"skipped"`
	// Ran distinguishes "the stage did nothing because there was nothing to do"
	// from "the stage never ran", which are different things to tell a vendor.
	Ran bool `json:"ran"`
}

// Used reports whether the stage did any work worth showing.
func (s AIStats) Used() bool { return s.Ran && (s.Reviewed > 0 || s.CacheHits > 0) }

// Enhancement runs the AI stage under its ceilings.
type Enhancement struct {
	ai     Enhancer
	memory MatchMemory
	index  *productmatch.Index
	log    *slog.Logger
	now    func() time.Time

	requests int64
	settled  int64

	// OnProgress, when set, receives the running count of rows settled. It is
	// the only thing that moves the bar during the one stage that waits on a
	// network, and it is called from several goroutines.
	OnProgress func(done, total int)

	mu    sync.Mutex
	Stats AIStats
}

// NewEnhancement constructs the AI stage. A nil memory or logger is allowed.
func NewEnhancement(ai Enhancer, memory MatchMemory, index *productmatch.Index,
	log *slog.Logger) *Enhancement {
	return &Enhancement{
		ai: ai, memory: memory, index: index, log: log, now: time.Now,
		Stats: AIStats{RefusedBy: map[string]int{}},
	}
}

// Retrieve builds the shortlist for one unsettled row.
//
// It is a separate, recall-tuned pass rather than a reuse of the scorer's own
// candidates, because by the time a row reaches here the scorer has already
// decided it cannot answer — and handing the model the top rows of the pool
// that defeated it asks a question the shortlist has answered wrongly. It also
// finds something for the rows the scorer found nothing for at all, which on a
// live supplier file is most of the residue.
//
// Nothing here calls a model; it is index arithmetic and costs no budget.
// It also decides which rows are worth asking about at all, which is the single
// largest saving available in this stage. A row whose best retrieved candidate
// is implausible has no answer for the model to choose from: the product is not
// in the shared catalogue, and i18n.TDefault("w4_mod.s_382_382") is already the honest outcome. On a
// live smart order of the same shape — 8,790 rows of cosmetics against a
// pharmaceutical catalogue — sending them anyway cost thirty requests, the
// ceiling, and five and a half minutes to improve 156 rows out of 7,939.
//
// The rows are returned rather than filtered in place, so the staging table
// still carries every one of them for the vendor to review by hand.
func (e *Enhancement) Retrieve(rows []*openRow) []*openRow {
	opts := productmatch.DefaultRecallOptions()
	opts.Limit = ceilings.RecallLimit
	askable := make([]*openRow, 0, len(rows))
	for _, r := range rows {
		r.candidates = e.index.Recall(r.row, opts)
		if plausible(r.candidates, ceilings.MinPlausible) {
			askable = append(askable, r)
		}
	}
	e.count(func(s *AIStats) { s.Skipped = len(rows) - len(askable) })
	return askable
}

// plausible reports whether retrieval found anything worth a second opinion.
//
// The best candidate decides it. A shortlist whose strongest member is a
// coincidence of one shared word is not a shortlist, and offering it to a model
// asks a question whose honest answer is the one already recorded.
func plausible(candidates []productmatch.MatchCandidate, floor float64) bool {
	for _, c := range candidates {
		if c.Score >= floor {
			return true
		}
	}
	return false
}

// Run improves what it can and leaves the rest as the deterministic engine
// found it.
//
// It never returns an error. Every failure path — Gateway down, budget spent,
// malformed response — degrades to the deterministic outcome, because a vendor
// must be able to import their catalogue when the AI is unavailable.
func (e *Enhancement) Run(ctx context.Context, rows []*openRow) []AIMatch {
	if e.ai == nil || e.index == nil || len(rows) == 0 {
		return nil
	}
	e.Stats.Ran = true

	// The wall clock is enforced on the calls themselves, not merely checked
	// between them: a check in the dispatch loop passes for every batch and
	// then lets slow requests run past the limit unbounded.
	runCtx, cancel := context.WithDeadline(ctx, e.now().Add(ceilings.MaxWallClock))
	defer cancel()

	// The cache answers first, so a remembered row never enters a request.
	pending := e.applyCache(ctx, rows)
	if len(pending) == 0 {
		return collect(rows)
	}

	batches := e.plan(pending)
	total := 0
	for _, b := range batches {
		total += b.rows
	}

	var (
		wg     sync.WaitGroup
		slots  = make(chan struct{}, ceilings.MaxConcurrent)
		saveMu sync.Mutex
		toSave []CachedDecision
	)
	for _, batch := range batches {
		wg.Add(1)
		slots <- struct{}{}
		go func(b plannedBatch) {
			defer wg.Done()
			defer func() { <-slots }()
			// A panic inside one batch must not take the vendor's whole import
			// down with it.
			defer func() {
				if rec := recover(); rec != nil {
					e.count(func(s *AIStats) { s.Rejected += b.rows })
				}
			}()
			// The batch is accounted whatever happens to it: a failed batch
			// still leaves its rows settled, with their deterministic outcome,
			// and a bar that stalls on failure reads as a hung run.
			defer e.report(b.rows, total)

			atomic.AddInt64(&e.requests, 1)
			outcomes, err := e.ai.Enhance(runCtx, b.request)
			if err != nil {
				if errors.Is(err, context.DeadlineExceeded) {
					e.count(func(s *AIStats) { s.CeilingHit = true })
				}
				return
			}
			if saved := e.apply(b, outcomes); len(saved) > 0 {
				saveMu.Lock()
				toSave = append(toSave, saved...)
				saveMu.Unlock()
			}
		}(batch)
	}
	wg.Wait()

	e.Stats.Requests = int(atomic.LoadInt64(&e.requests))
	e.flush(ctx, toSave)
	return collect(rows)
}

// collect spreads each accepted answer over every staged row that asked for it.
func collect(rows []*openRow) []AIMatch {
	out := make([]AIMatch, 0, len(rows))
	for _, r := range rows {
		if r.answer == nil {
			continue
		}
		for _, n := range r.sourceRows {
			out = append(out, AIMatch{
				SourceRow: n,
				ProductID: r.answer.ProductID,
				Score:     r.answer.Score,
				Reason:    r.answer.Reason,
			})
		}
	}
	return out
}

// report advances the progress callback by one batch.
func (e *Enhancement) report(n, total int) {
	if e.OnProgress == nil {
		return
	}
	e.OnProgress(int(atomic.AddInt64(&e.settled, int64(n))), total)
}

// count mutates the stats under the same lock apply uses.
func (e *Enhancement) count(fn func(*AIStats)) {
	e.mu.Lock()
	defer e.mu.Unlock()
	fn(&e.Stats)
}
