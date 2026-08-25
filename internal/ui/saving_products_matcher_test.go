package ui

import (
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
)

func TestSavingProductMatchEngine(t *testing.T) {
	catalogItems := []*catalog.CatalogMatchSource{
		{
			ID:      101,
			SKU:     "PAN-24",
			Barcode: "6221234567890",
			NameAr:  "بانادول إكسترا 24 قرص",
			NameEn:  "Panadol Extra 24 Tablets",
		},
		{
			ID:      102,
			SKU:     "20203380",
			Barcode: "20203380",
			NameAr:  "ليدي سبيد ستيك مزيل عرق 65 جرام",
			NameEn:  "Lady Speed Stick Deodorant 65g",
		},
		{
			ID:      103,
			SKU:     "AUG-1G",
			Barcode: "6229876543210",
			NameAr:  "أوجمنتين 1 جم 14 قرص",
			NameEn:  "Augmentin 1g 14 Tablets",
		},
		{
			ID:      104,
			SKU:     "FEM-250",
			Barcode: "5010724524221",
			NameAr:  "فيم فريش غسول يومي للمناطق الحساسة 250 مل",
			NameEn:  "Femfresh Daily Intimate Wash 250ml",
		},
	}

	engine := NewSavingProductMatchEngine(catalogItems)

	t.Run("Exact SKU Match", func(t *testing.T) {
		res := engine.Match(StrategySmartAuto, nil, "PAN-24", "اسم عشوائي مختلف")
		if res.ProductID == nil || *res.ProductID != 101 {
			t.Fatalf("expected product 101, got %v", res.ProductID)
		}
		if res.MatchType != "exact_sku" {
			t.Errorf("expected exact_sku, got %s", res.MatchType)
		}
	})

	t.Run("Cleaned Barcode with .0 and spaces", func(t *testing.T) {
		res := engine.Match(StrategySmartAuto, nil, " 6221234567890.0 ", "")
		if res.ProductID == nil || *res.ProductID != 101 {
			t.Fatalf("expected product 101, got %v", res.ProductID)
		}
		if res.MatchType != "clean_sku" {
			t.Errorf("expected clean_sku, got %s", res.MatchType)
		}
	})

	t.Run("Scientific notation barcode expansion", func(t *testing.T) {
		sanitized := SanitizeSKUCode("2.020338e+07")
		if sanitized != "20203380" {
			t.Fatalf("expected 20203380, got %s", sanitized)
		}
		res := engine.Match(StrategySmartAuto, nil, "2.020338e+07", "")
		if res.ProductID == nil || *res.ProductID != 102 {
			t.Fatalf("expected product 102, got %v", res.ProductID)
		}
	})

	t.Run("Normalized Arabic Name Match (Hamza variation)", func(t *testing.T) {
		// Catalog has "أوجمنتين 1 جم 14 قرص" (with Alif Hamza), input has "اوجمنتين 1 جم 14 قرص" (without Hamza)
		res := engine.Match(StrategySmartAuto, nil, "", "اوجمنتين 1 جم 14 قرص")
		if res.ProductID == nil || *res.ProductID != 103 {
			t.Fatalf("expected product 103, got %v", res.ProductID)
		}
		if res.MatchType != "norm_name" {
			t.Errorf("expected norm_name, got %s", res.MatchType)
		}
	})

	t.Run("Normalized Yaa / Alif Maqsura Match", func(t *testing.T) {
		// Catalog has "ليدي سبيد", input has "ليدى سبيد"
		res := engine.Match(StrategySmartAuto, nil, "", "ليدى سبيد ستيك مزيل عرق 65 جرام")
		if res.ProductID == nil || *res.ProductID != 102 {
			t.Fatalf("expected product 102, got %v", res.ProductID)
		}
	})

	t.Run("Core Drug Name Match without dosage suffix noise", func(t *testing.T) {
		// Input has "بانادول إكسترا" without "24 قرص"
		res := engine.Match(StrategySmartAuto, nil, "", "بانادول اكسترا")
		if res.ProductID == nil || *res.ProductID != 101 {
			t.Fatalf("expected product 101, got %v", res.ProductID)
		}
	})

	t.Run("Fuzzy Similarity Match for Pharmacy Description", func(t *testing.T) {
		// Input: "فيم فريش غسول انتميت سكير 250 مل"
		// Catalog: "فيم فريش غسول يومي للمناطق الحساسة 250 مل"
		res := engine.Match(StrategySmartAuto, nil, "INTERNAL-1234", "فيم فريش غسول انتميت سكير 250 مل")
		if res.ProductID == nil || *res.ProductID != 104 {
			t.Fatalf("expected product 104, got %v", res.ProductID)
		}
		if res.Confidence < 0.80 {
			t.Errorf("expected confidence >= 0.80, got %f", res.Confidence)
		}
	})

	t.Run("Strategy: SKU Only mode", func(t *testing.T) {
		// Correct name but unknown SKU
		res := engine.Match(StrategySKUOnly, nil, "UNKNOWN-SKU", "بانادول إكسترا 24 قرص")
		if res.ProductID != nil {
			t.Fatalf("expected no match in SKUOnly mode with unknown SKU, got %v", res.ProductID)
		}

		// Known SKU
		res2 := engine.Match(StrategySKUOnly, nil, "PAN-24", "اسم مختلف تماماً")
		if res2.ProductID == nil || *res2.ProductID != 101 {
			t.Fatalf("expected product 101, got %v", res2.ProductID)
		}
	})

	t.Run("Strategy: Name Only mode (Internal SKU ignored)", func(t *testing.T) {
		// Internal pharmacy cashier SKU that does NOT match catalog, but name matches
		res := engine.Match(StrategyNameOnly, nil, "MY-POS-998877", "بانادول اكسترا 24 قرص")
		if res.ProductID == nil || *res.ProductID != 101 {
			t.Fatalf("expected product 101 in NameOnly mode, got %v", res.ProductID)
		}
	})
}

func TestExcelSanitizers(t *testing.T) {
	t.Run("Summary and Total Row Detection", func(t *testing.T) {
		if !IsSummaryOrTotalRow([]string{"الإجمالي", "1500", "50000"}) {
			t.Errorf("expected true for الإجمالي")
		}
		if !IsSummaryOrTotalRow([]string{"Total Amount", "100", "200"}) {
			t.Errorf("expected true for Total Amount")
		}
		if !IsSummaryOrTotalRow([]string{"المجموع الكلي:", "50"}) {
			t.Errorf("expected true for المجموع الكلي:")
		}
		if IsSummaryOrTotalRow([]string{"بانادول اكسترا", "10", "50"}) {
			t.Errorf("expected false for standard product row")
		}
	})

	t.Run("ParseFlexibleQuantity", func(t *testing.T) {
		qty, ok := ParseFlexibleQuantity(" 1,500.00 ")
		if !ok || qty != 1500.0 {
			t.Errorf("expected 1500, got %f (ok=%v)", qty, ok)
		}

		qty2, ok := ParseFlexibleQuantity("25")
		if !ok || qty2 != 25.0 {
			t.Errorf("expected 25, got %f", qty2)
		}

		qty3, ok := ParseFlexibleQuantity("١٠") // Arabic-Indic digits
		if !ok || qty3 != 10.0 {
			t.Errorf("expected 10, got %f", qty3)
		}
	})

	t.Run("ParseFlexibleMoney", func(t *testing.T) {
		m1, ok := ParseFlexibleMoney(" 1,250.50 ج.م ")
		if !ok || m1.Minor() != 125050 {
			t.Errorf("expected 125050 minor, got %d (ok=%v)", m1.Minor(), ok)
		}

		m2, ok := ParseFlexibleMoney("45.00 EGP")
		if !ok || m2.Minor() != 4500 {
			t.Errorf("expected 4500 minor, got %d", m2.Minor())
		}

		m3, ok := ParseFlexibleMoney("٥٠.٢٥ جنيه")
		if !ok || m3.Minor() != 5025 {
			t.Errorf("expected 5025 minor, got %d", m3.Minor())
		}

		_, ok4 := ParseFlexibleMoney("-10")
		if ok4 {
			t.Errorf("expected negative price to be rejected")
		}
	})
}
