// Command server runs the Dawa24 Store HTTP process.
//
// Web and worker ship in the same image with different entrypoints. One build,
// one set of dependencies, no drift between the code serving a request and the
// code processing the job that request enqueued.
//
// Startup order matters here: configuration is validated and the HTTP listener
// opens FIRST, then PostgreSQL and Redis connect in the background. See deps.go
// for why.
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
	"github.com/muhiya/dawa24-store/internal/platform/config"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/gateway"
	"github.com/muhiya/dawa24-store/internal/platform/httpx"
	"github.com/muhiya/dawa24-store/internal/platform/observability"
	"github.com/muhiya/dawa24-store/web"
)

func main() {
	if err := run(); err != nil {
		// Configuration failures happen before the logger exists, so stderr is
		// the right destination here and nowhere else.
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// The one thing that still aborts at boot. A missing SESSION_SECRET or a
	// malformed DATABASE_URL is a deployment mistake; retrying cannot fix it,
	// and starting anyway would mean serving requests we cannot secure.
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	log := observability.NewLogger(cfg.Observ, cfg.Env)
	slog.SetDefault(log)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log.Info("starting dawa24-store",
		"env", cfg.Env,
		"port", cfg.HTTP.Port,
		"version", buildVersion,
		"commit", buildCommit)

	migrations, err := database.LoadMigrations(dbfs.Migrations, "migrations")
	if err != nil {
		return err
	}

	deps := newDependencies()
	defer deps.close()
	deps.connect(ctx, cfg, log)

	// The AI Gateway is optional by construction. Disabled or unreachable, every
	// capability serves its deterministic fallback and the marketplace keeps
	// trading.
	var ai gateway.Client
	if cfg.Gateway.Enabled {
		ai = gateway.New(cfg.Gateway, log)
		log.Info("ai gateway enabled",
			"base_url", cfg.Gateway.BaseURL, "client_app", cfg.Gateway.ClientApp)
	} else {
		ai = gateway.NewDisabled()
		log.Info("ai gateway disabled; capabilities will use deterministic fallbacks")
	}

	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.HTTP.Port),
		Handler:           newRouter(cfg, log, deps, ai, migrations),
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

	// Stop accepting, let in-flight requests finish. A checkout half-written
	// when the container stops is a support ticket.
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
	deps *dependencies,
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
		deps:       deps,
		ai:         ai,
		migrations: migrations,
		env:        string(cfg.Env),
		log:        log,
	}

	// Liveness: is the process running? Dependency-free on purpose, so a
	// database blip does not make the orchestrator restart a healthy container
	// and turn a brief outage into a crash loop.
	r.Get("/health", health.live)

	// Readiness: should this instance receive traffic? Checks dependencies and
	// refuses while migrations are pending.
	r.Get("/ready", health.ready)

	// Operator view: everything readiness reports, plus the Gateway, which never
	// affects the verdict.
	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/status", health.status)
	})

	// Mount all domain module endpoints
	mountModuleRoutes(r, cfg, log, deps, ai)

	// Static assets
	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(web.StaticFS())))

	// Web UI
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		htmlBytes, err := web.IndexHTML()
		if err != nil {
			health.root(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(htmlBytes)
	})

	return r
}
