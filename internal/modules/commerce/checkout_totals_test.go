package commerce

import (
	"os"
	"strings"
	"testing"
)

// An order's money figures have to agree with the sentence the order screen
// prints: إجمالي الأصناف − إجمالي الخصم + الشحن + الضريبة = الصافي.
//
// Checkout stored a subtotal already net of line discounts and the discount
// total beside it, so that sum came out short by exactly the discount. Editing
// the same order recomputed the subtotal gross, so one order's subtotal meant
// two different things depending on whether anyone had touched it. This pins
// the gross convention on the side that creates orders; the postgres package's
// order_edit_test.go pins the side that changes them.
func TestCheckoutStoresAGrossSubtotal(t *testing.T) {
	body, err := os.ReadFile("service.go")
	if err != nil {
		t.Fatalf("read service.go: %v", err)
	}
	src := string(body)

	if !strings.Contains(src, "orderSubtotal, addErr = orderSubtotal.Add(lineSubtotal)") {
		t.Error("the order subtotal is not accumulated gross")
	}
	if strings.Contains(src, "orderSubtotal, addErr = orderSubtotal.Add(lineTotal)") {
		t.Error("the order subtotal is still accumulated net of line discounts")
	}
	if !strings.Contains(src, "shipmentSubtotal, addErr = shipmentSubtotal.Add(lineGross)") {
		t.Error("a shipment's subtotal is not accumulated gross")
	}

	// The discount has to be stored, not zeroed: a subtotal that is gross and a
	// DiscountAmount of zero would report the pre-discount figure as the amount
	// owed on every screen that reads those two fields.
	if strings.Contains(src, "DiscountAmount:    money.Zero,") {
		t.Error("the order's discount is zeroed beside a gross subtotal")
	}

	// The offer's minimum is about what the customer actually pays, so the gate
	// stays on the net figure even though the stored subtotal is gross.
	if !strings.Contains(src, "orderNet.Minor() < input.MinOrderAmount.Minor()") {
		t.Error("the offer minimum is no longer measured against the net total")
	}
}
