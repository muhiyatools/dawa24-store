package pagecontrol

import (
	"log/slog"
	"net/http"
)

// Guard wraps the application router. It answers a disabled route with the same
// handler the router uses for an unknown route, so a disabled page is
// indistinguishable from one that never existed, and otherwise delegates.
//
// It runs before every middleware the router carries, authentication included:
// a disabled page is hidden from an anonymous visitor exactly as it is from a
// signed-in one, on both the HTML and the JSON surface. It is stateless — it
// never redirects and never touches the request context — so it cannot interfere
// with auth, CSRF or the session.
func Guard(next http.Handler, notFound http.HandlerFunc, log *slog.Logger) http.Handler {
	if log == nil {
		log = slog.Default()
	}
	if notFound == nil {
		notFound = http.NotFound
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := NormalizePath(r.URL.Path)
		if IsProtected(p) {
			next.ServeHTTP(w, r)
			return
		}
		e := Global()
		if e == nil {
			next.ServeHTTP(w, r)
			return
		}
		if blocked, ruleID := e.Decision(p); blocked {
			log.WarnContext(r.Context(), "pagecontrol: route disabled",
				"path", p, "method", r.Method, "rule_id", ruleID)
			notFound(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}
