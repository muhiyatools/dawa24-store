// Package integration_test verifies PostgreSQL row-level security policies.
//
// Every tenant-owned table must be tested here to prove that a query without
// tenant filtering returns zero rows for another tenant (ADR 0003).
package integration_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	dbfs "github.com/muhiya/dawa24-store/db"
	"github.com/muhiya/dawa24-store/internal/platform/config"
	"github.com/muhiya/dawa24-store/internal/platform/database"
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
		if os.Getenv("CI") == "true" {
			t.Fatal("DATABASE_URL or TEST_DATABASE_URL must be set in CI")
		}
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
		if os.Getenv("CI") == "true" {
			t.Fatalf("CI failed to connect to database at %s: %v", dbURL, err)
		}
		t.Skipf("cannot connect to database at %s: %v; skipping", dbURL, err)
		return nil
	}

	// Ensure migrations are applied
	migrations, err := database.LoadMigrations(dbfs.Migrations, "migrations")
	if err != nil {
		t.Fatalf("failed to load migrations: %v", err)
	}

	if err := db.Migrate(ctx, migrations, func(string, ...any) {}); err != nil {
		t.Fatalf("failed to apply migrations: %v", err)
	}

	return db
}

func TestTenantIsolation_Branches(t *testing.T) {
	db := getTestDB(t)
	if db == nil {
		return
	}
	defer db.Close()

	ctx := context.Background()
	const orgA int64 = 8801
	const orgB int64 = 8802

	// Setup: create test orgs and user in system context
	err := db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		_, _ = tx.Exec(txCtx, `
			INSERT INTO identity.users (id, email, password_hash, name)
			VALUES (88001, 'user88001@example.com', '$2y$10$xyz', '{"ar":"مستخدم","en":"User"}')
			ON CONFLICT (id) DO NOTHING;
		`)
		_, _ = tx.Exec(txCtx, `
			INSERT INTO org.organizations (id, name)
			VALUES ($1, '{"ar":"مؤسسة أ","en":"Org A"}')
			ON CONFLICT (id) DO NOTHING;
		`, orgA)
		_, _ = tx.Exec(txCtx, `
			INSERT INTO org.organizations (id, name)
			VALUES ($1, '{"ar":"مؤسسة ب","en":"Org B"}')
			ON CONFLICT (id) DO NOTHING;
		`, orgB)
		return nil
	})
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	// Step 1: Insert branch for Org A under Org A tenant context
	ctxA := database.WithTenant(ctx, orgA)
	var branchID int64
	err = db.InTx(ctxA, func(txCtx context.Context, tx pgx.Tx) error {
		row := tx.QueryRow(txCtx, `
			INSERT INTO org.branches (organization_id, name, is_main)
			VALUES ($1, '{"ar":"فرع القاهرة","en":"Cairo Branch"}', true)
			RETURNING id;
		`, orgA)
		return row.Scan(&branchID)
	})
	if err != nil {
		t.Fatalf("failed to insert branch for org A: %v", err)
	}

	// Step 2: Query branches from Org B tenant context — MUST RETURN 0 ROWS
	ctxB := database.WithTenant(ctx, orgB)
	err = db.InReadTx(ctxB, func(txCtx context.Context, tx pgx.Tx) error {
		var count int
		if err := tx.QueryRow(txCtx, "SELECT count(*) FROM org.branches WHERE id = $1", branchID).Scan(&count); err != nil {
			return err
		}
		if count != 0 {
			t.Errorf("SECURITY LEAK: Org B read Org A's branch! count = %d; want 0", count)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("read as Org B failed: %v", err)
	}

	// Step 3: Query branches without any tenant context — MUST RETURN 0 ROWS
	err = db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		var count int
		if err := tx.QueryRow(txCtx, "SELECT count(*) FROM org.branches WHERE id = $1", branchID).Scan(&count); err != nil {
			return err
		}
		if count != 0 {
			t.Errorf("SECURITY LEAK: Unauthenticated/tenant-less query read branch! count = %d; want 0", count)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("read without tenant failed: %v", err)
	}

	// Step 4: Query branches with AsSystem context — MUST BE VISIBLE
	err = db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		var count int
		if err := tx.QueryRow(txCtx, "SELECT count(*) FROM org.branches WHERE id = $1", branchID).Scan(&count); err != nil {
			return err
		}
		if count != 1 {
			t.Errorf("System query failed to find branch: count = %d; want 1", count)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("read as system failed: %v", err)
	}

	// Step 5: Attempt to insert row for Org A while authenticated as Org B — MUST FAIL RLS WITH CHECK
	err = db.InTx(ctxB, func(txCtx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(txCtx, `
			INSERT INTO org.branches (organization_id, name, is_main)
			VALUES ($1, '{"ar":"فرع خبيث","en":"Malicious Branch"}', false);
		`, orgA)
		return err
	})
	if err == nil {
		t.Errorf("SECURITY LEAK: Org B was able to insert a branch owned by Org A!")
	}
}

func TestTenantIsolation_Members(t *testing.T) {
	db := getTestDB(t)
	if db == nil {
		return
	}
	defer db.Close()

	ctx := context.Background()
	const orgA int64 = 8901
	const orgB int64 = 8902

	// Setup: create test orgs and user in system context
	err := db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		_, _ = tx.Exec(txCtx, `
			INSERT INTO identity.users (id, email, password_hash, name)
			VALUES (89001, 'user89001@example.com', '$2y$10$xyz', '{"ar":"مستخدم","en":"User"}')
			ON CONFLICT (id) DO NOTHING;
		`)
		_, _ = tx.Exec(txCtx, `
			INSERT INTO org.organizations (id, name)
			VALUES ($1, '{"ar":"مؤسسة 1","en":"Org 1"}')
			ON CONFLICT (id) DO NOTHING;
		`, orgA)
		_, _ = tx.Exec(txCtx, `
			INSERT INTO org.organizations (id, name)
			VALUES ($1, '{"ar":"مؤسسة 2","en":"Org 2"}')
			ON CONFLICT (id) DO NOTHING;
		`, orgB)
		return nil
	})
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	// Insert member in Org A tenant context
	ctxA := database.WithTenant(ctx, orgA)
	var memberID int64
	err = db.InTx(ctxA, func(txCtx context.Context, tx pgx.Tx) error {
		row := tx.QueryRow(txCtx, `
			INSERT INTO org.members (organization_id, user_id, role_key)
			VALUES ($1, 89001, 'org_manager')
			RETURNING id;
		`, orgA)
		return row.Scan(&memberID)
	})
	if err != nil {
		t.Fatalf("failed to insert member for org A: %v", err)
	}

	// Query from Org B context — MUST RETURN 0 ROWS
	ctxB := database.WithTenant(ctx, orgB)
	err = db.InReadTx(ctxB, func(txCtx context.Context, tx pgx.Tx) error {
		var count int
		if err := tx.QueryRow(txCtx, "SELECT count(*) FROM org.members WHERE id = $1", memberID).Scan(&count); err != nil {
			return err
		}
		if count != 0 {
			t.Errorf("SECURITY LEAK: Org B read Org A's members! count = %d; want 0", count)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("read as Org B failed: %v", err)
	}

	// Query with AsSystem — MUST BE VISIBLE
	err = db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		var count int
		if err := tx.QueryRow(txCtx, "SELECT count(*) FROM org.members WHERE id = $1", memberID).Scan(&count); err != nil {
			return err
		}
		if count != 1 {
			t.Errorf("System query failed to find member: count = %d; want 1", count)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("read as system failed: %v", err)
	}
}
