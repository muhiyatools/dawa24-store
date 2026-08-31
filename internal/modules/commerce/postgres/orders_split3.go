package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// ListShipmentsByVendor retrieves shipment partitions for a vendor organization.
func (r *Repository) ListShipmentsByVendor(ctx context.Context, vendorOrgID int64, limit, offset int) ([]*commerce.OrderShipment, error) {
	var shipments []*commerce.OrderShipment
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
			       COALESCE(org.name, '{"ar":i18n.TDefault("w4_mod.s_358_358"),"en":"Approved Pharmacy"}'::jsonb) AS customer_org_name,
			       COALESCE(b.name, '{"ar":i18n.TDefault("w4_mod.s_350_350"),"en":"Main Branch"}'::jsonb) AS branch_name,
			       COALESCE(b.address, '') AS branch_address,
			       COALESCE(b.phone, '') AS branch_phone,
			       COALESCE(b.manager_name, '') AS manager_name
			FROM commerce.order_shipments s
			JOIN commerce.orders ord ON ord.id = s.order_id
			LEFT JOIN org.organizations org ON org.id = ord.organization_id
			LEFT JOIN org.branches b ON b.id = ord.branch_id
			WHERE s.organization_id = $1
			ORDER BY s.created_at DESC
			LIMIT $2 OFFSET $3;
		`
		if limit <= 0 || limit > 100 {
			limit = 50
		}
		rows, err := tx.Query(txCtx, query, vendorOrgID, limit, offset)
		if err != nil {
			return err
		}
		defer rows.Close()

		var shipmentIDs []int64
		shipmentMap := make(map[int64]*commerce.OrderShipment)

		for rows.Next() {
			var s commerce.OrderShipment
			var statusStr, payStatusStr string
			if err := rows.Scan(
				&s.ID, &s.PublicID, &s.OrderID, &s.OrganizationID, &s.BranchID,
				&s.ShipmentNumber, &statusStr, &s.Subtotal, &s.ShippingFee,
				&s.TotalAmount, &s.TrackingNumber, &s.CarrierName,
				&s.DeliveryCode, &s.DeliveryAttempts, &s.DeliveryLockedUntil,
				&s.DeliveryNotes, &s.CollectedAmountMinor, &s.DeliveredByCourierAt,
				&s.ShippedAt, &s.DeliveredAt, &s.CreatedAt, &s.UpdatedAt,
				&s.OrderNumber, &s.PaymentMethod, &payStatusStr, &s.Notes,
				&s.CustomerOrgName, &s.CustomerBranchName, &s.CustomerBranchAddress,
				&s.CustomerBranchPhone, &s.CustomerManagerName,
			); err != nil {
				return err
			}
			s.Status = commerce.OrderStatus(statusStr)
			s.PaymentStatus = commerce.PaymentStatus(payStatusStr)
			shipments = append(shipments, &s)
			shipmentIDs = append(shipmentIDs, s.ID)
			shipmentMap[s.ID] = &s
		}

		if len(shipmentIDs) > 0 {
			queryLines := `
				SELECT id, order_id, shipment_id, organization_id, product_id,
				       product_variant_id, product_name, variant_name, sku,
				       offer_product_id, unit_price, quantity, discount_amount,
				       total_price, cost_price, COALESCE(cost_discount_percentage, 0.00),
				       list_price, original_price, original_discount, rating
				FROM commerce.order_lines
				WHERE shipment_id = ANY($1)
				ORDER BY id ASC;
			`
			lRows, err := tx.Query(txCtx, queryLines, shipmentIDs)
			if err == nil {
				defer lRows.Close()
				for lRows.Next() {
					var l commerce.OrderLine
					if err := lRows.Scan(
						&l.ID, &l.OrderID, &l.ShipmentID, &l.OrganizationID, &l.ProductID,
						&l.ProductVariantID, &l.ProductName, &l.VariantName, &l.SKU,
						&l.OfferProductID, &l.UnitPrice, &l.Quantity, &l.DiscountAmount,
						&l.TotalPrice, &l.CostPrice, &l.CostDiscountPercentage,
						&l.ListPrice, &l.OriginalPrice, &l.OriginalDiscount, &l.Rating,
					); err == nil {
						if sh, ok := shipmentMap[l.ShipmentID]; ok {
							sh.Lines = append(sh.Lines, &l)
						}
					}
				}
			}
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

// AcceptNegotiation confirms a negotiated order and sets its status to confirmed.
func (r *Repository) AcceptNegotiation(ctx context.Context, orderID int64, actorID int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			UPDATE commerce.orders
			SET negotiation_status = 'accepted',
			    status = 'confirmed',
			    updated_at = now()
			WHERE id = $1;
		`
		ct, err := tx.Exec(txCtx, query, orderID)
		if err != nil {
			return err
		}
		if ct.RowsAffected() == 0 {
			return apperr.NotFound("order")
		}

		// Also update shipments
		_, _ = tx.Exec(txCtx, `UPDATE commerce.order_shipments SET status = 'confirmed', updated_at = now() WHERE order_id = $1;`, orderID)

		// Record in history
		_, _ = tx.Exec(txCtx, `
			INSERT INTO commerce.order_status_history (order_id, to_status, notes, changed_by_user_id)
			VALUES ($1, 'confirmed', 'تم قبول السعر المتفاوض عليه واعتماد الطلب من قبل المورد', $2);
		`, orderID, actorID)

		return nil
	})
}

// RejectNegotiation rejects a negotiated order and sets its status to cancelled.
func (r *Repository) RejectNegotiation(ctx context.Context, orderID int64, reason string, actorID int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		if reason == "" {
			reason = i18n.TDefault("w4_mod.s_360_360")
		}
		query := `
			UPDATE commerce.orders
			SET negotiation_status = 'rejected',
			    status = 'cancelled',
			    negotiation_notes = $2,
			    updated_at = now()
			WHERE id = $1;
		`
		ct, err := tx.Exec(txCtx, query, orderID, reason)
		if err != nil {
			return err
		}
		if ct.RowsAffected() == 0 {
			return apperr.NotFound("order")
		}

		// Also update shipments
		_, _ = tx.Exec(txCtx, `UPDATE commerce.order_shipments SET status = 'cancelled', updated_at = now() WHERE order_id = $1;`, orderID)

		// Record in history
		_, _ = tx.Exec(txCtx, `
			INSERT INTO commerce.order_status_history (order_id, to_status, notes, changed_by_user_id)
			VALUES ($1, 'cancelled', $2, $3);
		`, orderID, reason, actorID)

		return nil
	})
}
