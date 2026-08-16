package postgres_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	dbfs "github.com/muhiya/dawa24-store/db"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/modules/org/postgres"
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

const testUserID int64 = 88590

func resetFixtures(t *testing.T, db *database.DB) {
	t.Helper()
	ctx := database.AsSystem(context.Background())
	err := db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		_, _ = tx.Exec(txCtx, `DELETE FROM org.organization_policies WHERE organization_id IN (SELECT id FROM org.organizations WHERE legal_name LIKE 'Test Org %')`)
		_, _ = tx.Exec(txCtx, `DELETE FROM org.organization_followers WHERE user_id = $1`, testUserID)
		_, _ = tx.Exec(txCtx, `DELETE FROM org.organization_reviews WHERE user_id = $1`, testUserID)
		_, _ = tx.Exec(txCtx, `DELETE FROM org.members WHERE user_id = $1`, testUserID)
		_, _ = tx.Exec(txCtx, `DELETE FROM org.branches WHERE organization_id IN (SELECT id FROM org.organizations WHERE legal_name LIKE 'Test Org %')`)
		_, _ = tx.Exec(txCtx, `DELETE FROM org.organizations WHERE legal_name LIKE 'Test Org %'`)
		_, _ = tx.Exec(txCtx, `DELETE FROM identity.users WHERE id = $1`, testUserID)

		// Create user for member, follower, review checks
		_, _ = tx.Exec(txCtx, `INSERT INTO identity.users (id, email, password_hash, name, role)
			VALUES ($1, 'user88590@example.com', 'x', '{"en":"User"}', 'customer') ON CONFLICT DO NOTHING`, testUserID)
		return nil
	})
	if err != nil {
		t.Fatalf("reset fixtures: %v", err)
	}
}

func TestOrgRepository(t *testing.T) {
	db := getTestDB(t)
	if db == nil {
		return
	}
	defer db.Close()
	resetFixtures(t, db)

	repo := postgres.NewRepository(db)
	ctx := context.Background()

	var orgID int64
	var branchID int64

	t.Run("Create and Get Organization", func(t *testing.T) {
		o := &org.Organization{
			LegalName:          "Test Org Alpha",
			CommercialRegister: "CR-ALPHA-01",
			Type:               org.TypePharmacy,
			Status:             org.StatusPending,
			CreditLimit:        money.FromMinor(500000),
			PaymentTermsDays:   30,
		}
		err := repo.CreateOrganization(ctx, o)
		if err != nil {
			t.Fatalf("failed to create org: %v", err)
		}
		if o.ID == 0 {
			t.Fatalf("expected non-zero org ID")
		}
		orgID = o.ID

		fetched, err := repo.GetOrganizationByID(ctx, orgID)
		if err != nil {
			t.Fatalf("failed to get org: %v", err)
		}
		if fetched.LegalName != o.LegalName {
			t.Errorf("got %q, want %q", fetched.LegalName, o.LegalName)
		}
		if fetched.CreditLimit.Minor() != 500000 {
			t.Errorf("got %d, want 500000", fetched.CreditLimit.Minor())
		}

		err = repo.UpdateOrganizationStatus(ctx, orgID, org.StatusApproved)
		if err != nil {
			t.Fatalf("failed to update status: %v", err)
		}

		o.LegalName = "Test Org Alpha Updated"
		err = repo.UpdateOrganization(ctx, o)
		if err != nil {
			t.Fatalf("failed to update org: %v", err)
		}

		list, err := repo.ListOrganizations(ctx, nil, nil, 10, 0)
		if err != nil || len(list) == 0 {
			t.Fatalf("failed to list orgs: %v", err)
		}
	})

	t.Run("Branches", func(t *testing.T) {
		b := &org.Branch{
			OrganizationID: orgID,
			Name:           nil,
			Code:           "BR-01",
			Address:        "123 Nile St",
			IsMain:         true,
		}
		if err := repo.CreateBranch(ctx, b); err != nil {
			t.Fatalf("failed to create branch: %v", err)
		}
		if b.ID == 0 {
			t.Fatal("expected generated branch ID")
		}
		branchID = b.ID

		fetched, err := repo.GetBranchByID(ctx, branchID)
		if err != nil {
			t.Fatalf("failed to get branch: %v", err)
		}
		if fetched.Code != "BR-01" {
			t.Errorf("got %q, want BR-01", fetched.Code)
		}

		branches, err := repo.ListBranchesByOrg(ctx, orgID)
		if err != nil || len(branches) == 0 {
			t.Fatalf("failed to list branches: %v", err)
		}

		b.Code = "BR-01-UPDATED"
		if err := repo.UpdateBranch(ctx, b); err != nil {
			t.Fatalf("failed to update branch: %v", err)
		}

		if err := repo.UnsetMainBranches(ctx, orgID); err != nil {
			t.Fatalf("failed to unset main branches: %v", err)
		}

		if err := repo.DeleteBranch(ctx, branchID, orgID); err != nil {
			t.Fatalf("failed to delete branch: %v", err)
		}
	})

	t.Run("Members", func(t *testing.T) {
		m := &org.Member{
			OrganizationID: orgID,
			UserID:         testUserID,
			RoleID:         1,
			IsActive:       true,
		}
		if err := repo.AddMember(ctx, m); err != nil {
			t.Fatalf("failed to add member: %v", err)
		}

		members, err := repo.ListMembersByOrg(ctx, orgID)
		if err != nil || len(members) == 0 {
			t.Fatalf("failed to list members: %v", err)
		}

		if err := repo.UpdateMemberRole(ctx, orgID, testUserID, "member"); err != nil {
			t.Fatalf("failed to update member role: %v", err)
		}

		if err := repo.RemoveMember(ctx, orgID, testUserID); err != nil {
			t.Fatalf("failed to remove member: %v", err)
		}
	})

	t.Run("Followers and Reviews", func(t *testing.T) {
		following, err := repo.ToggleFollower(ctx, orgID, testUserID)
		if err != nil {
			t.Fatalf("failed to toggle follower: %v", err)
		}
		if !following {
			t.Error("expected to be following")
		}

		isF, err := repo.IsFollowing(ctx, orgID, testUserID)
		if err != nil || !isF {
			t.Fatalf("expected isFollowing to be true: %v", err)
		}

		rev := &org.Review{
			OrganizationID: orgID,
			UserID:         testUserID,
			Rating:         5,
			ReviewText:     "Great service",
		}
		if err := repo.AddReview(ctx, rev); err != nil {
			t.Fatalf("failed to add review: %v", err)
		}

		reviews, err := repo.ListReviewsByOrg(ctx, orgID, 10, 0)
		if err != nil || len(reviews) == 0 {
			t.Fatalf("failed to list reviews: %v", err)
		}
	})

	t.Run("Policies", func(t *testing.T) {
		p := &org.Policy{
			OrganizationID: orgID,
			PolicyType:     "return_policy",
			Title:          "Return Policy",
			Content:        "14 days return policy for sealed items",
		}
		if err := repo.CreatePolicy(ctx, p); err != nil {
			t.Fatalf("failed to create policy: %v", err)
		}

		policies, err := repo.ListPoliciesByOrg(ctx, orgID)
		if err != nil || len(policies) == 0 {
			t.Fatalf("failed to list policies: %v", err)
		}
	})
}
