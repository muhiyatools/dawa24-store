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

	pending, err := db.PendingCount(ctx, migrations)
	if err != nil {
		t.Fatalf("cannot read migration state: %v", err)
	}
	if pending > 0 {
		t.Fatalf("%d migrations pending", pending)
	}
	return db
}

const (
	testOrgID      int64 = 88201
	testProductID  int64 = 88290
	testVariantID  int64 = 88291
	testCategoryID int64 = 88292
	testBrandID    int64 = 88293
)

func resetFixtures(t *testing.T, db *database.DB, orgID int64) {
	t.Helper()
	ctx := database.AsSystem(context.Background())
	err := db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		// Clean inventory tables
		_, _ = tx.Exec(txCtx, `DELETE FROM inventory.warehouse_transfers WHERE organization_id = $1`, orgID)
		_, _ = tx.Exec(txCtx, `DELETE FROM inventory.stock_movements WHERE organization_id = $1`, orgID)
		_, _ = tx.Exec(txCtx, `DELETE FROM inventory.stocks WHERE organization_id = $1`, orgID)
		_, _ = tx.Exec(txCtx, `DELETE FROM inventory.warehouses WHERE organization_id = $1`, orgID)
		_, _ = tx.Exec(txCtx, `DELETE FROM catalog.product_variants WHERE organization_id = $1`, orgID)
		_, _ = tx.Exec(txCtx, `DELETE FROM catalog.products WHERE organization_id = $1`, orgID)
		_, _ = tx.Exec(txCtx, `DELETE FROM catalog.categories WHERE id = $1`, testCategoryID)
		_, _ = tx.Exec(txCtx, `DELETE FROM catalog.brands WHERE id = $1`, testBrandID)

		// Setup prerequisite org, category, brand, product, variant for stock FKs
		_, _ = tx.Exec(txCtx, `INSERT INTO org.organizations (id, name) VALUES ($1, '{"en":"Inventory Test Org"}') ON CONFLICT DO NOTHING`, orgID)
		_, _ = tx.Exec(txCtx, `INSERT INTO catalog.categories (id, name, slug) VALUES ($1, '{"en":"Test Cat"}', 'inv-test-cat') ON CONFLICT DO NOTHING`, testCategoryID)
		_, _ = tx.Exec(txCtx, `INSERT INTO catalog.brands (id, name, slug) VALUES ($1, '{"en":"Test Brand"}', 'inv-test-brand') ON CONFLICT DO NOTHING`, testBrandID)
		_, _ = tx.Exec(txCtx, `INSERT INTO catalog.products (id, organization_id, category_id, brand_id, name, slug, dosage_form)
			VALUES ($1, $2, $3, $4, '{"en":"Test Prod"}', 'inv-test-prod', 'tablet') ON CONFLICT DO NOTHING`,
			testProductID, orgID, testCategoryID, testBrandID)
		_, _ = tx.Exec(txCtx, `INSERT INTO catalog.product_variants (id, organization_id, product_id, sku, price)
			VALUES ($1, $2, $3, 'INV-VAR-1', 50.00) ON CONFLICT DO NOTHING`,
			testVariantID, orgID, testProductID)
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

	resetFixtures(t, db, testOrgID)

	ctx := database.WithTenant(context.Background(), testOrgID)
	repo := postgres.NewRepository(db)

	var warehouse1ID int64
	var warehouse2ID int64
	var stockID int64
	var transferID int64

	t.Run("Warehouses", func(t *testing.T) {
		w := &inventory.Warehouse{
			OrganizationID: testOrgID,
			Name:           "Main Warehouse",
			IsActive:       true,
		}
		err := repo.CreateWarehouse(ctx, w)
		if err != nil {
			t.Fatalf("CreateWarehouse failed: %v", err)
		}
		if w.ID == 0 {
			t.Fatal("expected generated warehouse ID")
		}
		warehouse1ID = w.ID

		got, err := repo.GetWarehouseByID(ctx, warehouse1ID)
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

		count, err := repo.CountStockInWarehouse(ctx, warehouse1ID)
		if err != nil {
			t.Fatalf("CountStockInWarehouse failed: %v", err)
		}
		if count != 0 {
			t.Errorf("expected 0 stock items, got %d", count)
		}
	})

	t.Run("Stocks and Movements", func(t *testing.T) {
		s := &inventory.Stock{
			OrganizationID:   testOrgID,
			WarehouseID:      warehouse1ID,
			ProductID:        testProductID,
			ProductVariantID: testVariantID,
			Quantity:         100,
			MinThreshold:     10,
		}
		err := repo.UpsertStock(ctx, s)
		if err != nil {
			t.Fatalf("UpsertStock failed: %v", err)
		}

		got, err := repo.GetStock(ctx, warehouse1ID, testVariantID)
		if err != nil {
			t.Fatalf("GetStock failed: %v", err)
		}
		if got.Quantity != 100 {
			t.Errorf("expected 100 quantity, got %d", got.Quantity)
		}
		stockID = got.ID

		movement := inventory.StockMovement{
			OrganizationID: testOrgID,
			StockID:        stockID,
			Type:           inventory.MovementAdjustment,
			QuantityDelta:  -5,
			BalanceAfter:   95,
		}
		adjGot, err := repo.AdjustStock(ctx, stockID, -5, movement)
		if err != nil {
			t.Fatalf("AdjustStock failed: %v", err)
		}
		if adjGot.Quantity != 95 {
			t.Errorf("expected 95 quantity, got %d", adjGot.Quantity)
		}

		stocks, err := repo.ListStocksByWarehouse(ctx, warehouse1ID)
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
		_ = low

		moves, err := repo.ListStockMovements(ctx, stockID, 10)
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
		w2 := &inventory.Warehouse{
			OrganizationID: testOrgID,
			Name:           "Second Warehouse",
			IsActive:       true,
		}
		err := repo.CreateWarehouse(ctx, w2)
		if err != nil {
			t.Fatalf("CreateWarehouse 2 failed: %v", err)
		}
		warehouse2ID = w2.ID

		transfer := &inventory.WarehouseTransfer{
			OrganizationID:   testOrgID,
			FromWarehouseID:  warehouse1ID,
			ToWarehouseID:    warehouse2ID,
			ProductID:        testProductID,
			ProductVariantID: testVariantID,
			Quantity:         10,
			Status:           inventory.TransferPending,
		}
		err = repo.CreateTransfer(ctx, transfer)
		if err != nil {
			t.Fatalf("CreateTransfer failed: %v", err)
		}
		if transfer.ID == 0 {
			t.Fatal("expected generated transfer ID")
		}
		transferID = transfer.ID

		got, err := repo.GetTransferByID(ctx, transferID)
		if err != nil {
			t.Fatalf("GetTransferByID failed: %v", err)
		}
		if got.Quantity != 10 {
			t.Errorf("expected transfer quantity 10, got %d", got.Quantity)
		}

		err = repo.UpdateTransferStatus(ctx, transferID, inventory.TransferPending, inventory.TransferInTransit)
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
		err := repo.SoftDeleteWarehouse(ctx, warehouse2ID)
		if err != nil {
			t.Fatalf("SoftDeleteWarehouse %d failed: %v", warehouse2ID, err)
		}
		err = repo.SoftDeleteWarehouse(ctx, warehouse1ID)
		if err != nil {
			t.Fatalf("SoftDeleteWarehouse %d failed: %v", warehouse1ID, err)
		}
	})
}
