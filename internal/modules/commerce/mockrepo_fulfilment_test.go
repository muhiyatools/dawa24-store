package commerce

import (
	"context"

	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// Fulfilment methods of mockCommerceRepo.

func (m *mockCommerceRepo) SetCartItemQuantity(_ context.Context, _, _ int64, _ int) error {
	return nil
}

func (m *mockCommerceRepo) GetShipmentByID(_ context.Context, id int64) (*OrderShipment, error) {
	for _, list := range m.shipments {
		for _, s := range list {
			if s.ID == id {
				// A copy, as a real repository returns. Handing back the stored
				// pointer would let the service mutate the fake database before
				// the compare-and-swap ran.
				copied := *s
				return &copied, nil
			}
		}
	}
	return nil, apperr.NotFound("shipment")
}

func (m *mockCommerceRepo) UpdateShipmentStatus(
	_ context.Context,
	id int64,
	from, to OrderStatus,
	history OrderStatusHistory,
) error {
	for _, list := range m.shipments {
		for _, s := range list {
			if s.ID != id {
				continue
			}
			if s.Status != from {
				return apperr.Conflict("shipment.state_changed",
					"This shipment was already updated by someone else.")
			}
			s.Status = to
			m.history[history.OrderID] = append(m.history[history.OrderID], &history)
			return nil
		}
	}
	return nil
}

func (m *mockCommerceRepo) ListOrderHistory(_ context.Context, orderID int64) ([]*OrderStatusHistory, error) {
	return m.history[orderID], nil
}

func (m *mockCommerceRepo) RateOrder(_ context.Context, orderID, customerID int64, rating int, review string) error {
	o, ok := m.orders[orderID]
	if !ok || o.CustomerID != customerID {
		return apperr.NotFound("order")
	}
	return nil
}
