package corpus

import (
	"os"
	"testing"

	"github.com/muhiya/dawa24-store/internal/shared/productmatch"
)

func TestProbeSelfConflict(t *testing.T) {
	if os.Getenv("PROBE") == "" {
		t.Skip("")
	}
	products, err := LoadSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	idx := productmatch.NewIndex(append([]productmatch.MasterProduct(nil), products...))
	targets := []string{"اكتي-فيتا جولد 50 مل", "ادفاجراف 0.5 مجم 100 كبسولات", "اكني زنك محلول موضعي 20 مل"}
	for i := range products {
		p := &products[i]
		for _, tg := range targets {
			if p.NameAR != tg {
				continue
			}
			row := &productmatch.Row{Name: p.NameAR}
			t.Logf("AR=%q EN=%q", p.NameAR, p.NameEN)
			t.Logf("  conflicts=%v", productmatch.DebugConflicts(idx, row, p.ID))
			t.Logf("  ROW  %s", productmatch.DebugRowFacts(idx, row))
			pp, _ := idx.Lookup(p.ID)
			t.Logf("  PROD %s", productmatch.DebugProductFacts(pp))
		}
	}
}
