package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// AI-assisted matching.
//
// Deterministic matching is exact: a SKU, or a name that folds to the same
// string. It is right when it fires and silent when it does not, and on a real
// supplier file it is silent often — the same product arrives as "اوجمنتين 1
// جم اقراص" from one distributor and "أوجمنتين 1جم 14 قرص" from the next, and
// an importer that cannot see they are the same creates a duplicate.
//
// Trigram similarity finds those near misses; it cannot decide them. "بانادول
// اكسترا" and "بانادول نايت" score highly against each other and are different
// medicines, while "اوجمنتين 1 جم" and "أوجمنتين 1جم 14 قرص" score lower and are
// the same one. That judgement is what the model is asked for, and only for the
// rows that are genuinely ambiguous — an exact match never reaches here, and a
// name with no similar product in the catalogue never does either.

// MatchCandidate is an existing catalogue product a new row might be.
type MatchCandidate struct {
	ProductID     int64   `json:"id"`
	Name          string  `json:"name"`
	SKU           string  `json:"sku,omitempty"`
	Manufacturer  string  `json:"manufacturer,omitempty"`
	DosageForm    string  `json:"dosage_form,omitempty"`
	Concentration string  `json:"concentration,omitempty"`
	Similarity    float64 `json:"similarity"`
}

// matchCertain is the similarity above which a candidate is taken without
// asking. Two names this close, after Arabic folding, differ by punctuation or
// a word order the folding already handles.
const matchCertain = 0.92

// matchFloor is the similarity below which a candidate is not worth a question.
// Under this, trigram overlap is coincidence — shared brand prefixes and common
// pharmaceutical words.
const matchFloor = 0.55

// maxMatchQuestions bounds how many ambiguous rows are sent for adjudication in
// one import.
//
// A file whose every row is ambiguous is a file whose column mapping is wrong,
// and the answer to that is the review screen, not a thousand model calls. The
// bound keeps a misconfigured import cheap.
const maxMatchQuestions = 400

// maxCandidatesPerRow is how many alternatives the model sees per product.
// Three is enough to choose between; more is prompt weight for options that
// were never going to win.
const maxCandidatesPerRow = 3

// MatchQuestion is one ambiguous row with the alternatives it might be.
type MatchQuestion struct {
	Ref        int              `json:"ref"`
	Name       string           `json:"name"`
	SKU        string           `json:"sku,omitempty"`
	Maker      string           `json:"manufacturer,omitempty"`
	Candidates []MatchCandidate `json:"candidates"`
}

// MatchDecision is the model's answer for one ambiguous row.
type MatchDecision struct {
	Ref int `json:"ref"`
	// ProductID is the catalogue product this row is, or 0 for "none of these".
	ProductID  int64   `json:"product_id"`
	Confidence float64 `json:"confidence"`
	Reason     string  `json:"reason,omitempty"`
}

// MatchAdjudication is one batch of decisions.
type MatchAdjudication struct {
	Decisions []MatchDecision `json:"decisions"`
}

// matchSystemPrompt asks for identity, not similarity.
//
// The distinction is the entire value of the call: string similarity is what
// produced the candidates, and repeating it would add nothing. What a model can
// do that trigrams cannot is know that Augmentin 1g and Augmentin 1g in a
// 14-tablet pack are one product, while Panadol Extra and Panadol Night are not.
const matchSystemPromptText = `You match Egyptian pharmacy products against an existing catalogue.

For each product you are given a name and up to three candidate catalogue entries. Decide whether the product IS one of the candidates — the same product, possibly written differently, in a different pack size, or with a different word order.

Return product_id of the matching candidate, or 0 when none of them is the same product.

Rules:
- Same active substance, same strength, same form, same brand = the same product, even when pack size or wording differs.
- A different strength (500mg vs 1g), a different form (tablets vs syrup), or a different variant (Extra vs Night, adult vs children) is a DIFFERENT product. Return 0.
- A different manufacturer for the same brand name is usually still a different product. Return 0.
- When unsure, return 0. A wrong match overwrites a real catalogue entry with someone else's data, which is far worse than creating a new one.
- confidence is 0.0 to 1.0. reason is at most three Arabic words.

Respond with ONLY JSON: {"decisions":[{"ref":0,"product_id":0,"confidence":0.0,"reason":""}]}`

// MatchSystemPrompt is the instruction the matching capability runs under.
func MatchSystemPrompt() string { return matchSystemPromptText }

// MatchSchema constrains the model to the shape DecodeMatchDecisions expects.
func MatchSchema() map[string]any {
	return map[string]any{
		"name":   "catalog_match",
		"strict": false,
		"schema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"decisions": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"ref":        map[string]any{"type": "integer"},
							"product_id": map[string]any{"type": "integer"},
							"confidence": map[string]any{"type": "number"},
							"reason":     map[string]any{"type": "string"},
						},
						"required": []string{"ref", "product_id"},
					},
				},
			},
			"required": []string{"decisions"},
		},
	}
}

// Matcher adjudicates ambiguous product matches. Like Enricher it is a port, so
// the catalogue depends on the capability rather than on a transport.
type Matcher interface {
	AdjudicateMatches(ctx context.Context, req MatchRequest) (MatchAdjudication, error)
}

// MatchRequest is one batch of ambiguous rows.
type MatchRequest struct {
	Questions      []MatchQuestion `json:"products"`
	OrganizationID int64           `json:"-"`
	UserID         int64           `json:"-"`
}

// EncodeMatchInput renders a batch as the model's user message.
func EncodeMatchInput(req MatchRequest) (string, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("catalog: encode match request: %w", err)
	}
	return string(body), nil
}

// DecodeMatchDecisions reads the model's answer, tolerating markdown fences.
func DecodeMatchDecisions(content string) (MatchAdjudication, error) {
	clean := strings.TrimSpace(content)
	clean = strings.TrimPrefix(clean, "```json")
	clean = strings.TrimPrefix(clean, "```")
	clean = strings.TrimSuffix(clean, "```")
	clean = strings.TrimSpace(clean)

	var out MatchAdjudication
	if err := json.Unmarshal([]byte(clean), &out); err != nil {
		return MatchAdjudication{}, fmt.Errorf("catalog: decode match decisions: %w", err)
	}
	return out, nil
}

// BuildMatchQuestions selects the rows worth adjudicating.
//
// A row qualifies when deterministic matching found nothing, similarity found
// something plausible, and that something is not so close it can be taken
// outright. Everything else is decided without a model.
func BuildMatchQuestions(
	prods []*Product, matched map[int]ExistingMatch, candidates map[int][]MatchCandidate,
) (questions []MatchQuestion, certain map[int]ExistingMatch) {
	certain = map[int]ExistingMatch{}

	for idx, options := range candidates {
		if _, already := matched[idx]; already || idx < 0 || idx >= len(prods) {
			continue
		}
		if len(options) == 0 {
			continue
		}

		// A near-identical name needs no adjudication; taking it here keeps the
		// question count down to the rows that are genuinely in doubt.
		if options[0].Similarity >= matchCertain {
			certain[idx] = ExistingMatch{ProductID: options[0].ProductID, Reason: MatchName}
			continue
		}
		if options[0].Similarity < matchFloor {
			continue
		}
		if len(options) > maxCandidatesPerRow {
			options = options[:maxCandidatesPerRow]
		}

		p := prods[idx]
		questions = append(questions, MatchQuestion{
			Ref:        idx,
			Name:       p.Name.Get(i18n.AR),
			SKU:        p.SKU,
			Maker:      p.ManufacturingCompanies,
			Candidates: options,
		})
		if len(questions) >= maxMatchQuestions {
			break
		}
	}
	return questions, certain
}

// ApplyMatchDecisions turns accepted decisions into matches.
//
// A decision is only honoured when the model both named a candidate it was
// actually offered and was confident about it. Anything else leaves the row as
// a new product, which is the safe outcome: a spurious insert is visible in the
// review table, while a spurious update silently overwrites a real entry.
func ApplyMatchDecisions(
	decisions []MatchDecision, questions []MatchQuestion, matched map[int]ExistingMatch,
) int {
	offered := make(map[int]map[int64]bool, len(questions))
	for _, q := range questions {
		ids := make(map[int64]bool, len(q.Candidates))
		for _, c := range q.Candidates {
			ids[c.ProductID] = true
		}
		offered[q.Ref] = ids
	}

	applied := 0
	for _, decision := range decisions {
		if decision.ProductID <= 0 {
			continue
		}
		if decision.Confidence > 0 && decision.Confidence < minAIConfidence {
			continue
		}
		if !offered[decision.Ref][decision.ProductID] {
			// The model named a product it was not shown. Ignoring it is the
			// only safe reading: the id may not exist, or may be someone else's
			// row entirely.
			continue
		}
		if _, already := matched[decision.Ref]; already {
			continue
		}
		matched[decision.Ref] = ExistingMatch{ProductID: decision.ProductID, Reason: MatchAI}
		applied++
	}
	return applied
}

// matchBatchSize is how many ambiguous rows go in one call. Smaller than the
// enrichment batch because each question carries three candidates with it.
const matchBatchSize = 20

// MatchBatches splits the questions into Gateway-sized groups.
func MatchBatches(questions []MatchQuestion) [][]MatchQuestion {
	var out [][]MatchQuestion
	for start := 0; start < len(questions); start += matchBatchSize {
		out = append(out, questions[start:min(start+matchBatchSize, len(questions))])
	}
	return out
}
