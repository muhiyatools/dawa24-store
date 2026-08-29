package productmatch

import (
	"strings"
)

// Per-candidate scoring.
//
// The name gets a candidate onto the list; the attributes decide whether it
// stays. That order matters because the name is the least reliable field a
// supplier writes and the dose is the most: "أوجمنتين 1 جم" and "أوجمنتين 625"
// differ by one token out of three and are different medicines, while "بانادول
// اكسترا سعر جديد" and "Panadol Extra" differ in every token and are the same
// one.

// rate scores one catalogue product against one row.
func (idx *Index) rate(q *query, p *MasterProduct) (scoredProduct, bool) {
	name := idx.nameSimilarity(q, p)
	if name < 0.18 {
		return scoredProduct{}, false
	}

	weight := 0.75
	if !q.strength.known() && !p.strength.known() {
		// When neither side states a strength (cosmetics, OTC, supplies, devices),
		// the product name carries the full identity. Weighting it appropriately
		// prevents certain non-pharmaceutical matches from falling below the cutoff.
		weight = 0.88
	}
	score := name * weight
	reasons := make([]string, 0, 4)
	reasons = append(reasons, "تشابه الاسم "+percent(name))

	// The dose. Disagreement is disqualifying rather than merely negative: two
	// strengths of one brand are two products, and pricing one as the other is
	// the single most expensive mistake this engine can make.
	switch {
	case q.strength.known() && p.strength.known():
		if sameStrength(q.strength, p.strength) {
			score += 0.16
			reasons = append(reasons, "تطابق التركيز")
		} else {
			score -= 0.45
			reasons = append(reasons, "اختلاف التركيز")
		}
	case q.strength.known() != p.strength.known():
		// One side states a dose and the other does not. That is missing
		// information, not a conflict, and must not be scored as either.
		score -= 0.02
	}

	// The form. A syrup and a tablet of the same brand are different products.
	//
	// The guard on name similarity is there because catalogue metadata is not
	// always consistent with the catalogue's own names: a product called
	// "فلاى بيبى طبق لوكس + معلقه" is filed under a dosage form that reads as
	// tablets, and penalising an otherwise identical name for its own record's
	// bookkeeping turns a certain match into one the vendor has to confirm.
	if name < 0.98 && q.formKey != "" && p.formKey != "" {
		if q.formKey == p.formKey {
			score += 0.10
			reasons = append(reasons, "تطابق الشكل الصيدلي")
		} else {
			score -= 0.28
			reasons = append(reasons, "اختلاف الشكل الصيدلي")
		}
	}

	// The manufacturer corroborates but never disqualifies: supplier files write
	// the agent, the licensee and the parent company interchangeably.
	if q.makerKey != "" && p.makerKey != "" {
		switch {
		case q.makerKey == p.makerKey:
			score += 0.10
			reasons = append(reasons, "تطابق الشركة")
		case strings.Contains(p.makerKey, q.makerKey) || strings.Contains(q.makerKey, p.makerKey):
			score += 0.06
		}
	}

	// The molecule, where the file names it.
	if q.sciKey != "" && p.sciKey != "" && q.sciKey == p.sciKey {
		score += 0.08
		reasons = append(reasons, "تطابق المادة الفعالة")
	}

	numsAgree := true
	if delta, compared := numberDelta(q.nums, p.nums); compared {
		if delta {
			score += 0.06
		} else {
			// Same words, different figures: a 25-wipe pack against a 50-wipe
			// pack, or a 60-gum bottle against a 120. Nothing else in the row
			// tells those apart, so this carries the separation.
			score -= 0.22
			numsAgree = false
			reasons = append(reasons, "اختلاف الأرقام في الاسم (حجم أو عدد العبوة)")
		}
	}

	if q.packSize > 0 && p.packSize > 0 {
		if q.packSize == p.packSize {
			score += 0.05
		} else {
			score -= 0.12
			numsAgree = false
		}
	}

	noConflict := (!q.strength.known() || !p.strength.known() || sameStrength(q.strength, p.strength)) &&
		(q.formKey == "" || p.formKey == "" || q.formKey == p.formKey)

	exact := (name >= 0.98 || (name >= 0.90 && weight == 0.88)) && numsAgree && noConflict
	if exact {
		score = maxF(score, 0.97)
	} else if name >= 0.88 && numsAgree && noConflict {
		score = maxF(score, name*0.95)
	}

	return scoredProduct{
		product: p,
		score:   clamp(score),
		reason:  strings.Join(reasons, " + "),
		exact:   exact,
	}, true
}

// numberDelta compares the figures in two names. compared is false when either
// name carries none, which is missing information rather than agreement.
func numberDelta(a, b []float64) (agree, compared bool) {
	if len(a) == 0 || len(b) == 0 {
		return false, false
	}
	// Both sorted and deduplicated at construction, so one walk settles it.
	i, j, inter := 0, 0, 0
	for i < len(a) && j < len(b) {
		switch {
		case a[i] == b[j]:
			inter++
			i++
			j++
		case a[i] < b[j]:
			i++
		default:
			j++
		}
	}
	// Agreement means the shorter list is wholly contained in the longer one.
	// A supplier writing "بانادول 500 مجم 20 قرص" against a catalogue entry of
	// "بانادول 500مجم" agrees — the extra figure is the pack count the
	// catalogue omitted. "دوركو ايف 3 … 4 غيار" against "دوركو ايف 6 … 4 غيار"
	// does not: one figure matches and one contradicts, and the contradicting
	// one is the whole difference between the two products.
	shortest := len(a)
	if len(b) < shortest {
		shortest = len(b)
	}
	return inter >= shortest, true
}

// nameSimilarity compares two product names on their identifying words.
//
// Two measures are combined and the better wins. Weighted containment asks how
// much of the query's distinctive vocabulary the catalogue name carries, which
// is what a terse supplier line needs against a fully spelled catalogue entry.
// Trigram overlap catches the transliteration differences that leave no whole
// word shared at all — "ابيكوبريد" against "ابيكوبرايد".
func (idx *Index) nameSimilarity(q *query, p *MasterProduct) float64 {
	best := 0.0
	if v := idx.weightedContainment(q, p.coreAR); v > best {
		best = v
	}
	if v := idx.weightedContainment(q, p.coreEN); v > best {
		best = v
	}
	if best >= 0.99 || len(q.tri) == 0 {
		return best
	}

	// Trigram agreement is weaker evidence than a shared word, so it is
	// discounted before it can outrank one. Both sides are sorted sets built
	// once, so this is a walk rather than a set construction.
	for _, tri := range [][]string{p.triAR, p.triEN} {
		if v := jaccardSorted(q.tri, tri) * 0.92; v > best {
			best = v
		}
	}
	return best
}

// weightedContainment is the share of the query's information present in the
// candidate, weighted by how rare each word is in the catalogue.
//
// Rarity weighting is what stops "بانادول" — carried by forty products —
// counting as much as "اكسترا" in telling them apart, and it is why a two-word
// query against a five-word catalogue name is not punished for the three words
// it did not repeat.
func (idx *Index) weightedContainment(q *query, candidate []string) float64 {
	if q.totalWeight <= 0 || len(candidate) == 0 {
		return 0
	}
	// Each query token counts once however often the candidate repeats it. The
	// epoch stamp marks which were consumed in this comparison without clearing
	// a map three million times over a large file.
	q.epoch++
	var matched float64
	hits := 0
	for _, t := range candidate {
		i, ok := q.pos[t]
		if !ok || q.stamp[i] == q.epoch {
			continue
		}
		q.stamp[i] = q.epoch
		matched += q.weights[i]
		hits++
	}
	if matched <= 0 {
		return 0
	}

	ratio := matched / q.totalWeight
	// Punish a candidate carrying far more distinctive words than the query, so
	// "بانادول" does not match "بانادول اكسترا اقراص مسكن قوي" as strongly as it
	// matches "بانادول".
	if extra := len(candidate) - hits; extra > 0 {
		ratio *= 1 - minF(0.25, float64(extra)*0.05)
	}
	return clamp(ratio)
}

// sameStrength compares two doses, tolerating the rounding a supplier applies
// when they write 0.5g for 500mg.
func sameStrength(a, b strength) bool {
	if a.unit != b.unit {
		return false
	}
	if a.value == b.value {
		return true
	}
	hi, lo := a.value, b.value
	if hi < lo {
		hi, lo = lo, hi
	}
	if lo <= 0 {
		return false
	}
	return (hi-lo)/hi <= 0.01
}

func percent(v float64) string {
	return itoa(int(v*100)) + "%"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [12]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

func maxF(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func minF(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
