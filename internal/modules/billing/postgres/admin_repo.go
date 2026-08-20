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

// AdminListInvoices returns invoices across all organizations.
func (r *Repository) AdminListInvoices(ctx context.Context, limit, offset int) ([]*billing.Invoice, error) {
	var list []*billing.Invoice
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const query = `
			SELECT id, public_id, organization_id, customer_org_id, order_id, invoice_number,
			       issue_date, due_date, subtotal, tax_amount, discount_amount, total_amount,
			       status, payment_method, notes, created_at, updated_at
			FROM billing.invoices
			ORDER BY created_at DESC
			LIMIT $1 OFFSET $2;
		`
		rows, err := tx.Query(txCtx, query, pageLimit(limit), pageOffset(offset))
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var inv billing.Invoice
			var statusStr string
			var notes *string
			if err := rows.Scan(
				&inv.ID, &inv.PublicID, &inv.OrganizationID, &inv.CustomerOrgID, &inv.OrderID, &inv.InvoiceNumber,
				&inv.IssueDate, &inv.DueDate, &inv.Subtotal, &inv.TaxAmount, &inv.DiscountAmount, &inv.TotalAmount,
				&statusStr, &inv.PaymentMethod, &notes, &inv.CreatedAt, &inv.UpdatedAt,
			); err != nil {
				return err
			}
			inv.Status = billing.InvoiceStatus(statusStr)
			if notes != nil {
				inv.Notes = *notes
			}
			list = append(list, &inv)
		}
		return rows.Err()
	})
	return list, err
}

// AdminListWallets returns all user/organization wallets with current computed balance.
func (r *Repository) AdminListWallets(ctx context.Context, limit, offset int) ([]*billing.Wallet, error) {
	var list []*billing.Wallet
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const query = `
			SELECT id, public_id, user_id, organization_id, currency, created_at, updated_at
			FROM billing.wallets
			ORDER BY created_at DESC
			LIMIT $1 OFFSET $2;
		`
		rows, err := tx.Query(txCtx, query, pageLimit(limit), pageOffset(offset))
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var w billing.Wallet
			if err := rows.Scan(
				&w.ID, &w.PublicID, &w.UserID, &w.OrganizationID, &w.Currency, &w.CreatedAt, &w.UpdatedAt,
			); err != nil {
				return err
			}
			list = append(list, &w)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}

	for _, w := range list {
		_ = r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
			return tx.QueryRow(txCtx, `SELECT balance_after FROM billing.wallet_transactions WHERE wallet_id = $1 ORDER BY id DESC LIMIT 1;`, w.ID).Scan(&w.Balance)
		})
	}
	return list, nil
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
