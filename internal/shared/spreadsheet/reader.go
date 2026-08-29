package spreadsheet

import (
	"bytes"
	"encoding/csv"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/extrame/xls"
	"github.com/xuri/excelize/v2"
	"golang.org/x/net/html"
)

// Format represents the detected file format.
type Format string

const (
	FormatXLSX    Format = "xlsx"
	FormatXLS     Format = "xls"
	FormatXML2003 Format = "xml2003"
	FormatCSV     Format = "csv"
	FormatHTML    Format = "html"
)

// SniffFormat detects the workbook or tabular data format from raw bytes.
func SniffFormat(data []byte) Format {
	if len(data) >= 4 && data[0] == 'P' && data[1] == 'K' && data[2] == 0x03 && data[3] == 0x04 {
		return FormatXLSX
	}
	// OLE2 / Compound File Binary format (BIFF8 .xls)
	if len(data) >= 8 && data[0] == 0xD0 && data[1] == 0xCF && data[2] == 0x11 && data[3] == 0xE0 &&
		data[4] == 0xA1 && data[5] == 0xB1 && data[6] == 0x1A && data[7] == 0xE1 {
		return FormatXLS
	}

	sampleLen := len(data)
	if sampleLen > 2048 {
		sampleLen = 2048
	}
	trimmed := strings.TrimSpace(string(data[:sampleLen]))
	lower := strings.ToLower(trimmed)

	// Check for XML Spreadsheet 2003 disguised as .xls
	if strings.Contains(lower, "urn:schemas-microsoft-com:office:spreadsheet") ||
		(strings.Contains(lower, "<?xml") && strings.Contains(lower, "<workbook")) {
		return FormatXML2003
	}

	// Check for HTML table disguised as spreadsheet
	if strings.HasPrefix(lower, "<!doctype html") || strings.HasPrefix(lower, "<html") ||
		strings.Contains(lower, "<table") {
		return FormatHTML
	}

	return FormatCSV
}

// ReadRows parses all rows from a spreadsheet byte buffer regardless of whether it is
// .xlsx, .xls, .csv, XML-Spreadsheet 2003, or an HTML table.
//
// Every cell is scrubbed of NUL and other C0 control bytes before it is
// returned. Legacy BIFF .xls in particular carries NUL padding inside its
// string records, and PostgreSQL rejects those outright in a text column
// ("invalid byte sequence for encoding UTF8: 0x00"), which fails the whole
// import batch on a byte the user cannot see.
func ReadRows(data []byte) ([][]string, error) {
	rows, err := readRowsRaw(data)
	if err != nil {
		return nil, err
	}
	scrubRows(rows)
	return rows, nil
}

func readRowsRaw(data []byte) ([][]string, error) {
	if len(data) == 0 {
		return nil, errors.New("empty file data")
	}

	format := SniffFormat(data)

	switch format {
	case FormatXLSX:
		return readXLSX(data)
	case FormatXLS:
		return readXLS(data)
	case FormatXML2003:
		return readXMLSpreadsheet2003(data)
	case FormatHTML:
		return readHTMLTable(data)
	case FormatCSV:
		return readCSV(data)
	default:
		// Robust cascaded fallback
		if rows, err := readXLSX(data); err == nil && len(rows) > 0 {
			return rows, nil
		}
		if rows, err := readXLS(data); err == nil && len(rows) > 0 {
			return rows, nil
		}
		if rows, err := readXMLSpreadsheet2003(data); err == nil && len(rows) > 0 {
			return rows, nil
		}
		if rows, err := readHTMLTable(data); err == nil && len(rows) > 0 {
			return rows, nil
		}
		return readCSV(data)
	}
}

// scrubRows cleans every cell in place.
func scrubRows(rows [][]string) {
	for _, row := range rows {
		for i, cell := range row {
			row[i] = ScrubCell(cell)
		}
	}
}

// ScrubCell removes NUL and other C0 control characters from a cell, repairs
// invalid UTF-8, and trims surrounding whitespace. Tab, newline and carriage
// return are treated as spaces rather than dropped, so words either side of
// them do not run together.
func ScrubCell(s string) string {
	if s == "" {
		return s
	}
	if !strings.ContainsFunc(s, needsScrub) && utf8.ValidString(s) {
		return strings.TrimSpace(s)
	}

	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r == utf8.RuneError:
			// Dropped: an undecodable byte carries no meaning here.
		case isSpaceControl(r):
			b.WriteRune(0x20)
		case isScrubbable(r):
			// NUL and the remaining C0/C1 controls are dropped outright.
		default:
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(b.String())
}

// needsScrub reports any character ScrubCell would rewrite or drop.
func needsScrub(r rune) bool {
	return isScrubbable(r) || isSpaceControl(r)
}

// isSpaceControl reports the control characters that stand in for a space.
func isSpaceControl(r rune) bool {
	return r == 0x09 || r == 0x0a || r == 0x0d
}

// isScrubbable reports a character PostgreSQL will not accept in a text
// column, or that carries no meaning in a spreadsheet cell.
func isScrubbable(r rune) bool {
	if isSpaceControl(r) {
		return false
	}
	return r == 0 || r < 0x20 || (r >= 0x7f && r <= 0x9f)
}

// ReadHeadersAndPreview extracts the first detected header row and up to previewCount subsequent rows.
func ReadHeadersAndPreview(data []byte, previewCount int) (headers []string, preview [][]string, err error) {
	allRows, err := ReadRows(data)
	if err != nil {
		return nil, nil, err
	}
	if len(allRows) == 0 {
		return nil, nil, errors.New("file contains no rows")
	}

	headerIdx := FindHeaderRowIndex(allRows)
	headers = sanitizeRow(allRows[headerIdx])

	startRow := headerIdx + 1
	maxPreview := len(allRows)
	if startRow+previewCount < maxPreview {
		maxPreview = startRow + previewCount
	}
	for i := startRow; i < maxPreview; i++ {
		preview = append(preview, sanitizeRow(allRows[i]))
	}
	return headers, preview, nil
}

func readXLSX(data []byte) ([][]string, error) {
	f, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		// Fallbacks
		if xlsRows, xlsErr := readXLS(data); xlsErr == nil && len(xlsRows) > 0 {
			return xlsRows, nil
		}
		if xmlRows, xmlErr := readXMLSpreadsheet2003(data); xmlErr == nil && len(xmlRows) > 0 {
			return xmlRows, nil
		}
		if htmlRows, htmlErr := readHTMLTable(data); htmlErr == nil && len(htmlRows) > 0 {
			return htmlRows, nil
		}
		if csvRows, csvErr := readCSV(data); csvErr == nil && len(csvRows) > 0 {
			return csvRows, nil
		}
		return nil, fmt.Errorf("failed to open XLSX: %w", err)
	}
	defer f.Close()

	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		return nil, errors.New("workbook has no sheets")
	}

	// Read from first non-empty sheet
	for _, sheetName := range sheets {
		rows, err := f.GetRows(sheetName)
		if err == nil && len(rows) > 0 {
			cleaned := make([][]string, 0, len(rows))
			for _, r := range rows {
				if !isRowEmpty(r) {
					cleaned = append(cleaned, sanitizeRow(r))
				}
			}
			if len(cleaned) > 0 {
				return cleaned, nil
			}
		}
	}

	return nil, errors.New("workbook contains no data in any sheet")
}

func readXLS(data []byte) (rows [][]string, err error) {
	// Defend against panics in third-party legacy XLS binary parser
	defer func() {
		if r := recover(); r != nil {
			if xmlRows, xmlErr := readXMLSpreadsheet2003(data); xmlErr == nil && len(xmlRows) > 0 {
				rows = xmlRows
				err = nil
			} else if htmlRows, htmlErr := readHTMLTable(data); htmlErr == nil && len(htmlRows) > 0 {
				rows = htmlRows
				err = nil
			} else if csvRows, csvErr := readCSV(data); csvErr == nil && len(csvRows) > 0 {
				rows = csvRows
				err = nil
			} else {
				err = fmt.Errorf("failed to parse XLS workbook: %v", r)
			}
		}
	}()

	// Try UTF-8 first
	wb, openErr := xls.OpenReader(bytes.NewReader(data), "utf-8")
	if openErr != nil || wb == nil {
		// Try Windows-1256 Arabic encoding
		wb, openErr = xls.OpenReader(bytes.NewReader(data), "windows-1256")
	}

	if openErr != nil || wb == nil {
		// Fallback to XML Spreadsheet 2003 or HTML or CSV
		if xmlRows, xmlErr := readXMLSpreadsheet2003(data); xmlErr == nil && len(xmlRows) > 0 {
			return xmlRows, nil
		}
		if htmlRows, htmlErr := readHTMLTable(data); htmlErr == nil && len(htmlRows) > 0 {
			return htmlRows, nil
		}
		if csvRows, csvErr := readCSV(data); csvErr == nil && len(csvRows) > 0 {
			return csvRows, nil
		}
		return nil, fmt.Errorf("failed to open legacy XLS workbook: %w", openErr)
	}

	// Search all worksheets for the densest sheet
	numSheets := wb.NumSheets()
	if numSheets == 0 {
		return nil, errors.New("legacy XLS workbook has no sheets")
	}

	var bestSheetRows [][]string
	for sIdx := 0; sIdx < numSheets; sIdx++ {
		sheet := wb.GetSheet(sIdx)
		if sheet == nil {
			continue
		}
		var currentSheetRows [][]string
		for i := 0; i <= int(sheet.MaxRow); i++ {
			row := sheet.Row(i)
			if row == nil {
				continue
			}
			var rowVals []string
			lastCol := row.LastCol()
			for c := 0; c < lastCol; c++ {
				val := row.Col(c)
				rowVals = append(rowVals, ScrubCell(val))
			}
			if !isRowEmpty(rowVals) {
				currentSheetRows = append(currentSheetRows, sanitizeRow(rowVals))
			}
		}
		if len(currentSheetRows) > len(bestSheetRows) {
			bestSheetRows = currentSheetRows
		}
	}

	if len(bestSheetRows) == 0 {
		// If binary reader returned empty rows, attempt XML/HTML/CSV parsing
		if xmlRows, xmlErr := readXMLSpreadsheet2003(data); xmlErr == nil && len(xmlRows) > 0 {
			return xmlRows, nil
		}
		if htmlRows, htmlErr := readHTMLTable(data); htmlErr == nil && len(htmlRows) > 0 {
			return htmlRows, nil
		}
		if csvRows, csvErr := readCSV(data); csvErr == nil && len(csvRows) > 0 {
			return csvRows, nil
		}
	}

	return bestSheetRows, nil
}

// readXMLSpreadsheet2003 parses Microsoft Office XML Spreadsheet 2003 files (common in older Egyptian ERPs).
func readXMLSpreadsheet2003(data []byte) ([][]string, error) {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	var rows [][]string
	var currentRow []string
	var currentCell strings.Builder
	inCellData := false

	for {
		tok, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		switch se := tok.(type) {
		case xml.StartElement:
			local := strings.ToLower(se.Name.Local)
			if local == "row" {
				currentRow = make([]string, 0)
			} else if local == "cell" {
				currentCell.Reset()
			} else if local == "data" {
				inCellData = true
			}
		case xml.EndElement:
			local := strings.ToLower(se.Name.Local)
			if local == "row" {
				if !isRowEmpty(currentRow) {
					rows = append(rows, sanitizeRow(currentRow))
				}
				currentRow = nil
			} else if local == "cell" {
				currentRow = append(currentRow, strings.TrimSpace(currentCell.String()))
			} else if local == "data" {
				inCellData = false
			}
		case xml.CharData:
			if inCellData {
				currentCell.Write(se)
			}
		}
	}

	if len(rows) == 0 {
		return nil, errors.New("no rows found in XML spreadsheet")
	}
	return rows, nil
}

func readCSV(data []byte) ([][]string, error) {
	// Strip UTF-8 BOM if present
	if len(data) >= 3 && data[0] == 0xEF && data[1] == 0xBB && data[2] == 0xBF {
		data = data[3:]
	}

	// Sniff delimiter: comma, semicolon, tab, pipe
	delims := []rune{',', ';', '\t', '|'}
	sampleLen := len(data)
	if sampleLen > 4096 {
		sampleLen = 4096
	}
	sample := string(data[:sampleLen])
	bestDelim := ','
	maxCount := 0
	for _, d := range delims {
		c := strings.Count(sample, string(d))
		if c > maxCount {
			maxCount = c
			bestDelim = d
		}
	}

	r := csv.NewReader(bytes.NewReader(data))
	r.Comma = bestDelim
	r.FieldsPerRecord = -1
	r.TrimLeadingSpace = true
	r.LazyQuotes = true

	var cleaned [][]string
	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			// Recover from malformed lines in large files without aborting
			continue
		}
		if !isRowEmpty(record) {
			cleaned = append(cleaned, sanitizeRow(record))
		}
	}

	if len(cleaned) == 0 {
		return nil, errors.New("no valid rows found in CSV")
	}
	return cleaned, nil
}

func readHTMLTable(data []byte) ([][]string, error) {
	doc, err := html.Parse(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to parse HTML table: %w", err)
	}

	var rows [][]string
	var traverse func(*html.Node)
	traverse = func(n *html.Node) {
		if n.Type == html.ElementNode && strings.ToLower(n.Data) == "tr" {
			var rowVals []string
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				if c.Type == html.ElementNode && (strings.ToLower(c.Data) == "td" || strings.ToLower(c.Data) == "th") {
					text := extractText(c)
					rowVals = append(rowVals, strings.TrimSpace(text))
				}
			}
			if !isRowEmpty(rowVals) {
				rows = append(rows, sanitizeRow(rowVals))
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			traverse(c)
		}
	}
	traverse(doc)
	return rows, nil
}

func extractText(n *html.Node) string {
	if n.Type == html.TextNode {
		return n.Data
	}
	var sb strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		sb.WriteString(extractText(c))
	}
	return sb.String()
}

func isRowEmpty(row []string) bool {
	for _, cell := range row {
		if strings.TrimSpace(cell) != "" {
			return false
		}
	}
	return true
}

func sanitizeRow(row []string) []string {
	res := make([]string, len(row))
	for i, cell := range row {
		val := strings.TrimSpace(cell)
		if !utf8.ValidString(val) {
			val = strings.ToValidUTF8(val, "")
		}
		res[i] = val
	}
	return res
}

// FindHeaderRowIndex scans the leading rows (up to 10) to detect the true header row.
func FindHeaderRowIndex(rows [][]string) int {
	if len(rows) <= 1 {
		return 0
	}

	headerKeywords := []string{
		"اسم", "صنف", "منتج", "سعر", "خصم", "كود", "باركود", "كمية", "جمهور", "صافي",
		"name", "product", "item", "price", "discount", "sku", "code", "barcode", "qty", "net",
	}

	bestIdx := 0
	maxMatches := 0
	maxScan := len(rows)
	if maxScan > 10 {
		maxScan = 10
	}

	for i := 0; i < maxScan; i++ {
		row := rows[i]
		matches := 0
		for _, cell := range row {
			lower := strings.ToLower(strings.TrimSpace(cell))
			for _, kw := range headerKeywords {
				if strings.Contains(lower, kw) {
					matches++
					break
				}
			}
		}
		if matches > maxMatches {
			maxMatches = matches
			bestIdx = i
		}
	}

	return bestIdx
}

// ParseCleanDiscount robustly converts any raw discount string into a percentage float (e.g. "25%", "0.25", "25.5 %" -> 25.5).
func ParseCleanDiscount(raw string) float64 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0.0
	}

	// Remove percentage signs, currency indicators, and extraneous characters
	var sb strings.Builder
	hasDot := false
	for _, r := range raw {
		if unicode.IsDigit(r) {
			sb.WriteRune(r)
		} else if (r == '.' || r == ',') && !hasDot {
			sb.WriteRune('.')
			hasDot = true
		}
	}

	cleaned := sb.String()
	if cleaned == "" {
		return 0.0
	}

	val, err := strconv.ParseFloat(cleaned, 64)
	if err != nil || val <= 0 {
		return 0.0
	}

	// If entered as decimal ratio e.g. 0.25 -> 25.0%
	if val > 0 && val < 1.0 {
		val = val * 100.0
	}

	// Cap at 100%
	if val > 100.0 {
		val = 100.0
	}

	return val
}

// ParseCleanPrice extracts numerical price from raw text stripping currencies like EGP or ج.م.
func ParseCleanPrice(raw string) (float64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0.0, errors.New("empty price")
	}

	var sb strings.Builder
	hasDot := false
	for _, r := range raw {
		if unicode.IsDigit(r) {
			sb.WriteRune(r)
		} else if (r == '.' || r == ',') && !hasDot {
			sb.WriteRune('.')
			hasDot = true
		}
	}

	cleaned := sb.String()
	if cleaned == "" {
		return 0.0, errors.New("invalid price format")
	}

	return strconv.ParseFloat(cleaned, 64)
}
