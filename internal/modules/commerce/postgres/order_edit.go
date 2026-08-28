package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// UpdateCustomerPendingOrder modifies items, quantities and recalculates totals for a pending order.
func (r *Repository) UpdateCustomerPendingOrder(
	ctx context.Context,
	order *commerce.Order,
	lines []commerce.OrderLineEditItem,
	changedByUserID int64,
) (*commerce.Order, error) {
	err := r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		// 1. Lock the order row and verify status is still pending
		var currentStatus string
		var shippingFee, taxAmount money.Amount
		err := tx.QueryRow(txCtx, `
			SELECT status, shipping_fee, tax_amount 
			FROM commerce.orders 
			WHERE id = $1 FOR UPDATE;
		`, order.ID).Scan(&currentStatus, &shippingFee, &taxAmount)
		if err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("order")
			}
			return err
		}

		if commerce.OrderStatus(currentStatus) != commerce.StatusPending {
			return apperr.Forbidden("order.locked", fmt.Sprintf("لا يمكن تعديل الطلب بعد قبوله أو تأكيده من قِبل المورد (حالة الطلب الحالية: %s)", currentStatus))
		}

		// Get primary shipment ID and vendor organization ID if existing
		var defaultShipmentID, defaultOrgID int64
		_ = tx.QueryRow(txCtx, `
			SELECT id, organization_id FROM commerce.order_shipments WHERE order_id = $1 ORDER BY id ASC LIMIT 1;
		`, order.ID).Scan(&defaultShipmentID, &defaultOrgID)

		if defaultOrgID == 0 && order.OrganizationID != nil {
			defaultOrgID = *order.OrganizationID
		}

		// 2. Process line edits
		for _, l := range lines {
			if l.IsDeleted {
				if l.ID > 0 {
					_, _ = tx.Exec(txCtx, `DELETE FROM commerce.order_lines WHERE id = $1 AND order_id = $2;`, l.ID, order.ID)
				}
				continue
			}

			if l.Quantity <= 0 {
				continue
			}

			effectivePrice, _ := l.UnitPrice.Sub(l.DiscountAmount)
			if effectivePrice.IsNegative() {
				effectivePrice = money.Zero
			}
			lineTotal, _ := effectivePrice.MulInt(int64(l.Quantity))

			if l.ID > 0 {
				// Update existing line
				_, err = tx.Exec(txCtx, `
					UPDATE commerce.order_lines
					SET quantity = $1,
						unit_price = $2,
						discount_amount = $3,
						total_price = $4,
						product_name = CASE WHEN $5 != '' THEN jsonb_build_object('ar', $5::text, 'en', $5::text) ELSE product_name END
					WHERE id = $6 AND order_id = $7;
				`, l.Quantity, l.UnitPrice, l.DiscountAmount, lineTotal, l.ProductName, l.ID, order.ID)
				if err != nil {
					return fmt.Errorf("update line %d: %w", l.ID, err)
				}
			} else {
				// Insert newly added line
				nameJSON := fmt.Sprintf(`{"ar": %q, "en": %q}`, l.ProductName, l.ProductName)
				_, err = tx.Exec(txCtx, `
					INSERT INTO commerce.order_lines (
						order_id, shipment_id, organization_id, product_name, sku,
						unit_price, quantity, discount_amount, total_price
					) VALUES (
						$1, $2, $3, $4::jsonb, $5,
						$6, $7, $8, $9
					);
				`, order.ID, defaultShipmentID, defaultOrgID, nameJSON, "CUSTOM",
					l.UnitPrice, l.Quantity, l.DiscountAmount, lineTotal)
				if err != nil {
					return fmt.Errorf("insert line: %w", err)
				}
			}
		}

		// 3. Count remaining active lines
		var remainingCount int
		_ = tx.QueryRow(txCtx, `SELECT COUNT(*) FROM commerce.order_lines WHERE order_id = $1;`, order.ID).Scan(&remainingCount)
		if remainingCount == 0 {
			return apperr.Validation("order.empty", "لا يمكن حفظ الطلب بدون أصناف. يجب أن يحتوي الطلب على صنف واحد على الأقل.", nil)
		}

		// 4. Recalculate order totals from all lines in DB
		newSubtotal := money.Zero
		newTotalDiscount := money.Zero
		rows, err := tx.Query(txCtx, `
			SELECT unit_price, quantity, discount_amount 
			FROM commerce.order_lines 
			WHERE order_id = $1;
		`, order.ID)
		if err != nil {
			return err
		}
		for rows.Next() {
			var up, da money.Amount
			var qty int
			if err := rows.Scan(&up, &qty, &da); err == nil {
				lineSub, _ := up.MulInt(int64(qty))
				lineDisc, _ := da.MulInt(int64(qty))
				newSubtotal, _ = newSubtotal.Add(lineSub)
				newTotalDiscount, _ = newTotalDiscount.Add(lineDisc)
			}
		}
		rows.Close()

		newTotalAmount, _ := newSubtotal.Sub(newTotalDiscount)
		if newTotalAmount.IsNegative() {
			newTotalAmount = money.Zero
		}
		newTotalAmount, _ = newTotalAmount.Add(shippingFee)
		newTotalAmount, _ = newTotalAmount.Add(taxAmount)

		// 5. Update master order row
		_, err = tx.Exec(txCtx, `
			UPDATE commerce.orders
			SET subtotal = $1,
				discount_amount = $2,
				total_discount = $2,
				total_amount = $3,
				final_price = $3,
				updated_at = now()
			WHERE id = $4;
		`, newSubtotal, newTotalDiscount, newTotalAmount, order.ID)
		if err != nil {
			return err
		}

		// 6. Update shipment if exists
		if defaultShipmentID > 0 {
			_, _ = tx.Exec(txCtx, `
				UPDATE commerce.order_shipments
				SET subtotal = $1, total_amount = $2, updated_at = now()
				WHERE id = $3;
			`, newSubtotal, newTotalAmount, defaultShipmentID)
		}

		// 7. Record status history audit log
		historyQuery := `
			INSERT INTO commerce.order_status_history (
				order_id, shipment_id, from_status, to_status, notes, changed_by_user_id
			) VALUES ($1, $2, $3, $4, $5, $6);
		`
		var shipmentIDPtr *int64
		if defaultShipmentID > 0 {
			shipmentIDPtr = &defaultShipmentID
		}
		var userPtr *int64
		if changedByUserID > 0 {
			userPtr = &changedByUserID
		}
		_, _ = tx.Exec(txCtx, historyQuery, order.ID, shipmentIDPtr, currentStatus, currentStatus, "تم تعديل تفاصيل الطلب والأصناف والكميات من قِبل الصيدلي", userPtr)

		return nil
	})
	if err != nil {
		return nil, err
	}

	return r.GetOrderByID(ctx, order.ID)
}
