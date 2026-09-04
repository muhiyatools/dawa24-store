package ui_test

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/promo"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

func TestVendorAdsWizard_PageRender(t *testing.T) {
	ctx := context.Background()

	orgID := int64(42)
	now := time.Now().UTC()
	activePurchases := []*promo.SponsorshipPurchase{
		{
			ID:               1,
			OrganizationID:   orgID,
			PackageID:        10,
			CreditsTotal:     10,
			CreditsUsed:      4,
			CreditsRemaining: 6,
			Status:           promo.PurchaseActive,
			ExpiresAt:        now.Add(30 * 24 * time.Hour),
		},
	}

	ads := []*promo.Ad{
		{
			ID:             1,
			OrganizationID: &orgID,
			Title:          "بنر أوجمنتين الترويجي",
			TitleAr:        "بنر أوجمنتين الترويجي",
			Position:       promo.PositionHomeHero,
			MediaType:      promo.MediaImage,
			MediaURL:       "/uploads/ads/augmentin.png",
			AdminStatus:    promo.AdminApproved,
			Impressions:    1250,
			Clicks:         85,
			CTR:            6.8,
		},
	}

	inStockItems := []pages.VendorOfferItemOption{
		{
			VariantID:      101,
			NameAr:         "أوجمنتين 1 جم أقراص",
			NameEn:         "Augmentin 1g Tablets",
			SKU:            "AUG-1G-14T",
			WarehouseName:  "مستودع القاهرة المركزي",
			AvailableStock: 50,
			Price:          "120.50",
		},
	}

	data := pages.VendorAdsData{
		Ads:             ads,
		ItemOptions:     inStockItems,
		ActivePurchases: activePurchases,
		TotalCredits:    6,
	}

	var buf bytes.Buffer
	component := pages.VendorAdsPage("ar", "rtl", data)
	if err := component.Render(ctx, &buf); err != nil {
		t.Fatalf("VendorAdsPage.Render failed: %v", err)
	}

	html := buf.String()

	// The wizard's shell, its four steps and the placements it offers.
	expectedSnippets := []string{
		"معالج إنشاء الإعلان الترويجي",
		"الصنف",
		"الموضع والمدة",
		"الوسائط والمحتوى",
		"المراجعة",
		"معرض وبنر الواجهة الرئيسية (Hero Banner)",
		"شريط العروض والصفقات الترويجية (Deals Banner)",
		"بنر صدارة كتالوج وسوق الأدوية (Catalog Top Header)",
		"الرصيد المتاح",
		"بنر أوجمنتين الترويجي",
	}

	for _, snippet := range expectedSnippets {
		if !strings.Contains(html, snippet) {
			t.Errorf("Expected VendorAdsPage HTML to contain %q, but not found", snippet)
		}
	}

	// The picker fetches on demand rather than shipping the inventory.
	//
	// The wizard used to serialise every in-stock variant into its x-data
	// attribute — the whole catalogue inlined into the page whose job is to
	// pick one row out of it — and filter that array in the browser.
	if !strings.Contains(html, "/vendor/inventory/search-json") {
		t.Error("the product picker does not use the on-demand search endpoint")
	}
	if strings.Contains(html, "availableItems") {
		t.Error("the wizard still inlines the supplier's inventory into its x-data attribute")
	}

	// The retired click target must not be offered.
	if strings.Contains(html, "external_url") {
		t.Error("the wizard still offers the external-URL click target")
	}

	// The form has to be a flex child of the dialog or its footer is clipped;
	// that is a stylesheet rule, but the markup it needs is the form sitting
	// directly inside .modal-dialog with the body and footer inside the form.
	if !strings.Contains(html, "modal-footer") {
		t.Error("the wizard renders no footer")
	}
}

func TestVendorSponsorshipRequests_MultiSelectRender(t *testing.T) {
	ctx := context.Background()

	orgID := int64(42)
	now := time.Now().UTC()
	activePurchases := []*promo.SponsorshipPurchase{
		{
			ID:               1,
			OrganizationID:   orgID,
			PackageID:        5,
			CreditsTotal:     20,
			CreditsUsed:      5,
			CreditsRemaining: 15,
			Status:           promo.PurchaseActive,
			ExpiresAt:        now.Add(30 * 24 * time.Hour),
		},
	}

	inStockItems := []pages.VendorOfferItemOption{
		{
			VariantID:      201,
			NameAr:         "بنادول إكسترا 24 قرص",
			NameEn:         "Panadol Extra 24 Tabs",
			SKU:            "PAN-EXT-24",
			AvailableStock: 120,
			Price:          "45.00",
		},
		{
			VariantID:      202,
			NameAr:         "كونجستال 20 قرص",
			NameEn:         "Congestal 20 Tabs",
			SKU:            "CONG-20",
			AvailableStock: 80,
			Price:          "35.00",
		},
	}

	data := pages.SponsorshipRequestsData{
		ActivePurchases: activePurchases,
		ItemOptions:     inStockItems,
		TotalCredits:    15,
		OrgID:           orgID,
	}

	var buf bytes.Buffer
	component := pages.VendorSponsorshipRequestsPage("ar", "rtl", data)
	if err := component.Render(ctx, &buf); err != nil {
		t.Fatalf("VendorSponsorshipRequestsPage.Render failed: %v", err)
	}

	html := buf.String()

	expectedSnippets := []string{
		"تقديم طلب رعاية جديد لأصنافك وعروضك",
		"أصناف دوائية من المخزون (In-Stock Products)",
		"كل عنصر مختار يستهلك",
		"1 رصيد رعاية",
		"العناصر المحددة للرعاية",
		"15 / 20",
	}

	for _, snippet := range expectedSnippets {
		if !strings.Contains(html, snippet) {
			t.Errorf("Expected VendorSponsorshipRequestsPage HTML to contain %q, but not found", snippet)
		}
	}
}

func TestStorefrontAdPlacements_Render(t *testing.T) {
	ctx := context.Background()

	orgID := int64(10)
	heroAd := &promo.Ad{
		ID:             1,
		OrganizationID: &orgID,
		Title:          "إعلان الواجهة الرئيسية البارز",
		TitleAr:        "إعلان الواجهة الرئيسية البارز",
		Position:       promo.PositionHomeHero,
		MediaURL:       "/uploads/ads/hero.jpg",
		MediaType:      promo.MediaImage,
	}
	dealsAd := &promo.Ad{
		ID:             2,
		OrganizationID: &orgID,
		Title:          "إعلان صفقات وعروض حصرية",
		TitleAr:        "إعلان صفقات وعروض حصرية",
		Position:       promo.PositionHomeDeals,
		MediaURL:       "/uploads/ads/deals.jpg",
		MediaType:      promo.MediaImage,
	}
	bottomAd := &promo.Ad{
		ID:             3,
		OrganizationID: &orgID,
		Title:          "إعلان أسفل الصفحة الترويجي",
		TitleAr:        "إعلان أسفل الصفحة الترويجي",
		Position:       promo.PositionHomeBottom,
		MediaURL:       "/uploads/ads/bottom.jpg",
		MediaType:      promo.MediaImage,
	}

	stats := pages.HomeStats{
		TotalSuppliers: 50,
		TotalProducts:  1000,
		Ads:            []*promo.Ad{heroAd},
		DealsAds:       []*promo.Ad{dealsAd},
		BottomAds:      []*promo.Ad{bottomAd},
	}

	var buf bytes.Buffer
	comp := pages.CustomerHome(nil, nil, nil, stats, "ar", "rtl")
	if err := comp.Render(ctx, &buf); err != nil {
		t.Fatalf("CustomerHome.Render failed: %v", err)
	}

	html := buf.String()

	if !strings.Contains(html, "إعلان الواجهة الرئيسية البارز") {
		t.Errorf("Expected CustomerHome to render Hero Ad gallery")
	}
	if !strings.Contains(html, "إعلان صفقات وعروض حصرية") {
		t.Errorf("Expected CustomerHome to render Deals Ad banner")
	}
	if !strings.Contains(html, "إعلان أسفل الصفحة الترويجي") {
		t.Errorf("Expected CustomerHome to render Bottom Ad banner")
	}
}

func TestCatalogTopAdPlacement_Render(t *testing.T) {
	ctx := context.Background()

	orgID := int64(10)
	catalogAd := &promo.Ad{
		ID:             5,
		OrganizationID: &orgID,
		Title:          "إعلان صدارة الكتالوج الرسمي",
		TitleAr:        "إعلان صدارة الكتالوج الرسمي",
		Position:       promo.PositionCatalogTop,
		MediaURL:       "/uploads/ads/catalog_top.png",
		MediaType:      promo.MediaImage,
	}

	data := pages.CatalogPageData{
		CatalogAds: []*promo.Ad{catalogAd},
		Page:       1,
		PageSize:   24,
	}

	var buf bytes.Buffer
	comp := pages.CustomerCatalog(data, "ar", "rtl", false)
	if err := comp.Render(ctx, &buf); err != nil {
		t.Fatalf("CustomerCatalog.Render failed: %v", err)
	}

	html := buf.String()

	if !strings.Contains(html, "إعلان صدارة الكتالوج الرسمي") {
		t.Errorf("Expected CustomerCatalog to render Catalog Top Ad Banner")
	}
}
