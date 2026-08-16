package httpx

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"strings"

	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// CSRFCookieName is the cookie holding the double-submit CSRF token.
const CSRFCookieName = "dawa_csrf"

// CSRFHeaderName is the HTTP request header sent by HTMX/fetch.
const CSRFHeaderName = "X-CSRF-Token"

// GenerateCSRFToken generates a cryptographically secure hex token.
func GenerateCSRFToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// CSRF is a double-submit cookie CSRF middleware for browser requests.
// API requests authenticated via Authorization header are exempt.
func CSRF(isProd bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip CSRF validation for safe HTTP methods
			if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions || r.Method == http.MethodTrace {
				ensureCSRFCookie(w, r, isProd)
				next.ServeHTTP(w, r)
				return
			}

			// Skip CSRF if client authenticates via Authorization header (API / programmatic clients)
			if auth := r.Header.Get("Authorization"); auth != "" && strings.HasPrefix(auth, "Bearer ") {
				next.ServeHTTP(w, r)
				return
			}

			// Verify cookie token against header or form
			cookie, err := r.Cookie(CSRFCookieName)
			if err != nil || cookie.Value == "" {
				Error(w, r, nil, apperr.Forbidden("csrf.missing_cookie", "CSRF cookie is missing."))
				return
			}

			headerToken := r.Header.Get(CSRFHeaderName)
			if headerToken == "" {
				headerToken = r.FormValue("_csrf")
			}

			if headerToken == "" || subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(headerToken)) != 1 {
				Error(w, r, nil, apperr.Forbidden("csrf.invalid_token", "CSRF verification failed."))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func ensureCSRFCookie(w http.ResponseWriter, r *http.Request, isProd bool) {
	if _, err := r.Cookie(CSRFCookieName); err == nil {
		return
	}
	token, err := GenerateCSRFToken()
	if err != nil {
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     CSRFCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: false, // Must be readable by client-side HTMX script to attach to headers
		SameSite: http.SameSiteLaxMode,
		Secure:   isProd,
	})
}
