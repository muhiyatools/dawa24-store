package productmatch

// Contradiction is not a smaller score.
//
// The scorer used to answer one question with one number: how alike are these?
// Everything went into it — the name, the dose, the form, the figures — as
// additions and subtractions, and the sum decided. That works while the
// candidates are unrelated. It fails on the case that matters, which is a
// catalogue holding the right answer AND three of its siblings:
//
//	row        اماريل ام 2/500 مجم 30 قرص
//	chosen     اماريل 2مجم 30 قرص            name 1.00, dose "agrees", applied
//	correct    اماريل ام 2/500 مجم 30 قرص    name 0.97
//
// The chosen row is a different medicine — Amaryl M is glimepiride with
// metformin — and it won because it is a shorter name that the row's words
// cover completely, while the thing separating them (a second dose component,
// a letter) was worth a fraction of a point against a name similarity worth
// three quarters of one.
//
// So contradiction is taken out of the sum. A candidate that disagrees with
// something the row STATES is ranked behind every candidate that does not,
// whatever the names look like; the score only orders candidates that
// contradict the row equally. That is the whole idea of this file, and it is
// what makes "the right size is in the catalogue and the wrong one was chosen"
// a case the engine can no longer produce: the wrong size contradicts a stated
// figure and the right size does not, so the right size ranks first even when
// the wrong one is spelled more like the row.
//
// Nothing here is a veto. A conflicting candidate can still be the best there
// is, and it is still offered — as a candidate for review, at a score that says
// what it is. Refusing to rank it first is different from refusing to show it.

// conflict is one thing a row and a candidate disagree about.
type conflict struct {
	kind string
	// mass is how much identity the disagreement carries. It orders candidates
	// against each other and scales the penalty; it is not a probability.
	mass float64
}

// Conflict masses.
//
// A mass is read two ways and both are deliberate. It orders candidates against
// each other — less contradiction first, always, whatever the names look like —
// and it says how much of a candidate's score survives the contradiction: the
// score is multiplied by (1 − mass), so a mass of 1 leaves nothing and a mass
// of 0.45 leaves a bit over half.
//
// That second reading is why the numbers sit where they do. Everything at or
// above massLetter takes a candidate below the applied threshold on its own, so
// a row whose ONLY plausible answer contradicts its strength, its brand
// extension, its form or its identity letter reaches a person rather than a
// price list. Below that line — a pack count, a stray figure, a cream against
// an ointment — the contradiction still decides which candidate wins, but it
// does not by itself refuse the only candidate there is.
//
// Ordered by what the disagreement proves rather than by how visible it is.
const (
	massDose      = 1.00 // 500 mg is not 1 g
	massDoseParts = 0.95 // 16 mg is not 16/12.5 mg — a combination is another product
	massModifier  = 0.95 // بانادول is not بانادول اكسترا
	massForm      = 0.90 // a syrup is not a tablet
	massLetter    = 0.80 // بتنوفيت ان is not بتنوفيت سي
	massCount     = 0.70 // 20 tablets is not 200
	massFigure    = 0.60 // a figure that named nothing, and differs anyway
	massSubForm   = 0.55 // a cream is not an ointment
	massPack      = 0.45 // the pack-size column disagreeing with the name
)

// survival is the share of a candidate's evidence that survives everything it
// contradicts.
//
// Multiplicative rather than subtractive, and that is the change that made the
// reported score mean something. Subtraction let a large name similarity buy
// its way past a contradiction — 0.97 minus a fifth is still applied — and it
// made the number printed on the review screen an arithmetic residue rather
// than a statement about the product. A factor cannot be outrun: whatever the
// names look like, a candidate that contradicts the dose keeps a fifth of
// whatever it had.
//
// Independent factors, because the conflicts are largely independent evidence:
// a product that differs in strength AND in form is further away than one that
// differs in either.
func survival(cs []conflict) float64 {
	f := 1.0
	for _, c := range cs {
		f *= 1 - c.mass
	}
	return f
}

// conflictsOf lists everything a candidate contradicts about a row.
//
// Only attributes the row actually states are compared. Silence is not
// disagreement: a pharmacy writes "بانادول" and means the only بانادول there
// is, and holding that against a catalogue entry which spells out its strength
// would refuse the commonest correct match in the file.
func (idx *Index) conflictsOf(q *query, p *MasterProduct) []conflict {
	var out []conflict
	sides := p.sides()

	switch doseVerdict(q, p, sides) {
	case dosesConflict:
		out = append(out, conflict{"strength", massDose})
	case dosesPartsDiffer:
		out = append(out, conflict{"dose_parts", massDoseParts})
	}

	// Everything below is asked of each spelling separately and answered by the
	// spelling that agrees, if any exists.
	//
	// A spelling that says NOTHING about an attribute does not get a vote on
	// it, and that rule is load-bearing. The first version of this took the
	// whole side with the fewest conflicts, and the English half of a catalogue
	// record routinely states less than the Arabic half — "advochol 10mg 14
	// f.c. tabs" parsed to no pack count at all — so the uninformative side won
	// every comparison and switched the count check off for the entire
	// catalogue. Silence is not agreement.
	if conflictsOnEverySide(sides, func(f nameFacts) bool {
		return modifierSetsConflict(q.mods, f.mods, q.rawName, p.NameAR+" "+p.NameEN)
	}) {
		out = append(out, conflict{"modifier", massModifier})
	}
	if conflictsOnEverySide(sides, func(f nameFacts) bool {
		return !markSetsAgree(q.marks, f.marks)
	}) {
		out = append(out, conflict{"letter", massLetter})
	}

	switch {
	case q.formKey == "":
		// The row named no form. It cannot be contradicted about one.
	case conflictsOnStatingSide(sides,
		func(f nameFacts) bool { return p.formOf(f) != "" },
		func(f nameFacts) bool { return formClass(q.formKey) != formClass(p.formOf(f)) }):
		out = append(out, conflict{"form", massForm})
	case q.subForm != "" && conflictsOnStatingSide(sides,
		func(f nameFacts) bool { return f.subForm != "" },
		func(f nameFacts) bool { return q.subForm != f.subForm }):
		out = append(out, conflict{"sub_form", massSubForm})
	}

	countConflict := conflictsOnStatingSide(sides,
		func(f nameFacts) bool { return sharesCountClass(q.qty.counts, f.qty.counts) },
		func(f nameFacts) bool { return countsDisagree(q.qty.counts, f.qty.counts) })
	switch {
	case countConflict:
		out = append(out, conflict{"count", massCount})
	case conflictsOnStatingSide(sides,
		func(f nameFacts) bool { return len(f.qty.residual) > 0 },
		func(f nameFacts) bool { return residualsDisagree(q.qty.residual, f.qty.residual) }):
		out = append(out, conflict{"figure", massFigure})
	}

	if q.packSize > 0 && p.packSize > 0 && q.packSize != p.packSize && !countConflict {
		// Only when the names did not already say so, or the same difference
		// would be charged twice.
		out = append(out, conflict{"pack", massPack})
	}
	return out
}

// The three answers the dose comparison can give.
const (
	dosesAgree = iota
	dosesPartsDiffer
	dosesConflict
)

// doseVerdict compares the row's doses against each spelling of the candidate
// and reports the best answer any of them gives.
//
// A spelling that measures nothing the row also measured does not vote, which
// is the same rule the other attributes follow: silence is missing information,
// not agreement and not contradiction.
func doseVerdict(q *query, p *MasterProduct, sides []nameFacts) int {
	verdict := dosesAgree
	compared := false
	for _, f := range sides {
		doses := p.dosesOf(f)
		agree, comparable := compareStrengths(q.strengths, doses)
		if !comparable {
			continue
		}
		if agree {
			// This spelling agrees. Whether it agrees in the same NUMBER of
			// components is a separate question, and the one the combination
			// families turn on: اتاكاند is 32 مجم and اتاكاند بلس is 32/25 مجم.
			if !doseComponentsDiffer(q.strengths, doses) {
				return dosesAgree
			}
			verdict = dosesPartsDiffer
			compared = true
			continue
		}
		if !compared {
			verdict = dosesConflict
		}
		compared = true
	}
	if !compared {
		return dosesAgree
	}
	return verdict
}

// conflictsOnEverySide reports a disagreement no spelling of the candidate
// escapes.
//
// Used for the checks that are symmetric — a modifier or a letter present on
// one side and absent on the other is a difference either way round — where an
// empty side is a real answer rather than a missing one.
func conflictsOnEverySide(sides []nameFacts, differs func(nameFacts) bool) bool {
	for _, f := range sides {
		if !differs(f) {
			return false
		}
	}
	return len(sides) > 0
}

// conflictsOnStatingSide reports a disagreement among the spellings that have
// something to say, ignoring the ones that do not.
//
// Used for the checks where absence is silence: a catalogue name that omits the
// pack count has not contradicted a row that states one.
func conflictsOnStatingSide(sides []nameFacts, states, differs func(nameFacts) bool) bool {
	stated := false
	for _, f := range sides {
		if !states(f) {
			continue
		}
		stated = true
		if !differs(f) {
			return false
		}
	}
	return stated
}

// sharesCountClass reports whether the two names counted the same kind of thing,
// which is the precondition for their counts being able to disagree at all.
func sharesCountClass(a, b []countUnit) bool {
	for _, x := range a {
		for _, y := range b {
			if x.class == y.class {
				return true
			}
		}
	}
	return false
}

// formClass groups the dosage forms that a row and a catalogue entry may name
// differently while meaning the same shelf.
//
// Tablets and capsules are one class because the person typing an order uses
// اقراص and كبسول interchangeably for any solid oral form. Creams, ointments,
// gels and washes are one class because Egyptian names call the same bottle
// لوسيون and غسول — and what genuinely separates a cream from a shampoo is
// compared underneath, as a sub-form.
//
// Everything else stays distinct. A syrup is not an injection, and a sachet of
// granules is not a strip of tablets.
func formClass(key string) string {
	switch key {
	case "tablet", "capsule":
		return "solid_oral"
	case "topical", "wash":
		return "external"
	}
	return key
}

// conflictMass sums what a candidate contradicts.
func conflictMass(cs []conflict) float64 {
	var total float64
	for _, c := range cs {
		total += c.mass
	}
	return total
}

// doseComponentsDiffer reports whether two dose sets describe combinations of
// different width on a unit they both use.
//
// اتاكاند is 32 مجم and اتاكاند بلس is 32/25 مجم: the doses AGREE — 32 is in
// both — and the products are different, and this is the only thing that says
// so. It is asked per unit and only for units both sides state, so a row
// silent about a strength is never contradicted by one that spells it out.
func doseComponentsDiffer(a, b []strength) bool {
	for unit, wa := range widestPerUnit(a) {
		if wb, both := widestPerUnit(b)[unit]; both && wa != wb {
			return true
		}
	}
	return false
}

// widestPerUnit is the largest ratio a dose set states in each unit.
func widestPerUnit(set []strength) map[string]int {
	out := make(map[string]int, 2)
	for _, s := range set {
		parts := s.parts
		if parts < 1 {
			parts = 1
		}
		if parts > out[s.unit] {
			out[s.unit] = parts
		}
	}
	return out
}

// markSetsAgree compares the identity letters two names carry.
//
// Set equality, like the modifier check and for the same reason: the letter one
// side carries and the other does not IS the product. بتنوفيت against بتنوفيت
// ان is not a terser way of writing the same thing.
func markSetsAgree(a, b map[string]struct{}) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if _, ok := b[k]; !ok {
			return false
		}
	}
	return true
}

// countsDisagree compares the packaging counts two names state.
//
// Per class, and only where both sides counted the same thing. A row saying "30
// قرص" against a catalogue entry that states no count at all has not been
// contradicted — the catalogue frequently omits it — but against one saying "20
// قرص" it has.
func countsDisagree(a, b []countUnit) bool {
	for _, x := range a {
		for _, y := range b {
			if x.class != y.class {
				continue
			}
			if x.value == y.value {
				return false // this class agrees; that is enough
			}
		}
	}
	// No agreeing pair. Disagreement only where some class is present on both.
	for _, x := range a {
		for _, y := range b {
			if x.class == y.class {
				return true
			}
		}
	}
	return false
}

// residualsDisagree compares the figures that named nothing.
//
// Weaker evidence than a count, and treated as such: an unlabelled figure could
// be a year, a batch, a price the supplier appended. But when BOTH names carry
// one and they are not the same figure, something differs — "دوركو ايف 3" and
// "دوركو ايف 6" are two razors and nothing else in either line says so.
//
// Silence on either side is not disagreement, which is what stops a catalogue
// entry that omits the pack count from contradicting a row that states it.
func residualsDisagree(a, b []float64) bool {
	if len(a) == 0 || len(b) == 0 {
		return false
	}
	for _, x := range a {
		for _, y := range b {
			if x == y {
				return false
			}
		}
	}
	return true
}

// separated reports whether anything the ROW states picks one of two candidates
// over the other.
//
// This is the ambiguity test, and it asks a different question from "are their
// scores close". Two catalogue entries scoring 0.97 and 0.96 are not
// distinguishable by a hundredth of a point that came from a spelling; two
// entries scoring 0.97 and 0.80 are not distinguishable either if the whole
// difference between them is a word the row never mentioned. What settles it is
// evidence: does the row state an attribute that one of them agrees with and
// the other does not?
//
// When the answer is no, the honest result is "these two fit equally well",
// whatever the scores say — and that is a question for a person, not a coin
// toss made at four decimal places.
func (idx *Index) separated(q *query, a, b *MasterProduct) bool {
	if a == nil || b == nil {
		return true
	}
	ca, cb := idx.conflictsOf(q, a), idx.conflictsOf(q, b)
	if len(ca) != len(cb) {
		return true
	}
	if conflictMass(ca) != conflictMass(cb) {
		return true
	}
	// Neither contradicts the row, or both contradict it equally. The remaining
	// evidence is the row's own words: one candidate accounting for more of
	// them, letter for letter, is a real separation — a fold is not.
	ea, eb := idx.exactWordHits(q, a), idx.exactWordHits(q, b)
	return ea != eb
}

// exactWordHits counts the row's distinctive words the candidate carries
// spelled exactly the same way.
//
// The variant fold exists so "ابيكوبريد" can meet "ابيكوبرايد", and it earns
// its keep. It also makes سيفازون and سيفوزون — two different cephalosporins —
// the same key, and when both are in the catalogue and the row names one of
// them exactly, the fold is what lets the other one tie. Counting exact hits
// separately is how the tie is broken toward the spelling the supplier actually
// wrote.
func (idx *Index) exactWordHits(q *query, p *MasterProduct) int {
	hits := 0
	for _, side := range [][]string{p.coreAR, p.coreEN} {
		n := 0
		for _, t := range side {
			slot, ok := q.pos[t]
			if ok && q.distinct[slot] {
				n++
			}
		}
		if n > hits {
			hits = n
		}
	}
	return hits
}
