package pipeline

import (
	"fmt"
	"strings"

	"github.com/muhiya/dawa24-store/internal/modules/smartorder"
	"github.com/muhiya/dawa24-store/internal/shared/productmatch"
	"github.com/muhiya/dawa24-store/internal/shared/sheet"
)

// Reading the buyer's file.
//
// The workbook is streamed, never loaded whole: a ten-thousand-row file with a
// dozen columns is a lot of strings to hold at once, and the rows are handed
// straight to the database in batches anyway.
//
// The header row is found rather than assumed. Pharmacy exports routinely open
// with a title, a date, a branch name and a blank line before the real headers,
// and reading row 1 as the header turns every column into a mystery.

// MaxRows caps an import.
//
// Ten thousand is the figure the spec commits to (FR-002), and the cap is
// enforced at parse time so the buyer is told before waiting rather than after.
const MaxRows = 10000

// TargetFields are the columns smart ordering can use.
//
// Deliberately fewer than the vendor importer's: a purchase list carries no
// prices, because the prices are what the system is going to find.
var TargetFields = []struct {
	Key      string
	LabelAR  string
	Required bool
}{
	{"product_name", "اسم الصنف", true},
	{"sku", "كود الصنف", false},
	{"barcode", "الباركود", false},
	{"quantity", "الكمية", false},
}

// ParsedFile is what step 2 needs to render the mapping screen.
type ParsedFile struct {
	Headers    []string
	HeaderRow  int
	Preview    [][]string
	Detected   map[int]string
	Confidence map[string]float64
	RowCount   int
}

// Inspect opens the file and works out its shape, without staging any rows.
//
// Separated from Stage because the mapping screen needs the headers and a
// preview before the buyer has confirmed anything, and parsing ten thousand rows
// to show five would be wasteful.
func Inspect(content []byte, filename string) (*ParsedFile, error) {
	book, err := sheet.Open(content, filename)
	if err != nil {
		return nil, fmt.Errorf("could not read the file: %w", err)
	}
	defer book.Close()

	preview, err := book.Peek(25)
	if err != nil {
		return nil, fmt.Errorf("could not read the first rows: %w", err)
	}
	if len(preview.Rows) == 0 {
		return nil, fmt.Errorf("the file has no rows")
	}

	headerRow, detected, confidence := detectHeaders(preview.Rows)
	headers := preview.Rows[headerRow]

	body := preview.Rows[headerRow+1:]
	if len(body) > 5 {
		body = body[:5]
	}

	return &ParsedFile{
		Headers:    headers,
		HeaderRow:  headerRow,
		Preview:    body,
		Detected:   detected,
		Confidence: confidence,
	}, nil
}

// detectHeaders finds the real header row and maps the columns it recognises.
//
// Uses the shared column vocabulary, so smart ordering recognises the same
// Arabic and English aliases the vendor importer does — a pharmacy writing
// "الصنف" or "العدد" is understood by both.
func detectHeaders(rows [][]string) (int, map[int]string, map[string]float64) {
	best := 0
	bestScore := -1
	var bestMapping map[int]string
	var bestConfidence map[string]float64

	// Scan the first few rows: a banner is rarely more than three or four deep.
	limit := len(rows)
	if limit > 8 {
		limit = 8
	}
	for i := 0; i < limit; i++ {
		mapping, confidence := matchHeaderRow(rows[i])
		score := len(mapping)
		if score > bestScore {
			best, bestScore = i, score
			bestMapping, bestConfidence = mapping, confidence
		}
	}
	if bestMapping == nil {
		bestMapping = map[int]string{}
		bestConfidence = map[string]float64{}
	}
	return best, bestMapping, bestConfidence
}

// matchHeaderRow scores one candidate header row against the target fields.
func matchHeaderRow(row []string) (map[int]string, map[string]float64) {
	mapping := make(map[int]string)
	confidence := make(map[string]float64)
	taken := make(map[string]bool)

	for col, raw := range row {
		header := productmatch.NormalizeText(strings.TrimSpace(raw))
		if header == "" {
			continue
		}
		field, score := bestFieldFor(header)
		if field == "" || taken[field] {
			continue
		}
		// A weak guess is worse than none: it puts a confident-looking wrong
		// answer in front of the buyer, which is the failure step 2 exists to
		// prevent.
		if score < 0.6 {
			continue
		}
		mapping[col] = field
		confidence[field] = score
		taken[field] = true
	}
	return mapping, confidence
}

// fieldAliases are the header spellings each target field answers to.
var fieldAliases = []struct {
	Field   string
	Aliases []string
}{
	{"product_name", []string{"اسم الصنف", "الصنف", "اسم المنتج", "المنتج", "اسم الدواء", "الدواء",
		"البيان", "product", "product name", "item", "item name", "description", "name"}},
	{"sku", []string{"كود الصنف", "الكود", "كود المنتج", "رمز الصنف", "sku", "code", "item code",
		"product code", "ref"}},
	{"barcode", []string{"الباركود", "باركود", "barcode", "ean", "gtin", "upc"}},
	{"quantity", []string{"الكمية", "العدد", "الكميه", "كمية", "quantity", "qty", "count", "units",
		"required", "المطلوب"}},
}

// bestFieldFor picks the target field a header best matches.
func bestFieldFor(header string) (string, float64) {
	var bestField string
	var bestScore float64
	for _, candidate := range fieldAliases {
		for _, alias := range candidate.Aliases {
			if header == productmatch.NormalizeText(alias) {
				return candidate.Field, 1
			}
			score := productmatch.TextSimilarity(header, productmatch.NormalizeText(alias))
			if score > bestScore {
				bestField, bestScore = candidate.Field, score
			}
		}
	}
	return bestField, bestScore
}

// Stage reads the whole file through a confirmed mapping and returns the lines.
//
// Rows the mapping cannot give a product identity are dropped rather than staged
// as blanks — a spreadsheet's trailing empty rows and its subtotal line are not
// order lines, and staging them would put "المجموع" in front of the matcher.
func Stage(content []byte, filename string, m *smartorder.Mapping,
	runID, orgID int64) ([]*smartorder.Line, error) {

	book, err := sheet.Open(content, filename)
	if err != nil {
		return nil, fmt.Errorf("could not read the file: %w", err)
	}
	defer book.Close()

	nameCol, hasName := m.Column("product_name")
	skuCol, hasSKU := m.Column("sku")
	barcodeCol, hasBarcode := m.Column("barcode")
	qtyCol, hasQty := m.Column("quantity")

	var lines []*smartorder.Line
	var headers []string
	rowNumber := 0

	err = book.Walk(func(index int, row []string) error {
		if index == m.HeaderRow {
			headers = make([]string, len(row))
			copy(headers, row)
			return nil // header row itself
		}
		if index < m.HeaderRow {
			return nil // banner
		}
		if isRepeatedHeader(row, headers) {
			return nil // repeated header row inside the data
		}
		if len(lines) >= MaxRows {
			return fmt.Errorf("the file has more than %d rows", MaxRows)
		}

		name := cell(row, nameCol, hasName)
		sku := cell(row, skuCol, hasSKU)
		barcode := cell(row, barcodeCol, hasBarcode)

		if name == "" && sku == "" && barcode == "" {
			return nil // nothing identifies a product here
		}
		if smartorder.IsSummaryRow(name) {
			return nil
		}

		rowNumber++
		l := &smartorder.Line{
			RunID:          runID,
			OrganizationID: orgID,
			RowNumber:      rowNumber,
			Raw:            rawOf(row),
			RawName:        name,
			RawSKU:         sku,
			RawBarcode:     barcode,
			MatchMethod:    smartorder.MethodNone,
			Outcome:        smartorder.OutcomeUnmatched,
		}
		if hasQty {
			q := smartorder.ParseQuantity(cell(row, qtyCol, true))
			l.ImportedQty = q.Qty
			l.QtyParseNote = q.Note
		}
		lines = append(lines, l)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return lines, nil
}

func cell(row []string, col int, ok bool) string {
	if !ok || col < 0 || col >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[col])
}

// rawOf keeps the original cells so the buyer can always see what they uploaded,
// whatever the mapping decided.
func rawOf(row []string) map[string]string {
	out := make(map[string]string, len(row))
	for i, v := range row {
		if v = strings.TrimSpace(v); v != "" {
			out[fmt.Sprintf("%d", i)] = v
		}
	}
	return out
}

// isRepeatedHeader detects rows that are a duplicate copy of the file's header.
//
// Pharmacy exports with multiple sheets or sections repeat the header at each
// section boundary. Left in, these become lines whose raw_name is "Item
// Description" or "اسم الصنف", which the matcher dutifully tries to resolve.
// They dilute the stats, waste an AI slot, and confuse the buyer's review.
func isRepeatedHeader(row []string, headers []string) bool {
	if len(headers) == 0 || len(row) == 0 {
		return false
	}
	matches := 0
	checked := 0
	for i, h := range headers {
		h = strings.TrimSpace(h)
		if h == "" {
			continue
		}
		checked++
		if i < len(row) && strings.EqualFold(strings.TrimSpace(row[i]), h) {
			matches++
		}
	}
	return checked > 0 && matches >= 2
}
