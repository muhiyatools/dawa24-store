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

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/muhiya/dawa24-store/internal/modules/billing"
	billingPostgres "github.com/muhiya/dawa24-store/internal/modules/billing/postgres"
	catalogJobs "github.com/muhiya/dawa24-store/internal/modules/catalog/jobs"
	"github.com/muhiya/dawa24-store/internal/platform/config"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/observability"
	"github.com/muhiya/dawa24-store/internal/platform/queue"
	"github.com/muhiya/dawa24-store/internal/shared/arabic"
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
	river.AddWorker(workers, catalogJobs.NewProductReindexWorker(db, log))

	// Smart ordering (specs/001-smart-ordering-system). Registered last because
	// it is the only worker with an optional AI dependency; the rest run
	// regardless of Gateway state.
	registerSmartOrderWorker(workers, db, nil, log)

	queueClient, err := queue.New(db, workers, cfg.Worker, log)
	if err != nil {
		return err
	}

	if err := queueClient.Migrate(ctx); err != nil {
		return err
	}

	// Daily Subscription Renewal Scheduler (Runs once every 24 hours)
	go func() {
		billSvc := billing.NewService(billingPostgres.NewRepository(db), log)
		select {
		case <-ctx.Done():
			return
		case <-time.After(10 * time.Second):
			if r, f, err := billSvc.ProcessDueSubscriptionRenewals(ctx); err != nil {
				log.Error("initial subscription renewal pass failed", "error", err)
			} else if r > 0 || f > 0 {
				log.Info("initial subscription renewal pass completed", "renewed", r, "failed", f)
			}
		}

		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if r, f, err := billSvc.ProcessDueSubscriptionRenewals(ctx); err != nil {
					log.Error("daily subscription renewal pass failed", "error", err)
				} else {
					log.Info("daily subscription renewal pass completed", "renewed", r, "failed", f)
				}
			}
		}
	}()

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
	title := fmt.Sprintf("تحديث حالة الطلب #%d", job.Args.OrderID)
	body := fmt.Sprintf("تم تحديث حالة طلبك إلى: %s", job.Args.ToStatus)

	err := w.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			INSERT INTO notifications.logs (user_id, channel, event_type, recipient, title, body, status, sent_at)
			VALUES ($1, 'in_app', 'order_status', $2, $3, $4, 'delivered', now());
		`
		_, err := tx.Exec(txCtx, query, job.Args.CustomerID, fmt.Sprintf("user_%d", job.Args.CustomerID), title, body)
		return err
	})
	if err != nil {
		w.log.ErrorContext(ctx, "failed to record order notification", "order_id", job.Args.OrderID, "error", err)
		return err
	}
	w.log.InfoContext(ctx, "order notification recorded", "order_id", job.Args.OrderID, "customer_id", job.Args.CustomerID)
	return nil
}

// ingestBatchWorker processes staged catalog rows in batches.
type ingestBatchWorker struct {
	river.WorkerDefaults[queue.IngestBatchArgs]
	db  *database.DB
	log *slog.Logger
}

func (w *ingestBatchWorker) Work(ctx context.Context, job *river.Job[queue.IngestBatchArgs]) error {
	return w.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		queryRows := `
			SELECT id, normalized_name FROM ingest.import_rows
			WHERE session_id = $1 AND status = 'pending'
			ORDER BY row_number ASC
			LIMIT $2 OFFSET $3;
		`
		rows, err := tx.Query(txCtx, queryRows, job.Args.SessionID, job.Args.BatchSize, job.Args.Offset)
		if err != nil {
			return err
		}
		defer rows.Close()

		type staged struct {
			id   int64
			name string
		}
		var batch []staged
		for rows.Next() {
			var s staged
			if err := rows.Scan(&s.id, &s.name); err != nil {
				return err
			}
			batch = append(batch, s)
		}
		if len(batch) == 0 {
			return nil
		}

		catRows, err := tx.Query(txCtx, `SELECT id, name->>'ar' FROM catalog.products WHERE deleted_at IS NULL LIMIT 500;`)
		if err != nil {
			return err
		}
		defer catRows.Close()

		type candidate struct {
			id   int64
			name string
		}
		var candidates []candidate
		for catRows.Next() {
			var c candidate
			var name *string
			if err := catRows.Scan(&c.id, &name); err != nil {
				return err
			}
			if name != nil {
				c.name = *name
				candidates = append(candidates, c)
			}
		}

		for _, item := range batch {
			var bestID *int64
			var bestScore float64
			for _, cand := range candidates {
				score := arabic.Similarity(item.name, arabic.Normalize(cand.name))
				if score > bestScore {
					bestScore = score
					cID := cand.id
					bestID = &cID
				}
			}

			status := "unmatched"
			if bestScore >= 0.85 {
				status = "matched"
			}

			_, err = tx.Exec(txCtx, `
				UPDATE ingest.import_rows
				SET matched_product_id = $1, match_confidence = $2, status = $3, updated_at = now()
				WHERE id = $4;
			`, bestID, bestScore, status, item.id)
			if err != nil {
				return err
			}
		}

		_, err = tx.Exec(txCtx, `
			UPDATE ingest.import_sessions
			SET processed_rows = processed_rows + $1, updated_at = now()
			WHERE id = $2;
		`, len(batch), job.Args.SessionID)
		return err
	})
}

// expirePromotionsWorker marks expired promotional offers and sponsorships.
type expirePromotionsWorker struct {
	river.WorkerDefaults[queue.ExpirePromotionsArgs]
	db  *database.DB
	log *slog.Logger
}

func (w *expirePromotionsWorker) Work(ctx context.Context, job *river.Job[queue.ExpirePromotionsArgs]) error {
	return w.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		_, err1 := tx.Exec(txCtx, `UPDATE promo.offers SET is_active = false, updated_at = now() WHERE is_active = true AND expires_at < now();`)
		_, err2 := tx.Exec(txCtx, `UPDATE promo.offer_sponsorships SET status = 'expired' WHERE status = 'active' AND expires_at < now();`)
		_, err3 := tx.Exec(txCtx, `UPDATE promo.ads SET is_active = false, updated_at = now() WHERE is_active = true AND expires_at < now();`)
		if err1 != nil {
			return err1
		}
		if err2 != nil {
			return err2
		}
		return err3
	})
}
