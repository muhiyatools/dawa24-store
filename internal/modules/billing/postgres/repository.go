package postgres

import (
	"context"
	"fmt"
	"time"

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
		for rows.Next() {
			var p billing.Plan
			if err := rows.Scan(
				&p.ID, &p.PublicID, &p.Slug, &p.Name, &p.Description,
				&p.PriceMonth, &p.PriceYear, &p.DurationDays, &p.MaxUsers,
				&p.MaxLoginSessions, &p.MaxDevices, &p.AIPlanID, &p.IsDefault,
				&p.IsActive, &p.CreatedAt, &p.UpdatedAt,
			); err != nil {
				rows.Close()
				return err
			}
			p.Features = loadPlanFeatures(txCtx, tx, p.ID)
			plans = append(plans, &p)
		}
		rows.Close()

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

// GetPlanBySlug retrieves a plan by slug.
func (r *Repository) GetPlanBySlug(ctx context.Context, slug string) (*billing.Plan, error) {
	var p billing.Plan
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, public_id, slug, name, description, price_month, price_year,
			       duration_days, max_users, max_login_sessions, max_devices, ai_plan_id, is_default, is_active, created_at, updated_at
			FROM billing.plans
			WHERE slug = $1 AND is_active = true;
		`
		if err := tx.QueryRow(txCtx, query, slug).Scan(
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

// CreateSubscription activates a subscription for a user/org.
func (r *Repository) CreateSubscription(ctx context.Context, sub *billing.Subscription) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		if sub.BillingCycle == "" {
			sub.BillingCycle = "monthly"
		}
		query := `
			INSERT INTO billing.subscriptions (
				user_id, organization_id, plan_id, status, billing_cycle, auto_renew,
				starts_at, expires_at, last_renewed_at, renewal_attempts, source_system, source_id
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
			RETURNING id, public_id, created_at, updated_at;
		`
		return tx.QueryRow(txCtx, query,
			sub.UserID, sub.OrganizationID, sub.PlanID, string(sub.Status),
			sub.BillingCycle, sub.AutoRenew, sub.StartsAt, sub.ExpiresAt,
			sub.LastRenewedAt, sub.RenewalAttempts, sub.SourceSystem, sub.SourceID,
		).Scan(&sub.ID, &sub.PublicID, &sub.CreatedAt, &sub.UpdatedAt)
	})
}

// GetActiveSubscription retrieves current active subscription for a user.
func (r *Repository) GetActiveSubscription(ctx context.Context, userID int64) (*billing.Subscription, error) {
	var sub billing.Subscription
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, public_id, user_id, organization_id, plan_id, status,
			       COALESCE(billing_cycle, 'monthly'), COALESCE(auto_renew, false),
			       starts_at, expires_at, last_renewed_at, COALESCE(renewal_attempts, 0),
			       source_system, source_id, created_at, updated_at
			FROM billing.subscriptions
			WHERE user_id = $1 AND status = 'active' AND expires_at > now()
			ORDER BY expires_at DESC
			LIMIT 1;
		`
		var statusStr string
		err := tx.QueryRow(txCtx, query, userID).Scan(
			&sub.ID, &sub.PublicID, &sub.UserID, &sub.OrganizationID, &sub.PlanID,
			&statusStr, &sub.BillingCycle, &sub.AutoRenew,
			&sub.StartsAt, &sub.ExpiresAt, &sub.LastRenewedAt, &sub.RenewalAttempts,
			&sub.SourceSystem, &sub.SourceID, &sub.CreatedAt, &sub.UpdatedAt,
		)
		if err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("subscription")
			}
			return err
		}
		sub.Status = billing.SubscriptionStatus(statusStr)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &sub, nil
}

// GetActiveSubscriptionByOrg retrieves current active subscription for an organization.
func (r *Repository) GetActiveSubscriptionByOrg(ctx context.Context, orgID int64) (*billing.Subscription, error) {
	var sub billing.Subscription
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, public_id, user_id, organization_id, plan_id, status,
			       COALESCE(billing_cycle, 'monthly'), COALESCE(auto_renew, false),
			       starts_at, expires_at, last_renewed_at, COALESCE(renewal_attempts, 0),
			       source_system, source_id, created_at, updated_at
			FROM billing.subscriptions
			WHERE organization_id = $1 AND status = 'active' AND expires_at > now()
			ORDER BY expires_at DESC
			LIMIT 1;
		`
		var statusStr string
		err := tx.QueryRow(txCtx, query, orgID).Scan(
			&sub.ID, &sub.PublicID, &sub.UserID, &sub.OrganizationID, &sub.PlanID,
			&statusStr, &sub.BillingCycle, &sub.AutoRenew,
			&sub.StartsAt, &sub.ExpiresAt, &sub.LastRenewedAt, &sub.RenewalAttempts,
			&sub.SourceSystem, &sub.SourceID, &sub.CreatedAt, &sub.UpdatedAt,
		)
		if err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("subscription")
			}
			return err
		}
		sub.Status = billing.SubscriptionStatus(statusStr)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &sub, nil
}

// ListDueSubscriptionsForRenewal returns subscriptions where auto_renew is enabled and expires_at is due.
func (r *Repository) ListDueSubscriptionsForRenewal(ctx context.Context, now time.Time) ([]*billing.Subscription, error) {
	var list []*billing.Subscription
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, public_id, user_id, organization_id, plan_id, status,
			       COALESCE(billing_cycle, 'monthly'), COALESCE(auto_renew, false),
			       starts_at, expires_at, last_renewed_at, COALESCE(renewal_attempts, 0),
			       source_system, source_id, created_at, updated_at
			FROM billing.subscriptions
			WHERE auto_renew = true AND status = 'active' AND expires_at <= $1
			ORDER BY expires_at ASC
			LIMIT 100;
		`
		rows, err := tx.Query(txCtx, query, now)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var sub billing.Subscription
			var statusStr string
			if err := rows.Scan(
				&sub.ID, &sub.PublicID, &sub.UserID, &sub.OrganizationID, &sub.PlanID,
				&statusStr, &sub.BillingCycle, &sub.AutoRenew,
				&sub.StartsAt, &sub.ExpiresAt, &sub.LastRenewedAt, &sub.RenewalAttempts,
				&sub.SourceSystem, &sub.SourceID, &sub.CreatedAt, &sub.UpdatedAt,
			); err != nil {
				return err
			}
			sub.Status = billing.SubscriptionStatus(statusStr)
			list = append(list, &sub)
		}
		return rows.Err()
	})
	return list, err
}

// UpdateSubscriptionStatus updates the status and retry counter of a subscription.
func (r *Repository) UpdateSubscriptionStatus(ctx context.Context, id int64, status billing.SubscriptionStatus, renewalAttempts int) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(txCtx, `
			UPDATE billing.subscriptions
			SET status = $1, renewal_attempts = $2, updated_at = now()
			WHERE id = $3;
		`, string(status), renewalAttempts, id)
		return err
	})
}

// RenewSubscription performs atomic wallet deduction and advances the subscription expiration date.
func (r *Repository) RenewSubscription(ctx context.Context, subID int64, walletID int64, cost money.Amount, newExpiresAt time.Time, details string) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		// 1. Lock subscription row
		var currentStatus string
		var planID int64
		var orgID *int64
		var userID int64
		err := tx.QueryRow(txCtx, `
			SELECT status, plan_id, organization_id, user_id
			FROM billing.subscriptions
			WHERE id = $1
			FOR UPDATE;
		`, subID).Scan(&currentStatus, &planID, &orgID, &userID)
		if err != nil {
			return err
		}

		// 2. If cost > 0, deduct from wallet with ledger record
		if !cost.IsZero() && !cost.IsNegative() {
			var balance money.Amount
			err = tx.QueryRow(txCtx, `
				SELECT COALESCE(SUM(amount), 0)
				FROM billing.wallet_transactions
				WHERE wallet_id = $1;
			`, walletID).Scan(&balance)
			if err != nil {
				return err
			}

			if balance.Minor() < cost.Minor() {
				return apperr.Conflict("wallet.insufficient_funds", "رصيد المحفظة غير كافٍ للتجديد التلقائي.")
			}

			newBalance, _ := balance.Sub(cost)
			negCost := money.FromMinor(-cost.Minor())

			_, err = tx.Exec(txCtx, `
				INSERT INTO billing.wallet_transactions (
					wallet_id, type, amount, balance_after, reference_type, reference_id, description
				) VALUES ($1, $2, $3, $4, $5, $6, $7);
			`, walletID, string(billing.TxPurchase), negCost, newBalance, "subscription_renewal", subID, "تجديد تلقائي للاشتراك: "+details)
			if err != nil {
				return err
			}
		}

		// 3. Update subscription expiration and renewal metadata
		_, err = tx.Exec(txCtx, `
			UPDATE billing.subscriptions
			SET expires_at = $1, last_renewed_at = now(), renewal_attempts = 0, status = 'active', updated_at = now()
			WHERE id = $2;
		`, newExpiresAt, subID)
		if err != nil {
			return err
		}

		// 4. Record history entry
		var histOrgID int64
		if orgID != nil {
			histOrgID = *orgID
		}
		_, _ = tx.Exec(txCtx, `
			INSERT INTO billing.subscription_histories (
				subscription_id, organization_id, user_id, plan_id, action, amount_minor, currency, details
			) VALUES ($1, $2, $3, $4, 'renewed', $5, 'EGP', $6);
		`, subID, histOrgID, userID, planID, cost.Minor(), details)

		return nil
	})
}

// CheckEntitlement resolves whether a user has access to a specific feature via plan.
func (r *Repository) CheckEntitlement(ctx context.Context, userID int64, featureKey string) (bool, string, error) {
	var val string
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT pf.value
			FROM billing.subscriptions s
			JOIN billing.plan_features pf ON pf.plan_id = s.plan_id
			WHERE s.user_id = $1 AND s.status = 'active' AND s.expires_at > now() AND pf.feature_key = $2
			ORDER BY s.expires_at DESC
			LIMIT 1;
		`
		return tx.QueryRow(txCtx, query, userID, featureKey).Scan(&val)
	})
	if err != nil {
		if database.IsNotFound(err) {
			return false, "", nil
		}
		return false, "", err
	}
	return true, val, nil
}

// CheckOrgEntitlement resolves whether an organization or user has access to a specific feature key via their active subscription or default plan.
func (r *Repository) CheckOrgEntitlement(ctx context.Context, orgID, userID int64, featureKey string) (bool, error) {
	var val string
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		// 1. Check active subscription for organization or user
		querySub := `
			SELECT pf.value
			FROM billing.subscriptions s
			JOIN billing.plan_features pf ON pf.plan_id = s.plan_id
			WHERE ((s.organization_id IS NOT NULL AND s.organization_id = $1 AND $1 > 0)
			   OR (s.user_id = $2 AND $2 > 0))
			  AND s.status = 'active'
			  AND s.expires_at > now()
			  AND pf.feature_key = $3
			ORDER BY s.expires_at DESC
			LIMIT 1;
		`
		err := tx.QueryRow(txCtx, querySub, orgID, userID, featureKey).Scan(&val)
		if err == nil {
			return nil
		}
		if !database.IsNotFound(err) {
			return err
		}

		// 2. Fallback to default active plan features
		queryDefault := `
			SELECT pf.value
			FROM billing.plans p
			JOIN billing.plan_features pf ON pf.plan_id = p.id
			WHERE p.is_default = true AND p.is_active = true AND pf.feature_key = $1
			LIMIT 1;
		`
		return tx.QueryRow(txCtx, queryDefault, featureKey).Scan(&val)
	})
	if err != nil {
		if database.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return val == "true" || val == "1" || val == "enabled", nil
}

// CreateDepositRequest inserts a new deposit request with pending status.
func (r *Repository) CreateDepositRequest(ctx context.Context, dep *billing.WalletDeposit) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			INSERT INTO billing.wallet_deposits (
				wallet_id, user_id, organization_id, amount, currency, payment_method, 
				reference_number, attachment_url, user_notes, status
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, 'pending')
			RETURNING id, public_id, status, created_at, updated_at;
		`
		var statusStr string
		if err := tx.QueryRow(
			txCtx, query,
			dep.WalletID, dep.UserID, dep.OrganizationID, dep.Amount, dep.Currency,
			dep.PaymentMethod, dep.ReferenceNumber, dep.AttachmentURL, dep.UserNotes,
		).Scan(&dep.ID, &dep.PublicID, &statusStr, &dep.CreatedAt, &dep.UpdatedAt); err != nil {
			return fmt.Errorf("create deposit request: %w", err)
		}
		dep.Status = billing.DepositStatus(statusStr)
		return nil
	})
}

// GetDepositRequestByID retrieves a deposit request by ID.
func (r *Repository) GetDepositRequestByID(ctx context.Context, id int64) (*billing.WalletDeposit, error) {
	var dep billing.WalletDeposit
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, public_id::text, wallet_id, user_id, organization_id, amount, currency,
			       payment_method, reference_number, COALESCE(attachment_url, ''), COALESCE(user_notes, ''),
			       status, COALESCE(rejection_reason, ''), reviewed_by, reviewed_at, transaction_id,
			       created_at, updated_at
			FROM billing.wallet_deposits
			WHERE id = $1;
		`
		var statusStr string
		err := tx.QueryRow(txCtx, query, id).Scan(
			&dep.ID, &dep.PublicID, &dep.WalletID, &dep.UserID, &dep.OrganizationID, &dep.Amount, &dep.Currency,
			&dep.PaymentMethod, &dep.ReferenceNumber, &dep.AttachmentURL, &dep.UserNotes,
			&statusStr, &dep.RejectionReason, &dep.ReviewedBy, &dep.ReviewedAt, &dep.TransactionID,
			&dep.CreatedAt, &dep.UpdatedAt,
		)
		if err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("deposit_request")
			}
			return err
		}
		dep.Status = billing.DepositStatus(statusStr)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &dep, nil
}

// UpdatePendingDepositRequest updates the parameters of a pending deposit request.
func (r *Repository) UpdatePendingDepositRequest(ctx context.Context, dep *billing.WalletDeposit) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			UPDATE billing.wallet_deposits
			SET amount = $1, payment_method = $2, reference_number = $3,
			    attachment_url = CASE WHEN $4 <> '' THEN $4 ELSE attachment_url END,
			    user_notes = $5, updated_at = now()
			WHERE id = $6 AND user_id = $7 AND status = 'pending'
			RETURNING updated_at;
		`
		res, err := tx.Exec(
			txCtx, query,
			dep.Amount, dep.PaymentMethod, dep.ReferenceNumber,
			dep.AttachmentURL, dep.UserNotes, dep.ID, dep.UserID,
		)
		if err != nil {
			return fmt.Errorf("update pending deposit: %w", err)
		}
		if res.RowsAffected() == 0 {
			return apperr.Validation("deposit.not_editable", "لا يمكن تعديل العملية لأنها قيد المراجعة المتقدمة أو تم البت فيها بالفعل.", nil)
		}
		return nil
	})
}

// ListDepositRequestsByUser lists all deposit requests submitted by a user.
func (r *Repository) ListDepositRequestsByUser(ctx context.Context, userID int64, limit, offset int) ([]*billing.WalletDeposit, error) {
	if limit <= 0 {
		limit = 50
	}
	var list []*billing.WalletDeposit
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, public_id::text, wallet_id, user_id, organization_id, amount, currency,
			       payment_method, reference_number, COALESCE(attachment_url, ''), COALESCE(user_notes, ''),
			       status, COALESCE(rejection_reason, ''), reviewed_by, reviewed_at, transaction_id,
			       created_at, updated_at
			FROM billing.wallet_deposits
			WHERE user_id = $1
			ORDER BY created_at DESC
			LIMIT $2 OFFSET $3;
		`
		rows, err := tx.Query(txCtx, query, userID, limit, offset)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var dep billing.WalletDeposit
			var statusStr string
			if err := rows.Scan(
				&dep.ID, &dep.PublicID, &dep.WalletID, &dep.UserID, &dep.OrganizationID, &dep.Amount, &dep.Currency,
				&dep.PaymentMethod, &dep.ReferenceNumber, &dep.AttachmentURL, &dep.UserNotes,
				&statusStr, &dep.RejectionReason, &dep.ReviewedBy, &dep.ReviewedAt, &dep.TransactionID,
				&dep.CreatedAt, &dep.UpdatedAt,
			); err != nil {
				return err
			}
			dep.Status = billing.DepositStatus(statusStr)
			list = append(list, &dep)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return list, nil
}
