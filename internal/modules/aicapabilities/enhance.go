package aicapabilities

// Match enhancement.
//
// This capability answers one question and only one: for a pharmacy line the
// deterministic engine could not settle, which product in the catalogue window
// supplied with the request is the same medicine — or none of them?
//
// Everything about its shape follows from three facts.
//
// **The catalogue does not fit in a prompt.** Twenty thousand products at forty
// Arabic characters each is roughly a megabyte and several hundred thousand
// tokens, re-sent on every import of every pharmacy. So the request carries a
// *window*: the union of the candidates retrieved for the lines in it. Two
// hundred lines about antihypertensives retrieve heavily overlapping products,
// and de-duplicating that union is what turns a five-thousand-row catalogue
// block into a fifteen-hundred-row one.
//
// **The window is shared, not per item.** Each item names its own shortlist by
// id, but the model may answer with any id in the window. That costs nothing
// extra and repairs the most common retrieval failure there is: the right
// product was retrieved — for the line above.
//
// **The prompt is generated, never authored by a model.** RenderEnhanceInput is
// a pure function of the request. The same lines and the same window always
// produce byte-identical input, which is what makes the decision cache sound and
// what makes a prompt regression show up in a diff rather than in a bill.

import (
	"context"
	"errors"
	"strings"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/gateway"
)

// EnhancePromptVersion changes whenever the rendered input or the system prompt
// changes. It is part of the decision cache key, so a prompt change orphans the
// old answers instead of silently reusing answers to a different question.
const EnhancePromptVersion = "sm-enh-v3"

// CatalogEntry is one catalogue product as the model sees it.
//
// Arabic name first, because that is what the pharmacy wrote and what the
// catalogue is authored in. The English name is carried too and earns its
// tokens: transliteration is where Arabic pharmacy matching actually fails, and
// "ابليفاى" against "ابيليفاي" is obvious the moment both rows show `abilify`.
type CatalogEntry struct {
	ProductID     int64
	NameAR        string
	NameEN        string
	Scientific    string
	DosageForm    string
	Concentration string
	Manufacturer  string
}

// EnhanceItem is one line the deterministic engine could not settle.
type EnhanceItem struct {
	// Ref identifies the item inside this request. It is the request-local
	// index, not a database id: line ids are the caller's business and there is
	// no reason to send them to a third party.
	Ref int
	// Text is exactly what the pharmacy typed, unedited. The model needs the
	// noise — "س.ج 141ج" is a price, not a strength, and only the raw text says
	// so.
	Text string

	// The remaining fields are what the deterministic decomposition extracted.
	// They are hints, not constraints: the decomposition is sometimes wrong,
	// which is part of why the line reached here at all.
	Brand        string
	Strength     string
	DosageForm   string
	PackSize     int
	Manufacturer string
	Scientific   string
	SKU          string
	Barcode      string
	CurrentGuess *int64
	CurrentScore float64
	Options      []int64
}

// EnhanceRequest is one batch: a catalogue window and the lines to resolve
// against it.
type EnhanceRequest struct {
	Catalog []CatalogEntry
	Items   []EnhanceItem
}

// EnhanceDecision is one answer. A nil ProductID means "none of these", which is
// a correct and useful answer and is recorded as such.
type EnhanceDecision struct {
	Ref        int     `json:"ref"`
	ProductID  *int64  `json:"product_id"`
	Confidence float64 `json:"confidence"`
	Reason     string  `json:"reason,omitempty"`
}

// enhanceSchema constrains the response at the protocol level where the Gateway
// supports it. It is a belt to the prompt's braces: the prompt still specifies
// the shape in full, because a Gateway or model that ignores the schema must
// still produce something parseable.
func enhanceSchema() map[string]any {
	return map[string]any{
		"name":   "dawa24_match_enhancement",
		"strict": true,
		"schema": map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"required":             []string{"results"},
			"properties": map[string]any{
				"results": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type":                 "object",
						"additionalProperties": false,
						"required":             []string{"ref", "product_id", "confidence", "reason"},
						"properties": map[string]any{
							"ref":        map[string]any{"type": "integer"},
							"product_id": map[string]any{"type": []string{"integer", "null"}},
							"confidence": map[string]any{"type": "number"},
							"reason":     map[string]any{"type": "string"},
						},
					},
				},
			},
		},
	}
}

// EnhanceMatches resolves one batch.
//
// There is no deterministic fallback, and that is deliberate: the deterministic
// engine has already run and produced the result this is trying to improve on.
// Returning an error leaves that result standing, which is the correct
// degradation — a pharmacy must be able to order when the Gateway is down.
func (s *Service) EnhanceMatches(ctx context.Context, req EnhanceRequest) ([]EnhanceDecision, error) {
	if len(req.Items) == 0 {
		return nil, nil
	}
	if s.gw == nil {
		return nil, errors.New("aicapabilities: gateway not configured")
	}
	if len(req.Catalog) == 0 {
		// Nothing to choose from. Asking anyway would invite the model to
		// invent an id, which is the one failure that must never happen.
		return nil, errors.New("aicapabilities: empty catalogue window")
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

	gwReq := gateway.Request{
		Capability:     gateway.CapMatchEnhance,
		System:         enhanceSystemPrompt,
		Input:          RenderEnhanceInput(req),
		Schema:         enhanceSchema(),
		OrganizationID: orgID,
		UserID:         userID,
		VirtualKey:     vKey,
		MaxTokens:      enhanceMaxTokens(len(req.Items)),
	}

	resp, err := s.gw.Invoke(ctx, gwReq)
	if err != nil && isSchemaRejection(err) {
		// Not every Gateway deployment or model supports json_schema response
		// formats. Losing the whole stage over a protocol nicety would be a
		// poor trade when the prompt already specifies the shape in full.
		s.log.WarnContext(ctx, "gateway rejected structured output; retrying without schema",
			"capability", gateway.CapMatchEnhance, "err", err)
		gwReq.Schema = nil
		resp, err = s.gw.Invoke(ctx, gwReq)
	}
	if err != nil {
		s.log.WarnContext(ctx, "match enhancement failed; deterministic results stand",
			"items", len(req.Items), "catalog", len(req.Catalog), "err", err)
		return nil, err
	}
	if resp == nil || resp.Content == "" {
		return nil, errors.New("aicapabilities: empty enhancement response")
	}

	decisions, err := DecodeEnhanceResponse(resp.Content)
	if err != nil {
		s.log.WarnContext(ctx, "could not read enhancement response",
			"items", len(req.Items), "err", err)
		return nil, err
	}

	s.log.InfoContext(ctx, "match enhancement resolved",
		"items", len(req.Items), "catalog", len(req.Catalog), "decisions", len(decisions),
		"req_id", resp.RequestID, "model", resp.Model,
		"in_tok", resp.InputTok, "out_tok", resp.OutputTok)
	return decisions, nil
}

// enhanceMaxTokens sizes the completion budget to the batch.
//
// A budget that is too small truncates the JSON mid-array and loses every
// decision in the batch, including the ones already written; one that is too
// large costs nothing, because the model stops when it stops. So this is
// deliberately generous.
func enhanceMaxTokens(items int) int {
	n := items*80 + 1024
	if n > 60000 {
		n = 60000
	}
	return n
}

// isSchemaRejection reports whether an error looks like the Gateway or the model
// refusing the structured-output request rather than failing the work itself.
//
// Any bad request counts, not only one that names the schema. Structured output
// is the only optional, model-dependent thing in this call — everything else is
// a chat completion the Gateway has always accepted — so a 400 after sending one
// is overwhelmingly likely to be about it, and models differ in whether they say
// so. Retrying without the schema costs one request and recovers the whole
// stage; not retrying loses every decision in the batch because a model does not
// implement a protocol nicety. If the 400 was about something else the retry
// fails identically and the deterministic result stands, which is where this was
// heading anyway.
func isSchemaRejection(err error) bool {
	if err == nil || errors.Is(err, gateway.ErrDisabled) {
		return false
	}
	if errors.Is(err, gateway.ErrBadRequest) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "response_format") ||
		strings.Contains(msg, "json_schema") ||
		strings.Contains(msg, "schema")
}
