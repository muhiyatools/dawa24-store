package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/assistant"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// The selling side.
//
// commerce.order_shipments.organization_id is the SELLER, and its row-level
// security policy scopes it to the current tenant — so the same trade a
// pharmacy reads through commerce.orders, a supplier reads through this table.
// Neither side can reach the other's view of it, and neither query has to say
// so: the database refuses on its own.

// SupplyOrders lists shipments this vendor owes.
func (r *Repository) SupplyOrders(
	ctx context.Context, actor authctx.Actor, q assistant.OrderQuery,
) (assistant.Page[assistant.SupplyOrderRow], error) {
	var empty assistant.Page[assistant.SupplyOrderRow]
	orgID, err := scopeOf(actor)
	if err != nil {
		return empty, err
	}

	args := []any{orgID}
	where := ` WHERE sh.organization_id = $1`
	if q.Status != "" {
		args = append(args, q.Status)
		where += fmt.Sprintf(" AND sh.status = $%d", len(args))
	}
	if q.Search != "" {
		args = append(args, "%"+q.Search+"%")
		where += fmt.Sprintf(" AND (sh.shipment_number ILIKE $%d OR o.order_number ILIKE $%d)",
			len(args), len(args))
	}
	frag, args := dateFilter("sh.created_at", q.Range, args)
	where += frag

	args = append(args, q.Limit+1, q.Offset)
	limitArg, offsetArg := len(args)-1, len(args)

	var out []assistant.SupplyOrderRow
	err = r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(txCtx, `
			SELECT sh.id, sh.shipment_number, o.order_number,
			       COALESCE(`+nameExpr("buyer.name")+`,''), sh.status,
			       sh.total_amount::text, sh.created_at,
			       (SELECT COUNT(*) FROM commerce.order_lines l WHERE l.shipment_id = sh.id)
			  FROM commerce.order_shipments sh
			  JOIN commerce.orders o ON o.id = sh.order_id
			  LEFT JOIN org.organizations buyer ON buyer.id = o.organization_id`+where+`
			 ORDER BY sh.created_at DESC, sh.id DESC
			 LIMIT $`+fmt.Sprint(limitArg)+` OFFSET $`+fmt.Sprint(offsetArg)+`;
		`, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var row assistant.SupplyOrderRow
			var total string
			if err := rows.Scan(&row.ID, &row.Number, &row.OrderNumber, &row.Buyer,
				&row.Status, &total, &row.PlacedAt, &row.LineCount); err != nil {
				return err
			}
			row.Total = amount(total)
			out = append(out, row)
		}
		return rows.Err()
	})
	if err != nil {
		return empty, fmt.Errorf("assistant read: supply orders: %w", err)
	}
	return pageOf(out, q.Limit, q.Offset), nil
}

// SupplyOrderDetail returns one shipment and its lines.
func (r *Repository) SupplyOrderDetail(
	ctx context.Context, actor authctx.Actor, shipmentID int64,
) (*assistant.SupplyOrderDetail, error) {
	orgID, err := scopeOf(actor)
	if err != nil {
		return nil, err
	}

	var detail *assistant.SupplyOrderDetail
	err = r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		var row assistant.SupplyOrderRow
		var total, branch string
		err := tx.QueryRow(txCtx, `
			SELECT sh.id, sh.shipment_number, o.order_number,
			       COALESCE(`+nameExpr("buyer.name")+`,''), sh.status,
			       sh.total_amount::text, sh.created_at,
			       COALESCE(`+nameExpr("br.name")+`,'')
			  FROM commerce.order_shipments sh
			  JOIN commerce.orders o ON o.id = sh.order_id
			  LEFT JOIN org.organizations buyer ON buyer.id = o.organization_id
			  LEFT JOIN org.branches br ON br.id = sh.branch_id
			 WHERE sh.id = $1 AND sh.organization_id = $2;
		`, shipmentID, orgID).Scan(&row.ID, &row.Number, &row.OrderNumber, &row.Buyer,
			&row.Status, &total, &row.PlacedAt, &branch)
		if err != nil {
			if err == pgx.ErrNoRows {
				return nil
			}
			return err
		}
		row.Total = amount(total)
		d := &assistant.SupplyOrderDetail{Shipment: row, Branch: branch}

		lines, err := tx.Query(txCtx, `
			SELECT `+nameExpr("l.product_name")+`, COALESCE(l.sku,''),
			       l.unit_price::text, l.quantity, l.discount_amount::text, l.total_price::text
			  FROM commerce.order_lines l
			 WHERE l.shipment_id = $1
			 ORDER BY l.id ASC
			 LIMIT 60;
		`, shipmentID)
		if err != nil {
			return err
		}
		defer lines.Close()
		for lines.Next() {
			var ln assistant.OrderLineRow
			var unit, disc, tot string
			if err := lines.Scan(&ln.ProductName, &ln.SKU, &unit, &ln.Quantity, &disc, &tot); err != nil {
				return err
			}
			ln.UnitPrice, ln.Discount, ln.Total = amount(unit), amount(disc), amount(tot)
			d.Lines = append(d.Lines, ln)
		}
		if err := lines.Err(); err != nil {
			return err
		}
		d.Shipment.LineCount = len(d.Lines)
		detail = d
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("assistant read: supply order detail: %w", err)
	}
	return detail, nil
}

// SalesSummary aggregates this vendor's revenue over a period.
func (r *Repository) SalesSummary(
	ctx context.Context, actor authctx.Actor, q assistant.AggregateQuery,
) (*assistant.Aggregate, error) {
	orgID, err := scopeOf(actor)
	if err != nil {
		return nil, err
	}

	args := []any{orgID}
	where := ` WHERE sh.organization_id = $1`
	if q.Status != "" {
		args = append(args, q.Status)
		where += fmt.Sprintf(" AND sh.status = $%d", len(args))
	}
	frag, args := dateFilter("sh.created_at", q.Range, args)
	where += frag

	agg := &assistant.Aggregate{Total: money.FromMinor(0), Average: money.FromMinor(0)}
	agg.From, agg.To = rangeBounds(q.Range)

	err = r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		var total, avg string
		if err := tx.QueryRow(txCtx, `
			SELECT COUNT(*), COALESCE(SUM(sh.total_amount),0)::text,
			       COALESCE(AVG(sh.total_amount),0)::numeric(12,2)::text
			  FROM commerce.order_shipments sh`+where+`;
		`, args...).Scan(&agg.Count, &total, &avg); err != nil {
			return err
		}
		agg.Total, agg.Average = amount(total), amount(avg)
		if agg.Count == 0 || q.Group == assistant.GroupNone {
			return nil
		}
		buckets, err := salesBuckets(txCtx, tx, q.Group, where, args)
		if err != nil {
			return err
		}
		agg.Buckets = buckets
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("assistant read: sales summary: %w", err)
	}
	return agg, nil
}

// salesBuckets splits a revenue total. The grouping SQL is chosen from a fixed
// set rather than built from the argument, so the statement is a constant
// whichever branch runs.
func salesBuckets(
	ctx context.Context, tx pgx.Tx, group assistant.GroupBy, where string, args []any,
) ([]assistant.Bucket, error) {
	var query string
	switch group {
	case assistant.GroupByStatus:
		query = `SELECT sh.status, sh.status, COUNT(*), COALESCE(SUM(sh.total_amount),0)::text
		           FROM commerce.order_shipments sh` + where + `
		          GROUP BY sh.status ORDER BY 4 DESC LIMIT 25;`
	case assistant.GroupByMonth:
		query = `SELECT to_char(date_trunc('month', sh.created_at), 'YYYY-MM'),
		                to_char(date_trunc('month', sh.created_at), 'YYYY-MM'),
		                COUNT(*), COALESCE(SUM(sh.total_amount),0)::text
		           FROM commerce.order_shipments sh` + where + `
		          GROUP BY 1 ORDER BY 1 DESC LIMIT 24;`
	case assistant.GroupByCounterpar:
		query = `SELECT COALESCE(o.organization_id::text,'0'),
		                COALESCE(` + nameExpr("buyer.name") + `, 'unknown'),
		                COUNT(*), COALESCE(SUM(sh.total_amount),0)::text
		           FROM commerce.order_shipments sh
		           JOIN commerce.orders o ON o.id = sh.order_id
		           LEFT JOIN org.organizations buyer ON buyer.id = o.organization_id` + where + `
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

// SoldProducts ranks this vendor's best sellers.
func (r *Repository) SoldProducts(
	ctx context.Context, actor authctx.Actor, q assistant.AggregateQuery,
) (assistant.Page[assistant.SoldProductRow], error) {
	var empty assistant.Page[assistant.SoldProductRow]
	orgID, err := scopeOf(actor)
	if err != nil {
		return empty, err
	}

	args := []any{orgID}
	where := ` WHERE l.organization_id = $1`
	frag, args := dateFilter("sh.created_at", q.Range, args)
	where += frag
	args = append(args, q.Limit+1)

	order := "2 DESC"
	if q.Ranking != "quantity" {
		order = "3 DESC"
	}

	var out []assistant.SoldProductRow
	err = r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(txCtx, `
			SELECT `+nameExpr("l.product_name")+`,
			       COALESCE(SUM(l.quantity),0)::int,
			       COALESCE(SUM(l.total_price),0)::text,
			       COUNT(DISTINCT l.shipment_id)::int
			  FROM commerce.order_lines l
			  JOIN commerce.order_shipments sh ON sh.id = l.shipment_id`+where+`
			 GROUP BY 1
			 ORDER BY `+order+`
			 LIMIT $`+fmt.Sprint(len(args))+`;
		`, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var row assistant.SoldProductRow
			var revenue string
			if err := rows.Scan(&row.ProductName, &row.Quantity, &revenue, &row.Orders); err != nil {
				return err
			}
			row.Revenue = amount(revenue)
			out = append(out, row)
		}
		return rows.Err()
	})
	if err != nil {
		return empty, fmt.Errorf("assistant read: sold products: %w", err)
	}
	return pageOf(out, q.Limit, 0), nil
}
