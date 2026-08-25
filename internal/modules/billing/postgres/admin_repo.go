package postgres

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/billing"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// EnsureAllOrgWallets guarantees that every registered organization has an active wallet account.
func (r *Repository) EnsureAllOrgWallets(ctx context.Context) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const queryOwners = `
			INSERT INTO billing.wallets (user_id, organization_id, currency)
			SELECT o.owner_id, o.id, 'EGP'
			FROM org.organizations o
			WHERE o.owner_id IS NOT NULL AND o.owner_id > 0
			ON CONFLICT (user_id, currency) DO UPDATE SET organization_id = EXCLUDED.organization_id;
		`
		if _, err := tx.Exec(txCtx, queryOwners); err != nil {
			return fmt.Errorf("seed org owner wallets: %w", err)
		}
		return nil
	})
}

// AdminListDetailedWallets returns all user/organization wallets enriched with user and organization details.
func (r *Repository) AdminListDetailedWallets(ctx context.Context, filter billing.WalletFilter) ([]*billing.AdminWalletView, int, error) {
	// Auto-seed missing wallets first
	_ = r.EnsureAllOrgWallets(ctx)

	var list []*billing.AdminWalletView
	var total int

	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		baseQuery := `
			FROM billing.wallets w
			JOIN identity.users u ON w.user_id = u.id
			LEFT JOIN org.organizations o ON (w.organization_id = o.id OR (w.organization_id IS NULL AND o.owner_id = u.id))
			WHERE 1=1
		`
		args := []any{}
		argIdx := 1

		if filter.Search != "" {
			searchPattern := "%" + strings.ToLower(filter.Search) + "%"
			baseQuery += fmt.Sprintf(` AND (
				LOWER(COALESCE(u.name->>'ar', '')) LIKE $%d OR
				LOWER(COALESCE(u.name->>'en', '')) LIKE $%d OR
				LOWER(u.email) LIKE $%d OR
				LOWER(COALESCE(u.phone, '')) LIKE $%d OR
				LOWER(COALESCE(o.legal_name, '')) LIKE $%d OR
				LOWER(COALESCE(o.trade_name->>'ar', '')) LIKE $%d OR
				LOWER(COALESCE(o.trade_name->>'en', '')) LIKE $%d
			)`, argIdx, argIdx, argIdx, argIdx, argIdx, argIdx, argIdx)
			args = append(args, searchPattern)
			argIdx++
		}

		if filter.Type != "" && filter.Type != "all" {
			baseQuery += fmt.Sprintf(` AND o.type = $%d`, argIdx)
			args = append(args, filter.Type)
			argIdx++
		}

		countQuery := `SELECT COUNT(*) ` + baseQuery
		if err := tx.QueryRow(txCtx, countQuery, args...).Scan(&total); err != nil {
			return err
		}

		selectQuery := `
			SELECT 
				w.id, 
				w.public_id::text, 
				w.user_id, 
				COALESCE(u.name->>'ar', u.name->>'en', u.email, 'مستخدم'), 
				u.email, 
				COALESCE(u.phone, ''),
				w.organization_id,
				COALESCE(o.legal_name, o.trade_name->>'ar', o.trade_name->>'en', ''),
				COALESCE(o.type, ''),
				w.currency,
				w.created_at,
				(SELECT COUNT(*) FROM billing.wallet_transactions wt WHERE wt.wallet_id = w.id),
				COALESCE((SELECT balance_after FROM billing.wallet_transactions wt WHERE wt.wallet_id = w.id ORDER BY id DESC LIMIT 1), 0.00) AS balance
		` + baseQuery + fmt.Sprintf(` ORDER BY w.created_at DESC LIMIT $%d OFFSET $%d;`, argIdx, argIdx+1)

		args = append(args, pageLimit(filter.Limit), pageOffset(filter.Offset))

		rows, err := tx.Query(txCtx, selectQuery, args...)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var wv billing.AdminWalletView
			if err := rows.Scan(
				&wv.ID, &wv.PublicID, &wv.UserID, &wv.UserName, &wv.UserEmail, &wv.UserPhone,
				&wv.OrganizationID, &wv.OrganizationName, &wv.OrganizationType, &wv.Currency,
				&wv.CreatedAt, &wv.TransactionsCount, &wv.Balance,
			); err != nil {
				return err
			}
			list = append(list, &wv)
		}
		return rows.Err()
	})
	return list, total, err
}

// AdminListDetailedTransactions returns wallet transactions enriched with user and organization metadata.
func (r *Repository) AdminListDetailedTransactions(ctx context.Context, filter billing.TransactionFilter) ([]*billing.AdminWalletTransactionView, int, error) {
	var list []*billing.AdminWalletTransactionView
	var total int

	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		baseQuery := `
			FROM billing.wallet_transactions wt
			JOIN billing.wallets w ON wt.wallet_id = w.id
			JOIN identity.users u ON w.user_id = u.id
			LEFT JOIN org.organizations o ON w.organization_id = o.id
			WHERE 1=1
		`
		args := []any{}
		argIdx := 1

		if filter.WalletID > 0 {
			baseQuery += fmt.Sprintf(` AND wt.wallet_id = $%d`, argIdx)
			args = append(args, filter.WalletID)
			argIdx++
		}

		if filter.Type != "" && filter.Type != "all" {
			baseQuery += fmt.Sprintf(` AND wt.type = $%d`, argIdx)
			args = append(args, filter.Type)
			argIdx++
		}

		if filter.Search != "" {
			searchPattern := "%" + strings.ToLower(filter.Search) + "%"
			baseQuery += fmt.Sprintf(` AND (
				LOWER(COALESCE(wt.description, '')) LIKE $%d OR
				LOWER(COALESCE(u.name->>'ar', '')) LIKE $%d OR
				LOWER(u.email) LIKE $%d OR
				LOWER(COALESCE(o.legal_name, '')) LIKE $%d
			)`, argIdx, argIdx, argIdx, argIdx)
			args = append(args, searchPattern)
			argIdx++
		}

		countQuery := `SELECT COUNT(*) ` + baseQuery
		if err := tx.QueryRow(txCtx, countQuery, args...).Scan(&total); err != nil {
			return err
		}

		selectQuery := `
			SELECT 
				wt.id,
				wt.wallet_id,
				wt.type,
				wt.amount,
				wt.balance_after,
				COALESCE(wt.reference_type, ''),
				wt.reference_id,
				COALESCE(wt.description, ''),
				wt.created_at,
				w.user_id,
				COALESCE(u.name->>'ar', u.name->>'en', u.email, 'مستخدم'),
				u.email,
				COALESCE(o.legal_name, o.trade_name->>'ar', o.trade_name->>'en', ''),
				COALESCE(o.type, '')
		` + baseQuery + fmt.Sprintf(` ORDER BY wt.id DESC LIMIT $%d OFFSET $%d;`, argIdx, argIdx+1)

		args = append(args, pageLimit(filter.Limit), pageOffset(filter.Offset))

		rows, err := tx.Query(txCtx, selectQuery, args...)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var tv billing.AdminWalletTransactionView
			var typeStr string
			if err := rows.Scan(
				&tv.ID, &tv.WalletID, &typeStr, &tv.Amount, &tv.BalanceAfter,
				&tv.ReferenceType, &tv.ReferenceID, &tv.Description, &tv.CreatedAt,
				&tv.UserID, &tv.UserName, &tv.UserEmail, &tv.OrganizationName, &tv.OrganizationType,
			); err != nil {
				return err
			}
			tv.Type = billing.TransactionType(typeStr)
			list = append(list, &tv)
		}
		return rows.Err()
	})
	return list, total, err
}

// AdminListDetailedInvoices returns invoices enriched with party legal names and order details.
func (r *Repository) AdminListDetailedInvoices(ctx context.Context, filter billing.InvoiceFilter) ([]*billing.AdminInvoiceView, int, error) {
	var list []*billing.AdminInvoiceView
	var total int

	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		baseQuery := `
			FROM billing.invoices inv
			LEFT JOIN org.organizations v ON inv.organization_id = v.id
			LEFT JOIN org.organizations c ON inv.customer_org_id = c.id
			LEFT JOIN commerce.orders o ON inv.order_id = o.id
			WHERE 1=1
		`
		args := []any{}
		argIdx := 1

		if filter.Status != "" && filter.Status != "all" {
			baseQuery += fmt.Sprintf(` AND inv.status = $%d`, argIdx)
			args = append(args, filter.Status)
			argIdx++
		}

		if filter.Search != "" {
			searchPattern := "%" + strings.ToLower(filter.Search) + "%"
			baseQuery += fmt.Sprintf(` AND (
				LOWER(inv.invoice_number) LIKE $%d OR
				LOWER(COALESCE(v.legal_name, '')) LIKE $%d OR
				LOWER(COALESCE(c.legal_name, '')) LIKE $%d OR
				LOWER(COALESCE(o.order_number, '')) LIKE $%d
			)`, argIdx, argIdx, argIdx, argIdx)
			args = append(args, searchPattern)
			argIdx++
		}

		countQuery := `SELECT COUNT(*) ` + baseQuery
		if err := tx.QueryRow(txCtx, countQuery, args...).Scan(&total); err != nil {
			return err
		}

		selectQuery := `
			SELECT 
				inv.id, inv.public_id::text, inv.organization_id,
				COALESCE(v.legal_name, v.trade_name->>'ar', 'المورد'),
				inv.customer_org_id,
				COALESCE(c.legal_name, c.trade_name->>'ar', 'الصيدلية'),
				inv.order_id,
				COALESCE(o.order_number, ''),
				inv.invoice_number,
				inv.issue_date,
				inv.due_date,
				inv.subtotal,
				inv.tax_amount,
				inv.discount_amount,
				inv.total_amount,
				inv.status,
				inv.payment_method,
				COALESCE(inv.notes, ''),
				inv.created_at
		` + baseQuery + fmt.Sprintf(` ORDER BY inv.created_at DESC LIMIT $%d OFFSET $%d;`, argIdx, argIdx+1)

		args = append(args, pageLimit(filter.Limit), pageOffset(filter.Offset))

		rows, err := tx.Query(txCtx, selectQuery, args...)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var iv billing.AdminInvoiceView
			var statusStr string
			var issueDate, dueDate time.Time
			if err := rows.Scan(
				&iv.ID, &iv.PublicID, &iv.OrganizationID, &iv.VendorName,
				&iv.CustomerOrgID, &iv.CustomerName, &iv.OrderID, &iv.OrderNumber,
				&iv.InvoiceNumber, &issueDate, &dueDate, &iv.Subtotal,
				&iv.TaxAmount, &iv.DiscountAmount, &iv.TotalAmount, &statusStr,
				&iv.PaymentMethod, &iv.Notes, &iv.CreatedAt,
			); err != nil {
				return err
			}
			iv.IssueDate = issueDate
			iv.DueDate = dueDate
			iv.Status = billing.InvoiceStatus(statusStr)
			list = append(list, &iv)
		}
		return rows.Err()
	})
	return list, total, err
}

// AdminListDetailedPayments returns payments enriched with order numbers and user/organization metadata.
func (r *Repository) AdminListDetailedPayments(ctx context.Context, filter billing.PaymentFilter) ([]*billing.AdminPaymentView, int, error) {
	var list []*billing.AdminPaymentView
	var total int

	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		baseQuery := `
			FROM billing.payments p
			LEFT JOIN identity.users u ON p.user_id = u.id
			LEFT JOIN org.organizations org ON p.organization_id = org.id
			LEFT JOIN commerce.orders o ON p.order_id = o.id
			WHERE 1=1
		`
		args := []any{}
		argIdx := 1

		if filter.Status != "" && filter.Status != "all" {
			baseQuery += fmt.Sprintf(` AND p.status = $%d`, argIdx)
			args = append(args, filter.Status)
			argIdx++
		}

		if filter.Method != "" && filter.Method != "all" {
			baseQuery += fmt.Sprintf(` AND p.method = $%d`, argIdx)
			args = append(args, filter.Method)
			argIdx++
		}

		if filter.Search != "" {
			searchPattern := "%" + strings.ToLower(filter.Search) + "%"
			baseQuery += fmt.Sprintf(` AND (
				LOWER(COALESCE(p.transaction_id, '')) LIKE $%d OR
				LOWER(COALESCE(p.reference_number, '')) LIKE $%d OR
				LOWER(COALESCE(u.name->>'ar', '')) LIKE $%d OR
				LOWER(u.email) LIKE $%d OR
				LOWER(COALESCE(org.legal_name, '')) LIKE $%d OR
				LOWER(COALESCE(o.order_number, '')) LIKE $%d
			)`, argIdx, argIdx, argIdx, argIdx, argIdx, argIdx)
			args = append(args, searchPattern)
			argIdx++
		}

		countQuery := `SELECT COUNT(*) ` + baseQuery
		if err := tx.QueryRow(txCtx, countQuery, args...).Scan(&total); err != nil {
			return err
		}

		selectQuery := `
			SELECT 
				p.id, p.public_id::text, p.payment_integration_id, p.order_id,
				COALESCE(o.order_number, ''),
				p.user_id,
				COALESCE(u.name->>'ar', u.name->>'en', u.email, 'مستخدم'),
				p.organization_id,
				COALESCE(org.legal_name, org.trade_name->>'ar', ''),
				p.amount, p.method, p.status, p.transaction_id, p.reference_number, p.paid_at, p.created_at
		` + baseQuery + fmt.Sprintf(` ORDER BY p.created_at DESC LIMIT $%d OFFSET $%d;`, argIdx, argIdx+1)

		args = append(args, pageLimit(filter.Limit), pageOffset(filter.Offset))

		rows, err := tx.Query(txCtx, selectQuery, args...)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var pv billing.AdminPaymentView
			if err := rows.Scan(
				&pv.ID, &pv.PublicID, &pv.PaymentIntegrationID, &pv.OrderID,
				&pv.OrderNumber, &pv.UserID, &pv.UserName, &pv.OrganizationID,
				&pv.OrganizationName, &pv.Amount, &pv.Method, &pv.Status,
				&pv.TransactionID, &pv.ReferenceNumber, &pv.PaidAt, &pv.CreatedAt,
			); err != nil {
				return err
			}
			list = append(list, &pv)
		}
		return rows.Err()
	})
	return list, total, err
}

// AdminPerformWalletAdjustment posts a manual adjustment (deposit, withdrawal, or adjustment) to a wallet ledger.
func (r *Repository) AdminPerformWalletAdjustment(
	ctx context.Context,
	walletID int64,
	amount money.Amount,
	txType billing.TransactionType,
	reason string,
	actorID int64,
) error {
	if reason == "" {
		return apperr.Validation("wallet.reason_required", "A reason is required for a manual wallet adjustment.", nil)
	}
	if amount.Minor() == 0 {
		return apperr.Validation("wallet.zero_amount", "A wallet adjustment must be a non-zero amount.", nil)
	}
	if txType == "" {
		txType = billing.TxAdjustment
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
			) VALUES ($1, $2, $3, $4, 'admin_adjustment', $5);
		`, walletID, string(txType), amount, next, reason); err != nil {
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
				"type":    string(txType),
				"reason":  reason,
			},
		})
	})
}

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
func (r *Repository) AdminAdjustWallet(
	ctx context.Context,
	walletID int64,
	amount money.Amount,
	reason string,
	actorID int64,
) error {
	return r.AdminPerformWalletAdjustment(ctx, walletID, amount, billing.TxAdjustment, reason, actorID)
}

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
