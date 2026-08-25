package smartorder

import (
	"fmt"
	"strconv"
	"strings"
)

// Quantity parsing.
//
// A pharmacy's spreadsheet does not contain numbers. It contains "2", " 3 ",
// "٥", "5 علبة", "2-3", "12.0", "-1", and empty cells that mean "I did not
// decide yet". Every one of those has to become either a quantity or an honest
// admission that it could not be read.
//
// The rule throughout: **never guess silently.** A cell reading "2-3" could
// plausibly become 2, or 3, or 2.5, and any of those choices sends stock to a
// pharmacy that did not order it. The parser returns no quantity and a note
// saying why, the line falls back to the default quantity, and the buyer sees
// what happened in the results table. A wrong order costs more than an
// unanswered cell.

// QtyResult is the outcome of reading one quantity cell.
type QtyResult struct {
	// Qty is nil when the cell could not be read as a single quantity. That is
	// not an error — it means the default quantity applies and the buyer is told.
	Qty  *float64
	Note string
}

// arabicIndicDigits maps Arabic-Indic and Eastern Arabic-Indic numerals onto
// ASCII. Egyptian systems export both, sometimes in the same column.
var arabicIndicDigits = strings.NewReplacer(
	"٠", "0", "١", "1", "٢", "2", "٣", "3", "٤", "4",
	"٥", "5", "٦", "6", "٧", "7", "٨", "8", "٩", "9",
	"۰", "0", "۱", "1", "۲", "2", "۳", "3", "۴", "4",
	"۵", "5", "۶", "6", "۷", "7", "۸", "8", "۹", "9",
	"٫", ".", // Arabic decimal separator
	"،", "", // Arabic comma used as a thousands separator
)

// ParseQuantity reads a spreadsheet cell as a requested quantity.
func ParseQuantity(raw string) QtyResult {
	s := strings.TrimSpace(raw)
	if s == "" {
		return QtyResult{} // blank: the default applies, and that is not noteworthy
	}

	s = arabicIndicDigits.Replace(s)
	s = strings.TrimSpace(strings.ReplaceAll(s, ",", ""))

	// A plain number is the overwhelmingly common case.
	if v, err := strconv.ParseFloat(s, 64); err == nil {
		return validateQty(v, raw)
	}

	// A range: "2-3", "2 - 3", "٢-٣". Ambiguous by construction.
	if isRange(s) {
		return QtyResult{Note: fmt.Sprintf("cell %q is a range; enter a single quantity", raw)}
	}

	// A number with a unit: "5 علبة", "5 boxes", "5pcs". The number is usable,
	// but the unit may not be the unit the supplier sells in, so it is recorded.
	if v, unit, ok := numberWithUnit(s); ok {
		res := validateQty(v, raw)
		if res.Qty != nil {
			res.Note = fmt.Sprintf("read %g from %q; unit %q ignored — check it matches the supplier's unit", v, raw, unit)
		}
		return res
	}

	return QtyResult{Note: fmt.Sprintf("could not read a quantity from %q", raw)}
}

// validateQty rejects values that are numbers but not quantities.
func validateQty(v float64, raw string) QtyResult {
	if v < 0 {
		return QtyResult{Note: fmt.Sprintf("cell %q is negative", raw)}
	}
	if v != float64(int64(v)) {
		// Fractional quantities are meaningful for some units and meaningless
		// for boxes. Kept, but flagged, because rounding it here would be the
		// silent guess this parser exists to avoid.
		return QtyResult{Qty: &v, Note: fmt.Sprintf("cell %q is fractional", raw)}
	}
	return QtyResult{Qty: &v}
}

// isRange reports whether the cell looks like "2-3" rather than a negative
// number or a hyphenated code.
func isRange(s string) bool {
	for _, sep := range []string{"-", "–", "—", "to", "الى", "إلى"} {
		idx := strings.Index(s, sep)
		if idx <= 0 || idx+len(sep) >= len(s) {
			continue
		}
		left := strings.TrimSpace(s[:idx])
		right := strings.TrimSpace(s[idx+len(sep):])
		if _, err := strconv.ParseFloat(left, 64); err != nil {
			continue
		}
		if _, err := strconv.ParseFloat(right, 64); err != nil {
			continue
		}
		return true
	}
	return false
}

// numberWithUnit splits a leading number from a trailing unit word.
func numberWithUnit(s string) (float64, string, bool) {
	end := 0
	for i, r := range s {
		if (r >= '0' && r <= '9') || r == '.' {
			end = i + 1
			continue
		}
		if i == 0 {
			return 0, "", false
		}
		break
	}
	if end == 0 {
		return 0, "", false
	}
	v, err := strconv.ParseFloat(strings.TrimSpace(s[:end]), 64)
	if err != nil {
		return 0, "", false
	}
	unit := strings.TrimSpace(s[end:])
	if unit == "" {
		return 0, "", false
	}
	return v, unit, true
}

// ApplyQuantities sets each line's effective quantity from the config, and marks
// zero-quantity lines so they are visibly excluded rather than quietly dropped.
func ApplyQuantities(cfg *Config, lines []*Line) {
	for _, l := range lines {
		l.EffectiveQty = cfg.EffectiveQty(l)
		if l.EffectiveQty <= 0 && l.Outcome != OutcomeRemoved {
			l.Outcome = OutcomeZeroQty
		}
	}
}

// summaryWords are the labels a spreadsheet's own footer rows carry.
//
// A pharmacy's file usually ends with a total line, and "المجموع" is not a
// product. Staging it would put it in front of the matcher, which would dutifully
// find the nearest medicine to the word "total".
var summaryWords = []string{
	"المجموع", "الاجمالي", "الإجمالي", "اجمالي", "إجمالي", "المجموع الكلي",
	"total", "subtotal", "grand total", "sum",
}

// IsSummaryRow reports whether a product cell is really a footer label.
func IsSummaryRow(name string) bool {
	n := strings.ToLower(strings.TrimSpace(name))
	if n == "" {
		return false
	}
	for _, w := range summaryWords {
		if n == strings.ToLower(w) {
			return true
		}
	}
	return false
}
