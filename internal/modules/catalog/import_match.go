package catalog

import (
	"context"
	"strings"

	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/matchflow"
	"github.com/muhiya/dawa24-store/internal/shared/productmatch"
)

// Matching an imported row to the product it is meant to update.
//
// The exact tiers — item code, barcode, and the Arabic name folded by
// platform.normalize_arabic — settle a re-upload of the very same file and
// almost nothing else. Measured on the live catalogue they resolved under a
// tenth of a real supplier price list, because a supplier does not write
// "ابكسيدون 0.5 مجم أقراص" the way the catalogue records it: they write
// "ابكسيدون .5 مجم اقراص", or they put the strength in its own column, or they
// append the pack count. Every one of those is a different string and the same
// medicine.
//
// So the residue goes through the same engine the vendor import and the smart
// order use: an in-memory index of the catalogue, scored on the identifying
// words with the dose and the pharmaceutical form able to veto. One query
// builds it, and the per-row cost after that is CPU.
//
// What is deliberately NOT done here is applying a weak match silently. Every
// similarity match is staged under its own reason so the review screen shows it
// as a judgement rather than as a fact, and the admin commits it knowingly.

// MatchSimilar is a match made on name similarity with corroborating
// attributes rather than on an identifier. It updates an existing product, so
// the review screen marks it for a second look.
const MatchSimilar MatchReason = "similar"

// similarityFloor is the score at or above which a similarity match is offered
// as an update on the strength of the name alone.
//
// Higher than any other importer's, because the consequences differ: a vendor's
// wrong match mislabels one of their own offers, while a wrong match here
// overwrites the shared catalogue entry every pharmacy reads.
//
// What that produced, measured on the live table: 37,576 rows staged as
// `insert` with no reason, 82 as `similar`, 26 by name. A match rate of 0.29%,
// which means every re-upload of the same administrative file duplicates the
// catalogue. The floor was not wrong about the risk; it was the only tool being
// used to manage it.
const similarityFloor = 0.86

// corroboratedFloor is the score at or above which a match is offered as an
// update when something other than the name agrees as well.
//
// The engine's own settled levels — a barcode hit, a code hit, an exact folded
// name — are already identifiers rather than judgements. Below those, a strong
// score whose dose and pharmaceutical form both corroborate is a different
// proposition from a strong score on the name alone, and the identity re-check
// that guards the AI tier is available to say so.
//
// So the rule is: a bare name similarity still needs 0.86, and a corroborated
// one needs 0.78 — the figure the vendor import has always used, applied here
// only where the extra evidence exists.
const corroboratedFloor = 0.78

// aiFloor is the confidence a model must express before its choice is applied.
//
// From the shared table rather than a local number, and the shared table sets
// it higher here than anywhere else — 0.90 against 0.80 — because this is the
// one importer whose wrong match overwrites the catalogue entry every pharmacy
// reads. The local constant was 0.70, which is below what the other two tools
// demand for a decision with a far smaller blast radius.
var aiFloor = catalogCeilings.MinApplyConfidence

// catalogCeilings is what one administrative import may spend, from the shared
// table.
//
// It used to be two constants declared here — 24 batches of 25 rows — and that
// is precisely the drift internal/shared/matchflow was created to stop. The
// vendor importer and the smart order had each grown their own copy of the same
// numbers, every one of them documented as measured, and no two of them
// agreeing. Reading the shared table means a change to what a run may spend is
// one edit rather than three, and the master-catalogue profile is deliberately
// the most conservative of the three because a wrong match here overwrites the
// entry every pharmacy reads.
//
// It also lifts the ceiling this import was working under. 24 by 25 was 600
// rows: on a thirty-thousand-row file that is two per cent of the residue, and
// nothing told the administrator that the other ninety-eight per cent had never
// been looked at. The shared profile allows 12 requests of 100.
var catalogCeilings = matchflow.For(matchflow.ProfileCatalog)

// maxAIAdjudicationBatches bounds the AI tier for one import.
//
// It exists because an import must finish: a fifty-thousand-row file whose
// every row is ambiguous would otherwise spend the afternoon in a model, and
// the deterministic outcome it already has — "this is a new product" — is a
// serviceable answer. Rows past it keep that outcome and the screen says the
// ceiling was reached.
var maxAIAdjudicationBatches = catalogCeilings.MaxRequestsPerRun

// aiBatchSize is how many rows go in one adjudication request. Batched, never
// per row: the same rule the smart order's adjudication follows, for the same
// reason — one request per row turns a three-minute import into an hour.
var aiBatchSize = catalogCeilings.MaxItemsPerRequest

// MatchAdjudicationRequest is one batch of ambiguous rows, attributed to the
// organisation whose import asked for it so AI spend is billed and capped per
// tenant rather than against one platform key.
type MatchAdjudicationRequest struct {
	Items []MatchAdjudicationItem `json:"items"`

	OrganizationID int64 `json:"-"`
	UserID         int64 `json:"-"`
}

// MatchAdjudicationItem is one ambiguous row and the shortlist it may resolve
// to. Nothing else is sent, and the model may only answer with an id from the
// shortlist.
type MatchAdjudicationItem struct {
	// Ref is the caller's index into its own row slice. It travels out and back
	// so an answer can be tied to the row that asked the question.
	Ref        int64
	Text       string
	Candidates []MatchAdjudicationCandidate
}

// MatchAdjudicationCandidate is a catalogue product as the adjudicator sees it.
type MatchAdjudicationCandidate struct {
	ProductID     int64
	Name          string
	NameEN        string
	Scientific    string
	DosageForm    string
	Concentration string
	Manufacturer  string
}

// MatchAdjudicationResult is one decision. A nil ProductID means "none of
// these", which is a useful and frequent answer.
type MatchAdjudicationResult struct {
	Ref        int64
	ProductID  *int64
	Confidence float64
	Reason     string
}

// MatchAdjudicator resolves rows the deterministic tiers left ambiguous.
//
// It is a port rather than a dependency on the gateway, so the import can be
// tested without one and so an unconfigured deployment simply skips the tier.
type MatchAdjudicator interface {
	AdjudicateMatches(ctx context.Context, req MatchAdjudicationRequest) ([]MatchAdjudicationResult, error)
}

// SetMatchAdjudicator installs the AI matching port.
func (s *Service) SetMatchAdjudicator(a MatchAdjudicator) { s.adjudicator = a }

// MatchStats accounts for what each tier resolved, for the review screen.
type MatchStats struct {
	Exact      int `json:"exact"`
	Similar    int `json:"similar"`
	AI         int `json:"ai"`
	Unmatched  int `json:"unmatched"`
	AIRequests int `json:"ai_requests"`
	// CeilingHit means the AI tier stopped early and some rows kept their
	// deterministic outcome. Reported so a low match rate on a huge file is not
	// mistaken for a bad catalogue.
	CeilingHit bool `json:"ceiling_hit"`
}

// Matched is how many rows were tied to an existing catalogue product.
func (m MatchStats) Matched() int { return m.Exact + m.Similar + m.AI }

// Total is how many rows the matcher considered.
func (m MatchStats) Total() int { return m.Matched() + m.Unmatched }

// RatePercent is the share of rows tied to an existing product, 0–100.
func (m MatchStats) RatePercent() int {
	if m.Total() == 0 {
		return 0
	}
	return m.Matched() * 100 / m.Total()
}

// pendingMatch is a row the exact tiers missed, carried to the AI tier with its
// shortlist already retrieved so adjudication costs no further catalogue work.
type pendingMatch struct {
	index      int
	candidates []productmatch.MatchCandidate
}

// resolveSimilarMatches ties the rows the exact tiers missed to the catalogue
// products they are most likely meant to update.
//
// matches is mutated in place. Rows already matched by an identifier are never
// reconsidered: an exact code beats any similarity score, always.
func (s *Service) resolveSimilarMatches(
	ctx context.Context,
	session *ImportSession,
	prods []*Product,
	matches map[int]ExistingMatch,
) MatchStats {
	stats := MatchStats{Exact: len(matches)}

	residual := make([]int, 0, len(prods))
	for i, p := range prods {
		if p == nil {
			continue
		}
		if _, done := matches[i]; done {
			continue
		}
		residual = append(residual, i)
	}
	if len(residual) == 0 || s.imports == nil {
		stats.Unmatched = len(residual)
		return stats
	}

	index, ok := s.catalogueIndex(ctx, session)
	if !ok {
		stats.Unmatched = len(residual)
		return stats
	}

	opts := productmatch.DefaultMatchOptions()
	opts.MinStrong = corroboratedFloor
	opts.MaxCandidates = 5

	var forAI []pendingMatch
	for _, i := range residual {
		row := matchRowFor(prods[i])
		res := index.Match(row, opts)
		switch {
		case res.Matched() && acceptsUpdate(index, row, res):
			matches[i] = ExistingMatch{ProductID: res.ProductID, Reason: MatchSimilar}
			stats.Similar++
		case len(res.Candidates) > 0:
			forAI = append(forAI, pendingMatch{index: i, candidates: res.Candidates})
		default:
			stats.Unmatched++
		}
	}

	if len(forAI) == 0 {
		return stats
	}
	if !session.Options.UseAI || s.adjudicator == nil {
		stats.Unmatched += len(forAI)
		return stats
	}

	s.adjudicateMatches(ctx, session, prods, matches, forAI, &stats)
	stats.Unmatched += len(forAI) - stats.AI
	return stats
}

// acceptsUpdate decides whether a match is safe to apply to the shared
// catalogue without asking an administrator.
//
// Three ways in, in descending order of how little judgement each requires:
//
//   - the engine settled it on an identifier or an exact folded name;
//   - the score clears the bare-name floor;
//   - the score clears the corroborated floor AND the catalogue's own record
//     agrees about the product's identity — the same re-check that validates an
//     answer from the model before it is written.
//
// Ambiguity is refused in every case. Two products that fit equally well is not
// a weaker match; it is a question, and the review screen exists to ask it.
func acceptsUpdate(index *productmatch.Index, row *productmatch.Row, res productmatch.MatchResult) bool {
	if res.Level == productmatch.MatchAmbiguous {
		return false
	}
	if res.Level == productmatch.MatchBarcode || res.Level == productmatch.MatchCode ||
		res.Level == productmatch.MatchExact {
		return true
	}
	if res.Score >= similarityFloor {
		return true
	}
	if res.Score < corroboratedFloor {
		return false
	}
	return index.IdentityConflict(row, res.ProductID).None()
}

// catalogueIndex builds the in-memory catalogue the residue is scored against.
func (s *Service) catalogueIndex(ctx context.Context, session *ImportSession) (*productmatch.Index, bool) {
	catalogue, err := s.imports.ListMatchProducts(ctx)
	if err != nil {
		s.log.WarnContext(ctx, "catalogue projection unavailable; similarity matching skipped",
			"session", session.PublicID, "error", err)
		return nil, false
	}
	if len(catalogue) == 0 {
		return nil, false
	}
	masters := make([]productmatch.MasterProduct, 0, len(catalogue))
	for _, c := range catalogue {
		masters = append(masters, productmatch.MasterProduct{
			ID: c.ID, NameAR: c.NameAR, NameEN: c.NameEN, SKU: c.SKU,
			Barcode: c.Barcode, Scientific: c.Scientific, DosageForm: c.DosageForm,
			Concentration: c.Concentration, Unit: c.Unit,
			Manufacturer: c.Manufacturer, PublicPrice: c.PublicPrice,
		})
	}
	return productmatch.NewIndex(masters), true
}

// adjudicateMatches asks the model about the rows similarity could not settle,
// in batches, and applies only the answers that name a candidate it was given.
func (s *Service) adjudicateMatches(
	ctx context.Context,
	session *ImportSession,
	prods []*Product,
	matches map[int]ExistingMatch,
	forAI []pendingMatch,
	stats *MatchStats,
) {
	items := make([]MatchAdjudicationItem, 0, len(forAI))
	byRef := make(map[int64]pendingMatch, len(forAI))
	for _, p := range forAI {
		ref := int64(p.index)
		byRef[ref] = p
		items = append(items, MatchAdjudicationItem{
			Ref:        ref,
			Text:       adjudicationText(prods[p.index]),
			Candidates: summarizeCandidates(p.candidates),
		})
	}

	decided := map[int64]bool{}
	for start := 0; start < len(items); start += aiBatchSize {
		if stats.AIRequests >= maxAIAdjudicationBatches {
			stats.CeilingHit = true
			return
		}
		end := start + aiBatchSize
		if end > len(items) {
			end = len(items)
		}

		stats.AIRequests++
		session.AICalls++
		req := MatchAdjudicationRequest{
			Items:          items[start:end],
			OrganizationID: session.OrganizationID,
		}
		if session.CreatedBy != nil {
			req.UserID = *session.CreatedBy
		}
		results, err := s.adjudicator.AdjudicateMatches(ctx, req)
		if err != nil {
			// One failed batch is not a failed import: those rows keep the
			// deterministic outcome, which is "this is a new product".
			session.AIFallback = true
			s.log.WarnContext(ctx, "match adjudication batch failed; deterministic outcome stands",
				"session", session.PublicID, "items", end-start, "error", err)
			continue
		}
		for _, res := range results {
			p, known := byRef[res.Ref]
			if !known || decided[res.Ref] {
				continue
			}
			decided[res.Ref] = true
			if res.ProductID == nil || res.Confidence < aiFloor {
				continue
			}
			// A product that was not on the shortlist for that row is rejected
			// outright. This is the guard that stops an invented id becoming an
			// update to a real catalogue entry.
			if !inShortlist(p.candidates, *res.ProductID) {
				continue
			}
			matches[p.index] = ExistingMatch{ProductID: *res.ProductID, Reason: MatchAI}
			stats.AI++
			session.AIApplied++
		}
	}
}

// matchRowFor projects an imported product onto the row shape the shared
// matcher scores.
func matchRowFor(p *Product) *productmatch.Row {
	return &productmatch.Row{
		Name:          p.Name.Get(i18n.AR),
		NameEN:        p.Name.Get(i18n.EN),
		Scientific:    p.ScientificName,
		SKU:           p.SKU,
		Barcode:       p.Barcode,
		Manufacturer:  p.ManufacturingCompanies,
		DosageForm:    p.DosageForm,
		Concentration: p.Concentration,
		Unit:          p.Unit,
	}
}

// adjudicationText is what the model is shown for the incoming row: everything
// that identifies it, and nothing that does not.
func adjudicationText(p *Product) string {
	parts := make([]string, 0, 5)
	for _, v := range []string{
		p.Name.Get(i18n.AR), p.Name.Get(i18n.EN),
		p.Concentration, p.DosageForm, p.ManufacturingCompanies,
	} {
		if v = strings.TrimSpace(v); v != "" {
			parts = append(parts, v)
		}
	}
	return strings.Join(parts, " | ")
}

func summarizeCandidates(candidates []productmatch.MatchCandidate) []MatchAdjudicationCandidate {
	out := make([]MatchAdjudicationCandidate, 0, len(candidates))
	for _, c := range candidates {
		out = append(out, MatchAdjudicationCandidate{
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

func inShortlist(candidates []productmatch.MatchCandidate, productID int64) bool {
	for _, c := range candidates {
		if c.ProductID == productID {
			return true
		}
	}
	return false
}
