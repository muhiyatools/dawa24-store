package pipeline

import "context"

// The seam between the pipeline and the AI gateway.
//
// The pipeline defines Adjudicator in its own terms; the composition root wires
// an implementation backed by aicapabilities. Nothing in this package knows the
// gateway exists, which is what keeps provider and model names out of it
// (AGENTS.md R2) and what lets every adjudication test run without one.

// BatchAdjudicator is the shape aicapabilities exposes. Declaring it here rather
// than importing the module keeps the dependency pointing inward.
type BatchAdjudicator interface {
	AdjudicateBatch(ctx context.Context, items []GatewayItem) ([]GatewayDecision, error)
}

// GatewayItem mirrors aicapabilities.AdjudicateItem.
type GatewayItem struct {
	LineID     int64
	Text       string
	Candidates []GatewayCandidate
}

// GatewayCandidate mirrors aicapabilities.AdjudicateCandidate.
type GatewayCandidate struct {
	ProductID     int64
	Name          string
	NameEN        string
	Scientific    string
	DosageForm    string
	Concentration string
	Manufacturer  string
}

// GatewayDecision mirrors aicapabilities.AdjudicateDecision.
type GatewayDecision struct {
	LineID     int64
	ProductID  *int64
	Confidence float64
	Reason     string
}

// GatewayAdjudicator adapts a BatchAdjudicator to the pipeline's Adjudicator.
type GatewayAdjudicator struct {
	batch BatchAdjudicator
}

// NewGatewayAdjudicator wires the gateway into the pipeline.
//
// A nil batch adjudicator yields a nil Adjudicator, which the pipeline treats as
// "AI disabled" — so an unconfigured gateway is a quiet no-op rather than a
// runtime panic on the first import.
func NewGatewayAdjudicator(batch BatchAdjudicator) Adjudicator {
	if batch == nil {
		return nil
	}
	return &GatewayAdjudicator{batch: batch}
}

// Adjudicate translates the pipeline's request, calls the gateway, and
// translates back.
func (g *GatewayAdjudicator) Adjudicate(ctx context.Context, items []AdjudicationItem) ([]AdjudicationResult, error) {
	out := make([]GatewayItem, 0, len(items))
	for _, it := range items {
		gi := GatewayItem{LineID: it.LineID, Text: it.RawText}
		for _, c := range it.Candidates {
			gi.Candidates = append(gi.Candidates, GatewayCandidate{
				ProductID:     c.ProductID,
				Name:          c.NameAR,
				NameEN:        c.NameEN,
				Scientific:    c.Scientific,
				DosageForm:    c.DosageForm,
				Concentration: c.Concentration,
				Manufacturer:  c.Manufacturer,
			})
		}
		out = append(out, gi)
	}

	decisions, err := g.batch.AdjudicateBatch(ctx, out)
	if err != nil {
		return nil, err
	}

	results := make([]AdjudicationResult, 0, len(decisions))
	for _, d := range decisions {
		results = append(results, AdjudicationResult{
			LineID:     d.LineID,
			ProductID:  d.ProductID,
			Confidence: d.Confidence,
			Reason:     d.Reason,
		})
	}
	return results, nil
}
