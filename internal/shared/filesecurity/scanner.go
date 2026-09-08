package filesecurity

import (
	"archive/zip"
	"bufio"
	"bytes"
	"encoding/csv"
	"errors"
	"io"
	"path/filepath"
	"regexp"
	"strings"

	xreader "github.com/shakinm/xlsReader/xls"
	"github.com/xuri/excelize/v2"
)

// SecurityErrorMessage is the canonical Arabic message returned when an upload is blocked.
const SecurityErrorMessage = "فشل الرفع لأسباب امنية"

// ErrSecurityBlocked is the error returned when a spreadsheet contains suspicious URLs or domains.
var ErrSecurityBlocked = errors.New(SecurityErrorMessage)

var (
	schemeRegex     = regexp.MustCompile(`(?i)(https?|ftp|ftps|file)://|javascript:|data:text`)
	wwwRegex        = regexp.MustCompile(`(?i)\bwww\.[a-zA-Z0-9\-]+`)
	ipRegex         = regexp.MustCompile(`\b(?:(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.){3}(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)(?::\d{1,5})?(?:/[^\s]*)?\b`)
	formulaWebRegex = regexp.MustCompile(`(?i)^[=+\-@].*\b(HYPERLINK|WEBSERVICE)\b`)
	domainRegex     = regexp.MustCompile(`(?i)\b[a-zA-Z0-9](?:[a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?\.(?:[a-zA-Z0-9\-]{1,61}\.)*(com|net|org|edu|gov|io|co|ai|xyz|info|biz|top|online|site|shop|club|vip|pro|link|click|cloud|live|tech|space|website|store|pharmacy|health|care|me|app|dev|mobi|security|tv|cc|to|ly|gl|is|ru|cn|in|uk|us|de|fr|nl|ca|au|br|eu|ch|se|no|es|it|pl|ua|tr|eg|sa|ae|kw|qa|bh|om|jo|lb|sy|iq|ye|sd|tn|dz|ma)(?::\d{1,5})?(?:[/?#][^\s]*)?\b`)
	emailRegex      = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)
)

// Options configure spreadsheet security inspection.
type Options struct {
	AllowEmails bool
	// AllowURLs permits addresses in a file whose whole purpose is to carry
	// them.
	//
	// Exactly one importer qualifies: the product-image import, whose column IS
	// a list of image URLs and which is unusable without them. It is an opt-in
	// rather than an exception coded in here, for the same reason AllowEmails
	// is: the caller knows what its file is for, and this package does not.
	//
	// It is not a hole. The URLs in that file are fetched by the importer under
	// its own rules — scheme, host and size — and a scanner that refused to let
	// the file be read at all did not make that safer; it made the feature
	// impossible while leaving every other upload exactly as protected.
	AllowURLs bool
}

// Option modifies Options.
type Option func(*Options)

// WithAllowEmails allows legitimate employee email addresses (e.g. in team imports).
func WithAllowEmails(allow bool) Option {
	return func(o *Options) {
		o.AllowEmails = allow
	}
}

// WithAllowURLs allows addresses in a file whose purpose is to carry them.
// See Options.AllowURLs — the product-image import and nothing else.
func WithAllowURLs(allow bool) Option {
	return func(o *Options) {
		o.AllowURLs = allow
	}
}

// canCarryAnAddress is the cheap gate every cell passes through before any
// regular expression is compiled against it.
//
// It exists because the scan was the slowest thing in the upload path. A
// twenty-thousand-row price list is a quarter of a million cells, and running
// five regular expressions over each of them — one of which is a sixty-branch
// top-level-domain alternation — measured at 2.3 seconds of CPU per file, in
// the request, twice. Nearly all of that work was spent proving that "50 MG
// TAB" is not a web address.
//
// The gate is sound rather than heuristic. Every pattern this package refuses
// needs at least one of these characters to match at all:
//
//	schemeRegex      "://", "javascript:", "data:text"   -> ':'
//	wwwRegex         "www."                              -> '.'
//	ipRegex          dotted quad                         -> '.'
//	domainRegex      a label, a dot, then a TLD          -> '.'
//	emailRegex       local '@' domain '.' tld            -> '@' and '.'
//	formulaWebRegex  anchored at =, +, - or @            -> first byte
//
// so a cell holding none of them cannot match any of them, and skipping it
// changes no verdict. Keep this function and the regexes above in step: a new
// pattern that can match without one of these characters must widen the gate.
func canCarryAnAddress(t string) bool {
	f := scanCell(t)
	return f.formulaPrefix || f.colon || f.dotThenLabel
}

// cellFacts is what one byte scan of a cell tells us about which patterns
// could possibly match it.
type cellFacts struct {
	// formulaPrefix: the value opens with =, +, - or @.
	formulaPrefix bool
	// colon: the value contains ':' anywhere.
	colon bool
	// dotThenLabel: the value contains a '.' immediately followed by a
	// character that can begin a DNS label — an ASCII letter, a digit or a
	// hyphen.
	dotThenLabel bool
}

// scanCell derives the facts above in a single pass, so each cell is walked
// once instead of up to five times by five separate regex engines.
//
// All the characters it looks for are single-byte ASCII, so scanning bytes is
// exact on UTF-8: a multi-byte Arabic rune has every byte >= 0x80 and can
// never be mistaken for '.', ':' or a letter.
func scanCell(t string) cellFacts {
	var f cellFacts
	if len(t) == 0 {
		return f
	}
	f.formulaPrefix = isFormulaPrefix(t[0])
	for i := 0; i < len(t); i++ {
		switch t[i] {
		case ':':
			f.colon = true
		case '.':
			if i+1 < len(t) {
				c := t[i+1]
				if c == '-' ||
					(c >= '0' && c <= '9') ||
					(c >= 'a' && c <= 'z') ||
					(c >= 'A' && c <= 'Z') {
					f.dotThenLabel = true
				}
			}
		}
	}
	return f
}

// IsSuspiciousText checks whether a single cell text contains prohibited URLs, web addresses, or domains.
//
// Each pattern is tried only when the cheap scan says it could match. The
// mapping is derived from the expressions themselves and must be kept in step
// with them:
//
//	schemeRegex      "://" / "javascript:" / "data:text"  needs a colon
//	wwwRegex         "www." + a label                     needs a dot then a label
//	ipRegex          dotted quad                          needs a dot then a digit
//	domainRegex      labels, a dot, then a TLD            needs a dot then a letter
//	formulaWebRegex  anchored at =, +, - or @             needs a formula prefix
//
// This is what makes the scan cheap on a real price list: "PARACETAMOL 500MG
// TAB" and "generic pharma co." carry no dot-then-label and no colon, so
// neither reaches the sixty-branch top-level-domain alternation that dominated
// the old profile.
func IsSuspiciousText(text string, allowEmails bool) bool {
	t := strings.TrimSpace(text)
	if t == "" {
		return false
	}
	f := scanCell(t)
	if !f.formulaPrefix && !f.colon && !f.dotThenLabel {
		return false
	}
	if f.colon && schemeRegex.MatchString(t) {
		return true
	}
	if f.dotThenLabel && wwwRegex.MatchString(t) {
		return true
	}
	if f.formulaPrefix && formulaWebRegex.MatchString(t) {
		return true
	}
	if !f.dotThenLabel {
		// Neither ipRegex nor domainRegex can match without one.
		return false
	}
	if loc := ipRegex.FindStringIndex(t); loc != nil {
		// Corroborated the same way a domain is, and for the same reason. A
		// B-complex ingredient list — "vitamin b1.2.3.5.6.9.12" — carries four
		// dot-separated numbers in the middle of a sentence, and 2.3.5.6 is a
		// perfectly good IPv4 address. It is not an address here.
		if looksLikeAnAddress(t, loc) {
			return true
		}
	}
	if loc := domainRegex.FindStringIndex(t); loc != nil {
		// A cell holding nothing but an address is judged on its own terms: an
		// e-mail is refused unless the caller has said this file legitimately
		// carries them, which is the team import and nothing else.
		if emailRegex.MatchString(t) {
			return !allowEmails
		}
		return looksLikeAnAddress(t, loc)
	}
	return false
}

// looksLikeAnAddress decides whether a domain-shaped run of characters is
// actually an address, or a dot inside ordinary text.
//
// The distinction is not pedantry. Measured against the platform's own
// twenty-thousand-product catalogue file, the domain shape alone matched:
//
//	"ce.is.co srl > macro group pharmaceuticals"   an Italian manufacturer
//	"cholecalciferol 10mcg eq.to vit d3 400 iu"    a scientific name
//	"...+vitamin b1.2.3.5.6.9.12+vitamin k+..."    an ingredient list
//
// ".co", ".to" and ".is" are real top-level domains and also the tail of
// ordinary abbreviations, so a substring test rejects the master catalogue
// itself — and it does it with a message that says only that the upload failed
// for security reasons, which is unanswerable for the person holding the file.
//
// So the shape has to be corroborated by something an address has and prose
// does not: it stands alone as the whole value, or it carries a path, a query,
// a fragment or a port. Every genuine exfiltration vector is untouched — a
// scheme, "www.", an IP, a HYPERLINK or WEBSERVICE formula, and a bare domain
// in a cell of its own are all still refused.
func looksLikeAnAddress(text string, loc []int) bool {
	match := text[loc[0]:loc[1]]
	// A path, a query, a fragment or a port is inside the match itself.
	if strings.ContainsAny(match, "/?#:") {
		return true
	}
	// Otherwise the cell has to BE the address rather than to mention one.
	return strings.TrimSpace(text) == match
}

// maxCellsScanned bounds one inspection.
//
// The scan is a guard against a payload, not an audit, and a payload is not
// hiding at row forty thousand: an attacker who wanted one read has to put it
// where a spreadsheet program will evaluate it, and the importer validates
// every field it actually consumes regardless of what this pass concluded.
//
// A quarter of a million cells covers a twenty-thousand-row file at twelve
// columns with room over. Past it the file is accepted, because spending
// unbounded CPU inside an upload is its own availability problem — one very
// large workbook could otherwise hold a request open for minutes.
const maxCellsScanned = 250_000

// budget tracks how much of a file has been examined. The zero value is not
// usable; construct with newBudget.
type budget struct{ remaining int }

func newBudget() *budget { return &budget{remaining: maxCellsScanned} }

// spend reports whether there is room to examine one more cell.
func (b *budget) spend() bool {
	if b.remaining <= 0 {
		return false
	}
	b.remaining--
	return true
}

// ValidateSpreadsheetSecurity inspects any uploaded spreadsheet (.xlsx, .xls, .csv, text)
// and rejects files containing URLs, web addresses, or domains with ErrSecurityBlocked.
func ValidateSpreadsheetSecurity(content []byte, filename string, opts ...Option) (err error) {
	if len(content) == 0 {
		return nil
	}
	var cfg Options
	for _, opt := range opts {
		opt(&cfg)
	}
	if cfg.AllowURLs {
		return nil
	}

	defer func() {
		if r := recover(); r != nil {
			// Catch any unexpected panic from third-party binary parsers (e.g. malformed BIFF/XLS records)
			// and fall back to raw content inspection for URLs to maintain security without crashing the handler.
			err = inspectRawContentForURLs(content, cfg)
		}
	}()

	// 1. Detect format by magic bytes or extension
	isZIP := bytes.HasPrefix(content, []byte{'P', 'K', 0x03, 0x04})
	isOLE2 := bytes.HasPrefix(content, []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1})
	ext := strings.ToLower(filepath.Ext(filename))

	if isZIP || ext == ".xlsx" || ext == ".xlsm" {
		return inspectXLSX(content, cfg)
	}
	if isOLE2 || ext == ".xls" {
		return inspectXLS(content, cfg)
	}
	return inspectDelimited(content, cfg)
}

func inspectXLSX(content []byte, cfg Options) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = inspectRawContentForURLs(content, cfg)
		}
	}()

	// A. Deep inspect ZIP relationships for hidden external links
	zr, err := zip.NewReader(bytes.NewReader(content), int64(len(content)))
	if err == nil {
		for _, f := range zr.File {
			nameLower := strings.ToLower(f.Name)
			if strings.HasSuffix(nameLower, ".rels") {
				rc, rErr := f.Open()
				if rErr == nil {
					buf, _ := io.ReadAll(io.LimitReader(rc, 2<<20))
					_ = rc.Close()
					bufStr := strings.ToLower(string(buf))
					if strings.Contains(bufStr, "targetmode=\"external\"") ||
						strings.Contains(bufStr, "target=\"http://") ||
						strings.Contains(bufStr, "target=\"https://") ||
						strings.Contains(bufStr, "target=\"ftp://") {
						return ErrSecurityBlocked
					}
				}
			}
		}
	}

	// B. Inspect cell values across all sheets via excelize
	f, err := excelize.OpenReader(bytes.NewReader(content))
	if err != nil {
		return nil // Parser error will be handled by the caller
	}
	defer func() { _ = f.Close() }()

	b := newBudget()
	for _, sheetName := range f.GetSheetList() {
		rows, err := f.Rows(sheetName)
		if err != nil {
			continue
		}
		for rows.Next() {
			cols, cErr := rows.Columns()
			if cErr != nil {
				break
			}
			for _, cell := range cols {
				if !b.spend() {
					_ = rows.Close()
					return nil
				}
				if IsSuspiciousText(cell, cfg.AllowEmails) {
					_ = rows.Close()
					return ErrSecurityBlocked
				}
			}
		}
		_ = rows.Close()
	}
	return nil
}

func inspectXLS(content []byte, cfg Options) (err error) {
	defer func() {
		if r := recover(); r != nil {
			// xlsReader may panic on malformed or truncated BIFF records (e.g. slice bounds error).
			// We recover here and fallback to raw content inspection so the file can either be blocked
			// if it contains raw URLs, or passed to downstream parsers (which have multi-tier fallback).
			err = inspectRawContentForURLs(content, cfg)
		}
	}()

	wb, err := xreader.OpenReader(bytes.NewReader(content))
	if err != nil {
		return inspectRawContentForURLs(content, cfg)
	}
	b := newBudget()
	numSheets := wb.GetNumberSheets()
	for s := 0; s < numSheets; s++ {
		sh, serr := wb.GetSheet(s)
		if serr != nil || sh == nil {
			continue
		}
		totalRows := sh.GetNumberRows()
		for r := 0; r <= totalRows; r++ {
			row, rerr := sh.GetRow(r)
			if rerr != nil || row == nil {
				continue
			}
			cols := row.GetCols()
			for _, cell := range cols {
				if cell == nil {
					continue
				}
				if !b.spend() {
					return nil
				}
				if IsSuspiciousText(cell.GetString(), cfg.AllowEmails) {
					return ErrSecurityBlocked
				}
			}
		}
	}
	return nil
}

// splitDelimiter reports whether c separates two fields in a delimited file.
func splitDelimiter(c rune) bool {
	return c == ',' || c == '\t' || c == ';' || c == '|'
}

// isFormulaPrefix reports whether c opens a spreadsheet formula.
// Keep in step with formulaWebRegex's character class.
func isFormulaPrefix(c byte) bool {
	return c == '=' || c == '+' || c == '-' || c == '@'
}

// hasFieldFormulaPrefix reports whether any field on this line begins with a
// formula character, so the line-level gate in inspectDelimited cannot skip a
// line whose formula sits after the first delimiter.
//
// A field begins at the start of the line or just after a delimiter, and
// IsSuspiciousText trims each field before testing it, so leading whitespace
// is skipped here too. All four delimiters and the whitespace this steps over
// are single-byte ASCII, so a byte scan is exact on UTF-8 input.
func hasFieldFormulaPrefix(line string) bool {
	atFieldStart := true
	for i := 0; i < len(line); i++ {
		c := line[i]
		switch {
		case splitDelimiter(rune(c)):
			atFieldStart = true
		case c == ' ' || c == '\t' || c == '\r':
			// Whitespace does not end a field's leading run.
		case atFieldStart:
			if isFormulaPrefix(c) {
				return true
			}
			atFieldStart = false
		}
	}
	return false
}

// maxDelimitedLine is the longest single line the fallback scanner will read.
//
// bufio.Scanner defaults to 64 KB and reports an error past it; a price list
// exported as one long line would then be skipped silently. One megabyte is
// past anything a spreadsheet export produces and is still bounded.
const maxDelimitedLine = 1 << 20

func inspectDelimited(content []byte, cfg Options) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = inspectRawContentForURLs(content, cfg)
		}
	}()

	b := newBudget()

	// 1. Try the standard CSV reader.
	r := csv.NewReader(bytes.NewReader(content))
	r.FieldsPerRecord = -1
	r.LazyQuotes = true
	r.ReuseRecord = true // the record is consumed before the next Read
	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			break // Fallback to line scanning below
		}
		for _, cell := range record {
			if !b.spend() {
				return nil
			}
			if IsSuspiciousText(cell, cfg.AllowEmails) {
				return ErrSecurityBlocked
			}
		}
	}

	// 2. Line scanner for text/tsv/fallback.
	//
	// It streams rather than materialising the file twice. The previous
	// version did `strings.Split(string(content), "\n")`, which allocated a
	// second complete copy of the upload as a string AND a slice header for
	// every line in it — on a 200 MB import batch that is 200 MB of garbage
	// produced to re-read what had just been read.
	sc := bufio.NewScanner(bytes.NewReader(content))
	sc.Buffer(make([]byte, 0, 64<<10), maxDelimitedLine)
	for sc.Scan() {
		trimmed := strings.TrimSpace(sc.Text())
		if trimmed == "" {
			continue
		}
		// One test on the line replaces one test per field, and skips the
		// FieldsFunc allocation entirely for the ordinary case.
		//
		// Two conditions, not one. canCarryAnAddress covers the patterns that
		// match anywhere inside a value, and those survive being split on a
		// delimiter. It does NOT cover formulaWebRegex, which is anchored to
		// the start of a FIELD — "tab,=HYPERLINK(x)" holds no dot, colon or
		// at-sign, so the address test alone would wave it through while the
		// per-field test would have refused it.
		if !canCarryAnAddress(trimmed) && !hasFieldFormulaPrefix(trimmed) {
			continue
		}
		for _, token := range strings.FieldsFunc(trimmed, splitDelimiter) {
			if !b.spend() {
				return nil
			}
			if IsSuspiciousText(token, cfg.AllowEmails) {
				return ErrSecurityBlocked
			}
		}
	}
	return nil
}

// inspectRawContentForURLs scans raw file content bytes for suspicious URLs or external targets
// when a structured parser cannot parse or panics on malformed input.
func inspectRawContentForURLs(content []byte, cfg Options) error {
	if cfg.AllowURLs || len(content) == 0 {
		return nil
	}
	lower := bytes.ToLower(content)
	if bytes.Contains(lower, []byte("http://")) ||
		bytes.Contains(lower, []byte("https://")) ||
		bytes.Contains(lower, []byte("ftp://")) ||
		bytes.Contains(lower, []byte("javascript:")) ||
		bytes.Contains(lower, []byte("data:text")) ||
		wwwRegex.Match(content) {
		return ErrSecurityBlocked
	}
	return nil
}

// Scanned is an upload payload that has passed ValidateSpreadsheetSecurity.
//
// It exists because the compare upload used to scan every file twice: the
// handler scanned it before writing it to disk, and the service scanned the
// same bytes again a few lines later inside RegisterAndStage. On a ten-file
// batch of twenty-thousand-row price lists that was the single most expensive
// thing in the request.
//
// Deleting one of the two calls would have fixed the cost and left the trap:
// nothing in the type system said which layer owned the scan, so the next
// caller of RegisterAndStage would have had to know, and a caller that did not
// would have staged an unscanned file.
//
// A Scanned cannot be constructed except by passing the scan, and holding one
// is proof the scan already ran — so the scan cannot be skipped and cannot be
// repeated. The zero value carries no payload and stages nothing.
type Scanned struct {
	content []byte
}

// Scan validates content and, on success, returns proof of it.
func Scan(content []byte, filename string, opts ...Option) (s Scanned, err error) {
	defer func() {
		if r := recover(); r != nil {
			if secErr := inspectRawContentForURLs(content, Options{}); secErr != nil {
				err = secErr
				s = Scanned{}
			} else {
				err = nil
				s = Scanned{content: content}
			}
		}
	}()
	if err := ValidateSpreadsheetSecurity(content, filename, opts...); err != nil {
		return Scanned{}, err
	}
	return Scanned{content: content}, nil
}

// Bytes returns the validated payload.
func (s Scanned) Bytes() []byte { return s.content }
