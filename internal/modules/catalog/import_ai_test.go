package catalog_test

import (
	"context"
	"errors"
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// testVocabulary is the closed set the model must classify within.
func testVocabulary() catalog.EnrichVocabulary {
	return catalog.EnrichVocabulary{
		Categories: []catalog.TaxonomyOption{
			{ID: 53, Name: "أدوية"},
			{ID: 57, Name: "عناية بالبشرة الطبية"},
		},
		Brands: []catalog.TaxonomyOption{
			{ID: 900, Name: "جلاكسو"},
			{ID: 901, Name: "نوفارتس"},
		},
		DosageForms: []string{"أقراص", "شراب", "كريم"},
	}
}

func productNamed(nameAR string) *catalog.Product {
	return &catalog.Product{Name: i18n.New(nameAR, ""), InstitutionalWorkIDs: []int64{}}
}

// Enrichment costs money and latency per row, so a file that already answers a
// question must not ask it. This is what keeps a clean import free of AI spend.
func TestPlanEnrichmentSelectsOnlyRowsWithGaps(t *testing.T) {
	complete := productNamed("بانادول أقراص")
	complete.DosageForm = "أقراص"
	complete.ScientificName = "Paracetamol"

	incomplete := productNamed("منتج غامض")
	incomplete.DosageForm = "مستحضر صيدلاني" // the placeholder, not an answer

	opts := catalog.ImportOptions{
		UseAI: true, AssignDosageForm: true, AssignScientificName: true,
	}
	plan := catalog.PlanEnrichment([]*catalog.Product{complete, incomplete}, opts)

	if len(plan.Indices) != 1 || plan.Indices[0] != 1 {
		t.Fatalf("selected %v, want only the incomplete row at index 1", plan.Indices)
	}
	if !plan.WantDosageForm || !plan.WantScientificName {
		t.Error("the plan does not ask about the fields that are missing")
	}
	if plan.WantCategory {
		t.Error("the plan asks about a category the admin did not switch on")
	}
}

func TestPlanEnrichmentIsEmptyWhenAIIsOff(t *testing.T) {
	opts := catalog.ImportOptions{AssignCategory: true, AssignDosageForm: true}
	plan := catalog.PlanEnrichment([]*catalog.Product{productNamed("منتج")}, opts)

	if !plan.Empty() {
		t.Error("a plan was built with the AI switch off")
	}
}

func TestPlanEnrichmentBatchesLargeFiles(t *testing.T) {
	prods := make([]*catalog.Product, 130)
	for i := range prods {
		prods[i] = productNamed("منتج غامض")
	}

	plan := catalog.PlanEnrichment(prods,
		catalog.ImportOptions{UseAI: true, AssignScientificName: true})

	batches := plan.Batches()
	if len(batches) < 3 {
		t.Fatalf("130 rows produced %d batches; they must be split for the Gateway", len(batches))
	}
	total := 0
	for _, batch := range batches {
		if len(batch) > 40 {
			t.Errorf("batch of %d exceeds the per-call limit", len(batch))
		}
		total += len(batch)
	}
	if total != len(prods) {
		t.Errorf("batches cover %d rows, want %d — no row may be dropped", total, len(prods))
	}
}

func TestApplyEnrichmentFillsGapsWithoutOverwriting(t *testing.T) {
	// The model fills what the file left out; it does not correct the supplier.
	stated := productNamed("كتافلام أقراص")
	stated.DosageForm = "أقراص مغلفة"
	blank := productNamed("منتج بدون بيانات")

	opts := catalog.ImportOptions{
		UseAI: true, AssignCategory: true, AssignDosageForm: true, AssignScientificName: true,
	}
	results := []catalog.EnrichResult{
		{Ref: 0, DosageForm: "شراب", ScientificName: "Diclofenac", CategoryID: 53, Confidence: 0.9},
		{Ref: 1, DosageForm: "كريم", CategoryID: 57, Confidence: 0.8},
	}

	changes := catalog.ApplyEnrichment(
		[]*catalog.Product{stated, blank}, results, opts, testVocabulary())

	if stated.DosageForm != "أقراص مغلفة" {
		t.Errorf("dosage form = %q; a value the file supplied must not be overwritten", stated.DosageForm)
	}
	if stated.ScientificName != "Diclofenac" {
		t.Errorf("scientific name = %q, want the model's answer for the empty field", stated.ScientificName)
	}
	if blank.DosageForm != "كريم" {
		t.Errorf("dosage form = %q, want كريم", blank.DosageForm)
	}
	if blank.CategoryID == nil || *blank.CategoryID != 57 {
		t.Errorf("category = %v, want 57", blank.CategoryID)
	}

	// Every change is recorded so the review screen can show what AI decided.
	if len(changes[1]) < 2 {
		t.Errorf("recorded %d changes for row 1, want the category and the form", len(changes[1]))
	}
}

// A wrong category on a pharmaceutical product is worse than a missing one, so
// a model that says it is unsure is not believed.
func TestApplyEnrichmentDiscardsLowConfidenceAnswers(t *testing.T) {
	product := productNamed("منتج غامض")
	opts := catalog.ImportOptions{UseAI: true, AssignCategory: true, AssignScientificName: true}

	catalog.ApplyEnrichment([]*catalog.Product{product},
		[]catalog.EnrichResult{{Ref: 0, CategoryID: 53, ScientificName: "Guesswork", Confidence: 0.2}},
		opts, testVocabulary())

	if product.CategoryID != nil {
		t.Error("a low-confidence category was written")
	}
	if product.ScientificName != "" {
		t.Errorf("scientific name = %q; a low-confidence answer was written", product.ScientificName)
	}
}

// A category id the platform does not have would be a dangling reference the
// database would refuse, taking the whole import down with it.
func TestApplyEnrichmentRefusesUnknownCategoryIDs(t *testing.T) {
	product := productNamed("منتج")
	opts := catalog.ImportOptions{UseAI: true, AssignCategory: true}

	catalog.ApplyEnrichment([]*catalog.Product{product},
		[]catalog.EnrichResult{{Ref: 0, CategoryID: 999999, Confidence: 0.99}},
		opts, testVocabulary())

	if product.CategoryID != nil {
		t.Errorf("category = %v; an id outside the supplied vocabulary was accepted", *product.CategoryID)
	}
}

func TestApplyEnrichmentLinksKnownManufacturerAndProposesNewOnes(t *testing.T) {
	known := productNamed("منتج من شركة معروفة")
	unknown := productNamed("منتج من شركة جديدة")
	opts := catalog.ImportOptions{UseAI: true, AutoCreateBrands: true}

	catalog.ApplyEnrichment([]*catalog.Product{known, unknown}, []catalog.EnrichResult{
		{Ref: 0, BrandID: 900, Confidence: 0.9},
		{Ref: 1, BrandName: "شركة الدلتا للأدوية", Confidence: 0.9},
	}, opts, testVocabulary())

	if known.BrandID == nil || *known.BrandID != 900 {
		t.Errorf("brand = %v, want the existing brand 900 rather than a new one", known.BrandID)
	}
	if known.ManufacturingCompanies != "جلاكسو" {
		t.Errorf("manufacturer = %q, want جلاكسو", known.ManufacturingCompanies)
	}

	// An unrecognised manufacturer is carried as text and only becomes a brand
	// row at commit, and only if the admin left auto-creation on.
	if unknown.BrandID != nil {
		t.Error("a brand id was invented for a manufacturer the platform does not have")
	}
	if unknown.ManufacturingCompanies != "شركة الدلتا للأدوية" {
		t.Errorf("manufacturer = %q, want the proposed name", unknown.ManufacturingCompanies)
	}
}

func TestApplyEnrichmentIgnoresOutOfRangeReferences(t *testing.T) {
	// A model that echoes a reference wrongly must not corrupt a different row
	// or panic the import.
	product := productNamed("منتج")
	opts := catalog.ImportOptions{UseAI: true, AssignScientificName: true}

	catalog.ApplyEnrichment([]*catalog.Product{product}, []catalog.EnrichResult{
		{Ref: 42, ScientificName: "Wrong", Confidence: 0.99},
		{Ref: -1, ScientificName: "AlsoWrong", Confidence: 0.99},
	}, opts, testVocabulary())

	if product.ScientificName != "" {
		t.Errorf("scientific name = %q; an out-of-range reference was applied", product.ScientificName)
	}
}

func TestDecodeEnrichResponseToleratesMarkdownFences(t *testing.T) {
	// Models wrap JSON in fences often enough that stripping them is part of
	// parsing, not an error worth failing a nine-thousand-row import over.
	body := "```json\n{\"results\":[{\"ref\":3,\"dosage_form\":\"شراب\",\"confidence\":0.8}]}\n```"

	resp, err := catalog.DecodeEnrichResponse(body)
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if len(resp.Results) != 1 || resp.Results[0].Ref != 3 {
		t.Fatalf("decoded %+v, want one result for ref 3", resp.Results)
	}
	if resp.Results[0].DosageForm != "شراب" {
		t.Errorf("dosage form = %q, want شراب", resp.Results[0].DosageForm)
	}
}

func TestDecodeEnrichResponseRejectsGarbage(t *testing.T) {
	if _, err := catalog.DecodeEnrichResponse("I'm sorry, I can't help with that."); err == nil {
		t.Error("a non-JSON answer was accepted")
	}
}

func TestBuildEnrichRequestCarriesOnlyTheAskedVocabulary(t *testing.T) {
	prods := []*catalog.Product{productNamed("منتج أول"), productNamed("منتج ثانٍ")}
	plan := catalog.EnrichmentPlan{Indices: []int{0, 1}, WantDosageForm: true}

	req := catalog.BuildEnrichRequest(prods, []int{0, 1}, plan, testVocabulary())

	if len(req.Targets) != 2 {
		t.Fatalf("request carries %d products, want 2", len(req.Targets))
	}
	if req.Targets[1].Ref != 1 {
		t.Errorf("ref = %d, want the product's index so an out-of-order answer still lands right", req.Targets[1].Ref)
	}
	if len(req.DosageForms) == 0 {
		t.Error("the request omits the dosage forms it is asking about")
	}
	// Sending vocabulary nobody asked about is prompt weight paid for nothing.
	if len(req.Categories) != 0 || len(req.Brands) != 0 {
		t.Error("the request carries vocabulary for fields it is not asking about")
	}
}

// stubEnricher is a scripted model, so the whole import flow can be driven
// without a Gateway.
type stubEnricher struct {
	calls     int
	available bool
	fail      error
	answer    func(catalog.EnrichRequest) []catalog.EnrichResult
}

func (s *stubEnricher) Available(context.Context) bool { return s.available }

func (s *stubEnricher) Enrich(_ context.Context, req catalog.EnrichRequest) (catalog.EnrichResponse, error) {
	s.calls++
	if s.fail != nil {
		return catalog.EnrichResponse{Fallback: true}, s.fail
	}
	return catalog.EnrichResponse{Results: s.answer(req)}, nil
}

var errGatewayDown = errors.New("gateway: unavailable")
