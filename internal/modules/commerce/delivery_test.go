package commerce_test

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

type deliveryMockRepo struct {
	shipments map[string]*commerce.OrderShipment
}

func (m *deliveryMockRepo) GetOrCreateCart(_ context.Context, _ int64) (*commerce.Cart, error) {
	return nil, nil
}
func (m *deliveryMockRepo) GetCartWithItems(_ context.Context, _ int64) (*commerce.Cart, error) {
	return nil, nil
}
func (m *deliveryMockRepo) AddToCartItem(_ context.Context, _ int64, _ *commerce.CartItem) error {
	return nil
}
func (m *deliveryMockRepo) SetCartItemQuantity(_ context.Context, _, _ int64, _ int) error {
	return nil
}
func (m *deliveryMockRepo) RemoveCartItem(_ context.Context, _, _ int64) error { return nil }

func (m *deliveryMockRepo) RemoveCartItemByID(_ context.Context, _, _ int64) error { return nil }

func (m *deliveryMockRepo) SetCartItemQuantityByID(_ context.Context, _, _ int64, _ int) error {
	return nil
}
func (m *deliveryMockRepo) ClearCart(_ context.Context, _ int64) error { return nil }
func (m *deliveryMockRepo) CreateOrder(_ context.Context, _ *commerce.Order, _ []*commerce.OrderShipment, _ []*commerce.OrderLine) error {
	return nil
}
func (m *deliveryMockRepo) GetOrderByID(_ context.Context, _ int64) (*commerce.Order, error) {
	return nil, nil
}
func (m *deliveryMockRepo) GetOrderByNumber(_ context.Context, _ string) (*commerce.Order, error) {
	return nil, nil
}
func (m *deliveryMockRepo) UpdateOrderStatus(_ context.Context, _ int64, _ commerce.OrderStatus, _ commerce.OrderStatusHistory) error {
	return nil
}
func (m *deliveryMockRepo) UpdateCustomerPendingOrder(_ context.Context, _ *commerce.Order, _ []commerce.OrderLineEditItem, _ int64) (*commerce.Order, error) {
	return nil, nil
}
func (m *deliveryMockRepo) ListOrdersByCustomer(_ context.Context, _ int64, _, _ int) ([]*commerce.Order, error) {
	return nil, nil
}
func (m *deliveryMockRepo) ListOrdersByCustomerWithTotal(_ context.Context, _ int64, _, _ int) ([]*commerce.Order, int, error) {
	return nil, 0, nil
}
func (m *deliveryMockRepo) CountOrders(_ context.Context) (int, error) { return 0, nil }
func (m *deliveryMockRepo) CountVendorShipmentsByStatus(_ context.Context, _ int64, _ []string) (int, error) {
	return 0, nil
}
func (m *deliveryMockRepo) MonthSalesByVendor(_ context.Context, _ int64) (money.Amount, error) {
	return money.Zero, nil
}
func (m *deliveryMockRepo) MonthSpendByCustomer(_ context.Context, _ int64) (money.Amount, error) {
	return money.Zero, nil
}
func (m *deliveryMockRepo) ListShipmentsByVendor(_ context.Context, _ int64, _, _ int) ([]*commerce.OrderShipment, error) {
	return nil, nil
}
func (m *deliveryMockRepo) ListShipmentsByVendorWithTotal(_ context.Context, _ int64, _ string, _, _ int) ([]*commerce.OrderShipment, int, error) {
	return nil, 0, nil
}
func (m *deliveryMockRepo) GetShipmentByID(_ context.Context, id int64) (*commerce.OrderShipment, error) {
	for _, s := range m.shipments {
		if s.ID == id {
			c := *s
			return &c, nil
		}
	}
	return nil, nil
}
func (m *deliveryMockRepo) UpdateShipmentStatus(_ context.Context, id int64, _, to commerce.OrderStatus, _ commerce.OrderStatusHistory) error {
	for _, s := range m.shipments {
		if s.ID == id {
			s.Status = to
			return nil
		}
	}
	return nil
}
func (m *deliveryMockRepo) SetShipmentTracking(_ context.Context, id int64, carrier, tracking string) error {
	for _, s := range m.shipments {
		if s.ID == id {
			s.CarrierName = carrier
			s.TrackingNumber = tracking
			return nil
		}
	}
	return nil
}
func (m *deliveryMockRepo) ListOrderHistory(_ context.Context, _ int64) ([]*commerce.OrderStatusHistory, error) {
	return nil, nil
}
func (m *deliveryMockRepo) RateOrder(_ context.Context, _, _ int64, _ float64, _ string) error {
	return nil
}
func (m *deliveryMockRepo) AddToWishlist(_ context.Context, _, _ int64) error      { return nil }
func (m *deliveryMockRepo) RemoveFromWishlist(_ context.Context, _, _ int64) error { return nil }
func (m *deliveryMockRepo) ListWishlist(_ context.Context, _ int64) ([]*commerce.WishlistItem, error) {
	return nil, nil
}
func (m *deliveryMockRepo) CreateQuoteRequest(_ context.Context, _ *commerce.QuoteRequest) error {
	return nil
}
func (m *deliveryMockRepo) GetQuoteRequestByID(_ context.Context, _ int64) (*commerce.QuoteRequest, error) {
	return nil, nil
}
func (m *deliveryMockRepo) UpdateQuoteStatus(_ context.Context, _ int64, _ commerce.QuoteStatus, _ money.Amount, _ string) error {
	return nil
}
func (m *deliveryMockRepo) ListQuoteRequestsByOrg(_ context.Context, _ int64, _ bool, _, _ int) ([]*commerce.QuoteRequest, error) {
	return nil, nil
}
func (m *deliveryMockRepo) CreatePurchaseRequest(_ context.Context, _ *commerce.PurchaseRequest, _ []*commerce.PurchaseRequestLine) error {
	return nil
}
func (m *deliveryMockRepo) GetPurchaseRequestByID(_ context.Context, _ int64) (*commerce.PurchaseRequest, error) {
	return nil, nil
}
func (m *deliveryMockRepo) GetPurchaseRequestByNumber(_ context.Context, _ string) (*commerce.PurchaseRequest, error) {
	return nil, nil
}
func (m *deliveryMockRepo) ListPurchaseRequestsByCustomer(_ context.Context, _ int64, _ *int64, _ string, _, _ int) ([]*commerce.PurchaseRequest, error) {
	return nil, nil
}
func (m *deliveryMockRepo) ListPurchaseRequestsByVendor(_ context.Context, _ int64, _ string, _, _ int) ([]*commerce.PurchaseRequest, error) {
	return nil, nil
}
func (m *deliveryMockRepo) CountPurchaseRequestsByCustomer(_ context.Context, _ int64, _ *int64) (map[string]int, error) {
	return nil, nil
}
func (m *deliveryMockRepo) UpdatePurchaseRequestStatus(_ context.Context, _ int64, _ commerce.PurchaseRequestStatus, _ string, _ *int64) error {
	return nil
}
func (m *deliveryMockRepo) UpdatePurchaseRequestLineOffer(_ context.Context, _ int64, _ money.Amount, _ float64, _ string) error {
	return nil
}
func (m *deliveryMockRepo) AdminSearchOrders(_ context.Context, _ string, _, _ int) ([]*commerce.Order, error) {
	return nil, nil
}
func (m *deliveryMockRepo) AdminSearchOrdersWithTotal(_ context.Context, _, _ string, _, _ int) ([]*commerce.Order, int, error) {
	return nil, 0, nil
}
func (m *deliveryMockRepo) AdminOrderStats(_ context.Context) (int, int, int, error) {
	return 0, 0, 0, nil
}
func (m *deliveryMockRepo) AcceptNegotiation(_ context.Context, _, _ int64) error { return nil }
func (m *deliveryMockRepo) RejectNegotiation(_ context.Context, _ int64, _ string, _ int64) error {
	return nil
}

func (m *deliveryMockRepo) GetShipmentForDeliveryByTracking(_ context.Context, tracking string) (*commerce.OrderShipment, error) {
	for _, s := range m.shipments {
		if s.TrackingNumber == tracking || s.ShipmentNumber == tracking || s.PublicID == tracking {
			c := *s
			return &c, nil
		}
	}
	return nil, nil
}

func (m *deliveryMockRepo) VerifyAndCompleteDelivery(
	_ context.Context,
	shipmentID int64,
	deliveryCode string,
	notes string,
	collectedAmountMinor int64,
) (*commerce.OrderShipment, error) {
	for _, s := range m.shipments {
		if s.ID == shipmentID {
			now := time.Now()
			if s.DeliveryLockedUntil != nil && s.DeliveryLockedUntil.After(now) {
				return nil, commerce.ErrDeliveryLocked
			}
			if s.DeliveryCode != "" && s.DeliveryCode != deliveryCode {
				s.DeliveryAttempts++
				if s.DeliveryAttempts >= 5 {
					lockUntil := now.Add(15 * time.Minute)
					s.DeliveryLockedUntil = &lockUntil
					return nil, commerce.ErrDeliveryLocked
				}
				return nil, commerce.ErrInvalidDeliveryCode
			}
			s.Status = commerce.StatusDelivered
			s.DeliveredAt = &now
			s.DeliveredByCourierAt = &now
			s.DeliveryNotes = notes
			s.CollectedAmountMinor = collectedAmountMinor
			s.DeliveryAttempts = 0
			s.DeliveryLockedUntil = nil
			c := *s
			return &c, nil
		}
	}
	return nil, nil
}

func (m *deliveryMockRepo) GetVendorFinancialSummary(ctx context.Context, vendorOrgID int64, period string) (*commerce.VendorFinancialSummary, error) {
	return &commerce.VendorFinancialSummary{Period: period}, nil
}

func TestCourierDeliveryPINAndTrackingHelpers(t *testing.T) {
	pin1 := commerce.GenerateDeliveryCode()
	pin2 := commerce.GenerateDeliveryCode()
	if len(pin1) != 6 || len(pin2) != 6 {
		t.Fatalf("expected 6-digit delivery PIN, got %s, %s", pin1, pin2)
	}

	trk := commerce.GenerateTrackingNumber("ORD-100", 1)
	if len(trk) < 8 || trk[:3] != "TRK" {
		t.Fatalf("expected formatted tracking number, got %s", trk)
	}
}

func TestCourierDeliveryLifecycle_Success(t *testing.T) {
	repo := &deliveryMockRepo{
		shipments: map[string]*commerce.OrderShipment{
			"TRK-123456": {
				ID:             1,
				ShipmentNumber: "SH-1",
				TrackingNumber: "TRK-123456",
				Status:         commerce.StatusShipped,
				DeliveryCode:   "482915",
				TotalAmount:    money.FromMinor(54000),
			},
		},
	}

	svc := commerce.NewService(repo, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx := database.WithTenant(context.Background(), 10)

	// 1. Courier looks up shipment by tracking number
	sh, err := svc.GetShipmentForDelivery(ctx, "TRK-123456")
	if err != nil || sh == nil {
		t.Fatalf("GetShipmentForDelivery failed: %v", err)
	}
	if sh.ShipmentNumber != "SH-1" {
		t.Errorf("got shipment number %s, want SH-1", sh.ShipmentNumber)
	}

	// 2. Courier enters correct 6-digit delivery PIN
	completed, err := svc.VerifyAndCompleteDelivery(ctx, "TRK-123456", "482915", "تم التحصيل كاش", 54000)
	if err != nil || completed == nil {
		t.Fatalf("VerifyAndCompleteDelivery failed: %v", err)
	}

	if completed.Status != commerce.StatusDelivered {
		t.Errorf("status = %v, want delivered", completed.Status)
	}
	if completed.DeliveredByCourierAt == nil {
		t.Errorf("expected DeliveredByCourierAt to be set")
	}
	if completed.DeliveryNotes != "تم التحصيل كاش" {
		t.Errorf("delivery notes = %q, want 'تم التحصيل كاش'", completed.DeliveryNotes)
	}
}
