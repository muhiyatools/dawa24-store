package postgres_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	dbfs "github.com/muhiya/dawa24-store/db"
	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/modules/commerce/postgres"
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

	migrations, _ := database.LoadMigrations(dbfs.Migrations, "migrations")
	if pending, _ := db.PendingCount(ctx, migrations); pending > 0 {
		t.Fatalf("%d migrations pending", pending)
	}

	return db
}

const (
	testCommerceUserID   int64 = 88390
	testCommerceVendorID int64 = 88391
	testCommerceCustID   int64 = 88392
	testCommerceProdID   int64 = 88393
	testCommerceVarID    int64 = 88394
	testCommerceCatID    int64 = 88395
	testCommerceBrandID  int64 = 88396
)

func resetCommerceFixtures(t *testing.T, db *database.DB) {
	t.Helper()
	ctx := database.AsSystem(context.Background())
	err := db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		_, _ = tx.Exec(txCtx, `DELETE FROM commerce.order_ratings WHERE customer_id = $1`, testCommerceUserID)
		_, _ = tx.Exec(txCtx, `DELETE FROM commerce.order_status_history WHERE changed_by_user_id = $1`, testCommerceUserID)
		_, _ = tx.Exec(txCtx, `DELETE FROM commerce.order_lines WHERE organization_id = $1`, testCommerceVendorID)
		_, _ = tx.Exec(txCtx, `DELETE FROM commerce.order_shipments WHERE vendor_org_id = $1`, testCommerceVendorID)
		_, _ = tx.Exec(txCtx, `DELETE FROM commerce.orders WHERE customer_user_id = $1`, testCommerceUserID)
		_, _ = tx.Exec(txCtx, `DELETE FROM commerce.wishlists WHERE user_id = $1`, testCommerceUserID)
		_, _ = tx.Exec(txCtx, `DELETE FROM commerce.quote_requests WHERE organization_id IN ($1, $2)`, testCommerceVendorID, testCommerceCustID)
		_, _ = tx.Exec(txCtx, `DELETE FROM commerce.cart_items WHERE cart_id IN (SELECT id FROM commerce.carts WHERE user_id = $1)`, testCommerceUserID)
		_, _ = tx.Exec(txCtx, `DELETE FROM commerce.carts WHERE user_id = $1`, testCommerceUserID)
		_, _ = tx.Exec(txCtx, `DELETE FROM catalog.product_variants WHERE id = $1`, testCommerceVarID)
		_, _ = tx.Exec(txCtx, `DELETE FROM catalog.products WHERE id = $1`, testCommerceProdID)
		_, _ = tx.Exec(txCtx, `DELETE FROM catalog.categories WHERE id = $1`, testCommerceCatID)
		_, _ = tx.Exec(txCtx, `DELETE FROM catalog.brands WHERE id = $1`, testCommerceBrandID)
		_, _ = tx.Exec(txCtx, `DELETE FROM org.organizations WHERE id IN ($1, $2)`, testCommerceVendorID, testCommerceCustID)
		_, _ = tx.Exec(txCtx, `DELETE FROM identity.users WHERE id = $1`, testCommerceUserID)

		// Setup prerequisite user, orgs, and product
		_, _ = tx.Exec(txCtx, `INSERT INTO identity.users (id, email, password_hash, name)
			VALUES ($1, 'commuser88390@example.com', 'x', '{"en":"Commerce User"}') ON CONFLICT DO NOTHING`, testCommerceUserID)
		_, _ = tx.Exec(txCtx, `INSERT INTO org.organizations (id, name)
			VALUES ($1, '{"en":"Commerce Vendor Org"}') ON CONFLICT DO NOTHING`, testCommerceVendorID)
		_, _ = tx.Exec(txCtx, `INSERT INTO org.organizations (id, name)
			VALUES ($1, '{"en":"Commerce Cust Org"}') ON CONFLICT DO NOTHING`, testCommerceCustID)
		_, _ = tx.Exec(txCtx, `INSERT INTO catalog.categories (id, name, slug)
			VALUES ($1, '{"en":"Comm Cat"}', 'comm-cat') ON CONFLICT DO NOTHING`, testCommerceCatID)
		_, _ = tx.Exec(txCtx, `INSERT INTO catalog.brands (id, name, slug)
			VALUES ($1, '{"en":"Comm Brand"}', 'comm-brand') ON CONFLICT DO NOTHING`, testCommerceBrandID)
		_, _ = tx.Exec(txCtx, `INSERT INTO catalog.products (id, organization_id, category_id, brand_id, name, slug, dosage_form)
			VALUES ($1, $2, $3, $4, '{"en":"Comm Prod"}', 'comm-prod', 'tablet') ON CONFLICT DO NOTHING`,
			testCommerceProdID, testCommerceVendorID, testCommerceCatID, testCommerceBrandID)
		_, _ = tx.Exec(txCtx, `INSERT INTO catalog.product_variants (id, organization_id, product_id, sku, price)
			VALUES ($1, $2, $3, 'COMM-SKU-1', 100.00) ON CONFLICT DO NOTHING`,
			testCommerceVarID, testCommerceVendorID, testCommerceProdID)
		return nil
	})
	if err != nil {
		t.Fatalf("reset fixtures failed: %v", err)
	}
}

func TestCommerceRepository(t *testing.T) {
	db := getTestDB(t)
	if db == nil {
		return
	}
	defer db.Close()

	resetCommerceFixtures(t, db)

	ctx := context.Background()
	repo := postgres.NewRepository(db)

	t.Run("Cart_Operations", func(t *testing.T) {
		cart, err := repo.GetOrCreateCart(ctx, testCommerceUserID)
		if err != nil {
			t.Fatalf("GetOrCreateCart failed: %v", err)
		}

		item := &commerce.CartItem{
			CartID:           cart.ID,
			ProductID:        testCommerceProdID,
			ProductVariantID: testCommerceVarID,
			Quantity:         2,
			UnitPrice:        money.MustParse("100.00"),
		}
		err = repo.AddToCartItem(ctx, cart.ID, item)
		if err != nil {
			t.Fatalf("AddToCartItem failed: %v", err)
		}

		cartWithItems, err := repo.GetCartWithItems(ctx, cart.ID)
		if err != nil {
			t.Fatalf("GetCartWithItems failed: %v", err)
		}
		if len(cartWithItems.Items) != 1 {
			t.Fatalf("expected 1 item, got %d", len(cartWithItems.Items))
		}

		err = repo.SetCartItemQuantity(ctx, cart.ID, testCommerceVarID, 5)
		if err != nil {
			t.Fatalf("SetCartItemQuantity failed: %v", err)
		}

		err = repo.RemoveCartItem(ctx, cart.ID, testCommerceVarID)
		if err != nil {
			t.Fatalf("RemoveCartItem failed: %v", err)
		}

		err = repo.ClearCart(ctx, cart.ID)
		if err != nil {
			t.Fatalf("ClearCart failed: %v", err)
		}
	})

	t.Run("Wishlist_Operations", func(t *testing.T) {
		err := repo.AddToWishlist(ctx, testCommerceUserID, testCommerceProdID)
		if err != nil {
			t.Fatalf("AddToWishlist failed: %v", err)
		}

		list, err := repo.ListWishlist(ctx, testCommerceUserID)
		if err != nil || len(list) == 0 {
			t.Fatalf("ListWishlist failed: %v", err)
		}

		err = repo.RemoveFromWishlist(ctx, testCommerceUserID, testCommerceProdID)
		if err != nil {
			t.Fatalf("RemoveFromWishlist failed: %v", err)
		}
	})

	t.Run("Quotes_CRUD", func(t *testing.T) {
		pID := testCommerceProdID
		qr := &commerce.QuoteRequest{
			OrganizationID:    testCommerceVendorID,
			CustomerOrgID:     testCommerceCustID,
			ProductID:         &pID,
			ProductName:       "Comm Prod",
			RequestedQuantity: 500,
			TargetUnitPrice:   money.MustParse("90.00"),
			Status:            commerce.QuotePending,
			BuyerNotes:        "Quote for 500 units",
		}
		err := repo.CreateQuoteRequest(ctx, qr)
		if err != nil {
			t.Fatalf("CreateQuoteRequest failed: %v", err)
		}
		if qr.ID == 0 {
			t.Fatal("expected generated quote request ID")
		}

		gotQuote, err := repo.GetQuoteRequestByID(ctx, qr.ID)
		if err != nil {
			t.Fatalf("GetQuoteRequestByID failed: %v", err)
		}
		if gotQuote.BuyerNotes != "Quote for 500 units" {
			t.Errorf("got %q, want 'Quote for 500 units'", gotQuote.BuyerNotes)
		}

		err = repo.UpdateQuoteStatus(ctx, qr.ID, commerce.QuoteQuoted, money.MustParse("95.00"), "Discount applied")
		if err != nil {
			t.Fatalf("UpdateQuoteStatus failed: %v", err)
		}

		list, err := repo.ListQuoteRequestsByOrg(ctx, testCommerceCustID, false, 10, 0)
		if err != nil || len(list) == 0 {
			t.Fatalf("ListQuoteRequestsByOrg failed: %v", err)
		}
	})
}
