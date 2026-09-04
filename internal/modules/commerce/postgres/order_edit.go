package postgres

import (
	"context"
	"fmt"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"

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
		var oldSubtotal, oldDiscount, shippingFee, oldTax money.Amount
		err := tx.QueryRow(txCtx, `
			SELECT status, subtotal, discount_amount, shipping_fee, tax_amount 
			FROM commerce.orders 
			WHERE id = $1 FOR UPDATE;
		`, order.ID).Scan(&currentStatus, &oldSubtotal, &oldDiscount, &shippingFee, &oldTax)
		if err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("order")
			}
			return err
		}

		if commerce.OrderStatus(currentStatus) != commerce.StatusPending {
			return apperr.Forbidden("order.locked", fmt.Sprintf(i18n.TDefault("w4_mod.s_151"), currentStatus))
		}

		// Calculate initial tax percentage rate if tax was present on the order
		taxRate := 0.0
		oldTaxable, _ := oldSubtotal.Sub(oldDiscount)
		if oldTax.IsPositive() && oldTaxable.IsPositive() {
			taxRate = float64(oldTax.Minor()) / float64(oldTaxable.Minor())
		}

		// Get primary shipment ID and vendor organization ID if existing
		var defaultShipmentID, defaultOrgID int64
		_ = tx.QueryRow(txCtx, `
			SELECT id, organization_id FROM commerce.order_shipments WHERE order_id = $1 ORDER BY id ASC LIMIT 1;
		`, order.ID).Scan(&defaultShipmentID, &defaultOrgID)

		if defaultOrgID == 0 && order.OrganizationID != nil {
			defaultOrgID = *order.OrganizationID
		}

		// 2. Process line edits with stock and price validation
		for _, l := range lines {
			if l.IsDeleted {
				if l.ID > 0 {
					_, _ = tx.Exec(txCtx, `DELETE FROM commerce.order_lines WHERE id = $1 AND order_id = $2;`, l.ID, order.ID)
				}
				continue
			}

			if l.Quantity <= 0 {
				return apperr.Validation("quantity", i18n.TDefault("w4_mod.1_152"), nil)
			}

			if l.ID > 0 {
				// Query existing line to preserve authentic pricing and validate against catalog & offer
				var dbProductID, dbVariantID, dbOfferProductID *int64
				var dbUnitPrice, dbOldDiscount, dbOriginalDiscount money.Amount
				var dbOldQty int
				var dbProductName string
				var dbOrgID int64

				err := tx.QueryRow(txCtx, `
					SELECT product_id, product_variant_id, offer_product_id, organization_id, unit_price, quantity, discount_amount, COALESCE(original_discount, 0),
					       COALESCE(product_name->>'ar', product_name->>'en', '')
					FROM commerce.order_lines
					WHERE id = $1 AND order_id = $2
					FOR UPDATE;
				`, l.ID, order.ID).Scan(&dbProductID, &dbVariantID, &dbOfferProductID, &dbOrgID, &dbUnitPrice, &dbOldQty, &dbOldDiscount, &dbOriginalDiscount, &dbProductName)
				if err != nil {
					if database.IsNotFound(err) {
						continue
					}
					return fmt.Errorf("fetch line %d: %w", l.ID, err)
				}

				if dbProductName == "" {
					dbProductName = l.ProductName
				}

				// Check offer rules if order was placed under a special offer
				if dbOfferProductID != nil && *dbOfferProductID > 0 {
					var offerCustomQty, maxQtyPerOrder int
					_ = tx.QueryRow(txCtx, `
						SELECT COALESCE(custom_qty, 1), COALESCE(max_qty_per_order, 0)
						FROM promo.offer_products
						WHERE id = $1;
					`, *dbOfferProductID).Scan(&offerCustomQty, &maxQtyPerOrder)

					if maxQtyPerOrder > 0 && l.Quantity > maxQtyPerOrder {
						return apperr.Validation("max_qty_exceeded", fmt.Sprintf(i18n.TDefault("w4_mod.s_d_d_153"), dbProductName, l.Quantity, maxQtyPerOrder), nil)
					}
					if offerCustomQty > 1 && l.Quantity < offerCustomQty {
						return apperr.Validation("min_offer_qty", fmt.Sprintf(i18n.TDefault("w4_mod.s_d_154"), dbProductName, offerCustomQty), nil)
					}
				}

				// Check minimum order quantity & available stock if variant exists in catalog
				if dbVariantID != nil && *dbVariantID > 0 {
					var minOrderQty int
					_ = tx.QueryRow(txCtx, `
						SELECT COALESCE(min_order_qty, 0)
						FROM catalog.product_variants
						WHERE id = $1 AND deleted_at IS NULL;
					`, *dbVariantID).Scan(&minOrderQty)
					if minOrderQty > 0 && l.Quantity < minOrderQty {
						return apperr.Validation("min_order_qty", fmt.Sprintf(i18n.TDefault("w4_mod.s_d_155"), dbProductName, minOrderQty), nil)
					}

					var availableStock int
					checkOrgID := dbOrgID
					if checkOrgID == 0 {
						checkOrgID = defaultOrgID
					}
					_ = tx.QueryRow(txCtx, `
						SELECT COALESCE(SUM(quantity), 0)
						FROM inventory.stocks
						WHERE product_variant_id = $1 
						  AND ($2::bigint = 0 OR organization_id = $2)
						  AND deleted_at IS NULL;
					`, *dbVariantID, checkOrgID).Scan(&availableStock)
					if availableStock > 0 && l.Quantity > availableStock {
						return apperr.Validation("stock_exceeded", fmt.Sprintf(i18n.TDefault("w4_mod.s_d_d_156"), dbProductName, l.Quantity, availableStock), nil)
					}
				}

				// Calculate per-unit discount accurately
				unitDiscount := money.Zero
				if dbOldQty > 0 && dbOldDiscount.IsPositive() {
					discMinor := dbOldDiscount.Minor() / int64(dbOldQty)
					unitDiscount = money.FromMinor(discMinor)
				} else if dbOriginalDiscount.IsPositive() {
					unitDiscount = dbOriginalDiscount
				}

				lineDiscount, _ := unitDiscount.MulInt(int64(l.Quantity))
				lineSubtotal, _ := dbUnitPrice.MulInt(int64(l.Quantity))
				lineTotal, _ := lineSubtotal.Sub(lineDiscount)
				if lineTotal.IsNegative() {
					lineTotal = money.Zero
				}

				// Update existing line in DB
				_, err = tx.Exec(txCtx, `
					UPDATE commerce.order_lines
					SET quantity = $1,
						unit_price = $2,
						discount_amount = $3,
						total_price = $4
					WHERE id = $5 AND order_id = $6;
				`, l.Quantity, dbUnitPrice, lineDiscount, lineTotal, l.ID, order.ID)
				if err != nil {
					return fmt.Errorf("update line %d: %w", l.ID, err)
				}
			} else {
				// A pharmacy editing its own pending order may change quantities
				// and drop lines. It may not invent one.
				//
				// The old "إضافة صنف جديد يدوياً" path inserted a free-text row
				// at a price the buyer typed, with sku 'CUSTOM', no product, no
				// variant and therefore no stock check — and attached it to the
				// first shipment regardless of which supplier was meant to fill
				// it. A supplier was then asked to ship a line they had never
				// listed, at a price they had never quoted.
				return apperr.Validation("order.line_not_editable",
					i18n.TDefault("customer.order.no_manual_lines"), nil)
			}
		}

		// 3. Count remaining active lines
		var remainingCount int
		_ = tx.QueryRow(txCtx, `SELECT COUNT(*) FROM commerce.order_lines WHERE order_id = $1;`, order.ID).Scan(&remainingCount)
		if remainingCount == 0 {
			return apperr.Validation("order.empty", i18n.TDefault("w4_mod.w4str_157_157"), nil)
		}

		// 4. Recalculate order totals from all active lines in DB
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
				newSubtotal, _ = newSubtotal.Add(lineSub)
				newTotalDiscount, _ = newTotalDiscount.Add(da)
			}
		}
		rows.Close()

		newTaxable, _ := newSubtotal.Sub(newTotalDiscount)
		if newTaxable.IsNegative() {
			newTaxable = money.Zero
		}

		newTaxAmount := money.Zero
		if taxRate > 0 && newTaxable.IsPositive() {
			taxMinor := int64(float64(newTaxable.Minor()) * taxRate)
			newTaxAmount = money.FromMinor(taxMinor)
		} else if oldTax.IsPositive() {
			newTaxAmount = oldTax
		}

		newTotalAmount := newTaxable
		newTotalAmount, _ = newTotalAmount.Add(shippingFee)
		newTotalAmount, _ = newTotalAmount.Add(newTaxAmount)

		// 5. Update master order row
		_, err = tx.Exec(txCtx, `
			UPDATE commerce.orders
			SET subtotal = $1,
				discount_amount = $2,
				total_discount = $2,
				tax_amount = $3,
				total_amount = $4,
				final_price = $4,
				updated_at = now()
			WHERE id = $5;
		`, newSubtotal, newTotalDiscount, newTaxAmount, newTotalAmount, order.ID)
		if err != nil {
			return err
		}

		// 6. Recalculate every shipment from its own lines.
		//
		// This wrote the *whole order's* subtotal and total onto the *first*
		// shipment and left the rest untouched. A pharmacy buying from three
		// suppliers therefore ended up with parcel 1 claiming the entire order's
		// value and parcels 2 and 3 still showing pre-edit figures, which is
		// what each supplier reads on their own shipment screen.
		//
		// The shipping fee stays on the shipment: it is that vendor's delivery
		// quote and is not affected by a quantity change.
		if _, err := tx.Exec(txCtx, `
			UPDATE commerce.order_shipments s
			SET subtotal     = COALESCE(t.subtotal, 0),
			    total_amount = COALESCE(t.subtotal, 0) - COALESCE(t.discount, 0)
			                   + COALESCE(s.shipping_fee, 0),
			    updated_at   = now()
			FROM (
				SELECT sh.id,
				       COALESCE(SUM(l.unit_price * l.quantity), 0) AS subtotal,
				       COALESCE(SUM(l.discount_amount), 0)         AS discount
				FROM commerce.order_shipments sh
				LEFT JOIN commerce.order_lines l ON l.shipment_id = sh.id
				WHERE sh.order_id = $1
				GROUP BY sh.id
			) t
			WHERE s.id = t.id AND s.order_id = $1;
		`, order.ID); err != nil {
			return fmt.Errorf("recalculate shipments: %w", err)
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
		_, _ = tx.Exec(txCtx, historyQuery, order.ID, shipmentIDPtr, currentStatus, currentStatus, i18n.TDefault("w4_mod.s_361_361"), userPtr)

		return nil
	})
	if err != nil {
		return nil, err
	}

	return r.GetOrderByID(ctx, order.ID)
}
