package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// MonthSalesByVendor sums a vendor's sold order-line totals for the current
// month. The query runs tenant-scoped: order_lines.organization_id is the
// seller, so the caller's own tenant context already scopes it and the explicit
// predicate is the documented second guard.
func (r *Repository) MonthSalesByVendor(ctx context.Context, vendorOrgID int64) (money.Amount, error) {
	var total money.Amount
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		const query = `
			SELECT COALESCE(SUM(total_price), 0)
			FROM commerce.order_lines
			WHERE organization_id = $1 AND created_at >= date_trunc('month', now());
		`
		return tx.QueryRow(txCtx, query, vendorOrgID).Scan(&total)
	})
	return total, err
}

// MonthSpendByCustomer sums what a buyer paid across all suppliers this month.
//
// This is deliberately cross-tenant: a pharmacy's purchases sit in order_lines
// owned by each supplier it bought from, so no single tenant context would see
// the whole month. The predicate is the guard here, not RLS.
func (r *Repository) MonthSpendByCustomer(ctx context.Context, customerID int64) (money.Amount, error) {
	var total money.Amount
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const query = `
			SELECT COALESCE(SUM(ol.total_price), 0)
			FROM commerce.order_lines ol
			JOIN commerce.orders o ON o.id = ol.order_id
			WHERE o.customer_id = $1 AND ol.created_at >= date_trunc('month', now());
		`
		return tx.QueryRow(txCtx, query, customerID).Scan(&total)
	})
	return total, err
}
