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

	// Test 7: End-to-End Excel spreadsheet parsing and auto-detection
	t.Run("End-to-End Excel Matching", func(t *testing.T) {
		headers := []string{
			"اسم الصنف", "الباركود الدولي", "كود الصنف", "سعر البيع للجمهور",
			"سعر التكلفة", "نسبة الخصم", "الكمية المتوفرة", "رقم التشغيلة",
			"تاريخ الصلاحية", "الوحدة", "الشكل الدوائي", "التركيز", "الشركة المصنعة",
		}
		detected := DetectColumns(headers)
		if detected[FieldProductName] != "اسم الصنف" {
			t.Fatalf("expected product_name mapped to 'اسم الصنف', got %q", detected[FieldProductName])
		}
		if detected[FieldBarcode] != "الباركود الدولي" {
			t.Fatalf("expected barcode mapped to 'الباركود الدولي', got %q", detected[FieldBarcode])
		}
		if detected[FieldPrice] != "سعر البيع للجمهور" {
			t.Fatalf("expected price mapped to 'سعر البيع للجمهور', got %q", detected[FieldPrice])
		}
		if detected[FieldQuantity] != "الكمية المتوفرة" {
			t.Fatalf("expected quantity mapped to 'الكمية المتوفرة', got %q", detected[FieldQuantity])
		}

		// Test row 1: Barcode match
		row1 := MatchRowInput{
			RawName:  "بانادول إكسترا 500 مجم أقراص",
			Barcode:  "6221234567890",
			SKU:      "PAN-EXT-500",
			EnableAI: true,
		}
		res1 := index.Match(ctx, row1, nil)
		if res1.ConfidenceLevel != ConfidenceHigh || res1.MatchedProductID == nil || *res1.MatchedProductID != 101 {
			t.Fatalf("row 1: expected high confidence match 101, got level=%s id=%v", res1.ConfidenceLevel, res1.MatchedProductID)
		}

		// Test row 2: SKU match
		row2 := MatchRowInput{
			RawName:  "كونجستال أقراص للبرد والاحتقان",
			SKU:      "CONG-TAB",
			EnableAI: true,
		}
		res2 := index.Match(ctx, row2, nil)
		if res2.ConfidenceLevel != ConfidenceHigh || res2.MatchedProductID == nil || *res2.MatchedProductID != 102 {
			t.Fatalf("row 2: expected high confidence match 102, got level=%s id=%v", res2.ConfidenceLevel, res2.MatchedProductID)
		}

		// Test row 4: Savings product match
		row4 := MatchRowInput{
			RawName:       "بنادول احمر اكسترا توفير",
			EnableSavings: true,
		}
		res4 := index.Match(ctx, row4, nil)
		if res4.ConfidenceLevel != ConfidenceHigh || res4.MatchedProductID == nil || *res4.MatchedProductID != 101 {
			t.Fatalf("row 4: expected savings match 101, got level=%s id=%v", res4.ConfidenceLevel, res4.MatchedProductID)
		}
	})
}
