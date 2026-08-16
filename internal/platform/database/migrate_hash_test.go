package database

import "testing"

// A migration's checksum must describe its SQL, not the operating system the
// file was last written on.
//
// Without normalisation, a migration applied from a Windows working tree (CRLF)
// records one hash, and every Linux container then computes another and refuses
// to start with "modified after being applied". That failed a production deploy
// with no schema change of any kind, twice.
func TestHashIgnoresLineEndings(t *testing.T) {
	lf := []byte("BEGIN;\nALTER TABLE x ADD COLUMN y TEXT;\nCOMMIT;\n")
	crlf := []byte("BEGIN;\r\nALTER TABLE x ADD COLUMN y TEXT;\r\nCOMMIT;\r\n")

	if string(normaliseLineEndings(lf)) != string(normaliseLineEndings(crlf)) {
		t.Fatal("CRLF and LF forms of the same SQL normalise differently")
	}

	// A real content change must still be visible.
	edited := []byte("BEGIN;\nALTER TABLE x ADD COLUMN z TEXT;\nCOMMIT;\n")
	if string(normaliseLineEndings(lf)) == string(normaliseLineEndings(edited)) {
		t.Fatal("an edited statement was normalised away; the checksum would no longer detect edits")
	}
}
