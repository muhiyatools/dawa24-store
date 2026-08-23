package spreadsheet

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/extrame/xls"
	"github.com/xuri/excelize/v2"
	"golang.org/x/net/html"
)

// Format represents the detected file format.
type Format string

const (
	FormatXLSX Format = "xlsx"
	FormatXLS  Format = "xls"
	FormatCSV  Format = "csv"
	FormatHTML Format = "html"
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

	// Check for HTML table disguised as spreadsheet
	sampleLen := len(data)
	if sampleLen > 1024 {
		sampleLen = 1024
	}
	trimmed := strings.TrimSpace(string(data[:sampleLen]))
	lower := strings.ToLower(trimmed)
	if strings.HasPrefix(lower, "<!doctype html") || strings.HasPrefix(lower, "<html") ||
		strings.Contains(lower, "<table") {
		return FormatHTML
	}

	return FormatCSV
}

// ReadRows parses all rows from a spreadsheet byte buffer regardless of whether it is
// .xlsx, .xls, .csv, or an HTML table.
func ReadRows(data []byte) ([][]string, error) {
	if len(data) == 0 {
		return nil, errors.New("empty file data")
	}

	format := SniffFormat(data)

	switch format {
	case FormatXLSX:
		return readXLSX(data)
	case FormatXLS:
		return readXLS(data)
	case FormatHTML:
		return readHTMLTable(data)
	case FormatCSV:
		return readCSV(data)
	default:
		// Try XLSX first, then fallback to XLS, then CSV
		rows, err := readXLSX(data)
		if err == nil && len(rows) > 0 {
			return rows, nil
		}
		rows, err = readXLS(data)
		if err == nil && len(rows) > 0 {
			return rows, nil
		}
		return readCSV(data)
	}
}

// ReadHeadersAndPreview extracts the first row as headers and up to previewCount subsequent rows.
func ReadHeadersAndPreview(data []byte, previewCount int) (headers []string, preview [][]string, err error) {
	allRows, err := ReadRows(data)
	if err != nil {
		return nil, nil, err
	}
	if len(allRows) == 0 {
		return nil, nil, errors.New("file contains no rows")
	}

	headers = sanitizeRow(allRows[0])
	maxPreview := len(allRows)
	if 1+previewCount < maxPreview {
		maxPreview = 1 + previewCount
	}
	for i := 1; i < maxPreview; i++ {
		preview = append(preview, sanitizeRow(allRows[i]))
	}
	return headers, preview, nil
}

func readXLSX(data []byte) ([][]string, error) {
	f, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		// If excelize fails on a disguised file, try XLS fallback
		if xlsRows, xlsErr := readXLS(data); xlsErr == nil && len(xlsRows) > 0 {
			return xlsRows, nil
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

	// Read from the first sheet
	rows, err := f.GetRows(sheets[0])
	if err != nil {
		return nil, fmt.Errorf("failed to read sheet %s: %w", sheets[0], err)
	}

	var cleaned [][]string
	for _, r := range rows {
		if !isRowEmpty(r) {
			cleaned = append(cleaned, sanitizeRow(r))
		}
	}
	return cleaned, nil
}

func readXLS(data []byte) ([][]string, error) {
	wb, err := xls.OpenReader(bytes.NewReader(data), "utf-8")
	if err != nil {
		// Fallback to trying as XLSX or CSV
		if csvRows, csvErr := readCSV(data); csvErr == nil && len(csvRows) > 0 {
			return csvRows, nil
		}
		return nil, fmt.Errorf("failed to open legacy XLS workbook: %w", err)
	}

	sheet := wb.GetSheet(0)
	if sheet == nil {
		return nil, errors.New("legacy XLS workbook has no sheets")
	}

	var result [][]string
	for i := 0; i <= int(sheet.MaxRow); i++ {
		row := sheet.Row(i)
		if row == nil {
			continue
		}
		var rowVals []string
		lastCol := row.LastCol()
		for c := 0; c < lastCol; c++ {
			val := row.Col(c)
			rowVals = append(rowVals, strings.TrimSpace(val))
		}
		if !isRowEmpty(rowVals) {
			result = append(result, sanitizeRow(rowVals))
		}
	}
	return result, nil
}

func readCSV(data []byte) ([][]string, error) {
	// Strip UTF-8 BOM if present
	if len(data) >= 3 && data[0] == 0xEF && data[1] == 0xBB && data[2] == 0xBF {
		data = data[3:]
	}

	// Sniff delimiter: comma, semicolon, tab, pipe
	delims := []rune{',', ';', '\t', '|'}
	sampleLen := len(data)
	if sampleLen > 2048 {
		sampleLen = 2048
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

	rows, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("failed to parse CSV: %w", err)
	}

	var cleaned [][]string
	for _, row := range rows {
		if !isRowEmpty(row) {
			cleaned = append(cleaned, sanitizeRow(row))
		}
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
