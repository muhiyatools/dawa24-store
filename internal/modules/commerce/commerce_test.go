package commerce

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

type mockCommerceRepo struct {
	orders    map[int64]*Order
	shipments map[int64][]*OrderShipment
	lines     map[int64][]*OrderLine
	history   map[int64][]*OrderStatusHistory
	wishlist  map[int64][]int64
	quotes    map[int64]*QuoteRequest
	nextID    int64
}

func newMockCommerceRepo() *mockCommerceRepo {
	return &mockCommerceRepo{
		orders:    map[int64]*Order{},
		shipments: map[int64][]*OrderShipment{},
		history:   map[int64][]*OrderStatusHistory{},
		lines:     map[int64][]*OrderLine{},
		wishlist:  map[int64][]int64{},
		quotes:    map[int64]*QuoteRequest{},
		nextID:    1,
	}
}

func (m *mockCommerceRepo) GetOrCreateCart(_ context.Context, userID int64) (*Cart, error) {
	return &Cart{ID: 1, UserID: userID}, nil
}
func (m *mockCommerceRepo) GetCartWithItems(_ context.Context, cartID int64) (*Cart, error) {
	return &Cart{ID: cartID, UserID: 100}, nil
}
func (m *mockCommerceRepo) AddToCartItem(_ context.Context, cartID int64, item *CartItem) error {
	return nil
}
func (m *mockCommerceRepo) RemoveCartItem(_ context.Context, cartID int64, variantID int64) error {
	return nil
}
func (m *mockCommerceRepo) ClearCart(_ context.Context, cartID int64) error { return nil }

func (m *mockCommerceRepo) CreateOrder(
	_ context.Context,
	order *Order,
	shipments []*OrderShipment,
	lines []*OrderLine,
) error {
	order.ID = m.nextID
	m.nextID++
	for i, s := range shipments {
		s.ID = m.nextID + int64(i)
		s.OrderID = order.ID
	}
	m.orders[order.ID] = order
	m.shipments[order.ID] = shipments
	m.lines[order.ID] = lines
	return nil
}

func (m *mockCommerceRepo) GetOrderByID(_ context.Context, id int64) (*Order, error) {
	o, ok := m.orders[id]
	if !ok {
		return nil, apperr.NotFound("order")
	}
	return o, nil
}

func (m *mockCommerceRepo) GetOrderByNumber(_ context.Context, number string) (*Order, error) {
	for _, o := range m.orders {
		if o.OrderNumber == number {
			return o, nil
		}
	}
	return nil, apperr.NotFound("order")
}

func (m *mockCommerceRepo) UpdateOrderStatus(
	_ context.Context,
	orderID int64,
	toStatus OrderStatus,
	history OrderStatusHistory,
) error {
	o, ok := m.orders[orderID]
	if !ok {
		return apperr.NotFound("order")
	}
	if !IsValidStatusTransition(o.Status, toStatus) {
		return apperr.Validation("status.invalid", "Invalid transition", nil)
	}
	o.Status = toStatus
	return nil
}

func (m *mockCommerceRepo) ListOrdersByCustomer(_ context.Context, customerID int64, limit, offset int) ([]*Order, error) {
	var list []*Order
	for _, o := range m.orders {
		if o.CustomerID == customerID {
			list = append(list, o)
		}
	}
	return list, nil
}

func (m *mockCommerceRepo) ListShipmentsByVendor(_ context.Context, vendorOrgID int64, limit, offset int) ([]*OrderShipment, error) {
	var list []*OrderShipment
	for _, sList := range m.shipments {
		for _, s := range sList {
			if s.OrganizationID == vendorOrgID {
				list = append(list, s)
			}
		}
	}
	return list, nil
}

func (m *mockCommerceRepo) AddToWishlist(_ context.Context, userID int64, productID int64) error {
	m.wishlist[userID] = append(m.wishlist[userID], productID)
	return nil
}
func (m *mockCommerceRepo) RemoveFromWishlist(_ context.Context, userID int64, productID int64) error {
	var remaining []int64
	for _, id := range m.wishlist[userID] {
		if id != productID {
			remaining = append(remaining, id)
		}
	}
	m.wishlist[userID] = remaining
	return nil
}
func (m *mockCommerceRepo) ListWishlist(_ context.Context, userID int64) ([]*WishlistItem, error) {
	var items []*WishlistItem
	for _, pID := range m.wishlist[userID] {
		items = append(items, &WishlistItem{UserID: userID, ProductID: pID})
	}
	return items, nil
}

func (m *mockCommerceRepo) CreateQuoteRequest(_ context.Context, q *QuoteRequest) error {
	q.ID = m.nextID
	m.nextID++
	m.quotes[q.ID] = q
	return nil
}
func (m *mockCommerceRepo) GetQuoteRequestByID(_ context.Context, id int64) (*QuoteRequest, error) {
	q, ok := m.quotes[id]
	if !ok {
		return nil, apperr.NotFound("quote")
	}
	return q, nil
}
func (m *mockCommerceRepo) UpdateQuoteStatus(_ context.Context, id int64, status QuoteStatus, quotePrice money.Amount, supplierNotes string) error {
	q, ok := m.quotes[id]
	if !ok {
		return apperr.NotFound("quote")
	}
	q.Status = status
	q.QuoteUnitPrice = quotePrice
	q.SupplierNotes = supplierNotes
	return nil
}
func (m *mockCommerceRepo) ListQuoteRequestsByOrg(_ context.Context, orgID int64, isVendor bool, limit, offset int) ([]*QuoteRequest, error) {
	var list []*QuoteRequest
	for _, q := range m.quotes {
		if isVendor && q.OrganizationID == orgID {
			list = append(list, q)
		} else if !isVendor && q.CustomerOrgID == orgID {
			list = append(list, q)
		}
	}
	return list, nil
}

func (m *mockCommerceRepo) AdminSearchOrders(_ context.Context, query string, limit, offset int) ([]*Order, error) {
	var list []*Order
	for _, o := range m.orders {
		list = append(list, o)
	}
	return list, nil
}

func (m *mockCommerceRepo) MonthSalesByVendor(_ context.Context, _ int64) (money.Amount, error) {
	return money.Zero, nil
}

func (m *mockCommerceRepo) MonthSpendByCustomer(_ context.Context, _ int64) (money.Amount, error) {
	return money.Zero, nil
}

func (m *mockCommerceRepo) SetShipmentTracking(_ context.Context, _ int64, _, _ string) error {
	return nil
}

func TestCheckoutCalculationsAndSplitting(t *testing.T) {
	repo := newMockCommerceRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := NewService(repo, logger)

	ctx := context.Background()

	p1ID := int64(101)
	p2ID := int64(102)

	input := CheckoutInput{
		CustomerID:    123,
		PaymentMethod: "cash_on_delivery",
		Items: []CheckoutLineItem{
			{
				VendorOrgID: 10,
				ProductID:   &p1ID,
				ProductName: i18n.New("بانادول", "Panadol"),
				UnitPrice:   money.MustParse("20.00"),
				Quantity:    3,
			},
			{
				VendorOrgID: 20,
				ProductID:   &p2ID,
				ProductName: i18n.New("أوجمنتين", "Augmentin"),
				UnitPrice:   money.MustParse("50.00"),
				Quantity:    2,
			},
		},
	}

	order, err := svc.Checkout(ctx, input)
	if err != nil {
		t.Fatalf("Checkout failed: %v", err)
	}

	// 3 * 20.00 = 60.00; 2 * 50.00 = 100.00 => Total 160.00
	expectedSubtotal := money.MustParse("160.00")
	if order.Subtotal != expectedSubtotal {
		t.Errorf("Subtotal = %v; want %v", order.Subtotal, expectedSubtotal)
	}

	shipments := repo.shipments[order.ID]
	if len(shipments) != 2 {
		t.Fatalf("expected 2 shipments for 2 vendors, got %d", len(shipments))
	}

	// Sum of shipments must equal order subtotal
	var sumShipments money.Amount
	for _, s := range shipments {
		var err error
		sumShipments, err = sumShipments.Add(s.Subtotal)
		if err != nil {
			t.Fatalf("money add failed: %v", err)
		}
	}

	if sumShipments != order.Subtotal {
		t.Errorf("Sum of shipment subtotals (%v) != Order subtotal (%v)", sumShipments, order.Subtotal)
	}
}

// TestIsValidStatusTransition pins the Laravel-parity DAG (migration 063).
func TestIsValidStatusTransition(t *testing.T) {
	valid := []struct{ from, to OrderStatus }{
		{StatusPending, StatusProcessing},
		{StatusPending, StatusConfirmed},
		{StatusPending, StatusOnHold},
		{StatusPending, StatusCancelled},
		{StatusProcessing, StatusConfirmed},
		{StatusProcessing, StatusOnHold},
		{StatusProcessing, StatusCancelled},
		{StatusProcessing, StatusFailed},
		{StatusConfirmed, StatusShipped},
		{StatusConfirmed, StatusOnHold},
		{StatusConfirmed, StatusCancelled},
		{StatusConfirmed, StatusFailed},
		{StatusOnHold, StatusProcessing},
		{StatusOnHold, StatusConfirmed},
		{StatusOnHold, StatusCancelled},
		{StatusOnHold, StatusFailed},
		{StatusShipped, StatusInTransit},
		{StatusShipped, StatusOutForDelivery},
		{StatusShipped, StatusReturned},
		{StatusShipped, StatusFailed},
		{StatusInTransit, StatusOutForDelivery},
		{StatusInTransit, StatusReturned},
		{StatusInTransit, StatusFailed},
		{StatusOutForDelivery, StatusDelivered},
		{StatusOutForDelivery, StatusReturned},
		{StatusOutForDelivery, StatusFailed},
		{StatusDelivered, StatusCompleted},
		{StatusDelivered, StatusReturned},
		{StatusDelivered, StatusRefunded},
		{StatusCompleted, StatusRefunded},
	}
	for _, tc := range valid {
		if !IsValidStatusTransition(tc.from, tc.to) {
			t.Errorf("IsValidStatusTransition(%q, %q) = false; want true", tc.from, tc.to)
		}
	}

	invalid := []struct{ from, to OrderStatus }{
		{StatusPending, StatusShipped},        // jump forward
		{StatusShipped, StatusDelivered},      // bypass in_transit/out_for_delivery
		{StatusCancelled, StatusPending},      // terminal
		{StatusFailed, StatusRefunded},        // terminal
		{StatusReturned, StatusRefunded},      // terminal
		{StatusRefunded, StatusCompleted},     // terminal
		{StatusCompleted, StatusDelivered},    // after completion
		{StatusPending, StatusRefunded},       // refund without delivery
		{StatusDelivered, StatusCancelled},    // cancelled after delivery
		{OrderStatus("ready_for_pickup"), StatusShipped}, // removed legacy state
	}
	for _, tc := range invalid {
		if IsValidStatusTransition(tc.from, tc.to) {
			t.Errorf("IsValidStatusTransition(%q, %q) = true; want false", tc.from, tc.to)
		}
	}
}

// TestCheckoutOfferModel covers the §3.3 behavior: order carries offer/branch/
// address identity, lines carry the offer snapshots, TotalDiscount/FinalPrice
// reproduce the invoice, and the min-order gate refuses short checkouts.
func TestCheckoutOfferModel(t *testing.T) {
	repo := newMockCommerceRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := NewService(repo, logger)

	ctx := context.Background()
	variantID := int64(33)
	offerProductID := int64(7)
	offerID := int64(9)
	branchID := int64(12)
	vendorBranchID := int64(14)
	addressID := int64(5)

	input := CheckoutInput{
		CustomerID:     123,
		OfferID:        offerID,
		BranchID:       &branchID,
		VendorBranchID: &vendorBranchID,
		UserAddressID:  &addressID,
		PaymentMethod:  "cash_on_delivery",
		MinOrderAmount: money.MustParse("500.00"),
		Items: []CheckoutLineItem{
			{
				VendorOrgID:      10,
				ProductVariantID: &variantID,
				ProductName:      i18n.New("بانادول", "Panadol"),
				UnitPrice:        money.MustParse("45.00"),
				Quantity:         20,
				OfferProductID:   &offerProductID,
				DiscountAmount:   money.MustParse("4.00"),
				ListPrice:        money.MustParse("49.00"),
				OriginalPrice:    money.MustParse("50.00"),
				OriginalDiscount: money.MustParse("5.00"),
			},
		},
	}

	order, err := svc.Checkout(ctx, input)
	if err != nil {
		t.Fatalf("Checkout failed: %v", err)
	}

	if order.OfferID != offerID {
		t.Errorf("Order.OfferID = %d; want %d", order.OfferID, offerID)
	}
	if order.BranchID == nil || *order.BranchID != branchID {
		t.Errorf("Order.BranchID = %v; want %d", order.BranchID, branchID)
	}
	if order.VendorBranchID == nil || *order.VendorBranchID != vendorBranchID {
		t.Errorf("Order.VendorBranchID = %v; want %d", order.VendorBranchID, vendorBranchID)
	}
	if order.UserAddressID == nil || *order.UserAddressID != addressID {
		t.Errorf("Order.UserAddressID = %v; want %d", order.UserAddressID, addressID)
	}

	wantTotalDiscount := money.MustParse("4.00")
	if order.TotalDiscount != wantTotalDiscount {
		t.Errorf("Order.TotalDiscount = %v; want %v", order.TotalDiscount, wantTotalDiscount)
	}
	wantFinal := money.MustParse("896.00") // 20 × 45.00 − 4.00 discount
	if order.FinalPrice != wantFinal {
		t.Errorf("Order.FinalPrice = %v; want %v", order.FinalPrice, wantFinal)
	}

	lines := repo.lines[order.ID]
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}
	line := lines[0]
	if line.OfferProductID == nil || *line.OfferProductID != offerProductID {
		t.Errorf("Line.OfferProductID = %v; want %d", line.OfferProductID, offerProductID)
	}
	if line.ListPrice != money.MustParse("49.00") {
		t.Errorf("Line.ListPrice = %v; want 49.00", line.ListPrice)
	}
	if line.OriginalPrice != money.MustParse("50.00") {
		t.Errorf("Line.OriginalPrice = %v; want 50.00", line.OriginalPrice)
	}

	// Min-order gate: same order below the minimum is refused.
	lowInput := input
	lowInput.MinOrderAmount = money.MustParse("1000.00")
	if _, err := svc.Checkout(ctx, lowInput); err == nil {
		t.Fatal("Checkout below min_order_amount succeeded; want validation error")
	}

	// A zero/absent minimum does not gate.
	noMin := input
	noMin.MinOrderAmount = money.Zero
	if _, err := svc.Checkout(ctx, noMin); err != nil {
		t.Fatalf("Checkout without min_order_amount failed: %v", err)
	}
}
