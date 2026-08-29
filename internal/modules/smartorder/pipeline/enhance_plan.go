package pipeline

// Packing a run into requests.
//
// The whole cost argument for this stage lives in this file. Each review line
// retrieves about a dozen catalogue products, and across a few hundred lines
// those sets overlap heavily — the same twenty antihypertensives come back for
// every antihypertensive line. Sending each item with its own copy of its
// candidates repeats that overlap once per item; sending ONE window that every
// item references by id does not.
//
// Measured on a live 1,473-row file whose deterministic pass left 1,123 lines
// unresolved: 13,453 candidate references collapse to 10,294 catalogue rows, and
// the file fits in eight requests instead of the fifteen the same content would
// need one-shortlist-per-item. Files of ordinary size fit in one.
//
// Everything here is arithmetic on sizes rather than rendering, because
// rendering each candidate set to measure it would double the cost of planning.
// The estimate errs high, which is the safe direction: a request slightly under
// budget costs nothing, a request over it fails outright.

import (
	"sort"
)

// plannedBatch is one request plus what is needed to validate its answers.
type plannedBatch struct {
	request EnhanceBatch
	// refs maps a request-local ref to every line that asked that question.
	// One-to-many because a file that lists the same product twice must not be
	// two questions: identical text with an identical shortlist has an
	// identical answer, and paying for it twice is pure waste.
	refs map[int][]Review
	// window is every product id the model was offered, which is the set an
	// answer must come from. It is the whole window rather than the item's own
	// options on purpose — see the package comment.
	window map[int64]struct{}
	// lines is how many of the buyer's rows this batch settles. It differs from
	// the item count whenever duplicates were collapsed, and it is what the
	// progress bar counts — a bar measured in questions rather than rows would
	// stop short of the total on any file that repeats a product.
	lines int
}

// plan groups reviews into as few requests as the payload budget allows.
//
// Items are ordered by their normalised name first, because alphabetically
// adjacent pharmacy lines retrieve overlapping products, and overlap is exactly
// what the shared window converts into savings: neighbours cost almost nothing
// to add once their catalogue rows are already in the block.
func (e *Enhancement) plan(reviews []Review) []plannedBatch {
	sorted := make([]Review, len(reviews))
	copy(sorted, reviews)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].Line.NormName != sorted[j].Line.NormName {
			return sorted[i].Line.NormName < sorted[j].Line.NormName
		}
		return sorted[i].Line.ID < sorted[j].Line.ID
	})

	// Identical text with an identical shortlist is one question, however many
	// lines asked it. The decision cache would catch this on the *next* import;
	// collapsing here catches it on this one.
	groups := make([][]Review, 0, len(sorted))
	at := make(map[string]int, len(sorted))
	for _, r := range sorted {
		k := decisionKey(r)
		if i, ok := at[k]; ok {
			groups[i] = append(groups[i], r)
			continue
		}
		at[k] = len(groups)
		groups = append(groups, []Review{r})
	}

	var (
		batches []plannedBatch
		cur     = newPlannedBatch()
		size    int
		ref     int
	)
	flush := func() {
		if len(cur.request.Items) == 0 {
			return
		}
		cur.request.Catalog = orderWindow(cur.request.Catalog)
		batches = append(batches, cur)
		cur = newPlannedBatch()
		size = 0
		ref = 0
	}

	for _, group := range groups {
		r := group[0]
		if len(batches)+1 > ceilings.MaxRequestsPerRun {
			// Everything past the ceiling keeps its deterministic outcome and
			// is reported honestly rather than silently dropped.
			e.Stats.CeilingHit = true
			break
		}
		cost := reviewCost(r, cur.window)
		if len(cur.request.Items) > 0 &&
			(size+cost > ceilings.MaxInputBytes || len(cur.request.Items) >= ceilings.MaxItemsPerRequest) {
			flush()
			if len(batches) >= ceilings.MaxRequestsPerRun {
				e.Stats.CeilingHit = true
				break
			}
			cost = reviewCost(r, cur.window)
		}

		ref++
		item := ReviewLine{
			Ref:          ref,
			Text:         r.Line.RawName,
			Brand:        r.Row.Name,
			Strength:     r.Row.Concentration,
			DosageForm:   r.Row.DosageForm,
			PackSize:     r.Row.PackSize,
			Manufacturer: r.Row.Manufacturer,
			Scientific:   r.Row.Scientific,
			SKU:          r.Line.RawSKU,
			Barcode:      r.Line.RawBarcode,
			CurrentGuess: r.Line.MatchedProductID,
			CurrentScore: r.Line.MatchConfidence,
		}
		for _, c := range r.Candidates {
			item.Options = append(item.Options, c.ProductID)
			if _, seen := cur.window[c.ProductID]; seen {
				continue
			}
			cur.window[c.ProductID] = struct{}{}
			cur.request.Catalog = append(cur.request.Catalog, e.describe(c.ProductID))
		}
		cur.request.Items = append(cur.request.Items, item)
		cur.refs[ref] = group
		cur.lines += len(group)
		size += cost
	}
	flush()
	return batches
}

func newPlannedBatch() plannedBatch {
	return plannedBatch{
		refs:   make(map[int][]Review),
		window: make(map[int64]struct{}),
	}
}

// reviewCost estimates the characters one review adds to a request: its own item
// row, plus a catalogue row for each candidate the window does not already hold.
//
// An estimate rather than a rendering, because rendering every candidate set to
// measure it would double the work of planning. It errs high, which is the safe
// direction: a request slightly under budget costs nothing, one over it fails.
func reviewCost(r Review, window map[int64]struct{}) int {
	cost := len(r.Line.RawName) + 96 + len(r.Candidates)*8
	for _, c := range r.Candidates {
		if _, seen := window[c.ProductID]; seen {
			continue
		}
		cost += len(c.Name) + len(c.Scientific) + len(c.Manufacturer) +
			len(c.DosageForm) + len(c.Concentration) + 56
	}
	return cost
}

// orderWindow sorts the catalogue block by id.
//
// Deterministic order is not cosmetic: the rendered input is the cache's
// question, and a block whose order came from map iteration would render
// differently on every run and make every cached answer a miss.
func orderWindow(w []WindowProduct) []WindowProduct {
	sort.SliceStable(w, func(i, j int) bool { return w[i].ProductID < w[j].ProductID })
	return w
}

// describe projects a catalogue product for the window.
//
// It reads the in-memory index rather than the shortlist entry, because the
// index carries the English name and the shortlist does not — and the English
// name is what settles the transliteration cases this stage exists for.
func (e *Enhancement) describe(id int64) WindowProduct {
	p, ok := e.index.Lookup(id)
	if !ok {
		return WindowProduct{ProductID: id}
	}
	return WindowProduct{
		ProductID:     id,
		NameAR:        p.NameAR,
		NameEN:        p.NameEN,
		Scientific:    p.Scientific,
		DosageForm:    p.DosageForm,
		Concentration: p.Concentration,
		Manufacturer:  p.Manufacturer,
	}
}
