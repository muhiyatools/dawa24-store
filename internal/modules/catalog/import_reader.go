package catalog

import (
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/xuri/excelize/v2"

	"github.com/muhiya/dawa24-store/internal/shared/sheet"
)

// File decoding for spreadsheet import.
//
// The format is decided by content, not by the extension. Suppliers rename
// files freely — a .xlsx that is really a CSV, a .csv that is really a tab
// dump, a .xls that is really an HTML table saved by an old ERP — and an
// importer that trusts the extension fails on all three with an error that
// blames the wrong thing.

// SheetData is a decoded rectangle of cells plus what was learned decoding it.
type SheetData struct {
	Rows []([]string)
	// Sheet is the worksheet the rows came from, empty for CSV.
	Sheet string
	// SheetsSkipped lists other worksheets that held data and were not used.
	// A supplier who put January on one tab and February on another needs to be
	// told only one was read.
	SheetsSkipped []string
	// Format is "xlsx" or "csv".
	Format string
	// Delimiter is the character CSV was split on, empty for Excel.
	Delimiter string
	// Width is the widest row, after ragged rows were padded.
	Width int
}

// Magic prefixes. A ZIP container is an OOXML workbook; the OLE2 compound
// document signature is the legacy BIFF .xls that excelize cannot read.
var (
	magicZIP  = []byte{'P', 'K', 0x03, 0x04}
	magicOLE2 = []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1}
)

// ErrLegacyXLS used to be returned for a real BIFF .xls workbook, which this
// importer refused outright.
//
// It is kept only so a caller that still tests for it compiles, and it is no
// longer produced by anything: legacy .xls is read. A workbook so damaged that
// the decoder cannot recover it returns a plain message naming that, because
// "re-save it as .xlsx" is now advice about a broken file rather than about an
// unsupported format.
//
// Deprecated: legacy .xls is supported. Nothing returns this.
var ErrLegacyXLS = errors.New("catalog: legacy .xls workbook")

// ReadSpreadsheet decodes an uploaded file into rows.
//
// Every failure returns an Arabic message naming the actual problem. The old
// importer surfaced "تنسيق الملف غير صالح: <nil>" — the error was nil because
// the check was `err != nil || len(records) < 1` and an empty file took the
// second branch — which told the admin nothing at all.
func ReadSpreadsheet(content []byte, filename string) (*SheetData, error) {
	if len(content) == 0 {
		return nil, errors.New(i18n.T("ar", "err.empty_file"))
	}

	switch {
	case bytes.HasPrefix(content, magicZIP):
		return readExcel(content)

	case bytes.HasPrefix(content, []byte("%PDF")):
		return nil, errors.New(i18n.T("ar", "err.pdf_unsupported"))

	case bytes.HasPrefix(content, magicOLE2), looksLikeHTML(content):
		// Legacy BIFF .xls, and the HTML table a decade-old ERP writes and
		// names .xls. Both used to be refused outright with an instruction to
		// re-save the file, and roughly a fifth of what Egyptian distributors
		// actually send is one or the other — telling a supplier to convert
		// their file is how an import never happens.
		//
		// internal/shared/sheet has decoded both for the vendor import and the
		// smart order all along. This importer had its own reader and its own
		// refusal, which is precisely the drift a shared reader exists to stop:
		// the same file was importable through one screen and rejected by
		// another.
		return readViaSheet(content, filename)
	}

	return readDelimited(content, filename)
}

// readViaSheet decodes through the shared sniffing reader.
//
// It returns the same SheetData the local decoders do, so nothing downstream
// needs to know which reader ran — the row parser, the column mapper and the
// review screen all see one shape.
func readViaSheet(content []byte, filename string) (*SheetData, error) {
	book, err := sheet.Open(content, filename)
	if err != nil {
		return nil, err
	}
	defer func() { _ = book.Close() }()

	src := book.Source()
	data := &SheetData{Sheet: src.Sheet, Format: string(src.Format), Delimiter: src.Delimiter}
	for _, info := range src.Sheets {
		if !info.Chosen && info.Cells > 0 {
			data.SheetsSkipped = append(data.SheetsSkipped, info.Name)
		}
	}

	err = book.Walk(func(_ int, row []string) error {
		if len(data.Rows) >= maxSheetRows {
			return errSheetTooLarge
		}
		out := make([]string, len(row))
		for i, cell := range row {
			out[i] = sheet.CleanCell(cell)
		}
		data.Rows = append(data.Rows, out)
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(data.Rows) == 0 {
		return nil, errors.New(i18n.T("ar", "err.no_readable_rows"))
	}
	normalizeWidth(data)
	return data, nil
}

func looksLikeHTML(content []byte) bool {
	head := bytes.ToLower(bytes.TrimSpace(content))
	if len(head) > 512 {
		head = head[:512]
	}
	return bytes.HasPrefix(head, []byte("<!doctype html")) ||
		bytes.HasPrefix(head, []byte("<html")) ||
		bytes.HasPrefix(head, []byte("<?xml")) && bytes.Contains(head, []byte("<Workbook"))
}

// Decoding caps. An .xlsx is a ZIP of XML; 32 MB of highly compressible
// workbook decompresses to gigabytes, and GetRows materialises a sheet whole.
// These bound the decode so one malicious — or merely corrupt — upload cannot
// exhaust the process's memory. A real distributor file is tens of thousands
// of rows; both caps are an order of magnitude past that.
const (
	maxSheetRows  = 200_000
	maxSheetCells = 5_000_000
)

// errSheetTooLarge is the refusal for a workbook past the decode caps.
var errSheetTooLarge = errors.New(
	i18n.T("ar", "err.row_limit_exceeded"))

// readExcel picks the worksheet that actually holds the catalogue.
//
// "The sheet with the most rows" is not good enough: exports routinely carry a
// trailing sheet of a thousand blank formatted rows, and a summary tab whose
// twenty rows are the real data. Density — cells that contain something — picks
// the right one, and hidden sheets are skipped because a hidden sheet is
// something the supplier deliberately set aside.
//
// Sheets are scored through the streaming iterator rather than GetRows, so a
// bomb workbook dies against the row cap mid-decode instead of after it has
// been fully materialised in memory. Only the winning sheet is read into the
// slice the parser consumes.
func readExcel(content []byte) (*SheetData, error) {
	f, err := excelize.OpenReader(bytes.NewReader(content))
	if err != nil {
		return nil, fmt.Errorf("تعذر فتح ملف Excel — قد يكون الملف تالفاً أو محمياً بكلمة مرور (%v)", err)
	}
	defer func() { _ = f.Close() }()

	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		return nil, errors.New("ملف Excel لا يحتوي على أي أوراق عمل")
	}

	type score struct {
		name  string
		rows  int
		cells int
	}
	var best score
	var withData []string

	for _, name := range sheets {
		if visible, vErr := f.GetSheetVisible(name); vErr == nil && !visible {
			continue
		}
		rows, cells, sErr := scoreSheet(f, name)
		if sErr != nil {
			return nil, sErr
		}
		if rows == 0 || cells == 0 {
			continue
		}
		withData = append(withData, name)
		if cells > best.cells {
			best = score{name: name, rows: rows, cells: cells}
		}
	}

	if best.cells == 0 {
		return nil, errors.New("جميع أوراق العمل في ملف Excel فارغة. يرجى التأكد من أن البيانات محفوظة في الملف قبل رفعه")
	}

	rows, err := readRowsCapped(f, best.name)
	if err != nil {
		return nil, err
	}

	data := &SheetData{Rows: rows, Sheet: best.name, Format: "xlsx"}
	for _, name := range withData {
		if name != best.name {
			data.SheetsSkipped = append(data.SheetsSkipped, name)
		}
	}
	normalizeWidth(data)
	return data, nil
}

// scoreSheet counts a sheet's non-empty cells through the streaming iterator,
// enforcing the decode caps as it goes.
func scoreSheet(f *excelize.File, name string) (rows, cells int, err error) {
	iter, err := f.Rows(name)
	if err != nil {
		return 0, 0, nil // unreadable sheet: skip it, like the empty ones
	}
	defer func() { _ = iter.Close() }()

	for iter.Next() {
		rows++
		if rows > maxSheetRows {
			return 0, 0, errSheetTooLarge
		}
		row, err := iter.Columns()
		if err != nil {
			continue
		}
		for _, cell := range row {
			if CleanCellString(cell) != "" {
				cells++
			}
		}
		if cells > maxSheetCells {
			return 0, 0, errSheetTooLarge
		}
	}
	return rows, cells, nil
}

// readRowsCapped reads one sheet whole, refusing past the row cap.
func readRowsCapped(f *excelize.File, name string) ([][]string, error) {
	iter, err := f.Rows(name)
	if err != nil {
		return nil, fmt.Errorf("تعذر قراءة ورقة العمل «%s» من ملف Excel (%v)", name, err)
	}
	defer func() { _ = iter.Close() }()

	rows := make([][]string, 0, min(bestSheetCapacity, maxSheetRows))
	for iter.Next() {
		if len(rows) >= maxSheetRows {
			return nil, errSheetTooLarge
		}
		record, err := iter.Columns()
		if err != nil {
			continue
		}
		rows = append(rows, record)
	}
	if err := iter.Error(); err != nil {
		return nil, fmt.Errorf("تعذرت قراءة ملف Excel (%v)", err)
	}
	return rows, nil
}

// bestSheetCapacity is the initial allocation for the winning sheet: big enough
// for the ordinary case, small enough that a hostile file does not get a huge
// buffer up front.
const bestSheetCapacity = 4096

// readDelimited decodes CSV and its tab/semicolon/pipe relatives.
func readDelimited(content []byte, filename string) (*SheetData, error) {
	content = decodeText(content)

	// Anything still binary at this point is not a spreadsheet in any format we
	// recognise. encoding/csv will happily split a compiled binary on commas and
	// hand back rows of control characters, which the row parser then imports as
	// products — an admin who picks the wrong file deserves a refusal, not a
	// catalogue full of mojibake.
	if isBinary(content) {
		return nil, errors.New("تعذر التعرف على نوع الملف. الصيغ المدعومة هي Excel (.xlsx) و CSV و الملفات النصية المفصولة بفواصل")
	}

	delimiter, err := sniffDelimiter(content)
	if err != nil {
		return nil, err
	}

	rows, err := readCSVRows(content, delimiter)
	if err != nil {
		return nil, fmt.Errorf("تعذر قراءة ملف CSV: %s", csvErrorHint(err, filename))
	}
	if len(rows) == 0 {
		return nil, errors.New("ملف CSV لا يحتوي على أي صفوف")
	}

	data := &SheetData{Rows: rows, Format: "csv", Delimiter: string(delimiter)}
	normalizeWidth(data)
	return data, nil
}

// readCSVRows parses CSV while keeping every row at its true line number.
//
// encoding/csv drops blank lines entirely, so ReadAll returns a slice whose
// indices drift from the file's line numbers the moment a supplier leaves a gap
// between sections — and gaps are exactly what paginated exports are full of.
// The import report promises that its row numbers match the gutter in the
// admin's own spreadsheet, so the blanks are restored here from the line number
// the reader itself reports.
func readCSVRows(content []byte, delimiter rune) ([][]string, error) {
	reader := csv.NewReader(bytes.NewReader(content))
	reader.Comma = delimiter
	reader.LazyQuotes = true
	reader.TrimLeadingSpace = true
	// Without this, encoding/csv locks the field count to whatever the first row
	// had and rejects the whole file at the first row that differs. Supplier
	// CSVs are ragged constantly — a trailing note, a short subtotal line — and
	// the previous importer failed the entire upload with "record on line 3:
	// wrong number of fields" rather than importing the other 9,000 rows.
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

		// FieldPos reports where this record began. Padding to it restores the
		// blank lines the reader swallowed. A record that legitimately spans
		// several lines through a quoted newline leaves blank rows behind it,
		// which parse as empty and keep every later row number honest.
		//
		// The gap cap exists because a corrupt or hostile file can claim any
		// line number. Rather than padding past it — unbounded memory — or
		// skipping silently, which would shift every later row reference in
		// the report onto the wrong spreadsheet row, an over-wide gap is a
		// refusal the admin can act on.
		if line, _ := reader.FieldPos(0); line > 0 {
			gap := line - 1 - len(rows)
			if gap > maxBlankLineGap {
				return nil, fmt.Errorf(
					"يحتوي الملف على فراغ يتجاوز %d سطراً عند السطر %d، ولا يمكن ضمان صحة أرقام الصفوف بعده. يرجى إزالة الأسطر الفارغة الزائدة وإعادة الرفع",
					maxBlankLineGap, line)
			}
			for len(rows) < line-1 {
				rows = append(rows, nil)
			}
		}
		rows = append(rows, record)
	}
	return rows, nil
}

// maxBlankLineGap is the widest run of blank lines the reader will reconstruct.
const maxBlankLineGap = 1000

// csvErrorHint turns encoding/csv's terse errors into something an admin can
// act on without knowing what a "bare quote" is.
func csvErrorHint(err error, filename string) string {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "quote"):
		return fmt.Sprintf("يوجد خطأ في علامات التنصيص داخل الملف (%s). "+
			"يرجى فتح الملف في Excel وحفظه من جديد بصيغة «CSV UTF-8»", msg)
	case strings.Contains(msg, "wrong number of fields"):
		return fmt.Sprintf("عدد الأعمدة غير متساوٍ بين الصفوف (%s)", msg)
	default:
		if filename != "" {
			return fmt.Sprintf("%s (الملف: %s)", msg, filename)
		}
		return msg
	}
}

// decodeText strips a byte-order mark and converts UTF-16 to UTF-8.
//
// Excel's "Unicode Text (*.txt)" export is UTF-16LE with a BOM, and a Windows
// admin who saves a sheet that way gets a file whose every second byte is NUL.
// Reading it as UTF-8 yields one unusable column of mojibake, so it is worth the
// few lines to convert properly.
func decodeText(content []byte) []byte {
	switch {
	case bytes.HasPrefix(content, []byte{0xEF, 0xBB, 0xBF}):
		return content[3:]

	case bytes.HasPrefix(content, []byte{0xFF, 0xFE}):
		return utf16ToUTF8(content[2:], true)

	case bytes.HasPrefix(content, []byte{0xFE, 0xFF}):
		return utf16ToUTF8(content[2:], false)
	}

	// A BOM-less UTF-16LE file still gives itself away: ASCII text produces a
	// NUL in every high byte.
	if len(content) >= 64 && !utf8.Valid(content) {
		nulOdd := 0
		for i := 1; i < 64; i += 2 {
			if content[i] == 0 {
				nulOdd++
			}
		}
		if nulOdd > 24 {
			return utf16ToUTF8(content, true)
		}
	}
	return content
}

// isBinary reports whether decoded content is not text.
//
// A NUL byte is decisive — no text encoding this importer accepts produces one
// after decoding — and beyond that a run of control characters or invalid UTF-8
// settles it. Tab, newline and carriage return are text and are not counted.
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
	for _, b := range head {
		if b < 0x20 && b != '\t' && b != '\n' && b != '\r' {
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

// sniffDelimiter picks the separator that splits the file most consistently.
//
// Counting occurrences on the first line alone — what the previous importer did
// — misreads any file whose header contains a comma inside a quoted label, or
// whose first line is a title row. Consistency across several lines is a far
// better signal: the real delimiter yields the same field count row after row.
func sniffDelimiter(content []byte) (rune, error) {
	lines := sampleLines(content, 20)
	if len(lines) == 0 {
		return 0, errors.New("ملف CSV فارغ أو لا يحتوي على أسطر بيانات")
	}

	type result struct {
		delim rune
		score int
		width int
	}
	best := result{delim: ',', score: -1}

	for _, delim := range []rune{',', ';', '\t', '|'} {
		counts := make(map[int]int)
		for _, line := range lines {
			if n := countOutsideQuotes(line, delim); n > 0 {
				counts[n+1]++
			}
		}
		// Score the most common field count: how many lines agree, weighted by
		// how many columns that implies. Agreement matters more than width, so a
		// two-column consistent split beats a nine-column erratic one.
		for width, agree := range counts {
			score := agree*10 + width
			if agree < 2 && len(lines) > 2 {
				continue // a delimiter that only ever worked once is noise
			}
			if score > best.score {
				best = result{delim: delim, score: score, width: width}
			}
		}
	}

	if best.score < 0 {
		// A single column is legitimate: a bare list of product names. Treat it
		// as comma-separated and let row parsing decide whether it is usable.
		return ',', nil
	}
	return best.delim, nil
}

// sampleLines returns up to max non-empty lines from the head of the file.
func sampleLines(content []byte, max int) []string {
	var out []string
	for _, raw := range bytes.Split(content, []byte("\n")) {
		line := strings.TrimRight(string(raw), "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		out = append(out, line)
		if len(out) >= max {
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

// normalizeWidth pads every row to the width of the widest one.
//
// excelize trims trailing empty cells, so a row whose last three columns are
// blank comes back short. Row parsing then has to bounds-check every access,
// and a mapped column beyond the short row's end silently reads as absent.
// Padding once here makes every row addressable by column index.
func normalizeWidth(d *SheetData) {
	width := 0
	for _, row := range d.Rows {
		if len(row) > width {
			width = len(row)
		}
	}
	d.Width = width
	for i, row := range d.Rows {
		if len(row) < width {
			padded := make([]string, width)
			copy(padded, row)
			d.Rows[i] = padded
		}
	}
}

// ParseUploadedSpreadsheet reads Excel and CSV files into a 2D string slice.
//
// Retained as the simple entry point for callers that need only the cells;
// ReadSpreadsheet additionally reports which sheet and delimiter were chosen,
// which the import report shows so an admin can confirm the right tab was read.
func ParseUploadedSpreadsheet(content []byte, filename string) ([][]string, error) {
	data, err := ReadSpreadsheet(content, filename)
	if err != nil {
		return nil, err
	}
	return data.Rows, nil
}
