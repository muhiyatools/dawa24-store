package postgres

import (
	"context"
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/billing"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// Platform administration over billing.
//
// All three functions here were stubs returning nil, behind live routes:
// GET /api/v1/admin/billing/subscriptions, GET .../payments, and
// POST .../wallets/{id}/adjust. The two lists rendered permanently empty, and
// the adjustment answered 200 while moving no money at all - an operator could
// credit a supplier, see success, and nothing would happen.
//
// These read and write across every tenant, so they run under
// database.AsSystem, and the adjustment records an audit row in the same
// transaction as the ledger entry.

// AdminListSubscriptions returns subscriptions across all tenants.
func (r *Repository) AdminListSubscriptions(ctx context.Context, limit, offset int) ([]*billing.Subscription, error) {
	var list []*billing.Subscription
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const query = `
			SELECT id, public_id, user_id, organization_id, plan_id, status,
			       starts_at, expires_at, source_system, source_id, created_at, updated_at
			FROM billing.subscriptions
			ORDER BY created_at DESC
			LIMIT $1 OFFSET $2;
		`
		rows, err := tx.Query(txCtx, query, pageLimit(limit), pageOffset(offset))
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var s billing.Subscription
			var statusStr string
			if err := rows.Scan(
				&s.ID, &s.PublicID, &s.UserID, &s.OrganizationID, &s.PlanID, &statusStr,
				&s.StartsAt, &s.ExpiresAt, &s.SourceSystem, &s.SourceID,
				&s.CreatedAt, &s.UpdatedAt,
			); err != nil {
				return err
			}
			s.Status = billing.SubscriptionStatus(statusStr)
			list = append(list, &s)
		}
		return rows.Err()
	})
	return list, err
}

// AdminListPayments returns payments across all tenants.
func (r *Repository) AdminListPayments(ctx context.Context, limit, offset int) ([]*billing.Payment, error) {
	var list []*billing.Payment
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const query = `
			SELECT id, public_id, payment_integration_id, order_id, user_id, organization_id,
			       amount, method, status, transaction_id, reference_number, paid_at,
			       created_at, updated_at
			FROM billing.payments
			ORDER BY created_at DESC
			LIMIT $1 OFFSET $2;
		`
		rows, err := tx.Query(txCtx, query, pageLimit(limit), pageOffset(offset))
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var p billing.Payment
			if err := rows.Scan(
				&p.ID, &p.PublicID, &p.PaymentIntegrationID, &p.OrderID, &p.UserID,
				&p.OrganizationID, &p.Amount, &p.Method, &p.Status, &p.TransactionID,
				&p.ReferenceNumber, &p.PaidAt, &p.CreatedAt, &p.UpdatedAt,
			); err != nil {
				return err
			}
			list = append(list, &p)
		}
		return rows.Err()
	})
	return list, err
}

// AdminAdjustWallet posts a manual correction to a wallet ledger.
//
// The balance is not a column; it is the balance_after of the newest
// transaction, so an adjustment has to be appended to the ledger like any other
// movement rather than written over a total. The newest row is locked FOR
// UPDATE so two concurrent adjustments cannot both read the same starting
// balance and each overwrite the other's result.
//
// A negative amount deducts. The resulting balance may not go below zero: an
// operator correcting a mistake should not be able to leave an account owing
// money with no record of how it got there.
func (r *Repository) AdminAdjustWallet(
	ctx context.Context,
	walletID int64,
	amount money.Amount,
	reason string,
	actorID int64,
) error {
	if reason == "" {
		return apperr.Validation("wallet.reason_required",
			"A reason is required for a manual wallet adjustment.", nil)
	}
	if amount.Minor() == 0 {
		return apperr.Validation("wallet.zero_amount",
			"A wallet adjustment must be a non-zero amount.", nil)
	}

	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		var ownerUserID int64
		if err := tx.QueryRow(txCtx,
			`SELECT user_id FROM billing.wallets WHERE id = $1;`, walletID,
		).Scan(&ownerUserID); err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("wallet")
			}
			return fmt.Errorf("billing postgres: read wallet: %w", err)
		}

		var current money.Amount
		err := tx.QueryRow(txCtx, `
			SELECT balance_after FROM billing.wallet_transactions
			WHERE wallet_id = $1 ORDER BY id DESC LIMIT 1 FOR UPDATE;
		`, walletID).Scan(&current)
		if err != nil && !database.IsNotFound(err) {
			return fmt.Errorf("billing postgres: read wallet balance: %w", err)
		}

		next, addErr := current.Add(amount)
		if addErr != nil {
			return apperr.Internal(addErr)
		}
		if next.IsNegative() {
			return apperr.Validation("wallet.insufficient_funds",
				"This adjustment would take the wallet below zero.", nil)
		}

		if _, err := tx.Exec(txCtx, `
			INSERT INTO billing.wallet_transactions (
				wallet_id, type, amount, balance_after, reference_type, description
			) VALUES ($1, 'adjustment', $2, $3, 'admin_adjustment', $4);
		`, walletID, amount, next, reason); err != nil {
			return fmt.Errorf("billing postgres: record adjustment: %w", err)
		}

		return database.WriteAudit(txCtx, tx, database.AuditEntry{
			ActorUserID: actorID,
			Action:      "billing.wallet.adjusted",
			EntityType:  "billing.wallet",
			EntityID:    strconv.FormatInt(walletID, 10),
			Before:      map[string]string{"balance": current.String()},
			After: map[string]string{
				"balance": next.String(),
				"amount":  amount.String(),
				"reason":  reason,
			},
		})
	})
}

// pageLimit and pageOffset keep an unbounded admin query from reading the whole
// table when a caller omits paging.
func pageLimit(limit int) int {
	if limit <= 0 || limit > 200 {
		return 50
	}
	return limit
}

func pageOffset(offset int) int {
	if offset < 0 {
		return 0
	}
	return offset
}
