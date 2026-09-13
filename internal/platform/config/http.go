package config

import "time"

// The HTTP server's shape: ports, deadlines, and how much of the request the
// process is allowed to believe.
//
// Split from config.go, which was at the 400-line ceiling.

type HTTP struct {
	Port         int
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	IdleTimeout  time.Duration
	// RequestTimeout is the deadline put on every request's context. It must
	// stay below WriteTimeout so the application ends a slow request before the
	// socket does — see httpx.RequestTimeout for why that is the difference
	// between a rendered error and a 502.
	RequestTimeout  time.Duration
	ShutdownTimeout time.Duration
	TrustedProxies  []string
	// TrustedProxyHops is how many reverse proxies sit in front of this
	// process. Every per-address defence depends on it: X-Forwarded-For is
	// written by the caller and appended to by each proxy, so the real client
	// is the entry this many places from the right. Set it wrong and either
	// every visitor shares one bucket (too high) or a scraper mints a fresh
	// identity per request (too low, or unset with a proxy in front).
	//
	// One is right for the current deployment: Elest.io's proxy and nothing
	// else. Zero is right when the process is exposed directly.
	TrustedProxyHops int
	// ModuleAPI mounts the per-module JSON APIs under /api/v1 (catalog,
	// inventory, commerce, billing, ingest, promo, workflow, hr, platform,
	// notifications, organizations). The web dashboard uses none of them — it
	// is server-rendered — and they were written for row-level security the
	// database does not enforce (the application connects as a superuser), so
	// several answered another organisation's records by id. Off in production
	// unless MODULE_API_ENABLED is set; the assistant, smart-order, attachment,
	// identity and integration APIs are mounted regardless.
	ModuleAPI bool
}
