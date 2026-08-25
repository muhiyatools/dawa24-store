// Tiny in-memory TTL cache for expensive read-mostly lookups used by the
// vendor import wizard. The master/savings catalog snapshots and the manual
// override dropdown lists are rebuilt on every submit/page view otherwise,
// re-scanning hundreds of rows each time.
package ui

import (
	"time"

	"github.com/muhiya/dawa24-store/internal/modules/ingest"
)

const (
	matchingDataCacheTTL = 2 * time.Minute
	dropdownListCacheTTL = 30 * time.Second
)

type uiCacheEntry struct {
	expiresAt time.Time
	value     any
}

// matchingDataPair bundles the ingest matching snapshot under one cache key.
type matchingDataPair struct {
	master []*ingest.MasterProductData
	saving []*ingest.SavingProductData
}

func (h *UIHandler) cacheGet(key string) (any, bool) {
	h.cacheMu.Lock()
	defer h.cacheMu.Unlock()
	entry, ok := h.cache[key]
	if !ok || time.Now().After(entry.expiresAt) {
		if ok {
			delete(h.cache, key)
		}
		return nil, false
	}
	return entry.value, true
}

func (h *UIHandler) cacheSet(key string, value any, ttl time.Duration) {
	h.cacheMu.Lock()
	defer h.cacheMu.Unlock()
	if h.cache == nil {
		h.cache = make(map[string]uiCacheEntry)
	}
	h.cache[key] = uiCacheEntry{expiresAt: time.Now().Add(ttl), value: value}
}
