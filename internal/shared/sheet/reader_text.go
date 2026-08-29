package sheet

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/extrame/xls"
	"golang.org/x/net/html"
)

// Decoding for everything that is not an OOXML workbook.
//
// These three formats are read whole rather than streamed, because their
// libraries parse whole anyway: BIFF is a random-access binary the decoder must
// index before any row is addressable, and an HTML table is a DOM. Both are
// bounded in practice — BIFF caps at 65,536 rows, and nothing exports a
// million-row catalogue as HTML — so the grid is held and walked from memory.

// openXLS decodes a legacy Excel 97-2003 workbook.
//
// The previous importer refused these outright and told the vendor to re-save.
// Roughly a fifth of the files real Egyptian distributors send are BIFF, and
// telling a supplier to convert their file is how an import never happens.
func (b *Book) openXLS() (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = b.fallbackFromBinary(fmt.Errorf("panic in XLS parser: %v", r))
		}
	}()

	wb, err := xls.OpenReader(bytes.NewReader(b.content), "utf-8")
	if err != nil || wb == nil {
		wb, err = xls.OpenReader(bytes.NewReader(b.content), "windows-1256")
	}
	if err != nil || wb == nil {
		return b.fallbackFromBinary(err)
	}

	best, bestCells := -1, 0
	grids := make([][][]string, 0, wb.NumSheets())
	for i := 0; i < wb.NumSheets(); i++ {
		s := wb.GetSheet(i)
		if s == nil {
			grids = append(grids, nil)
			continue
		}
		grid := xlsSheetRows(s)
		grids = append(grids, grid)

		info := SheetInfo{Name: s.Name, Rows: len(grid)}
		for _, row := range grid {
			if len(row) > info.Width {
				info.Width = len(row)
			}
			for _, cell := range row {
				if CleanCell(cell) != "" {
					info.Cells++
				}
			}
		}
		b.source.Sheets = append(b.source.Sheets, info)
		if info.Cells > bestCells {
			bestCells, best = info.Cells, len(b.source.Sheets)-1
		}
	}

	if best < 0 || bestCells == 0 {
		return b.fallbackFromBinary(nil)
	}
	b.source.Sheets[best].Chosen = true
	b.source.Sheet = b.source.Sheets[best].Name
	b.rows = grids[best]
	b.finishGrid()
	return nil
}

// xlsColumnProbe is how far right a BIFF row is searched for content when the
// workbook's own row extent cannot be trusted, and the hard ceiling on how wide
// a legacy sheet is read.
const xlsColumnProbe = 64

// xlsSheetRows materialises one BIFF worksheet, keeping row positions intact.
func xlsSheetRows(s *xls.WorkSheet) (grid [][]string) {
	defer func() {
		if r := recover(); r != nil {
			// keep whatever rows were collected before panic
		}
	}()
	width := xlsSheetWidth(s)
	grid = make([][]string, 0, int(s.MaxRow)+1)
	for i := 0; i <= int(s.MaxRow); i++ {
		row := s.Row(i)
		if row == nil {
			grid = append(grid, nil)
			continue
		}
		w := max(row.LastCol(), width)
		if w > xlsColumnProbe {
			w = xlsColumnProbe
		}
		cells := make([]string, w)
		for c := 0; c < w; c++ {
			cells[c] = row.Col(c)
		}
		grid = append(grid, cells)
	}
	return grid
}

// xlsSheetWidth finds how many columns a BIFF worksheet actually uses.
//
// Row.LastCol reads the row's declared extent, and a real distributor export
// was found whose every row declares zero while holding four populated cells —
// the writer never filled the field in. Trusting it produced an empty grid and
// a refusal telling the supplier their file was corrupt, when it was not. So
// the declared extent is treated as a lower bound and the head of the sheet is
// searched rightwards for the last cell that actually holds something.
func xlsSheetWidth(s *xls.WorkSheet) int {
	width, probed := 0, 0
	for i := 0; i <= int(s.MaxRow) && probed < 60; i++ {
		row := s.Row(i)
		if row == nil {
			continue
		}
		probed++
		if lc := row.LastCol(); lc > width {
			width = lc
		}
		for c := xlsColumnProbe - 1; c >= width; c-- {
			if CleanCell(row.Col(c)) != "" {
				width = c + 1
				break
			}
		}
	}
	return width
}

// fallbackFromBinary handles a file whose OLE2 signature lied, which happens
// when an ERP writes an HTML table or a CSV and names it .xls.
func (b *Book) fallbackFromBinary(cause error) error {
	if looksLikeHTML(b.content) {
		b.format, b.source.Format = FormatHTML, FormatHTML
		return b.openHTML()
	}
	if cause != nil {
		return fmt.Errorf("تعذر قراءة ملف Excel 97-2003 (.xls) — قد يكون تالفاً أو محمياً. "+
			"يرجى فتحه في Excel وحفظه بصيغة «مصنف Excel (.xlsx)» ثم إعادة الرفع (%v)", cause)
	}
	return errors.New("ملف Excel 97-2003 (.xls) لا يحتوي على بيانات قابلة للقراءة. " +
		"يرجى فتحه في Excel وحفظه بصيغة «مصنف Excel (.xlsx)» ثم إعادة الرفع")
}

// openHTML reads the table an accounting package exported under a spreadsheet
// name. The largest table wins; a page whose real content is in the second
// table after a header banner is common.
func (b *Book) openHTML() error {
	doc, err := html.Parse(bytes.NewReader(b.content))
	if err != nil {
		return fmt.Errorf("تعذر قراءة جدول HTML: %w", err)
	}

	tables := collectTables(doc)
	best, bestCells := -1, 0
	for i, t := range tables {
		cells := 0
		for _, row := range t {
			for _, cell := range row {
				if CleanCell(cell) != "" {
					cells++
				}
			}
		}
		if cells > bestCells {
			bestCells, best = cells, i
		}
	}
	if best < 0 {
		return errors.New("هذا الملف صفحة HTML ولا يحتوي على أي جدول بيانات قابل للاستيراد")
	}

	b.rows = tables[best]
	b.source.Sheet = "HTML"
	b.source.Sheets = []SheetInfo{{Name: "HTML", Rows: len(b.rows), Cells: bestCells, Chosen: true}}
	b.finishGrid()
	return nil
}

// collectTables walks the document and returns each table as a grid.
func collectTables(root *html.Node) [][][]string {
	var tables [][][]string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && strings.EqualFold(n.Data, "table") {
			if grid := tableRows(n); len(grid) > 0 {
				tables = append(tables, grid)
			}
			// Nested tables are layout, not data; do not descend.
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(root)
	return tables
}

func tableRows(table *html.Node) [][]string {
	var grid [][]string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && strings.EqualFold(n.Data, "tr") {
			var row []string
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				if c.Type == html.ElementNode && (strings.EqualFold(c.Data, "td") || strings.EqualFold(c.Data, "th")) {
					row = append(row, nodeText(c))
				}
			}
			grid = append(grid, row)
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(table)
	return grid
}

func nodeText(n *html.Node) string {
	if n.Type == html.TextNode {
		return n.Data
	}
	var b strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		b.WriteString(nodeText(c))
	}
	return b.String()
}

// openDelimited decodes CSV and its tab, semicolon and pipe relatives.
func (b *Book) openDelimited(filename string) error {
	content, encoding := decodeText(b.content)
	b.source.Encoding = encoding

	// Anything still binary here is not a spreadsheet in any format we accept.
	// encoding/csv will happily split a compiled binary on commas and hand back
	// rows of control characters, which the row parser then imports as products.
	if isBinary(content) {
		return errors.New("تعذر التعرف على نوع الملف. الصيغ المدعومة هي Excel (.xlsx و .xls) و CSV والملفات النصية المفصولة")
	}

	delimiter := sniffDelimiter(content)
	rows, err := readCSVRows(content, delimiter)
	if err != nil {
		return fmt.Errorf("تعذر قراءة ملف CSV: %s", csvHint(err, filename))
	}
	if len(rows) == 0 {
		return errors.New("ملف CSV لا يحتوي على أي صفوف")
	}

	b.rows = rows
	b.source.Delimiter = string(delimiter)
	b.source.Sheet = ""
	b.finishGrid()
	return nil
}

// finishGrid records the extent of a fully decoded grid.
func (b *Book) finishGrid() {
	width := 0
	for _, row := range b.rows {
		if len(row) > width {
			width = len(row)
		}
	}
	pad(b.rows, width)
	b.source.TotalRows = len(b.rows)
	b.source.Estimated = false
}

// reloadSheet re-selects a worksheet in a format decoded whole. Only the legacy
// workbook has more than one, and its grids were discarded after selection, so
// the file is decoded again — a rare path on a small file.
func (b *Book) reloadSheet(name string) error {
	if b.format != FormatXLS {
		return nil
	}
	wb, err := xls.OpenReader(bytes.NewReader(b.content), "utf-8")
	if err != nil || wb == nil {
		return b.fallbackFromBinary(err)
	}
	for i := 0; i < wb.NumSheets(); i++ {
		s := wb.GetSheet(i)
		if s == nil || s.Name != name {
			continue
		}
		b.rows = xlsSheetRows(s)
		b.finishGrid()
		return nil
	}
	return fmt.Errorf("ورقة العمل «%s» غير موجودة في الملف", name)
}

// readCSVRows parses CSV while keeping every row at its true line number.
//
// encoding/csv drops blank lines entirely, so ReadAll returns a slice whose
// indices drift from the file's line numbers the moment a supplier leaves a gap
// between sections — and gaps are exactly what paginated exports are full of.
// Row numbers are promised to match the gutter in the vendor's own spreadsheet,
// so the blanks are restored from the line number the reader itself reports.
func readCSVRows(content []byte, delimiter rune) ([][]string, error) {
	reader := csv.NewReader(bytes.NewReader(content))
	reader.Comma = delimiter
	reader.LazyQuotes = true
	reader.TrimLeadingSpace = true
	// Without this, encoding/csv locks the field count to the first row's and
	// rejects the whole file at the first row that differs. Supplier CSVs are
	// ragged constantly — a trailing note, a short subtotal line — and failing
	// the upload over one of them loses the other nine thousand rows.
	reader.FieldsPerRecord = -1

	var rows [][]string
	for {
		record, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		if line, _ := reader.FieldPos(0); line > 0 {
			// Pad to where this record began, restoring swallowed blank lines.
			// The bound stops a corrupt line number from allocating wildly.
			if gap := line - 1 - len(rows); gap > 0 && gap <= 10000 {
				for len(rows) < line-1 {
					rows = append(rows, nil)
				}
			}
		}
		rows = append(rows, record)
	}
	return rows, nil
}

// csvHint turns encoding/csv's terse errors into something a vendor can act on
// without knowing what a bare quote is.
func csvHint(err error, filename string) string {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "quote"):
		return fmt.Sprintf("يوجد خطأ في علامات التنصيص داخل الملف (%s). "+
			"يرجى فتح الملف في Excel وحفظه من جديد بصيغة «CSV UTF-8»", msg)
	case strings.Contains(msg, "wrong number of fields"):
		return fmt.Sprintf("عدد الأعمدة غير متساوٍ بين الصفوف (%s)", msg)
	case filename != "":
		return fmt.Sprintf("%s (الملف: %s)", msg, filename)
	default:
		return msg
	}
}

// decodeText strips a byte-order mark and converts UTF-16 to UTF-8.
//
// Excel's "Unicode Text (*.txt)" export is UTF-16LE with a BOM, and a Windows
// admin who saves a sheet that way gets a file whose every second byte is NUL.
// Read as UTF-8 it yields one unusable column of mojibake.
func decodeText(content []byte) ([]byte, string) {
	switch {
	case bytes.HasPrefix(content, []byte{0xEF, 0xBB, 0xBF}):
		return content[3:], "UTF-8"
	case bytes.HasPrefix(content, []byte{0xFF, 0xFE}):
		return utf16ToUTF8(content[2:], true), "UTF-16LE"
	case bytes.HasPrefix(content, []byte{0xFE, 0xFF}):
		return utf16ToUTF8(content[2:], false), "UTF-16BE"
	}

	// A BOM-less UTF-16LE file still gives itself away: ASCII text puts a NUL
	// in every high byte.
	if len(content) >= 64 && !utf8.Valid(content) {
		nulOdd := 0
		for i := 1; i < 64; i += 2 {
			if content[i] == 0 {
				nulOdd++
			}
		}
		if nulOdd > 24 {
			return utf16ToUTF8(content, true), "UTF-16LE"
		}
	}
	return content, "UTF-8"
}

func utf16ToUTF8(b []byte, little bool) []byte {
	if len(b)%2 != 0 {
		b = b[:len(b)-1]
	}
	units := make([]uint16, 0, len(b)/2)
	for i := 0; i+1 < len(b); i += 2 {
		if little {
			units = append(units, uint16(b[i])|uint16(b[i+1])<<8)
		} else {
			units = append(units, uint16(b[i])<<8|uint16(b[i+1]))
		}
	}
	return []byte(string(utf16.Decode(units)))
}

// isBinary reports whether decoded content is not text. A NUL byte is decisive;
// beyond that a run of control characters or invalid UTF-8 settles it.
func isBinary(content []byte) bool {
	head := content
	if len(head) > 8192 {
		head = head[:8192]
	}
	if len(head) == 0 {
		return false
	}
	if bytes.IndexByte(head, 0) >= 0 {
		return true
	}
	control := 0
	for _, c := range head {
		if c < 0x20 && c != '\t' && c != '\n' && c != '\r' {
			control++
		}
	}
	if control*100 > len(head) {
		return true
	}
	// Truncating mid-rune at the 8 KB boundary would make valid UTF-8 look
	// invalid, so the tail is only trusted when it is the whole file.
	return len(content) <= 8192 && !utf8.Valid(content)
}

// sniffDelimiter picks the separator that splits the file most consistently.
//
// Counting occurrences on the first line alone misreads any file whose header
// contains a comma inside a quoted label, or whose first line is a title row.
// Consistency across several lines is a far better signal: the real delimiter
// yields the same field count row after row.
func sniffDelimiter(content []byte) rune {
	lines := sampleLines(content, 20)
	if len(lines) == 0 {
		return ','
	}

	bestDelim, bestScore := ',', -1
	for _, delim := range []rune{',', ';', '\t', '|'} {
		counts := make(map[int]int)
		for _, line := range lines {
			if n := countOutsideQuotes(line, delim); n > 0 {
				counts[n+1]++
			}
		}
		// Score the most common field count: how many lines agree, weighted by
		// how many columns that implies. Agreement matters more than width, so
		// a consistent two-column split beats an erratic nine-column one.
		for width, agree := range counts {
			if agree < 2 && len(lines) > 2 {
				continue // a delimiter that only ever worked once is noise
			}
			if score := agree*10 + width; score > bestScore {
				bestDelim, bestScore = delim, score
			}
		}
	}
	// A single column is legitimate — a bare list of product names — and reads
	// the same whichever separator is nominated.
	return bestDelim
}

func sampleLines(content []byte, limit int) []string {
	var out []string
	for _, raw := range bytes.Split(content, []byte("\n")) {
		line := strings.TrimRight(string(raw), "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		out = append(out, line)
		if len(out) >= limit {
			break
		}
	}
	return out
}

// countOutsideQuotes counts delimiters that are not inside a quoted field, so a
// product name such as "بانادول, أقراص" does not inflate the comma count.
func countOutsideQuotes(line string, delim rune) int {
	n, inQuote := 0, false
	for _, r := range line {
		switch {
		case r == '"':
			inQuote = !inQuote
		case r == delim && !inQuote:
			n++
		}
	}
	return n
}
