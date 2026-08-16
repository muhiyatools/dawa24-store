// Package cache wraps Redis for caching, sessions and rate limiting.
//
// The legacy system ran all three on MySQL (CACHE_STORE=database,
// SESSION_DRIVER=database, plus a jobs table), so every page view wrote to the
// same database handling order inserts. Moving them to Redis removes that write
// amplification from the primary.
//
// The tenant rule in this package is absolute: every cache key is namespaced by
// organisation. A key without a tenant prefix is a cross-tenant data leak that
// no amount of row-level security will catch, because the data never reaches the
// database on a cache hit.
package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/muhiya/dawa24-store/internal/platform/config"
)

// ErrMiss signals a key that is absent. Callers treat this as "compute it",
// never as a failure.
var ErrMiss = errors.New("cache: miss")

type Cache struct {
	rdb    *redis.Client
	prefix string
}

func Open(ctx context.Context, cfg config.Redis, appEnv config.Env) (*Cache, error) {
	opts, err := redis.ParseURL(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("cache: parse REDIS_URL: %w", err)
	}
	opts.PoolSize = cfg.PoolSize

	rdb := redis.NewClient(opts)

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := rdb.Ping(pingCtx).Err(); err != nil {
		_ = rdb.Close()
		return nil, fmt.Errorf("cache: ping: %w", err)
	}

	// Environment-prefixed keys mean a staging deploy pointed at the wrong
	// Redis cannot serve production data, and flushing one environment cannot
	// wipe another.
	return &Cache{rdb: rdb, prefix: "dawa24:" + string(appEnv) + ":"}, nil
}

func (c *Cache) Close() error { return c.rdb.Close() }

func (c *Cache) Health(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	return c.rdb.Ping(ctx).Err()
}

// Redis exposes the client for the session store and rate limiter.
func (c *Cache) Redis() *redis.Client { return c.rdb }

// Key builds a tenant-scoped cache key.
//
// Pass orgID 0 only for genuinely global data such as countries, currencies and
// platform settings. Everything else must carry its organisation.
func (c *Cache) Key(orgID int64, parts ...string) string {
	k := c.prefix
	if orgID > 0 {
		k += "org:" + strconv.FormatInt(orgID, 10) + ":"
	} else {
		k += "global:"
	}
	for i, p := range parts {
		if i > 0 {
			k += ":"
		}
		k += p
	}
	return k
}

// GetJSON reads and decodes a value, returning ErrMiss when absent.
func (c *Cache) GetJSON(ctx context.Context, key string, dst any) error {
	raw, err := c.rdb.Get(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		return ErrMiss
	}
	if err != nil {
		return fmt.Errorf("cache: get %s: %w", key, err)
	}
	if err := json.Unmarshal(raw, dst); err != nil {
		// A value we cannot decode is worse than no value: drop it so the next
		// request repopulates rather than failing forever on a stale shape.
		_ = c.rdb.Del(ctx, key).Err()
		return ErrMiss
	}
	return nil
}

// SetJSON encodes and stores a value with a TTL.
//
// A TTL is mandatory. Unbounded cache entries were how the legacy cache table
// grew without limit; here an omitted expiry would simply be a slow leak.
func (c *Cache) SetJSON(ctx context.Context, key string, val any, ttl time.Duration) error {
	if ttl <= 0 {
		return errors.New("cache: TTL must be positive")
	}
	raw, err := json.Marshal(val)
	if err != nil {
		return fmt.Errorf("cache: marshal %s: %w", key, err)
	}
	if err := c.rdb.Set(ctx, key, raw, ttl).Err(); err != nil {
		return fmt.Errorf("cache: set %s: %w", key, err)
	}
	return nil
}

// Delete removes keys, used on write to invalidate.
func (c *Cache) Delete(ctx context.Context, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}
	return c.rdb.Del(ctx, keys...).Err()
}

// Remember returns the cached value or computes, stores and returns it.
//
// A failure to read or write the cache is logged by the caller but never fails
// the request: the compute path is always available. Redis being down should
// make the site slow, not broken.
func Remember[T any](ctx context.Context, c *Cache, key string, ttl time.Duration, compute func(context.Context) (T, error)) (T, error) {
	var cached T
	if err := c.GetJSON(ctx, key, &cached); err == nil {
		return cached, nil
	}

	value, err := compute(ctx)
	if err != nil {
		return value, err
	}
	_ = c.SetJSON(ctx, key, value, ttl)
	return value, nil
}

// InvalidateTenant drops every cached entry for one organisation.
//
// It uses SCAN rather than KEYS: KEYS blocks Redis for the duration of the
// sweep, which on a shared instance stalls every other tenant's requests too.
func (c *Cache) InvalidateTenant(ctx context.Context, orgID int64) error {
	pattern := c.Key(orgID) + "*"
	var cursor uint64
	for {
		keys, next, err := c.rdb.Scan(ctx, cursor, pattern, 200).Result()
		if err != nil {
			return fmt.Errorf("cache: scan %s: %w", pattern, err)
		}
		if len(keys) > 0 {
			if err := c.rdb.Del(ctx, keys...).Err(); err != nil {
				return fmt.Errorf("cache: delete batch: %w", err)
			}
		}
		if next == 0 {
			return nil
		}
		cursor = next
	}
}
