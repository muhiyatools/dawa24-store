package catalog

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// Row interpretation for spreadsheet import.
//
// Every decision made here is recorded. A row that is dropped says why, a value
// that is guessed says it was guessed, and a price that could not be read is an
// error the admin sees rather than a zero written into the catalogue. The
// previous importer reported only three numbers — rows read, headers skipped,
// blanks skipped — and reported success even when it had discarded every price
// in the file.

// IssueSeverity classifies what an issue means for the row it belongs to.
type IssueSeverity string

const (
	// SeverityError means the row was not imported.
	SeverityError IssueSeverity = "error"
	// SeverityWarning means the row was imported with an assumption applied.
	SeverityWarning IssueSeverity = "warning"
)

// RowIssue is one problem found in one row, addressed the way the admin sees
// their file: by spreadsheet row number and column heading.
type RowIssue struct {
	Row      int           `json:"row"`
	Column   string        `json:"column,omitempty"`
	Value    string        `json:"value,omitempty"`
	Message  string        `json:"message"`
	Severity IssueSeverity `json:"severity"`
}

// ImportStats counts what the parse and the write did.
type ImportStats struct {
	TotalRowsRead  int
	ValidProducts  int
	RepeatedHeader int
	EmptyRows      int
	DuplicateRows  int
	RejectedRows   int
	Warnings       int
	Inserted       int
	Updated        int
	HeaderRow      int
	HeaderBlocks   int
	SheetName      string
	Format         string
	Delimiter      string
}

// ParseResult is everything learned from reading one uploaded file.
type ParseResult struct {
	Products []*Product
	Stats    ImportStats
	Issues   []RowIssue
	Plan     ColumnPlan
	// MissingFields lists important fields no column supplied, as Arabic labels.
	MissingFields []string
	// SheetsSkipped names other worksheets that held data and were not read.
	SheetsSkipped []string
	// Layout is the block structure the sheet was read as.
	Layout SheetLayout
	// SourceRows holds the spreadsheet row number each product came from,
	// parallel to Products. The staging table records it so a finding raised
	// after the write still points at a row in the admin's own file.
	SourceRows []int
}

// HasErrors reports whether any row was rejected.
func (r *ParseResult) HasErrors() bool { return r.Stats.RejectedRows > 0 }

// maxStorablePriceMinor is the largest value catalog.products' NUMERIC(12,2)
// price columns can hold, in piastres: 9,999,999,999.99.
const maxStorablePriceMinor int64 = 999999999999

// maxIssues caps what is retained. A file with 9,000 broken rows must not turn
// one bad upload into a gigabyte of report, and no admin reads past the first
// few hundred; the counters stay exact regardless.
const maxIssues = 500

// dosageFormKeywords maps words that appear in an Egyptian product name onto the
// pharmaceutical form. Longer, more specific entries come first: "غسول فم" must
// win over "غسول", and "معجون أسنان" over any bare match.
var dosageFormKeywords = []struct {
	keywords []string
	form     string
}{
	{[]string{"غسول فم", "مضمضة", "مضمضه", "mouthwash"}, "غسول فم"},
	{[]string{"معجون اسنان", "معجون أسنان", "toothpaste"}, "معجون أسنان"},
	{[]string{"غسول", "wash", "lotion", "لوشن"}, "غسول"},
	{[]string{"كريم", "cream"}, "كريم"},
	{[]string{"مرهم", "ointment", "oint"}, "مرهم"},
	{[]string{"جل", "جيل", "gel"}, "جل"},
	{[]string{"زيت", "oil", "اويل"}, "زيت"},
	{[]string{"سيروم", "serum"}, "سيروم"},
	{[]string{"شامبو", "shampoo"}, "شامبو"},
	{[]string{"صابون", "صابونة", "صابونه", "soap"}, "صابون"},
	{[]string{"رول اون", "roll on", "رول-اون", "رول_اون"}, "رول اون"},
	{[]string{"اسبراي", "سبراي", "اسبراى", "سبراى", "بخاخ", "spray", "بدى ميست", "body mist"}, "بخاخ / اسبراي"},
	{[]string{"مناديل", "wipes"}, "مناديل مبللة"},
	{[]string{"صبغة", "صبغه", "hair color", "color", "colour"}, "صبغة شعر"},
	{[]string{"اقراص", "أقراص", "قرص", "tab", "tabs", "tablet", "tablets"}, "أقراص"},
	{[]string{"كبسول", "كبسولات", "cap", "caps", "capsule", "capsules"}, "كبسولات"},
	{[]string{"شراب", "syrup", "susp", "معلق"}, "شراب"},
	{[]string{"نقط", "drops", "قطرة", "قطره"}, "نقط"},
	{[]string{"حقن", "حقنة", "حقنه", "امبول", "أمبول", "امبولات", "أمبولات", "فيال", "vial", "ampoule", "inj"}, "حقن وأمبولات"},
	{[]string{"فوار", "ساشيت", "sachet", "eff"}, "أكياس فوار"},
	{[]string{"لبوس", "تحاميل", "تحميلة", "supp", "suppository"}, "لبوس"},
	{[]string{"حفاضات", "حفاضه", "diapers"}, "مستلزمات عناية"},
	{[]string{"استيك", "ستيك", "stick"}, "ستيك مضاد للتعرق"},
}

// defaultDosageForm labels a product whose name gives no clue about its form.
const defaultDosageForm = "مستحضر صيدلاني"

var (
	concentrationPattern = regexp.MustCompile(`(?i)(\d+(?:\.\d+)?\s*(?:ملجرام|مليجرام|مجم|جم|جرام|مل|ملي|ملم|mg|g|gm|ml|mcg|iu|%|spf[+\d]*|[+\d]*spf))`)
	digitsOnlyPattern    = regexp.MustCompile(`^[\d\s./\-]+$`)
)

// ExtractDosageAndConcentration infers the pharmaceutical form and strength from
// a product name, for the very common supplier file that carries neither as its
// own column.
func ExtractDosageAndConcentration(name string) (dosage string, conc string) {
	lowered := strings.ToLower(name)
	for _, dk := range dosageFormKeywords {
		for _, kw := range dk.keywords {
			if strings.Contains(lowered, kw) {
				dosage = dk.form
				break
			}
		}
		if dosage != "" {
			break
		}
	}
	if dosage == "" {
		dosage = defaultDosageForm
	}

	if match := concentrationPattern.FindString(NormalizeDigits(name)); match != "" {
		conc = CleanCellString(match)
	}
	return dosage, conc
}

// DetectHeaderRow scans the head of the file for the row that names the columns.
//
// Scanning only the first rows is deliberate: a label row appearing at row 400
// is a repeated print header, which is handled separately.
func DetectHeaderRow(records [][]string) int {
	bestIdx, bestScore := -1, 0

	for i := 0; i < min(40, len(records)); i++ {
		// Later rows that score equally are usually the first data row echoing
		// the header's vocabulary, so earlier candidates win ties.
		score := scoreHeaderCandidate(records[i]) - i
		if score > bestScore {
			bestScore, bestIdx = score, i
		}
	}
	return bestIdx
}

// scoreHeaderCandidate rates how much one row looks like a set of column titles.
//
// It rewards cells that identify a known field, and penalises the two things
// that mark a row as data rather than titles: mostly numeric cells, and repeated
// values. A header row names things; a data row states them.
func scoreHeaderCandidate(row []string) int {
	if len(row) == 0 {
		return 0
	}
	plan := PlanColumns(row)
	if len(plan.Bindings) == 0 {
		return 0
	}

	filled, numeric, duplicates := 0, 0, 0
	seen := map[string]bool{}
	for _, cell := range row {
		clean := CleanCellString(cell)
		if clean == "" {
			continue
		}
		filled++
		if digitsOnlyPattern.MatchString(NormalizeDigits(clean)) {
			numeric++
		}
		key := NormalizeKey(clean)
		if key != "" && seen[key] {
			duplicates++
		}
		seen[key] = true
	}
	if filled == 0 {
		return 0
	}

	score := 0
	for _, b := range plan.Bindings {
		score += b.Score
	}
	return score - numeric*40 - duplicates*30
}

// ParseProducts converts a decoded sheet into products, with a full account of
// what was skipped, assumed, and rejected.
func ParseProducts(data *SheetData) *ParseResult {
	return ParseProductsWithOverrides(data, LayoutOverrides{})
}

// ParseProductsWithOverrides reads a sheet after applying the admin's
// corrections to the detected structure.
func ParseProductsWithOverrides(data *SheetData, overrides LayoutOverrides) *ParseResult {
	res := &ParseResult{Stats: ImportStats{HeaderRow: -1}}
	if data == nil || len(data.Rows) == 0 {
		return res
	}

	res.Stats.TotalRowsRead = len(data.Rows)
	res.Stats.SheetName = data.Sheet
	res.Stats.Format = data.Format
	res.Stats.Delimiter = data.Delimiter
	res.SheetsSkipped = data.SheetsSkipped

	res.Layout = AnalyzeLayout(data).Apply(data, overrides)
	res.Plan = res.Layout.Primary
	if len(res.Layout.HeaderRows) > 0 {
		res.Stats.HeaderRow = res.Layout.HeaderRows[0]
	}
	res.Stats.HeaderBlocks = len(res.Layout.Blocks)
	// Every header after the first is a reprint from a paginated export. They
	// are block boundaries rather than rows inside a block, so nothing skips
	// them row by row — but they must still be counted, or the admin's row
	// arithmetic silently fails to add up to the file's length.
	if n := len(res.Layout.HeaderRows); n > 1 {
		res.Stats.RepeatedHeader = n - 1
	}
	res.reportStructure()

	res.Products = res.collectProducts(data.Rows)
	res.Stats.ValidProducts = len(res.Products)
	sort.SliceStable(res.Issues, func(i, j int) bool { return res.Issues[i].Row < res.Issues[j].Row })
	return res
}

// reportStructure tells the admin what shape the file was read as, before any
// row is interpreted. A wrong reading here mislabels the entire catalogue, so it
// is surfaced rather than left implicit.
func (r *ParseResult) reportStructure() {
	if r.Layout.Positional {
		r.addIssue(RowIssue{
			Row:      1,
			Severity: SeverityWarning,
			Message: "تعذر التعرف على صف العناوين في الملف، وتمت قراءة الأعمدة بالترتيب " +
				"(كود الصنف، اسم الصنف، الشركة المصنعة). يمكنك تصحيح ذلك من خطوة مراجعة الأعمدة.",
		})
	}
	if n := len(r.Layout.HeaderRows); n > 1 {
		r.addIssue(RowIssue{
			Row:      1,
			Severity: SeverityWarning,
			Message: fmt.Sprintf(
				"الملف مقسّم إلى %d كتلة بيانات يتكرر فيها صف العناوين (تصدير مُقسّم للطباعة). "+
					"تمت قراءة جميع الكتل، وتم تجاهل صفوف العناوين المتكررة.", n),
		})
	}
	if r.Layout.VariantBlocks > 0 {
		r.addIssue(RowIssue{
			Row:      1,
			Severity: SeverityWarning,
			Message: fmt.Sprintf(
				"يحتوي الملف على %d قسم بترتيب أعمدة مختلف عن القسم الأول؛ تمت قراءة كل قسم وفق عناوينه الخاصة.",
				r.Layout.VariantBlocks),
		})
	}

	r.MissingFields = missingImportantFields(r.Plan)
	for _, label := range r.MissingFields {
		r.addIssue(RowIssue{
			Row:      1,
			Column:   label,
			Severity: SeverityWarning,
			Message: fmt.Sprintf(
				"لم يتم العثور على عمود «%s» في الملف. سيتم استيراد الأصناف بدون هذه البيانات.", label),
		})
	}
}

// collectProducts walks every block, skipping the noise and folding in-file
// duplicates together.
//
// Reading block by block is what lets a sheet whose second section adds a price
// column be read correctly: each block is interpreted through its own header
// rather than through the first block's mapping.
func (r *ParseResult) collectProducts(records [][]string) []*Product {
	// seen keys a product by the strongest identifier its row carried, so two
	// rows sharing a SKU merge even when their names differ by a typo.
	seen := make(map[string]*Product, r.Layout.DataRows)
	var products []*Product

	for _, block := range r.Layout.Blocks {
		headerKeys := map[string]bool{}
		if block.HeaderRow >= 0 && block.HeaderRow < len(records) {
			headerKeys = normalizedKeySet(records[block.HeaderRow])
		}

		for rIdx := block.FirstRow; rIdx <= block.LastRow && rIdx < len(records); rIdx++ {
			row := records[rIdx]

			if isBlankRow(row) {
				r.Stats.EmptyRows++
				continue
			}
			if isRepeatedHeader(row, headerKeys) {
				r.Stats.RepeatedHeader++
				continue
			}

			// Spreadsheet row numbers are 1-based and this is what the admin
			// sees in Excel's row gutter, so every issue is reported against it.
			cursor := rowCursor{result: r, plan: block.Plan, row: row, number: rIdx + 1, block: block.Index}
			prod, ok := cursor.parse()
			if !ok {
				r.Stats.RejectedRows++
				continue
			}

			key := dedupeKey(prod)
			if existing, isDuplicate := seen[key]; isDuplicate {
				mergeProduct(existing, prod)
				r.Stats.DuplicateRows++
				cursor.warn("", prod.Name.Get(i18n.AR),
					"صنف مكرر داخل الملف نفسه؛ تم دمج بياناته مع الصف السابق بدلاً من تكراره في الكتالوج.")
				continue
			}

			seen[key] = prod
			products = append(products, prod)
			r.SourceRows = append(r.SourceRows, rIdx+1)
		}
	}

	crossFillIdentifiers(products)
	return products
}

// rowCursor addresses one spreadsheet row through the resolved column plan.
//
// Bundling the row with its number and the plan is what keeps the readers below
// down to one or two parameters: without it every one of them has to be handed a
// value accessor, a label accessor and a row number, and the row itself rides
// along unused because the accessors already close over it.
type rowCursor struct {
	result *ParseResult
	// plan is the owning block's column mapping, which is not necessarily the
	// primary one: a sheet may stack sections of different shapes.
	plan   ColumnPlan
	row    []string
	number int
	block  int
}

// value returns a field's cell for this row, or empty when the field is unmapped
// or the row is short.
func (c rowCursor) value(field string) string {
	idx, ok := c.plan.Columns[field]
	if !ok || idx < 0 || idx >= len(c.row) {
		return ""
	}
	return CleanCellString(c.row[idx])
}

// label names a field the way the admin's own file names it, falling back to the
// canonical Arabic label when the file supplied no header for it.
func (c rowCursor) label(field string) string {
	for _, b := range c.plan.Bindings {
		if b.Field == field {
			return b.Header
		}
	}
	return FieldLabels[field]
}

func (c rowCursor) warn(column, value, message string) {
	c.result.addIssue(RowIssue{
		Row: c.number, Column: column, Value: value,
		Message: message, Severity: SeverityWarning,
	})
}

func (c rowCursor) reject(column, value, message string) {
	c.result.addIssue(RowIssue{
		Row: c.number, Column: column, Value: value,
		Message: message, Severity: SeverityError,
	})
}

// parse turns this row into a product, or rejects it with reasons.
func (c rowCursor) parse() (*Product, bool) {
	nameAR, nameEN, ok := c.resolveNames()
	if !ok {
		return nil, false
	}

	// Status is left empty unless the file states one. The write path defaults
	// an empty status to active on insert and leaves it untouched on update, so
	// re-importing a supplier's price list cannot silently reactivate a product
	// an admin deliberately took off the catalogue.
	prod := &Product{
		SKU:                    c.value(FieldSKU),
		Barcode:                c.value(FieldBarcode),
		ScientificName:         c.value(FieldGenericName),
		Active:                 c.value(FieldActive),
		DosageForm:             c.value(FieldDosageForm),
		Concentration:          NormalizeDigits(c.value(FieldConcentration)),
		Unit:                   c.value(FieldUnit),
		ManufacturingCompanies: c.value(FieldManufacturer),
		InstitutionalWorkIDs:   []int64{},

		// The name columns stay in their own language. Copying Arabic into the
		// English slot — which the previous importer did unconditionally — makes
		// every English search match Arabic text and makes the catalogue look
		// translated when it is not.
		Name:        i18n.New(nameAR, nameEN),
		Description: i18n.New(c.value(FieldDescriptionAR), c.value(FieldDescriptionEN)),
	}

	// Fill the form and strength from the name only where the file gave none.
	autoDosage, autoConc := ExtractDosageAndConcentration(nameAR + " " + nameEN)
	if prod.DosageForm == "" {
		prod.DosageForm = autoDosage
	}
	if prod.Concentration == "" {
		prod.Concentration = autoConc
	}

	if !c.readPrices(prod) {
		return nil, false
	}
	c.readStatus(prod)
	return prod, true
}

// resolveNames determines the product's Arabic and English names, recovering
// from a mis-mapped or missing name column where it can.
func (c rowCursor) resolveNames() (nameAR, nameEN string, ok bool) {
	nameAR = c.value(FieldNameAR)
	nameEN = c.value(FieldNameEN)
	if nameAR != "" || nameEN != "" {
		return nameAR, nameEN, true
	}

	// A file whose name column is mapped wrongly, or missing entirely, still
	// usually carries the name somewhere.
	if guess := guessNameCell(c.row, c.plan); guess != "" {
		c.warn("", guess, "لم يتم العثور على اسم الصنف في العمود المتوقع؛ تم استنتاجه من محتوى الصف.")
		return guess, "", true
	}

	identifier := c.value(FieldSKU)
	if identifier == "" {
		identifier = c.value(FieldBarcode)
	}
	if identifier == "" {
		c.reject("", "", "تم تجاهل الصف: لا يحتوي على اسم صنف ولا كود ولا باركود.")
		return "", "", false
	}

	// An identifier with no name is a real row with a missing label; keep it
	// rather than dropping stock the pharmacy owns, and flag it for review.
	c.warn("", identifier,
		"الصف لا يحتوي على اسم صنف؛ تم استيراده باسم مؤقت مبني على الكود ويحتاج إلى مراجعة.")
	return "صنف دوائي #" + identifier, "", true
}

// readPrices fills the three price fields, rejecting the row only when a price
// is present but unusable. A missing price is normal — plenty of master
// catalogue rows are priced per supplier later — and must not lose the product.
func (c rowCursor) readPrices(prod *Product) bool {
	price, ok := c.readAmount(FieldPrice, true)
	if !ok {
		return false
	}
	public, ok := c.readAmount(FieldPublicPrice, false)
	if !ok {
		return false
	}

	prod.Price = price
	prod.OldPrice = public
	// A file may carry only the public price. That is the product's price until
	// a supplier quotes their own, so use it rather than storing a zero.
	if prod.Price.IsZero() && prod.OldPrice.IsPositive() {
		prod.Price = prod.OldPrice
	}

	c.readDiscount(prod)
	return true
}

// readAmount reads one money column. A required column that holds an unreadable
// value rejects the row; an optional one only warns.
func (c rowCursor) readAmount(field string, required bool) (money.Amount, bool) {
	raw := c.value(field)
	if raw == "" {
		return money.Zero, true
	}

	amt, info, err := CoerceMoney(raw)
	switch {
	case err == ErrNoValue:
		return money.Zero, true

	case err != nil:
		if required {
			c.reject(c.label(field), raw,
				fmt.Sprintf("تم رفض الصف: قيمة السعر «%s» ليست رقماً صالحاً.", raw))
			return money.Zero, false
		}
		c.warn(c.label(field), raw, fmt.Sprintf("تعذر قراءة القيمة «%s» كرقم؛ تم تجاهلها.", raw))
		return money.Zero, true

	case amt.IsNegative():
		c.reject(c.label(field), raw, "تم رفض الصف: السعر لا يمكن أن يكون قيمة سالبة.")
		return money.Zero, false

	case amt.Minor() > maxStorablePriceMinor:
		// Caught here rather than at the write, because NUMERIC(12,2) would
		// refuse it and abort the whole transaction for one bad cell — losing an
		// otherwise clean import of nine thousand rows.
		c.reject(c.label(field), raw,
			"تم رفض الصف: قيمة السعر تتجاوز الحد الأقصى المسموح به (9,999,999,999.99).")
		return money.Zero, false
	}

	if info.Rounded {
		c.warn(c.label(field), raw,
			fmt.Sprintf("تم تقريب القيمة إلى منزلتين عشريتين (%s).", amt.String()))
	}
	return amt, true
}

// readDiscount interprets the discount column, which arrives either as a
// percentage ("20%") or as an amount ("9.20"). Both are common in the same
// supplier's files, so the unit is decided per cell and not per column.
func (c rowCursor) readDiscount(prod *Product) {
	raw := c.value(FieldDiscount)
	if raw == "" {
		return
	}

	amt, info, err := CoerceMoney(raw)
	switch {
	case err == ErrNoValue:
		return
	case err != nil:
		c.warn(c.label(FieldDiscount), raw, "تعذر قراءة قيمة الخصم؛ تم تجاهلها.")
		return
	case amt.IsNegative():
		c.warn(c.label(FieldDiscount), raw, "قيمة الخصم سالبة؛ تم تجاهلها.")
		return
	}

	if info.Percent {
		if prod.Price.IsPositive() {
			// Percent of price, in minor units, truncated to the piastre. Both
			// operands are already scaled by 100, hence the 100*100 divisor.
			prod.Discount = money.FromMinor(prod.Price.Minor() * amt.Minor() / (100 * 100))
		}
		return
	}

	if prod.Price.IsPositive() && amt.Minor() >= prod.Price.Minor() {
		c.warn(c.label(FieldDiscount), raw, "قيمة الخصم تساوي السعر أو تزيد عليه؛ تم تجاهلها.")
		return
	}
	prod.Discount = amt
}

// readStatus applies the status column when the file supplies a value the
// products table's CHECK constraint accepts.
func (c rowCursor) readStatus(prod *Product) {
	raw := c.value(FieldStatus)
	if raw == "" {
		return
	}
	status, ok := CoerceStatus(raw)
	if !ok {
		c.warn(c.label(FieldStatus), raw,
			"حالة الصنف غير معروفة؛ تم تجاهل العمود والإبقاء على الحالة الافتراضية للصنف.")
		return
	}
	prod.Status = status
}

// guessNameCell finds the cell most likely to be a product name in a row whose
// name column is missing or mis-mapped.
func guessNameCell(row []string, plan ColumnPlan) string {
	identifierCols := map[int]bool{}
	for _, field := range []string{FieldSKU, FieldBarcode, FieldPrice, FieldPublicPrice,
		FieldCostPrice, FieldDiscount, FieldQuantity, FieldStatus, FieldUnit} {
		if idx, ok := plan.Columns[field]; ok {
			identifierCols[idx] = true
		}
	}

	best := ""
	for idx, cell := range row {
		if identifierCols[idx] {
			continue
		}
		clean := CleanCellString(cell)
		// Six characters excludes codes, dates and units without excluding a
		// short real name such as "بنادول".
		if len([]rune(clean)) < 6 || digitsOnlyPattern.MatchString(NormalizeDigits(clean)) {
			continue
		}
		if len([]rune(clean)) > len([]rune(best)) {
			best = clean
		}
	}
	return best
}

// dedupeKey picks the identity a row carries. SKU is the primary unique identifier;
// a normalised name with manufacturer is the fallback. Barcode is intentionally not
// used as a uniqueness key so multiple products or packages sharing a barcode are
// preserved and not wrongly collapsed.
func dedupeKey(p *Product) string {
	if sku := strings.ToLower(strings.TrimSpace(p.SKU)); sku != "" {
		return "sku:" + sku
	}
	name := NormalizeName(p.Name.Get(i18n.AR))
	if name == "" {
		name = NormalizeName(p.Name.Get(i18n.EN))
	}
	return "name:" + name + "|" + NormalizeKey(p.ManufacturingCompanies)
}

// mergeProduct folds a later duplicate row into the one already kept, filling
// gaps without overwriting values that were already supplied. A supplier who
// lists a product twice — once with a price, once with a barcode — ends up with
// one complete row instead of two half-empty ones.
func mergeProduct(dst, src *Product) {
	fillString(&dst.SKU, src.SKU)
	fillString(&dst.Barcode, src.Barcode)
	fillString(&dst.ScientificName, src.ScientificName)
	fillString(&dst.Active, src.Active)
	fillString(&dst.Unit, src.Unit)
	fillString(&dst.ManufacturingCompanies, src.ManufacturingCompanies)
	fillString(&dst.Concentration, src.Concentration)

	// An inferred form is weaker evidence than a stated one, so a later row that
	// names the form outright replaces the placeholder.
	if dst.DosageForm == "" || dst.DosageForm == defaultDosageForm {
		if src.DosageForm != "" && src.DosageForm != defaultDosageForm {
			dst.DosageForm = src.DosageForm
		}
	}
	if dst.Price.IsZero() {
		dst.Price = src.Price
	}
	if dst.OldPrice.IsZero() {
		dst.OldPrice = src.OldPrice
	}
	if dst.Discount.IsZero() {
		dst.Discount = src.Discount
	}
	if dst.Status == "" {
		dst.Status = src.Status
	}
	if dst.Name.Get(i18n.EN) == "" && src.Name.Get(i18n.EN) != "" {
		dst.Name[i18n.EN] = src.Name.Get(i18n.EN)
	}
	if dst.Description.IsEmpty() && !src.Description.IsEmpty() {
		dst.Description = src.Description
	}
}

func fillString(dst *string, src string) {
	if *dst == "" {
		*dst = src
	}
}

// crossFillIdentifiers fills an empty barcode from SKU when available, without forcing
// barcode into SKU.
func crossFillIdentifiers(prods []*Product) {
	for _, p := range prods {
		if p.Barcode == "" && p.SKU != "" {
			p.Barcode = p.SKU
		}
	}
}

// isBlankRow reports whether every cell is empty.
func isBlankRow(row []string) bool {
	for _, cell := range row {
		if CleanCellString(cell) != "" {
			return false
		}
	}
	return true
}

// normalizedKeySet folds a header row into a set for repeated-header matching.
func normalizedKeySet(header []string) map[string]bool {
	set := make(map[string]bool, len(header))
	for _, cell := range header {
		if key := NormalizeKey(cell); key != "" {
			set[key] = true
		}
	}
	return set
}

// isRepeatedHeader reports whether a data row is really the header printed
// again.
//
// Distributor systems paginate their exports and reprint the column titles every
// page — one real file carried 114 of them. Matching against the detected header
// generalises past the previous hardcoded list of nine literal strings, which
// silently imported every reprinted header of every file whose titles were not
// on that list as if it were a product.
func isRepeatedHeader(row []string, headerKeys map[string]bool) bool {
	if len(headerKeys) == 0 {
		return false
	}
	filled, matched := 0, 0
	for _, cell := range row {
		key := NormalizeKey(cell)
		if key == "" {
			continue
		}
		filled++
		if headerKeys[key] {
			matched++
		}
	}
	// Two thirds agreement, and at least two cells, so a product legitimately
	// named after a column ("سعر") cannot trip it on its own.
	return filled >= 2 && matched*3 >= filled*2
}

// missingImportantFields lists the fields worth telling the admin were absent.
// Their absence is not an error — plenty of valid files carry only names and
// prices — but a silently missing price column is how a catalogue ends up
// entirely free of charge.
func missingImportantFields(plan ColumnPlan) []string {
	var missing []string
	// Price is satisfied by any of the price columns, because row parsing falls
	// back from the selling price to the public price. Reporting "السعر مفقود"
	// for a file that plainly carries "سعر البيع للجمهور" trains admins to
	// ignore the warnings, which is worse than not showing them.
	if !plan.Has(FieldPrice) && !plan.Has(FieldPublicPrice) {
		missing = append(missing, FieldLabels[FieldPrice])
	}
	for _, field := range []string{FieldNameAR, FieldSKU, FieldManufacturer} {
		if !plan.Has(field) {
			missing = append(missing, FieldLabels[field])
		}
	}
	return missing
}

// addIssue records an issue, keeping the counters exact even once the retained
// list is full.
func (r *ParseResult) addIssue(issue RowIssue) {
	if issue.Severity == SeverityWarning {
		r.Stats.Warnings++
	}
	if len(r.Issues) < maxIssues {
		r.Issues = append(r.Issues, issue)
	}
}

// ParseProductRows converts raw spreadsheet rows into cleaned, valid products.
//
// Retained as the narrow entry point for callers that need only the products;
// ParseProducts carries the per-row issues the import report renders.
func ParseProductRows(records [][]string) ([]*Product, ImportStats) {
	data := &SheetData{Rows: records}
	normalizeWidth(data)
	res := ParseProducts(data)
	return res.Products, res.Stats
}
