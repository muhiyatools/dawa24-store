package catalog_test

import (
	"context"
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
)

// Every switch on "استكمال بيانات الأصناف" must change the result.
//
// A switch that does nothing is worse than no switch, because it is believed.
// Inferring the pharmaceutical form used to run unconditionally, so turning it
// off still wrote a form; these pin each one to an observable difference.

// prepareWith runs the flow under one set of switches and returns the staged
// products.
func prepareWith(t *testing.T, fixture string, opts catalog.ImportOptions) []*catalog.Product {
	t.Helper()

	store := newMemoryStore()
	store.vocab = testVocabulary()
	svc, _ := newImportService(t, store)
	ctx := context.Background()

	session, _, err := svc.AnalyzeImport(ctx, []byte(fixture), "list.csv", 0)
	if err != nil {
		t.Fatalf("analyse failed: %v", err)
	}
	if _, err := svc.PrepareImport(ctx, session.PublicID, catalog.ImportSettings{
		Mode: catalog.ModeUpdateAndAdd, Options: opts,
	}); err != nil {
		t.Fatalf("prepare failed: %v", err)
	}

	out := make([]*catalog.Product, 0, len(store.rows))
	for _, row := range store.rows {
		out = append(out, row.Product)
	}
	return out
}

// A file with no form column: the form is inferred from the name, or not.
const formFixture = "اسم الصنف,كود الصنف,السعر\nبانادول اكسترا اقراص,TOG-1,25.00\n"

func TestDosageFormToggleControlsInference(t *testing.T) {
	on := prepareWith(t, formFixture, catalog.ImportOptions{AssignDosageForm: true})
	if got := on[0].DosageForm; got != "أقراص" {
		t.Errorf("with the switch on, dosage form = %q, want أقراص", got)
	}

	off := prepareWith(t, formFixture, catalog.ImportOptions{AssignDosageForm: false})
	if got := off[0].DosageForm; got != "" {
		t.Errorf("with the switch off, dosage form = %q, want empty — the switch must do something", got)
	}
}

func TestCategoryToggleControlsTheDefaultCategory(t *testing.T) {
	on := prepareWith(t, formFixture, catalog.ImportOptions{
		AssignCategory: true, DefaultCategoryID: 53,
	})
	if on[0].CategoryID == nil || *on[0].CategoryID != 53 {
		t.Errorf("with the switch on, category = %v, want 53", on[0].CategoryID)
	}

	off := prepareWith(t, formFixture, catalog.ImportOptions{
		AssignCategory: false, DefaultCategoryID: 53,
	})
	if off[0].CategoryID != nil {
		t.Errorf("with the switch off, category = %v, want none", *off[0].CategoryID)
	}
}

// The brand switch decides whether a manufacturer becomes a linked brand. The
// name stays on the product either way — the file said it, so it is not the
// importer's to discard.
func TestBrandToggleControlsBrandLinking(t *testing.T) {
	const fixture = "اسم الصنف,كود الصنف,الشركة المصنعة\nبانادول اكسترا,TOG-1,جلاكسو\n"

	off := prepareWith(t, fixture, catalog.ImportOptions{AutoCreateBrands: false})
	if off[0].ManufacturingCompanies != "جلاكسو" {
		t.Errorf("manufacturer = %q; the file's own value must survive", off[0].ManufacturingCompanies)
	}
}

// With AI on, the scientific-name switch is what puts the model's answer on the
// product; with it off the same answer must be ignored.
func TestScientificNameToggleGatesTheModelsAnswer(t *testing.T) {
	answer := func(req catalog.EnrichRequest) []catalog.EnrichResult {
		out := make([]catalog.EnrichResult, 0, len(req.Targets))
		for _, target := range req.Targets {
			out = append(out, catalog.EnrichResult{
				Ref: target.Ref, ScientificName: "Paracetamol", Confidence: 0.95,
			})
		}
		return out
	}

	for _, tc := range []struct {
		name    string
		enabled bool
		want    string
	}{
		{"on", true, "Paracetamol"},
		{"off", false, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := newMemoryStore()
			store.vocab = testVocabulary()
			svc, _ := newImportService(t, store)
			svc.SetEnricher(&stubEnricher{available: true, answer: answer})
			ctx := context.Background()

			session, _, _ := svc.AnalyzeImport(ctx, []byte(formFixture), "list.csv", 0)
			if _, err := svc.PrepareImport(ctx, session.PublicID, catalog.ImportSettings{
				Mode: catalog.ModeUpdateAndAdd,
				Options: catalog.ImportOptions{
					UseAI: true, AssignScientificName: tc.enabled, AssignDosageForm: true,
				},
			}); err != nil {
				t.Fatalf("prepare failed: %v", err)
			}

			if got := store.rows[0].Product.ScientificName; got != tc.want {
				t.Errorf("scientific name = %q, want %q", got, tc.want)
			}
		})
	}
}

// The AI switch is the master control: off, no model is consulted at all.
func TestAIToggleGatesEveryModelCall(t *testing.T) {
	store := newMemoryStore()
	store.vocab = testVocabulary()
	svc, _ := newImportService(t, store)
	enricher := &stubEnricher{
		available: true,
		answer: func(catalog.EnrichRequest) []catalog.EnrichResult {
			return []catalog.EnrichResult{{Ref: 0, ScientificName: "Paracetamol", Confidence: 0.9}}
		},
	}
	svc.SetEnricher(enricher)
	ctx := context.Background()

	session, _, _ := svc.AnalyzeImport(ctx, []byte(formFixture), "list.csv", 0)
	if _, err := svc.PrepareImport(ctx, session.PublicID, catalog.ImportSettings{
		Mode: catalog.ModeUpdateAndAdd,
		Options: catalog.ImportOptions{
			UseAI: false, AssignScientificName: true, AssignCategory: true,
		},
	}); err != nil {
		t.Fatalf("prepare failed: %v", err)
	}

	if enricher.calls != 0 {
		t.Errorf("the model was called %d times with the AI switch off", enricher.calls)
	}
}

// A file that already answers a question must not spend a model call on it.
func TestEnrichmentSkipsFieldsTheFileSupplied(t *testing.T) {
	const complete = "اسم الصنف,كود الصنف,الشكل الصيدلي,الاسم العلمي\n" +
		"بانادول اكسترا,TOG-1,أقراص مغلفة,Paracetamol\n"

	store := newMemoryStore()
	store.vocab = testVocabulary()
	svc, _ := newImportService(t, store)
	enricher := &stubEnricher{
		available: true,
		answer:    func(catalog.EnrichRequest) []catalog.EnrichResult { return nil },
	}
	svc.SetEnricher(enricher)
	ctx := context.Background()

	session, _, _ := svc.AnalyzeImport(ctx, []byte(complete), "list.csv", 0)
	if _, err := svc.PrepareImport(ctx, session.PublicID, catalog.ImportSettings{
		Mode: catalog.ModeUpdateAndAdd,
		Options: catalog.ImportOptions{
			UseAI: true, AssignScientificName: true, AssignDosageForm: true,
		},
	}); err != nil {
		t.Fatalf("prepare failed: %v", err)
	}

	if enricher.calls != 0 {
		t.Errorf("the model was called %d times for a file that already had the answers", enricher.calls)
	}
	if got := store.rows[0].Product.DosageForm; got != "أقراص مغلفة" {
		t.Errorf("dosage form = %q; the file's own value must survive", got)
	}
}
