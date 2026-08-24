// Package gateway adapts the platform AI Gateway to the catalogue's Enricher
// port.
//
// It lives beside the catalogue module rather than inside it so the domain
// depends on the capability it needs and not on a transport. The catalogue
// knows nothing about virtual keys, budgets, or circuit breakers; this package knows
// nothing about products beyond the request shape it is handed.
package gateway

import (
	"context"
	"errors"
	"log/slog"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/platform/gateway"
)

// KeyResolver resolves an organisation's Gateway virtual key, so AI spend is
// attributed and capped per tenant rather than against one platform key.
type KeyResolver func(ctx context.Context, orgID int64) (string, error)

// Enricher fills catalogue gaps through the AI Gateway.
type Enricher struct {
	client      gateway.Client
	log         *slog.Logger
	keyResolver KeyResolver
}

// NewEnricher wires the Gateway client into the catalogue's Enricher port.
func NewEnricher(client gateway.Client, log *slog.Logger) *Enricher {
	return &Enricher{client: client, log: log.With("component", "catalog_enricher")}
}

// SetKeyResolver installs per-organisation Gateway billing.
func (e *Enricher) SetKeyResolver(r KeyResolver) { e.keyResolver = r }

// Available reports whether the Gateway can be called at all right now. The
// wizard uses it to explain why the AI switch is unavailable rather than
// offering a toggle that silently does nothing.
func (e *Enricher) Available(ctx context.Context) bool {
	if e == nil || e.client == nil {
		return false
	}
	return e.client.Enabled()
}

// Enrich classifies one batch of products.
//
// The error it returns is always worth falling back on rather than surfacing:
// the caller keeps the deterministic values it already had and marks the session
// as having degraded, so an import never fails because a model was unavailable.
func (e *Enricher) Enrich(ctx context.Context, req catalog.EnrichRequest) (catalog.EnrichResponse, error) {
	if e == nil || e.client == nil {
		return catalog.EnrichResponse{Fallback: true}, gateway.ErrDisabled
	}

	input, err := catalog.EncodeEnrichInput(req)
	if err != nil {
		return catalog.EnrichResponse{Fallback: true}, err
	}

	gwReq := gateway.Request{
		// Filling missing catalogue attributes is enrichment, and its budget is
		// the slow, high-quality one: a batch of forty products is worth one
		// careful answer, not three fast wrong ones.
		Capability:     gateway.CapProductEnrich,
		System:         catalog.EnrichSystemPrompt(),
		Input:          input,
		Schema:         catalog.EnrichSchema(),
		OrganizationID: req.OrganizationID,
		UserID:         req.UserID,
		VirtualKey:     e.virtualKey(ctx, req.OrganizationID),
	}

	resp, err := e.client.Invoke(ctx, gwReq)
	if err != nil {
		if gateway.ShouldFallback(err) {
			e.log.WarnContext(ctx, "enrichment unavailable, keeping deterministic values",
				"error", err, "batch", len(req.Targets))
		}
		return catalog.EnrichResponse{Fallback: true}, err
	}
	if resp == nil || resp.Content == "" {
		return catalog.EnrichResponse{Fallback: true}, errors.New("catalog gateway: empty enrichment response")
	}

	decoded, err := catalog.DecodeEnrichResponse(resp.Content)
	if err != nil {
		e.log.WarnContext(ctx, "enrichment response unreadable",
			"error", err, "request_id", resp.RequestID, "model", resp.Model)
		return catalog.EnrichResponse{Fallback: true}, err
	}

	e.log.InfoContext(ctx, "catalogue enrichment batch complete",
		"products", len(req.Targets), "answers", len(decoded.Results),
		"request_id", resp.RequestID, "model", resp.Model, "cached", resp.FromCache)
	return decoded, nil
}

func (e *Enricher) virtualKey(ctx context.Context, orgID int64) string {
	if e.keyResolver == nil || orgID <= 0 {
		return ""
	}
	key, err := e.keyResolver(ctx, orgID)
	if err != nil {
		e.log.WarnContext(ctx, "tenant gateway key unavailable, using platform key",
			"org_id", orgID, "error", err)
		return ""
	}
	return key
}
