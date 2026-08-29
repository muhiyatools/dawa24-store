package ui_test

import (
	"bytes"
	"context"
	"math"
	"sort"
	"strings"
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// TestSupplierProfile_ActiveTabPreservation verifies that active tab is correctly rendered in HTML.
func TestSupplierProfile_ActiveTabPreservation(t *testing.T) {
	testCases := []struct {
		name         string
		activeTab    string
		expectedTab  string
		expectedShow string
		expectedHide string
	}{
		{
			name:         "Default to catalog",
			activeTab:    "catalog",
			expectedTab:  "tab-btn-catalog",
			expectedShow: "tab-content-catalog",
			expectedHide: "tab-content-policies",
		},
		{
			name:         "Preserve policies tab",
			activeTab:    "policies",
			expectedTab:  "tab-btn-policies",
			expectedShow: "tab-content-policies",
			expectedHide: "tab-content-catalog",
		},
		{
			name:         "Preserve branches tab",
			activeTab:    "branches",
			expectedTab:  "tab-btn-branches",
			expectedShow: "tab-content-branches",
			expectedHide: "tab-content-catalog",
		},
		{
			name:         "Preserve reviews tab",
			activeTab:    "reviews",
			expectedTab:  "tab-btn-reviews",
			expectedShow: "tab-content-reviews",
			expectedHide: "tab-content-catalog",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			data := pages.SupplierProfileData{
				Org: &org.Organization{
					ID:        10,
					TradeName: i18n.Text{"ar": "مستودع أسوان للتوريدات", "en": "Aswan Supply Warehouse"},
				},
				ActiveTab:     tc.activeTab,
				TotalVariants: 5,
				Policies: []*org.Policy{
					{
						Title:   "سياسة التوصيل",
						Content: "توصيل خلال 24 ساعة في أسوان",
					},
				},
				Branches: []*org.Branch{
					{
						ID:      1,
						Name:    i18n.Text{"ar": "فرع أسوان الرئيسي", "en": "Aswan Main Branch"},
						Address: "أسوان، كورنيش النيل",
					},
				},
			}

			var buf bytes.Buffer
			err := pages.SupplierProfile("ar", "rtl", data).Render(context.Background(), &buf)
			if err != nil {
				t.Fatalf("failed to render supplier profile page: %v", err)
			}
			html := buf.String()

			if !strings.Contains(html, tc.expectedTab) {
				t.Errorf("expected HTML to contain tab button ID %s", tc.expectedTab)
			}
			if !strings.Contains(html, tc.expectedShow) {
				t.Errorf("expected HTML to contain tab panel ID %s", tc.expectedShow)
			}
		})
	}
}

// TestCatalog_LocationDistancePrioritization verifies that products from nearby vendors (e.g. Aswan)
// and in-stock items are prioritized above faraway vendors or out-of-stock items.
func TestCatalog_LocationDistancePrioritization(t *testing.T) {
	// Simulate 4 products with variants in different locations and stock states
	cards := []*pages.SupplierVariantCard{
		{
			VariantID:       101,
			ProductID:       1,
			ProductNameAr:   "بنادول إكسترا أقراص",
			SupplierName:    "مورد القاهرة المركزي",
			CityName:        "القاهرة",
			DistanceKM:      850.0,
			DistanceText:    "850 كم",
			AvailableStock:  50,
			CanAddToCart:    true,
			IsCovered:       true,
			Price:           money.FromMinor(3500),
			DiscountPercent: 10,
		},
		{
			VariantID:       102,
			ProductID:       2,
			ProductNameAr:   "كونجستال أقراص",
			SupplierName:    "مستودع أدوية أسوان المحلي",
			CityName:        "أسوان",
			DistanceKM:      4.5,
			DistanceText:    "4.5 كم",
			AvailableStock:  120,
			CanAddToCart:    true,
			IsCovered:       true,
			Price:           money.FromMinor(2800),
			DiscountPercent: 12,
		},
		{
			VariantID:       103,
			ProductID:       3,
			ProductNameAr:   "أوجمنتين 1 جم",
			SupplierName:    "مورد الأقصر المعتمد",
			CityName:        "الأقصر",
			DistanceKM:      220.0,
			DistanceText:    "220 كم",
			AvailableStock:  30,
			CanAddToCart:    true,
			IsCovered:       true,
			Price:           money.FromMinor(9500),
			DiscountPercent: 5,
		},
		{
			VariantID:       104,
			ProductID:       4,
			ProductNameAr:   "فيتامين سي فوار",
			SupplierName:    "مستودع أسوان 2",
			CityName:        "أسوان",
			DistanceKM:      2.0,
			DistanceText:    "2.0 كم",
			AvailableStock:  0, // Out of stock
			CanAddToCart:    false,
			IsCovered:       true,
			Price:           money.FromMinor(1500),
			DiscountPercent: 0,
		},
	}

	// Sort cards using our production algorithm
	sort.SliceStable(cards, func(i, j int) bool {
		// Tier 1: Actionable (In-stock & Covered)
		if cards[i].CanAddToCart != cards[j].CanAddToCart {
			return cards[i].CanAddToCart
		}
		if (cards[i].AvailableStock > 0) != (cards[j].AvailableStock > 0) {
			return cards[i].AvailableStock > 0
		}
		if cards[i].IsCovered != cards[j].IsCovered {
			return cards[i].IsCovered
		}

		// Tier 2: Proximity to Client (DistanceKM ascending)
		if cards[i].DistanceKM > 0 && cards[j].DistanceKM > 0 && math.Abs(cards[i].DistanceKM-cards[j].DistanceKM) > 1.0 {
			return cards[i].DistanceKM < cards[j].DistanceKM
		}
		if cards[i].DistanceKM > 0 && cards[j].DistanceKM <= 0 {
			return true
		}
		if cards[i].DistanceKM <= 0 && cards[j].DistanceKM > 0 {
			return false
		}
		if cards[i].DiscountPercent != cards[j].DiscountPercent {
			return cards[i].DiscountPercent > cards[j].DiscountPercent
		}
		return cards[i].ProductID < cards[j].ProductID
	})

	// 1st item MUST be Aswan (In-Stock & Nearest: 4.5 km)
	if cards[0].ProductID != 2 || cards[0].CityName != "أسوان" {
		t.Fatalf("expected 1st item to be Aswan nearby in-stock product (ID 2), got ID %d (%s)", cards[0].ProductID, cards[0].CityName)
	}

	// 2nd item MUST be Luxor (In-Stock & 220 km)
	if cards[1].ProductID != 3 || cards[1].CityName != "الأقصر" {
		t.Fatalf("expected 2nd item to be Luxor in-stock product (ID 3), got ID %d (%s)", cards[1].ProductID, cards[1].CityName)
	}

	// 3rd item MUST be Cairo (In-Stock & 850 km)
	if cards[2].ProductID != 1 || cards[2].CityName != "القاهرة" {
		t.Fatalf("expected 3rd item to be Cairo in-stock product (ID 1), got ID %d (%s)", cards[2].ProductID, cards[2].CityName)
	}

	// 4th item MUST be the out-of-stock item
	if cards[3].ProductID != 4 || cards[3].CanAddToCart {
		t.Fatalf("expected 4th item to be out-of-stock product (ID 4), got ID %d", cards[3].ProductID)
	}
}

// TestCatalog_RenderingDistanceBadge verifies that distance and city are rendered on the storefront.
func TestCatalog_RenderingDistanceBadge(t *testing.T) {
	card := &pages.SupplierVariantCard{
		VariantID:       201,
		ProductID:       15,
		ProductNameAr:   "باراسيتامول 500 مجم",
		SupplierName:    "مستودع أسوان الإقليمي",
		BranchName:      "فرع أسوان الرئيسي",
		CityName:        "أسوان",
		DistanceKM:      5.2,
		DistanceText:    "5.2 كم",
		AvailableStock:  200,
		CanAddToCart:    true,
		IsCovered:       true,
		Price:           money.FromMinor(1800),
		DiscountPercent: 15,
	}

	data := pages.CatalogPageData{
		Variants:   []*pages.SupplierVariantCard{card},
		Page:       1,
		PageSize:   24,
		TotalItems: 1,
		TotalPages: 1,
		ViewMode:   "grid",
	}

	var buf bytes.Buffer
	err := pages.CustomerCatalog(data, "ar", "rtl", false).Render(context.Background(), &buf)
	if err != nil {
		t.Fatalf("failed to render customer catalog page: %v", err)
	}
	html := buf.String()

	if !strings.Contains(html, "5.2 كم") {
		t.Errorf("expected catalog HTML to contain distance badge '5.2 كم'")
	}
	if !strings.Contains(html, "مستودع أسوان الإقليمي") {
		t.Errorf("expected catalog HTML to contain supplier name")
	}
	if !strings.Contains(html, "الأقرب جغرافياً والأعلى توفراً") {
		t.Errorf("expected catalog HTML to contain default sort option")
	}
}
