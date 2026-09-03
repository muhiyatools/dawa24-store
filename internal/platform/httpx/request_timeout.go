package httpx

import (
	"context"
	"net/http"
	"strings"
	"time"
)

// RequestTimeout gives every request a deadline on its context.
//
// Without one, a handler blocked on a slow query or an exhausted connection
// pool waits indefinitely. Two things then happen, in this order: the server's
// WriteTimeout fires and closes the connection, and the reverse proxy in front
// of it — which saw a connection close before any response — reports 502 Bad
// Gateway. The user sees a gateway error for what was really one slow query.
//
// Worse, the abandoned handler keeps its pool connection. Under load the slow
// requests pile up, the pool empties, and every other request starts waiting
// too: one slow endpoint takes the whole site down with 502s that name no
// cause.
//
// A context deadline fixes the cause rather than the symptom. pgx honours it,
// so the query is cancelled server-side and the pool connection comes straight
// back; the handler returns an error the app renders as a normal page. The
// deadline is set shorter than the server's WriteTimeout on purpose, so the
// application always wins the race and gets to write a real response.
//
// Long-lived responses are exempt: a streamed download or an event stream is
// meant to stay open, and a deadline would truncate it.
func RequestTimeout(d time.Duration, exempt func(*http.Request) bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if d <= 0 || (exempt != nil && exempt(r)) {
				next.ServeHTTP(w, r)
				return
			}
			ctx, cancel := context.WithTimeout(r.Context(), d)
			defer cancel()
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// longRunningPrefixes are the paths whose responses are meant to stay open
// longer than an ordinary page: file exports, import runs the browser polls,
// and streamed assistant replies.
var longRunningPrefixes = []string{
	// A batch of supplier spreadsheets is read, parsed and matched inside the
	// request; a dozen large files legitimately outlast an ordinary deadline,
	// and cutting one short abandons the batch half-processed.
	"/compare/upload",
	"/compare/run",
	"/compare/files/",
	"/compare/file/",
	"/vendor/ingest/",
	"/admin/products/import/",
	"/admin/products/images/import/",
	"/smart-order/",
	"/assistant/",
	"/documents/",
}

// IsLongRunning reports whether a request should be exempt from the standard
// request deadline.
func IsLongRunning(r *http.Request) bool {
	p := r.URL.Path
	if strings.HasSuffix(p, "/export") || strings.HasSuffix(p, ".xlsx") || strings.HasSuffix(p, ".csv") {
		return true
	}
	for _, prefix := range longRunningPrefixes {
		if strings.HasPrefix(p, prefix) {
			return true
		}
	}
	// Server-sent events and other streams.
	return strings.Contains(r.Header.Get("Accept"), "text/event-stream")
}
