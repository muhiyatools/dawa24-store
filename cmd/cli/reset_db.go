package main

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/identity"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// runResetDB wipes all transactional, catalogue, tenant, and customer rows from
// the database while preserving the platform super_admin account and essential
// reference tables (cities, roles, review criteria).
func runResetDB(ctx context.Context, db *database.DB, log *slog.Logger) error {
	log.InfoContext(ctx, "starting database full reset (leaving only super_admin)")

	hash, err := identity.HashPassword("Dawa24!Test")
	if err != nil {
		return fmt.Errorf("hash admin password: %w", err)
	}

	adminName, err := i18n.New("مدير المنصة العام", "Platform Super Admin").Value()
	if err != nil {
		return fmt.Errorf("encode admin name: %w", err)
	}

	err = db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		// 1. Truncate all transactional, tenant, and domain tables.
		truncateQuery := `
			TRUNCATE TABLE
				commerce.order_status_history,
				commerce.order_items,
				commerce.invoices,
				commerce.payments,
				commerce.orders,
				commerce.cart_items,
				commerce.carts,
				inventory.lots,
				inventory.adjustments,
				inventory.transfers,
				catalog.product_images,
				catalog.products,
				promo.coupon_redemptions,
				promo.coupons,
				promo.offer_products,
				promo.offers,
				promo.special_offers,
				hr.job_applications,
				hr.resumes,
				hr.jobs,
				platform_admin.documents,
				platform_admin.service_requests,
				platform_admin.services,
				platform_admin.job_applications,
				platform_admin.jobs,
				platform_admin.activity_logs,
				chat.messages,
				chat.conversations,
				workflow.approvals,
				workflow.audit_logs,
				notifications.notifications,
				billing.transactions,
				billing.wallets,
				org.review_ratings,
				org.reviews,
				org.policies,
				org.coverage_areas,
				org.members,
				org.branches,
				org.organizations,
				identity.sessions,
				identity.user_addresses,
				identity.user_address_history,
				identity.user_favorites,
				identity.user_preferences,
				identity.audit_logs
			CASCADE;
		`
		if _, err := tx.Exec(txCtx, truncateQuery); err != nil {
			return fmt.Errorf("truncate application tables: %w", err)
		}

		// 2. Delete all non-staff users.
		if _, err := tx.Exec(txCtx, `
			DELETE FROM identity.user_security WHERE user_id IN (SELECT id FROM identity.users WHERE role != 'super_admin');
			DELETE FROM identity.user_mfa WHERE user_id IN (SELECT id FROM identity.users WHERE role != 'super_admin');
			DELETE FROM identity.users WHERE role != 'super_admin';
		`); err != nil {
			return fmt.Errorf("delete non-admin users: %w", err)
		}

		// 3. Upsert super_admin account.
		var adminID int64
		if err := tx.QueryRow(txCtx, `
			INSERT INTO identity.users (email, password_hash, name, role, status, language, timezone)
			VALUES ('admin@dawa24.com', $1, $2::jsonb, 'super_admin', 'active', 'ar', 'Africa/Cairo')
			ON CONFLICT (email) WHERE deleted_at IS NULL DO UPDATE SET
				password_hash = EXCLUDED.password_hash,
				name          = EXCLUDED.name,
				role          = 'super_admin',
				status        = 'active',
				deleted_at    = NULL,
				updated_at    = now()
			RETURNING id;
		`, hash, adminName).Scan(&adminID); err != nil {
			return fmt.Errorf("upsert admin@dawa24.com: %w", err)
		}

		// Also ensure admin@dawa24.test exists for local testing
		if _, err := tx.Exec(txCtx, `
			INSERT INTO identity.users (email, password_hash, name, role, status, language, timezone)
			VALUES ('admin@dawa24.test', $1, $2::jsonb, 'super_admin', 'active', 'ar', 'Africa/Cairo')
			ON CONFLICT (email) WHERE deleted_at IS NULL DO UPDATE SET
				password_hash = EXCLUDED.password_hash,
				name          = EXCLUDED.name,
				role          = 'super_admin',
				status        = 'active',
				deleted_at    = NULL,
				updated_at    = now();
		`, hash, adminName); err != nil {
			return fmt.Errorf("upsert admin@dawa24.test: %w", err)
		}

		// Enable user_security for admin.
		if _, err := tx.Exec(txCtx, `
			INSERT INTO identity.user_security (user_id, login_attempts)
			VALUES ($1, 0)
			ON CONFLICT (user_id) DO UPDATE SET login_attempts = 0, locked_until = NULL;
		`, adminID); err != nil {
			return fmt.Errorf("reset admin user_security: %w", err)
		}

		log.InfoContext(txCtx, "database successfully reset to clean zero state with super_admin intact")
		return nil
	})

	return err
}

func resetDBHelp() string {
	return `
Database Reset Complete!
All application data rows have been wiped.
The platform is ready for clean end-to-end testing from scratch.

Super Admin Account:
  Email:    admin@dawa24.com (also admin@dawa24.test)
  Password: Dawa24!Test
  Role:     super_admin
  URL:      /admin/dashboard
`
}