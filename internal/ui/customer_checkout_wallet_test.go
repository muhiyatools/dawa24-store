package ui_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/ui"
)

type walletMockCommerceRepo struct {
	commerce.Repository
	shipment *commerce.OrderShipment
}

func (m *walletMockCommerceRepo) GetShipmentByID(_ context.Context, id int64) (*commerce.OrderShipment, error) {
	if m.shipment != nil && m.shipment.ID == id {
		c := *m.shipment
		return &c, nil
	}
	return nil, apperr.NotFound("shipment")
}

func (m *walletMockCommerceRepo) GetShipmentForDeliveryByTracking(_ context.Context, tracking string) (*commerce.OrderShipment, error) {
	if m.shipment != nil && (m.shipment.TrackingNumber == tracking || m.shipment.ShipmentNumber == tracking) {
		c := *m.shipment
		return &c, nil
	}
	return nil, apperr.NotFound("shipment")
}

func (m *walletMockCommerceRepo) VerifyAndCompleteDelivery(
	_ context.Context,
	shipmentID int64,
	deliveryCode string,
	notes string,
	collectedAmountMinor int64,
) (*commerce.OrderShipment, error) {
	if m.shipment != nil && m.shipment.ID == shipmentID {
		m.shipment.Status = commerce.StatusDelivered
		m.shipment.CollectedAmountMinor = collectedAmountMinor
		return m.shipment, nil
	}
	return nil, apperr.NotFound("shipment")
}

func (m *walletMockCommerceRepo) GetOrderByID(_ context.Context, id int64) (*commerce.Order, error) {
	return &commerce.Order{ID: id, OrderNumber: "ORD-WALLET-100", CustomerID: 10}, nil
}

func TestCourierDeliveryPage_WalletPayment(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	mockShipment := &commerce.OrderShipment{
		ID:             101,
		ShipmentNumber: "SH-WALLET-001",
		TrackingNumber: "TRK-WALLET-001",
		OrderID:        202,
		OrganizationID: 1,
		Status:         commerce.StatusShipped,
		DeliveryCode:   "123456",
		PaymentMethod:  "wallet",
		PaymentStatus:  commerce.PaymentPaid,
		Subtotal:       money.FromMinor(50000), // 500.00 EGP
		ShippingFee:    money.FromMinor(3500),  // 35.00 EGP delivery fee
		TotalAmount:    money.FromMinor(53500), // 535.00 EGP total
		VendorName:     i18n.New("شركة المتحدة للتوزيع", "United Distribution"),
		CustomerOrgName: i18n.New("صيدلية الشفاء", "Al-Shifa Pharmacy"),
	}

	repo := &walletMockCommerceRepo{shipment: mockShipment}
	commSvc := commerce.NewService(repo, log)
	handler := ui.NewUIHandler(nil, nil, nil, commSvc, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, log)

	req := httptest.NewRequest(http.MethodGet, "/delivery?tracking=TRK-WALLET-001", nil)
	rr := httptest.NewRecorder()
	handler.CourierDeliveryPage(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rr.Code)
	}
	body := rr.Body.String()

	// Must indicate goods prepaid via organization wallet
	if !strings.Contains(body, "مسدد بالمحفظة المؤسسية مسبقاً") {
		t.Errorf("expected wallet payment alert banner")
	}
	// Must display only the delivery fee to be collected (35.00 EGP)
	if !strings.Contains(body, "35.00") {
		t.Errorf("expected shipping fee 35.00 to be collected")
	}
	// Must state collection is only shipping fee
	if !strings.Contains(body, "رسوم التوصيل فقط") {
		t.Errorf("expected text clarifying collection is shipping fee only")
	}
	// Checkbox must confirm delivery fee only
	if !strings.Contains(body, "أؤكد تحصيل رسوم التوصيل فقط (35.00 ج.م)") {
		t.Errorf("expected confirmation checkbox for shipping fee only")
	}
}

func TestCourierDeliveryPage_WalletPayment_FreeShipping(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	mockShipment := &commerce.OrderShipment{
		ID:             102,
		ShipmentNumber: "SH-WALLET-002",
		TrackingNumber: "TRK-WALLET-002",
		OrderID:        203,
		OrganizationID: 1,
		Status:         commerce.StatusShipped,
		DeliveryCode:   "654321",
		PaymentMethod:  "wallet",
		PaymentStatus:  commerce.PaymentPaid,
		Subtotal:       money.FromMinor(50000),
		ShippingFee:    money.Zero, // Free shipping
		TotalAmount:    money.FromMinor(50000),
		VendorName:     i18n.New("شركة فارما للتوزيع", "Pharma Distribution"),
		CustomerOrgName: i18n.New("صيدلية النور", "Al-Nour Pharmacy"),
	}

	repo := &walletMockCommerceRepo{shipment: mockShipment}
	commSvc := commerce.NewService(repo, log)
	handler := ui.NewUIHandler(nil, nil, nil, commSvc, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, log)

	req := httptest.NewRequest(http.MethodGet, "/delivery?tracking=TRK-WALLET-002", nil)
	rr := httptest.NewRecorder()
	handler.CourierDeliveryPage(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rr.Code)
	}
	body := rr.Body.String()

	// Must show 0.00 EGP to collect
	if !strings.Contains(body, "0.00") {
		t.Errorf("expected 0.00 EGP to collect for fully covered wallet shipment")
	}
	if !strings.Contains(body, "مدفوع بالكامل من المحفظة المؤسسية") {
		t.Errorf("expected full wallet prepaid alert")
	}
}