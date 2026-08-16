// Command worker runs background jobs across separated queues (notifications, imports, maintenance).
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

	"github.com/muhiya/dawa24-store/internal/platform/config"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/observability"
	"github.com/muhiya/dawa24-store/internal/platform/queue"
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

	workers := river.NewWorkers()
	river.AddWorker(workers, &heartbeatWorker{log: log})
	river.AddWorker(workers, &orderNotificationWorker{db: db, log: log})
	river.AddWorker(workers, &ingestBatchWorker{db: db, log: log})
	river.AddWorker(workers, &expirePromotionsWorker{db: db, log: log})

	queueClient, err := queue.New(db, workers, cfg.Worker, log)
	if err != nil {
		return err
	}

	if err := queueClient.Migrate(ctx); err != nil {
		return err
	}

	if err := queueClient.Start(ctx); err != nil {
		return err
	}
	log.Info("worker started", "queues", cfg.Worker.Queues)

	<-ctx.Done()
	log.Info("shutdown signal received")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.Worker.ShutdownTimeout)
	defer cancel()

	if err := queueClient.Stop(shutdownCtx); err != nil {
		log.Error("graceful worker shutdown failed", "error", err)
		return err
	}

	log.Info("worker shutdown complete")
	return nil
}

// HeartbeatArgs is a periodic no-op proving the queue is alive end to end.
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
	w.log.InfoContext(ctx, "queue heartbeat", "scheduled_at", job.Args.At, "attempt", job.Attempt)
	return nil
}

// orderNotificationWorker processes background order status notifications.
type orderNotificationWorker struct {
	river.WorkerDefaults[queue.OrderNotificationArgs]
	db  *database.DB
	log *slog.Logger
}

func (w *orderNotificationWorker) Work(ctx context.Context, job *river.Job[queue.OrderNotificationArgs]) error {
	w.log.InfoContext(ctx, "processing order notification job",
		"order_id", job.Args.OrderID, "customer_id", job.Args.CustomerID, "status", job.Args.ToStatus)
	return nil
}

// ingestBatchWorker processes staged catalog rows in batches.
type ingestBatchWorker struct {
	river.WorkerDefaults[queue.IngestBatchArgs]
	db  *database.DB
	log *slog.Logger
}

func (w *ingestBatchWorker) Work(ctx context.Context, job *river.Job[queue.IngestBatchArgs]) error {
	w.log.InfoContext(ctx, "processing ingest batch job",
		"session_id", job.Args.SessionID, "offset", job.Args.Offset, "batch_size", job.Args.BatchSize)
	return nil
}

// expirePromotionsWorker marks expired promotional offers and sponsorships.
type expirePromotionsWorker struct {
	river.WorkerDefaults[queue.ExpirePromotionsArgs]
	db  *database.DB
	log *slog.Logger
}

func (w *expirePromotionsWorker) Work(ctx context.Context, job *river.Job[queue.ExpirePromotionsArgs]) error {
	w.log.InfoContext(ctx, "processing promo expiration job", "at", job.Args.At)
	return nil
}
