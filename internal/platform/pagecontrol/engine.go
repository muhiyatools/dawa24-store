package pagecontrol

import (
	"context"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// Engine holds the current enable/disable rules in memory and answers the
// blocking question for the guard.
//
// Fail-open by construction: an engine that never loaded, or whose last reload
// failed, holds no rules and blocks nothing. A page-control cache miss must not
// be able to take the marketplace down — the same posture features takes when
// its table is unreachable, and the one AGENTS.md R3 calls for.
type Engine struct {
	store *Store
	log   *slog.Logger

	mu       sync.RWMutex
	byExact  map[string]rule
	prefixes []rule // sorted by descending path length
	version  int64
}

const (
	reloadInterval = 20 * time.Second
	channelName    = "pagecontrol_changed"
)

var (
	globalMu sync.RWMutex
	global   *Engine
)

// Init builds the process-wide engine, loads it once, and starts the background
// refresh. A load failure is logged, not returned: the process must start.
func Init(ctx context.Context, db *database.DB, log *slog.Logger) *Engine {
	if log == nil {
		log = slog.Default()
	}
	e := &Engine{store: NewStore(db), log: log, byExact: map[string]rule{}}
	if err := e.Reload(ctx); err != nil {
		log.Warn("pagecontrol: initial load failed, serving every route", "error", err)
	}
	globalMu.Lock()
	global = e
	globalMu.Unlock()

	go e.refreshLoop(db)
	return e
}

// Global returns the engine Init created, or nil before it runs.
func Global() *Engine {
	globalMu.RLock()
	defer globalMu.RUnlock()
	return global
}

// Blocked reports whether the global engine would refuse path. A nil engine
// blocks nothing.
func Blocked(path string) bool {
	e := Global()
	if e == nil {
		return false
	}
	b, _ := e.Decision(path)
	return b
}

// Reload replaces the in-memory snapshot from the store.
func (e *Engine) Reload(ctx context.Context) error {
	rules, err := e.store.Snapshot(ctx)
	if err != nil {
		return err
	}
	ver, _ := e.store.Version(ctx)

	exact := make(map[string]rule, len(rules))
	var prefixes []rule
	for _, r := range rules {
		if r.mode == MatchPrefix {
			prefixes = append(prefixes, r)
		} else {
			exact[r.path] = r
		}
	}
	sort.Slice(prefixes, func(i, j int) bool { return len(prefixes[i].path) > len(prefixes[j].path) })

	e.mu.Lock()
	e.byExact = exact
	e.prefixes = prefixes
	e.version = ver
	e.mu.Unlock()
	return nil
}

// Decision reports whether path is currently disabled, and the id of the rule
// that decided it. The most specific matching rule wins: the longest path, and
// on a tie an exact rule over a prefix one. A protected path is never blocked.
func (e *Engine) Decision(path string) (bool, int64) {
	if e == nil {
		return false, 0
	}
	p := NormalizePath(path)
	if IsProtected(p) {
		return false, 0
	}

	e.mu.RLock()
	defer e.mu.RUnlock()

	var (
		winner   rule
		haveWin  bool
		winLen   int
		winExact bool
	)
	if r, ok := e.byExact[p]; ok {
		winner, haveWin, winLen, winExact = r, true, len(r.path), true
	}
	for _, r := range e.prefixes {
		if p != r.path && !strings.HasPrefix(p, r.path+"/") {
			continue
		}
		if !haveWin || len(r.path) > winLen || (len(r.path) == winLen && !winExact) {
			winner, haveWin, winLen, winExact = r, true, len(r.path), false
		}
		// prefixes are sorted longest-first; once we have a match no shorter
		// prefix can beat it and no exact rule is left to consider.
		break
	}
	if !haveWin {
		return false, 0
	}
	return !winner.enabled, winner.id
}

// Version is the invalidation counter the last reload saw.
func (e *Engine) Version() int64 {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.version
}

// refreshLoop keeps the snapshot fresh: a LISTEN for near-immediate propagation
// across instances, with a timer as the safety net if the notification is missed
// or the listen connection drops.
func (e *Engine) refreshLoop(db *database.DB) {
	ctx := context.Background()
	ticker := time.NewTicker(reloadInterval)
	defer ticker.Stop()

	notifications := make(chan struct{}, 1)
	go e.listen(ctx, db, notifications)

	for {
		select {
		case <-ticker.C:
		case <-notifications:
		}
		if err := e.Reload(ctx); err != nil {
			e.log.Warn("pagecontrol: background reload failed", "error", err)
		}
	}
}

// listen waits on Postgres NOTIFY and nudges the loop. Any failure falls back to
// the timer; it retries the listen after a short pause.
func (e *Engine) listen(ctx context.Context, db *database.DB, out chan<- struct{}) {
	if db == nil {
		return
	}
	for {
		func() {
			pool := db.Pool()
			if pool == nil {
				time.Sleep(5 * time.Second)
				return
			}
			conn, err := pool.Acquire(ctx)
			if err != nil {
				time.Sleep(5 * time.Second)
				return
			}
			defer conn.Release()
			if _, err := conn.Exec(ctx, "LISTEN "+channelName); err != nil {
				time.Sleep(5 * time.Second)
				return
			}
			for {
				if _, err := conn.Conn().WaitForNotification(ctx); err != nil {
					time.Sleep(2 * time.Second)
					return
				}
				select {
				case out <- struct{}{}:
				default:
				}
			}
		}()
	}
}
