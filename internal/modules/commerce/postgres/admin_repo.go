package postgres

import (
	"context"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// AdminSearchOrders finds orders across every tenant.
func (r *Repository) AdminSearchOrders(ctx context.Context, query string, limit, offset int) ([]*commerce.Order, error) {
	var list []*commerce.Order
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const sql = `
			SELECT ` + orderColumns + `
			FROM commerce.orders
			WHERE deleted_at IS NULL
			  AND ($1::text = '' OR order_number ILIKE '%' || $1 || '%')
			ORDER BY created_at DESC, id DESC
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

// AdminSearchOrdersWithTotal finds orders across every tenant with pagination and optional tab filtering.
func (r *Repository) AdminSearchOrdersWithTotal(ctx context.Context, query, tab string, limit, offset int) ([]*commerce.Order, int, error) {
	var list []*commerce.Order
	var total int
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		where := []string{"deleted_at IS NULL"}
		var args []any

		if query != "" {
			args = append(args, "%"+query+"%")
			p := "$" + strconv.Itoa(len(args))
			where = append(where, "order_number ILIKE "+p)
		}

		if tab == "direct" {
			where = append(where, "is_negotiation = false")
		} else if tab == "negotiations" {
			where = append(where, "is_negotiation = true")
		}

		clause := strings.Join(where, " AND ")

		countSQL := "SELECT count(*) FROM commerce.orders WHERE " + clause + ";"
		if err := tx.QueryRow(txCtx, countSQL, args...).Scan(&total); err != nil {
			return err
		}

		if limit <= 0 || limit > 100 {
			limit = 25
		}
		if offset < 0 {
			offset = 0
		}

		args = append(args, limit, offset)
		limParam := "$" + strconv.Itoa(len(args)-1)
		offParam := "$" + strconv.Itoa(len(args))

		sql := `
			SELECT ` + orderColumns + `
			FROM commerce.orders
			WHERE ` + clause + `
			ORDER BY created_at DESC, id DESC
			LIMIT ` + limParam + ` OFFSET ` + offParam + `;
		`

		rows, err := tx.Query(txCtx, sql, args...)
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
	return list, total, err
}

// AdminOrderStats returns count of all, direct, and negotiation orders.
func (r *Repository) AdminOrderStats(ctx context.Context) (allCount, directCount, negotiationCount int, err error) {
	err = r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const query = `
			SELECT
				COUNT(*) AS all_orders,
				COUNT(*) FILTER (WHERE is_negotiation = false) AS direct_orders,
				COUNT(*) FILTER (WHERE is_negotiation = true) AS negotiation_orders
			FROM commerce.orders
			WHERE deleted_at IS NULL;
		`
		return tx.QueryRow(txCtx, query).Scan(&allCount, &directCount, &negotiationCount)
	})
	return allCount, directCount, negotiationCount, err
}
