package sheet

import (
	"bytes"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/muhiya/dawa24-store/internal/shared/filesecurity"
)

// File decoding.
//
// The format is decided by content, never by the extension. Suppliers rename
// files freely — a .xlsx that is really a CSV, a .csv that is really a tab
// dump, a .xls that is really an HTML table saved by a decade-old ERP — and an
// importer that trusts the name fails on all three while blaming the wrong
// thing.
//
// Reading is two-phase on purpose. Peek touches only the head of the file, so
// the analysis screen can answer "what is in here" for a fifty-megabyte
// workbook without holding fifty megabytes of parsed rows. Walk then streams
// every row past a callback, so the import itself is bounded by one row rather
// than by the file.

// Format is the container a file turned out to be.
type Format string

const (
	FormatXLSX Format = "xlsx"
	FormatXLS  Format = "xls"
	FormatCSV  Format = "csv"
	FormatHTML Format = "html"
	// FormatXML2003 is Microsoft Office XML Spreadsheet, which arrives named
	// .xls and is neither BIFF nor HTML. See reader_xml2003.go.
	FormatXML2003 Format = "xml2003"
)

// Label renders a format for the review screen.
func (f Format) Label() string {
	switch f {
	case FormatXLSX:
		return "Excel (.xlsx)"
	case FormatXLS:
		return "Excel 97-2003 (.xls)"
	case FormatHTML:
		return "جدول HTML"
	case FormatXML2003:
		return "Excel XML 2003"
	default:
		return "نص مفصول (CSV)"
	}
}

// SheetInfo describes one worksheet found in a workbook.
type SheetInfo struct {
	Name string `json:"name"`
	// Rows is the worksheet's declared extent, which for a workbook whose
	// dimension record is missing or stale is an estimate.
	Rows int `json:"rows"`
	// Cells is how many non-empty cells were seen in the sampled head. It is
	// what decides which sheet holds the catalogue.
	Cells int `json:"cells"`
	// Width is the widest sampled row.
	Width  int  `json:"width"`
	Hidden bool `json:"hidden"`
	// Chosen marks the sheet that was read.
	Chosen bool `json:"chosen"`
}

// Source records how a file was decoded, so the review screen can tell the
// vendor which tab and which separator were used before they trust the numbers.
type Source struct {
	Format    Format      `json:"format"`
	Sheet     string      `json:"sheet,omitempty"`
	Sheets    []SheetInfo `json:"sheets,omitempty"`
	Delimiter string      `json:"delimiter,omitempty"`
	Encoding  string      `json:"encoding,omitempty"`
	SizeBytes int         `json:"size_bytes"`
	// TotalRows is the chosen sheet's row count. Estimated says whether it came
	// from the workbook's own dimension record rather than from counting.
	TotalRows int  `json:"total_rows"`
	Estimated bool `json:"estimated"`
}

// Preview is the head of a sheet plus what was learned decoding it.
type Preview struct {
	Source
	// Rows are the sampled rows, padded to Width and indexed from zero, where
	// index i is spreadsheet row i+1.
	Rows [][]string `json:"rows"`
	// Width is the widest row in the sample.
	Width int `json:"width"`
	// Truncated is true when the sheet has more rows than were sampled.
	Truncated bool `json:"truncated"`
}

// RowFunc receives one row during a Walk. index is zero-based and matches the
// spreadsheet's own row numbering minus one, blank rows included, so a finding
// raised here still points at a row in the vendor's copy of the file.
//
// Returning ErrStop ends the walk without an error.
type RowFunc func(index int, row []string) error

// ErrStop ends a Walk early from inside a RowFunc.
var ErrStop = errors.New("sheet: walk stopped")

// ErrEmpty is returned for a file with no readable rows at all.
var ErrEmpty = errors.New("sheet: no rows")

// Magic prefixes. A ZIP container is an OOXML workbook; the OLE2 compound
// document signature is the legacy BIFF .xls.
var (
	magicZIP  = []byte{'P', 'K', 0x03, 0x04}
	magicOLE2 = []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1}
)

// Detect reports which container the bytes are, ignoring the filename.
func Detect(content []byte) Format {
	switch {
	case bytes.HasPrefix(content, magicZIP):
		return FormatXLSX
	case bytes.HasPrefix(content, magicOLE2):
		return FormatXLS
	// Tested before HTML: an XML Spreadsheet satisfies the HTML sniff too, and
	// reading it as HTML yields a table with no rows rather than an error.
	case looksLikeXML2003(content):
		return FormatXML2003
	case looksLikeHTML(content):
		return FormatHTML
	}
	return FormatCSV
}

func looksLikeHTML(content []byte) bool {
	head := bytes.ToLower(bytes.TrimSpace(content))
	if len(head) > 2048 {
		head = head[:2048]
	}
	switch {
	case bytes.HasPrefix(head, []byte("<!doctype html")), bytes.HasPrefix(head, []byte("<html")):
		return true
	case bytes.HasPrefix(head, []byte("<table")):
		return true
	}
	return false
}

// Book is an opened file, ready to be sampled and then streamed.
//
// Open decodes only as much as the format forces: a workbook is unzipped and
// its sheet index read, but no worksheet body is parsed until Peek or Walk asks
// for one.
type Book struct {
	format  Format
	content []byte
	source  Source

	// xlsx holds the excelize handle for OOXML workbooks; nil otherwise.
	xlsx *xlsxBook
	// rows holds the fully decoded grid for formats that cannot stream — the
	// legacy BIFF workbook and the HTML table, both of which are parsed whole
	// by their libraries anyway.
	rows [][]string
}

// OpenConfig configures spreadsheet opening.
type OpenConfig struct {
	AllowEmails bool
}

// OpenOption modifies OpenConfig.
type OpenOption func(*OpenConfig)

// WithAllowEmails permits valid email addresses in spreadsheet cells (e.g. for team member imports).
func WithAllowEmails(allow bool) OpenOption {
	return func(c *OpenConfig) {
		c.AllowEmails = allow
	}
}

// Open decodes a file's container and index. filename is used only to improve
// error messages; it never decides the format.
func Open(content []byte, filename string, opts ...OpenOption) (*Book, error) {
	if len(content) == 0 {
		return nil, fmt.Errorf("الملف المرفوع فارغ (0 بايت). يرجى التأكد من اكتمال رفع الملف ثم المحاولة مرة أخرى")
	}

	var cfg OpenConfig
	for _, opt := range opts {
		opt(&cfg)
	}

	var secOpts []filesecurity.Option
	if cfg.AllowEmails {
		secOpts = append(secOpts, filesecurity.WithAllowEmails(true))
	}
	if err := filesecurity.ValidateSpreadsheetSecurity(content, filename, secOpts...); err != nil {
		return nil, err
	}

	b := &Book{format: Detect(content), content: content}
	b.source.Format = b.format
	b.source.SizeBytes = len(content)

	var err error
	switch b.format {
	case FormatXLSX:
		err = b.openXLSX()
	case FormatXLS:
		err = b.openXLS()
	case FormatHTML:
		err = b.openHTML()
	case FormatXML2003:
		err = b.openXML2003()
	default:
		err = b.openDelimited(filename)
	}
	if err != nil {
		return nil, err
	}
	return b, nil
}

// Close releases anything the decoder is holding. Safe on a nil Book.
func (b *Book) Close() error {
	if b == nil || b.xlsx == nil {
		return nil
	}
	return b.xlsx.close()
}

// Source reports how the file was decoded.
func (b *Book) Source() Source { return b.source }

// Sheets lists the worksheets that held data.
func (b *Book) Sheets() []SheetInfo { return b.source.Sheets }

// Use forces a worksheet by name, for a workbook whose densest tab is not the
// one the vendor meant. An unknown name is an error rather than a silent
// fallback: reading the wrong tab is exactly the mistake this guards against.
func (b *Book) Use(name string) error {
	if name == "" || name == b.source.Sheet {
		return nil
	}
	for i := range b.source.Sheets {
		if b.source.Sheets[i].Name != name {
			continue
		}
		for j := range b.source.Sheets {
			b.source.Sheets[j].Chosen = j == i
		}
		b.source.Sheet = name
		b.source.TotalRows = b.source.Sheets[i].Rows
		if b.xlsx == nil {
			return b.reloadSheet(name)
		}
		b.xlsx.sheet = name
		return nil
	}
	return fmt.Errorf("ورقة العمل «%s» غير موجودة في الملف", name)
}

// Peek returns the first maxRows rows of the chosen sheet.
func (b *Book) Peek(maxRows int) (*Preview, error) {
	if maxRows <= 0 {
		maxRows = DefaultPeekRows
	}
	p := &Preview{Source: b.source}
	err := b.Walk(func(index int, row []string) error {
		if index >= maxRows {
			p.Truncated = true
			return ErrStop
		}
		clean := make([]string, len(row))
		for i, cell := range row {
			clean[i] = CleanCell(cell)
		}
		p.Rows = append(p.Rows, clean)
		if len(clean) > p.Width {
			p.Width = len(clean)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(p.Rows) == 0 {
		return nil, ErrEmpty
	}
	// Trailing blank rows in the sample say nothing and only push the real data
	// out of the window the analyser looks at.
	for len(p.Rows) > 0 && isBlank(p.Rows[len(p.Rows)-1]) {
		p.Rows = p.Rows[:len(p.Rows)-1]
	}
	pad(p.Rows, p.Width)
	if !p.Truncated {
		p.TotalRows = len(p.Rows)
		p.Estimated = false
		p.Source.TotalRows = len(p.Rows)
		p.Source.Estimated = false
	}
	return p, nil
}

// DefaultPeekRows is how much of a sheet the analysis stage reads. It is large
// enough to see past a paginated export's first repeated header and to profile
// a column's values honestly, and small enough that a huge workbook is analysed
// in well under a second.
const DefaultPeekRows = 400

// Walk streams every row of the chosen sheet past fn.
func (b *Book) Walk(fn RowFunc) error {
	var err error
	if b.xlsx != nil {
		err = b.xlsx.walk(fn)
	} else {
		err = walkRows(b.rows, fn)
	}
	if errors.Is(err, ErrStop) {
		return nil
	}
	return err
}

func walkRows(rows [][]string, fn RowFunc) error {
	for i, row := range rows {
		if err := fn(i, row); err != nil {
			return err
		}
	}
	return nil
}

func isBlank(row []string) bool {
	for _, cell := range row {
		if CleanCell(cell) != "" {
			return false
		}
	}
	return true
}

// pad widens every row to the same length.
//
// Decoders trim trailing empty cells, so a row whose last three columns are
// blank comes back short. Padding once here makes every row addressable by
// column index instead of forcing a bounds check at every read — and a mapped
// column beyond a short row's end would otherwise read as absent rather than
// empty, which are different things.
func pad(rows [][]string, width int) {
	for i, row := range rows {
		if len(row) >= width {
			continue
		}
		grown := make([]string, width)
		copy(grown, row)
		rows[i] = grown
	}
}

// parseDimensionRows reads the trailing row number out of an OOXML dimension
// reference such as "A1:H9020".
func parseDimensionRows(dim string) int {
	_, end, ok := strings.Cut(dim, ":")
	if !ok {
		return 0
	}
	digits := strings.TrimLeftFunc(end, func(r rune) bool { return r < '0' || r > '9' })
	n, err := strconv.Atoi(digits)
	if err != nil {
		return 0
	}
	return n
}
