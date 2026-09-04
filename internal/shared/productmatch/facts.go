package productmatch

// One name, reduced to the attributes it states.
//
// A catalogue product has two names and they do not always say the same thing.
// "اكتي-فيتا جولد 50 مل" carries one line-extension word; the English name the
// same record holds is "acti-vita gold procgen creme day/night 50 ml" and
// carries three. "ادفاجراف 0.5 مجم 100 كبسولات" states no identity letter; its
// English name says "prolonged r. caps" and states an R. Neither is wrong —
// they were written by different people for different screens — but a
// comparison that pools both sides asks the row to agree with the UNION of two
// descriptions, and no supplier line agrees with that.
//
// Measured on twenty thousand labelled rows, pooling raised a conflict against
// the correct product on one row in twenty for the modifier check alone, every
// one of them a product disagreeing with itself.
//
// So each side is reduced separately and the row is compared against both. It
// agrees with a product when it agrees with EITHER of the product's names,
// which is the same rule nameEvidenceOf has always applied to similarity and
// for the same reason.

// nameFacts is what one spelling of a product name states about it.
type nameFacts struct {
	// formKey is the pharmaceutical form the NAME states. The record's own
	// dosage-form column is kept apart, in MasterProduct.formMeta, because it
	// is bookkeeping rather than something a supplier wrote: a solution filed
	// under "topical" contradicts a row that reads "محلول" off the same name.
	formKey string
	subForm string
	mods    map[string]struct{}
	marks   map[string]struct{}
	qty     quantities
	// strengths are the doses the NAME states. The record's concentration
	// column is kept apart, in MasterProduct.strengthsMeta, and consulted only
	// where the name states nothing — it disagrees with the name it sits beside
	// often enough to matter. "ايه سي سي 200مجم 20 كيس" is filed under a
	// concentration of 20 mg; "اكنيتون 5مجم/مل" under 15 mg/ml. Pooling the two
	// gave every such product a second, wrong strength that contradicted every
	// row asking for it, including a row that had copied the name exactly.
	strengths []strength
}

// factsOf reduces one name.
func factsOf(name string) nameFacts {
	if name == "" {
		return nameFacts{}
	}
	return nameFacts{
		formKey:   formKeyOf(name),
		subForm:   topicalSubForm(name),
		mods:      modifiersIn(name),
		marks:     identityMarks(name),
		qty:       readQuantities(name),
		strengths: strengthSet(name),
	}
}

// empty reports a side that states nothing worth comparing, which is a record
// holding only one of its two names.
func (f nameFacts) empty() bool {
	return f.formKey == "" && f.subForm == "" &&
		len(f.mods) == 0 && len(f.marks) == 0 && len(f.strengths) == 0 &&
		len(f.qty.counts) == 0 && len(f.qty.residual) == 0
}

// sides returns the name descriptions a candidate may be compared against,
// widest first.
//
// A record with one usable name is compared against that one. A record with two
// is compared against both and keeps the better answer — see conflictsOf. A
// record with neither is compared against the pooled facts, which is what a
// name too terse to state anything reduces to anyway.
func (p *MasterProduct) sides() []nameFacts {
	switch {
	case p.factsEN.empty():
		return []nameFacts{p.factsAR}
	case p.factsAR.empty():
		return []nameFacts{p.factsEN}
	}
	return []nameFacts{p.factsAR, p.factsEN}
}

// formOf is the form a side states, falling back to the record's own dosage-form
// column where the name states none.
//
// The order is the point. A supplier and a catalogue both write the form into
// the name, and where both do, that is the comparison. The column is consulted
// only where the name is silent, because it is filled by an importer rather
// than by a person and it is wrong often enough to matter: eight hundred
// products in the live catalogue carry a dosage form their own name
// contradicts.
func (p *MasterProduct) formOf(f nameFacts) string {
	if f.formKey != "" {
		return f.formKey
	}
	return p.formMeta
}

// dosesOf is the strengths a side states, falling back to the record's own
// concentration column where the name states none.
//
// Same order as formOf, for the same reason and on the same evidence: the name
// is what a person wrote and the column is what an importer filled in.
func (p *MasterProduct) dosesOf(f nameFacts) []strength {
	if len(f.strengths) > 0 {
		return f.strengths
	}
	return p.strengthsMeta
}
