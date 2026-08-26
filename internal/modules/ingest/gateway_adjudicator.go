package ingest

import (
	"context"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
)

// The seam between the vendor import and the AI gateway.
//
// The import defines Adjudicator in its own terms and the composition root
// wires something that satisfies it. This adapter exists because the catalogue
// module already owns a working batch adjudicator — the same prompt, the same
// shortlist contract, the same "an id you were given or null" rule — and two
// copies of that prompt would drift apart the first time either was tuned.
//
// Nothing here knows about the gateway. It translates one module's request
// shape into another's, which keeps the dependency pointing at a capability
// rather than at a transport.

// CatalogAdjudicator is the shape the catalogue module exposes.
type CatalogAdjudicator interface {
	AdjudicateMatches(ctx context.Context, req catalog.MatchAdjudicationRequest) ([]catalog.MatchAdjudicationResult, error)
}

// NewCatalogAdjudicator adapts the catalogue's batch adjudicator for the vendor
// import.
//
// A nil argument yields a nil Adjudicator, which the import treats as "AI
// unavailable" — so a deployment with no gateway simply does not offer the
// switch, rather than offering one that silently does nothing.
func NewCatalogAdjudicator(inner CatalogAdjudicator) Adjudicator {
	if inner == nil {
		return nil
	}
	return &catalogAdjudicator{inner: inner}
}

type catalogAdjudicator struct {
	inner CatalogAdjudicator
}

func (a *catalogAdjudicator) AdjudicateMatches(
	ctx context.Context, req AdjudicationRequest,
) ([]AdjudicationDecision, error) {
	out := catalog.MatchAdjudicationRequest{
		Items:          make([]catalog.MatchAdjudicationItem, 0, len(req.Items)),
		OrganizationID: req.OrganizationID,
		UserID:         req.UserID,
	}
	for _, it := range req.Items {
		item := catalog.MatchAdjudicationItem{Ref: it.Ref, Text: it.Text}
		for _, c := range it.Candidates {
			item.Candidates = append(item.Candidates, catalog.MatchAdjudicationCandidate{
				ProductID:     c.ProductID,
				Name:          c.Name,
				NameEN:        c.NameEN,
				Scientific:    c.Scientific,
				DosageForm:    c.DosageForm,
				Concentration: c.Concentration,
				Manufacturer:  c.Manufacturer,
			})
		}
		out.Items = append(out.Items, item)
	}

	results, err := a.inner.AdjudicateMatches(ctx, out)
	if err != nil {
		return nil, err
	}

	decisions := make([]AdjudicationDecision, 0, len(results))
	for _, r := range results {
		decisions = append(decisions, AdjudicationDecision{
			Ref:        r.Ref,
			ProductID:  r.ProductID,
			Confidence: r.Confidence,
			Reason:     r.Reason,
		})
	}
	return decisions, nil
}
