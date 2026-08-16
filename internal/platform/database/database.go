// Package database owns the PostgreSQL connection pool and, more importantly,
// the tenant isolation contract.
//
// The legacy system scoped tenants by hand: 353 Livewire components each
// remembering to add "where organization_id = ...". One omission leaks a
// competitor's catalogue or order book. That class of bug cannot be fixed by
// review discipline alone at that scale, so this package makes the database
// enforce it.
//
// The contract:
//
//   - Tenant-scoped work runs inside a transaction opened by InTx or InReadTx.
//   - Those helpers issue SET LOCAL app.current_org_id from the request context.
//   - Row-level security policies on every tenant-owned table compare against
//     that setting.
//   - A query that forgets its WHERE clause therefore returns zero rows instead
//     of another organisation's data. It fails closed.
//
// Bypassing RLS is possible but must be spelled out with AsSystem, which is
// greppable in review and shows up in the audit log.
package database

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/muhiya/dawa24-store/internal/platform/config"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// ErrNoTenant is returned when tenant-scoped work is attempted without an
// organisation in context.
//
// This is usually a legitimate request from a user who simply has no active
// organisation — a customer hitting a vendor endpoint, or a member who has not
// selected one yet. Classifying it as an internal error turned that into a 500
// reading "something went wrong on our side", which is both wrong and
// unactionable. It is a forbidden request with a message that says what to do.
var ErrNoTenant = apperr.Forbidden("tenant.required",
	"No active organization. Select one, or ask to be added to a supplier account.")

type ctxKey int

const (
	ctxKeyOrgID ctxKey = iota
	ctxKeySystem
)

// WithTenant marks the context as belonging to one organisation. HTTP middleware
// calls this after resolving the authenticated user's active organisation.
func WithTenant(ctx context.Context, orgID int64) context.Context {
	return context.WithValue(ctx, ctxKeyOrgID, orgID)
}

// TenantFrom returns the organisation bound to this context.
func TenantFrom(ctx context.Context) (int64, bool) {
	orgID, ok := ctx.Value(ctxKeyOrgID).(int64)
	return orgID, ok && orgID > 0
}

// AsSystem marks a context as exempt from row-level security.
//
// Use it only for platform-admin screens, migrations, and background jobs that
// legitimately span tenants — and log why. Every call site is a deliberate hole
// in the isolation guarantee and should read like one.
func AsSystem(ctx context.Context) context.Context {
	return context.WithValue(ctx, ctxKeySystem, true)
}

func isSystem(ctx context.Context) bool {
	v, _ := ctx.Value(ctxKeySystem).(bool)
	return v
}

// ErrNotConnected is returned when the pool has not been established yet.
var ErrNotConnected = errors.New("database: not connected yet")

// DB wraps the pool and exposes only transaction-scoped access.
//
// The handle is created before the pool exists and is filled in once dialling
// succeeds. That indirection matters: the HTTP server starts before its
// dependencies are up (see cmd/server/deps.go), so routes are mounted — and
// repositories constructed — while the database is still connecting. Handing
// those repositories a *DB that is nil at that moment would leave every one of
// them holding a nil pointer forever, which is exactly the panic this replaced.
type DB struct {
	mu   sync.RWMutex
	pool *pgxpool.Pool
}

// New returns an unconnected handle. Call Connect to establish the pool.
func New() *DB { return &DB{} }

// Connect dials PostgreSQL and attaches the pool to this handle.
//
// Safe to call repeatedly: a successful connection replaces any previous pool
// and closes it, so a retry loop cannot leak connections.
func (db *DB) Connect(ctx context.Context, cfg config.Database) error {
	pool, err := newPool(ctx, cfg)
	if err != nil {
		return err
	}

	db.mu.Lock()
	old := db.pool
	db.pool = pool
	db.mu.Unlock()

	if old != nil {
		old.Close()
	}
	return nil
}

// Connected reports whether the pool is established.
func (db *DB) Connected() bool {
	db.mu.RLock()
	defer db.mu.RUnlock()
	return db.pool != nil
}

func (db *DB) getPool() (*pgxpool.Pool, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()
	if db.pool == nil {
		return nil, ErrNotConnected
	}
	return db.pool, nil
}

// Open builds the pool and verifies connectivity. A process that cannot reach
// its database should fail at boot, not on its first request.
func Open(ctx context.Context, cfg config.Database) (*DB, error) {
	pool, err := newPool(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return &DB{pool: pool}, nil
}

func newPool(ctx context.Context, cfg config.Database) (*pgxpool.Pool, error) {
	poolCfg, err := pgxpool.ParseConfig(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("database: parse DATABASE_URL: %w", err)
	}

	poolCfg.MaxConns = cfg.MaxConns
	poolCfg.MinConns = cfg.MinConns
	poolCfg.MaxConnLifetime = cfg.MaxConnLifetime
	poolCfg.MaxConnIdleTime = cfg.MaxConnIdleTime
	poolCfg.HealthCheckPeriod = time.Minute

	// A statement timeout is the difference between one pathological query and a
	// saturated pool. The legacy system had neither, which is why a single
	// unindexed admin report could stall checkout.
	if poolCfg.ConnConfig.RuntimeParams == nil {
		poolCfg.ConnConfig.RuntimeParams = map[string]string{}
	}
	poolCfg.ConnConfig.RuntimeParams["statement_timeout"] =
		fmt.Sprintf("%d", cfg.StatementTimeout.Milliseconds())
	poolCfg.ConnConfig.RuntimeParams["application_name"] = "dawa24-store"

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("database: create pool: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("database: ping: %w", err)
	}

	return pool, nil
}

// Close releases the pool if one is attached.
func (db *DB) Close() {
	db.mu.Lock()
	defer db.mu.Unlock()
	if db.pool != nil {
		db.pool.Close()
		db.pool = nil
	}
}

// Health verifies the database answers, for the /health endpoint and the
// container healthcheck.
func (db *DB) Health(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	pool, err := db.getPool()
	if err != nil {
		return err
	}

	var one int
	if err := pool.QueryRow(ctx, "SELECT 1").Scan(&one); err != nil {
		return fmt.Errorf("database: health: %w", err)
	}
	return nil
}

// Pool exposes the raw pool for the River queue driver, which manages its own
// transactions. Application code must not use this.
func (db *DB) Pool() *pgxpool.Pool {
	db.mu.RLock()
	defer db.mu.RUnlock()
	return db.pool
}

// InTx runs fn inside a read-write transaction with tenant isolation applied.
//
// The transaction commits if fn returns nil and rolls back otherwise, including
// on panic — a panic mid-order must never leave a half-written basket behind.
func (db *DB) InTx(ctx context.Context, fn func(context.Context, pgx.Tx) error) error {
	return db.transact(ctx, pgx.TxOptions{AccessMode: pgx.ReadWrite}, fn)
}

// InReadTx runs fn inside a read-only transaction with tenant isolation applied.
//
// Read-only is not just a hint: it makes PostgreSQL reject writes, so a query
// helper that accidentally mutates state fails loudly in development.
func (db *DB) InReadTx(ctx context.Context, fn func(context.Context, pgx.Tx) error) error {
	return db.transact(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly}, fn)
}

func (db *DB) transact(ctx context.Context, opts pgx.TxOptions, fn func(context.Context, pgx.Tx) error) (err error) {
	pool, err := db.getPool()
	if err != nil {
		return err
	}

	tx, err := pool.BeginTx(ctx, opts)
	if err != nil {
		return fmt.Errorf("database: begin: %w", err)
	}

	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback(context.WithoutCancel(ctx))
			panic(p)
		}
		if err != nil {
			// Use a detached context so rollback still runs when the request
			// context is already cancelled — otherwise a client disconnect
			// leaves the transaction open until the server times it out.
			_ = tx.Rollback(context.WithoutCancel(ctx))
		}
	}()

	if err = applyTenant(ctx, tx); err != nil {
		return err
	}

	if err = fn(ctx, tx); err != nil {
		return err
	}

	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("database: commit: %w", err)
	}
	return nil
}

// applyTenant sets the GUC that row-level security policies read.
//
// SET LOCAL scopes the setting to this transaction, so it is discarded when the
// connection returns to the pool. Session-level SET would leak one tenant's id
// into the next request that borrowed the same connection — which is exactly the
// bug this whole mechanism exists to prevent.
func applyTenant(ctx context.Context, tx pgx.Tx) error {
	if isSystem(ctx) {
		// Explicit cross-tenant access. RLS policies grant the bypass role;
		// we still clear any inherited org so nothing is silently scoped.
		if _, err := tx.Exec(ctx, "SELECT set_config('app.is_system', 'on', true)"); err != nil {
			return fmt.Errorf("database: set system context: %w", err)
		}
		return nil
	}

	orgID, ok := TenantFrom(ctx)
	if !ok {
		// Not every query is tenant-scoped: login, the public catalogue, and
		// reference data are legitimately tenant-free. Those tables have no RLS
		// policy, so leaving the GUC unset is correct. Tenant-owned tables will
		// return zero rows, which is the safe direction to fail.
		if _, err := tx.Exec(ctx, "SELECT set_config('app.current_org_id', '', true)"); err != nil {
			return fmt.Errorf("database: clear tenant context: %w", err)
		}
		return nil
	}

	// The value is formatted in Go rather than cast in SQL.
	//
	// `$1::text` tells PostgreSQL the parameter's type is text, so pgx must
	// encode an int64 as text and has no plan for that — every tenant-scoped
	// transaction failed with "cannot find encode plan". set_config's second
	// argument is text by signature, so the conversion has to happen on this
	// side of the wire.
	if _, err := tx.Exec(ctx,
		"SELECT set_config('app.current_org_id', $1, true)",
		strconv.FormatInt(orgID, 10),
	); err != nil {
		return fmt.Errorf("database: set tenant context: %w", err)
	}
	return nil
}

// IsUniqueViolation reports whether err is a PostgreSQL unique constraint
// violation, so services can turn it into a domain conflict instead of a 500.
func IsUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// IsForeignKeyViolation reports a referential integrity failure.
func IsForeignKeyViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23503"
}

// ConstraintName returns the violated constraint, letting a service map a
// specific unique index to a specific field error.
func ConstraintName(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.ConstraintName
	}
	return ""
}

// IsNotFound reports the no-rows case.
func IsNotFound(err error) bool { return errors.Is(err, pgx.ErrNoRows) }
