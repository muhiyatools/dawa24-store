package ingest

import (
	"context"
	"testing"
)

type mockAIMatcher struct {
	expectedCand string
	score        float64
}

func (m *mockAIMatcher) MatchCandidate(_ context.Context, _ string, _ []string) (string, float64) {
	return m.expectedCand, m.score
}

func TestMultiStageMatchingEngine(t *testing.T) {
	ctx := context.Background()

	masterProducts := []*MasterProductData{
		{
			ID:            101,
			NameAR:        "بانادول إكسترا أقراص",
			NameEN:        "Panadol Extra Tablets",
			SKU:           "PAN-EXT-500",
			Barcode:       "6221234567890",
			DosageForm:    "أقراص",
			Concentration: "500mg",
			Manufacturer:  "GSK",
		},
		{
			ID:            102,
			NameAR:        "كونجستال أقراص للبرد",
			NameEN:        "Congestal Tablets",
			SKU:           "CONG-TAB",
			Barcode:       "6229876543210",
			DosageForm:    "أقراص",
			Concentration: "650mg",
			Manufacturer:  "Sigma",
		},
		{
			ID:            103,
			NameAR:        "أوجمنتين 1 جم أقراص",
			NameEN:        "Augmentin 1g Tablets",
			SKU:           "AUG-1G",
			Barcode:       "6225555555555",
			DosageForm:    "أقراص",
			Concentration: "1g",
			Manufacturer:  "GSK",
		},
	}

	savingProducts := []*SavingProductData{
		{
			ProductID:   101,
			NameProduct: "بنادول اكسترا احمر",
			SKU:         "PAN-EXT-500",
		},
	}

	index := NewCatalogMatchIndex(masterProducts, savingProducts)

	// Test 1: Stage 1 - Exact Barcode Match
	t.Run("Stage 1 - Barcode Match", func(t *testing.T) {
		res := index.Match(ctx, MatchRowInput{
			RawName:       "اسم عشوائي مختلف",
			Barcode:       "6221234567890",
			EnableAI:      true,
			EnableSavings: true,
			MinSimilarity: 0.85,
		}, nil)

		if res.ConfidenceLevel != ConfidenceHigh {
			t.Fatalf("expected High confidence, got %s", res.ConfidenceLevel)
		}
		if res.MatchedProductID == nil || *res.MatchedProductID != 101 {
			t.Fatalf("expected MatchedProductID 101, got %v", res.MatchedProductID)
		}
		if res.ConfidenceScore < 0.98 {
			t.Fatalf("expected score >= 0.98, got %f", res.ConfidenceScore)
		}
	})

	// Test 2: Stage 2 - Exact Normalized Name Match
	t.Run("Stage 2 - Exact Normalized Name Match", func(t *testing.T) {
		res := index.Match(ctx, MatchRowInput{
			RawName:       "بانادول إكسترا أقراص",
			EnableAI:      true,
			EnableSavings: true,
			MinSimilarity: 0.85,
		}, nil)

		if res.ConfidenceLevel != ConfidenceHigh {
			t.Fatalf("expected High confidence, got %s", res.ConfidenceLevel)
		}
		if res.MatchedProductID == nil || *res.MatchedProductID != 101 {
			t.Fatalf("expected MatchedProductID 101, got %v", res.MatchedProductID)
		}
	})

	// Test 3: Stage 3 - Savings Products Match
	t.Run("Stage 3 - Savings Products Match", func(t *testing.T) {
		res := index.Match(ctx, MatchRowInput{
			RawName:       "بنادول اكسترا احمر",
			EnableAI:      true,
			EnableSavings: true,
			MinSimilarity: 0.85,
		}, nil)

		if res.ConfidenceLevel != ConfidenceHigh {
			t.Fatalf("expected High confidence, got %s", res.ConfidenceLevel)
		}
		if res.MatchedProductID == nil || *res.MatchedProductID != 101 {
			t.Fatalf("expected MatchedProductID 101, got %v", res.MatchedProductID)
		}
		if res.ConfidenceScore < 0.90 {
			t.Fatalf("expected score >= 0.90, got %f", res.ConfidenceScore)
		}
	})

	// Test 4: Stage 4 - Fuzzy and Attribute Match (Congestal with dosage & manufacturer)
	t.Run("Stage 4 - Multi-Attribute Match", func(t *testing.T) {
		res := index.Match(ctx, MatchRowInput{
			RawName:       "كونجستال",
			DosageForm:    "أقراص",
			Concentration: "650mg",
			Manufacturer:  "Sigma",
			EnableAI:      false,
			EnableSavings: false,
			MinSimilarity: 0.85,
		}, nil)

		if res.ConfidenceLevel != ConfidenceHigh {
			t.Fatalf("expected High confidence, got %s", res.ConfidenceLevel)
		}
		if res.MatchedProductID == nil || *res.MatchedProductID != 102 {
			t.Fatalf("expected MatchedProductID 102, got %v", res.MatchedProductID)
		}
	})

	// Test 5: Stage 5 - AI Fallback Match
	t.Run("Stage 5 - AI Match", func(t *testing.T) {
		ai := &mockAIMatcher{
			expectedCand: "أوجمنتين 1 جم أقراص",
			score:        0.88,
		}

		res := index.Match(ctx, MatchRowInput{
			RawName:       "أوجمنتين حبوب 1000 مجم",
			EnableAI:      true,
			EnableSavings: true,
			MinSimilarity: 0.85,
		}, ai)

		if res.ConfidenceLevel != ConfidenceHigh {
			t.Fatalf("expected High confidence via AI, got %s", res.ConfidenceLevel)
		}
		if res.MatchedProductID == nil || *res.MatchedProductID != 103 {
			t.Fatalf("expected MatchedProductID 103, got %v", res.MatchedProductID)
		}
	})

	// Test 6: Unmatched Product
	t.Run("Unmatched Product", func(t *testing.T) {
		res := index.Match(ctx, MatchRowInput{
			RawName:       "منتج غريب جدا غير موجود إطلاقا في قاعدة البيانات",
			EnableAI:      false,
			EnableSavings: false,
			MinSimilarity: 0.85,
		}, nil)

		if res.ConfidenceLevel != ConfidenceUnmatched {
			t.Fatalf("expected Unmatched, got %s", res.ConfidenceLevel)
		}
		if res.MatchedProductID != nil {
			t.Fatalf("expected nil MatchedProductID, got %v", res.MatchedProductID)
		}
	})
}
