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
