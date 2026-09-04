package corpus

import (
	"fmt"
	"os"
	"sort"
	"testing"

	"github.com/muhiya/dawa24-store/internal/shared/productmatch"
)

// TestDiagnoseMistakes classifies every wrong applied match by what separates
// the chosen product from the correct one.
//
// A diagnostic, not a gate: it prints and asserts nothing. It exists so a
// change to the scorer is aimed at a class of failure with a count beside it
// rather than at the last example somebody happened to read.
func TestDiagnoseMistakes(t *testing.T) {
	if os.Getenv("DIAGNOSE") == "" || !Available() {
		t.Skip("set DIAGNOSE=1")
	}
	products, err := LoadSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	opts := productmatch.DefaultMatchOptions()

	crossLabels, err := CrossScriptLabels(products)
	if err != nil {
		t.Fatal(err)
	}
	sets := []struct {
		name   string
		idx    *productmatch.Index
		labels []Labelled
	}{
		{"cross-script", productmatch.NewIndex(BlankEnglish(products)), crossLabels},
		{"siblings", productmatch.NewIndex(append([]productmatch.MasterProduct(nil), products...)), SiblingLabels(products)},
	}

	for _, s := range sets {
		classes := map[string]int{}
		examples := map[string][]string{}
		for _, l := range s.labels {
			res := s.idx.Match(l.Row, opts)
			if !res.Level.Settled() || res.ProductID == l.WantID {
				continue
			}
			got, _ := s.idx.Lookup(res.ProductID)
			want, _ := s.idx.Lookup(l.WantID)
			if got == nil || want == nil {
				continue
			}
			k := classify(l.Row, got, want)
			classes[k]++
			if len(examples[k]) < 4 {
				examples[k] = append(examples[k], fmt.Sprintf("%.2f %-42s got=%-38s want=%s",
					res.Score, trunc(l.Row.DisplayName(), 42),
					trunc(productmatch.DebugName(got), 38), trunc(productmatch.DebugName(want), 38)))
			}
		}
		keys := make([]string, 0, len(classes))
		for k := range classes {
			keys = append(keys, k)
		}
		sort.SliceStable(keys, func(i, j int) bool { return classes[keys[i]] > classes[keys[j]] })
		t.Logf("=== %s ===", s.name)
		for _, k := range keys {
			t.Logf("%-28s %d", k, classes[k])
			for _, e := range examples[k] {
				t.Logf("      %s", e)
			}
		}
	}
}

// classify names what distinguishes the correct product from the chosen one.
//
// The order is the order the fixes matter in: a difference the row itself
// states is a difference the engine had the evidence to see.
func classify(row *productmatch.Row, got, want *productmatch.MasterProduct) string {
	rowText := row.Name + " " + row.NameEN + " " + row.Concentration
	switch {
	case productmatch.DebugStrengthDiffers(rowText, got, want):
		return "strength/combination"
	case productmatch.DebugNumbersDiffer(rowText, got, want):
		return "size or pack figure"
	case productmatch.DebugModifierDiffers(rowText, got, want):
		return "line-extension word"
	case productmatch.DebugFormDiffers(row, got, want):
		return "dosage form"
	case productmatch.DebugExtraWord(rowText, got, want) != "":
		return "unmatched word: " + productmatch.DebugExtraWord(rowText, got, want)
	}
	return "other"
}
