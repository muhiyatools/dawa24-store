package ingest

import (
	"testing"
)

func TestCatalogImportSettingsMinScore(t *testing.T) {
	s := DefaultSettings()
	if s.MinMatchScore != 0.30 {
		t.Fatalf("DefaultSettings().MinMatchScore = %v, want 0.30", s.MinMatchScore)
	}

	// Test low similarity threshold (e.g. 0.20 or 0.10) is allowed and preserved
	custom := Settings{MinMatchScore: 0.15}.Normalize()
	if custom.MinMatchScore != 0.15 {
		t.Fatalf("Normalize() clamped 0.15 to %v, want 0.15", custom.MinMatchScore)
	}

	// Test zero defaults to 0.30
	zeroSetting := Settings{}.Normalize()
	if zeroSetting.MinMatchScore != 0.30 {
		t.Fatalf("Normalize() on zero score gave %v, want 0.30", zeroSetting.MinMatchScore)
	}
}

func TestPhaseReviewState(t *testing.T) {
	if !PhaseReview.Open() {
		t.Fatal("PhaseReview.Open() should be true")
	}
	if PhaseReview.Terminal() {
		t.Fatal("PhaseReview.Terminal() should be false")
	}
	if PhaseReview.Label() == "" {
		t.Fatal("PhaseReview.Label() should have Arabic description")
	}
}

func TestRowOutcomeEffectiveVariantName(t *testing.T) {
	row := &RowOutcome{
		DisplayName:       "Panadol Extra 500mg",
		CustomVariantName: "بنادول اكسترا احمر - صيدليتي",
	}
	if got := row.EffectiveVariantName(); got != "بنادول اكسترا احمر - صيدليتي" {
		t.Fatalf("EffectiveVariantName() = %q, want custom name", got)
	}

	rowNoCustom := &RowOutcome{
		DisplayName: "Panadol Extra 500mg",
	}
	if got := rowNoCustom.EffectiveVariantName(); got != "Panadol Extra 500mg" {
		t.Fatalf("EffectiveVariantName() = %q, want original display name", got)
	}
}
