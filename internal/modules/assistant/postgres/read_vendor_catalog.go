package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/assistant"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
)

// VendorProducts lists this vendor's own catalogue.
func (r *Repository) VendorProducts(
	ctx context.Context, actor authctx.Actor, q assistant.ProductQuery,
) (assistant.Page[assistant.VendorProductRow], error) {
	var empty assistant.Page[assistant.VendorProductRow]
	orgID, err := scopeOf(actor)
	if err != nil {
		return empty, err
	}

	args := []any{orgID}
	where := ` WHERE p.organization_id = $1 AND p.deleted_at IS NULL`
	if q.Status != "" {
		args = append(args, q.Status)
		where += fmt.Sprintf(" AND p.status = $%d", len(args))
	}
	if q.Search != "" {
		args = append(args, "%"+q.Search+"%")
		n := len(args)
		where += fmt.Sprintf(
			" AND (p.name->>'ar' ILIKE $%d OR p.name->>'en' ILIKE $%d OR COALESCE(p.sku,'') ILIKE $%d)",
			n, n, n)
	}
	args = append(args, q.Limit+1, q.Offset)
	limitArg, offsetArg := len(args)-1, len(args)

	var out []assistant.VendorProductRow
	err = r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(txCtx, `
			SELECT p.id, `+nameExpr("p.name")+`, COALESCE(p.sku,''),
			       p.price::text, p.discount::text, p.status, p.sold_times,
			       (SELECT COALESCE(SUM(s.quantity),0)::int
			          FROM inventory.stocks s
			         WHERE s.product_id = p.id AND s.deleted_at IS NULL)
			  FROM catalog.products p`+where+`
			 ORDER BY p.sold_times DESC, p.id DESC
			 LIMIT $`+fmt.Sprint(limitArg)+` OFFSET $`+fmt.Sprint(offsetArg)+`;
		`, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var row assistant.VendorProductRow
			var price, disc string
			var stock int
			if err := rows.Scan(&row.ID, &row.Name, &row.SKU, &price, &disc,
				&row.Status, &row.SoldTimes, &stock); err != nil {
				return err
			}
			row.Price, row.Discount = amount(price), amount(disc)
			row.FinalPrice = netPrice(row.Price, row.Discount)
			row.Stock = &stock
			out = append(out, row)
		}
		return rows.Err()
	})
	if err != nil {
		return empty, fmt.Errorf("assistant read: vendor products: %w", err)
	}
	return pageOf(out, q.Limit, q.Offset), nil
}

// LowStock lists items at or below their reorder threshold.
func (r *Repository) LowStock(
	ctx context.Context, actor authctx.Actor, limit int,
) (assistant.Page[assistant.LowStockRow], error) {
	var empty assistant.Page[assistant.LowStockRow]
	orgID, err := scopeOf(actor)
	if err != nil {
		return empty, err
	}

	var out []assistant.LowStockRow
	err = r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(txCtx, `
			SELECT `+nameExpr("p.name")+`, COALESCE(`+nameExpr("v.name")+`,''),
			       COALESCE(`+nameExpr("w.name")+`,''), s.quantity, s.min_threshold
			  FROM inventory.stocks s
			  JOIN catalog.products p ON p.id = s.product_id
			  LEFT JOIN catalog.product_variants v ON v.id = s.product_variant_id
			  LEFT JOIN inventory.warehouses w ON w.id = s.warehouse_id
			 WHERE s.organization_id = $1
			   AND s.deleted_at IS NULL
			   AND s.quantity <= s.min_threshold
			 ORDER BY (s.quantity - s.min_threshold) ASC, s.quantity ASC
			 LIMIT $2;
		`, orgID, limit+1)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var row assistant.LowStockRow
			if err := rows.Scan(&row.ProductName, &row.VariantName, &row.Warehouse,
				&row.Quantity, &row.MinThreshold); err != nil {
				return err
			}
			out = append(out, row)
		}
		return rows.Err()
	})
	if err != nil {
		return empty, fmt.Errorf("assistant read: low stock: %w", err)
	}
	return pageOf(out, limit, 0), nil
}

// Offers lists this vendor's promotional offers and their reach.
func (r *Repository) Offers(
	ctx context.Context, actor authctx.Actor, activeOnly bool, limit int,
) (assistant.Page[assistant.OfferRow], error) {
	var empty assistant.Page[assistant.OfferRow]
	orgID, err := scopeOf(actor)
	if err != nil {
		return empty, err
	}

	where := ` WHERE o.organization_id = $1 AND o.deleted_at IS NULL`
	if activeOnly {
		where += ` AND o.is_active = true AND o.expires_at > now() AND o.starts_at <= now()`
	}

	var out []assistant.OfferRow
	err = r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(txCtx, `
			SELECT o.id, `+nameExpr("o.title")+`, o.discount_type,
			       o.discount_value::text, o.min_order_value::text,
			       o.starts_at, o.expires_at, o.is_active,
			       o.views_count, o.clicks_count,
			       (SELECT COUNT(*) FROM promo.offer_products op WHERE op.offer_id = o.id)
			  FROM promo.offers o`+where+`
			 ORDER BY o.expires_at DESC, o.id DESC
			 LIMIT $2;
		`, orgID, limit+1)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var row assistant.OfferRow
			var minOrder string
			if err := rows.Scan(&row.ID, &row.Title, &row.DiscountType, &row.DiscountValue,
				&minOrder, &row.StartsAt, &row.ExpiresAt, &row.Active,
				&row.Views, &row.Clicks, &row.ProductCount); err != nil {
				return err
			}
			row.MinOrderValue = amount(minOrder)
			out = append(out, row)
		}
		return rows.Err()
	})
	if err != nil {
		return empty, fmt.Errorf("assistant read: offers: %w", err)
	}
	return pageOf(out, limit, 0), nil
}
