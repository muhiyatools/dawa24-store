package productmatch

import (
	"fmt"
	"sort"

	"github.com/muhiya/dawa24-store/internal/shared/sheet"
)

// Resolution.
//
// Two independent witnesses per column — what the header claims, and what the
// values are — combined into one score, then assigned globally so the strongest
// evidence anywhere in the sheet wins rather than the leftmost.
//
// Global assignment is the part that is easy to get wrong and expensive to get
// wrong. Deciding column by column, first match winning, is how an importer
// binds "الباركود الدولي" to the item code — the code rule tests for "كود",
// which is a substring of "باركود" — and then never looks at the real item-code
// column further right. Scoring every pair and settling the best first cannot
// make that mistake.

// Confidence buckets a binding for the review screen.
type Confidence string

const (
	// ConfidenceCertain means both witnesses agree, or one is conclusive.
	ConfidenceCertain Confidence = "certain"
	// ConfidenceHigh means the binding is sound but worth a glance.
	ConfidenceHigh Confidence = "high"
	// ConfidenceMedium means the vendor should confirm it.
	ConfidenceMedium Confidence = "medium"
	// ConfidenceLow means the engine is guessing and says so.
	ConfidenceLow Confidence = "low"
)

// Label renders a confidence level in Arabic.
func (c Confidence) Label() string {
	switch c {
	case ConfidenceCertain:
		return "مؤكد"
	case ConfidenceHigh:
		return "مرجّح"
	case ConfidenceMedium:
		return "يحتاج مراجعة"
	default:
		return "غير مؤكد"
	}
}

// NeedsReview reports whether a binding should be flagged to the vendor.
func (c Confidence) NeedsReview() bool {
	return c == ConfidenceMedium || c == ConfidenceLow
}

// Source records who decided a binding.
type Source string

const (
	// SourceAuto is the analyser's own decision.
	SourceAuto Source = "auto"
	// SourceManual is the vendor's override.
	SourceManual Source = "manual"
	// SourceCompleted is a binding the engine filled in after the vendor's
	// review, for a field they left unmapped.
	SourceCompleted Source = "completed"
)

// Candidate is one field a column could be, with the evidence for it.
type Candidate struct {
	Field Field    `json:"field"`
	Label string   `json:"label"`
	Score float64  `json:"score"`
	Why   []string `json:"why,omitempty"`
}

// Column is one spreadsheet column and what became of it.
type Column struct {
	Index   int    `json:"index"`
	Header  string `json:"header"`
	Field   Field  `json:"field,omitempty"`
	Ignored bool   `json:"ignored"`

	Score      float64    `json:"score"`
	Confidence Confidence `json:"confidence,omitempty"`
	Source     Source     `json:"source,omitempty"`
	// Why explains the binding in Arabic, one clause per witness.
	Why []string `json:"why,omitempty"`
	// Candidates are the other fields this column scored for, best first, which
	// is what the review screen offers in its dropdown.
	Candidates []Candidate `json:"candidates,omitempty"`
	// Preview holds a few real values, so the vendor recognises the column
	// without opening their own file beside the screen.
	Preview []string `json:"preview,omitempty"`
	// Profile is the measured evidence, kept for the explanation panel.
	Profile *sheet.ColumnProfile `json:"profile,omitempty"`
}

// Conflict is something the vendor must look at before the import runs.
type Conflict struct {
	Kind     ConflictKind `json:"kind"`
	Field    Field        `json:"field,omitempty"`
	Column   int          `json:"column"`
	Severity Severity     `json:"severity"`
	Message  string       `json:"message"`
}

// ConflictKind classifies why a binding was flagged.
type ConflictKind string

const (
	// ConflictHeaderValue means the header names one field and the values look
	// like another. This is the finding that saves imports.
	ConflictHeaderValue ConflictKind = "header_value"
	// ConflictAmbiguous means two readings scored too close to separate.
	ConflictAmbiguous ConflictKind = "ambiguous"
	// ConflictDuplicateHeader means two columns carry the same title.
	ConflictDuplicateHeader ConflictKind = "duplicate_header"
	// ConflictMissing means a field the import depends on found no column.
	ConflictMissing ConflictKind = "missing"
	// ConflictInconsistent comes from the cross-column arithmetic checks.
	ConflictInconsistent ConflictKind = "inconsistent"
)

// Mapping is the resolved reading of a sheet's columns.
type Mapping struct {
	Columns   []*Column     `json:"columns"`
	ByField   map[Field]int `json:"by_field"`
	Conflicts []Conflict    `json:"conflicts,omitempty"`
	Notes     []Note        `json:"notes,omitempty"`
}

// Column returns the column bound to a field, and whether one is.
func (m *Mapping) Column(f Field) (int, bool) {
	idx, ok := m.ByField[f]
	return idx, ok
}

// Has reports whether a field is bound.
func (m *Mapping) Has(f Field) bool {
	_, ok := m.ByField[f]
	return ok
}

// Bound lists the bound fields in catalogue order, which is the order the
// review screen renders.
func (m *Mapping) Bound() []Field {
	var out []Field
	for _, spec := range Specs {
		if m.Has(spec.Field) {
			out = append(out, spec.Field)
		}
	}
	return out
}

// Unmapped lists the columns no field claimed.
func (m *Mapping) Unmapped() []*Column {
	var out []*Column
	for _, c := range m.Columns {
		if c.Field == "" && !c.Ignored {
			out = append(out, c)
		}
	}
	return out
}

// NeedsReview reports whether anything is flagged for the vendor's attention.
func (m *Mapping) NeedsReview() bool {
	for _, c := range m.Conflicts {
		if c.Severity != SeverityInfo {
			return true
		}
	}
	for _, c := range m.Columns {
		if c.Field != "" && c.Confidence.NeedsReview() {
			return true
		}
	}
	return false
}

// pair is one scored (field, column) reading awaiting assignment.
type pair struct {
	field  Field
	column int
	score  float64
	why    []string
	// headerOnly marks a reading the values could not corroborate, which the
	// conflict pass reports.
	headerScore float64
	valueScore  float64
	hasValue    bool
	// namedExactly marks a reading whose column header *is* this field's name,
	// as opposed to merely containing a word associated with it.
	namedExactly bool
}

// minPairScore is the floor below which a reading is not offered at all. It is
// low on purpose: a weak reading still belongs in the review screen's dropdown,
// where the vendor can pick it, even though it is too weak to apply by itself.
const minPairScore = 0.22

// minBindScore is the floor for binding a field without being asked.
const minBindScore = 0.34

// uncorroboratedCap is the ceiling on a reading that neither the header
// supports nor the values settle. It sits below minBindScore on purpose: such a
// reading is offered in the review dropdown and never applied by itself.
const uncorroboratedCap = 0.30

// score combines the two witnesses.
//
// The combination is a maximum of three readings rather than a single weighted
// sum, because a sum dilutes conclusive evidence. A column of valid GS1 check
// digits under a header nobody wrote is certainly a barcode; a weighted average
// against a zero header would score it 0.45 and leave it unmapped, which is a
// worse answer than either witness gave on its own.
func combineScores(headerScore int, v verdict, hasDetector bool) (float64, []string) {
	// A field whose values do not identify it on their own needs the header to
	// vouch for it. Half the catalogue is in that position — a small whole
	// number is equally a pack size, a minimum order and a re-order level — and
	// binding those on shape alone is how an import invents data the file never
	// contained. Capping below the binding threshold keeps them available in
	// the review dropdown while refusing to assert them.
	header := clamp(float64(headerScore) / 130)
	var why []string
	if headerScore >= scoreExact {
		why = append(why, "عنوان العمود مطابق تماماً لهذا الحقل")
	} else if headerScore >= scoreStrong {
		why = append(why, "عنوان العمود يشير بوضوح إلى هذا الحقل")
	} else if headerScore >= scoreFloor {
		why = append(why, "عنوان العمود يحتمل هذا الحقل")
	}
	if v.why != "" {
		why = append(why, v.why)
	}

	if !hasDetector {
		// Nothing measurable distinguishes this field's values, so the header
		// carries it alone — and is capped, because an uncorroborated header is
		// never certain.
		return clamp(header * 0.8), why
	}
	if headerScore < scoreFloor && !v.decisive {
		return clamp(min(v.score*0.85, uncorroboratedCap)), why
	}

	blended := 0.55*header + 0.45*v.score
	if header >= 0.5 && v.score >= 0.55 {
		blended += 0.15 // both witnesses agree
	}
	best := blended
	if solo := v.score * 0.85; solo > best {
		best = solo
	}
	if solo := header * 0.8; solo > best {
		best = solo
	}
	return clamp(best), why
}

// confidenceOf buckets a score.
func confidenceOf(score float64) Confidence {
	switch {
	case score >= 0.82:
		return ConfidenceCertain
	case score >= 0.62:
		return ConfidenceHigh
	case score >= 0.42:
		return ConfidenceMedium
	default:
		return ConfidenceLow
	}
}

// Resolve reads a sheet's headers and profiled values into a mapping, offering
// every field in the catalogue.
func Resolve(headers []string, profiles []*sheet.ColumnProfile, vocab *Vocabulary) *Mapping {
	return ResolveWith(headers, profiles, vocab, nil)
}

// ResolveWith reads a sheet under an explicit field set. A nil set allows every
// field, which is what Resolve passes.
func ResolveWith(headers []string, profiles []*sheet.ColumnProfile, vocab *Vocabulary, fields *FieldSet) *Mapping {
	if vocab == nil {
		vocab = &Vocabulary{}
	}
	m := &Mapping{ByField: map[Field]int{}}

	shapes := make([]*shape, len(profiles))
	for i, p := range profiles {
		shapes[i] = newShape(p)
		header := ""
		if i < len(headers) {
			header = headers[i]
		}
		m.Columns = append(m.Columns, &Column{
			Index:   i,
			Header:  header,
			Profile: p,
			Preview: previewOf(p),
		})
	}

	pairs, vetoes := scorePairs(headers, shapes, vocab, fields)
	assign(m, pairs)
	// The one place value evidence may overturn a settled header binding. See
	// identity_swap.go for why it is limited to this pair.
	fixIdentitySwap(m, shapes, headers)
	attachCandidates(m, pairs)
	flagConflicts(m, pairs, vetoes, headers)
	return m
}

// scorePairs evaluates every field against every column.
func scorePairs(headers []string, shapes []*shape, vocab *Vocabulary, fields *FieldSet) ([]pair, map[Field]map[int]string) {
	vetoes := map[Field]map[int]string{}
	var pairs []pair
	named := map[int]bool{}

	for col := range shapes {
		header := ""
		if col < len(headers) {
			header = headers[col]
		}
		evidence := headerEvidence(header)
		for _, he := range evidence {
			if he.Score >= scoreStrong && !he.Blocked {
				named[col] = true
				break
			}
		}

		for _, spec := range Specs {
			f := spec.Field
			if !fields.Allows(f) {
				continue
			}
			he := evidence[f]
			if he.Blocked {
				continue
			}
			v := valueEvidence(f, shapes[col], vocab)
			if v.veto {
				if vetoes[f] == nil {
					vetoes[f] = map[int]string{}
				}
				vetoes[f][col] = v.why
				continue
			}
			_, hasDetector := detectors[f]
			score, why := combineScores(he.Score, v, hasDetector)
			if score < minPairScore {
				continue
			}
			// A column whose header clearly names a field is not available to a
			// field the header does not mention at all, however the values
			// profile. Both halves matter: the named field may still lose to
			// another field the header also suggests, because that is a genuine
			// ambiguity for the values to settle — but a field with no header
			// support whatsoever is not in that conversation.
			//
			// Two real files needed this. A column headed "كود الصنف" was bound
			// to `price` because its codes carried a trailing ".00" and so
			// profiled as money; the item code went unmapped and 453 rows whose
			// name cell was blank were rejected as having no identity. And a
			// column headed "معرف الصنف (ID)" was bound to `sku` because its
			// ids are short unique integers — the platform's own product id has
			// no value signature that distinguishes it from an item code, so
			// its header is the only witness there will ever be.
			//
			// When the named field is itself vetoed on evidence, the right
			// outcome is an unmapped column the user is asked about, not a
			// confident wrong binding.
			if named[col] && he.Score <= 0 {
				continue
			}
			pairs = append(pairs, pair{
				field:        f,
				column:       col,
				score:        score,
				why:          why,
				headerScore:  clamp(float64(he.Score) / 130),
				valueScore:   v.score,
				hasValue:     hasDetector,
				namedExactly: he.Score >= scoreExact,
			})
		}
	}

	// Deterministic order: best first, then leftmost column, then catalogue
	// order of the field. Two identically named columns therefore always
	// resolve the same way, run after run.
	fieldRank := map[Field]int{}
	for i, spec := range Specs {
		fieldRank[spec.Field] = i
	}
	sort.SliceStable(pairs, func(i, j int) bool {
		if pairs[i].score != pairs[j].score {
			return pairs[i].score > pairs[j].score
		}
		if pairs[i].column != pairs[j].column {
			return pairs[i].column < pairs[j].column
		}
		return fieldRank[pairs[i].field] < fieldRank[pairs[j].field]
	})
	return pairs, vetoes
}

// assign walks the scored readings best first, claiming each field and each
// column once.
func assign(m *Mapping, pairs []pair) {
	takenCol := make(map[int]bool, len(m.Columns))
	for _, p := range pairs {
		if p.score < minBindScore {
			break // the slice is sorted, so nothing after this qualifies either
		}
		if _, done := m.ByField[p.field]; done || takenCol[p.column] {
			continue
		}
		m.ByField[p.field] = p.column
		takenCol[p.column] = true

		c := m.Columns[p.column]
		c.Field = p.field
		c.Score = p.score
		c.Confidence = confidenceOf(p.score)
		c.Source = SourceAuto
		c.Why = p.why
	}
}

// attachCandidates records the runner-up readings for every column, so the
// review screen's dropdown is ordered by evidence rather than alphabetically.
func attachCandidates(m *Mapping, pairs []pair) {
	byColumn := map[int][]Candidate{}
	for _, p := range pairs {
		byColumn[p.column] = append(byColumn[p.column], Candidate{
			Field: p.field,
			Label: p.field.Label(),
			Score: p.score,
			Why:   p.why,
		})
	}
	for col, cands := range byColumn {
		sort.SliceStable(cands, func(i, j int) bool { return cands[i].Score > cands[j].Score })
		if len(cands) > 6 {
			cands = cands[:6]
		}
		m.Columns[col].Candidates = cands
	}
}

// previewOf takes a few real values from a column for the review screen.
func previewOf(p *sheet.ColumnProfile) []string {
	if p == nil {
		return nil
	}
	n := min(4, len(p.Sample))
	out := make([]string, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, p.Sample[i])
	}
	return out
}

// flagConflicts records everything the vendor should look at.
func flagConflicts(m *Mapping, pairs []pair, vetoes map[Field]map[int]string, headers []string) {
	m.Conflicts = append(m.Conflicts, headerValueConflicts(m, pairs, vetoes)...)
	m.Conflicts = append(m.Conflicts, ambiguityConflicts(m, pairs)...)
	m.Conflicts = append(m.Conflicts, duplicateHeaderConflicts(headers)...)
	sort.SliceStable(m.Conflicts, func(i, j int) bool { return m.Conflicts[i].Column < m.Conflicts[j].Column })
}

// headerValueConflicts reports the columns whose title and contents disagree.
//
// Both directions matter. A column headed "السعر" whose values are all between
// zero and a hundred on half steps is a discount list mislabelled, and binding
// it as the price would publish nine thousand products at thirty pounds. A
// column headed "القائمة" whose values say discount is the same disagreement
// seen from the other side, and is the reason the values are allowed to win.
func headerValueConflicts(m *Mapping, pairs []pair, vetoes map[Field]map[int]string) []Conflict {
	var out []Conflict
	byPair := map[Field]map[int]pair{}
	for _, p := range pairs {
		if byPair[p.field] == nil {
			byPair[p.field] = map[int]pair{}
		}
		byPair[p.field][p.column] = p
	}

	for _, c := range m.Columns {
		if c.Field == "" || c.Header == "" {
			continue
		}
		p, ok := byPair[c.Field][c.Index]
		if !ok || !p.hasValue {
			continue
		}
		if p.headerScore >= 0.45 && p.valueScore <= 0.25 {
			out = append(out, Conflict{
				Kind:     ConflictHeaderValue,
				Field:    c.Field,
				Column:   c.Index,
				Severity: SeverityWarning,
				Message: fmt.Sprintf(
					"عنوان العمود «%s» يشير إلى «%s»، لكن القيم بداخله لا تطابق ذلك. راجع هذا العمود قبل المتابعة.",
					c.Header, c.Field.Label()),
			})
		}
		if p.headerScore < 0.2 && p.valueScore >= 0.7 {
			out = append(out, Conflict{
				Kind:     ConflictHeaderValue,
				Field:    c.Field,
				Column:   c.Index,
				Severity: SeverityInfo,
				Message: fmt.Sprintf(
					"عنوان العمود «%s» غير واضح، وتم تحديده كـ«%s» بناءً على محتوى الصفوف.",
					c.Header, c.Field.Label()),
			})
		}
	}

	// A field whose every candidate column was vetoed is worth saying out loud
	// when its header was promising.
	for field, cols := range vetoes {
		for col, why := range cols {
			if why == "" || col >= len(m.Columns) {
				continue
			}
			hdr := m.Columns[col].Header
			if hdr == "" {
				continue
			}
			if ev := headerEvidence(hdr)[field]; ev.Score >= scoreStrong {
				out = append(out, Conflict{
					Kind:     ConflictHeaderValue,
					Field:    field,
					Column:   col,
					Severity: SeverityWarning,
					Message: fmt.Sprintf(
						"العمود «%s» عنوانه يدل على «%s» لكن %s؛ لم يتم ربطه تلقائياً.",
						hdr, field.Label(), why),
				})
			}
		}
	}
	return out
}

// ambiguityConflicts reports bindings whose runner-up was almost as strong.
func ambiguityConflicts(m *Mapping, pairs []pair) []Conflict {
	var out []Conflict
	for _, c := range m.Columns {
		if c.Field == "" || len(c.Candidates) < 2 {
			continue
		}
		best, second := c.Candidates[0], c.Candidates[1]
		if best.Field != c.Field || best.Score-second.Score > 0.08 {
			continue
		}
		// A runner-up that is already bound to a better-scoring column is not a
		// competing reading of this one. Reporting those made the review screen
		// warn that "سعر الجمهور" might be the item code on every well-mapped
		// file in the corpus, which is the fastest way to teach a vendor to
		// click past warnings without reading them.
		if other, bound := m.ByField[second.Field]; bound && other != c.Index {
			continue
		}
		out = append(out, Conflict{
			Kind:     ConflictAmbiguous,
			Field:    c.Field,
			Column:   c.Index,
			Severity: SeverityWarning,
			Message: fmt.Sprintf(
				"العمود «%s» يحتمل «%s» أو «%s» بنفس الدرجة تقريباً. اختر الصحيح يدوياً.",
				c.Header, best.Label, second.Label),
		})
	}
	return out
}

// duplicateHeaderConflicts reports two columns sharing a title, which is how a
// file that lists a price per branch confuses every importer that assumes
// headers are unique.
func duplicateHeaderConflicts(headers []string) []Conflict {
	seen := map[string]int{}
	var out []Conflict
	for i, h := range headers {
		key := sheet.NormalizeKey(h)
		if key == "" {
			continue
		}
		if first, dup := seen[key]; dup {
			out = append(out, Conflict{
				Kind:     ConflictDuplicateHeader,
				Column:   i,
				Severity: SeverityInfo,
				Message: fmt.Sprintf(
					"العمودان %d و %d يحملان نفس العنوان «%s»؛ تم التعامل معهما كعمودين مختلفين.",
					first+1, i+1, sheet.CleanCell(h)),
			})
			continue
		}
		seen[key] = i
	}
	return out
}
