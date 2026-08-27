package main

// Wiring the vendor catalogue import's AI stage.
//
// The translation is mechanical and lives here rather than in either module,
// because this is the only place allowed to know about both. It is the same
// capability, the same system prompt and the same response contract the smart
// order runs through — deliberately, so the two share one prompt to tune and
// one decision cache to hit.

import (
	"context"

	"github.com/muhiya/dawa24-store/internal/modules/aicapabilities"
	"github.com/muhiya/dawa24-store/internal/modules/ingest"
)

type ingestEnhanceAdapter struct {
	caps *aicapabilities.Service
}

func (a *ingestEnhanceAdapter) EnhanceBatch(
	ctx context.Context, batch ingest.GatewayBatch,
) ([]ingest.GatewayOutcome, error) {
	req := aicapabilities.EnhanceRequest{
		Catalog: make([]aicapabilities.CatalogEntry, 0, len(batch.Catalog)),
		Items:   make([]aicapabilities.EnhanceItem, 0, len(batch.Items)),
	}
	for _, c := range batch.Catalog {
		req.Catalog = append(req.Catalog, aicapabilities.CatalogEntry(c))
	}
	for _, it := range batch.Items {
		req.Items = append(req.Items, aicapabilities.EnhanceItem(it))
	}

	decisions, err := a.caps.EnhanceMatches(ctx, req)
	if err != nil {
		return nil, err
	}
	out := make([]ingest.GatewayOutcome, 0, len(decisions))
	for _, d := range decisions {
		out = append(out, ingest.GatewayOutcome(d))
	}
	return out, nil
}
