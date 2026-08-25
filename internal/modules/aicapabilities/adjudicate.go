package aicapabilities

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/gateway"
)

// Batch product-match adjudication.
//
// CapProductMatch answers one row at a time, which is right for a manual
// correction and ruinous for an import: ten thousand rows would be ten thousand
// requests. This capability answers a batch, and the batch carries its own
// candidates — the model is never given a way to look anything up.
//
// The response contract is deliberately narrow. The model returns a product id
// *from the list it was given*, or null. It does not return a name to be matched
// again, because that would reintroduce the ambiguity the shortlist removed.

// CapMatchAdjudicate is the capability name.
const CapMatchAdjudicate = "matching.adjudicate"

// AdjudicateItem is one ambiguous row with its shortlist.
type AdjudicateItem struct {
	LineID     int64                 `json:"line_id"`
	Text       string                `json:"text"`
	Candidates []AdjudicateCandidate `json:"candidates"`
}

// AdjudicateCandidate is a catalogue product offered as an option.
type AdjudicateCandidate struct {
	ProductID     int64  `json:"product_id"`
	Name          string `json:"name"`
	NameEN        string `json:"name_en,omitempty"`
	Scientific    string `json:"scientific,omitempty"`
	DosageForm    string `json:"dosage_form,omitempty"`
	Concentration string `json:"concentration,omitempty"`
	Manufacturer  string `json:"manufacturer,omitempty"`
}

// AdjudicateDecision is one answer.
type AdjudicateDecision struct {
	LineID     int64   `json:"line_id"`
	ProductID  *int64  `json:"product_id"`
	Confidence float64 `json:"confidence"`
	Reason     string  `json:"reason,omitempty"`
}

// adjudicateSystemPrompt is versioned with pipeline.PromptVersion. Changing it
// without bumping that constant would silently reuse cached answers to a
// different question.
const adjudicateSystemPrompt = `You match pharmacy order lines to a pharmaceutical catalogue.

For each item you are given the text a pharmacy typed and a short list of catalogue products. Choose the ONE product that is the same medicine, or null if none of them is.

Rules, in order of importance:
1. Strength must match. 500 mg and 1 g are different products. If the text states a strength and no candidate has it, answer null.
2. Dosage form must be compatible. A syrup is not a tablet, an injection is not a cream.
3. Brand and generic names of the same medicine at the same strength and form ARE the same product.
4. Pack size differences are acceptable unless the text states one explicitly and no candidate matches it.
5. If two candidates fit equally well, answer null. A wrong confident match is worse than no match.

You MUST choose a product_id from the candidates given for that item, or null. Never invent an id.

Return ONLY a JSON object:
{"results":[{"line_id":<number>,"product_id":<number or null>,"confidence":<0.0-1.0>,"reason":"<short Arabic explanation>"}]}

Return one result for every item you were given.`

// AdjudicateBatch resolves a batch of ambiguous rows.
//
// There is no deterministic fallback here, and that is deliberate: the
// deterministic engine has already run and produced the outcome this is trying
// to improve on. Returning an error leaves that outcome standing, which is the
// correct degradation (AGENTS.md R3 — the fallback exists, it is the caller's
// prior result).
func (s *Service) AdjudicateBatch(ctx context.Context, items []AdjudicateItem) ([]AdjudicateDecision, error) {
	// Nothing to adjudicate is a no-op, whatever the gateway's state. Checking
	// this first means the pipeline's "no residual lines" case never surfaces as
	// a configuration error.
	if len(items) == 0 {
		return nil, nil
	}
	if s.gw == nil {
		return nil, fmt.Errorf("aicapabilities: gateway not configured")
	}

	var orgID, userID int64
	if actor, ok := authctx.From(ctx); ok {
		orgID = actor.OrgID
		if orgID <= 0 {
			orgID = actor.OrganizationID
		}
		userID = actor.UserID
	}

	var vKey string
	if s.keyResolver != nil && orgID > 0 {
		if k, err := s.keyResolver(ctx, orgID); err == nil && k != "" {
			vKey = k
		}
	}

	payload, err := json.Marshal(struct {
		Items []AdjudicateItem `json:"items"`
	}{Items: items})
	if err != nil {
		return nil, fmt.Errorf("aicapabilities: marshal adjudication batch: %w", err)
	}

	resp, err := s.gw.Invoke(ctx, gateway.Request{
		Capability:     gateway.CapMatchAdjudicate,
		System:         adjudicateSystemPrompt,
		Input:          string(payload),
		OrganizationID: orgID,
		UserID:         userID,
		VirtualKey:     vKey,
	})
	if err != nil {
		s.log.WarnContext(ctx, "adjudication batch failed; deterministic results stand",
			"items", len(items), "err", err)
		return nil, err
	}
	if resp == nil || resp.Content == "" {
		return nil, fmt.Errorf("aicapabilities: empty adjudication response")
	}

	decisions, err := decodeDecisions(resp.Content)
	if err != nil {
		s.log.WarnContext(ctx, "could not read adjudication response",
			"items", len(items), "err", err)
		return nil, err
	}

	s.log.InfoContext(ctx, "adjudication batch resolved",
		"items", len(items), "decisions", len(decisions),
		"req_id", resp.RequestID, "model", resp.Model)
	return decisions, nil
}

// decodeDecisions parses the response, tolerating the code fences models add.
func decodeDecisions(content string) ([]AdjudicateDecision, error) {
	clean := strings.TrimSpace(content)
	clean = strings.TrimPrefix(clean, "```json")
	clean = strings.TrimPrefix(clean, "```")
	clean = strings.TrimSuffix(clean, "```")
	clean = strings.TrimSpace(clean)

	var wrapper struct {
		Results []AdjudicateDecision `json:"results"`
	}
	if err := json.Unmarshal([]byte(clean), &wrapper); err != nil {
		return nil, fmt.Errorf("aicapabilities: decode adjudication: %w", err)
	}
	return wrapper.Results, nil
}
