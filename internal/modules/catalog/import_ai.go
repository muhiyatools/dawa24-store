package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// AI enrichment for the catalogue import.
//
// The file this was built against is three columns wide — item code,
// description, vendor — and 8,790 rows long. It carries no category, no
// pharmaceutical form, no scientific name and no price. Deterministic rules
// recover the form from the product name for about two thirds of it and can say
// nothing at all about the rest, which is the gap this closes.
//
// Three rules shape the design:
//
//   - AI never blocks a commit. Every field it fills has a deterministic answer
//     underneath, and a Gateway that is disabled, throttled, or out of budget
//     degrades to that answer rather than failing the import.
//   - AI only sees rows the deterministic pass could not settle. Sending all
//     8,790 rows would be 180 calls to decide 3,000 questions.
//   - AI proposes; the admin disposes. Nothing it decides is written until the
//     review screen is confirmed, and every change it made is shown as its own
//     line in the preview.

// Enricher fills the fields a file did not carry. It is an interface so the
// catalogue module depends on the capability rather than on the Gateway client,
// and so tests can drive the whole import with a scripted model.
type Enricher interface {
	// Enrich resolves the requested fields for a batch of products. It must
	// return one result per input, in order, and must not error for a row it
	// cannot resolve — an empty result is a valid "I don't know".
	Enrich(ctx context.Context, req EnrichRequest) (EnrichResponse, error)
	// Available reports whether the enricher can currently be called at all.
	Available(ctx context.Context) bool
}

// EnrichTarget is one product handed to the model.
type EnrichTarget struct {
	Ref            int    `json:"ref"`
	Name           string `json:"name"`
	NameEN         string `json:"name_en,omitempty"`
	Manufacturer   string `json:"manufacturer,omitempty"`
	DosageForm     string `json:"dosage_form,omitempty"`
	Concentration  string `json:"concentration,omitempty"`
	ScientificName string `json:"scientific_name,omitempty"`
}

// TaxonomyOption is one value the model is allowed to choose from.
type TaxonomyOption struct {
	ID   int64  `json:"id,omitempty"`
	Name string `json:"name"`
}

// EnrichRequest is one batch, with the vocabulary the model must choose within.
type EnrichRequest struct {
	Targets []EnrichTarget `json:"products"`
	// Categories and Brands are what the platform already has. The model picks
	// from these by id rather than inventing names, which is what keeps an
	// import from fragmenting the taxonomy into near-duplicates.
	Categories []TaxonomyOption `json:"categories,omitempty"`
	Brands     []TaxonomyOption `json:"brands,omitempty"`
	// DosageForms are the forms already in use across the catalogue.
	DosageForms []string `json:"dosage_forms,omitempty"`

	WantCategory       bool `json:"-"`
	WantDosageForm     bool `json:"-"`
	WantScientificName bool `json:"-"`
	WantManufacturer   bool `json:"-"`

	OrganizationID int64 `json:"-"`
	UserID         int64 `json:"-"`
}

// EnrichResult is the model's answer for one product.
type EnrichResult struct {
	Ref        int   `json:"ref"`
	CategoryID int64 `json:"category_id,omitempty"`
	// BrandID names an existing brand. BrandName is set instead when the model
	// identified a manufacturer the catalogue does not have yet, which the
	// import registers only if the admin allowed auto-created brands.
	BrandID        int64  `json:"brand_id,omitempty"`
	BrandName      string `json:"brand_name,omitempty"`
	DosageForm     string `json:"dosage_form,omitempty"`
	ScientificName string `json:"scientific_name,omitempty"`
	Reason         string `json:"reason,omitempty"`
	// Confidence is the model's own certainty, 0..1. Anything below
	// minAIConfidence is discarded rather than written.
	Confidence float64 `json:"confidence,omitempty"`
}

// EnrichResponse is one batch's answers.
type EnrichResponse struct {
	Results []EnrichResult `json:"results"`
	// Fallback is true when the deterministic path answered instead of a model.
	Fallback bool `json:"-"`
}

// minAIConfidence is the floor below which a model's answer is dropped.
//
// A wrong category on a pharmaceutical product is worse than a missing one: an
// empty column reads as "not classified yet", while a confident wrong value
// reads as fact and propagates into search and reporting.
const minAIConfidence = 0.55

// aiBatchSize is how many products go into one Gateway call.
//
// Large enough that a few thousand uncertain rows is tens of calls rather than
// thousands; small enough that one batch's prompt, taxonomy and answer stay well
// inside a normal context window and one failure loses little work.
const aiBatchSize = 40

// maxTaxonomyOptions bounds how much vocabulary is sent per call. A platform
// with hundreds of brands cannot put all of them in every prompt, so the list is
// narrowed to what the batch could plausibly match before it is sent.
const maxTaxonomyOptions = 120

// enrichSystemPrompt is versioned in this repository, alongside the code that
// parses what it asks for.
//
// It is deliberately narrow: choose from the given lists, say nothing when
// unsure, and never invent a category. The model is doing classification against
// a fixed vocabulary, not open-ended generation.
const enrichSystemPromptText = `You are a pharmaceutical catalogue classification assistant for an Egyptian medical marketplace. Product names are mostly Arabic, sometimes transliterated English, and often abbreviated by a distributor's data entry.

For every product you are given, decide only what you are confident about:

- category_id: choose the single best id from the supplied categories list. Use 0 when no category clearly fits. NEVER invent a category id that is not in the list.
- dosage_form: choose from the supplied dosage_forms list when one fits, in Arabic. Use "" when the name gives no indication of the form.
- scientific_name: the international generic (INN) name of the active ingredient, in English, e.g. "Paracetamol" or "Amoxicillin + Clavulanic Acid". Use "" for cosmetics, devices and anything with no pharmaceutical active ingredient.
- brand_id: the id of the manufacturer from the supplied brands list if the product clearly belongs to one. Use 0 if none matches.
- brand_name: only when you recognise the manufacturer but it is absent from the brands list, give its name. Otherwise "".
- confidence: your own certainty from 0.0 to 1.0 for this product overall.
- reason: a short Arabic phrase explaining the classification.

Rules:
- Never guess. An empty value is correct and expected when the product name does not tell you. A wrong classification is much worse than a missing one.
- Cosmetics, personal-care items, body sprays, shampoos and wipes are NOT medicines: leave scientific_name empty and classify them by their care category.
- Return one object per input product, with its "ref" copied exactly.

Respond with ONLY a JSON object of the form:
{"results":[{"ref":1,"category_id":0,"brand_id":0,"brand_name":"","dosage_form":"","scientific_name":"","confidence":0.0,"reason":""}]}`

// EnrichSystemPrompt is the versioned instruction the enrichment capability
// runs under. It is exported so the Gateway adapter can send it without the
// catalogue having to know what a Gateway request looks like.
func EnrichSystemPrompt() string { return enrichSystemPromptText }

// EnrichSchema constrains the model to the shape DecodeEnrichResponse expects.
func EnrichSchema() map[string]any { return enrichSchema() }

// enrichSchema constrains the model to the shape the parser expects. The Gateway
// forwards it as a structured-output request where the provider supports one.
func enrichSchema() map[string]any {
	return map[string]any{
		"name":   "catalog_enrichment",
		"strict": false,
		"schema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"results": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"ref":             map[string]any{"type": "integer"},
							"category_id":     map[string]any{"type": "integer"},
							"brand_id":        map[string]any{"type": "integer"},
							"brand_name":      map[string]any{"type": "string"},
							"dosage_form":     map[string]any{"type": "string"},
							"scientific_name": map[string]any{"type": "string"},
							"confidence":      map[string]any{"type": "number"},
							"reason":          map[string]any{"type": "string"},
						},
						"required": []string{"ref"},
					},
				},
			},
			"required": []string{"results"},
		},
	}
}

// EnrichmentPlan decides which rows need a model and what to ask about them.
type EnrichmentPlan struct {
	// Indices are positions in the product slice that need enrichment.
	Indices []int
	// The fields worth asking about, narrowed to what is both switched on and
	// actually missing across the selected rows.
	WantCategory       bool
	WantDosageForm     bool
	WantScientificName bool
	WantManufacturer   bool
}

// Empty reports whether there is nothing to ask.
func (p EnrichmentPlan) Empty() bool {
	return len(p.Indices) == 0 ||
		!(p.WantCategory || p.WantDosageForm || p.WantScientificName || p.WantManufacturer)
}

// Batches splits the selected rows into Gateway-sized groups.
func (p EnrichmentPlan) Batches() [][]int {
	var out [][]int
	for start := 0; start < len(p.Indices); start += aiBatchSize {
		out = append(out, p.Indices[start:min(start+aiBatchSize, len(p.Indices))])
	}
	return out
}

// PlanEnrichment selects the rows a model could usefully answer for.
//
// A row is selected only when a switched-on field is genuinely empty on it. A
// file that already names the pharmaceutical form in its own column asks the
// model nothing, however loudly the switch is on — which is what keeps a clean
// file free of AI cost entirely.
func PlanEnrichment(prods []*Product, opts ImportOptions) EnrichmentPlan {
	var plan EnrichmentPlan
	if !opts.UseAI {
		return plan
	}

	for i, p := range prods {
		if p == nil {
			continue
		}
		needsCategory := opts.AssignCategory && (p.CategoryID == nil || *p.CategoryID <= 0)
		// An inferred form is a placeholder, not an answer: "مستحضر صيدلاني"
		// means the name gave no clue, which is exactly when a model helps.
		needsDosage := opts.AssignDosageForm && (p.DosageForm == "" || p.DosageForm == defaultDosageForm)
		needsScientific := opts.AssignScientificName && p.ScientificName == ""
		needsBrand := opts.AutoCreateBrands && p.ManufacturingCompanies == ""

		if !needsCategory && !needsDosage && !needsScientific && !needsBrand {
			continue
		}
		plan.Indices = append(plan.Indices, i)
		plan.WantCategory = plan.WantCategory || needsCategory
		plan.WantDosageForm = plan.WantDosageForm || needsDosage
		plan.WantScientificName = plan.WantScientificName || needsScientific
		plan.WantManufacturer = plan.WantManufacturer || needsBrand
	}
	return plan
}

// BuildEnrichRequest assembles one batch's payload.
func BuildEnrichRequest(prods []*Product, batch []int, plan EnrichmentPlan, vocab EnrichVocabulary) EnrichRequest {
	req := EnrichRequest{
		WantCategory:       plan.WantCategory,
		WantDosageForm:     plan.WantDosageForm,
		WantScientificName: plan.WantScientificName,
		WantManufacturer:   plan.WantManufacturer,
	}

	for _, idx := range batch {
		p := prods[idx]
		req.Targets = append(req.Targets, EnrichTarget{
			// The reference is the product's position in the whole slice, so an
			// answer that arrives out of order still lands on the right row.
			Ref:            idx,
			Name:           p.Name.Get(i18n.AR),
			NameEN:         p.Name.Get(i18n.EN),
			Manufacturer:   p.ManufacturingCompanies,
			DosageForm:     p.DosageForm,
			Concentration:  p.Concentration,
			ScientificName: p.ScientificName,
		})
	}

	if plan.WantCategory {
		req.Categories = vocab.Categories
	}
	if plan.WantDosageForm {
		req.DosageForms = vocab.DosageForms
	}
	if plan.WantManufacturer {
		req.Brands = truncateOptions(vocab.Brands, maxTaxonomyOptions)
	}
	return req
}

// EnrichVocabulary is what the platform already knows, offered to the model as
// the closed set it must choose within.
type EnrichVocabulary struct {
	Categories  []TaxonomyOption
	Brands      []TaxonomyOption
	DosageForms []string
}

func truncateOptions(in []TaxonomyOption, limit int) []TaxonomyOption {
	if len(in) <= limit {
		return in
	}
	return in[:limit]
}

// ApplyEnrichment writes a model's accepted answers onto the products and
// reports what it changed, per row, for the preview.
//
// Nothing here overwrites a value the file supplied. The model fills gaps; it
// does not correct the supplier.
func ApplyEnrichment(
	prods []*Product, results []EnrichResult, opts ImportOptions, vocab EnrichVocabulary,
) map[int][]AIChange {
	changes := map[int][]AIChange{}
	categoryNames := optionIndex(vocab.Categories)
	brandNames := optionIndex(vocab.Brands)

	for _, res := range results {
		if res.Ref < 0 || res.Ref >= len(prods) || prods[res.Ref] == nil {
			continue
		}
		if res.Confidence > 0 && res.Confidence < minAIConfidence {
			continue
		}
		p := prods[res.Ref]
		var applied []AIChange

		if opts.AssignCategory && res.CategoryID > 0 && (p.CategoryID == nil || *p.CategoryID <= 0) {
			if name, known := categoryNames[res.CategoryID]; known {
				id := res.CategoryID
				p.CategoryID = &id
				applied = append(applied, AIChange{
					Field: FieldCategory, Label: "فئة المنتج", Value: name, Reason: res.Reason,
				})
			}
		}

		if opts.AssignDosageForm && res.DosageForm != "" &&
			(p.DosageForm == "" || p.DosageForm == defaultDosageForm) {
			p.DosageForm = CleanCellString(res.DosageForm)
			applied = append(applied, AIChange{
				Field: FieldDosageForm, Label: "الشكل الصيدلي", Value: p.DosageForm, Reason: res.Reason,
			})
		}

		if opts.AssignScientificName && res.ScientificName != "" && p.ScientificName == "" {
			p.ScientificName = CleanCellString(res.ScientificName)
			applied = append(applied, AIChange{
				Field: FieldGenericName, Label: "الاسم العلمي", Value: p.ScientificName, Reason: res.Reason,
			})
		}

		if opts.AutoCreateBrands && p.ManufacturingCompanies == "" {
			if name, known := brandNames[res.BrandID]; known && res.BrandID > 0 {
				id := res.BrandID
				p.BrandID = &id
				p.ManufacturingCompanies = name
				applied = append(applied, AIChange{
					Field: FieldManufacturer, Label: "الشركة المصنعة", Value: name, Reason: res.Reason,
				})
			} else if clean := CleanCellString(res.BrandName); clean != "" {
				// A manufacturer the catalogue has not seen. It is written onto
				// the product as text here and only becomes a brand row at
				// commit, and only if the admin left auto-creation on.
				p.ManufacturingCompanies = clean
				applied = append(applied, AIChange{
					Field: FieldManufacturer, Label: "الشركة المصنعة (جديدة)", Value: clean, Reason: res.Reason,
				})
			}
		}

		if len(applied) > 0 {
			changes[res.Ref] = applied
		}
	}
	return changes
}

// FieldCategory names the category field in AI changes. The other field names
// are the import field constants, so a change lines up with the column it fills.
const FieldCategory = "category"

func optionIndex(options []TaxonomyOption) map[int64]string {
	out := make(map[int64]string, len(options))
	for _, o := range options {
		out[o.ID] = o.Name
	}
	return out
}

// EncodeEnrichInput renders a batch as the model's user message.
func EncodeEnrichInput(req EnrichRequest) (string, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("catalog: encode enrichment request: %w", err)
	}
	return string(body), nil
}

// DecodeEnrichResponse reads the model's answer.
//
// Models wrap JSON in markdown fences often enough that stripping them is part
// of parsing rather than an error worth failing a nine-thousand-row import over.
func DecodeEnrichResponse(content string) (EnrichResponse, error) {
	clean := strings.TrimSpace(content)
	clean = strings.TrimPrefix(clean, "```json")
	clean = strings.TrimPrefix(clean, "```")
	clean = strings.TrimSuffix(clean, "```")
	clean = strings.TrimSpace(clean)

	var out EnrichResponse
	if err := json.Unmarshal([]byte(clean), &out); err != nil {
		return EnrichResponse{}, fmt.Errorf("catalog: decode enrichment response: %w", err)
	}
	return out, nil
}
