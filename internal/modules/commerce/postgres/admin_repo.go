package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// AdminSearchOrders finds orders across every tenant.
//
// This was a stub returning nil behind a live route
// (GET /api/v1/admin/commerce/orders), so the platform order search answered
// 200 with an empty list no matter what was typed - indistinguishable from
// "no orders match".
//
// The search is deliberately narrow: order number and payment status, both
// indexed, rather than a LIKE across every text column. An admin looking up a
// disputed order knows its number.
func (r *Repository) AdminSearchOrders(ctx context.Context, query string, limit, offset int) ([]*commerce.Order, error) {
	var list []*commerce.Order
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const sql = `
			SELECT ` + orderColumns + `
			FROM commerce.orders
			WHERE deleted_at IS NULL
			  AND ($1::text = '' OR order_number ILIKE '%' || $1 || '%')
			ORDER BY created_at DESC
			LIMIT $2 OFFSET $3;
		`
		if limit <= 0 || limit > 200 {
			limit = 50
		}
		if offset < 0 {
			offset = 0
		}

		rows, err := tx.Query(txCtx, sql, query, limit, offset)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			o, err := scanOrder(rows)
			if err != nil {
				return err
			}
			list = append(list, o)
		}
		return rows.Err()
	})
	return list, err
}
