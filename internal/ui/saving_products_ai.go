package ui

import (
	"context"
	"log/slog"
	"sort"
	"strings"

	"github.com/muhiya/dawa24-store/internal/shared/matchflow"
	"github.com/muhiya/dawa24-store/internal/shared/productmatch"
)

// The saving list's AI stage.
//
// It is the last of the four importers to get one, and it gets exactly the
// stage the other three run: the same prompt version, the same ceilings, the
// same shared decision cache, the same shared catalogue window, and the same
// identity re-check before an answer is written. Nothing here is a second
// implementation of the idea — the parts that could drift live in
// internal/shared/matchflow, and this file is the plumbing between them and a
// list of staged rows.
//
// It runs on the residue and only the residue. Everything the deterministic
// tiers settled is already decided before this is called, and a line the
// catalogue cannot plausibly answer is never sent: the retrieval gate that saved
// two thirds of a smart order's requests applies here unchanged.

// enhanceSaving is the single entry point every staging path calls.
//
// The toggle is applied here rather than at each call site. There are four
// staging paths in this package, and the first version of this feature was
// wired into two of them — so it was present in the flow nobody uses and absent
// from the one the upload screen actually drives. One function that every path
// must go through is how that stops being possible.
func (h *UIHandler) enhanceSaving(
	ctx context.Context, useAI bool, engine *SavingProductMatchEngine, items []*StagedSavingItem,
) int {
	if !useAI {
		return 0
	}
	return enhanceSavingItems(ctx, h.matchEnhancer, engine, items, h.log)
}

// savingAIUnavailableReason explains a disabled switch.
//
// A toggle that ticks and then does nothing is worse than one that says why it
// cannot: the old strategy dropdown promised "ذكاء اصطناعي" from an engine that
// had none, and nobody could tell the promise was empty.
func savingAIUnavailableReason(e matchflow.Enhancer) string {
	if e != nil {
		return ""
	}
	return "خدمة الذكاء الاصطناعي غير مهيّأة على هذا الخادم؛ تعمل المطابقة الحتمية وحدها."
}

// savingAICeilings is what one saving-list run may spend.
//
// The order profile rather than the vendor one: these files are short, a person
// is waiting on the screen, and a wrong link is visible in their own table.
var savingAICeilings = matchflow.For(matchflow.ProfileOrder)

// enhanceSavingItems asks the model about the rows the deterministic tiers left
// unlinked, and links the ones it can verify.
//
// It never returns an error and never fails the import. Every failure path —
// no Gateway, no answer, a malformed response, an answer that does not survive
// the re-check — leaves the row exactly as the deterministic engine left it,
// which is a usable result the user can correct by hand.
func enhanceSavingItems(
	ctx context.Context,
	enhancer matchflow.Enhancer,
	engine *SavingProductMatchEngine,
	items []*StagedSavingItem,
	log *slog.Logger,
) (improved int) {
	if enhancer == nil || engine == nil || engine.index == nil || len(items) == 0 {
		return 0
	}

	questions, window := planSavingQuestions(engine.index, items)
	if len(questions) == 0 {
		return 0
	}

	batches := chunkSaving(questions, savingAICeilings.MaxItemsPerRequest,
		savingAICeilings.MaxRequestsPerRun)

	for _, batch := range batches {
		req := matchflow.Batch{Catalog: window, Items: make([]matchflow.Item, 0, len(batch))}
		for ref, q := range batch {
			it := q.item
			it.Ref = ref
			req.Items = append(req.Items, it)
		}

		decisions, err := enhancer.Enhance(ctx, req)
		if err != nil {
			// The deterministic outcome stands. A saving list that imports
			// without its AI pass is a saving list; one that fails to import is
			// nothing.
			if log != nil {
				log.WarnContext(ctx, "saving-list AI pass failed; deterministic outcome stands",
					"items", len(batch), "error", err)
			}
			continue
		}

		for _, d := range decisions {
			if d.Ref < 0 || d.Ref >= len(batch) {
				continue
			}
			q := batch[d.Ref]
			if d.ProductID == nil || d.Confidence < savingAICeilings.MinApplyConfidence {
				continue
			}
			id := *d.ProductID
			// The answer must name a product that was actually offered, and it
			// must survive the catalogue's own record of what that product is.
			// An instruction in a prompt is a tendency; this is the guarantee.
			if !engine.known[id] {
				continue
			}
			if !engine.index.IdentityConflict(q.row, id).None() {
				continue
			}
			q.target.ProductID = &id
			q.target.MatchType = "ai"
			q.target.Confidence = d.Confidence
			q.target.MasterProductName, q.target.MasterProductSKU = engine.Describe(id)
			improved++
		}
	}
	return improved
}

// savingQuestion is one unlinked row with the retrieval that justifies asking.
type savingQuestion struct {
	target *StagedSavingItem
	row    *productmatch.Row
	item   matchflow.Item
}

// planSavingQuestions retrieves candidates for every unlinked row, drops the
// ones nothing plausible was found for, and builds the shared catalogue window.
//
// The window is the union of every retrieved product, de-duplicated, and every
// item may be answered with any id in it. That costs nothing extra and repairs
// the commonest retrieval failure there is: the right product was retrieved for
// the row above.
func planSavingQuestions(
	idx *productmatch.Index, items []*StagedSavingItem,
) ([]savingQuestion, []matchflow.CatalogEntry) {
	recall := productmatch.DefaultRecallOptions()
	recall.Limit = savingAICeilings.RecallLimit

	questions := make([]savingQuestion, 0, len(items))
	seen := make(map[int64]bool)
	var window []matchflow.CatalogEntry

	for _, it := range items {
		if it == nil || it.ProductID != nil || strings.TrimSpace(it.NameProduct) == "" {
			continue
		}
		row := &productmatch.Row{Name: it.NameProduct, SKU: it.SKU}
		candidates := idx.Recall(row, recall)

		options := make([]int64, 0, len(candidates))
		plausible := false
		for _, c := range candidates {
			if c.Score >= savingAICeilings.MinPlausible {
				plausible = true
			}
			options = append(options, c.ProductID)
			if seen[c.ProductID] {
				continue
			}
			seen[c.ProductID] = true
			if p, ok := idx.Lookup(c.ProductID); ok {
				window = append(window, matchflow.CatalogEntry{
					ProductID: p.ID, NameAR: p.NameAR, NameEN: p.NameEN,
					Scientific: p.Scientific, DosageForm: p.DosageForm,
					Concentration: p.Concentration, Manufacturer: p.Manufacturer,
				})
			}
		}
		if !plausible {
			// Nothing in the catalogue is close. "غير مرتبط" is already the
			// honest answer and no model can improve on it.
			continue
		}

		questions = append(questions, savingQuestion{
			target: it,
			row:    row,
			item: matchflow.Item{
				Text:    it.NameProduct,
				SKU:     it.SKU,
				Options: options,
			},
		})
	}

	sort.SliceStable(window, func(i, j int) bool {
		return window[i].ProductID < window[j].ProductID
	})
	return questions, window
}

// chunkSaving splits the questions into requests, under the run's ceiling.
//
// Questions past the ceiling keep their deterministic outcome. That is a
// deliberate stop rather than a silent truncation: a list long enough to exceed
// it is one whose column mapping is probably wrong, and the rows are all still
// on the review screen.
func chunkSaving(qs []savingQuestion, perRequest, maxRequests int) [][]savingQuestion {
	if perRequest <= 0 {
		perRequest = 100
	}
	out := make([][]savingQuestion, 0, maxRequests)
	for i := 0; i < len(qs) && len(out) < maxRequests; i += perRequest {
		end := i + perRequest
		if end > len(qs) {
			end = len(qs)
		}
		out = append(out, qs[i:end])
	}
	return out
}
