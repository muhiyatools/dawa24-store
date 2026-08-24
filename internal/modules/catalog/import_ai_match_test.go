package catalog_test

import (
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
)

func candidate(id int64, name string, sim float64) catalog.MatchCandidate {
	return catalog.MatchCandidate{ProductID: id, Name: name, Similarity: sim}
}

// A near-identical name is taken without spending a model call on it.
func TestBuildMatchQuestionsTakesCertainMatchesWithoutAsking(t *testing.T) {
	prods := []*catalog.Product{productNamed("أوجمنتين 1 جم أقراص")}
	candidates := map[int][]catalog.MatchCandidate{
		0: {candidate(4812, "اوجمنتين 1 جم اقراص", 0.97)},
	}

	questions, certain := catalog.BuildMatchQuestions(prods, map[int]catalog.ExistingMatch{}, candidates)

	if len(questions) != 0 {
		t.Errorf("asked %d questions about a near-identical name; it should be taken outright", len(questions))
	}
	if got, ok := certain[0]; !ok || got.ProductID != 4812 {
		t.Errorf("certain match = %+v, want product 4812", got)
	}
}

// Coincidental trigram overlap is not worth a question.
func TestBuildMatchQuestionsIgnoresWeakSimilarity(t *testing.T) {
	prods := []*catalog.Product{productNamed("منتج مختلف تماماً")}
	candidates := map[int][]catalog.MatchCandidate{
		0: {candidate(11, "شيء آخر", 0.31)},
	}

	questions, certain := catalog.BuildMatchQuestions(prods, map[int]catalog.ExistingMatch{}, candidates)
	if len(questions) != 0 || len(certain) != 0 {
		t.Errorf("a 0.31 similarity produced %d questions and %d matches; both should be zero",
			len(questions), len(certain))
	}
}

func TestBuildMatchQuestionsAsksAboutTheAmbiguousBand(t *testing.T) {
	prods := []*catalog.Product{productNamed("بانادول اكسترا 500")}
	candidates := map[int][]catalog.MatchCandidate{
		0: {
			candidate(21, "بانادول نايت 500", 0.78),
			candidate(22, "بانادول اكسترا اقراص", 0.72),
		},
	}

	questions, _ := catalog.BuildMatchQuestions(prods, map[int]catalog.ExistingMatch{}, candidates)
	if len(questions) != 1 {
		t.Fatalf("asked %d questions, want 1", len(questions))
	}
	if len(questions[0].Candidates) != 2 {
		t.Errorf("question carries %d candidates, want 2", len(questions[0].Candidates))
	}
}

// A row exact matching already settled must never be re-decided.
func TestBuildMatchQuestionsSkipsAlreadyMatchedRows(t *testing.T) {
	prods := []*catalog.Product{productNamed("بانادول اكسترا")}
	matched := map[int]catalog.ExistingMatch{0: {ProductID: 99, Reason: catalog.MatchSKU}}
	candidates := map[int][]catalog.MatchCandidate{0: {candidate(21, "بانادول نايت", 0.8)}}

	questions, certain := catalog.BuildMatchQuestions(prods, matched, candidates)
	if len(questions) != 0 || len(certain) != 0 {
		t.Error("a row matched by SKU was sent for adjudication")
	}
}

// A wrong match silently overwrites a real catalogue entry, so a decision is
// only honoured when the model chose something it was actually offered.
func TestApplyMatchDecisionsRefusesUnofferedProducts(t *testing.T) {
	questions := []catalog.MatchQuestion{{
		Ref:        0,
		Candidates: []catalog.MatchCandidate{candidate(21, "بانادول نايت", 0.8)},
	}}
	matched := map[int]catalog.ExistingMatch{}

	applied := catalog.ApplyMatchDecisions([]catalog.MatchDecision{
		{Ref: 0, ProductID: 9999, Confidence: 0.99},
	}, questions, matched)

	if applied != 0 || len(matched) != 0 {
		t.Errorf("a product the model was never shown was accepted: applied=%d matched=%v", applied, matched)
	}
}

func TestApplyMatchDecisionsHonoursConfidentChoices(t *testing.T) {
	questions := []catalog.MatchQuestion{{
		Ref: 0,
		Candidates: []catalog.MatchCandidate{
			candidate(21, "بانادول نايت", 0.8),
			candidate(22, "بانادول اكسترا اقراص", 0.72),
		},
	}}
	matched := map[int]catalog.ExistingMatch{}

	applied := catalog.ApplyMatchDecisions([]catalog.MatchDecision{
		{Ref: 0, ProductID: 22, Confidence: 0.9, Reason: "نفس الصنف"},
	}, questions, matched)

	if applied != 1 {
		t.Fatalf("applied %d decisions, want 1", applied)
	}
	if matched[0].ProductID != 22 || matched[0].Reason != catalog.MatchAI {
		t.Errorf("match = %+v, want product 22 attributed to AI", matched[0])
	}
}

func TestApplyMatchDecisionsDiscardsLowConfidence(t *testing.T) {
	questions := []catalog.MatchQuestion{{
		Ref:        0,
		Candidates: []catalog.MatchCandidate{candidate(21, "بانادول نايت", 0.8)},
	}}
	matched := map[int]catalog.ExistingMatch{}

	if applied := catalog.ApplyMatchDecisions([]catalog.MatchDecision{
		{Ref: 0, ProductID: 21, Confidence: 0.2},
	}, questions, matched); applied != 0 {
		t.Error("a low-confidence match was written")
	}
}

// "None of these" is the expected answer for a genuinely new product.
func TestApplyMatchDecisionsAcceptsNoMatch(t *testing.T) {
	questions := []catalog.MatchQuestion{{
		Ref:        0,
		Candidates: []catalog.MatchCandidate{candidate(21, "بانادول نايت", 0.8)},
	}}
	matched := map[int]catalog.ExistingMatch{}

	if applied := catalog.ApplyMatchDecisions([]catalog.MatchDecision{
		{Ref: 0, ProductID: 0, Confidence: 0.95},
	}, questions, matched); applied != 0 {
		t.Error("a zero product id was treated as a match")
	}
	if len(matched) != 0 {
		t.Error("the row was matched despite the model declining")
	}
}

func TestDecodeMatchDecisionsToleratesFences(t *testing.T) {
	body := "```json\n{\"decisions\":[{\"ref\":2,\"product_id\":77,\"confidence\":0.9}]}\n```"

	out, err := catalog.DecodeMatchDecisions(body)
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if len(out.Decisions) != 1 || out.Decisions[0].ProductID != 77 {
		t.Fatalf("decoded %+v, want one decision for product 77", out.Decisions)
	}
}
