// Package features provides real-time, high-performance in-memory feature flag
// evaluation and HTTP route middleware.
package features

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/httpx"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// FeatureFlag is the domain model representing a toggleable system feature.
type FeatureFlag struct {
	Key         string            `json:"key"`
	Name        map[string]string `json:"name"`
	Description map[string]string `json:"description"`
	IsEnabled   bool              `json:"is_enabled"`
	UpdatedBy   *int64            `json:"updated_by,omitempty"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

// Engine manages cached flags with automatic background refresh.
type Engine struct {
	db    *database.DB
	log   *slog.Logger
	mu    sync.RWMutex
	flags map[string]bool
	list  []FeatureFlag
}

var globalEngine *Engine

// Init initializes the global feature flag engine.
func Init(ctx context.Context, db *database.DB, log *slog.Logger) (*Engine, error) {
	e := &Engine{
		db:    db,
		log:   log,
		flags: make(map[string]bool),
		list:  make([]FeatureFlag, 0),
	}

	if err := e.Reload(ctx); err != nil {
		log.Warn("features: initial flag load failed, using default true", "error", err)
	}

	globalEngine = e

	// Background ticker to reload every 60 seconds
	go func() {
		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			if err := e.Reload(context.Background()); err != nil {
				e.log.Warn("features: background reload failed", "error", err)
			}
		}
	}()

	return e, nil
}

// Reload queries the database for all feature flags and updates in-memory cache.
func (e *Engine) Reload(ctx context.Context) error {
	newFlags := make(map[string]bool)
	var newList []FeatureFlag

	err := e.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `SELECT key, name, description, is_enabled, updated_by, updated_at FROM platform_admin.feature_flags;`
		rows, err := tx.Query(txCtx, query)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var key string
			var nameBytes, descBytes []byte
			var isEnabled bool
			var updatedBy *int64
			var updatedAt time.Time

			if err := rows.Scan(&key, &nameBytes, &descBytes, &isEnabled, &updatedBy, &updatedAt); err != nil {
				return err
			}

			newFlags[key] = isEnabled

			name := make(map[string]string)
			desc := make(map[string]string)
			_ = json.Unmarshal(nameBytes, &name)
			_ = json.Unmarshal(descBytes, &desc)

			newList = append(newList, FeatureFlag{
				Key:         key,
				Name:        name,
				Description: desc,
				IsEnabled:   isEnabled,
				UpdatedBy:   updatedBy,
				UpdatedAt:   updatedAt,
			})
		}
		return rows.Err()
	})

	if err != nil {
		return fmt.Errorf("features.Reload: %w", err)
	}

	e.mu.Lock()
	e.flags = newFlags
	e.list = newList
	e.mu.Unlock()

	return nil
}

// Enabled checks if a feature flag is enabled in memory. Default is true for unknown flags.
func (e *Engine) Enabled(key string) bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if val, ok := e.flags[key]; ok {
		return val
	}
	return true
}

// List returns a copy of all registered feature flags.
func (e *Engine) List() []FeatureFlag {
	e.mu.RLock()
	defer e.mu.RUnlock()
	res := make([]FeatureFlag, len(e.list))
	copy(res, e.list)
	return res
}

// Set updates a feature flag in the database and immediately flushes the in-memory cache.
func (e *Engine) Set(ctx context.Context, key string, enabled bool, updatedBy int64) error {
	err := e.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			INSERT INTO platform_admin.feature_flags (key, name, is_enabled, updated_by, updated_at)
			VALUES ($1, jsonb_build_object('ar', $1, 'en', $1), $2, $3, now())
			ON CONFLICT (key) DO UPDATE
			SET is_enabled = EXCLUDED.is_enabled,
			    updated_by = EXCLUDED.updated_by,
			    updated_at = now();
		`
		_, err := tx.Exec(txCtx, query, key, enabled, updatedBy)
		return err
	})

	if err != nil {
		return fmt.Errorf("features.Set: %w", err)
	}

	_ = e.Reload(ctx)
	return nil
}

// Global accessor functions:

// Enabled checks if a feature is enabled on the global engine.
func Enabled(ctx context.Context, key string) bool {
	if globalEngine == nil {
		return true
	}
	return globalEngine.Enabled(key)
}

// List returns all feature flags from the global engine.
func List() []FeatureFlag {
	if globalEngine == nil {
		return nil
	}
	return globalEngine.List()
}

// Require returns a Chi HTTP middleware that yields a 404 Not Found if the feature is disabled.
func Require(flagKey string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !Enabled(r.Context(), flagKey) {
				var l *slog.Logger
				if globalEngine != nil {
					l = globalEngine.log
				}
				httpx.Error(w, r, l, apperr.NotFound("الصفحة المطلوبة غير مفعلة حالياً"))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}


// GetEngine returns the initialized singleton engine.
func GetEngine() *Engine {
	return globalEngine
}
