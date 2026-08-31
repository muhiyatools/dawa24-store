package antiscrape

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/httpx"
)

// Window lengths. Two of them, because one cannot separate the two behaviours
// worth separating: a burst window catches a parallel fetcher, and a long
// window catches the patient one that paces itself just under the burst limit.
const (
	burstWindow     = 10 * time.Second
	sustainedWindow = 10 * time.Minute
)

// budget is how many protected requests one caller may make per window.
type budget struct {
	burst     int
	sustained int
}

// budgets per class. The numbers are set from what a person does: opening the
// catalogue, filtering it, typing in the search box (htmx debounces to roughly
// three requests a second while typing) and paging through results. Forty
// requests in ten seconds is a busy pharmacist; four hundred in ten minutes is
// a very busy one. Neither is a machine reading a price list.
var budgets = map[Class]budget{
	ClassBrowser: {burst: 40, sustained: 400},
	ClassCrawler: {burst: 10, sustained: 150},
	ClassUnknown: {burst: 12, sustained: 90},
	// ClassAutomation never reaches a budget when signed out; when signed in it
	// is treated as Unknown. See Protect.
	ClassAutomation: {burst: 12, sustained: 90},
}

// authenticatedFactor multiplies the budget of a signed-in caller.
//
// A pharmacy building a basket of two hundred lines is a heavier user than any
// anonymous visitor, and it has a real account to lose, so it is metered per
// user rather than per address and given room to work.
const authenticatedFactor = 4

// Options configures a Guard. The zero value is a disabled guard.
type Options struct {
	// Enabled turns the whole guard off. A disabled guard is a passthrough.
	Enabled bool
	// Redis resolves the shared counter store. Called per request because the
	// server starts before Redis is dialled; nil, or returning nil, falls back
	// to per-process counters.
	Redis func() *redis.Client
	// Log receives one line per refusal. Defaults to slog.Default().
	Log *slog.Logger
	// KeyPrefix namespaces the counters, normally by environment.
	KeyPrefix string
	// TrustedProxyHops is how many reverse proxies sit in front of this
	// process. See httpx.ClientIP: without it, X-Forwarded-For is caller-
	// supplied text and every request can claim a fresh address.
	TrustedProxyHops int
	// PenaltyTTL is how long a honeypot trip keeps a caller refused.
	PenaltyTTL time.Duration
}

// Guard applies classification, budgets and penalties to a route group.
//
// Every method is nil-safe and returns a passthrough when the guard is absent,
// so a test harness or a deployment without Redis mounts the same routes.
type Guard struct {
	enabled    bool
	log        *slog.Logger
	store      store
	prefix     string
	hops       int
	penaltyTTL time.Duration
}

// New builds a Guard from Options.
func New(o Options) *Guard {
	log := o.Log
	if log == nil {
		log = slog.Default()
	}
	prefix := o.KeyPrefix
	if prefix == "" {
		prefix = "dawa24:antiscrape:"
	}
	ttl := o.PenaltyTTL
	if ttl <= 0 {
		ttl = time.Hour
	}
	hops := o.TrustedProxyHops
	if hops < 0 {
		hops = 0
	}
	return &Guard{
		enabled:    o.Enabled,
		log:        log,
		store:      newHybridStore(o.Redis),
		prefix:     prefix,
		hops:       hops,
		penaltyTTL: ttl,
	}
}

// Enabled reports whether this guard does anything.
func (g *Guard) Enabled() bool { return g != nil && g.enabled }

// Protect is the middleware for a public route that publishes a lot of data.
//
// It is deliberately not mounted on the marketing pages: /about does not need
// a request budget, and every middleware on a route is latency on it.
func (g *Guard) Protect(next http.Handler) http.Handler {
	if !g.Enabled() {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		id, authenticated := g.identify(r)

		if g.store.Penalized(ctx, g.prefix+"penalty:"+id) {
			g.refuse(w, r, reasonPenalty, ClassUnknown)
			return
		}

		class := Classify(r)
		if class == ClassAutomation && !authenticated {
			g.refuse(w, r, reasonAutomation, class)
			return
		}

		b := budgets[class]
		if authenticated {
			b.burst *= authenticatedFactor
			b.sustained *= authenticatedFactor
		}

		if !g.within(ctx, id, "burst", burstWindow, b.burst) ||
			!g.within(ctx, id, "sustained", sustainedWindow, b.sustained) {
			g.refuse(w, r, reasonBudget, class)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// RequireSiteOrigin refuses a request that did not come from a page of this
// site. It guards the JSON endpoints, whose whole value to a scraper is that
// they answer a bare URL with structured data.
func (g *Guard) RequireSiteOrigin(next http.Handler) http.Handler {
	if !g.Enabled() {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, authenticated := g.identify(r); authenticated || FromSite(r) {
			next.ServeHTTP(w, r)
			return
		}
		g.refuse(w, r, reasonOffSite, Classify(r))
	})
}

// Penalize puts the caller behind this request on the refused list for the
// penalty window. It is called when a caller does something no person does —
// filling a hidden form field, or fetching a path that only exists as bait.
func (g *Guard) Penalize(r *http.Request, reason string) {
	if !g.Enabled() {
		return
	}
	id, _ := g.identify(r)
	g.store.Penalize(r.Context(), g.prefix+"penalty:"+id, g.penaltyTTL)
	g.log.WarnContext(r.Context(), "antiscrape: caller penalised",
		"reason", reason,
		"path", r.URL.Path,
		"ip", httpx.ClientIP(r, g.hops),
		"user_agent", r.Header.Get("User-Agent"),
		"for", g.penaltyTTL.String(),
	)
}

// Trap is the handler for a bait path: a link no person can see, disallowed in
// robots.txt, reachable only by something that reads the HTML and follows
// everything in it.
func (g *Guard) Trap(w http.ResponseWriter, r *http.Request) {
	g.Penalize(r, "honeypot_path")
	g.refuse(w, r, reasonPenalty, Classify(r))
}

// within reports whether one more request fits in the caller's allowance.
func (g *Guard) within(ctx context.Context, id, window string, d time.Duration, limit int) bool {
	if limit <= 0 {
		return true
	}
	key := g.prefix + "rl:" + window + ":" + id
	return g.store.Hit(ctx, key, d) <= int64(limit)
}

// identify returns the counting key for a caller and whether it is signed in.
//
// A signed-in caller is metered per user: an office of pharmacists sharing one
// public address is one address and many people, and counting them together is
// how a real customer gets refused for their colleague's browsing.
func (g *Guard) identify(r *http.Request) (string, bool) {
	if actor, ok := authctx.From(r.Context()); ok && actor.UserID > 0 {
		return "u" + strconv.FormatInt(actor.UserID, 10), true
	}
	return "ip" + httpx.ClientIP(r, g.hops), false
}
