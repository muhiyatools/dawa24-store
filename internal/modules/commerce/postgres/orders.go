package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// orderColumns is the canonical order projection (063: offer_id,
// branch_id, vendor_branch_id, user_address_id, total_discount, final_price).
// All order scans use it so the column order lives in exactly one place.
const orderColumns = `id, public_id, order_number, customer_id, organization_id,
	offer_id, branch_id, vendor_branch_id, user_address_id, status,
	subtotal, discount_amount, total_discount, shipping_fee, tax_amount,
	total_amount, final_price, payment_method, payment_status, notes,
	is_negotiation, negotiation_status, COALESCE(negotiation_notes, '') AS negotiation_notes,
	rating, review, rated_at, delivered_at,
	created_at, updated_at, deleted_at`

// scanOrder scans one order row (pgx.Row covers both QueryRow and Rows).
func scanOrder(row pgx.Row) (*commerce.Order, error) {
	var o commerce.Order
	var statusStr, payStatusStr string
	var offerID *int64
	if err := row.Scan(
		&o.ID, &o.PublicID, &o.OrderNumber, &o.CustomerID, &o.OrganizationID,
		&offerID, &o.BranchID, &o.VendorBranchID, &o.UserAddressID,
		&statusStr, &o.Subtotal, &o.DiscountAmount, &o.TotalDiscount, &o.ShippingFee,
		&o.TaxAmount, &o.TotalAmount, &o.FinalPrice,
		&o.PaymentMethod, &payStatusStr, &o.Notes,
		&o.IsNegotiation, &o.NegotiationStatus, &o.NegotiationNotes,
		&o.Rating, &o.Review, &o.RatedAt, &o.DeliveredAt,
		&o.CreatedAt, &o.UpdatedAt, &o.DeletedAt,
	); err != nil {
		return nil, err
	}
	if offerID != nil {
		o.OfferID = *offerID
	}
	o.Status = commerce.OrderStatus(statusStr)
	o.PaymentStatus = commerce.PaymentStatus(payStatusStr)
	return &o, nil
}

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
				order_number, customer_id, organization_id, offer_id, branch_id,
				vendor_branch_id, user_address_id, status, subtotal,
				discount_amount, total_discount, shipping_fee, tax_amount, total_amount,
				final_price, payment_method, payment_status, notes,
				is_negotiation, negotiation_status, negotiation_notes
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21)
			RETURNING id, public_id, created_at, updated_at;
		`
		var offerID *int64
		if order.OfferID > 0 {
			offerID = &order.OfferID
		}
		var branchID *int64
		if order.BranchID != nil && *order.BranchID > 0 {
			branchID = order.BranchID
		}
		var vendorBranchID *int64
		if order.VendorBranchID != nil && *order.VendorBranchID > 0 {
			vendorBranchID = order.VendorBranchID
		}
		var userAddressID *int64
		if order.UserAddressID != nil && *order.UserAddressID > 0 {
			userAddressID = order.UserAddressID
		}
		var orgID *int64
		if order.OrganizationID != nil && *order.OrganizationID > 0 {
			orgID = order.OrganizationID
		}

		negStatus := string(order.NegotiationStatus)
		if negStatus == "" {
			negStatus = "none"
		}

		err := tx.QueryRow(txCtx, queryOrder,
			order.OrderNumber, order.CustomerID, orgID, offerID,
			branchID, vendorBranchID, userAddressID, string(order.Status),
			order.Subtotal, order.DiscountAmount, order.TotalDiscount, order.ShippingFee,
			order.TaxAmount, order.TotalAmount, order.FinalPrice,
			order.PaymentMethod, string(order.PaymentStatus), order.Notes,
			order.IsNegotiation, negStatus, order.NegotiationNotes,
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
			if s.DeliveryCode == "" {
				s.DeliveryCode = commerce.GenerateDeliveryCode()
			}
			if s.TrackingNumber == "" {
				s.TrackingNumber = commerce.GenerateTrackingNumber(order.OrderNumber, seq+1)
			}

			var sBranchID *int64
			if s.BranchID != nil && *s.BranchID > 0 {
				sBranchID = s.BranchID
			}

			queryShipment := `
				INSERT INTO commerce.order_shipments (
					order_id, organization_id, branch_id, shipment_number,
					status, subtotal, shipping_fee, total_amount, tracking_number, delivery_code
				) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
				RETURNING id, public_id, created_at, updated_at;
			`
			err := tx.QueryRow(txCtx, queryShipment,
				s.OrderID, s.OrganizationID, sBranchID, s.ShipmentNumber,
				string(s.Status), s.Subtotal, s.ShippingFee, s.TotalAmount,
				s.TrackingNumber, s.DeliveryCode,
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

			var pID *int64
			if line.ProductID != nil && *line.ProductID > 0 {
				pID = line.ProductID
			}
			var pvID *int64
			if line.ProductVariantID != nil && *line.ProductVariantID > 0 {
				pvID = line.ProductVariantID
			}
			var opID *int64
			if line.OfferProductID != nil && *line.OfferProductID > 0 {
				opID = line.OfferProductID
			}

			pName := line.ProductName
			vName := line.VariantName
			sku := line.SKU

			if pName.IsEmpty() && pvID != nil && *pvID > 0 {
				var fetchedPName, fetchedVName, fetchedSKU string
				var fetchedPID int64
				var fetchedCostPrice *money.Amount
				var fetchedCostDiscount float64
				err := tx.QueryRow(txCtx, `
					SELECT COALESCE(NULLIF(p.name->>'ar', ''), NULLIF(p.name->>'en', ''), pv.sku, ''),
					       COALESCE(NULLIF(pv.name->>'ar', ''), NULLIF(pv.name->>'en', ''), ''),
					       COALESCE(pv.sku, ''),
					       p.id,
					       pv.cost_price,
					       COALESCE(pv.cost_discount_percentage, 0.00)
					FROM catalog.product_variants pv
					JOIN catalog.products p ON p.id = pv.product_id
					WHERE pv.id = $1`, *pvID).Scan(&fetchedPName, &fetchedVName, &fetchedSKU, &fetchedPID, &fetchedCostPrice, &fetchedCostDiscount)
				if err == nil {
					if pName.IsEmpty() && fetchedPName != "" {
						pName = i18n.New(fetchedPName, fetchedPName)
					}
					if vName.IsEmpty() && fetchedVName != "" {
						vName = i18n.New(fetchedVName, fetchedVName)
					}
					if sku == "" {
						sku = fetchedSKU
					}
					if pID == nil && fetchedPID > 0 {
						pID = &fetchedPID
					}
					if line.CostPrice == nil && fetchedCostPrice != nil && fetchedCostPrice.IsPositive() {
						line.CostPrice = fetchedCostPrice
						line.CostDiscountPercentage = fetchedCostDiscount
					}
				}
			}

			if pName.IsEmpty() && pID != nil && *pID > 0 {
				var fetchedPName, fetchedSKU string
				err := tx.QueryRow(txCtx, `
					SELECT COALESCE(NULLIF(name->>'ar', ''), NULLIF(name->>'en', ''), sku, ''),
					       COALESCE(sku, '')
					FROM catalog.products
					WHERE id = $1`, *pID).Scan(&fetchedPName, &fetchedSKU)
				if err == nil {
					if pName.IsEmpty() && fetchedPName != "" {
						pName = i18n.New(fetchedPName, fetchedPName)
					}
					if sku == "" {
						sku = fetchedSKU
					}
				}
			}

			if pName.IsEmpty() {
				pName = i18n.New("صنف دواء 24", "Dawa24 Product")
			}

			queryLine := `
				INSERT INTO commerce.order_lines (
					order_id, shipment_id, organization_id, product_id,
					product_variant_id, product_name, variant_name, sku,
					offer_product_id, unit_price, quantity, discount_amount,
					total_price, cost_price, cost_discount_percentage,
					list_price, original_price, original_discount,
					is_negotiated, proposed_unit_price
				) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20)
				RETURNING id, created_at;
			`
			err := tx.QueryRow(txCtx, queryLine,
				line.OrderID, line.ShipmentID, line.OrganizationID, pID,
				pvID, pName, vName, sku,
				opID, line.UnitPrice, line.Quantity, line.DiscountAmount,
				line.TotalPrice, line.CostPrice, line.CostDiscountPercentage,
				line.ListPrice, line.OriginalPrice, line.OriginalDiscount,
				line.IsNegotiated, line.ProposedUnitPrice,
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

func hydrateOrderDetails(txCtx context.Context, tx pgx.Tx, o *commerce.Order) error {
	if o == nil {
		return nil
	}

	// 1. Hydrate customer branch & organization info
	queryCustomer := `
		SELECT COALESCE(org.name, '{"ar":"","en":""}'::jsonb),
		       COALESCE(b.name, '{"ar":"","en":""}'::jsonb),
		       COALESCE(b.address, ''),
		       COALESCE(b.phone, ''),
		       COALESCE(b.manager_name, '')
		FROM commerce.orders ord
		LEFT JOIN org.organizations org ON org.id = ord.organization_id
		LEFT JOIN org.branches b ON b.id = ord.branch_id
		WHERE ord.id = $1;
	`
	_ = tx.QueryRow(txCtx, queryCustomer, o.ID).Scan(
		&o.CustomerOrgName,
		&o.CustomerBranchName,
		&o.CustomerBranchAddress,
		&o.CustomerBranchPhone,
		&o.CustomerManagerName,
	)

	// 2. Load shipments
	queryShipments := `
		SELECT s.id, s.public_id, s.order_id, s.organization_id, s.branch_id, s.shipment_number,
		       s.status, s.subtotal, s.shipping_fee, s.total_amount, s.tracking_number,
		       s.carrier_name, s.delivery_code, s.delivery_attempts, s.delivery_locked_until,
		       s.delivery_notes, s.collected_amount_minor, s.delivered_by_courier_at,
		       s.shipped_at, s.delivered_at, s.created_at, s.updated_at,
		       COALESCE(org.name, '{"ar":"مورد معتمد","en":"Approved Supplier"}'::jsonb) AS vendor_name
		FROM commerce.order_shipments s
		LEFT JOIN org.organizations org ON org.id = s.organization_id
		WHERE s.order_id = $1
		ORDER BY s.id ASC;
	`
	sRows, err := tx.Query(txCtx, queryShipments, o.ID)
	if err == nil {
		defer sRows.Close()
		shipmentMap := make(map[int64]*commerce.OrderShipment)
		for sRows.Next() {
			var s commerce.OrderShipment
			var statusStr string
			if err := sRows.Scan(
				&s.ID, &s.PublicID, &s.OrderID, &s.OrganizationID, &s.BranchID,
				&s.ShipmentNumber, &statusStr, &s.Subtotal, &s.ShippingFee,
				&s.TotalAmount, &s.TrackingNumber, &s.CarrierName,
				&s.DeliveryCode, &s.DeliveryAttempts, &s.DeliveryLockedUntil,
				&s.DeliveryNotes, &s.CollectedAmountMinor, &s.DeliveredByCourierAt,
				&s.ShippedAt, &s.DeliveredAt, &s.CreatedAt, &s.UpdatedAt,
				&s.VendorName,
			); err == nil {
				s.Status = commerce.OrderStatus(statusStr)
				s.OrderNumber = o.OrderNumber
				s.CustomerOrgName = o.CustomerOrgName
				s.CustomerBranchName = o.CustomerBranchName
				s.CustomerBranchAddress = o.CustomerBranchAddress
				s.CustomerBranchPhone = o.CustomerBranchPhone
				s.CustomerManagerName = o.CustomerManagerName
				s.PaymentMethod = o.PaymentMethod
				s.PaymentStatus = o.PaymentStatus
				s.Notes = o.Notes

				o.Shipments = append(o.Shipments, &s)
				shipmentMap[s.ID] = &s
			}
		}

		// 3. Load order lines with real-time stock & constraint metadata
		var orgIDParam int64
		if o.OrganizationID != nil {
			orgIDParam = *o.OrganizationID
		}

		queryLines := `
			SELECT l.id, l.order_id, l.shipment_id, l.organization_id, l.product_id,
			       l.product_variant_id, l.product_name, l.variant_name, l.sku,
			       l.offer_product_id, l.unit_price, l.quantity, l.discount_amount,
			       l.total_price, l.cost_price, COALESCE(l.cost_discount_percentage, 0.00),
			       l.list_price, l.original_price, l.original_discount,
			       l.is_negotiated, COALESCE(l.proposed_unit_price, 0) as proposed_unit_price, l.rating,
			       COALESCE(stk.available_stock, 0) as available_stock,
			       COALESCE(pv.min_order_qty, 1) as min_order_qty,
			       COALESCE(op.max_qty_per_order, 0) as max_qty_per_order
			FROM commerce.order_lines l
			LEFT JOIN catalog.product_variants pv ON pv.id = l.product_variant_id AND pv.deleted_at IS NULL
			LEFT JOIN promo.offer_products op ON op.id = l.offer_product_id
			LEFT JOIN LATERAL (
			    SELECT COALESCE(SUM(s.quantity), 0)::int AS available_stock
			    FROM inventory.stocks s
			    WHERE s.product_variant_id = l.product_variant_id
			      AND (s.organization_id = l.organization_id OR ($2::bigint > 0 AND s.organization_id = $2))
			      AND s.deleted_at IS NULL
			) stk ON true
			WHERE l.order_id = $1
			ORDER BY l.id ASC;
		`
		lRows, err := tx.Query(txCtx, queryLines, o.ID, orgIDParam)
		if err == nil {
			defer lRows.Close()
			for lRows.Next() {
				var l commerce.OrderLine
				if err := lRows.Scan(
					&l.ID, &l.OrderID, &l.ShipmentID, &l.OrganizationID, &l.ProductID,
					&l.ProductVariantID, &l.ProductName, &l.VariantName, &l.SKU,
					&l.OfferProductID, &l.UnitPrice, &l.Quantity, &l.DiscountAmount,
					&l.TotalPrice, &l.CostPrice, &l.CostDiscountPercentage,
					&l.ListPrice, &l.OriginalPrice, &l.OriginalDiscount,
					&l.IsNegotiated, &l.ProposedUnitPrice, &l.Rating,
					&l.AvailableStock, &l.MinOrderQty, &l.MaxQtyPerOrder,
				); err == nil {
					o.Lines = append(o.Lines, &l)
					if sh, ok := shipmentMap[l.ShipmentID]; ok {
						sh.Lines = append(sh.Lines, &l)
					}
				}
			}
		}
	}
	return nil
}

// GetOrderByID retrieves order details.
func (r *Repository) GetOrderByID(ctx context.Context, id int64) (*commerce.Order, error) {
	var o *commerce.Order
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT ` + orderColumns + `
			FROM commerce.orders
			WHERE id = $1 AND deleted_at IS NULL;
		`
		var err error
		o, err = scanOrder(tx.QueryRow(txCtx, query, id))
		if err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("order")
			}
			return err
		}
		return hydrateOrderDetails(txCtx, tx, o)
	})
	if err != nil {
		return nil, err
	}
	return o, nil
}

// GetOrderByNumber retrieves an order by its public business identifier.
func (r *Repository) GetOrderByNumber(ctx context.Context, number string) (*commerce.Order, error) {
	var o *commerce.Order
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT ` + orderColumns + `
			FROM commerce.orders
			WHERE order_number = $1 AND deleted_at IS NULL;
		`
		var err error
		o, err = scanOrder(tx.QueryRow(txCtx, query, number))
		if err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("order")
			}
			return err
		}
		return hydrateOrderDetails(txCtx, tx, o)
	})
	if err != nil {
		return nil, err
	}
	return o, nil
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
			SELECT ` + orderColumns + `
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
			o, err := scanOrder(rows)
			if err != nil {
				return err
			}
			orders = append(orders, o)
		}
		return rows.Err()
	})
	return orders, err
}

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
			       COALESCE(org.name, '{"ar":"صيدلية معتمدة","en":"Approved Pharmacy"}'::jsonb) AS customer_org_name,
			       COALESCE(b.name, '{"ar":"الفرع الرئيسي","en":"Main Branch"}'::jsonb) AS branch_name,
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
			reason = "تم رفض السعر المقترح من قبل المورد"
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
