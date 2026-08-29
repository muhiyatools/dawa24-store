package test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/smartorder"
	"github.com/muhiya/dawa24-store/internal/modules/smartorder/pipeline"
	"github.com/muhiya/dawa24-store/internal/modules/smartorder/postgres"
	"github.com/muhiya/dawa24-store/internal/platform/config"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

func TestLiveBenchmarkRun42(t *testing.T) {
	connStr := os.Getenv("DATABASE_URL")
	if connStr == "" {
		connStr = "postgres://postgres:RBSW2NW9-dy4d-63ZLK0DC@postgres-u74003.vm.elestio.app:5432/dawa24_store?sslmode=require"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	db, err := database.Open(ctx, config.Database{URL: connStr, MaxConns: 5})
	if err != nil {
		t.Skipf("cannot connect to DB: %v", err)
	}
	defer db.Close()

	repo := postgres.New(db)

	// Load lines from run 42
	lines, _, err := repo.ListLines(ctx, 42, smartorder.LineFilter{All: true})
	if err != nil {
		t.Fatalf("failed to list lines: %v", err)
	}
	t.Logf("Loaded %d lines from run 42", len(lines))

	for _, l := range lines {
		l.MatchedProductID = nil
		l.MatchMethod = smartorder.MethodNone
		l.MatchConfidence = 0.0
	}

	cfg := &smartorder.Config{
		RunID:             42,
		OrganizationID:    50,
		UseSavingProducts: true,
		UseAIMatching:     false,
		MatchLanguage:     "ar",
	}

	pipeline.Normalize(lines)

	start := time.Now()
	resolver := pipeline.NewResolver(repo, cfg)
	if err := resolver.Resolve(ctx, lines); err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	dur := time.Since(start)

	byMethod := make(map[smartorder.MatchMethod]int)
	resolvedCount := 0
	for _, l := range lines {
		if l.Matched() {
			resolvedCount++
			byMethod[l.MatchMethod]++
		}
	}

	t.Logf("\n=== DETERMINISTIC MATCHING RESULTS ON RUN 42 ===")
	t.Logf("Total lines: %d", len(lines))
	t.Logf("Resolved deterministically: %d (%.2f%%) in %v",
		resolvedCount, float64(resolvedCount)/float64(len(lines))*100, dur)
	t.Logf("Original run 42 resolved deterministically: 57 (0.64%%)")
	for method, count := range byMethod {
		t.Logf("  - %-20s: %d", method, count)
	}
}
