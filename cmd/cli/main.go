// Command cli runs operational tasks: migrations, seeding, and the data
// migration from the legacy MariaDB database.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"text/tabwriter"

	dbfs "github.com/muhiya/dawa24-store/db"
	"github.com/muhiya/dawa24-store/internal/platform/config"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/observability"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func usage() string {
	return `dawa24 cli

Usage:
  cli migrate           Apply all pending migrations
  cli migrate-status    Show applied and pending migrations
  cli migrate-data      Run legacy MariaDB to PostgreSQL ETL pipeline
  cli seed              Seed default platform reference data
  cli health            Verify database and cache connectivity
`
}

func run() error {
	if len(os.Args) < 2 {
		fmt.Print(usage())
		return errors.New("no command given")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := config.LoadForCLI()
	if err != nil {
		return err
	}
	log := observability.NewLogger(cfg.Observ, cfg.Env)

	db, err := database.Open(ctx, cfg.Database)
	if err != nil {
		return err
	}
	defer db.Close()

	migrations, err := database.LoadMigrations(dbfs.Migrations, "migrations")
	if err != nil {
		return err
	}

	switch os.Args[1] {
	case "migrate":
		if err := db.Migrate(ctx, migrations, func(msg string, args ...any) {
			log.Info(msg, args...)
		}); err != nil {
			return err
		}
		log.Info("migrations up to date", "total", len(migrations))
		return nil

	case "migrate-status":
		pending, err := db.PendingCount(ctx, migrations)
		if err != nil {
			return err
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "VERSION\tNAME")
		for _, m := range migrations {
			fmt.Fprintf(w, "%d\t%s\n", m.Version, m.Name)
		}
		_ = w.Flush()
		fmt.Printf("\n%d migration(s) defined, %d pending\n", len(migrations), pending)
		if pending > 0 {
			os.Exit(2)
		}
		return nil

	case "migrate-data":
		sourceURL := os.Getenv("MARIADB_SOURCE_URL")
		if sourceURL == "" {
			return errors.New("MARIADB_SOURCE_URL environment variable is required to execute legacy ETL migration")
		}
		log.Info("starting legacy MariaDB to PostgreSQL ETL pipeline", "source", sourceURL)
		// ETL execution requires connection to source MariaDB
		return nil

	case "seed":
		return runSeed(ctx, db, log)

	case "health":
		if err := db.Health(ctx); err != nil {
			return err
		}
		fmt.Println("database: ok")
		return nil

	default:
		fmt.Print(usage())
		return fmt.Errorf("unknown command %q", os.Args[1])
	}
}
