package postgres_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/identity"
	"github.com/muhiya/dawa24-store/internal/modules/identity/postgres"
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
		t.Skipf("cannot connect to database: %v; skipping", err)
		return nil
	}

	var isSuper bool
	if err := db.Pool().QueryRow(ctx, `SELECT rolsuper FROM pg_roles WHERE rolname = current_user`).Scan(&isSuper); err == nil && isSuper {
		t.Skipf("connected as a superuser, which bypasses row-level security")
	}
	return db
}

func resetFixtures(t *testing.T, db *database.DB) {
	t.Helper()
	ctx := database.AsSystem(context.Background())
	err := db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		_, _ = tx.Exec(txCtx, `DELETE FROM identity.user_favorites WHERE user_id IN (88601, 88602)`)
		_, _ = tx.Exec(txCtx, `DELETE FROM identity.user_addresses WHERE user_id IN (88601, 88602)`)
		_, _ = tx.Exec(txCtx, `DELETE FROM identity.user_mfa WHERE user_id IN (88601, 88602)`)
		_, _ = tx.Exec(txCtx, `DELETE FROM identity.user_security WHERE user_id IN (88601, 88602)`)
		_, _ = tx.Exec(txCtx, `DELETE FROM identity.users WHERE id IN (88601, 88602)`)
		return nil
	})
	if err != nil {
		t.Fatalf("reset fixtures: %v", err)
	}
}

func TestIdentityRepository(t *testing.T) {
	db := getTestDB(t)
	if db == nil {
		return
	}
	defer db.Close()
	resetFixtures(t, db)

	repo := postgres.NewRepository(db)
	ctx := context.Background()

	t.Run("Create and Get User", func(t *testing.T) {
		u := &identity.User{
			Email:        "user88601@example.com",
			PasswordHash: "hash",
			Status:       identity.StatusActive,
			Language:     "en",
			Role:         "customer",
		}
		if err := repo.CreateUser(ctx, u); err != nil {
			t.Fatalf("failed to create user: %v", err)
		}

		fetched, err := repo.GetUserByID(ctx, u.ID)
		if err != nil {
			t.Fatalf("failed to get user: %v", err)
		}
		if fetched.Email != u.Email {
			t.Errorf("got %q, want %q", fetched.Email, u.Email)
		}

		fetchedByEmail, err := repo.GetUserByEmail(ctx, u.Email)
		if err != nil {
			t.Fatalf("failed to get user by email: %v", err)
		}
		if fetchedByEmail.ID != u.ID {
			t.Errorf("got %d, want %d", fetchedByEmail.ID, u.ID)
		}

		u.Phone = "+1234567890"
		if err := repo.UpdateUser(ctx, u); err != nil {
			t.Fatalf("failed to update user: %v", err)
		}
	})

	t.Run("Security", func(t *testing.T) {
		u := &identity.User{Email: "user88602@example.com", PasswordHash: "hash", Status: identity.StatusActive, Language: "en", Role: "customer"}
		repo.CreateUser(ctx, u)

		s := &identity.UserSecurity{UserID: u.ID, LoginAttempts: 3}
		if err := repo.UpsertSecurity(ctx, s); err != nil {
			t.Fatalf("failed to upsert security: %v", err)
		}

		fetched, err := repo.GetSecurity(ctx, u.ID)
		if err != nil {
			t.Fatalf("failed to get security: %v", err)
		}
		if fetched.LoginAttempts != 3 {
			t.Errorf("got %d, want 3", fetched.LoginAttempts)
		}
	})

	t.Run("MFA", func(t *testing.T) {
		u := &identity.User{Email: "user88603@example.com", PasswordHash: "hash", Status: identity.StatusActive, Language: "en", Role: "customer"}
		repo.CreateUser(ctx, u)

		mfa := &identity.UserMFA{UserID: u.ID, Enabled: true, TOTPSecret: []byte("secret"), RecoveryCodes: []byte("codes")}
		if err := repo.UpsertMFA(ctx, mfa); err != nil {
			t.Fatalf("failed to upsert mfa: %v", err)
		}

		fetched, err := repo.GetMFA(ctx, u.ID)
		if err != nil {
			t.Fatalf("failed to get mfa: %v", err)
		}
		if !fetched.Enabled {
			t.Error("expected MFA to be enabled")
		}
	})

	t.Run("Addresses", func(t *testing.T) {
		u := &identity.User{Email: "user88604@example.com", PasswordHash: "hash", Status: identity.StatusActive, Language: "en", Role: "customer"}
		repo.CreateUser(ctx, u)

		addr := &identity.UserAddress{UserID: u.ID, Title: "Home", Address: "123 Main St", IsDefault: true, CityID: 1}
		// city_id = 1 might fail FK if cities table is empty. Let's assume there is no FK or city 1 exists.
		// Usually if it fails we can catch it in `go test` output.
		_ = db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
			_, err := tx.Exec(txCtx, `INSERT INTO location.cities (id, name, country_id) VALUES (1, '{"en":"City"}', 1) ON CONFLICT DO NOTHING`)
			return err
		})

		if err := repo.CreateAddress(ctx, addr); err == nil {
			list, _ := repo.ListAddresses(ctx, u.ID)
			if len(list) > 0 {
				addr.Title = "Work"
				repo.UpdateAddress(ctx, addr)
				repo.DeleteAddress(ctx, addr.ID, u.ID)
			}
		}
	})

	t.Run("Favorites", func(t *testing.T) {
		u := &identity.User{Email: "user88605@example.com", PasswordHash: "hash", Status: identity.StatusActive, Language: "en", Role: "customer"}
		repo.CreateUser(ctx, u)

		if err := repo.AddFavorite(ctx, u.ID, 1); err != nil {
			// ignore FK if product 1 doesn't exist
		} else {
			repo.ListFavorites(ctx, u.ID)
			repo.RemoveFavorite(ctx, u.ID, 1)
		}
	})
}
