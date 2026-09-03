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
	"github.com/jackc/pgx/v5"

	dbfs "github.com/muhiya/dawa24-store/db"
	"github.com/muhiya/dawa24-store/internal/modules/billing"
	billingPostgres "github.com/muhiya/dawa24-store/internal/modules/billing/postgres"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	orgPostgres "github.com/muhiya/dawa24-store/internal/modules/org/postgres"
	platformadmin "github.com/muhiya/dawa24-store/internal/modules/platform_admin"
	platformadminPostgres "github.com/muhiya/dawa24-store/internal/modules/platform_admin/postgres"
	"github.com/muhiya/dawa24-store/internal/platform/aiusage"
	aiusagePostgres "github.com/muhiya/dawa24-store/internal/platform/aiusage/postgres"
	"github.com/muhiya/dawa24-store/internal/platform/config"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/gateway"
	"github.com/muhiya/dawa24-store/internal/platform/httpx"
	"github.com/muhiya/dawa24-store/internal/platform/observability"
	"github.com/muhiya/dawa24-store/internal/platform/pagecontrol"
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

	// The Gateway administrator credential is typed by an operator and sent as
	// Basic auth to a third-party host. Registering the database password here
	// is what lets the settings screen recognise and refuse it — a bare
	// password has no shape a heuristic can catch.
	platformadmin.SetKnownDatabaseSecret(config.DatabasePassword(cfg.Database.URL))

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

	// Error capture starts as early as the service it writes through, so a
	// failure during the rest of start-up is recorded rather than only logged.
	stopErrorTracking := installErrorTracking(adminSvc, log)
	defer stopErrorTracking()

	// Sync custom database translations into the runtime i18n engine in background
	go func() {
		for i := 0; i < 30; i++ {
			time.Sleep(1500 * time.Millisecond)
			if err := adminSvc.SyncRuntimeOverrides(context.Background()); err == nil {
				break
			}
		}
	}()

	// Ensure orphaned variants, warehouses, and offers are linked to real vendor branches
	go func() {
		for i := 0; i < 30; i++ {
			time.Sleep(2000 * time.Millisecond)
			dbHandle := deps.Handle()
			if dbHandle != nil && dbHandle.Connected() {
				_ = dbHandle.InTx(database.AsSystem(context.Background()), func(txCtx context.Context, tx pgx.Tx) error {
					_, _ = tx.Exec(txCtx, `
						UPDATE catalog.product_variants v
						SET branch_id = (
							SELECT b.id FROM org.branches b
							WHERE b.organization_id = v.organization_id AND b.deleted_at IS NULL
							ORDER BY b.is_main DESC, b.id ASC LIMIT 1
						)
						WHERE v.branch_id IS NULL;

						UPDATE inventory.warehouses w
						SET branch_id = (
							SELECT b.id FROM org.branches b
							WHERE b.organization_id = w.organization_id AND b.deleted_at IS NULL
							ORDER BY b.is_main DESC, b.id ASC LIMIT 1
						)
						WHERE w.branch_id IS NULL;

						UPDATE promo.offers o
						SET branch_id = (
							SELECT b.id FROM org.branches b
							WHERE b.organization_id = o.organization_id AND b.deleted_at IS NULL
							ORDER BY b.is_main DESC, b.id ASC LIMIT 1
						)
						WHERE o.branch_id IS NULL;
					`)
					return nil
				})
				break
			}
		}
	}()

	// The admin panel's Gateway identity is provisioned on demand from the
	// administrator credentials in إعدادات النظام, so an operator never has to
	// paste a Bearer key by hand.
	adminKeys := newAdminKeyProvisioner(adminSvc, log)
	gwSource := newAdminGatewaySettings(adminSvc, adminKeys)

	// One Gateway identity per منشأة, resolved through a single cache.
	//
	// It is built here rather than at each mount point for the same reason
	// adminKeys is: two instances would provision independently, and because
	// issuing a virtual key revokes the previous one, the second would silently
	// invalidate the first tenant key the moment both were asked at once.
	tenantKeys := newTenantKeyProvisioner(
		org.NewService(orgPostgres.NewRepository(deps.Handle()), log),
		billing.NewService(billingPostgres.NewRepository(deps.Handle()), log),
		adminSvc,
		adminKeys,
		log,
	)

	// The usage ledger. Every AI call is written down locally, attributed to
	// the منشأة that paid for it, so a tenant's consumption history survives a
	// Gateway outage and is not capped at whatever one live API page returns.
	//
	// Wrapping is the last step in building the client, so nothing downstream
	// can hold an unrecorded reference to it.
	usageRecorder := aiusage.NewRecorder(aiusagePostgres.NewRepository(deps.Handle()), log)

	var ai gateway.Client = gateway.New(cfg.Gateway, log).WithSettingsSource(gwSource)
	ai = gateway.WithUsageRecorder(ai, usageRecorder)
	if ai.Enabled() {
		log.Info("ai gateway enabled",
			"base_url", cfg.Gateway.BaseURL, "client_app", cfg.Gateway.ClientApp)
	} else {
		log.Info("ai gateway configured with admin settings source (awaiting credentials)")
	}

	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.HTTP.Port),
		Handler:           newRouter(cfg, log, deps, ai, adminKeys, tenantKeys, migrations),
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

	// Drain the usage ledger after the requests that fill it have finished, so
	// the last import's consumption is recorded rather than discarded on a
	// deploy.
	usageRecorder.Close(shutdownCtx)

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
	// tenantKeys resolves the per-organisation Gateway key every employee of
	// that organisation spends against. Threaded through for the same reason as
	// adminKeys: a second instance would cache and provision independently.
	tenantKeys *tenantKeyProvisioner,
	migrations []database.Migration,
) http.Handler {
	r := chi.NewRouter()

	r.Use(httpx.RequestID)
	r.Use(httpx.Recover(log))
	r.Use(httpx.Logger(log))
	r.Use(httpx.SecurityHeaders)
	r.Use(httpx.Locale)
	// Shorter than HTTP.WriteTimeout so the application, not the socket, ends a
	// slow request: the query is cancelled, its pool connection is released,
	// and the user gets a rendered error instead of the proxy's 502.
	r.Use(httpx.RequestTimeout(cfg.HTTP.RequestTimeout, httpx.IsLongRunning))
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
	notFound := func(w http.ResponseWriter, req *http.Request) {
		httpx.Error(w, req, log, apperr.New(apperr.KindNotFound, "route.not_found",
			"No such endpoint. Try /health, /ready, /api/v1/status, or /."))
	}
	r.NotFound(notFound)

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
	mountModuleRoutes(r, cfg, log, deps, ai, adminKeys, tenantKeys)

	// Route-level page control. The engine loads platform_admin.managed_pages
	// and keeps it fresh in the background; discovery walks the route table just
	// mounted above so no route has to be typed in by hand. Guard then wraps the
	// whole mux and answers a disabled route with the same 404 an unknown one
	// gets — before auth, for every caller. The bootstrap waits for the database
	// in a goroutine so a slow first connect does not delay the listener; until
	// it runs, Guard finds no engine and serves everything.
	go func() {
		db := deps.Handle()
		for i := 0; i < 30; i++ {
			if db != nil && db.Connected() {
				break
			}
			time.Sleep(time.Second)
		}
		pagecontrol.Init(context.Background(), db, log)
		pagecontrol.SetRouter(r)
		if added, err := pagecontrol.SyncDiscovered(context.Background(), db, r); err != nil {
			log.Warn("pagecontrol: route discovery failed", "error", err)
		} else {
			log.Info("pagecontrol: catalogue synced", "discovered_added", added)
		}
	}()

	return pagecontrol.Guard(r, notFound, log)
}
