package postgres

import (
	"context"
	"fmt"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// SetCartItemQuantity changes quantity without touching unit_price.
//
// Leaving unit_price alone is the entire point: it holds the price captured
// when the item was added, and a quantity change must not reprice the line.
func (r *Repository) SetCartItemQuantity(ctx context.Context, cartID, variantID int64, quantity int) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			UPDATE commerce.cart_items
			SET quantity = $3, updated_at = now()
			WHERE cart_id = $1 AND product_variant_id = $2;
		`
		res, err := tx.Exec(txCtx, query, cartID, variantID, quantity)
		if err != nil {
			return fmt.Errorf("commerce postgres: set cart quantity: %w", err)
		}
		if res.RowsAffected() == 0 {
			return apperr.NotFound("cart item")
		}
		return nil
	})
}

// GetShipmentByID returns one shipment.
func (r *Repository) GetShipmentByID(ctx context.Context, id int64) (*commerce.OrderShipment, error) {
	var s commerce.OrderShipment
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, public_id, order_id, organization_id, branch_id, shipment_number,
			       status, subtotal, shipping_fee, total_amount, tracking_number,
			       carrier_name, delivery_code, delivery_attempts, delivery_locked_until,
			       delivery_notes, collected_amount_minor, delivered_by_courier_at,
			       shipped_at, delivered_at, created_at, updated_at
			FROM commerce.order_shipments
			WHERE id = $1;
		`
		var statusStr string
		err := tx.QueryRow(txCtx, query, id).Scan(
			&s.ID, &s.PublicID, &s.OrderID, &s.OrganizationID, &s.BranchID, &s.ShipmentNumber,
			&statusStr, &s.Subtotal, &s.ShippingFee, &s.TotalAmount, &s.TrackingNumber,
			&s.CarrierName, &s.DeliveryCode, &s.DeliveryAttempts, &s.DeliveryLockedUntil,
			&s.DeliveryNotes, &s.CollectedAmountMinor, &s.DeliveredByCourierAt,
			&s.ShippedAt, &s.DeliveredAt, &s.CreatedAt, &s.UpdatedAt,
		)
		if err != nil {
			return err
		}
		s.Status = commerce.OrderStatus(statusStr)
		return nil
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, apperr.NotFound("shipment")
		}
		return nil, fmt.Errorf("commerce postgres: get shipment: %w", err)
	}
	return &s, nil
}

// UpdateShipmentStatus advances a shipment and records the transition.
//
// The status change and its history row are written in one transaction, so the
// audit trail cannot disagree with the current state. The `status = $2`
// predicate makes it a compare-and-swap: two vendor staff clicking at once
// produce one transition and one conflict, not two history rows.
func (r *Repository) UpdateShipmentStatus(
	ctx context.Context,
	id int64,
	from, to commerce.OrderStatus,
	history commerce.OrderStatusHistory,
) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		update := `
			UPDATE commerce.order_shipments
			SET status = $3,
			    shipped_at   = CASE WHEN $3 = 'shipped'   THEN now() ELSE shipped_at   END,
			    delivered_at = CASE WHEN $3 = 'delivered' THEN now() ELSE delivered_at END
			WHERE id = $1 AND status = $2;
		`
		res, err := tx.Exec(txCtx, update, id, string(from), string(to))
		if err != nil {
			return fmt.Errorf("commerce postgres: update shipment status: %w", err)
		}
		if res.RowsAffected() == 0 {
			return apperr.Conflict("shipment.state_changed",
				"This shipment was already updated by someone else. Reload and try again.")
		}

		insert := `
			INSERT INTO commerce.order_status_history
				(order_id, shipment_id, from_status, to_status, notes, changed_by_user_id)
			VALUES ($1, $2, $3, $4, $5, $6);
		`
		if _, err := tx.Exec(txCtx, insert,
			history.OrderID, history.ShipmentID, history.FromStatus,
			history.ToStatus, history.Notes, history.ChangedByUserID,
		); err != nil {
			return fmt.Errorf("commerce postgres: insert shipment history: %w", err)
		}

		// Synchronize parent order status if appropriate
		if to == commerce.StatusDelivered {
			var nonDeliveredCount int
			_ = tx.QueryRow(txCtx, `SELECT COUNT(*) FROM commerce.order_shipments WHERE order_id = $1 AND status != 'delivered';`, history.OrderID).Scan(&nonDeliveredCount)
			if nonDeliveredCount == 0 {
				_, _ = tx.Exec(txCtx, `UPDATE commerce.orders SET status = 'delivered', delivered_at = now(), updated_at = now() WHERE id = $1;`, history.OrderID)
			}
		} else if to == commerce.StatusShipped {
			_, _ = tx.Exec(txCtx, `UPDATE commerce.orders SET status = 'shipped', updated_at = now() WHERE id = $1 AND status NOT IN ('delivered', 'completed');`, history.OrderID)
		} else if to == commerce.StatusConfirmed {
			_, _ = tx.Exec(txCtx, `UPDATE commerce.orders SET status = 'confirmed', updated_at = now() WHERE id = $1 AND status = 'pending';`, history.OrderID)
		}

		return nil
	})
}

// ListOrderHistory returns every status transition for an order, oldest first.
func (r *Repository) ListOrderHistory(ctx context.Context, orderID int64) ([]*commerce.OrderStatusHistory, error) {
	var list []*commerce.OrderStatusHistory
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, order_id, shipment_id, from_status, to_status, notes,
			       changed_by_user_id, created_at
			FROM commerce.order_status_history
			WHERE order_id = $1
			ORDER BY created_at ASC, id ASC;
		`
		rows, err := tx.Query(txCtx, query, orderID)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var h commerce.OrderStatusHistory
			if err := rows.Scan(
				&h.ID, &h.OrderID, &h.ShipmentID, &h.FromStatus, &h.ToStatus,
				&h.Notes, &h.ChangedByUserID, &h.CreatedAt,
			); err != nil {
				return err
			}
			list = append(list, &h)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("commerce postgres: list order history: %w", err)
	}
	return list, nil
}

// RateOrder records a customer rating on their own order (audit §3.3).
//
// Validates that the order is delivered and hasn't already been rated.
// The customer_id predicate stops one buyer rating another's order.
func (r *Repository) RateOrder(ctx context.Context, orderID, customerID int64, rating float64, review string) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		var status string
		var ratedAt *time.Time
		err := tx.QueryRow(txCtx, `SELECT status, rated_at FROM commerce.orders WHERE id = $1 AND customer_id = $2;`, orderID, customerID).Scan(&status, &ratedAt)
		if err != nil {
			if err == pgx.ErrNoRows {
				return apperr.NotFound("order")
			}
			return fmt.Errorf("commerce postgres: check order: %w", err)
		}
		if status != string(commerce.StatusDelivered) {
			return apperr.Validation("order.not_delivered", "Only delivered orders can be rated.", nil)
		}
		if ratedAt != nil {
			return apperr.Validation("order.already_rated", "This order has already been rated.", nil)
		}
		query := `
			UPDATE commerce.orders
			SET rating = $3, review = $4, rated_at = now(), updated_at = now()
			WHERE id = $1 AND customer_id = $2;
		`
		res, err := tx.Exec(txCtx, query, orderID, customerID, rating, review)
		if err != nil {
			return fmt.Errorf("commerce postgres: rate order: %w", err)
		}
		if res.RowsAffected() == 0 {
			return apperr.NotFound("order")
		}
		return nil
	})
}

// SetShipmentTracking records the carrier and tracking number for a shipment.
func (r *Repository) SetShipmentTracking(ctx context.Context, id int64, carrier, tracking string) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(txCtx, `UPDATE commerce.order_shipments SET carrier_name = $2, tracking_number = $3 WHERE id = $1;`, id, carrier, tracking)
		return err
	})
}

// CountVendorShipmentsByStatus counts a vendor's shipments in the given
// statuses. Counting a capped page instead reports the cap, not the total.
func (r *Repository) CountVendorShipmentsByStatus(ctx context.Context, orgID int64, statuses []string) (int, error) {
	var total int
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		const query = `
			SELECT COUNT(*) FROM commerce.order_shipments
			WHERE organization_id = $1 AND status = ANY($2);
		`
		return tx.QueryRow(txCtx, query, orgID, statuses).Scan(&total)
	})
	return total, err
}

// GetShipmentForDeliveryByTracking fetches shipment and delivery information for the courier portal.
func (r *Repository) GetShipmentForDeliveryByTracking(ctx context.Context, tracking string) (*commerce.OrderShipment, error) {
	var s commerce.OrderShipment
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT s.id, s.public_id, s.order_id, s.organization_id, s.branch_id, s.shipment_number,
			       s.status, s.subtotal, s.shipping_fee, s.total_amount, s.tracking_number,
			       s.carrier_name, s.delivery_code, s.delivery_attempts, s.delivery_locked_until,
			       s.delivery_notes, s.collected_amount_minor, s.delivered_by_courier_at,
			       s.shipped_at, s.delivered_at, s.created_at, s.updated_at,
			       COALESCE(ord.order_number, ''),
			       COALESCE(ord.payment_method, 'cod'),
			       COALESCE(ord.payment_status, 'unpaid'),
			       COALESCE(ord.notes, ''),
			       COALESCE(vendor_org.name, '{"ar":i18n.TDefault("w4_mod.s_348_348"),"en":"Approved Supplier"}'::jsonb) AS vendor_name,
			       COALESCE(cust_org.name, '{"ar":i18n.TDefault("w4_mod.s_358_358"),"en":"Approved Pharmacy"}'::jsonb) AS customer_org_name,
			       COALESCE(b.name, '{"ar":i18n.TDefault("w4_mod.s_350_350"),"en":"Main Branch"}'::jsonb) AS branch_name,
			       COALESCE(b.address, '') AS branch_address,
			       COALESCE(b.phone, '') AS branch_phone,
			       COALESCE(b.manager_name, '') AS manager_name
			FROM commerce.order_shipments s
			JOIN commerce.orders ord ON ord.id = s.order_id
			LEFT JOIN org.organizations vendor_org ON vendor_org.id = s.organization_id
			LEFT JOIN org.organizations cust_org ON cust_org.id = ord.organization_id
			LEFT JOIN org.branches b ON b.id = ord.branch_id
			WHERE LOWER(TRIM(s.tracking_number)) = LOWER(TRIM($1))
			   OR LOWER(TRIM(s.shipment_number)) = LOWER(TRIM($1))
			   OR LOWER(TRIM(s.public_id)) = LOWER(TRIM($1))
			ORDER BY s.id DESC
			LIMIT 1;
		`
		var statusStr, payStatusStr string
		err := tx.QueryRow(txCtx, query, tracking).Scan(
			&s.ID, &s.PublicID, &s.OrderID, &s.OrganizationID, &s.BranchID, &s.ShipmentNumber,
			&s.Status, &s.Subtotal, &s.ShippingFee, &s.TotalAmount, &s.TrackingNumber,
			&s.CarrierName, &s.DeliveryCode, &s.DeliveryAttempts, &s.DeliveryLockedUntil,
			&s.DeliveryNotes, &s.CollectedAmountMinor, &s.DeliveredByCourierAt,
			&s.ShippedAt, &s.DeliveredAt, &s.CreatedAt, &s.UpdatedAt,
			&s.OrderNumber, &s.PaymentMethod, &payStatusStr, &s.Notes,
			&s.VendorName, &s.CustomerOrgName, &s.CustomerBranchName, &s.CustomerBranchAddress,
			&s.CustomerBranchPhone, &s.CustomerManagerName,
		)
		if err != nil {
			return err
		}
		s.Status = commerce.OrderStatus(statusStr)
		s.PaymentStatus = commerce.PaymentStatus(payStatusStr)
		return nil
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, apperr.NotFound("shipment")
		}
		return nil, fmt.Errorf("commerce postgres: get shipment for delivery: %w", err)
	}

	// Load lines
	err = r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		queryLines := `
			SELECT id, order_id, shipment_id, organization_id, product_id, product_variant_id,
			       product_name, variant_name, sku, unit_price, quantity, discount_amount, total_price,
			       cost_price, COALESCE(cost_discount_percentage, 0.00)
			FROM commerce.order_lines
			WHERE shipment_id = $1
			ORDER BY id ASC;
		`
		rows, err := tx.Query(txCtx, queryLines, s.ID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var l commerce.OrderLine
			if err := rows.Scan(
				&l.ID, &l.OrderID, &l.ShipmentID, &l.OrganizationID, &l.ProductID, &l.ProductVariantID,
				&l.ProductName, &l.VariantName, &l.SKU, &l.UnitPrice, &l.Quantity, &l.DiscountAmount, &l.TotalPrice,
				&l.CostPrice, &l.CostDiscountPercentage,
			); err != nil {
				return err
			}
			s.Lines = append(s.Lines, &l)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("commerce postgres: get shipment lines for delivery: %w", err)
	}

	return &s, nil
}

// VerifyAndCompleteDelivery validates the 6-digit delivery confirmation PIN and marks the shipment as delivered.
func (r *Repository) VerifyAndCompleteDelivery(
	ctx context.Context,
	shipmentID int64,
	deliveryCode string,
	notes string,
	collectedAmountMinor int64,
) (*commerce.OrderShipment, error) {
	err := r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		// 1. Lock shipment row for update
		query := `
			SELECT id, order_id, organization_id, status, delivery_code, delivery_attempts,
			       delivery_locked_until, total_amount
			FROM commerce.order_shipments
			WHERE id = $1
			FOR UPDATE;
		`
		var currentStatus string
		var actualCode string
		var attempts int
		var lockedUntil *time.Time
		var totalAmount money.Amount
		var orderID int64
		var orgID int64

		err := tx.QueryRow(txCtx, query, shipmentID).Scan(
			&shipmentID, &orderID, &orgID, &currentStatus, &actualCode,
			&attempts, &lockedUntil, &totalAmount,
		)
		if err != nil {
			if err == pgx.ErrNoRows {
				return apperr.NotFound("shipment")
			}
			return fmt.Errorf("commerce postgres: lock shipment: %w", err)
		}

		currentOrderStatus := commerce.OrderStatus(currentStatus)
		now := time.Now()

		// 2. Check if locked due to too many failed attempts
		if lockedUntil != nil && lockedUntil.After(now) {
			remainingMinutes := int(lockedUntil.Sub(now).Minutes()) + 1
			return apperr.Conflict("delivery.locked",
				fmt.Sprintf(i18n.TDefault("w4_mod.d_145"), remainingMinutes))
		}

		// 3. Check status
		if currentOrderStatus == commerce.StatusDelivered || currentOrderStatus == commerce.StatusCompleted {
			return nil // Already delivered
		}
		if currentOrderStatus == commerce.StatusCancelled || currentOrderStatus == commerce.StatusReturned {
			return apperr.Conflict("delivery.cancelled", i18n.TDefault("w4_mod.w4str_146_146"))
		}

		// 4. Verify Delivery Code (PIN)
		if actualCode != "" && deliveryCode != actualCode {
			newAttempts := attempts + 1
			if newAttempts >= 5 {
				lockTime := now.Add(15 * time.Minute)
				_, _ = tx.Exec(txCtx, `
					UPDATE commerce.order_shipments
					SET delivery_attempts = $2, delivery_locked_until = $3, updated_at = now()
					WHERE id = $1;
				`, shipmentID, newAttempts, lockTime)
				return apperr.Conflict("delivery.locked",
					i18n.TDefault("w4_mod.5_15_147"))
			}

			_, _ = tx.Exec(txCtx, `
				UPDATE commerce.order_shipments
				SET delivery_attempts = $2, updated_at = now()
				WHERE id = $1;
			`, shipmentID, newAttempts)
			return apperr.Validation("delivery.invalid_code",
				fmt.Sprintf(i18n.TDefault("w4_mod.d_148"), 5-newAttempts),
				map[string]string{"delivery_code": "invalid"})
		}

		// 5. Success: update shipment to delivered
		if collectedAmountMinor <= 0 {
			collectedAmountMinor = totalAmount.Minor()
		}

		updateQuery := `
			UPDATE commerce.order_shipments
			SET status = 'delivered',
			    delivered_at = now(),
			    delivered_by_courier_at = now(),
			    delivery_notes = $2,
			    collected_amount_minor = $3,
			    delivery_attempts = 0,
			    delivery_locked_until = NULL,
			    updated_at = now()
			WHERE id = $1;
		`
		if _, err := tx.Exec(txCtx, updateQuery, shipmentID, notes, collectedAmountMinor); err != nil {
			return fmt.Errorf("commerce postgres: update delivery status: %w", err)
		}

		// 6. Record audit trail in order_status_history
		fromStatus := currentStatus
		historyQuery := `
			INSERT INTO commerce.order_status_history
				(order_id, shipment_id, from_status, to_status, notes)
			VALUES ($1, $2, $3, 'delivered', $4);
		`
		auditNotes := i18n.TDefault("w4_mod.s_359_359")
		if notes != "" {
			auditNotes += i18n.TDefault("w4_mod.w4str_149_149") + notes
		}
		if _, err := tx.Exec(txCtx, historyQuery, orderID, shipmentID, fromStatus, auditNotes); err != nil {
			return fmt.Errorf("commerce postgres: insert delivery history: %w", err)
		}

		// 7. Synchronize parent order status if all shipments are delivered
		var nonDeliveredCount int
		_ = tx.QueryRow(txCtx, `SELECT COUNT(*) FROM commerce.order_shipments WHERE order_id = $1 AND status != 'delivered';`, orderID).Scan(&nonDeliveredCount)
		if nonDeliveredCount == 0 {
			_, _ = tx.Exec(txCtx, `
				UPDATE commerce.orders
				SET status = 'delivered',
				    payment_status = CASE WHEN payment_method = 'cod' THEN 'paid' ELSE payment_status END,
				    delivered_at = now(),
				    updated_at = now()
				WHERE id = $1;
			`, orderID)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return r.GetShipmentByID(database.AsSystem(ctx), shipmentID)
}
