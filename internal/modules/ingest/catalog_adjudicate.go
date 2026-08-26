package ingest

import (
	"context"
	"time"

	"github.com/muhiya/dawa24-store/internal/shared/productmatch"
)

// The AI tier of the vendor catalogue import.
//
// It sits after the deterministic engine and sees only what the engine left
// unsettled — on a real supplier file, tens of rows out of thousands. Three
// rules keep it that way, and every one of them exists because the obvious
// implementation is ruinous at nine thousand rows:
//
//   - Never per row. Rows are batched, and each batch carries its own
//     shortlist. A model asked about one row at a time turns a two-minute
//     import into an hour and a month's AI budget into an afternoon.
//   - Never with catalogue access. The batch carries everything the decision
//     needs, so there is nothing for a model to go looking through.
//   - Never unbounded. A run has a request budget and a wall clock, and when
//     either runs out the remaining rows keep the deterministic outcome —
//     which is a complete, usable answer, not a failure.
//
// And the rule that matters most for a pharmaceutical catalogue: an answer
// naming a product that was not among that row's candidates is discarded. A
// hallucinated id here would tie a vendor's price to the wrong medicine.

// Adjudicator resolves rows the deterministic engine left unsettled by choosing
// among candidates it is given.
//
// Declared here rather than imported so the module depends on the capability
// and not on a transport, and so every test in this package runs without a
// gateway.
type Adjudicator interface {
	AdjudicateMatches(ctx context.Context, req AdjudicationRequest) ([]AdjudicationDecision, error)
}

// AdjudicationRequest is one batch, attributed to the vendor whose import
// asked for it so the spend is billed and capped against their own budget.
type AdjudicationRequest struct {
	Items          []AdjudicationItem
	OrganizationID int64
	UserID         int64
}

// AdjudicationItem is one row and its shortlist.
type AdjudicationItem struct {
	// Ref ties an answer back to the row that asked. It is the row's position
	// in the batch being written, never a database id.
	Ref        int64
	Text       string
	Candidates []AdjudicationCandidate
}

// AdjudicationCandidate is a catalogue product as the adjudicator sees it.
type AdjudicationCandidate struct {
	ProductID     int64
	Name          string
	NameEN        string
	Scientific    string
	DosageForm    string
	Concentration string
	Manufacturer  string
}

// AdjudicationDecision is one answer. A nil ProductID means "none of these",
// which is a legitimate and frequent result.
type AdjudicationDecision struct {
	Ref        int64
	ProductID  *int64
	Confidence float64
	Reason     string
}

// SetAdjudicator installs the AI matching port. Leaving it unset means the tier
// is skipped, which is the same path a disabled gateway takes.
func (s *Service) SetAdjudicator(a Adjudicator) { s.adjudicator = a }

// AIAvailable reports whether the import screen may offer the AI switch.
func (s *Service) AIAvailable() bool { return s != nil && s.adjudicator != nil }

// Ceilings for one run.
const (
	// adjudicationBatchSize is how many rows go in one request.
	adjudicationBatchSize = 25
	// adjudicationRequestBudget is the whole run's allowance. At the batch size
	// above this is 1,000 rows adjudicated, which is far more residue than a
	// well-mapped file produces and a firm stop for one that is not.
	adjudicationRequestBudget = 40
	// adjudicationWallClock stops the tier once a run has spent this long in it,
	// however much budget is left. A vendor watching a progress bar should not
	// wait on a model that is answering slowly.
	adjudicationWallClock = 3 * time.Minute
	// adjudicationFloor is the confidence a decision must carry to be applied.
	adjudicationFloor = 0.70
)

// openRow is one row the deterministic engine left unsettled, with the
// shortlist it retrieved. Carrying the candidates alongside the decision is
// what lets the answer be validated against what was actually offered.
type openRow struct {
	d          *decision
	candidates []productmatch.MatchCandidate
}

// aiBudget is the per-run allowance, carried on the writer so it is spent
// across batches rather than reset by each one.
type aiBudget struct {
	requests int
	started  time.Time
	// Adjudicated and Rejected are reported in the run log so a vendor asking
	// "did the AI do anything" gets an answer.
	adjudicated int
	rejected    int
	ceilingHit  bool
}

func (b *aiBudget) spent() bool {
	if b.requests >= adjudicationRequestBudget {
		return true
	}
	return !b.started.IsZero() && time.Since(b.started) > adjudicationWallClock
}

// adjudicate resolves the unsettled rows of one batch and applies what it can.
//
// It never returns an error. Every failure path — gateway down, budget spent,
// malformed answer — leaves the deterministic outcome standing, because a
// vendor must be able to import their catalogue when the model is unavailable.
func (w *importWriter) adjudicate(ctx context.Context, decisions []*decision) {
	if !w.settings.UseAI || w.svc.adjudicator == nil || w.ai.spent() {
		return
	}
	if w.ai.started.IsZero() {
		w.ai.started = time.Now()
	}

	// Only rows the engine could not settle, and only those it found something
	// plausible for. A row with no candidates has nothing to choose between,
	// and asking anyway invites the model to invent a product.
	var pending []openRow
	for _, d := range decisions {
		if d.outcome != "" || d.match.Level.Settled() || len(d.match.Candidates) == 0 {
			continue
		}
		pending = append(pending, openRow{d: d, candidates: d.match.Candidates})
	}
	if len(pending) == 0 {
		return
	}

	byRef := make(map[int64]openRow, len(pending))
	items := make([]AdjudicationItem, 0, len(pending))
	for _, p := range pending {
		ref := int64(p.d.ref)
		byRef[ref] = p
		items = append(items, AdjudicationItem{
			Ref:        ref,
			Text:       p.d.row.DisplayName(),
			Candidates: adjudicationCandidates(p.candidates),
		})
	}

	for start := 0; start < len(items); start += adjudicationBatchSize {
		if w.ai.spent() {
			w.ai.ceilingHit = true
			return
		}
		end := start + adjudicationBatchSize
		if end > len(items) {
			end = len(items)
		}

		w.ai.requests++
		req := AdjudicationRequest{
			Items:          items[start:end],
			OrganizationID: w.session.OrganizationID,
		}
		if w.session.CreatedBy != nil {
			req.UserID = *w.session.CreatedBy
		}

		results, err := w.svc.adjudicator.AdjudicateMatches(ctx, req)
		if err != nil {
			w.svc.log.WarnContext(ctx, "match adjudication failed; deterministic outcome stands",
				"import", w.session.PublicID, "items", end-start, "error", err)
			continue
		}
		w.applyDecisions(byRef, results)
	}
}

// applyDecisions folds accepted answers back onto the rows that asked.
func (w *importWriter) applyDecisions(byRef map[int64]openRow, results []AdjudicationDecision) {
	for _, res := range results {
		p, known := byRef[res.Ref]
		if !known || p.d.outcome != "" {
			continue
		}
		if res.ProductID == nil || res.Confidence < adjudicationFloor {
			continue
		}
		// A product that was not on this row's shortlist is rejected outright.
		// It is the guard that stops an invented id becoming a live price.
		if !candidateOffered(p.candidates, *res.ProductID) {
			w.ai.rejected++
			continue
		}

		// The row is now matched. It stops being a new catalogue product and
		// becomes an update to an existing one, which is the whole point: the
		// alternative was a duplicate entry in the shared catalogue.
		p.d.productID = *res.ProductID
		p.d.match.ProductID = *res.ProductID
		p.d.match.Level = productmatch.MatchStrong
		p.d.match.Score = res.Confidence
		p.d.match.Reason = adjudicationReason(res.Reason)

		// Move the row out of whichever counter it was first tallied under
		// rather than assuming it was "needs review": a row with candidates but
		// a weak best score was counted as unmatched, and decrementing the wrong
		// counter makes the results screen disagree with itself.
		w.count(p.d.bucket, -1)
		p.d.bucket = bucketMatched
		w.count(bucketMatched, 1)
		w.ai.adjudicated++
	}
}

func adjudicationReason(reason string) string {
	if reason == "" {
		return "مطابقة بالذكاء الاصطناعي بين المرشحين"
	}
	return "مطابقة بالذكاء الاصطناعي: " + reason
}

func adjudicationCandidates(in []productmatch.MatchCandidate) []AdjudicationCandidate {
	out := make([]AdjudicationCandidate, 0, len(in))
	for _, c := range in {
		out = append(out, AdjudicationCandidate{
			ProductID:     c.ProductID,
			Name:          c.Name,
			Scientific:    c.Scientific,
			DosageForm:    c.DosageForm,
			Concentration: c.Concentration,
			Manufacturer:  c.Manufacturer,
		})
	}
	return out
}

func candidateOffered(candidates []productmatch.MatchCandidate, productID int64) bool {
	for _, c := range candidates {
		if c.ProductID == productID {
			return true
		}
	}
	return false
}
