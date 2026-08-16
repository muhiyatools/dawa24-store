package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// CreateOrder writes the master order, its vendor shipment partitions, and line item snapshots atomically.
func (r *Repository) CreateOrder(
	ctx context.Context,
	order *commerce.Order,
	shipments []*commerce.OrderShipment,
	lines []*commerce.OrderLine,
) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		// 1. Insert master order
		queryOrder := `
			INSERT INTO commerce.orders (
				order_number, customer_id, organization_id, status, subtotal,
				discount_amount, shipping_fee, tax_amount, total_amount,
				payment_method, payment_status, notes
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
			RETURNING id, public_id, created_at, updated_at;
		`
		err := tx.QueryRow(txCtx, queryOrder,
			order.OrderNumber, order.CustomerID, order.OrganizationID, string(order.Status),
			order.Subtotal, order.DiscountAmount, order.ShippingFee, order.TaxAmount,
			order.TotalAmount, order.PaymentMethod, string(order.PaymentStatus), order.Notes,
		).Scan(&order.ID, &order.PublicID, &order.CreatedAt, &order.UpdatedAt)
		if err != nil {
			return fmt.Errorf("commerce postgres: insert order: %w", err)
		}

		// 2. Insert shipments map
		shipmentIDMap := make(map[int64]int64) // key: vendorOrgID, val: shipment.ID
		for seq, s := range shipments {
			s.OrderID = order.ID
			s.ShipmentNumber = commerce.GenerateShipmentNumber(order.OrderNumber, seq+1)
			s.Status = order.Status

			queryShipment := `
				INSERT INTO commerce.order_shipments (
					order_id, organization_id, branch_id, shipment_number,
					status, subtotal, shipping_fee, total_amount
				) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
				RETURNING id, public_id, created_at, updated_at;
			`
			err := tx.QueryRow(txCtx, queryShipment,
				s.OrderID, s.OrganizationID, s.BranchID, s.ShipmentNumber,
				string(s.Status), s.Subtotal, s.ShippingFee, s.TotalAmount,
			).Scan(&s.ID, &s.PublicID, &s.CreatedAt, &s.UpdatedAt)
			if err != nil {
				return fmt.Errorf("commerce postgres: insert shipment: %w", err)
			}
			shipmentIDMap[s.OrganizationID] = s.ID
		}

		// 3. Insert line snapshots
		for _, line := range lines {
			line.OrderID = order.ID
			if line.ShipmentID == 0 {
				line.ShipmentID = shipmentIDMap[line.OrganizationID]
			}

			queryLine := `
				INSERT INTO commerce.order_lines (
					order_id, shipment_id, organization_id, product_id,
					product_variant_id, product_name, variant_name, sku,
					unit_price, quantity, discount_amount, total_price
				) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
				RETURNING id, created_at;
			`
			err := tx.QueryRow(txCtx, queryLine,
				line.OrderID, line.ShipmentID, line.OrganizationID, line.ProductID,
				line.ProductVariantID, line.ProductName, line.VariantName, line.SKU,
				line.UnitPrice, line.Quantity, line.DiscountAmount, line.TotalPrice,
			).Scan(&line.ID, &line.CreatedAt)
			if err != nil {
				return fmt.Errorf("commerce postgres: insert order line: %w", err)
			}
		}

		// 4. Initial status history
		queryHistory := `
			INSERT INTO commerce.order_status_history (order_id, to_status, notes)
			VALUES ($1, $2, 'Order placed successfully');
		`
		_, err = tx.Exec(txCtx, queryHistory, order.ID, string(order.Status))
		return err
	})
}

// GetOrderByID retrieves order details.
func (r *Repository) GetOrderByID(ctx context.Context, id int64) (*commerce.Order, error) {
	var o commerce.Order
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, public_id, order_number, customer_id, organization_id, status,
			       subtotal, discount_amount, shipping_fee, tax_amount, total_amount,
			       payment_method, payment_status, notes, created_at, updated_at, deleted_at
			FROM commerce.orders
			WHERE id = $1 AND deleted_at IS NULL;
		`
		var statusStr, payStatusStr string
		err := tx.QueryRow(txCtx, query, id).Scan(
			&o.ID, &o.PublicID, &o.OrderNumber, &o.CustomerID, &o.OrganizationID,
			&statusStr, &o.Subtotal, &o.DiscountAmount, &o.ShippingFee, &o.TaxAmount,
			&o.TotalAmount, &o.PaymentMethod, &payStatusStr, &o.Notes,
			&o.CreatedAt, &o.UpdatedAt, &o.DeletedAt,
		)
		if err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("order")
			}
			return err
		}
		o.Status = commerce.OrderStatus(statusStr)
		o.PaymentStatus = commerce.PaymentStatus(payStatusStr)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &o, nil
}

// GetOrderByNumber retrieves an order by its public business identifier.
func (r *Repository) GetOrderByNumber(ctx context.Context, number string) (*commerce.Order, error) {
	var o commerce.Order
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, public_id, order_number, customer_id, organization_id, status,
			       subtotal, discount_amount, shipping_fee, tax_amount, total_amount,
			       payment_method, payment_status, notes, created_at, updated_at, deleted_at
			FROM commerce.orders
			WHERE order_number = $1 AND deleted_at IS NULL;
		`
		var statusStr, payStatusStr string
		err := tx.QueryRow(txCtx, query, number).Scan(
			&o.ID, &o.PublicID, &o.OrderNumber, &o.CustomerID, &o.OrganizationID,
			&statusStr, &o.Subtotal, &o.DiscountAmount, &o.ShippingFee, &o.TaxAmount,
			&o.TotalAmount, &o.PaymentMethod, &payStatusStr, &o.Notes,
			&o.CreatedAt, &o.UpdatedAt, &o.DeletedAt,
		)
		if err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("order")
			}
			return err
		}
		o.Status = commerce.OrderStatus(statusStr)
		o.PaymentStatus = commerce.PaymentStatus(payStatusStr)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &o, nil
}

// UpdateOrderStatus transitions order status and records an entry in the audit history.
func (r *Repository) UpdateOrderStatus(
	ctx context.Context,
	orderID int64,
	toStatus commerce.OrderStatus,
	history commerce.OrderStatusHistory,
) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		var currentStatus string
		err := tx.QueryRow(txCtx, `SELECT status FROM commerce.orders WHERE id = $1 FOR UPDATE;`, orderID).Scan(&currentStatus)
		if err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("order")
			}
			return err
		}

		if !commerce.IsValidStatusTransition(commerce.OrderStatus(currentStatus), toStatus) {
			return apperr.Validation("order.invalid_transition", fmt.Sprintf("Cannot transition order from %s to %s", currentStatus, toStatus), nil)
		}

		queryUpdate := `UPDATE commerce.orders SET status = $2, updated_at = now() WHERE id = $1;`
		if _, err := tx.Exec(txCtx, queryUpdate, orderID, string(toStatus)); err != nil {
			return err
		}

		queryHistory := `
			INSERT INTO commerce.order_status_history (
				order_id, shipment_id, from_status, to_status, notes, changed_by_user_id
			) VALUES ($1, $2, $3, $4, $5, $6);
		`
		_, err = tx.Exec(txCtx, queryHistory,
			orderID, history.ShipmentID, currentStatus, string(toStatus), history.Notes, history.ChangedByUserID,
		)
		return err
	})
}

// ListOrdersByCustomer retrieves customer orders.
func (r *Repository) ListOrdersByCustomer(ctx context.Context, customerID int64, limit, offset int) ([]*commerce.Order, error) {
	var orders []*commerce.Order
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, public_id, order_number, customer_id, organization_id, status,
			       subtotal, discount_amount, shipping_fee, tax_amount, total_amount,
			       payment_method, payment_status, notes, created_at, updated_at, deleted_at
			FROM commerce.orders
			WHERE customer_id = $1 AND deleted_at IS NULL
			ORDER BY created_at DESC
			LIMIT $2 OFFSET $3;
		`
		if limit <= 0 || limit > 100 {
			limit = 20
		}
		rows, err := tx.Query(txCtx, query, customerID, limit, offset)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var o commerce.Order
			var statusStr, payStatusStr string
			if err := rows.Scan(
				&o.ID, &o.PublicID, &o.OrderNumber, &o.CustomerID, &o.OrganizationID,
				&statusStr, &o.Subtotal, &o.DiscountAmount, &o.ShippingFee, &o.TaxAmount,
				&o.TotalAmount, &o.PaymentMethod, &payStatusStr, &o.Notes,
				&o.CreatedAt, &o.UpdatedAt, &o.DeletedAt,
			); err != nil {
				return err
			}
			o.Status = commerce.OrderStatus(statusStr)
			o.PaymentStatus = commerce.PaymentStatus(payStatusStr)
			orders = append(orders, &o)
		}
		return rows.Err()
	})
	return orders, err
}

// ListShipmentsByVendor retrieves shipment partitions for a vendor organization.
func (r *Repository) ListShipmentsByVendor(ctx context.Context, vendorOrgID int64, limit, offset int) ([]*commerce.OrderShipment, error) {
	var shipments []*commerce.OrderShipment
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, public_id, order_id, organization_id, branch_id, shipment_number,
			       status, subtotal, shipping_fee, total_amount, tracking_number,
			       carrier_name, shipped_at, delivered_at, created_at, updated_at
			FROM commerce.order_shipments
			WHERE organization_id = $1
			ORDER BY created_at DESC
			LIMIT $2 OFFSET $3;
		`
		if limit <= 0 || limit > 100 {
			limit = 20
		}
		rows, err := tx.Query(txCtx, query, vendorOrgID, limit, offset)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var s commerce.OrderShipment
			var statusStr string
			if err := rows.Scan(
				&s.ID, &s.PublicID, &s.OrderID, &s.OrganizationID, &s.BranchID,
				&s.ShipmentNumber, &statusStr, &s.Subtotal, &s.ShippingFee,
				&s.TotalAmount, &s.TrackingNumber, &s.CarrierName,
				&s.ShippedAt, &s.DeliveredAt, &s.CreatedAt, &s.UpdatedAt,
			); err != nil {
				return err
			}
			s.Status = commerce.OrderStatus(statusStr)
			shipments = append(shipments, &s)
		}
		return rows.Err()
	})
	return shipments, err
}

var _ time.Time

// CountOrders returns the total number of orders on the platform.
//
// Used by the admin dashboard, which previously counted len() of a page capped
// at 100 and so reported 100 for any platform with more than that.
func (r *Repository) CountOrders(ctx context.Context) (int, error) {
	var total int
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(txCtx, `SELECT COUNT(*) FROM commerce.orders;`).Scan(&total)
	})
	return total, err
}
