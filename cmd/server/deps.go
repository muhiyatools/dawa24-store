package main

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/muhiya/dawa24-store/internal/platform/cache"
	"github.com/muhiya/dawa24-store/internal/platform/config"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// dependencies holds connections that may not be available at boot.
//
// The server deliberately starts BEFORE these are connected. A process that
// exits because PostgreSQL was briefly unreachable leaves the reverse proxy
// with nothing to talk to, and the operator sees a bare "502 Bad Gateway" with
// no indication of what failed. Starting anyway means /health answers, /ready
// explains precisely which dependency is down, and the platform can distinguish
// "still starting" from "misconfigured".
//
// Configuration errors are different and still abort at boot: a bad DATABASE_URL
// is a deployment mistake that no amount of retrying will fix.
type dependencies struct {
	mu       sync.RWMutex
	db       *database.DB
	cache    *cache.Cache
	dbErr    error
	cacheErr error
	ready    bool
}

func newDependencies() *dependencies {
	return &dependencies{
		dbErr:    errNotConnectedYet,
		cacheErr: errNotConnectedYet,
	}
}

type notConnected struct{}

func (notConnected) Error() string { return "not connected yet" }

var errNotConnectedYet = notConnected{}

func (d *dependencies) DB() (*database.DB, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.db, d.dbErr
}

func (d *dependencies) Cache() (*cache.Cache, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.cache, d.cacheErr
}

// Ready reports whether every dependency connected at least once.
func (d *dependencies) Ready() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.ready
}

func (d *dependencies) setDB(db *database.DB, err error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.db, d.dbErr = db, err
	d.recompute()
}

func (d *dependencies) setCache(c *cache.Cache, err error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.cache, d.cacheErr = c, err
	d.recompute()
}

// recompute must be called with the write lock held.
func (d *dependencies) recompute() {
	d.ready = d.dbErr == nil && d.cacheErr == nil
}

func (d *dependencies) close() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.db != nil {
		d.db.Close()
	}
	if d.cache != nil {
		_ = d.cache.Close()
	}
}

// connect dials both dependencies in the background, retrying until the context
// is cancelled.
//
// Retrying forever rather than giving up after N attempts is deliberate: on a
// fresh deploy the database service may genuinely still be provisioning, and a
// container that exits after 30 seconds turns a slow start into a crash loop
// that hides the original cause.
func (d *dependencies) connect(ctx context.Context, cfg *config.Config, log *slog.Logger) {
	go retryForever(ctx, log, "database", func(ctx context.Context) error {
		db, err := database.Open(ctx, cfg.Database)
		if err != nil {
			d.setDB(nil, err)
			return err
		}
		d.setDB(db, nil)
		return nil
	})

	go retryForever(ctx, log, "redis", func(ctx context.Context) error {
		c, err := cache.Open(ctx, cfg.Redis, cfg.Env)
		if err != nil {
			d.setCache(nil, err)
			return err
		}
		d.setCache(c, nil)
		return nil
	})
}

func retryForever(ctx context.Context, log *slog.Logger, name string, dial func(context.Context) error) {
	backoff := time.Second
	const maxBackoff = 30 * time.Second

	for attempt := 1; ; attempt++ {
		if ctx.Err() != nil {
			return
		}

		err := dial(ctx)
		if err == nil {
			log.Info("dependency connected", "dependency", name, "attempts", attempt)
			return
		}

		// Logged at warn, not error: during a normal cold start this is
		// expected for a few seconds and should not page anyone.
		log.Warn("dependency unavailable, retrying",
			"dependency", name,
			"attempt", attempt,
			"retry_in", backoff.String(),
			"error", err)

		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}

		if backoff < maxBackoff {
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		}
	}
}
