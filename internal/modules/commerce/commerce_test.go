package commerce_test

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

type mockCommerceRepo struct {
	orders    map[int64]*commerce.Order
	shipments map[int64][]*commerce.OrderShipment
	lines     map[int64][]*commerce.OrderLine
	nextID    int64
}

func newMockCommerceRepo() *mockCommerceRepo {
	return &mockCommerceRepo{
		orders:    map[int64]*commerce.Order{},
		shipments: map[int64][]*commerce.OrderShipment{},
		lines:     map[int64][]*commerce.OrderLine{},
		nextID:    1,
	}
}

func (m *mockCommerceRepo) GetOrCreateCart(_ context.Context, userID int64) (*commerce.Cart, error) {
	return &commerce.Cart{ID: 1, UserID: userID}, nil
}
func (m *mockCommerceRepo) GetCartWithItems(_ context.Context, cartID int64) (*commerce.Cart, error) {
	return &commerce.Cart{ID: cartID}, nil
}
func (m *mockCommerceRepo) AddToCartItem(_ context.Context, cartID int64, item *commerce.CartItem) error {
	return nil
}
func (m *mockCommerceRepo) RemoveCartItem(_ context.Context, cartID int64, variantID int64) error {
	return nil
}
func (m *mockCommerceRepo) ClearCart(_ context.Context, cartID int64) error { return nil }

func (m *mockCommerceRepo) CreateOrder(
	_ context.Context,
	order *commerce.Order,
	shipments []*commerce.OrderShipment,
	lines []*commerce.OrderLine,
) error {
	order.ID = m.nextID
	m.nextID++
	m.orders[order.ID] = order
	m.shipments[order.ID] = shipments
	m.lines[order.ID] = lines
	return nil
}

func (m *mockCommerceRepo) GetOrderByID(_ context.Context, id int64) (*commerce.Order, error) {
	o, ok := m.orders[id]
	if !ok {
		return nil, apperr.NotFound("order")
	}
	return o, nil
}

func (m *mockCommerceRepo) GetOrderByNumber(_ context.Context, number string) (*commerce.Order, error) {
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
	toStatus commerce.OrderStatus,
	history commerce.OrderStatusHistory,
) error {
	o, ok := m.orders[orderID]
	if !ok {
		return apperr.NotFound("order")
	}
	if !commerce.IsValidStatusTransition(o.Status, toStatus) {
		return apperr.Validation("status.invalid", "Invalid transition", nil)
	}
	o.Status = toStatus
	return nil
}

func (m *mockCommerceRepo) ListOrdersByCustomer(_ context.Context, customerID int64, limit, offset int) ([]*commerce.Order, error) {
	var list []*commerce.Order
	for _, o := range m.orders {
		if o.CustomerID == customerID {
			list = append(list, o)
		}
	}
	return list, nil
}

func (m *mockCommerceRepo) ListShipmentsByVendor(_ context.Context, vendorOrgID int64, limit, offset int) ([]*commerce.OrderShipment, error) {
	var list []*commerce.OrderShipment
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
	return nil
}
func (m *mockCommerceRepo) RemoveFromWishlist(_ context.Context, userID int64, productID int64) error {
	return nil
}
func (m *mockCommerceRepo) ListWishlist(_ context.Context, userID int64) ([]*commerce.WishlistItem, error) {
	return nil, nil
}

func (m *mockCommerceRepo) CreateQuoteRequest(_ context.Context, q *commerce.QuoteRequest) error {
	q.ID = m.nextID
	m.nextID++
	return nil
}
func (m *mockCommerceRepo) GetQuoteRequestByID(_ context.Context, id int64) (*commerce.QuoteRequest, error) {
	return nil, apperr.NotFound("quote_request")
}
func (m *mockCommerceRepo) UpdateQuoteStatus(_ context.Context, id int64, status commerce.QuoteStatus, quotePrice money.Amount, supplierNotes string) error {
	return nil
}
func (m *mockCommerceRepo) ListQuoteRequestsByOrg(_ context.Context, orgID int64, isVendor bool, limit, offset int) ([]*commerce.QuoteRequest, error) {
	return nil, nil
}

func TestOrderStatusTransitions(t *testing.T) {
	validTransitions := [][2]commerce.OrderStatus{
		{commerce.StatusPending, commerce.StatusConfirmed},
		{commerce.StatusPending, commerce.StatusCancelled},
		{commerce.StatusConfirmed, commerce.StatusProcessing},
		{commerce.StatusProcessing, commerce.StatusShipped},
		{commerce.StatusShipped, commerce.StatusDelivered},
	}

	for _, tr := range validTransitions {
		if !commerce.IsValidStatusTransition(tr[0], tr[1]) {
			t.Errorf("expected transition %s -> %s to be valid", tr[0], tr[1])
		}
	}

	invalidTransitions := [][2]commerce.OrderStatus{
		{commerce.StatusPending, commerce.StatusDelivered},
		{commerce.StatusDelivered, commerce.StatusPending},
		{commerce.StatusCancelled, commerce.StatusConfirmed},
		{commerce.StatusRefunded, commerce.StatusProcessing},
	}

	for _, tr := range invalidTransitions {
		if commerce.IsValidStatusTransition(tr[0], tr[1]) {
			t.Errorf("expected transition %s -> %s to be invalid", tr[0], tr[1])
		}
	}
}

func TestMultiVendorCheckout(t *testing.T) {
	ctx := context.Background()
	repo := newMockCommerceRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := commerce.NewService(repo, logger)

	// Checkout 3 items across 2 distinct vendor organizations
	input := commerce.CheckoutInput{
		CustomerID:    101,
		PaymentMethod: "credit_card",
		Items: []commerce.CheckoutLineItem{
			{
				VendorOrgID: 1, // Vendor 1
				ProductName: i18n.New("كونجستال", "Congestal"),
				UnitPrice:   money.MustParse("25.00"),
				Quantity:    2,
			},
			{
				VendorOrgID: 2, // Vendor 2
				ProductName: i18n.New("كتافلام 50", "Cataflam 50"),
				UnitPrice:   money.MustParse("35.50"),
				Quantity:    1,
			},
			{
				VendorOrgID: 1, // Vendor 1
				ProductName: i18n.New("أوجمنتين 1 جم", "Augmentin 1g"),
				UnitPrice:   money.MustParse("90.00"),
				Quantity:    1,
			},
		},
	}

	order, err := svc.Checkout(ctx, input)
	if err != nil {
		t.Fatalf("Checkout failed: %v", err)
	}

	// Total = (25.00 * 2) + (35.50 * 1) + (90.00 * 1) = 50.00 + 35.50 + 90.00 = 175.50
	expectedTotal := money.MustParse("175.50")
	if order.TotalAmount != expectedTotal {
		t.Errorf("order.TotalAmount = %v; want %v", order.TotalAmount, expectedTotal)
	}

	// Check that 2 vendor shipments were created
	if len(order.Shipments) != 2 {
		t.Fatalf("expected 2 shipments, got %d", len(order.Shipments))
	}
}

func TestWishlistOperations(t *testing.T) {
	ctx := context.Background()
	repo := newMockCommerceRepo()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := commerce.NewService(repo, logger)

	err := svc.AddToWishlist(ctx, 100, 42)
	if err != nil {
		t.Fatalf("AddToWishlist failed: %v", err)
	}

	err = svc.RemoveFromWishlist(ctx, 100, 42)
	if err != nil {
		t.Fatalf("RemoveFromWishlist failed: %v", err)
	}
}
