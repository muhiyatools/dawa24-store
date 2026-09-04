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
// Ordered by what the disagreement proves rather than by how visible it is. A
// dose that differs proves two products; a tube that is 25 g on one side and 50
// on the other proves two products; a cream against an ointment is two products
// in this catalogue but is also the attribute suppliers are loosest about, so
// it carries less.
const (
	massDose      = 1.00 // 500 mg is not 1 g
	massDoseParts = 0.90 // 16 mg is not 16/12.5 mg — a combination is another product
	massModifier  = 0.90 // بانادول is not بانادول اكسترا
	massForm      = 0.85 // a syrup is not a tablet
	massLetter    = 0.70 // بتنوفيت ان is not بتنوفيت سي
	massCount     = 0.55 // 20 tablets is not 200
	massFigure    = 0.50 // a figure that named nothing, and differs anyway
	massSubForm   = 0.45 // a cream is not an ointment
	massPack      = 0.35 // the pack-size column disagreeing with the name
)

// conflictsOf lists everything a candidate contradicts about a row.
//
// Only attributes the row actually states are compared. Silence is not
// disagreement: a pharmacy writes "بانادول" and means the only بانادول there
// is, and holding that against a catalogue entry which spells out its strength
// would refuse the commonest correct match in the file.
func (idx *Index) conflictsOf(q *query, p *MasterProduct) []conflict {
	var best []conflict
	for i, side := range p.sides() {
		cs := idx.conflictsAgainst(q, p, side)
		if i == 0 || conflictMass(cs) < conflictMass(best) {
			best = cs
		}
		if len(best) == 0 {
			break
		}
	}
	return best
}

// conflictsAgainst compares a row with one spelling of a candidate.
func (idx *Index) conflictsAgainst(q *query, p *MasterProduct, side nameFacts) []conflict {
	var out []conflict

	switch agree, comparable := compareStrengths(q.strengths, p.strengths); {
	case !comparable:
		// Neither side measured the same thing. Missing information.
	case agree:
		// The doses match. Whether they match in the same NUMBER of parts is a
		// separate question and the one the combination families turn on.
		if doseComponentsDiffer(q.strengths, p.strengths) {
			out = append(out, conflict{"dose_parts", massDoseParts})
		}
	default:
		out = append(out, conflict{"strength", massDose})
	}

	if modifierSetsConflict(q.mods, side.mods, q.rawName, p.NameAR+" "+p.NameEN) {
		out = append(out, conflict{"modifier", massModifier})
	}
	if !markSetsAgree(q.marks, side.marks) {
		out = append(out, conflict{"letter", massLetter})
	}
	sideForm := p.formOf(side)
	if q.formKey != "" && sideForm != "" && q.formKey != sideForm {
		out = append(out, conflict{"form", massForm})
	} else if q.subForm != "" && side.subForm != "" && q.subForm != side.subForm {
		out = append(out, conflict{"sub_form", massSubForm})
	}
	countConflict := countsDisagree(q.qty.counts, side.qty.counts)
	if countConflict {
		out = append(out, conflict{"count", massCount})
	} else if residualsDisagree(q.qty.residual, side.qty.residual) {
		out = append(out, conflict{"figure", massFigure})
	}
	if q.packSize > 0 && p.packSize > 0 && q.packSize != p.packSize && !countConflict {
		// Only when the names did not already say so, or the same difference
		// would be charged twice.
		out = append(out, conflict{"pack", massPack})
	}
	return out
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
