package progress

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

// Carrying a snapshot between processes.
//
// Imports run in cmd/worker and screens are served by cmd/server, so an
// in-process Hub on its own would never see the progress it exists to show. One
// Redis channel closes that gap: the worker publishes, every server process
// subscribes ONCE and feeds its local hub, and the number of Redis subscribers
// is the number of app instances rather than the number of people watching.
//
// Redis being unavailable degrades the feature rather than breaking it. The
// stream handler still reads the run from the database when a viewer connects
// and on its slow safety tick, so a bar keeps moving — just in seconds rather
// than instantly. That is the same trade the rest of the platform makes with
// the cache: Redis down should make the site slower, never broken.

// channel is the Redis pub/sub channel every import's progress travels on.
//
// One channel rather than one per run: a channel per run means a SUBSCRIBE and
// UNSUBSCRIBE round trip for every import a viewer opens, and the message rate
// here is a handful a second across the whole platform. Filtering by run id in
// the process is free by comparison.
const channel = "dawa24:import-progress"

// Publisher sends snapshots to the other processes.
type Publisher struct {
	rdb *redis.Client
	hub *Hub
	log *slog.Logger
}

// NewPublisher returns a publisher that feeds the local hub and, when a Redis
// client is available, the other processes too.
//
// rdb may be nil. A worker with no Redis still publishes locally, which is what
// the tests use and what a single-process deployment gets.
func NewPublisher(rdb *redis.Client, hub *Hub, log *slog.Logger) *Publisher {
	return &Publisher{rdb: rdb, hub: hub, log: log}
}

// Publish delivers a snapshot locally and across the bus.
//
// The local hub is written first and unconditionally: a publisher and a viewer
// in the same process must not depend on Redis being reachable to see each
// other. The Redis write is best effort — a dropped snapshot costs at most one
// tick of a bar, and the stream's safety poll covers it.
func (p *Publisher) Publish(ctx context.Context, s Snapshot) {
	if p == nil {
		return
	}
	if s.At.IsZero() {
		s.At = time.Now()
	}
	p.hub.Publish(s)

	if p.rdb == nil {
		return
	}
	payload, err := json.Marshal(s)
	if err != nil {
		return
	}
	// Deliberately not the request context: a snapshot must still be delivered
	// when the thing that produced it is finishing up or being cancelled, and
	// the terminal snapshot is the one that matters most.
	sendCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	if err := p.rdb.Publish(sendCtx, channel, payload).Err(); err != nil && p.log != nil {
		p.log.DebugContext(ctx, "progress publish failed", "run", s.ID, "error", err)
	}
}

// Bridge feeds a local hub from the Redis channel.
//
// Run one per server process. It returns when ctx is cancelled; a connection
// that drops is retried, because a bridge that gives up silently turns every
// progress bar in the deployment into a five-second poll with nobody the wiser.
func Bridge(ctx context.Context, rdb *redis.Client, hub *Hub, log *slog.Logger) {
	if rdb == nil || hub == nil {
		return
	}
	const retryDelay = 2 * time.Second

	for ctx.Err() == nil {
		sub := rdb.Subscribe(ctx, channel)
		ch := sub.Channel()

		for msg := range ch {
			var s Snapshot
			if err := json.Unmarshal([]byte(msg.Payload), &s); err != nil {
				continue
			}
			// Nothing is watching this run in this process. Dropping it here is
			// what keeps a hub from accumulating a room for every import the
			// whole deployment runs.
			if hub.Watchers(s.ID) == 0 {
				continue
			}
			hub.Publish(s)
		}

		_ = sub.Close()
		if ctx.Err() != nil {
			return
		}
		if log != nil {
			log.WarnContext(ctx, "progress bridge disconnected; retrying")
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(retryDelay):
		}
	}
}
