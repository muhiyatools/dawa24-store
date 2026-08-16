package postgres

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	dbfs "github.com/muhiya/dawa24-store/db"
	platformadmin "github.com/muhiya/dawa24-store/internal/modules/platform_admin"
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

func resetFixtures(t *testing.T, db *database.DB) {
	t.Helper()
	ctx := database.AsSystem(context.Background())
	err := db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(txCtx, `DELETE FROM platform_admin.contact_messages WHERE email = 'contact-test@example.com'`); err != nil {
			return fmt.Errorf("delete contact_messages: %w", err)
		}
		if _, err := tx.Exec(txCtx, `DELETE FROM platform_admin.system_settings WHERE key = 'test_maintenance_mode'`); err != nil {
			return fmt.Errorf("delete system_settings: %w", err)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("resetFixtures: %v", err)
	}
}

func TestPlatformAdminRepository(t *testing.T) {
	db := getTestDB(t)
	resetFixtures(t, db)

	repo := NewRepository(db)
	ctx := context.Background()

	t.Run("Set and Get System Setting", func(t *testing.T) {
		setting := &platformadmin.SystemSetting{
			Key:         "test_maintenance_mode",
			Value:       map[string]any{"enabled": false, "message": "All systems operational"},
			Description: "Test maintenance flag",
			IsPublic:    true,
		}

		if err := repo.SetSetting(ctx, setting); err != nil {
			t.Fatalf("SetSetting failed: %v", err)
		}

		got, err := repo.GetSetting(ctx, "test_maintenance_mode")
		if err != nil {
			t.Fatalf("GetSetting failed: %v", err)
		}
		if got.Key != "test_maintenance_mode" {
			t.Errorf("got key %q, want test_maintenance_mode", got.Key)
		}

		list, err := repo.ListPublicSettings(ctx)
		if err != nil {
			t.Fatalf("ListPublicSettings failed: %v", err)
		}
		if len(list) == 0 {
			t.Fatal("expected at least one public setting")
		}
	})

	t.Run("List Master Data (Countries, Currencies, Languages)", func(t *testing.T) {
		countries, err := repo.ListCountries(ctx)
		if err != nil {
			t.Fatalf("ListCountries failed: %v", err)
		}
		if len(countries) > 0 {
			cities, err := repo.ListCities(ctx, countries[0].ID)
			if err != nil {
				t.Fatalf("ListCities failed: %v", err)
			}
			_ = cities
		}

		currencies, err := repo.ListCurrencies(ctx)
		if err != nil {
			t.Fatalf("ListCurrencies failed: %v", err)
		}
		_ = currencies

		languages, err := repo.ListLanguages(ctx)
		if err != nil {
			t.Fatalf("ListLanguages failed: %v", err)
		}
		_ = languages
	})

	t.Run("Create and List Contact Messages", func(t *testing.T) {
		msg := &platformadmin.ContactMessage{
			Name:    "Test User",
			Email:   "contact-test@example.com",
			Phone:   "+201000000000",
			Subject: "Inquiry",
			Message: "Need help with onboarding",
			Status:  "unread",
		}

		if err := repo.CreateContactMessage(ctx, msg); err != nil {
			t.Fatalf("CreateContactMessage failed: %v", err)
		}
		if msg.ID <= 0 {
			t.Fatalf("expected positive message ID, got %d", msg.ID)
		}

		list, err := repo.ListContactMessages(ctx, "unread", 10, 0)
		if err != nil {
			t.Fatalf("ListContactMessages failed: %v", err)
		}
		if len(list) == 0 {
			t.Fatal("expected at least one contact message in list")
		}
	})
}
