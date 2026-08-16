package arabic

import (
	"math"
	"testing"
)

func TestNormalizeCollapsesOrthographicVariants(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"alif hamza above", "أحمد", "احمد"},
		{"alif hamza below", "إبراهيم", "ابراهيم"},
		{"alif madda", "آمن", "امن"},
		{"taa marbuta to haa", "شربة", "شربه"},
		{"alif maqsura to yaa", "مصطفى", "مصطفي"},
		{"tatweel removed", "بـــانادول", "بانادول"},
		{"tashkeel removed", "بَانَادُول", "بانادول"},
		{"arabic-indic digits", "بانادول ٥٠٠", "بانادول 500"},
		{"punctuation becomes space", "بانادول-500", "بانادول 500"},
		{"slash separated dosage", "50/100", "50 100"},
		{"whitespace collapsed", "بانادول    اكسترا", "بانادول اكسترا"},
		{"latin lowercased", "Panadol EXTRA", "panadol extra"},
		{"empty", "", ""},
		{"whitespace only", "   ", ""},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Normalize(c.in); got != c.want {
				t.Errorf("Normalize(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestNormalizeMakesSpellingVariantsIdentical(t *testing.T) {
	// The whole point: two vendors spelling the same medicine differently must
	// produce one key.
	variants := []string{
		"بَانادُول اكسترا",
		"بـانادول اكسترا",
		"بانادول اكسترا",
	}
	first := Normalize(variants[0])
	for _, v := range variants[1:] {
		if got := Normalize(v); got != first {
			t.Errorf("Normalize(%q) = %q, want %q", v, got, first)
		}
	}
}

func TestSimilarityExactAfterNormalisation(t *testing.T) {
	if got := Similarity("بَانادول", "بانادول"); got != 1.0 {
		t.Errorf("normalised-equal strings scored %v, want 1.0", got)
	}
}

func TestSimilarityContainmentBranch(t *testing.T) {
	// A vendor's longer form of the same product. Plain edit distance would
	// score this well below the 0.85 import threshold; the containment branch
	// is what keeps these matching.
	short := "بانادول"
	long := "بانادول اكسترا 500"

	got := Similarity(short, long)
	if got < 0.80 || got > 0.98 {
		t.Errorf("containment similarity = %v, want within [0.80, 0.98]", got)
	}

	// Verify the exact legacy curve: 0.80 + 0.18 * (shorter/longer).
	wantRatio := float64(len([]rune(Normalize(short)))) / float64(len([]rune(Normalize(long))))
	want := 0.80 + 0.18*wantRatio
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("containment similarity = %v, want %v (legacy formula)", got, want)
	}
}

func TestSimilarityIsSymmetric(t *testing.T) {
	a, b := "بانادول اكسترا", "بانادول اكستر"
	if math.Abs(Similarity(a, b)-Similarity(b, a)) > 1e-9 {
		t.Error("similarity should not depend on argument order")
	}
}

func TestSimilarityUnrelatedStringsScoreLow(t *testing.T) {
	if got := Similarity("بانادول", "اموكسيسيلين"); got > 0.5 {
		t.Errorf("unrelated medicines scored %v, expected well below 0.5", got)
	}
}

func TestSimilarityEmptyInput(t *testing.T) {
	if got := Similarity("بانادول", ""); got != 0.0 {
		t.Errorf("Similarity with empty input = %v, want 0.0", got)
	}
	// Two empties normalise equal, and the legacy implementation returns 1.0
	// because its equality check precedes its emptiness check. Reproduced
	// deliberately so parity comparisons against the old importer match.
	if got := Similarity("", ""); got != 1.0 {
		t.Errorf("Similarity(\"\",\"\") = %v, want 1.0 to match legacy ordering", got)
	}
}

func TestLevenshteinCountsRunesNotBytes(t *testing.T) {
	// Arabic letters are two bytes in UTF-8. A byte-wise distance would report
	// 2 for a single substituted letter and distort every score.
	a := []rune("بانادول")
	b := []rune("بانادوك")
	if d := levenshtein(a, b); d != 1 {
		t.Errorf("single-letter substitution gave distance %d, want 1", d)
	}
}

func TestMatchesAtImportThreshold(t *testing.T) {
	// 0.85 is the stored default of import_sessions.min_similarity_score.
	const threshold = 0.85

	if !Matches("بانادول اكسترا", "بَانادول اكسترا", threshold) {
		t.Error("diacritic-only difference should match at the import threshold")
	}
	if Matches("بانادول", "اسبرين", threshold) {
		t.Error("unrelated medicines should not match at the import threshold")
	}
}

func BenchmarkNormalize(b *testing.B) {
	const s = "بَانادول اكسترا ٥٠٠ مجم - أقراص"
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = Normalize(s)
	}
}
