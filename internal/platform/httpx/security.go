package httpx

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"strings"
)

// SecurityHeaders applies baseline hardening while allowing maps, fonts, and required scripts.
//
// A fresh 16-byte CSP nonce is generated for every request and stored in the
// context (layouts.WithNonce). Templates read it with layouts.Nonce(ctx) to
// stamp each inline <script> tag, which lets the policy allow those scripts
// without the blanket 'unsafe-inline' relaxation.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "SAMEORIGIN")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		// microphone=(self) and not microphone=(): the assistant's voice input
		// calls getUserMedia({audio:true}) from this origin. With the
		// microphone disabled for every origin including our own, that call
		// rejected before the browser ever showed a permission prompt, so the
		// microphone button did nothing at all and the failure was invisible.
		// Camera stays closed: nothing here opens a camera stream, and file
		// attachment via <input capture> goes through the OS picker instead.
		h.Set("Permissions-Policy", "geolocation=(self), camera=(), microphone=(self)")
		h.Set("Cross-Origin-Opener-Policy", "same-origin-allow-popups")
		h.Set("Content-Signal", "ai-train=no, search=yes, ai-input=no")
		// HSTS: one year, including subdomains. Without this header, a single
		// plaintext HTTP request — a bare hostname bookmark, an image on a
		// subdomain — transmits a 30-day session token in cleartext.
		h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		h.Set("X-Permitted-Cross-Domain-Policies", "none")
		h.Set("Origin-Agent-Cluster", "?1")

		// Generate a fresh nonce for this request. 16 random bytes → 22-char
		// base64 is the minimum OWASP recommends; crypto/rand is the only
		// acceptable source.
		var nonceBuf [16]byte
		_, _ = rand.Read(nonceBuf[:])
		nonce := base64.RawStdEncoding.EncodeToString(nonceBuf[:])

		// Build the per-request CSP by inserting the nonce into script-src.
		//
		// With a nonce present, 'unsafe-inline' is ignored by spec (CSP2+),
		// so inline <script> blocks execute only if they carry the matching
		// nonce attribute and injected scripts are blocked. 'unsafe-eval'
		// remains because Alpine 3 compiles expressions with new Function();
		// removing it requires switching to the @alpinejs/csp build.
		//
		// Inline event handlers (onclick= etc.) are NOT covered by nonces.
		// script-src-attr 'unsafe-inline' covers them during the migration
		// period while they are being converted to addEventListener / Alpine calls.
		csp := "script-src 'self' 'nonce-" + nonce + "' 'unsafe-eval'; " +
			"script-src-attr 'unsafe-inline'; " +
			cspStaticDirectives

		h.Set("Content-Security-Policy", csp)
		if PrivateArea(r.URL.Path) {
			h.Set("X-Robots-Tag", "noindex, nofollow")
		}

		// Store the nonce in context so templates can read it with Nonce(ctx).
		ctx := r.Context()
		ctx = WithNonce(ctx, nonce)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// cspStaticDirectives holds the CSP directives that do not change per request.
// Precomputed at startup to avoid string joins on every response.
var cspStaticDirectives = strings.Join([]string{
	"default-src 'none'",
	"style-src 'self' 'unsafe-inline' https://fonts.googleapis.com",
	// Remote images are product and organization media held on object
	// storage, plus OpenStreetMap tiles.
	"img-src 'self' data: blob: https:",
	"media-src 'self' blob: https:",
	"font-src 'self' data: https://fonts.gstatic.com",
	// Same-origin XHR, fetch and the assistant's event streams, plus the
	// address lookup the branch map performs in the browser. It was any https
	// host, which let injected script send data anywhere.
	"connect-src 'self' https://nominatim.openstreetmap.org",
	"frame-src 'self' https://www.google.com https://maps.google.com https://*.google.com https://*.openstreetmap.org",
	"child-src 'self' blob:",
	"worker-src 'self' blob:",
	"manifest-src 'self'",
	"object-src 'none'",
	"base-uri 'self'",
	"form-action 'self'",
	"frame-ancestors 'self'",
	"upgrade-insecure-requests",
	"report-uri /api/v1/csp-report",
}, "; ")

// privateAreas are the path prefixes that are never search results: dashboards,
// administration, account screens and the API.
var privateAreas = []string{"/admin", "/vendor", "/customer", "/account", "/api", "/moderator", "/settings"}

// PrivateArea reports whether a path lies in an area that must not be indexed.
// The OpenAPI description is the exception: it is published for agents.
func PrivateArea(path string) bool {
	if strings.HasPrefix(path, "/api/v1/openapi.") {
		return false
	}
	for _, prefix := range privateAreas {
		if path == prefix || strings.HasPrefix(path, prefix+"/") {
			return true
		}
	}
	return false
}
