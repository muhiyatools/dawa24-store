package ui

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/shared/arabic"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/shared/productmatch"
)

// Matching a saving-products file against the shared catalogue.
//
// This was a bespoke engine: it held every catalogue product in a slice and, for
// each row the exact tiers missed, walked all 19,996 of them computing a
// Levenshtein-and-token blend. It accepted anything scoring 0.75 — with no
// strength check and no dosage-form check, so 500 mg could match 1 g, which is
// the single mistake the shared scorer was written to make impossible.
//
// It ran on a pharmacy's private list, so a wrong link is recoverable. It is
// also the list the smart order's first resolution tier reads: 8,777 entries
// with a catalogue link is what makes a recurring order need no manual work, and
// 756 of 1,596 is what the platform actually had.
//
// It now runs the same index the vendor import and the smart order run, with the
// same vetoes and the same shortlist, and the file's own product id and barcode
// settle a row before any of that is reached.

// MatchStrategy determines how rows are linked to the master catalogue.
type MatchStrategy string

const (
	// StrategySmartAuto runs every tier: id, code, then the scored index.
	StrategySmartAuto MatchStrategy = "smart_auto"
	// StrategySKUOnly stops after the identifier tiers.
	StrategySKUOnly MatchStrategy = "sku_only"
	// StrategyNameOnly skips the identifiers and scores the name.
	StrategyNameOnly MatchStrategy = "name_only"
	// StrategyIDOnly accepts only a dawa24 product id stated in the file.
	StrategyIDOnly MatchStrategy = "id_only"
)

// A link is made when the engine calls the match settled, not when a score
// clears a number this file invented.
//
// The previous engine compared a Levenshtein-and-token blend against 0.75. That
// figure is not on the same scale as the shared scorer's and carrying it across
// would have meant nothing: the same real row — a pharmacy's
// "فيم فريش غسول انتميت سكير 250 مل" against the catalogue's
// "فيم فريش غسول يومي للمناطق الحساسة 250 مل" — scores 0.55 here, is ranked
// first, and has both its concentration and its dosage form corroborated. It is
// the same product, and a borrowed threshold would refuse it.
//
// MatchLevel.Settled is that judgement, and it is the same judgement the vendor
// import applies. Ambiguous and review both come back unlinked, which is what
// the review screen exists to resolve.

// MatchResult captures the resolution of a single saving product row.
type MatchResult struct {
	ProductID  *int64  `json:"product_id,omitempty"`
	MatchType  string  `json:"match_type"`
	Confidence float64 `json:"confidence"`
}

// SavingProductMatchEngine links rows to catalogue products.
type SavingProductMatchEngine struct {
	index *productmatch.Index
	// known is the set of product ids, so a product id stated in the file can
	// be validated without scoring anything.
	known map[int64]bool
	opts  productmatch.MatchOptions
}

// NewSavingProductMatchEngine builds the index once per import.
func NewSavingProductMatchEngine(products []catalog.MatchProduct) *SavingProductMatchEngine {
	masters := make([]productmatch.MasterProduct, 0, len(products))
	known := make(map[int64]bool, len(products))
	for _, p := range products {
		if p.ID <= 0 {
			continue
		}
		known[p.ID] = true
		masters = append(masters, productmatch.MasterProduct{
			ID: p.ID, NameAR: p.NameAR, NameEN: p.NameEN, SKU: p.SKU,
			Barcode: p.Barcode, Scientific: p.Scientific, DosageForm: p.DosageForm,
			Concentration: p.Concentration, Unit: p.Unit,
			Manufacturer: p.Manufacturer, PublicPrice: p.PublicPrice,
		})
	}

	opts := productmatch.DefaultMatchOptions()
	// The file's own code column is trusted here, unlike in the vendor import.
	// A pharmacy keeping a private list of what it buys is recording the code
	// it looks the product up by, and that code is very often the catalogue's
	// own — which is the whole reason the column is offered.
	opts.TrustSupplierCode = true
	// The user mapped a code column on the mapping screen; that is them saying
	// the column holds the code they look products up by. Demanding the name
	// agree as well refuses exactly the rows the column was mapped for.
	opts.CodeIsAuthoritative = true

	return &SavingProductMatchEngine{
		index: productmatch.NewIndex(masters),
		known: known,
		opts:  opts,
	}
}

// Size reports how many catalogue products are indexed.
func (e *SavingProductMatchEngine) Size() int {
	if e == nil || e.index == nil {
		return 0
	}
	return e.index.Size()
}

// Describe returns a matched product's catalogue name and code.
//
// The index already holds every name it scored against, so the review screen can
// say which product a row resolved to without a second query. A table that
// prints only the id has told the reader nothing they can check.
func (e *SavingProductMatchEngine) Describe(id int64) (name, sku string) {
	if e == nil || e.index == nil {
		return "", ""
	}
	p, ok := e.index.Lookup(id)
	if !ok {
		return "", ""
	}
	name = p.NameAR
	if name == "" {
		name = p.NameEN
	}
	return name, p.SKU
}

// Match resolves one row under the chosen strategy.
func (e *SavingProductMatchEngine) Match(strategy MatchStrategy, productID *int64, rawSKU, rawName string) MatchResult {
	unlinked := MatchResult{MatchType: "unlinked"}
	if e == nil || e.index == nil || e.index.Size() == 0 {
		return unlinked
	}
	if strategy == "" {
		strategy = StrategySmartAuto
	}

	// A product id the file states outright, checked against the catalogue
	// rather than trusted. A stale export names ids that no longer exist.
	if productID != nil && *productID > 0 && e.known[*productID] {
		id := *productID
		return MatchResult{ProductID: &id, MatchType: "direct_id", Confidence: 1.0}
	}
	if strategy == StrategyIDOnly {
		return unlinked
	}

	row := &productmatch.Row{Name: strings.TrimSpace(rawName)}
	opts := e.opts
	switch strategy {
	case StrategySKUOnly:
		row.SKU = strings.TrimSpace(rawSKU)
		row.Barcode = strings.TrimSpace(rawSKU)
		// The identifier tiers only. Scored matching is suppressed by putting
		// the thresholds out of reach rather than by a second code path, so the
		// strategies cannot drift apart.
		opts.MinStrong, opts.MinReview = 1.01, 1.01
	case StrategyNameOnly:
		opts.TrustSupplierCode = false
	default:
		row.SKU = strings.TrimSpace(rawSKU)
		row.Barcode = strings.TrimSpace(rawSKU)
	}
	if row.Name == "" && row.SKU == "" {
		return unlinked
	}

	res := e.index.Match(row, opts)
	if !res.Matched() || !res.Level.Settled() {
		// Ambiguity is reported as unlinked rather than resolved arbitrarily.
		// Two products that fit equally well is not a match with a lower score;
		// it is a question, and this list has a screen for asking it.
		return unlinked
	}
	return MatchResult{
		ProductID:  &res.ProductID,
		MatchType:  savingMatchType(res.Level),
		Confidence: res.Score,
	}
}

// savingMatchType renders the engine's level in the vocabulary this screen and
// its stored sessions already use.
func savingMatchType(level productmatch.MatchLevel) string {
	switch level {
	case productmatch.MatchBarcode:
		return "barcode"
	case productmatch.MatchCode:
		return "exact_sku"
	case productmatch.MatchExact:
		return "exact_name"
	default:
		return "fuzzy_name"
	}
}

var scientificNotationRegex = regexp.MustCompile(`(?i)^[0-9]+(\.[0-9]+)?e\+[0-9]+$`)

// NormalizeDigitsOnly converts Arabic-Indic digits ٠-٩ to 0-9 without altering
// periods, commas, or letters.
func NormalizeDigitsOnly(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r >= '٠' && r <= '٩':
			b.WriteRune('0' + (r - '٠'))
		case r >= '۰' && r <= '۹':
			b.WriteRune('0' + (r - '۰'))
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// SanitizeSKUCode cleans the artefacts a spreadsheet adds to a code: Arabic
// digits, scientific notation, a trailing ".0", and separator noise.
func SanitizeSKUCode(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	s = NormalizeDigitsOnly(s)

	// Excel renders a long numeric code as 6.22123E+12 once it decides the cell
	// is a number. Reading that as text stores the rendering, not the code.
	if scientificNotationRegex.MatchString(s) {
		if f, err := strconv.ParseFloat(s, 64); err == nil && !math.IsNaN(f) && !math.IsInf(f, 0) {
			s = fmt.Sprintf("%.0f", f)
		}
	}
	s = strings.TrimSuffix(s, ".0")

	s = strings.ToLower(s)
	for _, noise := range []string{"-", " ", "_", "/"} {
		s = strings.ReplaceAll(s, noise, "")
	}
	if isNumericOnly(s) {
		if trimmed := strings.TrimLeft(s, "0"); trimmed != "" {
			return trimmed
		}
	}
	return s
}

func isNumericOnly(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return len(s) > 0
}

// ParseFlexibleQuantity parses the quantity formats a spreadsheet produces.
func ParseFlexibleQuantity(raw string) (float64, bool) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return 0, false
	}
	s = NormalizeDigitsOnly(s)
	s = strings.ReplaceAll(s, ",", "")
	s = strings.ReplaceAll(s, " ", "")

	f, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) || f < 0 {
		return 0, false
	}
	return f, true
}

// ParseFlexibleMoney parses a price carrying a currency word or thousands
// separators.
func ParseFlexibleMoney(raw string) (money.Amount, bool) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return money.Amount{}, false
	}
	s = NormalizeDigitsOnly(s)
	s = strings.ReplaceAll(s, ",", "")
	for _, unit := range []string{"ج.م", "جم", "جنيه", "egp", "EGP", "le", "LE", "$"} {
		s = strings.ReplaceAll(s, unit, "")
	}
	s = strings.TrimSpace(s)

	amt, err := money.Parse(s)
	if err != nil {
		f, ferr := strconv.ParseFloat(s, 64)
		if ferr != nil || math.IsNaN(f) || math.IsInf(f, 0) || f < 0 {
			return money.Amount{}, false
		}
		return money.FromMinor(int64(math.Round(f * 100))), true
	}
	if amt.IsNegative() {
		return money.Amount{}, false
	}
	return amt, true
}

// summaryWords are the labels a spreadsheet's footer uses.
var summaryWords = []string{
	"اجمالي", "الاجمالي", "مجموع", "المجموع", "المجموع الكلي", "العدد",
	"total", "grand total", "subtotal", "sum", "summary", "count",
}

// IsSummaryOrTotalRow reports whether a row is a footer rather than a product.
func IsSummaryOrTotalRow(row []string) bool {
	if len(row) == 0 {
		return true
	}
	for _, cell := range row {
		clean := strings.TrimSpace(strings.ToLower(cell))
		if clean == "" {
			continue
		}
		norm := arabic.Normalize(clean)
		for _, w := range summaryWords {
			if norm == w || strings.HasPrefix(norm, w+" ") || strings.HasPrefix(norm, w+":") {
				return true
			}
		}
		// Only the first non-empty cell decides: a footer labels itself in its
		// leading cell, and a product named "Total Care Shampoo" must survive.
		break
	}
	return false
}

// IsAllEmptyRow reports whether every cell is blank.
func IsAllEmptyRow(row []string) bool {
	for _, cell := range row {
		if strings.TrimSpace(cell) != "" {
			return false
		}
	}
	return true
}
