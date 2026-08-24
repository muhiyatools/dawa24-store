package catalog_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
)

func TestReadSpreadsheetTolerantesRaggedCSV(t *testing.T) {
	// encoding/csv locks the field count to the first record unless told
	// otherwise, and the importer used to let it: one short row failed the whole
	// upload with "record on line 3: wrong number of fields", discarding
	// thousands of good rows with it.
	csv := "اسم الصنف,كود الصنف,السعر\n" +
		"بانادول,PAN-1,25.00\n" +
		"كتافلام,PAN-2\n" +
		"اوجمنتين,PAN-3,115.00,ملاحظة إضافية\n"

	data, err := catalog.ReadSpreadsheet([]byte(csv), "list.csv")
	if err != nil {
		t.Fatalf("ragged CSV rejected: %v", err)
	}
	if len(data.Rows) != 4 {
		t.Fatalf("expected 4 rows, got %d", len(data.Rows))
	}
	// Every row is padded to the widest, so a mapped column index is always
	// addressable without a bounds check at every read site.
	for i, row := range data.Rows {
		if len(row) != data.Width {
			t.Errorf("row %d has width %d, want %d", i, len(row), data.Width)
		}
	}
}

func TestReadSpreadsheetSniffsDelimiters(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
		columns int
	}{
		{"comma", "a,b,c\n1,2,3\n4,5,6\n", ",", 3},
		{"semicolon", "a;b;c\n1;2;3\n4;5;6\n", ";", 3},
		{"tab", "a\tb\tc\n1\t2\t3\n4\t5\t6\n", "\t", 3},
		{"pipe", "a|b|c\n1|2|3\n4|5|6\n", "|", 3},
		// A comma inside a quoted product name must not outvote the real
		// delimiter — counting the header line alone got this wrong.
		{"quoted comma with semicolons", "\"اسم, تجاري\";كود;سعر\n\"بانادول, أقراص\";P1;25\n\"كتافلام, فوار\";P2;42\n", ";", 3},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			data, err := catalog.ReadSpreadsheet([]byte(tc.content), "f.csv")
			if err != nil {
				t.Fatalf("read failed: %v", err)
			}
			if data.Delimiter != tc.want {
				t.Errorf("delimiter = %q, want %q", data.Delimiter, tc.want)
			}
			if data.Width != tc.columns {
				t.Errorf("width = %d, want %d", data.Width, tc.columns)
			}
		})
	}
}

func TestReadSpreadsheetStripsBOM(t *testing.T) {
	data, err := catalog.ReadSpreadsheet([]byte("\xEF\xBB\xBFاسم الصنف,السعر\nبانادول,25\n"), "f.csv")
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if got := data.Rows[0][0]; got != "اسم الصنف" {
		t.Errorf("BOM survived into first header cell: %q", got)
	}
}

func TestReadSpreadsheetRejectsUnusableFiles(t *testing.T) {
	tests := []struct {
		name    string
		content []byte
		wants   string
	}{
		{"empty", nil, "فارغ"},
		{"legacy xls", append([]byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1}, make([]byte, 64)...), ".xlsx"},
		{"html", []byte("<html><body><table></table></body></html>"), "HTML"},
		{"pdf", []byte("%PDF-1.7\nrubbish"), "PDF"},
		{"binary", append([]byte("MZ\x90\x00"), make([]byte, 600)...), "تعذر التعرف"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := catalog.ReadSpreadsheet(tc.content, "upload.dat")
			if err == nil {
				t.Fatal("expected a refusal, got none")
			}
			if !strings.Contains(err.Error(), tc.wants) {
				t.Errorf("message %q does not mention %q", err.Error(), tc.wants)
			}
		})
	}
}

func TestReadSpreadsheetNamesLegacyXLS(t *testing.T) {
	// The distinct error lets a caller offer the specific fix — resave as .xlsx
	// — rather than a generic "invalid format".
	_, err := catalog.ReadSpreadsheet(
		append([]byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1}, make([]byte, 64)...), "old.xls")
	if !errors.Is(err, catalog.ErrLegacyXLS) {
		t.Fatalf("expected ErrLegacyXLS, got %v", err)
	}
}

func TestReadSpreadsheetDecodesUTF16(t *testing.T) {
	// Excel's "Unicode Text" export. Read as UTF-8 it is one column of mojibake.
	utf16le := []byte{0xFF, 0xFE}
	for _, r := range "اسم,سعر\nبانادول,25\n" {
		utf16le = append(utf16le, byte(r), byte(r>>8))
	}

	data, err := catalog.ReadSpreadsheet(utf16le, "unicode.txt")
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if data.Rows[0][0] != "اسم" {
		t.Errorf("UTF-16 not decoded: %q", data.Rows[0][0])
	}
}

func TestReadSpreadsheetKeepsBlankLinesAtTheirRowNumbers(t *testing.T) {
	// encoding/csv drops blank lines, which silently shifts every later row
	// number away from the gutter in the admin's own spreadsheet — and the
	// import report promises those numbers match.
	csv := "اسم الصنف,السعر\n\n\nبانادول,25.00\n"

	data, err := catalog.ReadSpreadsheet([]byte(csv), "f.csv")
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if len(data.Rows) != 4 {
		t.Fatalf("got %d rows, want 4 with the two blanks preserved", len(data.Rows))
	}
	if got := catalog.CleanCellString(data.Rows[3][0]); got != "بانادول" {
		t.Errorf("row 4 holds %q, want بانادول — the product moved up by the dropped blanks", got)
	}
}
