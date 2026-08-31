package antiscrape

import (
	"context"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// store counts requests per key per window and remembers penalties.
//
// Two implementations back it: Redis, which is shared across instances, and a
// per-process map used when Redis is not connected. The fallback is not an
// optimisation — it is what keeps the defence standing during a Redis outage,
// which is exactly when an unprotected catalogue is most attractive.
type store interface {
	// Hit increments the counter for key and returns its new value. The window
	// is applied only when the counter is created, so a window is fixed from
	// its first request rather than extended by every later one.
	Hit(ctx context.Context, key string, window time.Duration) int64
	// Penalize marks a key as misbehaving for ttl.
	Penalize(ctx context.Context, key string, ttl time.Duration)
	// Penalized reports whether a key is currently marked.
	Penalized(ctx context.Context, key string) bool
}

// hybridStore prefers Redis and falls back to memory.
//
// The client is resolved per call rather than captured: the HTTP server starts
// before Redis is dialled, so a client captured at construction would be nil
// for the lifetime of the process.
type hybridStore struct {
	redis func() *redis.Client
	mem   *memStore
}

func newHybridStore(provider func() *redis.Client) *hybridStore {
	return &hybridStore{redis: provider, mem: newMemStore()}
}

func (s *hybridStore) client() *redis.Client {
	if s.redis == nil {
		return nil
	}
	return s.redis()
}

func (s *hybridStore) Hit(ctx context.Context, key string, window time.Duration) int64 {
	rdb := s.client()
	if rdb == nil {
		return s.mem.Hit(ctx, key, window)
	}

	n, err := rdb.Incr(ctx, key).Result()
	if err != nil {
		// Redis is unreachable. Counting in memory is a weaker guarantee than
		// counting centrally, and a far better one than not counting.
		return s.mem.Hit(ctx, key, window)
	}
	if n == 1 {
		_ = rdb.Expire(ctx, key, window).Err()
	}
	return n
}

func (s *hybridStore) Penalize(ctx context.Context, key string, ttl time.Duration) {
	rdb := s.client()
	if rdb == nil {
		s.mem.Penalize(ctx, key, ttl)
		return
	}
	if err := rdb.Set(ctx, key, "1", ttl).Err(); err != nil {
		s.mem.Penalize(ctx, key, ttl)
	}
}

func (s *hybridStore) Penalized(ctx context.Context, key string) bool {
	rdb := s.client()
	if rdb == nil {
		return s.mem.Penalized(ctx, key)
	}
	n, err := rdb.Exists(ctx, key).Result()
	if err != nil {
		return s.mem.Penalized(ctx, key)
	}
	return n > 0
}

// memStore is the in-process fallback.
//
// It is bounded rather than unbounded: a spray of requests from many forged
// addresses must not be able to grow this map until the process runs out of
// memory. Past the ceiling the map is dropped wholesale, which loses history
// but never allocates more than the ceiling holds.
type memStore struct {
	mu      sync.Mutex
	entries map[string]memEntry
}

type memEntry struct {
	count   int64
	expires time.Time
}

const memStoreCeiling = 20000

func newMemStore() *memStore {
	return &memStore{entries: make(map[string]memEntry)}
}

func (m *memStore) Hit(_ context.Context, key string, window time.Duration) int64 {
	now := time.Now()

	m.mu.Lock()
	defer m.mu.Unlock()

	m.pruneLocked(now)

	e, ok := m.entries[key]
	if !ok || now.After(e.expires) {
		m.entries[key] = memEntry{count: 1, expires: now.Add(window)}
		return 1
	}
	e.count++
	m.entries[key] = e
	return e.count
}

func (m *memStore) Penalize(_ context.Context, key string, ttl time.Duration) {
	now := time.Now()

	m.mu.Lock()
	defer m.mu.Unlock()

	m.pruneLocked(now)
	m.entries[key] = memEntry{count: 1, expires: now.Add(ttl)}
}

func (m *memStore) Penalized(_ context.Context, key string) bool {
	now := time.Now()

	m.mu.Lock()
	defer m.mu.Unlock()

	e, ok := m.entries[key]
	return ok && now.Before(e.expires)
}

// pruneLocked drops expired entries, and everything if the map is still over
// its ceiling afterwards. Callers hold m.mu.
func (m *memStore) pruneLocked(now time.Time) {
	if len(m.entries) < memStoreCeiling {
		return
	}
	for k, e := range m.entries {
		if now.After(e.expires) {
			delete(m.entries, k)
		}
	}
	if len(m.entries) >= memStoreCeiling {
		m.entries = make(map[string]memEntry)
	}
}
