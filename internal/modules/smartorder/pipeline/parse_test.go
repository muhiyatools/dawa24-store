package pipeline

import (
	"testing"

	"github.com/muhiya/dawa24-store/internal/shared/productmatch"
)

func TestDetectHeadersFindsRowBeneathBanner(t *testing.T) {
	// The layout the spec calls out: a title, a date, a blank, then the headers.
	rows := [][]string{
		{"صيدلية النور", "", "", ""},
		{"طلب توريد 2026/08", "", "", ""},
		{"", "", "", ""},
		{"اسم الصنف", "كود الصنف", "الباركود", "العدد"},
		{"بانادول", "P1", "622", "5"},
	}
	headerRow, mapping, confidence := detectHeaders(rows)

	if headerRow != 3 {
		t.Fatalf("expected the header on row 3, got %d", headerRow)
	}
	if len(mapping) != 4 {
		t.Fatalf("expected all four columns mapped, got %d: %v", len(mapping), mapping)
	}
	if mapping[0] != "product_name" {
		t.Errorf("column 0 should be product_name, got %q", mapping[0])
	}
	// "العدد" is the alias the spec names explicitly.
	if mapping[3] != "quantity" {
		t.Errorf("العدد should map to quantity, got %q", mapping[3])
	}
	if confidence["product_name"] <= 0 {
		t.Error("expected a confidence for product_name")
	}
}

func TestDetectHeadersHandlesEnglishHeaders(t *testing.T) {
	rows := [][]string{{"Item Name", "SKU", "Barcode", "Qty"}}
	_, mapping, _ := detectHeaders(rows)

	want := map[int]string{0: "product_name", 1: "sku", 2: "barcode", 3: "quantity"}
	for col, field := range want {
		if mapping[col] != field {
			t.Errorf("column %d: expected %q, got %q", col, field, mapping[col])
		}
	}
}

func TestWeakHeaderGuessesAreNotOffered(t *testing.T) {
	// A confident-looking wrong guess is worse than none: it is the failure the
	// mapping screen exists to catch.
	rows := [][]string{{"ملاحظات", "xyz123", "???"}}
	_, mapping, _ := detectHeaders(rows)
	for col, field := range mapping {
		t.Errorf("column %d should not have been mapped, got %q", col, field)
	}
}

func TestBestFieldForRecognisesArabicAliases(t *testing.T) {
	cases := map[string]string{
		"الصنف":     "product_name",
		"المنتج":    "product_name",
		"الكمية":    "quantity",
		"العدد":     "quantity",
		"الباركود":  "barcode",
		"كود الصنف": "sku",
	}
	for header, want := range cases {
		got, score := bestFieldFor(productmatch.NormalizeText(header))
		if got != want {
			t.Errorf("%q: expected %q, got %q (score %.2f)", header, want, got, score)
		}
	}
}

func TestIsSummaryRowCatchesFooterLabels(t *testing.T) {
	for _, s := range []string{"المجموع", "الإجمالي", "Total", "SUM"} {
		if !isSummaryForTest(s) {
			t.Errorf("%q should be recognised as a footer label, not a product", s)
		}
	}
	for _, s := range []string{"بانادول", "Total Care Shampoo"} {
		if isSummaryForTest(s) {
			t.Errorf("%q is a product name and must not be dropped", s)
		}
	}
}

func TestMaxRowsIsTheSpecifiedCap(t *testing.T) {
	if MaxRows != 10000 {
		t.Fatalf("FR-002 commits to 10,000 rows, got %d", MaxRows)
	}
}

func TestRawOfKeepsOriginalCells(t *testing.T) {
	// Whatever the mapping decided, the buyer must be able to see what they
	// actually uploaded.
	raw := rawOf([]string{" بانادول ", "", "P1"})
	if raw["0"] != "بانادول" {
		t.Errorf("expected the trimmed original, got %q", raw["0"])
	}
	if _, present := raw["1"]; present {
		t.Error("blank cells should not be stored")
	}
	if raw["2"] != "P1" {
		t.Errorf("expected P1, got %q", raw["2"])
	}
}

func TestCellIsSafeForShortRows(t *testing.T) {
	// A ragged spreadsheet must not panic the importer.
	row := []string{"a"}
	if got := cell(row, 5, true); got != "" {
		t.Errorf("out-of-range column should be empty, got %q", got)
	}
	if got := cell(row, 0, false); got != "" {
		t.Errorf("an unmapped column should be empty, got %q", got)
	}
}

func TestIsRepeatedHeader(t *testing.T) {
	headers := []string{"Item No.", "Item Description", "Preferred Vendor", "Qty"}
	repeated := []string{"Item No.", "Item Description", "Preferred Vendor", "Qty"}
	repeatedCase := []string{"item no.", "item description", "Preferred Vendor", ""}
	dataRow := []string{"10104677", "سيروبايب بلسم للشعر 300 مل", "Parkville", "10"}

	if !isRepeatedHeader(repeated, headers) {
		t.Errorf("exact repeated header row should be detected")
	}
	if !isRepeatedHeader(repeatedCase, headers) {
		t.Errorf("case-insensitive repeated header row should be detected")
	}
	if isRepeatedHeader(dataRow, headers) {
		t.Errorf("data row must not be treated as repeated header")
	}
}
