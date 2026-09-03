package ui

import (
	"errors"
	"testing"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/promo"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// Special offers that must not be sold are refused with a specific reason.
func TestValidateSpecialOfferForCheckout(t *testing.T) {
	past := time.Now().Add(-48 * time.Hour)
	future := time.Now().Add(48 * time.Hour)

	active := &promo.SpecialOffer{
		ID: 7, OrganizationID: 3, Status: "active", AdminStatus: "approved",
		StartDate: &past, EndDate: &future,
	}
	if msg := validateSpecialOfferForCheckout(active); msg != "" {
		t.Errorf("active approved offer refused: %q", msg)
	}
	if msg := validateSpecialOfferForCheckout(nil); msg == "" {
		t.Error("nil offer accepted, want refusal")
	}

	cases := []struct {
		name   string
		mutate func(*promo.SpecialOffer)
	}{
		{"inactive", func(o *promo.SpecialOffer) { o.Status = "inactive" }},
		{"draft", func(o *promo.SpecialOffer) { o.Status = "draft" }},
		{"expired_status", func(o *promo.SpecialOffer) { o.Status = "expired" }},
		{"admin_pending", func(o *promo.SpecialOffer) { o.AdminStatus = "pending" }},
		{"admin_rejected", func(o *promo.SpecialOffer) { o.AdminStatus = "rejected" }},
		{"not_started", func(o *promo.SpecialOffer) { o.StartDate = &future }},
		{"past_end", func(o *promo.SpecialOffer) { o.EndDate = &past }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cp := *active
			tc.mutate(&cp)
			if msg := validateSpecialOfferForCheckout(&cp); msg == "" {
				t.Errorf("%s offer accepted, want specific refusal message", tc.name)
			}
		})
	}
}

// Checkout validation failures map to specific Arabic messages instead of the
// generic envelope, so the pharmacy knows what to fix.
func TestCheckoutValidationMessage(t *testing.T) {
	minErr := apperr.Validation("checkout.min_order_not_met", "below minimum", map[string]string{
		"order_total": "200.00", "min_order_total": "500.00",
	})
	msg, ok := checkoutValidationMessage("ar", minErr)
	if !ok || msg == "" {
		t.Fatalf("min_order_not_met not mapped: %q %v", msg, ok)
	}

	vendorErr := apperr.Validation("item.vendor_required", "vendor missing", nil)
	if msg, ok := checkoutValidationMessage("ar", vendorErr); !ok || msg == "" {
		t.Errorf("vendor_required not mapped: %q %v", msg, ok)
	}

	lineErr := apperr.Validation("checkout.line_unavailable.out_of_stock", "نفد المخزون", nil)
	if msg, ok := checkoutValidationMessage("ar", lineErr); !ok || msg == "" {
		t.Errorf("line_unavailable not mapped: %q %v", msg, ok)
	}

	// Unknown codes and non-validation errors keep the generic path.
	unknown := apperr.Validation("checkout.something_new", "new", nil)
	if _, ok := checkoutValidationMessage("ar", unknown); ok {
		t.Error("unknown code mapped, want generic path")
	}
	if _, ok := checkoutValidationMessage("ar", apperr.Internal(errors.New("boom"))); ok {
		t.Error("internal error mapped, want generic path")
	}
}
