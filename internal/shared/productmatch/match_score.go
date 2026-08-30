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
//
// What the name may do at all is decided in match_evidence.go, and it is
// decided before any of this runs. A candidate that agrees with the row only on
// words the catalogue carries by the thousand never reaches here, whatever the
// dose and the form would have said about it.

// corroborationFloor is the share of an attribute bonus that survives when the
// names agree not at all, and corroborationFull is the name agreement above
// which corroboration counts in full.
const (
	corroborationFloor = 0.50
	corroborationFull  = 0.50
)

// corroborationFactor discounts attribute agreement when the names barely
// agree, and leaves it alone once they do.
//
// The shape matters more than the numbers. Above half a name in common the
// attributes are corroborating a real candidate and must count for what they
// are worth — clipping them there costs correct matches on every vendor file
// measured. Below it they are the only thing holding the match up, and an
// attribute is a poor thing to hold a match up with: a bottle size, a
// pharmaceutical form and a round figure are each true of a hundred products,
// and together they were worth enough to carry a 20% name agreement past the
// applied threshold.
func corroborationFactor(name float64) float64 {
	if name >= corroborationFull {
		return 1
	}
	return corroborationFloor + (1-corroborationFloor)*(name/corroborationFull)
}

// rate scores one catalogue product against one row.
func (idx *Index) rate(q *query, p *MasterProduct) (scoredProduct, bool) {
	ev := idx.nameEvidenceOf(q, p)
	if !ev.sufficient(q) {
		return scoredProduct{}, false
	}
	name := ev.similarity

	weight := 0.75
	if !q.strength.known() && !p.strength.known() {
		// When neither side states a strength (cosmetics, OTC, supplies, devices),
		// the product name carries the full identity. Weighting it appropriately
		// prevents certain non-pharmaceutical matches from falling below the cutoff.
		weight = 0.88
	}
	score := name * weight
	// Corroboration is accumulated apart from the score and applied scaled by
	// the name, because an attribute agreeing is not evidence of identity on
	// its own.
	//
	// It used to be added flat, and that is how "ستار فيل غسول نسائى جيل مرطب
	// 200 مل" reached 0.50 against another company's "تيلير غسول نسائي 200 مل":
	// the names agreed on 0.20 and the bottle size, the pharmaceutical form and
	// the figure 200 added 0.32 between them. Every one of those three is true
	// of a hundred products. Penalties are NOT scaled — a dose that contradicts
	// contradicts whatever the names look like.
	var bonus float64
	reasons := make([]string, 0, 5)
	reasons = append(reasons, "تشابه الاسم "+percent(name))

	// The dose. Disagreement is disqualifying rather than merely negative: two
	// strengths of one brand are two products, and pricing one as the other is
	// the single most expensive mistake this engine can make.
	//
	// Compared per unit and over every dose each side states, not first against
	// first. Both of those were real defects. "ايفرزين 30 جم كريم" against the
	// catalogue's "ايفرزين 1% كريم 30 جم" is one product; comparing a tube size
	// against a concentration and calling the difference a conflict cost it
	// 0.45 and pushed it under the threshold. "جانوميت 1000/50مجم" against
	// "جانوميت 50/1000مجم" is one product written in the other order; taking
	// only the leading figure made them contradict.
	doseAgree, doseComparable := compareStrengths(q.strengths, p.strengths)
	switch {
	case doseComparable && doseAgree:
		bonus += doseBonus(q.strengths, p.strengths)
		reasons = append(reasons, "تطابق التركيز")
	case doseComparable:
		score -= 0.45
		reasons = append(reasons, "اختلاف التركيز")
	case q.strength.known() != p.strength.known():
		// One side states a dose and the other does not. That is missing
		// information, not a conflict, and must not be scored as either.
		score -= 0.02
	}

	// The line-extension word, which is the whole difference between two
	// products in one brand family.
	//
	// It used to be checked only on an answer the model proposed, which left
	// the deterministic scorer unable to tell "بانادول" from "بانادول اكسترا"
	// by anything but one token of name similarity — and one token out of four
	// is a rounding error against a brand they genuinely share. The vocabulary
	// is curated precisely so that its absence from a terser supplier line is
	// not read as evidence, so it is safe to apply here on every comparison.
	modMismatch := modifierSetsConflict(q.mods, p.mods, q.rawName, p.NameAR+" "+p.NameEN)
	if modMismatch {
		score -= 0.35
		reasons = append(reasons, "اختلاف في صنف المنتج داخل نفس العلامة")
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
			bonus += 0.10
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
			bonus += 0.10
			reasons = append(reasons, "تطابق الشركة")
		case strings.Contains(p.makerKey, q.makerKey) || strings.Contains(q.makerKey, p.makerKey):
			bonus += 0.06
		}
	}

	// The molecule, where the file names it.
	if q.sciKey != "" && p.sciKey != "" && q.sciKey == p.sciKey {
		bonus += 0.08
		reasons = append(reasons, "تطابق المادة الفعالة")
	}

	numsAgree := true
	if delta, compared := numberDelta(q.nums, p.nums); compared {
		if delta {
			bonus += 0.06
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
			bonus += 0.05
		} else {
			score -= 0.12
			numsAgree = false
		}
	}

	score += bonus * corroborationFactor(name)

	noConflict := !modMismatch && (!doseComparable || doseAgree) &&
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

// doseBonus is what an agreeing dose is worth, which depends on what was
// measured.
//
// A milligram is a dose: two products agreeing on 500 mg are very likely the
// same medicine, and that is the strongest corroboration in the row. A
// millilitre is usually a bottle, and "200 مل" is true of a hundred unrelated
// washes — counting it as a dose match is how a 20% name agreement reached the
// applied threshold on a cosmetics file. Percentages sit between: a topical's
// 1% is a real concentration, but round percentages are also common.
func doseBonus(rowDoses, prodDoses []strength) float64 {
	best := 0.0
	for _, a := range rowDoses {
		for _, b := range prodDoses {
			if a.unit != b.unit || !sameStrength(a, b) {
				continue
			}
			switch a.unit {
			case "mg", "iu":
				return 0.16
			case "%", "spf":
				best = maxF(best, 0.12)
			default:
				best = maxF(best, 0.07)
			}
		}
	}
	return best
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
