package ui

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

func TestCustomerOrderReviewModal_StructureAndStyling(t *testing.T) {
	order := &commerce.Order{
		ID:          180677,
		OrderNumber: "ORD-20260821-180677",
	}

	var buf bytes.Buffer
	ctx := context.Background()
	err := pages.CustomerOrderReviewModal(order, "ar").Render(ctx, &buf)
	if err != nil {
		t.Fatalf("Failed to render CustomerOrderReviewModal: %v", err)
	}

	html := buf.String()

	// 1. Must use standard platform modal system with .modal and .modal-box
	if !strings.Contains(html, `<dialog id="vendor-review-dialog" class="modal"`) {
		t.Errorf("Expected <dialog id=\"vendor-review-dialog\" class=\"modal\"> for centered layout")
	}
	if !strings.Contains(html, `class="modal-box modal-md"`) {
		t.Errorf("Expected <div class=\"modal-box modal-md\"> for centered modal card, not found in HTML")
	}

	// 2. Must use SVG star rating picker with accessible hidden inputs
	if !strings.Contains(html, `class="star-rating-picker"`) {
		t.Errorf("Expected class=\"star-rating-picker\" in HTML")
	}
	if !strings.Contains(html, `class="star-picker-input"`) {
		t.Errorf("Expected class=\"star-picker-input\" for visually hidden inputs in HTML")
	}
	if !strings.Contains(html, `class="star-picker-icon"`) {
		t.Errorf("Expected class=\"star-picker-icon\" for crisp SVG stars in HTML")
	}

	// 3. Must have zero inline styles (Strict: no inline ever)
	if strings.Contains(html, `style=`) {
		t.Errorf("CustomerOrderReviewModal contains forbidden inline style: %s", html)
	}
}

func TestSupplierProfile_NoSpecialQuoteAndCleanStyles(t *testing.T) {
	supplier := &org.Organization{
		ID:        51,
		TradeName: i18n.Text{"ar": "شركة ويزر فارما", "en": "Wizar Pharma"},
		LegalName: "شركة ويزر فارما للتوزيع",
		Type:      org.TypeVendor,
		Status:    org.StatusApproved,
	}

	variant := &catalog.ProductVariant{
		ID:             30784,
		ProductID:      1001,
		OrganizationID: 51,
		Name:           i18n.Text{"ar": "اباندروكير كبسول للهشاشه", "en": "Ibandrocare Capsules"},
		Price:          money.FromMajor(80),
		Status:         catalog.ProductStatus("active"),
		StockQty:       150,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	data := pages.SupplierProfileData{
		Org:           supplier,
		Variants:      []*catalog.ProductVariant{variant},
		TotalVariants: 1,
		ActiveTab:     "catalog",
		VariantMeta: map[int64]pages.SupplierVariantMeta{
			30784: {
				AvailableStock: 150,
				MinOrderQty:    1,
				IsCovered:      true,
				CanAddToCart:   true,
			},
		},
	}

	var buf bytes.Buffer
	ctx := context.Background()
	err := pages.SupplierProfile("ar", "rtl", data).Render(ctx, &buf)
	if err != nil {
		t.Fatalf("Failed to render SupplierProfile: %v", err)
	}

	html := buf.String()

	// 1. Must NOT contain "طلب عرض أسعار خاص" anywhere
	if strings.Contains(html, "طلب عرض أسعار خاص") || strings.Contains(html, "طلب عرض اسعار خاص") {
		t.Errorf("Forbidden 'طلب عرض أسعار خاص' found in SupplierProfile HTML!")
	}
	if strings.Contains(html, "quote-modal") {
		t.Errorf("Forbidden 'quote-modal' found in SupplierProfile HTML!")
	}

	// 2. Tab IDs and script wiring must match tab-btn-
	if !strings.Contains(html, `id="tab-btn-catalog"`) {
		t.Errorf("Expected id=\"tab-btn-catalog\" in tabs")
	}
	if !strings.Contains(html, `id="tab-btn-policies"`) {
		t.Errorf("Expected id=\"tab-btn-policies\" in tabs")
	}
	if !strings.Contains(html, `id="tab-btn-branches"`) {
		t.Errorf("Expected id=\"tab-btn-branches\" in tabs")
	}
	if !strings.Contains(html, `id="tab-btn-reviews"`) {
		t.Errorf("Expected id=\"tab-btn-reviews\" in tabs")
	}

	// 3. Tab switching script must query 'tab-btn-' + t and toggle d-none
	if !strings.Contains(html, `document.getElementById('tab-btn-' + t)`) {
		t.Errorf("Expected script to query 'tab-btn-' + t for active buttons")
	}
	if !strings.Contains(html, `panel.classList.remove('d-none')`) {
		t.Errorf("Expected script to remove 'd-none' from active panel")
	}

	// 4. Must render catalog product with add to cart
	if !strings.Contains(html, "اباندروكير كبسول للهشاشه") {
		t.Errorf("Expected product variant name in catalog HTML")
	}
	if !strings.Contains(html, "متوفر (150 عبوة)") {
		t.Errorf("Expected stock badge in catalog HTML")
	}

	// 5. Must have zero inline styles (Strict: no inline ever)
	if strings.Contains(html, `style=`) {
		t.Errorf("SupplierProfile contains forbidden inline style: %s", html)
	}
}
