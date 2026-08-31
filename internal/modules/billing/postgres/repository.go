package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/billing"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// Repository implements billing.Repository using PostgreSQL.
type Repository struct {
	db *database.DB
}

// NewRepository creates a new billing PostgreSQL repository.
func NewRepository(db *database.DB) *Repository {
	return &Repository{db: db}
}

// GetOrCreateWallet retrieves or initializes a user wallet.
func (r *Repository) GetOrCreateWallet(ctx context.Context, userID int64, currency string) (*billing.Wallet, error) {
	if currency == "" {
		currency = "EGP"
	}
	var w billing.Wallet
	err := r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			INSERT INTO billing.wallets (user_id, currency)
			VALUES ($1, $2)
			ON CONFLICT (user_id, currency) DO UPDATE SET updated_at = now()
			RETURNING id, public_id, user_id, organization_id, currency, created_at, updated_at;
		`
		if err := tx.QueryRow(txCtx, query, userID, currency).Scan(
			&w.ID, &w.PublicID, &w.UserID, &w.OrganizationID, &w.Currency, &w.CreatedAt, &w.UpdatedAt,
		); err != nil {
			return err
		}

		// Compute balance from latest transaction
		queryBal := `SELECT balance_after FROM billing.wallet_transactions WHERE wallet_id = $1 ORDER BY id DESC LIMIT 1;`
		err := tx.QueryRow(txCtx, queryBal, w.ID).Scan(&w.Balance)
		if err != nil && database.IsNotFound(err) {
			w.Balance = money.Zero
			return nil
		}
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("billing postgres: get or create wallet: %w", err)
	}
	return &w, nil
}

// GetWallet retrieves a wallet and computes its current balance.
func (r *Repository) GetWallet(ctx context.Context, id int64) (*billing.Wallet, error) {
	var w billing.Wallet
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `SELECT id, public_id, user_id, organization_id, currency, created_at, updated_at FROM billing.wallets WHERE id = $1;`
		if err := tx.QueryRow(txCtx, query, id).Scan(
			&w.ID, &w.PublicID, &w.UserID, &w.OrganizationID, &w.Currency, &w.CreatedAt, &w.UpdatedAt,
		); err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("wallet")
			}
			return err
		}

		queryBal := `SELECT balance_after FROM billing.wallet_transactions WHERE wallet_id = $1 ORDER BY id DESC LIMIT 1;`
		err := tx.QueryRow(txCtx, queryBal, w.ID).Scan(&w.Balance)
		if err != nil && database.IsNotFound(err) {
			w.Balance = money.Zero
			return nil
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	return &w, nil
}

// RecordTransaction writes an append-only ledger row and updates the balance projection.
func (r *Repository) RecordTransaction(
	ctx context.Context,
	walletID int64,
	txType billing.TransactionType,
	delta money.Amount,
	refType string,
	refID *int64,
	desc string,
) (*billing.WalletTransaction, error) {
	var txRecord billing.WalletTransaction
	err := r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		var currentBalance money.Amount
		queryLatest := `SELECT balance_after FROM billing.wallet_transactions WHERE wallet_id = $1 ORDER BY id DESC LIMIT 1 FOR UPDATE;`
		err := tx.QueryRow(txCtx, queryLatest, walletID).Scan(&currentBalance)
		if err != nil && !database.IsNotFound(err) {
			return err
		}

		newBalance, addErr := currentBalance.Add(delta)
		if addErr != nil {
			return apperr.Internal(addErr)
		}
		if newBalance.IsNegative() {
			return apperr.Validation("wallet.insufficient_funds", "Insufficient wallet funds for this operation.", nil)
		}

		queryInsert := `
			INSERT INTO billing.wallet_transactions (
				wallet_id, type, amount, balance_after, reference_type, reference_id, description
			) VALUES ($1, $2, $3, $4, $5, $6, $7)
			RETURNING id, wallet_id, type, amount, balance_after, reference_type, reference_id, description, created_at;
		`
		var typeStr string
		err = tx.QueryRow(txCtx, queryInsert,
			walletID, string(txType), delta, newBalance, refType, refID, desc,
		).Scan(
			&txRecord.ID, &txRecord.WalletID, &typeStr, &txRecord.Amount, &txRecord.BalanceAfter,
			&txRecord.ReferenceType, &txRecord.ReferenceID, &txRecord.Description, &txRecord.CreatedAt,
		)
		if err != nil {
			return err
		}
		txRecord.Type = billing.TransactionType(typeStr)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &txRecord, nil
}

// ListTransactions retrieves paginated transactions for a wallet.
func (r *Repository) ListTransactions(ctx context.Context, walletID int64, limit, offset int) ([]*billing.WalletTransaction, error) {
	var list []*billing.WalletTransaction
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, wallet_id, type, amount, balance_after, reference_type, reference_id, description, created_at
			FROM billing.wallet_transactions
			WHERE wallet_id = $1
			ORDER BY id DESC
			LIMIT $2 OFFSET $3;
		`
		if limit <= 0 || limit > 100 {
			limit = 20
		}
		rows, err := tx.Query(txCtx, query, walletID, limit, offset)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var t billing.WalletTransaction
			var typeStr string
			if err := rows.Scan(
				&t.ID, &t.WalletID, &typeStr, &t.Amount, &t.BalanceAfter,
				&t.ReferenceType, &t.ReferenceID, &t.Description, &t.CreatedAt,
			); err != nil {
				return err
			}
			t.Type = billing.TransactionType(typeStr)
			list = append(list, &t)
		}
		return rows.Err()
	})
	return list, err
}

// CreatePayment inserts a payment record.
func (r *Repository) CreatePayment(ctx context.Context, p *billing.Payment) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			INSERT INTO billing.payments (
				payment_integration_id, order_id, user_id, organization_id,
				amount, method, status, transaction_id, reference_number, paid_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
			RETURNING id, public_id, created_at, updated_at;
		`
		return tx.QueryRow(txCtx, query,
			p.PaymentIntegrationID, p.OrderID, p.UserID, p.OrganizationID,
			p.Amount, p.Method, p.Status, p.TransactionID, p.ReferenceNumber, p.PaidAt,
		).Scan(&p.ID, &p.PublicID, &p.CreatedAt, &p.UpdatedAt)
	})
}

// GetPaymentByID retrieves a payment by ID.
func (r *Repository) GetPaymentByID(ctx context.Context, id int64) (*billing.Payment, error) {
	var p billing.Payment
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, public_id, payment_integration_id, order_id, user_id, organization_id,
			       amount, method, status, transaction_id, reference_number, paid_at, created_at, updated_at
			FROM billing.payments
			WHERE id = $1;
		`
		return tx.QueryRow(txCtx, query, id).Scan(
			&p.ID, &p.PublicID, &p.PaymentIntegrationID, &p.OrderID, &p.UserID, &p.OrganizationID,
			&p.Amount, &p.Method, &p.Status, &p.TransactionID, &p.ReferenceNumber, &p.PaidAt,
			&p.CreatedAt, &p.UpdatedAt,
		)
	})
	if err != nil {
		if database.IsNotFound(err) {
			return nil, apperr.NotFound("payment")
		}
		return nil, err
	}
	return &p, nil
}

// ListPaymentsByOrg retrieves payments associated with an organization.
func (r *Repository) ListPaymentsByOrg(ctx context.Context, orgID int64, limit, offset int) ([]*billing.Payment, error) {
	var list []*billing.Payment
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const query = `
			SELECT id, public_id, payment_integration_id, order_id, user_id, organization_id,
			       amount, method, status, transaction_id, reference_number, paid_at,
			       created_at, updated_at
			FROM billing.payments
			WHERE organization_id = $1
			ORDER BY created_at DESC
			LIMIT $2 OFFSET $3;
		`
		if limit <= 0 || limit > 100 {
			limit = 20
		}
		rows, err := tx.Query(txCtx, query, orgID, limit, offset)
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

// ListPlans lists all active subscription plans.
func (r *Repository) ListPlans(ctx context.Context) ([]*billing.Plan, error) {
	var plans []*billing.Plan
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, public_id, slug, name, description, price_month, price_year,
			       duration_days, max_users, max_login_sessions, max_devices, ai_plan_id, is_default, is_active, created_at, updated_at
			FROM billing.plans
			WHERE is_active = true
			ORDER BY id ASC;
		`
		rows, err := tx.Query(txCtx, query)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var p billing.Plan
			if err := rows.Scan(
				&p.ID, &p.PublicID, &p.Slug, &p.Name, &p.Description,
				&p.PriceMonth, &p.PriceYear, &p.DurationDays, &p.MaxUsers,
				&p.MaxLoginSessions, &p.MaxDevices, &p.AIPlanID, &p.IsDefault,
				&p.IsActive, &p.CreatedAt, &p.UpdatedAt,
			); err != nil {
				return err
			}
			plans = append(plans, &p)
		}
		if rows.Err() != nil {
			return rows.Err()
		}

		for _, p := range plans {
			p.Features = loadPlanFeatures(txCtx, tx, p.ID)
		}
		return nil
	})
	return plans, err
}

// GetPlanByID retrieves a plan by ID.
func (r *Repository) GetPlanByID(ctx context.Context, id int64) (*billing.Plan, error) {
	var p billing.Plan
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, public_id, slug, name, description, price_month, price_year,
			       duration_days, max_users, max_login_sessions, max_devices, ai_plan_id, is_default, is_active, created_at, updated_at
			FROM billing.plans
			WHERE id = $1;
		`
		if err := tx.QueryRow(txCtx, query, id).Scan(
			&p.ID, &p.PublicID, &p.Slug, &p.Name, &p.Description,
			&p.PriceMonth, &p.PriceYear, &p.DurationDays, &p.MaxUsers,
			&p.MaxLoginSessions, &p.MaxDevices, &p.AIPlanID, &p.IsDefault,
			&p.IsActive, &p.CreatedAt, &p.UpdatedAt,
		); err != nil {
			return err
		}
		p.Features = loadPlanFeatures(txCtx, tx, p.ID)
		return nil
	})
	if err != nil {
		if database.IsNotFound(err) {
			return nil, apperr.NotFound("plan")
		}
		return nil, err
	}
	return &p, nil
}
