package pipeline

// The AI enhancement stage.
//
// It runs after the deterministic engine has finished and seen the whole file,
// and it exists to answer the lines that engine could not: the ones it left
// unmatched (غير مطابق) and the ones it matched only tentatively, below the
// cutoff (مطلوب للمراجعة). Nothing it produces overwrites a confident
// deterministic result, and everything it produces is re-checked before it is
// written.
//
// Why the previous shape did not help, and what changed:
//
//   - It offered the model the five candidates the scorer had picked — the same
//     scorer that had just failed on that row. If the correct product was not in
//     those five, no amount of intelligence could find it, and when the scorer
//     found nothing at all the line was skipped entirely. On live data that was
//     most of the residue. Retrieval is now a separate, recall-tuned pass
//     (productmatch.Recall) that unions token, trigram and molecule strategies,
//     and lines with no shortlist are exactly the ones that need it most.
//   - It sent twenty-five rows per request with the candidates repeated inside
//     each one, so a run of five hundred residual lines was twenty requests, and
//     a ninety-second wall clock cut that off at seven. Candidates are now
//     de-duplicated into ONE catalogue window shared by every item in the
//     request, which is what makes a whole file fit in a single call.
//   - It scored a decision only against that row's own shortlist. The model may
//     now answer with any product in the window, which repairs the commonest
//     retrieval failure of all: the right product was retrieved — for the line
//     above.

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/smartorder"
	"github.com/muhiya/dawa24-store/internal/shared/productmatch"
)

// PromptVersion changes whenever the question being asked changes — the system
// prompt, the rendered input, or the retrieval that fills it. It is part of the
// cache key, so a change orphans old decisions rather than silently reusing
// answers to a different question.
const PromptVersion = "sm-enh-v4"

// Ceilings bound what one run may do.
//
// The Gateway enforces its own budget, but hitting that mid-run surfaces to the
// buyer as an opaque failure. Stopping first, and saying so, is the difference
// between "the system degraded and told me" and "the system broke".
const (
	// RecallLimit is how many catalogue products are retrieved per line.
	//
	// Sixteen rather than a dozen, because the model behind this now has a
	// million-token context and charges three cents a million: the four extra
	// rows cost nothing measurable and buy the cases where the correct product
	// ranks eighth because the pharmacy misspelled the brand. Far above this the
	// list stops being a shortlist and starts being a haystack.
	RecallLimit = 16

	// MaxInputBytes caps one request's rendered input, in BYTES rather than
	// characters — Arabic is two bytes per character in UTF-8, and conflating
	// the two halves the budget without anyone noticing.
	//
	// Measured against the live Gateway, this mixture of Arabic, Latin and
	// digits runs about two bytes to the token: a 290 KB request reported 145k
	// input tokens. The default model's window is 262k tokens, so 400 KB is
	// roughly 200k in — leaving the answer, the system prompt and any
	// Gateway-side overhead a comfortable margin.
	//
	// It is a backstop rather than the binding constraint: the item ceiling
	// below fills about 300 KB at its limit, so this only ever splits a batch
	// whose catalogue window came out unusually wide. That is the right shape —
	// a request should be sized by how many answers it can safely produce, not
	// by how much text it can hold.
	MaxInputBytes = 400_000

	// MaxItemsPerRequest bounds the ANSWER, and that is now what limits a batch.
	//
	// Not the token ceiling — three hundred decisions is only twenty thousand
	// output tokens against a model that allows sixty-six — but LATENCY, which
	// is measured: a 104-item request against the live Gateway returned in
	// thirty-five seconds, and 300-item requests were still generating past
	// ninety. A buyer is waiting on this stage, and a batch that is too large
	// also loses more when it fails, since a failed batch takes every line in it
	// back to the deterministic outcome.
	//
	// Two hundred sits where the whole residue of an ordinary file still fits in
	// one round of concurrent requests without any single one of them running
	// long.
	MaxItemsPerRequest = 200

	// MaxRequestsPerRun is the spend ceiling, and it is the one number here
	// that is a business decision rather than a measurement. Twelve requests at
	// two hundred items cover a file with 2,400 unresolved lines — one where the
	// deterministic engine failed on almost everything. Anything past it keeps
	// its deterministic outcome and is reported as such rather than silently
	// dropped.
	//
	// At the model's published price a full twelve costs under five US cents,
	// so this ceiling is about bounding latency and blast radius, not spend.
	MaxRequestsPerRun = 12

	// MaxConcurrent is what actually determines how long the stage takes, since
	// the total output tokens are the same however the items are divided. Four
	// clears a thousand-line residue in two rounds while staying well inside any
	// sane rate limit.
	MaxConcurrent = 4

	// MaxWallClock bounds the stage. The run itself is allowed twenty minutes,
	// so this leaves ample room for supplier resolution afterwards.
	MaxWallClock = 4 * time.Minute

	// MinApplyConfidence is the floor below which an answer is recorded but not
	// applied. The prompt instructs the model to answer null below the same
	// figure; this enforces it, because an instruction is not a guarantee.
	//
	// Eight tenths, because the model's confidence turns out to be well
	// calibrated and sharply bimodal: on a live 1,004-line residue it answered
	// 0.95 for everything it was sure of, and the handful it scored in the
	// seventies included "ابى ديرم كريم" matched to "هاي ديرم كريم" — two
	// different products sharing only the category suffix ديرم, which no
	// deterministic guard can tell from a brand. The model knew. Taking it at
	// its word costs almost nothing and removes exactly that class of mistake.
	MinApplyConfidence = 0.80
)

// Enhancer resolves a batch of review lines against a catalogue window.
//
// Declared here, in the pipeline's own vocabulary, so this package never imports
// the gateway and every test in it runs without one (AGENTS.md R2, R5).
type Enhancer interface {
	Enhance(ctx context.Context, batch EnhanceBatch) ([]EnhanceOutcome, error)
}

// EnhanceBatch is one request: a de-duplicated catalogue window and the lines to
// resolve against it.
type EnhanceBatch struct {
	Catalog []WindowProduct
	Items   []ReviewLine
}

// WindowProduct is one catalogue product offered to the model.
type WindowProduct struct {
	ProductID     int64
	NameAR        string
	NameEN        string
	Scientific    string
	DosageForm    string
	Concentration string
	Manufacturer  string
}

// ReviewLine is one line needing a second opinion, as the model sees it.
type ReviewLine struct {
	Ref          int
	Text         string
	Brand        string
	Strength     string
	DosageForm   string
	PackSize     int
	Manufacturer string
	Scientific   string
	SKU          string
	Barcode      string
	CurrentGuess *int64
	CurrentScore float64
	Options      []int64
}

// EnhanceOutcome is one decision, keyed by the request-local ref.
type EnhanceOutcome struct {
	Ref        int
	ProductID  *int64
	Confidence float64
	Reason     string
}

// Review is one line carried into the AI stage with its retrieval attached.
type Review struct {
	Line       *smartorder.Line
	Row        *productmatch.Row
	Candidates []productmatch.MatchCandidate
}

// EnhancementStats is what the run records about this stage.
//
// Improved is the number that answers the question the buyer actually has —
// "did the AI do anything for me?" — and it counts lines whose matched product
// changed, not lines the model replied about.
type EnhancementStats struct {
	Reviewed  int
	CacheHits int
	Requests  int
	Improved  int
	Confirmed int
	Abstained int
	Rejected  int
	// RefusedBy counts refusals by conflict kind — strength, modifier, form,
	// evidence. It is the number that says whether the guards are protecting the
	// buyer or fighting the model, and it is the first thing to look at when
	// this stage stops helping.
	RefusedBy  map[string]int
	CeilingHit bool
}

// Enhancement runs the AI stage under its ceilings.
type Enhancement struct {
	repo  smartorder.Repository
	ai    Enhancer
	index *productmatch.Index
	log   *slog.Logger
	now   func() time.Time

	requests int64
	settled  int64

	// OnProgress, when set, receives the running count of lines settled. It is
	// the only thing that moves the bar during the one stage that waits on a
	// network, and it is called from several goroutines.
	OnProgress func(done int)

	mu        sync.Mutex
	persistMu sync.Mutex
	Stats     EnhancementStats
}

// NewEnhancement constructs the AI stage. A nil logger is allowed; refusals are
// then simply not recorded.
func NewEnhancement(repo smartorder.Repository, ai Enhancer, index *productmatch.Index,
	log *slog.Logger) *Enhancement {
	return &Enhancement{
		repo:  repo,
		ai:    ai,
		index: index,
		log:   log,
		now:   time.Now,
		Stats: EnhancementStats{RefusedBy: map[string]int{}},
	}
}

// Run improves what it can and leaves the rest as the deterministic engine found
// it.
//
// It never returns an error that fails the run. Every failure path — Gateway
// down, budget exhausted, malformed response — degrades to the deterministic
// outcome, because a pharmacy must be able to order when the AI is unavailable.
func (e *Enhancement) Run(ctx context.Context, reviews []Review) {
	if e.ai == nil || e.index == nil || len(reviews) == 0 {
		return
	}

	deadline := e.now().Add(MaxWallClock)
	// The wall clock is enforced on the calls themselves, not merely checked
	// between them: a check in the dispatch loop passes for every batch and
	// then lets slow requests run past the limit unbounded.
	runCtx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()

	// The cache answers first, so a remembered line never enters a request.
	pending := e.applyCache(ctx, reviews)
	if len(pending) == 0 {
		return
	}

	batches := e.plan(pending)

	var (
		wg    sync.WaitGroup
		slots = make(chan struct{}, MaxConcurrent)
	)

	for _, batch := range batches {
		wg.Add(1)
		slots <- struct{}{}
		go func(b plannedBatch) {
			defer wg.Done()
			defer func() { <-slots }()
			// A panic inside one batch must not take the buyer's whole import
			// down with it.
			defer func() {
				if rec := recover(); rec != nil {
					e.count(func(s *EnhancementStats) { s.Rejected += b.lines })
				}
			}()
			// The batch is accounted whatever happens to it: a failed batch
			// still leaves its lines settled, with their deterministic outcome,
			// and a bar that stalls on failure reads as a hung run.
			defer e.report(b.lines)

			atomic.AddInt64(&e.requests, 1)
			outcomes, err := e.ai.Enhance(runCtx, b.request)
			if err != nil {
				if errors.Is(err, context.DeadlineExceeded) {
					e.count(func(s *EnhancementStats) { s.CeilingHit = true })
				}
				return
			}

			saved := e.apply(b, outcomes)
			e.flush(runCtx, saved)
		}(batch)
	}

	wg.Wait()

	e.Stats.Requests = int(atomic.LoadInt64(&e.requests))
}

// report advances the progress callback by one batch.
func (e *Enhancement) report(n int) {
	if e.OnProgress == nil {
		return
	}
	e.OnProgress(int(atomic.AddInt64(&e.settled, int64(n))))
}

// count mutates the stats under the same lock apply uses.
func (e *Enhancement) count(fn func(*EnhancementStats)) {
	e.mu.Lock()
	defer e.mu.Unlock()
	fn(&e.Stats)
}
