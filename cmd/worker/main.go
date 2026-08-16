// Command worker runs background jobs.
//
// It ships in the same image as the server and shares its configuration, so a
// job and the request that enqueued it always run the same code.
//
// Queues are separated by workload (see config.Worker.Queues) rather than
// pooled together. A supplier uploading 500,000 SKUs must not be able to starve
// order confirmations or notification delivery — which is exactly what a single
// shared queue does under load.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"

	"github.com/muhiya/dawa24-store/internal/platform/config"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/observability"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	log := observability.NewLogger(cfg.Observ, cfg.Env)
	slog.SetDefault(log)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	db, err := database.Open(ctx, cfg.Database)
	if err != nil {
		return err
	}
	defer db.Close()

	driver := riverpgxv5.New(db.Pool())

	// River owns its own schema. Migrating it here rather than in the
	// application's migration set keeps the two independent: upgrading River
	// does not require authoring a Dawa24 migration.
	migrator, err := rivermigrate.New(driver, nil)
	if err != nil {
		return fmt.Errorf("worker: river migrator: %w", err)
	}
	if _, err := migrator.Migrate(ctx, rivermigrate.DirectionUp, nil); err != nil {
		return fmt.Errorf("worker: river migrate: %w", err)
	}

	workers := river.NewWorkers()
	river.AddWorker(workers, &heartbeatWorker{log: log})

	queues := make(map[string]river.QueueConfig, len(cfg.Worker.Queues))
	for name, count := range cfg.Worker.Queues {
		queues[name] = river.QueueConfig{MaxWorkers: count}
	}

	client, err := river.NewClient(driver, &river.Config{
		Queues:  queues,
		Workers: workers,
		Logger:  log,
		// A job that outlives this is stuck, not slow. Imports are chunked, so
		// no single job should approach the ceiling.
		JobTimeout: 30 * time.Minute,
	})
	if err != nil {
		return fmt.Errorf("worker: river client: %w", err)
	}

	if err := client.Start(ctx); err != nil {
		return fmt.Errorf("worker: start: %w", err)
	}
	log.Info("worker started", "queues", cfg.Worker.Queues)

	<-ctx.Done()
	log.Info("shutdown signal received")

	// Stop lets running jobs finish. A half-applied stock movement is worse
	// than a slightly slower deploy.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.Worker.ShutdownTimeout)
	defer cancel()

	if err := client.Stop(shutdownCtx); err != nil {
		log.Error("graceful worker shutdown failed", "error", err)
		return err
	}

	log.Info("worker shutdown complete")
	return nil
}

// HeartbeatArgs is a periodic no-op proving the queue is alive end to end.
//
// It exists so that "are jobs actually being processed?" is answerable by
// monitoring rather than by inference from an empty queue — an idle queue and a
// broken worker look identical from the outside.
type HeartbeatArgs struct {
	At time.Time `json:"at"`
}

func (HeartbeatArgs) Kind() string { return "maintenance.heartbeat" }

func (HeartbeatArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: "maintenance"}
}

type heartbeatWorker struct {
	river.WorkerDefaults[HeartbeatArgs]
	log *slog.Logger
}

func (w *heartbeatWorker) Work(ctx context.Context, job *river.Job[HeartbeatArgs]) error {
	w.log.InfoContext(ctx, "queue heartbeat",
		"scheduled_at", job.Args.At, "attempt", job.Attempt)
	return nil
}
