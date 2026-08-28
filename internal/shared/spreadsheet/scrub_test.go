package spreadsheet

import "testing"

func TestScrubCellRemovesPostgresHostileBytes(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"nul in the middle", "بانادول\x00إكسترا", "بانادولإكسترا"},
		{"trailing nul padding", "SKU-1234\x00\x00\x00", "SKU-1234"},
		{"lone nul", "\x00", ""},
		{"other c0 control", "abc\x01\x1fdef", "abcdef"},
		{"tab becomes a space", "abc\tdef", "abc def"},
		{"newline becomes a space", "abc\ndef", "abc def"},
		{"clean text is untouched", "بانادول إكسترا", "بانادول إكسترا"},
		{"whitespace trimmed", "  abc  ", "abc"},
		{"empty stays empty", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ScrubCell(c.in); got != c.want {
				t.Errorf("ScrubCell(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestReadRowsScrubsEveryCell(t *testing.T) {
	// A CSV standing in for the legacy .xls: the NUL is what PostgreSQL
	// rejects with "invalid byte sequence for encoding UTF8: 0x00".
	rows, err := ReadRows([]byte("name,price\nبانادول\x00,120.50\n"))
	if err != nil {
		t.Fatalf("ReadRows: %v", err)
	}
	for _, row := range rows {
		for _, cell := range row {
			for _, r := range cell {
				if r == 0 {
					t.Fatalf("NUL survived scrubbing in %q", cell)
				}
			}
		}
	}
}
