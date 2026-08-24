package catalog

import (
	"errors"
	"strconv"
	"strings"

	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// Value coercion for spreadsheet cells.
//
// money.Parse is deliberately strict — it refuses thousands separators, refuses
// more than two decimals, and refuses anything that is not a bare decimal —
// because a silent rounding in the ledger is how money goes missing. That is
// the right rule for an order total typed into a form. It is the wrong rule for
// a supplier price list, where the same column legitimately arrives as
// "1,234.50", "1.234,50", "٢٥٫٥٠", "115.00 ج.م", "42 EGP" and "35".
//
// So the strictness stays, and this layer sits in front of it: it decides what
// the human meant, produces a canonical decimal string, and reports what it had
// to assume. Anything it cannot read confidently becomes a row error the admin
// sees, never a silent zero — the previous importer discarded every price it
// could not parse and still reported success.

var (
	// ErrNoValue marks an empty cell, which is a normal absence rather than a
	// malformed value and must not be reported as an error.
	ErrNoValue = errors.New("import: empty value")
	// ErrNotNumeric marks a cell that carries no readable number.
	ErrNotNumeric = errors.New("import: value is not a number")
)

// currencyNoise is stripped before numeric parsing. Order matters: longer
// tokens come first, so "ج.م" is removed before a bare "م" could be.
var currencyNoise = []string{
	"ج.م.", "ج.م", "جنيه مصري", "جنيه", "جم", "egp", "l.e.", "l.e", "le",
	"pound", "pounds", "ر.س", "sar", "usd", "$", "€", "£", "ريال", "دولار",
	"pcs", "قطعة", "قطع", "علبة", "vat", "incl", "excl",
}

// NumericResult reports what CoerceDecimal made of a cell.
type NumericResult struct {
	// Canonical is the value as a bare decimal string with at most two
	// fraction digits, ready for money.Parse.
	Canonical string
	// Rounded is true when the source carried more precision than the
	// NUMERIC(12,2) column can hold and the value was rounded half away from
	// zero.
	Rounded bool
	// Percent is true when the source was written as a percentage ("20%").
	Percent bool
}

// CoerceDecimal reads a spreadsheet cell as a decimal number, tolerating the
// formatting a human or an export tool is likely to have applied.
//
// It returns ErrNoValue for an empty cell so the caller can distinguish "the
// supplier left the price out" from "the supplier typed something we could not
// read". Those need different messages.
func CoerceDecimal(raw string) (NumericResult, error) {
	s := strings.ToLower(CleanCellString(NormalizeDigits(raw)))
	if s == "" {
		return NumericResult{}, ErrNoValue
	}

	// Excel writes an unavailable value as one of these rather than leaving the
	// cell blank. They mean "no value", not "bad value".
	switch s {
	case "-", "--", "n/a", "na", "#n/a", "null", "nil", "none",
		"#value!", "#div/0!", "#ref!", "لا يوجد", "غير متاح":
		return NumericResult{}, ErrNoValue
	}

	for _, tok := range currencyNoise {
		s = strings.ReplaceAll(s, tok, " ")
	}

	percent := strings.Contains(s, "%") || strings.Contains(s, "٪")
	s = strings.NewReplacer(
		"%", "", "٪", "", " ", "", "٬", "", "’", "", "_", "",
	).Replace(s)

	// U+066B is the Arabic decimal separator; it always means a decimal point.
	s = strings.ReplaceAll(s, "٫", ".")

	neg := false
	// Accounting exports write a negative as "(12.50)".
	if strings.HasPrefix(s, "(") && strings.HasSuffix(s, ")") {
		neg = true
		s = strings.TrimSuffix(strings.TrimPrefix(s, "("), ")")
	}
	switch {
	case strings.HasPrefix(s, "-"):
		neg = true
		s = s[1:]
	case strings.HasPrefix(s, "+"):
		s = s[1:]
	}

	s = disambiguateSeparators(s)
	if !isASCIIDigits(strings.ReplaceAll(s, ".", "")) {
		return NumericResult{}, ErrNotNumeric
	}

	intPart, fracPart, hasFrac := strings.Cut(s, ".")
	if intPart == "" {
		intPart = "0"
	}
	if hasFrac && fracPart == "" {
		hasFrac = false
	}
	if !isASCIIDigits(intPart) || (hasFrac && !isASCIIDigits(fracPart)) {
		return NumericResult{}, ErrNotNumeric
	}

	res := NumericResult{Percent: percent}
	if hasFrac && len(fracPart) > 2 {
		intPart, fracPart = roundToTwo(intPart, fracPart)
		res.Rounded = true
	}

	// Trim leading zeros so "0000115" cannot overflow ParseInt on a long run.
	intPart = strings.TrimLeft(intPart, "0")
	if intPart == "" {
		intPart = "0"
	}

	canonical := intPart
	if hasFrac {
		canonical += "." + fracPart
	}
	if neg && canonical != "0" && canonical != "0.00" {
		canonical = "-" + canonical
	}
	res.Canonical = canonical
	return res, nil
}

// disambiguateSeparators decides whether "." and "," are decimal points or
// thousands groupings.
//
// The rule follows how the two conventions actually differ: whichever separator
// appears last is the decimal one, except that a repeated separator, or a
// trailing group of exactly three digits behind a short head, only ever happens
// in grouping.
func disambiguateSeparators(s string) string {
	lastDot := strings.LastIndex(s, ".")
	lastComma := strings.LastIndex(s, ",")

	switch {
	case lastDot < 0 && lastComma < 0:
		return s

	case lastDot >= 0 && lastComma >= 0:
		// Both present: the rightmost is the decimal separator and the other is
		// grouping. "1.234,50" and "1,234.50" both mean 1234.50.
		if lastComma > lastDot {
			return strings.ReplaceAll(s[:lastComma], ".", "") + "." + s[lastComma+1:]
		}
		return strings.ReplaceAll(s[:lastDot], ",", "") + "." + s[lastDot+1:]

	case lastComma >= 0:
		return resolveSingleSeparator(s, ',')

	default:
		return resolveSingleSeparator(s, '.')
	}
}

// resolveSingleSeparator handles a value using only one separator character.
//
// A repeated separator is always grouping — "1.234.567" and "1,234,567" are
// never decimals. A single one is genuinely ambiguous at three digits, and the
// two characters break the tie differently:
//
//   - A comma with a three-digit tail is grouping. "1,234" means one thousand
//     two hundred and thirty-four in every file this importer has seen; a price
//     of one and a bit written with a decimal comma would be "1,23".
//   - A dot is always a decimal point. Reading "25.005" as twenty-five thousand
//     — which the grouping rule did — overstates a price a thousandfold, while
//     misreading a European "1.234" as 1.23 is both rarer and far less harmful.
//
// Anything mixing both characters never reaches here; disambiguateSeparators
// resolves those by position, which is unambiguous.
func resolveSingleSeparator(s string, sep byte) string {
	parts := strings.Split(s, string(sep))
	tail := parts[len(parts)-1]

	grouping := len(parts) > 2 ||
		(sep == ',' && len(parts) == 2 && len(tail) == 3 && parts[0] != "")
	if grouping && isASCIIDigits(tail) {
		return strings.Join(parts, "")
	}
	return strings.Join(parts[:len(parts)-1], "") + "." + tail
}

// roundToTwo rounds a decimal given as separate integer and fraction strings to
// two fraction digits, half away from zero, carrying into the integer part.
func roundToTwo(intPart, fracPart string) (string, string) {
	keep := fracPart[:2]
	if fracPart[2] < '5' {
		return intPart, keep
	}

	frac, err := strconv.ParseInt(keep, 10, 64)
	if err != nil {
		return intPart, keep
	}
	frac++
	if frac < 100 {
		return intPart, pad2(frac)
	}

	// Carry: .995 rounds to 1.00, rolling the integer part up by one. Done as a
	// string addition so an integer part longer than int64 does not overflow on
	// the way through.
	return addOneDecimalString(intPart), "00"
}

func pad2(n int64) string {
	if n < 10 {
		return "0" + strconv.FormatInt(n, 10)
	}
	return strconv.FormatInt(n, 10)
}

func addOneDecimalString(s string) string {
	if s == "" {
		return "1"
	}
	b := []byte(s)
	for i := len(b) - 1; i >= 0; i-- {
		if b[i] < '9' {
			b[i]++
			return string(b)
		}
		b[i] = '0'
	}
	return "1" + string(b)
}

func isASCIIDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// CoerceMoney reads a cell as a monetary amount. The error cases stay distinct
// so the caller can report "the cell is empty" and "the cell is not a number"
// separately; both are useful to an admin fixing a supplier's file.
func CoerceMoney(raw string) (money.Amount, NumericResult, error) {
	res, err := CoerceDecimal(raw)
	if err != nil {
		return money.Zero, res, err
	}
	amt, err := money.Parse(res.Canonical)
	if err != nil {
		return money.Zero, res, err
	}
	return amt, res, nil
}

// CoerceInt reads a cell as a whole number, truncating any fraction.
// Quantities arrive as "45", "45.00" and "45 علبة".
func CoerceInt(raw string) (int64, error) {
	res, err := CoerceDecimal(raw)
	if err != nil {
		return 0, err
	}
	intPart, _, _ := strings.Cut(res.Canonical, ".")
	n, err := strconv.ParseInt(intPart, 10, 64)
	if err != nil {
		return 0, ErrNotNumeric
	}
	return n, nil
}

// CoerceStatus maps a status cell onto the four states catalog.products
// accepts. An unrecognised value returns false so the caller can warn rather
// than write a value the CHECK constraint would reject — which would abort the
// whole import transaction for one careless cell.
func CoerceStatus(raw string) (ProductStatus, bool) {
	switch NormalizeKey(raw) {
	case "":
		return "", false
	case "active", "enabled", "published", "مفعل", "نشط", "فعال", "متاح", "1", "yes", "نعم":
		return StatusActive, true
	case "inactive", "disabled", "hidden", "معطل", "غيرنشط", "موقوف", "غيرمتاح", "0", "no", "لا":
		return StatusInactive, true
	case "pending", "review", "معلق", "قيدالمراجعه", "مراجعه", "منتظر":
		return StatusPending, true
	case "rejected", "refused", "مرفوض", "ملغي", "ملغى":
		return StatusRejected, true
	}
	return "", false
}
