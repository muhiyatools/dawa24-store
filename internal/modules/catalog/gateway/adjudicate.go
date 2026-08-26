package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/platform/gateway"
)

// Batch match adjudication for the catalogue imports.
//
// The import's other three AI calls are questions about the *file* — which
// column is which, what a category word means — asked once per import. This one
// is a question about rows, which is exactly the shape that gets expensive, so
// three rules bound it and all three are enforced by the caller in
// import_match.go:
//
//   - never per row: rows are batched, and each batch carries its own shortlist;
//   - never with catalogue access: the batch carries everything the decision
//     needs, so the model has no way to go looking;
//   - never unbounded: the caller stops after a fixed number of batches and
//     leaves the rest with their deterministic outcome.
//
// The response contract is narrow on purpose. The model returns an id *from the
// list it was given*, or null. It never returns a name to be matched again,
// because that would put back the ambiguity the shortlist removed.

// adjudicateSystemPrompt is the instruction the tier runs under.
//
// It is written for a catalogue import rather than for an order: the question
// is "is this row the same product as one of these catalogue entries", and the
// expensive mistake is a false yes, which silently overwrites a real product.
const adjudicateSystemPrompt = `You decide whether a row from a supplier's spreadsheet is the SAME pharmaceutical product as one of the catalogue entries offered for it.

For each item you are given the text of the incoming row and a short list of catalogue products. Choose the ONE product that is the same product, or null if none of them is.

Rules, in order of importance:
1. Strength must match. 500 mg and 1 g are different products. If the row states a strength and no candidate has it, answer null.
2. Dosage form must be compatible. A syrup is not a tablet, a cream is not an injection.
3. Different spellings, transliterations and word orders of the same brand ARE the same product. Arabic and English names of the same product match.
4. A brand and its generic at the same strength and form are the same product.
5. Pack size differences are acceptable unless the row states one explicitly and no candidate matches it.
6. If two candidates fit equally well, answer null. A wrong confident match overwrites a real catalogue entry; an unmatched row is merely added as new.

You MUST choose a product_id from the candidates given for that item, or null. Never invent an id.

Return ONLY a JSON object:
{"results":[{"ref":<number>,"product_id":<number or null>,"confidence":<0.0-1.0>,"reason":"<short Arabic explanation>"}]}

Return one result for every item you were given.`

// adjudicateSchema constrains the model to the shape the parser expects.
func adjudicateSchema() map[string]any {
	return map[string]any{
		"name":   "match_adjudication",
		"strict": false,
		"schema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"results": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"ref":        map[string]any{"type": "integer"},
							"product_id": map[string]any{"type": []string{"integer", "null"}},
							"confidence": map[string]any{"type": "number"},
							"reason":     map[string]any{"type": "string"},
						},
						"required": []string{"ref", "product_id"},
					},
				},
			},
			"required": []string{"results"},
		},
	}
}

// wireItem is one row as it goes over the wire. The field names are short and
// stable because they are part of the prompt contract.
type wireItem struct {
	Ref        int64           `json:"ref"`
	Text       string          `json:"text"`
	Candidates []wireCandidate `json:"candidates"`
}

type wireCandidate struct {
	ProductID     int64  `json:"product_id"`
	Name          string `json:"name"`
	NameEN        string `json:"name_en,omitempty"`
	Scientific    string `json:"scientific,omitempty"`
	DosageForm    string `json:"dosage_form,omitempty"`
	Concentration string `json:"concentration,omitempty"`
	Manufacturer  string `json:"manufacturer,omitempty"`
}

type wireDecision struct {
	Ref        int64   `json:"ref"`
	ProductID  *int64  `json:"product_id"`
	Confidence float64 `json:"confidence"`
	Reason     string  `json:"reason"`
}

// AdjudicateMatches resolves one batch of ambiguous rows.
//
// An error here is never fatal to the import: the caller keeps whatever the
// deterministic tiers decided, which is a correct if less generous answer. That
// is the whole reason the AI tier can be switched off without changing what the
// importer is able to do.
func (m *Mapper) AdjudicateMatches(
	ctx context.Context, req catalog.MatchAdjudicationRequest,
) ([]catalog.MatchAdjudicationResult, error) {
	items := req.Items
	if len(items) == 0 {
		return nil, nil
	}
	if m == nil || m.client == nil {
		return nil, gateway.ErrDisabled
	}

	payload := struct {
		Items []wireItem `json:"items"`
	}{Items: make([]wireItem, 0, len(items))}
	for _, it := range items {
		w := wireItem{Ref: it.Ref, Text: it.Text}
		for _, c := range it.Candidates {
			w.Candidates = append(w.Candidates, wireCandidate{
				ProductID:     c.ProductID,
				Name:          c.Name,
				NameEN:        c.NameEN,
				Scientific:    c.Scientific,
				DosageForm:    c.DosageForm,
				Concentration: c.Concentration,
				Manufacturer:  c.Manufacturer,
			})
		}
		payload.Items = append(payload.Items, w)
	}

	content, err := m.invoke(ctx, aiCall{
		capability: gateway.CapMatchAdjudicate,
		system:     adjudicateSystemPrompt,
		schema:     adjudicateSchema(),
		payload:    payload,
		orgID:      req.OrganizationID,
		userID:     req.UserID,
		label:      "match adjudication",
	})
	if err != nil {
		return nil, err
	}

	decisions, err := decodeAdjudication(content)
	if err != nil {
		m.log.WarnContext(ctx, "match adjudication response unreadable",
			"items", len(items), "error", err)
		return nil, err
	}

	out := make([]catalog.MatchAdjudicationResult, 0, len(decisions))
	for _, d := range decisions {
		out = append(out, catalog.MatchAdjudicationResult{
			Ref:        d.Ref,
			ProductID:  d.ProductID,
			Confidence: d.Confidence,
			Reason:     d.Reason,
		})
	}
	m.log.InfoContext(ctx, "match adjudication resolved",
		"items", len(items), "decisions", len(out))
	return out, nil
}

// decodeAdjudication parses the response, tolerating the code fences models
// wrap JSON in often enough that handling them is part of parsing.
func decodeAdjudication(content string) ([]wireDecision, error) {
	clean := strings.TrimSpace(content)
	clean = strings.TrimPrefix(clean, "```json")
	clean = strings.TrimPrefix(clean, "```")
	clean = strings.TrimSuffix(clean, "```")

	var wrapper struct {
		Results []wireDecision `json:"results"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(clean)), &wrapper); err != nil {
		return nil, fmt.Errorf("catalog gateway: decode match adjudication: %w", err)
	}
	return wrapper.Results, nil
}
