package ui_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

func seedProduct(t *testing.T, db *database.DB) int64 {
	t.Helper()
	ctx := context.Background()
	var prodID int64

	err := db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(txCtx, `
			INSERT INTO catalog.products (
				name, type, status, created_at, updated_at
			) VALUES (
				'{"ar": "بنادول إكسترا", "en": "Panadol Extra"}'::jsonb,
				'medicine', 'active', now(), now()
			) RETURNING id
		`).Scan(&prodID)
	})
	if err != nil {
		t.Fatalf("seedProduct failed: %v", err)
	}

	t.Cleanup(func() {
		_ = db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
			_, _ = tx.Exec(txCtx, "DELETE FROM catalog.products WHERE id = $1", prodID)
			return nil
		})
	})

	return prodID
}

func seedOffer(t *testing.T, db *database.DB, orgID int64) int64 {
	t.Helper()
	ctx := context.Background()
	var offerID int64

	err := db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(txCtx, `
			INSERT INTO promo.offers (
				organization_id, title, discount_type, discount_value, min_order_value, starts_at, expires_at, is_active, created_at, updated_at
			) VALUES (
				$1, '{"ar": "عرض خصم خاص", "en": "Special Offer"}'::jsonb,
				'percentage', 15.00, 100.00, now(), now() + interval '30 days', true, now(), now()
			) RETURNING id
		`, orgID).Scan(&offerID)
	})
	if err != nil {
		t.Fatalf("seedOffer failed: %v", err)
	}

	t.Cleanup(func() {
		_ = db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
			_, _ = tx.Exec(txCtx, "DELETE FROM promo.offers WHERE id = $1", offerID)
			return nil
		})
	})

	return offerID
}

func seedOrder(t *testing.T, db *database.DB, customerOrgID, vendorOrgID int64) int64 {
	t.Helper()
	ctx := context.Background()
	var orderID int64

	err := db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		orderNum := fmt.Sprintf("ORD-%d", time.Now().UnixNano()%10000000)
		return tx.QueryRow(txCtx, `
			INSERT INTO commerce.orders (
				order_number, customer_org_id, vendor_org_id, total_amount, status, created_at, updated_at
			) VALUES (
				$1, $2, $3, 500.00, 'pending', now(), now()
			) RETURNING id
		`, orderNum, customerOrgID, vendorOrgID).Scan(&orderID)
	})
	if err != nil {
		t.Fatalf("seedOrder failed: %v", err)
	}

	t.Cleanup(func() {
		_ = db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
			_, _ = tx.Exec(txCtx, "DELETE FROM commerce.orders WHERE id = $1", orderID)
			return nil
		})
	})

	return orderID
}

func seedInvoice(t *testing.T, db *database.DB, orgID int64, amount money.Amount) int64 {
	t.Helper()
	ctx := context.Background()
	var invID int64

	err := db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		invNum := fmt.Sprintf("INV-%d", time.Now().UnixNano()%10000000)
		return tx.QueryRow(txCtx, `
			INSERT INTO billing.invoices (
				invoice_number, organization_id, total_amount, status, created_at, updated_at
			) VALUES (
				$1, $2, $3, 'unpaid', now(), now()
			) RETURNING id
		`, invNum, orgID, amount).Scan(&invID)
	})
	if err != nil {
		t.Fatalf("seedInvoice failed: %v", err)
	}

	t.Cleanup(func() {
		_ = db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
			_, _ = tx.Exec(txCtx, "DELETE FROM billing.invoices WHERE id = $1", invID)
			return nil
		})
	})

	return invID
}

func seedPayment(t *testing.T, db *database.DB, orgID int64, amount money.Amount, status string) int64 {
	t.Helper()
	ctx := context.Background()
	var payID int64

	userID := seedUser(t, db, orgID, "member")

	err := db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(txCtx, `
			INSERT INTO billing.payments (
				user_id, organization_id, amount, method, status, created_at, updated_at
			) VALUES (
				$1, $2, $3, 'cash', $4, now(), now()
			) RETURNING id
		`, userID, orgID, amount, status).Scan(&payID)
	})
	if err != nil {
		t.Fatalf("seedPayment failed: %v", err)
	}

	t.Cleanup(func() {
		_ = db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
			_, _ = tx.Exec(txCtx, "DELETE FROM billing.payments WHERE id = $1", payID)
			return nil
		})
	})

	return payID
}

func seedWarehouse(t *testing.T, db *database.DB, orgID int64) int64 {
	t.Helper()
	ctx := context.Background()
	var whID int64

	err := db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(txCtx, `
			INSERT INTO inventory.warehouses (
				organization_id, name, is_active, created_at, updated_at
			) VALUES (
				$1, '{"ar": "مستودع تجريبي", "en": "Test Warehouse"}'::jsonb,
				true, now(), now()
			) RETURNING id
		`, orgID).Scan(&whID)
	})
	if err != nil {
		t.Fatalf("seedWarehouse failed: %v", err)
	}

	t.Cleanup(func() {
		_ = db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
			_, _ = tx.Exec(txCtx, "DELETE FROM inventory.warehouses WHERE id = $1", whID)
			return nil
		})
	})

	return whID
}

func seedPolicy(t *testing.T, db *database.DB, key, title, content string) int64 {
	t.Helper()
	ctx := context.Background()
	var policyID int64

	err := db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(txCtx, `
			INSERT INTO platform_admin.policies (
				key, title, content, is_published, version, created_at, updated_at
			) VALUES (
				$1, $2, $3, true, 1, now(), now()
			) RETURNING id
		`, key, title, content).Scan(&policyID)
	})
	if err != nil {
		t.Fatalf("seedPolicy failed: %v", err)
	}

	t.Cleanup(func() {
		_ = db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
			_, _ = tx.Exec(txCtx, "DELETE FROM platform_admin.policies WHERE id = $1", policyID)
			return nil
		})
	})

	return policyID
}
