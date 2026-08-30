// Package httpx holds HTTP middleware and response helpers.
//
// Everything here is transport concern only. No business rule lives in this
// package, and no module imports net/http outside its own http/ subdirectory.
package httpx

import (
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"github.com/muhiya/dawa24-store/internal/platform/observability"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// RequestID assigns or adopts a correlation id and echoes it back.
//
// An inbound id is trusted only for correlation, never for authorisation, and is
// length-capped so a hostile client cannot inflate every log line downstream.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if id == "" || len(id) > 64 {
			id = newRequestID()
		}
		ctx := observability.WithRequestID(r.Context(), id)
		w.Header().Set("X-Request-ID", id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func newRequestID() string {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand failing is close to unrecoverable, but a correlation id is
		// not worth killing a request over; fall back to a timestamp.
		return "ts-" + time.Now().UTC().Format("20060102T150405.000000000")
	}
	return hex.EncodeToString(b)
}

// Recover converts a panic into a 500 and keeps the process alive.
//
// It re-panics on http.ErrAbortHandler, which is the standard library's signal
// that the handler intentionally gave up on a connection.
func Recover(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				rec := recover()
				if rec == nil {
					return
				}
				if rec == http.ErrAbortHandler {
					panic(rec)
				}
				log.ErrorContext(r.Context(), "panic recovered",
					"panic", rec,
					"path", r.URL.Path,
					"method", r.Method,
					"stack", string(debug.Stack()),
				)
				if !headersSent(w) {
					http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// Logger records one structured line per request.
type statusWriter struct {
	http.ResponseWriter
	status int
	bytes  int
	wrote  bool
}

func (w *statusWriter) WriteHeader(code int) {
	if w.wrote {
		return
	}
	w.status = code
	w.wrote = true
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if !w.wrote {
		w.WriteHeader(http.StatusOK)
	}
	n, err := w.ResponseWriter.Write(b)
	w.bytes += n
	return n, err
}

// Unwrap lets http.ResponseController reach the underlying writer.
func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// Flush forwards to the underlying writer.
//
// Unwrap alone is not enough: it only helps callers that go through
// http.ResponseController. A handler doing the conventional
// `w.(http.Flusher)` assertion sees this wrapper, not the real writer, and
// without this method the assertion fails — which silently disabled every SSE
// endpoint behind this middleware, including import progress.
func (w *statusWriter) Flush() {
	if !w.wrote {
		w.WriteHeader(http.StatusOK)
	}
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func headersSent(w http.ResponseWriter) bool {
	sw, ok := w.(*statusWriter)
	return ok && sw.wrote
}

func Logger(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}

			next.ServeHTTP(sw, r)

			level := slog.LevelInfo
			switch {
			case sw.status >= 500:
				level = slog.LevelError
			case sw.status >= 400:
				level = slog.LevelWarn
			}

			log.Log(r.Context(), level, "http request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", sw.status,
				"bytes", sw.bytes,
				"duration_ms", time.Since(start).Milliseconds(),
				"ip", clientIP(r),
			)
		})
	}
}

// SecurityHeaders applies baseline hardening while allowing maps, fonts, and required scripts.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "SAMEORIGIN")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("Permissions-Policy", "geolocation=(self), camera=(), microphone=()")
		h.Set("Cross-Origin-Opener-Policy", "same-origin-allow-popups")
		// htmx, Alpine and Leaflet are served from this origin now, so the three
		// CDN origins that used to be script and style sources are gone. Two
		// relaxations remain and both are load-bearing rather than sloppy:
		//
		//   'unsafe-inline' — 56 inline <script> blocks live in the templates.
		//   'unsafe-eval'   — Alpine 3 compiles expressions with new Function.
		//
		// Removing the first means moving those blocks into app.js; removing
		// the second means switching to Alpine's CSP build, which requires
		// rewriting the expressions it can no longer evaluate. Until both are
		// done this policy stops cross-origin script injection but not inline
		// injection, and saying so here is more useful than implying otherwise.
		h.Set("Content-Security-Policy", strings.Join([]string{
			"default-src 'self'",
			"script-src 'self' 'unsafe-inline' 'unsafe-eval'",
			"style-src 'self' 'unsafe-inline' https://fonts.googleapis.com",
			// Remote images are product and organization media held on object
			// storage, plus OpenStreetMap tiles.
			"img-src 'self' data: blob: https:",
			"font-src 'self' data: https://fonts.gstatic.com",
			"connect-src 'self' https:",
			"frame-src 'self' https://www.google.com https://maps.google.com https://*.google.com https://*.openstreetmap.org",
			"child-src 'self' blob:",
			"object-src 'none'",
			"base-uri 'self'",
			"form-action 'self'",
			"frame-ancestors 'self'",
		}, "; "))
		next.ServeHTTP(w, r)
	})
}

// Locale resolves the request language and writes it into the context.
//
// Order of precedence: explicit ?lang override, then the user's saved cookie,
// then Accept-Language, then Arabic. Arabic is the default because this platform
// is Arabic-first, not because it is alphabetically convenient.
func Locale(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var lang i18n.Lang

		switch {
		case r.URL.Query().Get("lang") != "":
			lang = i18n.ParseLang(r.URL.Query().Get("lang"))
			http.SetCookie(w, &http.Cookie{
				Name: "lang", Value: string(lang), Path: "/",
				MaxAge: 365 * 24 * 3600, HttpOnly: false, SameSite: http.SameSiteLaxMode,
			})
		case cookieValue(r, "lang") != "":
			lang = i18n.ParseLang(cookieValue(r, "lang"))
		case r.Header.Get("Accept-Language") != "":
			lang = i18n.ParseLang(r.Header.Get("Accept-Language"))
		default:
			lang = i18n.Default
		}

		next.ServeHTTP(w, r.WithContext(WithLang(r.Context(), lang)))
	})
}

func cookieValue(r *http.Request, name string) string {
	c, err := r.Cookie(name)
	if err != nil {
		return ""
	}
	return c.Value
}

// clientIP resolves the caller address, preferring X-Forwarded-For only because
// the app runs behind Elest.io's proxy. It takes the first entry, which is the
// original client when the proxy chain is trusted.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if idx := strings.IndexByte(xff, ','); idx > 0 {
			return strings.TrimSpace(xff[:idx])
		}
		return strings.TrimSpace(xff)
	}
	if rip := r.Header.Get("X-Real-IP"); rip != "" {
		return rip
	}
	host, _, found := strings.Cut(r.RemoteAddr, ":")
	if !found {
		return r.RemoteAddr
	}
	return host
}
