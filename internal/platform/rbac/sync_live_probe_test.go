package rbac

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// TestLiveSyncProbe replays rbac.Sync's statements against a real database and
// ALWAYS rolls back, to find which one a deployment is failing on.
//
// Skipped unless RBAC_PROBE_URL is set. Temporary diagnostic.
func TestLiveSyncProbe(t *testing.T) {
	url := os.Getenv("RBAC_PROBE_URL")
	if url == "" {
		t.Skip("RBAC_PROBE_URL not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.Close(ctx)

	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
		t.Log("rolled back — database unchanged")
	}()

	c := Default()

	if err := syncPermissions(ctx, tx, c); err != nil {
		t.Fatalf("STEP syncPermissions: %v", err)
	}
	t.Log("ok  syncPermissions")

	if err := syncPlatformRoles(ctx, tx, c); err != nil {
		t.Fatalf("STEP syncPlatformRoles: %v", err)
	}
	t.Log("ok  syncPlatformRoles")

	if _, err := tx.Exec(ctx, `SELECT identity.bump_rbac_version('platform');`); err != nil {
		t.Fatalf("STEP bump_rbac_version: %v", err)
	}
	t.Log("ok  bump_rbac_version")
}
