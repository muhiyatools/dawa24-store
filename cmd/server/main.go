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
	chimw "github.com/go-chi/chi/v5/middleware"

	dbfs "github.com/muhiya/dawa24-store/db"
	platformadmin "github.com/muhiya/dawa24-store/internal/modules/platform_admin"
	platformadminPostgres "github.com/muhiya/dawa24-store/internal/modules/platform_admin/postgres"
	"github.com/muhiya/dawa24-store/internal/platform/config"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/gateway"
	"github.com/muhiya/dawa24-store/internal/platform/httpx"
	"github.com/muhiya/dawa24-store/internal/platform/observability"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
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
	adminRepo := platformadminPostgres.NewRepository(deps.Handle())
	adminSvc := platformadmin.NewService(adminRepo, log)
	// The admin panel's Gateway identity is provisioned on demand from the
	// administrator credentials in إعدادات النظام, so an operator never has to
	// paste a Bearer key by hand.
	adminKeys := newAdminKeyProvisioner(adminSvc, log)
	gwSource := newAdminGatewaySettings(adminSvc, adminKeys)

	ai := gateway.New(cfg.Gateway, log).WithSettingsSource(gwSource)
	if ai.Enabled() {
		log.Info("ai gateway enabled",
			"base_url", cfg.Gateway.BaseURL, "client_app", cfg.Gateway.ClientApp)
	} else {
		log.Info("ai gateway configured with admin settings source (awaiting credentials)")
	}

	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.HTTP.Port),
		Handler:           newRouter(cfg, log, deps, ai, adminKeys, migrations),
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
	// adminKeys is the admin panel's provisioned Gateway credential. The
	// settings screen has to be able to drop it when an operator changes the
	// credentials it was issued from, so it is threaded through rather than
	// rebuilt here — two instances would cache independently and one would go
	// on serving a key the operator had just replaced.
	adminKeys *adminKeyProvisioner,
	migrations []database.Migration,
) http.Handler {
	r := chi.NewRouter()

	r.Use(httpx.RequestID)
	r.Use(httpx.Recover(log))
	r.Use(httpx.Logger(log))
	r.Use(httpx.SecurityHeaders)
	r.Use(httpx.Locale)
	r.Use(chimw.Compress(5))

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

	// Unmatched routes get the same JSON envelope as every other error, with the
	// request id, rather than chi's bare "404 page not found" text. The hint
	// matters more than it looks: this domain previously served a different
	// application, so stale bookmarks and browser autocomplete land on paths
	// that never existed here.
	r.NotFound(func(w http.ResponseWriter, req *http.Request) {
		httpx.Error(w, req, log, apperr.New(apperr.KindNotFound, "route.not_found",
			"No such endpoint. Try /health, /ready, /api/v1/status, or /."))
	})

	// 405, not 422: the request body was never the problem, the verb was.
	// apperr has no method-not-allowed kind, so this writes the envelope
	// directly rather than mapping onto a kind that means something else.
	r.MethodNotAllowed(func(w http.ResponseWriter, req *http.Request) {
		var body httpx.ErrorBody
		body.Error.Code = "route.method_not_allowed"
		body.Error.Message = "That endpoint exists but does not accept this HTTP method."
		body.Error.RequestID = observability.RequestIDFrom(req.Context())
		httpx.JSON(w, http.StatusMethodNotAllowed, body)
	})

	// Mount all domain module and UI endpoints
	mountModuleRoutes(r, cfg, log, deps, ai, adminKeys)

	return r
}
