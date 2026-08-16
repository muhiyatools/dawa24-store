package postgres_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	dbfs "github.com/muhiya/dawa24-store/db"
	"github.com/muhiya/dawa24-store/internal/modules/inventory"
	"github.com/muhiya/dawa24-store/internal/modules/inventory/postgres"
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
		t.Skipf("cannot connect to database: %v", err)
	}

	migrations, err := database.LoadMigrations(dbfs.Migrations, "migrations")
	if err != nil {
		t.Fatalf("failed to load migrations: %v", err)
	}

	// Deliberately no superuser skip.
	//
	// The RLS suite skips for a superuser because a superuser bypasses
	// row-level security, so it cannot prove isolation either way. That
	// reasoning does not transfer here: these tests check that the SQL is
	// correct -- columns exist, types scan, money round-trips -- and a
	// superuser answers those questions perfectly well. Copying the skip meant
	// these tests reported `ok` while executing nothing, which is the exact
	// failure mode they were written to prevent.

	pending, err := db.PendingCount(ctx, migrations)
	if err != nil {
		t.Fatalf("cannot read migration state: %v", err)
	}
	if pending > 0 {
		t.Fatalf("%d migrations pending", pending)
	}
	return db
}

func resetFixtures(t *testing.T, db *database.DB, orgID int64) {
	t.Helper()
	ctx := database.AsSystem(context.Background())
	err := db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		// Clean inventory tables
		_, _ = tx.Exec(txCtx, `DELETE FROM inventory.warehouse_transfers WHERE organization_id = $1`, orgID)
		_, _ = tx.Exec(txCtx, `DELETE FROM inventory.stock_movements WHERE organization_id = $1`, orgID)
		_, _ = tx.Exec(txCtx, `DELETE FROM inventory.stocks WHERE organization_id = $1`, orgID)
		_, _ = tx.Exec(txCtx, `DELETE FROM inventory.warehouses WHERE organization_id = $1`, orgID)
		// Setup org
		_, _ = tx.Exec(txCtx, `INSERT INTO org.organizations (id, name) VALUES ($1, '{"en":"Inventory Test Org"}') ON CONFLICT DO NOTHING`, orgID)
		return nil
	})
	if err != nil {
		t.Fatalf("reset fixtures failed: %v", err)
	}
}

func TestInventoryRepository(t *testing.T) {
	db := getTestDB(t)
	if db == nil {
		return
	}
	defer db.Close()

	const orgID int64 = 88201
	resetFixtures(t, db, orgID)

	ctx := database.WithTenant(context.Background(), orgID)
	repo := postgres.NewRepository(db)

	t.Run("Warehouses", func(t *testing.T) {
		w := &inventory.Warehouse{
			ID:             88200,
			OrganizationID: orgID,
			Name:           "Main Warehouse",
			IsActive:       true,
		}
		err := repo.CreateWarehouse(ctx, w)
		if err != nil {
			t.Fatalf("CreateWarehouse failed: %v", err)
		}

		got, err := repo.GetWarehouseByID(ctx, 88200)
		if err != nil {
			t.Fatalf("GetWarehouseByID failed: %v", err)
		}
		if got.Name != "Main Warehouse" {
			t.Errorf("expected 'Main Warehouse', got %q", got.Name)
		}
		if got.Latitude != nil || got.BranchID != nil {
			t.Errorf("expected nullable columns to be nil")
		}

		w.Name = "Updated Warehouse"
		err = repo.UpdateWarehouse(ctx, w)
		if err != nil {
			t.Fatalf("UpdateWarehouse failed: %v", err)
		}

		list, err := repo.ListWarehouses(ctx)
		if err != nil {
			t.Fatalf("ListWarehouses failed: %v", err)
		}
		if len(list) == 0 {
			t.Error("expected at least 1 warehouse")
		}

		count, err := repo.CountStockInWarehouse(ctx, 88200)
		if err != nil {
			t.Fatalf("CountStockInWarehouse failed: %v", err)
		}
		if count != 0 {
			t.Errorf("expected 0 stock items, got %d", count)
		}
	})

	t.Run("Stocks and Movements", func(t *testing.T) {
		s := &inventory.Stock{
			ID:               88201,
			OrganizationID:   orgID,
			WarehouseID:      88200,
			ProductID:        88202, // dummy
			ProductVariantID: 88203, // dummy
			Quantity:         100,
			MinThreshold:     10,
		}
		err := repo.UpsertStock(ctx, s)
		if err != nil {
			t.Fatalf("UpsertStock failed: %v", err)
		}

		got, err := repo.GetStock(ctx, 88200, 88203)
		if err != nil {
			t.Fatalf("GetStock failed: %v", err)
		}
		if got.Quantity != 100 {
			t.Errorf("expected 100 quantity, got %d", got.Quantity)
		}

		movement := inventory.StockMovement{
			OrganizationID: orgID,
			StockID:        got.ID,
			Type:           inventory.MovementAdjustment,
			QuantityDelta:  -5,
			BalanceAfter:   95,
		}
		adjGot, err := repo.AdjustStock(ctx, got.ID, -5, movement)
		if err != nil {
			t.Fatalf("AdjustStock failed: %v", err)
		}
		if adjGot.Quantity != 95 {
			t.Errorf("expected 95 quantity, got %d", adjGot.Quantity)
		}

		stocks, err := repo.ListStocksByWarehouse(ctx, 88200)
		if err != nil {
			t.Fatalf("ListStocksByWarehouse failed: %v", err)
		}
		if len(stocks) == 0 {
			t.Error("expected at least 1 stock")
		}

		low, err := repo.ListLowStock(ctx, 10, 0)
		if err != nil {
			t.Fatalf("ListLowStock failed: %v", err)
		}
		// Quantity is 95, min threshold is 10, so it's not low. We just check no error.
		_ = low

		moves, err := repo.ListStockMovements(ctx, got.ID, 10)
		if err != nil {
			t.Fatalf("ListStockMovements failed: %v", err)
		}
		if len(moves) == 0 {
			t.Error("expected at least 1 movement")
		}

		orgMoves, err := repo.ListMovementsByOrg(ctx, 10, 0)
		if err != nil {
			t.Fatalf("ListMovementsByOrg failed: %v", err)
		}
		if len(orgMoves) == 0 {
			t.Error("expected at least 1 org movement")
		}
	})

	t.Run("Transfers", func(t *testing.T) {
		// Create another warehouse for transfer
		w2 := &inventory.Warehouse{
			ID:             88201,
			OrganizationID: orgID,
			Name:           "Second Warehouse",
			IsActive:       true,
		}
		err := repo.CreateWarehouse(ctx, w2)
		if err != nil {
			t.Fatalf("CreateWarehouse 2 failed: %v", err)
		}

		transfer := &inventory.WarehouseTransfer{
			ID:               88202,
			OrganizationID:   orgID,
			FromWarehouseID:  88200,
			ToWarehouseID:    88201,
			ProductID:        88202,
			ProductVariantID: 88203,
			Quantity:         10,
			Status:           inventory.TransferPending,
		}
		err = repo.CreateTransfer(ctx, transfer)
		if err != nil {
			t.Fatalf("CreateTransfer failed: %v", err)
		}

		got, err := repo.GetTransferByID(ctx, 88202)
		if err != nil {
			t.Fatalf("GetTransferByID failed: %v", err)
		}
		if got.Quantity != 10 {
			t.Errorf("expected transfer quantity 10, got %d", got.Quantity)
		}

		err = repo.UpdateTransferStatus(ctx, 88202, inventory.TransferPending, inventory.TransferInTransit)
		if err != nil {
			t.Fatalf("UpdateTransferStatus failed: %v", err)
		}

		list, err := repo.ListTransfers(ctx, string(inventory.TransferInTransit), 10, 0)
		if err != nil {
			t.Fatalf("ListTransfers failed: %v", err)
		}
		if len(list) == 0 {
			t.Error("expected at least 1 transfer in list")
		}
	})

	t.Run("Cleanup", func(t *testing.T) {
		err := repo.SoftDeleteWarehouse(ctx, 88201)
		if err != nil {
			t.Fatalf("SoftDeleteWarehouse 88201 failed: %v", err)
		}
		err = repo.SoftDeleteWarehouse(ctx, 88200)
		if err != nil {
			t.Fatalf("SoftDeleteWarehouse 88200 failed: %v", err)
		}
	})
}
