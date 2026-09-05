package ui_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/ui"
)

type courierMockCommerceRepo struct {
	commerce.Repository
	shipment *commerce.OrderShipment
}

func (m *courierMockCommerceRepo) GetOrCreateCart(_ context.Context, _ int64) (*commerce.Cart, error) {
	return nil, nil
}
func (m *courierMockCommerceRepo) GetCartWithItems(_ context.Context, _ int64) (*commerce.Cart, error) {
	return nil, nil
}
func (m *courierMockCommerceRepo) AddToCartItem(_ context.Context, _ int64, _ *commerce.CartItem) error {
	return nil
}
func (m *courierMockCommerceRepo) SetCartItemQuantity(_ context.Context, _, _ int64, _ int) error {
	return nil
}
func (m *courierMockCommerceRepo) RemoveCartItem(_ context.Context, _, _ int64) error { return nil }

func (m *courierMockCommerceRepo) RemoveCartItemByID(_ context.Context, _, _ int64) error {
	return nil
}

func (m *courierMockCommerceRepo) SetCartItemQuantityByID(_ context.Context, _, _ int64, _ int) error {
	return nil
}
func (m *courierMockCommerceRepo) ClearCart(_ context.Context, _ int64) error { return nil }
func (m *courierMockCommerceRepo) CreateOrder(_ context.Context, _ *commerce.Order, _ []*commerce.OrderShipment, _ []*commerce.OrderLine) error {
	return nil
}
func (m *courierMockCommerceRepo) GetOrderByID(_ context.Context, id int64) (*commerce.Order, error) {
	return &commerce.Order{ID: id, OrderNumber: "ORD-999", CustomerID: 10}, nil
}
func (m *courierMockCommerceRepo) GetOrderByNumber(_ context.Context, _ string) (*commerce.Order, error) {
	return nil, nil
}
func (m *courierMockCommerceRepo) UpdateOrderStatus(_ context.Context, _ int64, _ commerce.OrderStatus, _ commerce.OrderStatusHistory) error {
	return nil
}
func (m *courierMockCommerceRepo) UpdateCustomerPendingOrder(_ context.Context, _ *commerce.Order, _ []commerce.OrderLineEditItem, _ int64) (*commerce.Order, error) {
	return nil, nil
}
func (m *courierMockCommerceRepo) ListOrdersByCustomer(_ context.Context, _ int64, _, _ int) ([]*commerce.Order, error) {
	return nil, nil
}
func (m *courierMockCommerceRepo) ListOrdersByCustomerWithTotal(_ context.Context, _ int64, _, _ int) ([]*commerce.Order, int, error) {
	return nil, 0, nil
}
func (m *courierMockCommerceRepo) CountOrders(_ context.Context) (int, error) { return 0, nil }
func (m *courierMockCommerceRepo) CountVendorShipmentsByStatus(_ context.Context, _ int64, _ []string) (int, error) {
	return 0, nil
}
func (m *courierMockCommerceRepo) MonthSalesByVendor(_ context.Context, _ int64) (money.Amount, error) {
	return money.Zero, nil
}
func (m *courierMockCommerceRepo) MonthSpendByCustomer(_ context.Context, _ int64) (money.Amount, error) {
	return money.Zero, nil
}
func (m *courierMockCommerceRepo) ListShipmentsByVendor(_ context.Context, _ int64, _, _ int) ([]*commerce.OrderShipment, error) {
	return nil, nil
}
func (m *courierMockCommerceRepo) ListShipmentsByVendorWithTotal(_ context.Context, _ int64, _ string, _, _ int) ([]*commerce.OrderShipment, int, error) {
	return nil, 0, nil
}
func (m *courierMockCommerceRepo) GetShipmentByID(_ context.Context, id int64) (*commerce.OrderShipment, error) {
	if m.shipment != nil && m.shipment.ID == id {
		c := *m.shipment
		return &c, nil
	}
	return nil, apperr.NotFound("shipment")
}
func (m *courierMockCommerceRepo) UpdateShipmentStatus(_ context.Context, id int64, _, to commerce.OrderStatus, _ commerce.OrderStatusHistory) error {
	if m.shipment != nil && m.shipment.ID == id {
		m.shipment.Status = to
		return nil
	}
	return nil
}
func (m *courierMockCommerceRepo) SetShipmentTracking(_ context.Context, id int64, carrier, tracking string) error {
	if m.shipment != nil && m.shipment.ID == id {
		m.shipment.CarrierName = carrier
		m.shipment.TrackingNumber = tracking
		return nil
	}
	return nil
}
func (m *courierMockCommerceRepo) ListOrderHistory(_ context.Context, _ int64) ([]*commerce.OrderStatusHistory, error) {
	return nil, nil
}
func (m *courierMockCommerceRepo) RateOrder(_ context.Context, _, _ int64, _ float64, _ string) error {
	return nil
}
func (m *courierMockCommerceRepo) AddToWishlist(_ context.Context, _, _ int64) error      { return nil }
func (m *courierMockCommerceRepo) RemoveFromWishlist(_ context.Context, _, _ int64) error { return nil }
func (m *courierMockCommerceRepo) ListWishlist(_ context.Context, _ int64) ([]*commerce.WishlistItem, error) {
	return nil, nil
}
func (m *courierMockCommerceRepo) CreateQuoteRequest(_ context.Context, _ *commerce.QuoteRequest) error {
	return nil
}
func (m *courierMockCommerceRepo) GetQuoteRequestByID(_ context.Context, _ int64) (*commerce.QuoteRequest, error) {
	return nil, nil
}
func (m *courierMockCommerceRepo) UpdateQuoteStatus(_ context.Context, _ int64, _ commerce.QuoteStatus, _ money.Amount, _ string) error {
	return nil
}
func (m *courierMockCommerceRepo) ListQuoteRequestsByOrg(_ context.Context, _ int64, _ bool, _, _ int) ([]*commerce.QuoteRequest, error) {
	return nil, nil
}
func (m *courierMockCommerceRepo) CreatePurchaseRequest(_ context.Context, _ *commerce.PurchaseRequest, _ []*commerce.PurchaseRequestLine) error {
	return nil
}
func (m *courierMockCommerceRepo) GetPurchaseRequestByID(_ context.Context, _ int64) (*commerce.PurchaseRequest, error) {
	return nil, nil
}
func (m *courierMockCommerceRepo) GetPurchaseRequestByNumber(_ context.Context, _ string) (*commerce.PurchaseRequest, error) {
	return nil, nil
}
func (m *courierMockCommerceRepo) ListPurchaseRequestsByCustomer(_ context.Context, _ int64, _ *int64, _ string, _, _ int) ([]*commerce.PurchaseRequest, error) {
	return nil, nil
}
func (m *courierMockCommerceRepo) ListPurchaseRequestsByVendor(_ context.Context, _ int64, _ string, _, _ int) ([]*commerce.PurchaseRequest, error) {
	return nil, nil
}
func (m *courierMockCommerceRepo) CountPurchaseRequestsByCustomer(_ context.Context, _ int64, _ *int64) (map[string]int, error) {
	return nil, nil
}
func (m *courierMockCommerceRepo) UpdatePurchaseRequestStatus(_ context.Context, _ int64, _ commerce.PurchaseRequestStatus, _ string, _ *int64) error {
	return nil
}
func (m *courierMockCommerceRepo) UpdatePurchaseRequestLineOffer(_ context.Context, _ int64, _ money.Amount, _ float64, _ string) error {
	return nil
}
func (m *courierMockCommerceRepo) AdminSearchOrders(_ context.Context, _ string, _, _ int) ([]*commerce.Order, error) {
	return nil, nil
}
func (m *courierMockCommerceRepo) AdminSearchOrdersWithTotal(_ context.Context, _, _ string, _, _ int) ([]*commerce.Order, int, error) {
	return nil, 0, nil
}
func (m *courierMockCommerceRepo) AdminOrderStats(_ context.Context) (int, int, int, error) {
	return 0, 0, 0, nil
}
func (m *courierMockCommerceRepo) AcceptNegotiation(_ context.Context, _, _ int64) error { return nil }
func (m *courierMockCommerceRepo) RejectNegotiation(_ context.Context, _ int64, _ string, _ int64) error {
	return nil
}
func (m *courierMockCommerceRepo) GetVendorFinancialSummary(_ context.Context, _ int64, period string) (*commerce.VendorFinancialSummary, error) {
	return &commerce.VendorFinancialSummary{Period: period}, nil
}

func (m *courierMockCommerceRepo) GetShipmentForDeliveryByTracking(_ context.Context, tracking string) (*commerce.OrderShipment, error) {
	if m.shipment != nil && (m.shipment.TrackingNumber == tracking || m.shipment.ShipmentNumber == tracking || m.shipment.PublicID == tracking) {
		c := *m.shipment
		return &c, nil
	}
	return nil, apperr.NotFound("shipment")
}

func (m *courierMockCommerceRepo) VerifyAndCompleteDelivery(
	_ context.Context,
	shipmentID int64,
	deliveryCode string,
	notes string,
	collectedAmountMinor int64,
) (*commerce.OrderShipment, error) {
	if m.shipment != nil && m.shipment.ID == shipmentID {
		if m.shipment.DeliveryCode != "" && m.shipment.DeliveryCode != deliveryCode {
			return nil, apperr.Validation("delivery.invalid_code", "كود تأكيد الاستلام غير صحيح.", nil)
		}
		m.shipment.Status = commerce.StatusDelivered
		now := time.Now()
		m.shipment.DeliveredAt = &now
		m.shipment.DeliveredByCourierAt = &now
		m.shipment.DeliveryNotes = notes
		m.shipment.CollectedAmountMinor = collectedAmountMinor
		c := *m.shipment
		return &c, nil
	}
	return nil, apperr.NotFound("shipment")
}

func TestCourierDeliveryHandlers(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	mockShipment := &commerce.OrderShipment{
		ID:                      101,
		OrderID:                 505,
		ShipmentNumber:          "SH-2026-001",
		TrackingNumber:          "TRK-987654",
		Status:                  commerce.StatusShipped,
		DeliveryCode:            "654321",
		TotalAmount:             money.FromMinor(125000),
		CustomerOrgName:         i18n.New("صيدلية النور الحديثة", "Al-Noor Modern Pharmacy"),
		CustomerBranchName:      i18n.New("فرع المعادي", "Maadi Branch"),
		CustomerBranchAddress:   "شارع النصر، أمام مستشفى المعادي، القاهرة",
		CustomerBranchPhone:     "01012345678",
		CustomerBranchLatitude:  func(f float64) *float64 { return &f }(29.9602),
		CustomerBranchLongitude: func(f float64) *float64 { return &f }(31.2825),
		Lines: []*commerce.OrderLine{
			{
				ID:          1,
				ProductName: i18n.New("بنادول إكسترا 24 قرص", "Panadol Extra 24 Tablets"),
				Quantity:    10,
				UnitPrice:   money.FromMinor(4500),
				TotalPrice:  money.FromMinor(45000),
			},
			{
				ID:          2,
				ProductName: i18n.New("أوجمنتين 1 جم 14 قرص", "Augmentin 1g 14 Tablets"),
				Quantity:    5,
				UnitPrice:   money.FromMinor(8500),
				TotalPrice:  money.FromMinor(42500),
			},
			{
				ID:          3,
				ProductName: i18n.New("كونجستال 20 قرص", "Congestal 20 Tablets"),
				Quantity:    8,
				UnitPrice:   money.FromMinor(2500),
				TotalPrice:  money.FromMinor(20000),
			},
			{
				ID:          4,
				ProductName: i18n.New("كاتافلام 50 مجم 20 قرص", "Cataflam 50mg 20 Tablets"),
				Quantity:    12,
				UnitPrice:   money.FromMinor(3300),
				TotalPrice:  money.FromMinor(39600),
			},
			{
				ID:          5,
				ProductName: i18n.New("أوميبرازول 20 مجم 14 كبسولة", "Omeprazole 20mg 14 Caps"),
				Quantity:    6,
				UnitPrice:   money.FromMinor(4000),
				TotalPrice:  money.FromMinor(24000),
			},
			{
				ID:          6,
				ProductName: i18n.New("فيتامين سي 1000 مجم 10 أقراص فوارة", "Vitamin C 1000mg 10 Eff"),
				Quantity:    15,
				UnitPrice:   money.FromMinor(2000),
				TotalPrice:  money.FromMinor(30000),
			},
		},
	}

	repo := &courierMockCommerceRepo{shipment: mockShipment}
	commSvc := commerce.NewService(repo, log)

	handler := ui.NewUIHandler(
		nil, nil, nil, commSvc, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, log,
	)

	// 1. Test GET /delivery (empty state)
	t.Run("GET /delivery - search form", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/delivery", nil)
		rr := httptest.NewRecorder()
		handler.CourierDeliveryPage(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", rr.Code)
		}
		body := rr.Body.String()
		if !strings.Contains(body, "استعلام وتسليم شحنة") {
			t.Errorf("missing title in courier delivery page")
		}
	})

	// 2. Test GET /delivery?tracking=TRK-987654 (shipment loaded state)
	t.Run("GET /delivery?tracking=TRK-987654 - details loaded", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/delivery?tracking=TRK-987654", nil)
		rr := httptest.NewRecorder()
		handler.CourierDeliveryPage(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", rr.Code)
		}
		body := rr.Body.String()
		if !strings.Contains(body, "SH-2026-001") {
			t.Errorf("missing shipment number in rendered page")
		}
		if !strings.Contains(body, "صيدلية النور الحديثة") {
			t.Errorf("missing customer org name in rendered page")
		}
		if !strings.Contains(body, "بنادول إكسترا") {
			t.Errorf("missing order item name in rendered page")
		}
		if !strings.Contains(body, "تأكيد تسليم الشحنة بالكود") {
			t.Errorf("missing verification form in rendered page")
		}
		// Location & Map assertions
		if !strings.Contains(body, "موقع GPS دقيق") {
			t.Errorf("missing GPS exact location badge")
		}
		if !strings.Contains(body, "29.960200, 31.282500") {
			t.Errorf("missing coordinates in rendered page")
		}
		if !strings.Contains(body, "https://www.google.com/maps/dir/?api=1&amp;destination=29.960200,31.282500") &&
			!strings.Contains(body, "destination=29.960200,31.282500") {
			t.Errorf("missing Google Maps GPS navigation link in rendered page")
		}
		if !strings.Contains(body, "courier-branch-map") {
			t.Errorf("missing mini-map container in rendered page")
		}
		// Pagination & Total units assertions
		if !strings.Contains(body, "إجمالي: 56 عبوة") {
			t.Errorf("missing total units count (56 عبوة) in rendered page")
		}
		if !strings.Contains(body, "courier-pagination-bar") {
			t.Errorf("missing courier pagination bar in rendered page for >5 items")
		}
	})

	// 3. Test POST /delivery/verify - invalid PIN
	t.Run("POST /delivery/verify - invalid delivery code", func(t *testing.T) {
		form := url.Values{}
		form.Set("tracking", "TRK-987654")
		form.Set("delivery_code", "000000") // Wrong PIN
		req := httptest.NewRequest(http.MethodPost, "/delivery/verify", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rr := httptest.NewRecorder()
		handler.CourierVerifyDeliverySubmit(rr, req)

		body := rr.Body.String()
		if !strings.Contains(body, "كود تأكيد الاستلام غير صحيح") {
			t.Errorf("expected invalid delivery code error message in response")
		}
	})

	// 4. Test POST /delivery/verify - valid PIN
	t.Run("POST /delivery/verify - success", func(t *testing.T) {
		form := url.Values{}
		form.Set("tracking", "TRK-987654")
		form.Set("delivery_code", "654321") // Correct PIN
		form.Set("notes", "تم التسليم للصيدلي المسؤول بالفرع")
		req := httptest.NewRequest(http.MethodPost, "/delivery/verify", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rr := httptest.NewRecorder()
		handler.CourierVerifyDeliverySubmit(rr, req)

		body := rr.Body.String()
		if !strings.Contains(body, "تم تسليم الشحنة وتوثيقها بنجاح") && !strings.Contains(body, "تم تأكيد الاستلام") {
			t.Errorf("expected success delivery message in response")
		}

		if mockShipment.Status != commerce.StatusDelivered {
			t.Errorf("expected shipment status to be delivered, got %s", mockShipment.Status)
		}
	})
}
