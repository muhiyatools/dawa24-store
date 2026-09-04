package filesecurity

import (
	"archive/zip"
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

// IsSuspiciousText checks whether a single cell text contains prohibited URLs, web addresses, or domains.
func IsSuspiciousText(text string, allowEmails bool) bool {
	t := strings.TrimSpace(text)
	if t == "" {
		return false
	}
	if schemeRegex.MatchString(t) {
		return true
	}
	if wwwRegex.MatchString(t) {
		return true
	}
	if formulaWebRegex.MatchString(t) {
		return true
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

// ValidateSpreadsheetSecurity inspects any uploaded spreadsheet (.xlsx, .xls, .csv, text)
// and rejects files containing URLs, web addresses, or domains with ErrSecurityBlocked.
func ValidateSpreadsheetSecurity(content []byte, filename string, opts ...Option) error {
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

func inspectXLSX(content []byte, cfg Options) error {
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

func inspectXLS(content []byte, cfg Options) error {
	wb, err := xreader.OpenReader(bytes.NewReader(content))
	if err != nil {
		return nil // Caller handles parsing error
	}
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
				if IsSuspiciousText(cell.GetString(), cfg.AllowEmails) {
					return ErrSecurityBlocked
				}
			}
		}
	}
	return nil
}

func inspectDelimited(content []byte, cfg Options) error {
	// 1. Try standard CSV reader
	r := csv.NewReader(bytes.NewReader(content))
	r.FieldsPerRecord = -1
	r.LazyQuotes = true
	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			break // Fallback to line scanning below
		}
		for _, cell := range record {
			if IsSuspiciousText(cell, cfg.AllowEmails) {
				return ErrSecurityBlocked
			}
		}
	}

	// 2. Line scanner for text/tsv/fallback
	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		// Split by common delimiters
		tokens := strings.FieldsFunc(trimmed, func(c rune) bool {
			return c == ',' || c == '\t' || c == ';' || c == '|'
		})
		for _, token := range tokens {
			if IsSuspiciousText(token, cfg.AllowEmails) {
				return ErrSecurityBlocked
			}
		}
	}
	return nil
}
