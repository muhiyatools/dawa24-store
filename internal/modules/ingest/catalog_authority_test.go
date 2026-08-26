package ingest

import (
	"strings"
	"testing"

	"github.com/muhiya/dawa24-store/internal/shared/productmatch"
)

// parsedRow is the minimum a decision needs: a row with a name.
func parsedRow(name string) *productmatch.Row {
	return &productmatch.Row{Number: 2, Name: name}
}

// The shared catalogue belongs to the administrator.
//
// A vendor's import matches against it and prices what it finds. It does not
// extend it. That used to be a setting — `UnmatchedCreate`, on by default — and
// the result was a catalogue made of whatever any supplier happened to type:
// thousands of near-duplicate entries, each one a product no other vendor's row
// could ever match, which is the opposite of what a shared catalogue is for.
//
// These tests are cheap and they exist because the change is easy to undo by
// accident: the writer has a decision struct with a product on it, and adding
// "just create it when nothing matches" back is a two-line edit.

// An unmatched row is reported, never invented into the catalogue.
func TestUnmatchedRowsAreSkippedNotCreated(t *testing.T) {
	w := &importWriter{settings: DefaultSettings()}
	d := &decision{row: parsedRow("صنف لا وجود له")}

	w.settle([]*decision{d})

	if d.outcome != OutcomeSkipped {
		t.Fatalf("outcome = %q, want %q", d.outcome, OutcomeSkipped)
	}
	if d.productID != 0 {
		t.Errorf("an unmatched row acquired product %d", d.productID)
	}
	if w.counts.skipped != 1 {
		t.Errorf("skipped = %d, want 1", w.counts.skipped)
	}
}

// The message has to be actionable. "Skipped" alone leaves a vendor who stocks
// the product with nothing to do about it.
func TestUnmatchedMessageSaysWhatToDo(t *testing.T) {
	w := &importWriter{settings: DefaultSettings()}
	d := &decision{row: parsedRow("صنف لا وجود له")}
	w.settle([]*decision{d})

	for _, want := range []string{"الكتالوج المعتمد", "الباركود"} {
		if !strings.Contains(d.message, want) {
			t.Errorf("message %q does not mention %q", d.message, want)
		}
	}
}

// A matched row still resolves onto the vendor's own variant and is written.
// The rule removes creation, not matching.
func TestMatchedRowsStillResolveToAVariant(t *testing.T) {
	w := &importWriter{
		settings: DefaultSettings(),
		variants: newVariantIndex(nil),
	}
	d := &decision{row: parsedRow("بروفين"), productID: 4242}

	w.settle([]*decision{d})

	if d.outcome != "" {
		t.Fatalf("a matched row was given outcome %q", d.outcome)
	}
	if d.productID != 4242 {
		t.Errorf("product id = %d, want 4242", d.productID)
	}
}

// AI is on by default. It is the tier that decides the match rate, and it
// cannot invent a product — it only picks among candidates the deterministic
// engine already retrieved — so there is no reason to make a vendor find a
// checkbox for it.
func TestAIMatchingIsOnByDefault(t *testing.T) {
	if !DefaultSettings().UseAI {
		t.Error("AI matching defaults to off; the tier that does the most work should not need finding")
	}
}
