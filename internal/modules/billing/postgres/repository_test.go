package postgres_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	dbfs "github.com/muhiya/dawa24-store/db"
	"github.com/muhiya/dawa24-store/internal/modules/billing"
	"github.com/muhiya/dawa24-store/internal/modules/billing/postgres"
	"github.com/muhiya/dawa24-store/internal/platform/config"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

func getTestDB(t *testing.T) *database.DB {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping database integration test in short mode")
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = os.Getenv("TEST_DATABASE_URL")
	}
	if dbURL == "" {
		t.Skip("DATABASE_URL not set; skipping integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cfg := config.Database{
		URL:              dbURL,
		MaxConns:         5,
		MinConns:         1,
		MaxConnLifetime:  time.Hour,
		MaxConnIdleTime:  30 * time.Minute,
		StatementTimeout: 10 * time.Second,
	}

	db, err := database.Open(ctx, cfg)
	if err != nil {
		t.Skipf("cannot connect to database: %v; skipping", err)
	}

	var isSuper bool
	if err := db.Pool().QueryRow(ctx,
		`SELECT rolsuper FROM pg_roles WHERE rolname = current_user`).Scan(&isSuper); err == nil && isSuper {
		t.Skip("connected as a superuser, which bypasses RLS; point DATABASE_URL at application role")
	}

	migrations, _ := database.LoadMigrations(dbfs.Migrations, "migrations")
	if pending, _ := db.PendingCount(ctx, migrations); pending > 0 {
		t.Fatalf("%d migrations pending", pending)
	}

	return db
}

func resetBillingFixtures(t *testing.T, db *database.DB) {
	t.Helper()
	ctx := database.AsSystem(context.Background())
	err := db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		tx.Exec(txCtx, `DELETE FROM billing.wallet_transactions WHERE wallet_id IN (SELECT id FROM billing.wallets WHERE user_id = 88400)`)
		tx.Exec(txCtx, `DELETE FROM billing.wallets WHERE user_id = 88400`)
		tx.Exec(txCtx, `DELETE FROM billing.payments WHERE user_id = 88400`)
		tx.Exec(txCtx, `DELETE FROM billing.subscriptions WHERE user_id = 88400`)
		tx.Exec(txCtx, `DELETE FROM billing.invoices WHERE organization_id = 88401`)
		tx.Exec(txCtx, `DELETE FROM org.organizations WHERE id = 88401`)
		tx.Exec(txCtx, `DELETE FROM identity.users WHERE id = 88400`)
		return nil
	})
	if err != nil {
		t.Fatalf("reset fixtures failed: %v", err)
	}
}

func TestBillingRepository(t *testing.T) {
	db := getTestDB(t)
	if db == nil {
		return
	}
	defer db.Close()

	resetBillingFixtures(t, db)

	ctx := context.Background()
	sysCtx := database.AsSystem(ctx)

	err := db.InTx(sysCtx, func(txCtx context.Context, tx pgx.Tx) error {
		_, _ = tx.Exec(txCtx, `INSERT INTO identity.users (id, email, password_hash, name) VALUES (88400, 'user88400@example.com', 'x', '{"en":"User"}') ON CONFLICT DO NOTHING`)
		_, _ = tx.Exec(txCtx, `INSERT INTO org.organizations (id, name) VALUES (88401, '{"en":"Org A"}') ON CONFLICT DO NOTHING`)
		return nil
	})
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	repo := postgres.NewRepository(db)

	t.Run("Wallet_Operations", func(t *testing.T) {
		w, err := repo.GetOrCreateWallet(ctx, 88400, "EGP")
		if err != nil {
			t.Fatalf("GetOrCreateWallet failed: %v", err)
		}

		if w.Balance.Minor() != 0 {
			t.Errorf("expected 0 balance, got %v", w.Balance)
		}

		amount := money.MustParse("150.00")
		txRec, err := repo.RecordTransaction(ctx, w.ID, billing.TxDeposit, amount, "manual", nil, "test deposit")
		if err != nil {
			t.Fatalf("RecordTransaction failed: %v", err)
		}

		if txRec.Amount.Minor() != amount.Minor() {
			t.Errorf("money round-trip failed: got %v, want %v", txRec.Amount, amount)
		}
		if txRec.ReferenceID != nil {
			t.Errorf("expected nil ReferenceID, got %v", txRec.ReferenceID)
		}

		w2, _ := repo.GetWallet(ctx, w.ID)
		if w2.Balance.Minor() != amount.Minor() {
			t.Errorf("balance update failed: got %v", w2.Balance)
		}

		list, _ := repo.ListTransactions(ctx, w.ID, 10, 0)
		if len(list) != 1 {
			t.Errorf("expected 1 tx, got %d", len(list))
		}
	})

	t.Run("Invoices_RLS_and_CRUD", func(t *testing.T) {
		ctxOrgA := database.WithTenant(ctx, 88401)
		ctxOrgB := database.WithTenant(ctx, 88402) // non-existent but tests isolation

		subtotal := money.MustParse("500.00")
		inv := &billing.Invoice{
			OrganizationID: 88401,
			InvoiceNumber:  "TEST-88401",
			Subtotal:       subtotal,
			Status:         billing.InvoiceDraft,
			IssueDate:      time.Now(),
			DueDate:        time.Now().AddDate(0, 1, 0),
		}

		err := repo.CreateInvoice(ctxOrgA, inv)
		if err != nil {
			t.Fatalf("CreateInvoice failed: %v", err)
		}

		// Check RLS isolation
		_, err = repo.GetInvoiceByID(ctxOrgB, inv.ID)
		if err == nil {
			t.Error("SECURITY LEAK: OrgB read OrgA's invoice")
		}

		readInv, err := repo.GetInvoiceByID(ctxOrgA, inv.ID)
		if err != nil {
			t.Fatalf("GetInvoiceByID failed: %v", err)
		}
		if readInv.Subtotal.Minor() != subtotal.Minor() {
			t.Errorf("money round-trip failed for invoice: %v", readInv.Subtotal)
		}

		err = repo.UpdateInvoiceStatus(ctxOrgA, inv.ID, billing.InvoicePaid)
		if err != nil {
			t.Fatalf("UpdateInvoiceStatus failed: %v", err)
		}

		list, err := repo.ListInvoicesByOrg(ctxOrgA, 88401, 10, 0)
		if err != nil {
			t.Fatalf("ListInvoicesByOrg failed: %v", err)
		}
		if len(list) == 0 {
			t.Error("expected at least 1 invoice")
		}
	})
}
