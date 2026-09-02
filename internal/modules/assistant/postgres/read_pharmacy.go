package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/assistant"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// The buying side.
//
// commerce.orders.organization_id is the BUYER, and its row-level security
// policy scopes it to the current tenant. So a pharmacy reading this table sees
// its own purchases and nothing else, without this file having to say so — and
// a bug that dropped the WHERE clause would return zero rows rather than the
// market's order book.

// PurchaseOrders lists this pharmacy's own orders.
func (r *Repository) PurchaseOrders(
	ctx context.Context, actor authctx.Actor, q assistant.OrderQuery,
) (assistant.Page[assistant.PurchaseOrderRow], error) {
	var empty assistant.Page[assistant.PurchaseOrderRow]
	orgID, err := scopeOf(actor)
	if err != nil {
		return empty, err
	}

	args := []any{orgID}
	where := ` WHERE o.organization_id = $1 AND o.deleted_at IS NULL`
	if q.Status != "" {
		args = append(args, q.Status)
		where += fmt.Sprintf(" AND o.status = $%d", len(args))
	}
	if q.PaymentStatus != "" {
		args = append(args, q.PaymentStatus)
		where += fmt.Sprintf(" AND o.payment_status = $%d", len(args))
	}
	if q.Search != "" {
		args = append(args, "%"+q.Search+"%")
		where += fmt.Sprintf(" AND o.order_number ILIKE $%d", len(args))
	}
	frag, args := dateFilter("o.created_at", q.Range, args)
	where += frag

	args = append(args, q.Limit+1)
	limitArg := len(args)
	args = append(args, q.Offset)
	offsetArg := len(args)

	var rowsOut []assistant.PurchaseOrderRow
	err = r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(txCtx, `
			SELECT o.id, o.order_number, o.status, o.payment_status,
			       o.subtotal::text, o.discount_amount::text, o.shipping_fee::text,
			       o.total_amount::text, o.created_at,
			       (SELECT COUNT(*) FROM commerce.order_lines l WHERE l.order_id = o.id),
			       COALESCE((
			           SELECT array_agg(DISTINCT `+nameExpr("org.name")+`)
			             FROM commerce.order_shipments sh
			             JOIN org.organizations org ON org.id = sh.organization_id
			            WHERE sh.order_id = o.id
			       ), '{}')
			  FROM commerce.orders o`+where+`
			 ORDER BY o.created_at DESC, o.id DESC
			 LIMIT $`+fmt.Sprint(limitArg)+` OFFSET $`+fmt.Sprint(offsetArg)+`;
		`, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var row assistant.PurchaseOrderRow
			var sub, disc, ship, total string
			if err := rows.Scan(&row.ID, &row.Number, &row.Status, &row.PaymentStatus,
				&sub, &disc, &ship, &total, &row.PlacedAt, &row.LineCount, &row.Suppliers); err != nil {
				return err
			}
			row.Subtotal, row.Discount = amount(sub), amount(disc)
			row.Shipping, row.Total = amount(ship), amount(total)
			rowsOut = append(rowsOut, row)
		}
		return rows.Err()
	})
	if err != nil {
		return empty, fmt.Errorf("assistant read: purchase orders: %w", err)
	}
	return pageOf(rowsOut, q.Limit, q.Offset), nil
}

// PurchaseOrderDetail returns one order with its lines and shipments.
//
// The organisation predicate is in the WHERE clause and not in an if-statement
// after the fetch. A wrong id therefore returns no rows: the record is never
// read into memory and then compared, so there is nothing to leak through a
// timing difference or a future refactor that drops the check.
func (r *Repository) PurchaseOrderDetail(
	ctx context.Context, actor authctx.Actor, orderID int64,
) (*assistant.OrderDetail, error) {
	orgID, err := scopeOf(actor)
	if err != nil {
		return nil, err
	}

	var detail *assistant.OrderDetail
	err = r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		var row assistant.PurchaseOrderRow
		var sub, disc, ship, total, notes string
		err := tx.QueryRow(txCtx, `
			SELECT o.id, o.order_number, o.status, o.payment_status,
			       o.subtotal::text, o.discount_amount::text, o.shipping_fee::text,
			       o.total_amount::text, o.created_at, COALESCE(o.notes,'')
			  FROM commerce.orders o
			 WHERE o.id = $1 AND o.organization_id = $2 AND o.deleted_at IS NULL;
		`, orderID, orgID).Scan(&row.ID, &row.Number, &row.Status, &row.PaymentStatus,
			&sub, &disc, &ship, &total, &row.PlacedAt, &notes)
		if err != nil {
			if err == pgx.ErrNoRows {
				return nil
			}
			return err
		}
		row.Subtotal, row.Discount = amount(sub), amount(disc)
		row.Shipping, row.Total = amount(ship), amount(total)

		d := &assistant.OrderDetail{Order: row, Notes: notes}

		lines, err := tx.Query(txCtx, `
			SELECT `+nameExpr("l.product_name")+`, COALESCE(l.sku,''),
			       COALESCE(`+nameExpr("org.name")+`,''),
			       l.unit_price::text, l.quantity, l.discount_amount::text, l.total_price::text
			  FROM commerce.order_lines l
			  LEFT JOIN org.organizations org ON org.id = l.organization_id
			 WHERE l.order_id = $1
			 ORDER BY l.id ASC
			 LIMIT 60;
		`, orderID)
		if err != nil {
			return err
		}
		defer lines.Close()
		for lines.Next() {
			var ln assistant.OrderLineRow
			var unit, ldisc, ltotal string
			if err := lines.Scan(&ln.ProductName, &ln.SKU, &ln.Supplier,
				&unit, &ln.Quantity, &ldisc, &ltotal); err != nil {
				return err
			}
			ln.UnitPrice, ln.Discount, ln.Total = amount(unit), amount(ldisc), amount(ltotal)
			d.Lines = append(d.Lines, ln)
		}
		if err := lines.Err(); err != nil {
			return err
		}

		shipments, err := tx.Query(txCtx, `
			SELECT sh.shipment_number, COALESCE(`+nameExpr("org.name")+`,''), sh.status,
			       sh.total_amount::text, COALESCE(sh.carrier_name,''),
			       COALESCE(sh.tracking_number,''), sh.shipped_at, sh.delivered_at
			  FROM commerce.order_shipments sh
			  LEFT JOIN org.organizations org ON org.id = sh.organization_id
			 WHERE sh.order_id = $1
			 ORDER BY sh.id ASC
			 LIMIT 30;
		`, orderID)
		if err != nil {
			return err
		}
		defer shipments.Close()
		for shipments.Next() {
			var sh assistant.ShipmentRow
			var tot string
			if err := shipments.Scan(&sh.Number, &sh.Counterparty, &sh.Status, &tot,
				&sh.Carrier, &sh.Tracking, &sh.ShippedAt, &sh.DeliveredAt); err != nil {
				return err
			}
			sh.Total = amount(tot)
			d.Shipments = append(d.Shipments, sh)
		}
		if err := shipments.Err(); err != nil {
			return err
		}

		detail = d
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("assistant read: order detail: %w", err)
	}
	return detail, nil
}

// PurchaseSummary aggregates spend over a period.
//
// The whole point of this tool is that the aggregation happens in Postgres.
// The alternative — listing orders and letting the model add them up — is both
// far more expensive in tokens and, more importantly, wrong: a model asked to
// sum forty decimal figures will get it right most of the time, and "most of
// the time" is not a property a pharmacy's monthly spend can have.
func (r *Repository) PurchaseSummary(
	ctx context.Context, actor authctx.Actor, q assistant.AggregateQuery,
) (*assistant.Aggregate, error) {
	orgID, err := scopeOf(actor)
	if err != nil {
		return nil, err
	}

	args := []any{orgID}
	where := ` WHERE o.organization_id = $1 AND o.deleted_at IS NULL`
	if q.Status != "" {
		args = append(args, q.Status)
		where += fmt.Sprintf(" AND o.status = $%d", len(args))
	}
	frag, args := dateFilter("o.created_at", q.Range, args)
	where += frag

	agg := &assistant.Aggregate{Total: money.FromMinor(0), Average: money.FromMinor(0)}
	agg.From, agg.To = rangeBounds(q.Range)

	err = r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		var total, avg string
		if err := tx.QueryRow(txCtx, `
			SELECT COUNT(*), COALESCE(SUM(o.total_amount),0)::text,
			       COALESCE(AVG(o.total_amount),0)::numeric(12,2)::text
			  FROM commerce.orders o`+where+`;
		`, args...).Scan(&agg.Count, &total, &avg); err != nil {
			return err
		}
		agg.Total, agg.Average = amount(total), amount(avg)
		if agg.Count == 0 || q.Group == assistant.GroupNone {
			return nil
		}

		buckets, err := purchaseBuckets(txCtx, tx, q.Group, where, args)
		if err != nil {
			return err
		}
		agg.Buckets = buckets
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("assistant read: purchase summary: %w", err)
	}
	return agg, nil
}

// purchaseBuckets splits a spend total.
//
// The grouping expression is chosen from a fixed set here rather than built
// from the argument: q.Group has already been validated against an enum, and
// this keeps the SQL a constant either way.
func purchaseBuckets(
	ctx context.Context, tx pgx.Tx, group assistant.GroupBy, where string, args []any,
) ([]assistant.Bucket, error) {
	var query string
	switch group {
	case assistant.GroupByStatus:
		query = `SELECT o.status, o.status, COUNT(*), COALESCE(SUM(o.total_amount),0)::text
		           FROM commerce.orders o` + where + `
		          GROUP BY o.status ORDER BY 4 DESC LIMIT 25;`
	case assistant.GroupByMonth:
		query = `SELECT to_char(date_trunc('month', o.created_at), 'YYYY-MM'),
		                to_char(date_trunc('month', o.created_at), 'YYYY-MM'),
		                COUNT(*), COALESCE(SUM(o.total_amount),0)::text
		           FROM commerce.orders o` + where + `
		          GROUP BY 1 ORDER BY 1 DESC LIMIT 24;`
	case assistant.GroupByCounterpar:
		query = `SELECT COALESCE(sh.organization_id::text,'0'),
		                COALESCE(` + nameExpr("org.name") + `, 'غير محدد'),
		                COUNT(DISTINCT o.id), COALESCE(SUM(sh.total_amount),0)::text
		           FROM commerce.orders o
		           JOIN commerce.order_shipments sh ON sh.order_id = o.id
		           LEFT JOIN org.organizations org ON org.id = sh.organization_id` + where + `
		          GROUP BY 1, 2 ORDER BY 4 DESC LIMIT 25;`
	default:
		return nil, nil
	}

	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []assistant.Bucket
	for rows.Next() {
		var b assistant.Bucket
		var total string
		if err := rows.Scan(&b.Key, &b.Label, &b.Count, &total); err != nil {
			return nil, err
		}
		b.Total = amount(total)
		out = append(out, b)
	}
	return out, rows.Err()
}

// PurchasedProducts ranks what this pharmacy spends its budget on.
func (r *Repository) PurchasedProducts(
	ctx context.Context, actor authctx.Actor, q assistant.AggregateQuery,
) (assistant.Page[assistant.ProductSpendRow], error) {
	var empty assistant.Page[assistant.ProductSpendRow]
	orgID, err := scopeOf(actor)
	if err != nil {
		return empty, err
	}

	args := []any{orgID}
	where := ` WHERE o.organization_id = $1 AND o.deleted_at IS NULL`
	frag, args := dateFilter("o.created_at", q.Range, args)
	where += frag
	args = append(args, q.Limit+1)

	order := "3 DESC" // quantity
	if q.Ranking != "quantity" {
		order = "4 DESC" // spend
	}

	var out []assistant.ProductSpendRow
	err = r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(txCtx, `
			SELECT `+nameExpr("l.product_name")+` AS product,
			       COALESCE(`+nameExpr("org.name")+`,'') AS supplier,
			       COALESCE(SUM(l.quantity),0)::int,
			       COALESCE(SUM(l.total_price),0)::text,
			       COUNT(DISTINCT o.id)::int
			  FROM commerce.order_lines l
			  JOIN commerce.orders o ON o.id = l.order_id
			  LEFT JOIN org.organizations org ON org.id = l.organization_id`+where+`
			 GROUP BY 1, 2
			 ORDER BY `+order+`
			 LIMIT $`+fmt.Sprint(len(args))+`;
		`, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var row assistant.ProductSpendRow
			var total string
			if err := rows.Scan(&row.ProductName, &row.Supplier, &row.Quantity,
				&total, &row.Orders); err != nil {
				return err
			}
			row.Total = amount(total)
			out = append(out, row)
		}
		return rows.Err()
	})
	if err != nil {
		return empty, fmt.Errorf("assistant read: purchased products: %w", err)
	}
	return pageOf(out, q.Limit, 0), nil
}
