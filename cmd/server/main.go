// Command server runs the Dawa24 Store HTTP process.
//
// Web and worker ship in the same image with different entrypoints. One build,
// one set of dependencies, no drift between the code serving a request and the
// code processing the job that request enqueued.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"

	dbfs "github.com/muhiya/dawa24-store/db"
	"github.com/muhiya/dawa24-store/internal/platform/cache"
	"github.com/muhiya/dawa24-store/internal/platform/config"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/gateway"
	"github.com/muhiya/dawa24-store/internal/platform/httpx"
	"github.com/muhiya/dawa24-store/internal/platform/observability"
)

func main() {
	if err := run(); err != nil {
		// Config and dependency failures happen before the logger exists, so
		// this is the one place stderr is the right destination.
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

	// Signals are handled from the very start: a container killed during
	// dependency setup should exit promptly rather than hold the deploy open.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log.Info("starting dawa24-store",
		"env", cfg.Env, "port", cfg.HTTP.Port, "app", cfg.AppName)

	db, err := database.Open(ctx, cfg.Database)
	if err != nil {
		return err
	}
	defer db.Close()
	log.Info("database connected")

	rdb, err := cache.Open(ctx, cfg.Redis, cfg.Env)
	if err != nil {
		return err
	}
	defer func() { _ = rdb.Close() }()
	log.Info("redis connected")

	migrations, err := database.LoadMigrations(dbfs.Migrations, "migrations")
	if err != nil {
		return err
	}

	// The AI Gateway is optional by construction. When it is disabled or
	// unreachable, every capability serves its deterministic fallback and the
	// marketplace keeps trading.
	var ai gateway.Client
	if cfg.Gateway.Enabled {
		ai = gateway.New(cfg.Gateway, log)
		log.Info("ai gateway enabled", "base_url", cfg.Gateway.BaseURL, "client_app", cfg.Gateway.ClientApp)
	} else {
		ai = gateway.NewDisabled()
		log.Warn("ai gateway disabled; all AI capabilities will use deterministic fallbacks")
	}

	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.HTTP.Port),
		Handler:           newRouter(cfg, log, db, rdb, ai, migrations),
		ReadTimeout:       cfg.HTTP.ReadTimeout,
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      cfg.HTTP.WriteTimeout,
		IdleTimeout:       cfg.HTTP.IdleTimeout,
		ErrorLog:          slog.NewLogLogger(log.Handler(), slog.LevelError),
	}

	errc := make(chan error, 1)
	go func() {
		log.Info("http server listening", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errc <- err
		}
	}()

	select {
	case err := <-errc:
		return fmt.Errorf("http server: %w", err)
	case <-ctx.Done():
		log.Info("shutdown signal received")
	}

	// Graceful shutdown: stop accepting, let in-flight requests finish. A
	// checkout half-written when the container stops is a support ticket.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.HTTP.ShutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error("graceful shutdown failed; forcing close", "error", err)
		_ = srv.Close()
		return err
	}

	log.Info("shutdown complete")
	return nil
}

func newRouter(
	cfg *config.Config,
	log *slog.Logger,
	db *database.DB,
	rdb *cache.Cache,
	ai gateway.Client,
	migrations []database.Migration,
) http.Handler {
	r := chi.NewRouter()

	r.Use(httpx.RequestID)
	r.Use(httpx.Recover(log))
	r.Use(httpx.Logger(log))
	r.Use(httpx.SecurityHeaders)
	r.Use(httpx.Locale)

	health := &healthHandler{
		db: db, cache: rdb, ai: ai,
		migrations: migrations,
		env:        string(cfg.Env),
		log:        log,
	}

	// Liveness: is the process running? Deliberately dependency-free, so a
	// database blip does not cause the orchestrator to restart a healthy pod
	// and turn a brief outage into a crash loop.
	r.Get("/health", health.live)

	// Readiness: should this instance receive traffic? Checks dependencies and
	// refuses traffic when migrations are pending.
	r.Get("/ready", health.ready)

	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/status", health.status)
	})

	return r
}
