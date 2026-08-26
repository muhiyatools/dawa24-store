package pipeline

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/smartorder"
)

// fakeRepo implements only the methods adjudication touches.
type fakeRepo struct {
	smartorder.Repository
	cached  map[string]smartorder.CachedDecision
	saved   []smartorder.CachedDecision
	aliases []aliasWrite
	lookups int
}

// aliasWrite records what the adjudicator asked to remember.
type aliasWrite struct {
	productID int64
	alias     string
	source    string
}

func (f *fakeRepo) LookupDecisions(_ context.Context, keys []string) (map[string]smartorder.CachedDecision, error) {
	f.lookups++
	out := make(map[string]smartorder.CachedDecision)
	for _, k := range keys {
		if d, ok := f.cached[k]; ok {
			out[k] = d
		}
	}
	return out, nil
}

func (f *fakeRepo) SaveDecisions(_ context.Context, d []smartorder.CachedDecision) error {
	f.saved = append(f.saved, d...)
	return nil
}

func (f *fakeRepo) SaveAlias(_ context.Context, productID int64, alias, source string, _ float64) error {
	f.aliases = append(f.aliases, aliasWrite{productID: productID, alias: alias, source: source})
	return nil
}

// fakeAI records what it was asked and returns scripted answers.
//
// The recording is under a mutex because batches are adjudicated concurrently:
// a double that races is a double that reports the wrong call count, which is
// exactly what these tests assert on.
type fakeAI struct {
	mu        sync.Mutex
	calls     int
	batchSize []int
	respond   func(items []AdjudicationItem) ([]AdjudicationResult, error)
}

func (f *fakeAI) Adjudicate(_ context.Context, items []AdjudicationItem) ([]AdjudicationResult, error) {
	f.mu.Lock()
	f.calls++
	f.batchSize = append(f.batchSize, len(items))
	respond := f.respond
	f.mu.Unlock()

	if respond != nil {
		return respond(items)
	}
	return nil, nil
}

// callCount reads the recorded number of requests safely.
func (f *fakeAI) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

// batchSizes returns a copy of the recorded batch sizes.
func (f *fakeAI) batchSizes() []int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]int(nil), f.batchSize...)
}

func residualOf(lineID int64, name string, candidateIDs ...int64) Residual {
	l := &smartorder.Line{ID: lineID, RawName: name, NormName: name}
	r := Residual{Line: l}
	for _, id := range candidateIDs {
		r.Candidates = append(r.Candidates, matchCandidate(id, name))
	}
	return r
}

// SC-012 / AGENTS.md R3. This is the merge gate: a pharmacy must be able to
// order when the AI is unavailable.
func TestGatewayDisabledLeavesRunUsable(t *testing.T) {
	repo := &fakeRepo{}
	a := NewAdjudication(repo, nil) // nil adjudicator == gateway disabled

	residual := []Residual{residualOf(1, "بانادول", 10, 11)}
	a.Run(context.Background(), residual)

	if a.Stats.Requests != 0 {
		t.Fatal("no request may be made when the gateway is disabled")
	}
	if residual[0].Line.Matched() {
		t.Fatal("a disabled gateway must not invent a match")
	}
}

func TestAdjudicatorErrorDoesNotFailTheRun(t *testing.T) {
	repo := &fakeRepo{}
	ai := &fakeAI{respond: func([]AdjudicationItem) ([]AdjudicationResult, error) {
		return nil, errors.New("gateway unreachable")
	}}
	a := NewAdjudication(repo, ai)

	residual := []Residual{residualOf(1, "بانادول", 10, 11)}
	a.Run(context.Background(), residual) // must not panic or block

	if residual[0].Line.Matched() {
		t.Fatal("a failed adjudication must leave the deterministic outcome alone")
	}
}

// FR-020. The guard that stops a hallucinated product id becoming an order.
func TestResultOutsideCandidateListIsRejected(t *testing.T) {
	repo := &fakeRepo{}
	rogue := int64(999)
	ai := &fakeAI{respond: func(items []AdjudicationItem) ([]AdjudicationResult, error) {
		return []AdjudicationResult{{LineID: items[0].LineID, ProductID: &rogue, Confidence: 0.99}}, nil
	}}
	a := NewAdjudication(repo, ai)

	residual := []Residual{residualOf(1, "بانادول", 10, 11)}
	a.Run(context.Background(), residual)

	if residual[0].Line.Matched() {
		t.Fatalf("product 999 was not among the candidates and must be rejected")
	}
	if a.Stats.Rejected != 1 {
		t.Fatalf("expected the rejection to be counted, got %d", a.Stats.Rejected)
	}
}

func TestResultInsideCandidateListIsApplied(t *testing.T) {
	repo := &fakeRepo{}
	chosen := int64(11)
	ai := &fakeAI{respond: func(items []AdjudicationItem) ([]AdjudicationResult, error) {
		return []AdjudicationResult{{LineID: items[0].LineID, ProductID: &chosen, Confidence: 0.93}}, nil
	}}
	a := NewAdjudication(repo, ai)

	residual := []Residual{residualOf(1, "بانادول", 10, 11)}
	a.Run(context.Background(), residual)

	l := residual[0].Line
	if !l.Matched() || *l.MatchedProductID != 11 {
		t.Fatalf("expected product 11, got %v", l.MatchedProductID)
	}
	if l.MatchMethod != smartorder.MethodAI {
		t.Fatalf("an AI decision must be labelled as such, got %s", l.MatchMethod)
	}
	if len(repo.saved) != 1 {
		t.Fatalf("the decision must be cached, got %d saved", len(repo.saved))
	}
}

func TestOutOfRangeConfidenceIsRejected(t *testing.T) {
	repo := &fakeRepo{}
	chosen := int64(11)
	ai := &fakeAI{respond: func(items []AdjudicationItem) ([]AdjudicationResult, error) {
		return []AdjudicationResult{{LineID: items[0].LineID, ProductID: &chosen, Confidence: 5}}, nil
	}}
	a := NewAdjudication(repo, ai)
	residual := []Residual{residualOf(1, "بانادول", 10, 11)}
	a.Run(context.Background(), residual)

	if residual[0].Line.Matched() {
		t.Fatal("a confidence outside [0,1] is a malformed response")
	}
}

// FR-018c: batched, never one row at a time.
func TestLinesAreBatchedNotSentIndividually(t *testing.T) {
	repo := &fakeRepo{}
	ai := &fakeAI{respond: func(items []AdjudicationItem) ([]AdjudicationResult, error) {
		return nil, nil
	}}
	a := NewAdjudication(repo, ai)

	var residual []Residual
	for i := int64(1); i <= 60; i++ {
		residual = append(residual, residualOf(i, "منتج", 10, 11))
	}
	a.Run(context.Background(), residual)

	if ai.callCount() > 4 {
		t.Fatalf("60 lines should be a handful of requests, got %d", ai.callCount())
	}
	for _, size := range ai.batchSizes() {
		if size > MaxItemsPerRequest {
			t.Fatalf("batch of %d exceeds the cap of %d", size, MaxItemsPerRequest)
		}
	}
}

// SC-007c: the second import of an unchanged file must cost almost nothing.
func TestCachedDecisionsSkipTheGatewayEntirely(t *testing.T) {
	residual := []Residual{residualOf(1, "بانادول", 10, 11)}
	chosen := int64(11)
	repo := &fakeRepo{cached: map[string]smartorder.CachedDecision{
		decisionKey(residual[0]): {ChosenProductID: &chosen, Confidence: 0.93},
	}}
	ai := &fakeAI{}
	a := NewAdjudication(repo, ai)

	a.Run(context.Background(), residual)

	if ai.callCount() != 0 {
		t.Fatalf("a cached line must never reach the gateway, got %d calls", ai.callCount())
	}
	if a.Stats.CacheHits != 1 {
		t.Fatalf("expected 1 cache hit, got %d", a.Stats.CacheHits)
	}
	if !residual[0].Line.Matched() {
		t.Fatal("the cached decision should have been applied")
	}
}

// The cache key must distinguish different questions.
func TestDecisionKeyDependsOnCandidateSet(t *testing.T) {
	a := residualOf(1, "بانادول", 10, 11)
	b := residualOf(1, "بانادول", 10, 12)
	if decisionKey(a) == decisionKey(b) {
		t.Fatal("a different shortlist is a different question and must not share a cached answer")
	}
}

func TestDecisionKeyIsOrderIndependent(t *testing.T) {
	a := residualOf(1, "بانادول", 10, 11, 12)
	b := residualOf(1, "بانادول", 12, 10, 11)
	if decisionKey(a) != decisionKey(b) {
		t.Fatal("the same shortlist in a different order is the same question")
	}
}

func TestLinesWithNoCandidatesAreNeverSent(t *testing.T) {
	repo := &fakeRepo{}
	ai := &fakeAI{}
	a := NewAdjudication(repo, ai)

	a.Run(context.Background(), []Residual{residualOf(1, "شيء غامض")}) // no candidates

	if ai.callCount() != 0 {
		t.Fatal("asking a model to choose from an empty list invites invention")
	}
}

// FR-018f: the ceiling degrades the run instead of stalling it.
func TestWallClockCeilingStopsAdjudication(t *testing.T) {
	repo := &fakeRepo{}
	ai := &fakeAI{respond: func([]AdjudicationItem) ([]AdjudicationResult, error) { return nil, nil }}
	a := NewAdjudication(repo, ai)

	// The clock must advance rather than sit at a constant: the deadline is
	// computed from the first reading, so a fixed "far future" would move the
	// deadline with it and never trip. First call sets the deadline; every call
	// after it is past.
	start := time.Now()
	first := true
	a.now = func() time.Time {
		if first {
			first = false
			return start
		}
		return start.Add(2 * MaxWallClock)
	}

	var residual []Residual
	for i := int64(1); i <= 100; i++ {
		residual = append(residual, residualOf(i, "منتج", 10, 11))
	}
	a.Run(context.Background(), residual)

	if !a.Stats.CeilingHit {
		t.Fatal("expected the ceiling to be recorded so the buyer can be told")
	}
	if ai.callCount() != 0 {
		t.Fatalf("no request should be made past the deadline, got %d", ai.callCount())
	}
}

func TestRequestCeilingIsEnforced(t *testing.T) {
	repo := &fakeRepo{}
	ai := &fakeAI{respond: func([]AdjudicationItem) ([]AdjudicationResult, error) { return nil, nil }}
	a := NewAdjudication(repo, ai)

	var residual []Residual
	for i := int64(1); i <= int64(MaxItemsPerRequest*(MaxRequestsPerRun+10)); i++ {
		residual = append(residual, residualOf(i, "منتج", 10, 11))
	}
	a.Run(context.Background(), residual)

	if ai.callCount() > MaxRequestsPerRun {
		t.Fatalf("expected at most %d requests, got %d", MaxRequestsPerRun, ai.callCount())
	}
	if !a.Stats.CeilingHit {
		t.Fatal("hitting the request ceiling must be recorded")
	}
}

// A batch that fails wholesale is retried once at half size.
func TestFailedBatchIsBisected(t *testing.T) {
	repo := &fakeRepo{}
	attempt := 0
	ai := &fakeAI{respond: func(items []AdjudicationItem) ([]AdjudicationResult, error) {
		attempt++
		if attempt == 1 {
			return nil, errors.New("response too large")
		}
		out := make([]AdjudicationResult, 0, len(items))
		for _, it := range items {
			id := it.Candidates[0].ProductID
			out = append(out, AdjudicationResult{LineID: it.LineID, ProductID: &id, Confidence: 0.9})
		}
		return out, nil
	}}
	a := NewAdjudication(repo, ai)

	var residual []Residual
	for i := int64(1); i <= 10; i++ {
		residual = append(residual, residualOf(i, "منتج", 10, 11))
	}
	a.Run(context.Background(), residual)

	if ai.callCount() != 3 {
		t.Fatalf("expected one failed call then two halves, got %d", ai.callCount())
	}
	if a.Stats.Adjudicated != 10 {
		t.Fatalf("bisection should have salvaged all 10 lines, got %d", a.Stats.Adjudicated)
	}
}

func TestNoneOfTheseIsAcceptedWithoutChangingTheLine(t *testing.T) {
	repo := &fakeRepo{}
	ai := &fakeAI{respond: func(items []AdjudicationItem) ([]AdjudicationResult, error) {
		return []AdjudicationResult{{LineID: items[0].LineID, ProductID: nil, Confidence: 0.8}}, nil
	}}
	a := NewAdjudication(repo, ai)

	residual := []Residual{residualOf(1, "بانادول", 10, 11)}
	a.Run(context.Background(), residual)

	if residual[0].Line.Matched() {
		t.Fatal(`"none of these" must not produce a match`)
	}
	if a.Stats.Rejected != 0 {
		t.Fatal(`"none of these" is a valid answer, not a rejection`)
	}
}

// AI output is stored so a person can promote it, never trusted on its own.
// The deterministic alias tier excludes 'ai_confirmed' exactly so that one
// confident mistake cannot propagate silently to every pharmacy.
func TestAdjudicatedAliasesAreStoredButNotTrusted(t *testing.T) {
	repo := &fakeRepo{}
	chosen := int64(11)
	ai := &fakeAI{respond: func(items []AdjudicationItem) ([]AdjudicationResult, error) {
		return []AdjudicationResult{{LineID: items[0].LineID, ProductID: &chosen, Confidence: 0.9}}, nil
	}}
	a := NewAdjudication(repo, ai)
	a.Run(context.Background(), []Residual{residualOf(1, "بانادول", 10, 11)})

	if len(repo.aliases) != 1 {
		t.Fatalf("expected the decision to be recorded as an alias, got %d", len(repo.aliases))
	}
	if repo.aliases[0].source != "ai_confirmed" {
		t.Fatalf("an AI alias must be marked untrusted, got source %q", repo.aliases[0].source)
	}
	if repo.aliases[0].productID != 11 {
		t.Fatalf("expected product 11, got %d", repo.aliases[0].productID)
	}
}
