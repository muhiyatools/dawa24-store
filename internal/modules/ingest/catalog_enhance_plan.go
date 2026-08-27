package ingest

// Packing a run into requests.
//
// The whole cost argument for this stage lives in this file, and supplier files
// are where it pays most. A distributor's price list is nine thousand rows of a
// few hundred molecules: the rows that reach here retrieve heavily overlapping
// catalogue products, and the same product is often listed two or three times
// under slightly different spellings. Sending each row with its own copy of its
// candidates repeats that overlap once per row; sending ONE window that every
// row references by id does not.
//
// Repeated rows were already collapsed onto one openRow before retrieval ran,
// so what is left to save here is the overlap BETWEEN questions: candidates are
// de-duplicated into a shared catalogue window, and the twentieth
// antihypertensive costs its item line and nothing else.
//
// Everything here is arithmetic on sizes rather than rendering, because
// rendering each candidate set to measure it would double the cost of planning.
// The estimate errs high, which is the safe direction: a request slightly under
// budget costs nothing, a request over it fails outright.

import "sort"

// plannedBatch is one request plus what is needed to validate its answers.
type plannedBatch struct {
	request EnhanceBatch
	// refs maps a request-local ref back to the question that was asked.
	refs map[int]*openRow
	// window is every product id the model was offered, which is the set an
	// answer must come from. It is the whole window rather than the item's own
	// options on purpose — the correct product is often in the block because it
	// was retrieved for a neighbouring row.
	window map[int64]struct{}
	// rows is how many of the vendor's spreadsheet rows this batch settles. It
	// exceeds the item count whenever duplicates were collapsed, and it is what
	// the progress bar counts — a bar measured in questions rather than rows
	// would stop short of the total on any file that repeats a product.
	rows int
}

// plan groups open rows into as few requests as the payload budget allows.
//
// Items are ordered by their normalised name first, because alphabetically
// adjacent product lines retrieve overlapping catalogue rows, and overlap is
// exactly what the shared window converts into savings: neighbours cost almost
// nothing to add once their catalogue rows are already in the block.
func (e *Enhancement) plan(rows []*openRow) []plannedBatch {
	sorted := make([]*openRow, len(rows))
	copy(sorted, rows)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].normName != sorted[j].normName {
			return sorted[i].normName < sorted[j].normName
		}
		return sorted[i].firstRow() < sorted[j].firstRow()
	})

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

	for _, r := range sorted {
		if len(batches)+1 > MaxRequestsPerRun {
			// Everything past the ceiling keeps its deterministic outcome and
			// is reported honestly rather than silently dropped.
			e.Stats.CeilingHit = true
			break
		}
		cost := rowCost(r, cur.window)
		if len(cur.request.Items) > 0 &&
			(size+cost > MaxInputBytes || len(cur.request.Items) >= MaxItemsPerRequest) {
			flush()
			if len(batches) >= MaxRequestsPerRun {
				e.Stats.CeilingHit = true
				break
			}
			cost = rowCost(r, cur.window)
		}

		ref++
		item := ReviewLine{
			Ref:          ref,
			Text:         r.row.DisplayName(),
			Brand:        r.row.Name,
			Strength:     r.row.Concentration,
			DosageForm:   r.row.DosageForm,
			PackSize:     r.row.PackSize,
			Manufacturer: r.row.Manufacturer,
			Scientific:   r.row.Scientific,
			SKU:          r.row.SKU,
			Barcode:      r.row.Barcode,
			CurrentGuess: r.guess,
			CurrentScore: r.score,
		}
		for _, c := range r.candidates {
			item.Options = append(item.Options, c.ProductID)
			if _, seen := cur.window[c.ProductID]; seen {
				continue
			}
			cur.window[c.ProductID] = struct{}{}
			cur.request.Catalog = append(cur.request.Catalog, e.describe(c.ProductID))
		}
		cur.request.Items = append(cur.request.Items, item)
		cur.refs[ref] = r
		cur.rows += len(r.sourceRows)
		size += cost
	}
	flush()
	return batches
}

func newPlannedBatch() plannedBatch {
	return plannedBatch{
		refs:   make(map[int]*openRow),
		window: make(map[int64]struct{}),
	}
}

// rowCost estimates the characters one row adds to a request: its own item
// line, plus a catalogue line for each candidate the window does not already
// hold.
//
// An estimate rather than a rendering, because rendering every candidate set to
// measure it would double the work of planning. It errs high, which is the safe
// direction.
func rowCost(r *openRow, window map[int64]struct{}) int {
	cost := len(r.row.DisplayName()) + 96 + len(r.candidates)*8
	for _, c := range r.candidates {
		if _, seen := window[c.ProductID]; seen {
			continue
		}
		cost += len(c.Name) + len(c.Scientific) + len(c.Manufacturer) +
			len(c.DosageForm) + len(c.Concentration) + 56
	}
	return cost
}

// firstRow is the lowest spreadsheet row that asked this question, used only to
// keep the ordering of the plan stable.
func (r *openRow) firstRow() int {
	if len(r.sourceRows) == 0 {
		return 0
	}
	return r.sourceRows[0]
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
