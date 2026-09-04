package productmatch

// Diagnostics.
//
// The labelled benchmarks in test/corpus need to say WHY a wrong match was
// wrong — whether the two products differ by a strength, a pack figure, a
// line-extension word or a dosage form — and every one of those comparisons
// already exists here, unexported, because the scorer is the only thing that
// needed them.
//
// Exporting a thin, read-only layer is better than the alternatives: copying
// the comparisons into the benchmark makes the benchmark measure a second
// implementation, and moving them out of this package puts the scorer's own
// vocabulary somewhere it cannot see.

import (
	"sort"
	"strings"
)

// DebugName is a catalogue product's label, Arabic first.
func DebugName(p *MasterProduct) string {
	if p == nil {
		return ""
	}
	if p.NameAR != "" {
		return p.NameAR
	}
	return p.NameEN
}

// DebugStrengthDiffers reports whether the two candidates state different dose
// sets, on the units they have in common.
func DebugStrengthDiffers(rowText string, got, want *MasterProduct) bool {
	return !strengthSetsEqual(strengthSet(productText(got)), strengthSet(productText(want)))
}

// DebugNumbersDiffer reports whether the two candidates carry different figures
// in their names — the pack count, the bottle size, the wipe count.
func DebugNumbersDiffer(rowText string, got, want *MasterProduct) bool {
	return !floatsEqual(got.nums, want.nums)
}

// DebugModifierDiffers reports whether the two candidates carry different
// line-extension words.
func DebugModifierDiffers(rowText string, got, want *MasterProduct) bool {
	_, differ := modifierSetsDiffer(got.mods, want.mods, productText(got), productText(want))
	return differ
}

// DebugFormDiffers reports whether the two candidates are filed under different
// vetoable dosage forms.
func DebugFormDiffers(row *Row, got, want *MasterProduct) bool {
	a, b := vetoableForm(got.formKey), vetoableForm(want.formKey)
	return a != "" && b != "" && a != b
}

// DebugExtraWord returns a word one candidate's name carries and the other's
// does not, which is what separates them when no structured attribute does.
func DebugExtraWord(rowText string, got, want *MasterProduct) string {
	a := append(append([]string{}, got.coreAR...), got.coreEN...)
	b := append(append([]string{}, want.coreAR...), want.coreEN...)
	if w := firstMissing(b, a); w != "" {
		return w
	}
	return firstMissing(a, b)
}

func firstMissing(from, in []string) string {
	for _, w := range from {
		found := false
		for _, o := range in {
			if o == w {
				found = true
				break
			}
		}
		if !found {
			return w
		}
	}
	return ""
}

func productText(p *MasterProduct) string {
	return p.NameAR + " " + p.NameEN + " " + p.Concentration
}

func strengthSetsEqual(a, b []strength) bool {
	if len(a) != len(b) {
		return true // different counts are a difference
	}
	for _, x := range a {
		hit := false
		for _, y := range b {
			if x.unit == y.unit && sameStrength(x, y) {
				hit = true
				break
			}
		}
		if !hit {
			return false
		}
	}
	return true
}

func floatsEqual(a, b []float64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// DebugRowFacts renders what the engine extracted from a row, for a diagnostic
// that needs to say what the scorer was actually comparing.
func DebugRowFacts(idx *Index, row *Row) string {
	q := idx.newQuery(row)
	var b strings.Builder
	b.WriteString("tokens=")
	b.WriteString(strings.Join(q.tokens, ","))
	b.WriteString(" form=")
	b.WriteString(q.formKey)
	b.WriteString(" doses=")
	for _, s := range q.strengths {
		b.WriteString(formatFloat(s.value))
		b.WriteString(s.unit)
		b.WriteByte(' ')
	}
	b.WriteString(" nums=")
	for _, n := range q.nums {
		b.WriteString(formatFloat(n))
		b.WriteByte(' ')
	}
	b.WriteString(" mods=")
	for m := range q.mods {
		b.WriteString(m)
		b.WriteByte(' ')
	}
	return b.String()
}

// DebugProductFacts renders the same for a catalogue product.
func DebugProductFacts(p *MasterProduct) string {
	var b strings.Builder
	b.WriteString("tokens=")
	b.WriteString(strings.Join(p.coreAR, ","))
	b.WriteString(" form=")
	b.WriteString(p.formKey)
	b.WriteString(" doses=")
	for _, s := range p.strengths {
		b.WriteString(formatFloat(s.value))
		b.WriteString(s.unit)
		b.WriteByte(' ')
	}
	b.WriteString(" nums=")
	for _, n := range p.nums {
		b.WriteString(formatFloat(n))
		b.WriteByte(' ')
	}
	b.WriteString(" mods=")
	for m := range p.mods {
		b.WriteString(m)
		b.WriteByte(' ')
	}
	return b.String()
}

func formatFloat(v float64) string {
	if v == float64(int64(v)) {
		return itoa(int(v))
	}
	whole := int64(v)
	frac := int((v - float64(whole)) * 100)
	if frac < 0 {
		frac = -frac
	}
	return itoa(int(whole)) + "." + itoa(frac)
}

// DebugConflicts names every disagreement the engine finds between a row and
// one catalogue product, for the benchmark that measures how often a
// discrimination rule fires against the correct answer.
func DebugConflicts(idx *Index, row *Row, productID int64) []string {
	p, ok := idx.byID[productID]
	if !ok {
		return nil
	}
	cs := idx.conflictsOf(idx.newQuery(row), p)
	out := make([]string, 0, len(cs))
	for _, c := range cs {
		out = append(out, c.kind)
	}
	return out
}

// DebugMarks and DebugSubForm expose the two signals added most recently, so a
// probe can say what a name produced rather than infer it from an outcome.
func DebugMarks(text string) []string {
	out := make([]string, 0, 2)
	for k := range identityMarks(text) {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// DebugSubForm exposes the topical sub-form a name states.
func DebugSubForm(text string) string { return topicalSubForm(text) }

// DebugModifiers exposes the line-extension keys a name carries.
func DebugModifiers(text string) []string {
	out := make([]string, 0, 2)
	for k := range modifiersIn(text) {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// DebugSides renders each of a product's name reductions, so a probe can say
// which spelling a comparison actually agreed with.
func DebugSides(p *MasterProduct) []string {
	out := make([]string, 0, 2)
	for _, f := range p.sides() {
		s := "form=" + f.formKey + " sub=" + f.subForm + " counts="
		for _, c := range f.qty.counts {
			s += c.class + ":" + formatFloat(c.value) + " "
		}
		s += "residual="
		for _, r := range f.qty.residual {
			s += formatFloat(r) + " "
		}
		s += "marks="
		for m := range f.marks {
			s += m + " "
		}
		s += "mods="
		for m := range f.mods {
			s += m + " "
		}
		out = append(out, s)
	}
	return out
}
