package corpus

import (
	"os"
	"sort"
	"testing"

	"github.com/muhiya/dawa24-store/internal/shared/productmatch"
)

// TestFalseConflicts counts, per conflict kind, how often the engine finds that
// conflict between a row and the product the row IS.
//
// A conflict raised against the correct answer is a false conflict by
// definition, and it costs twice: the correct product is ranked behind whatever
// the row does not contradict, and its score is docked. The rate here is what
// says whether a discrimination rule is separating products or inventing
// differences.
func TestFalseConflicts(t *testing.T) {
	if os.Getenv("DIAGNOSE") == "" || !Available() {
		t.Skip("set DIAGNOSE=1")
	}
	products, err := LoadSnapshot()
	if err != nil {
		t.Fatal(err)
	}
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
		counts := map[string]int{}
		samples := map[string][]string{}
		for _, l := range s.labels {
			for _, kind := range productmatch.DebugConflicts(s.idx, l.Row, l.WantID) {
				counts[kind]++
				if len(samples[kind]) < 5 {
					want, _ := s.idx.Lookup(l.WantID)
					samples[kind] = append(samples[kind],
						trunc(l.Row.DisplayName(), 46)+"  ||  "+trunc(productmatch.DebugName(want), 46))
				}
			}
		}
		keys := make([]string, 0, len(counts))
		for k := range counts {
			keys = append(keys, k)
		}
		sort.SliceStable(keys, func(i, j int) bool { return counts[keys[i]] > counts[keys[j]] })
		t.Logf("=== %s (%d labels) — conflicts raised against the CORRECT product ===", s.name, len(s.labels))
		for _, k := range keys {
			t.Logf("%-12s %5d  (%.1f%%)", k, counts[k], float64(counts[k])*100/float64(len(s.labels)))
			for _, e := range samples[k] {
				t.Logf("      %s", e)
			}
		}
	}
}
