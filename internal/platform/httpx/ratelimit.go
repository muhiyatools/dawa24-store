package httpx

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// Limiter provides Redis-backed rate limiting middlewares.
type Limiter struct {
	rdb    *redis.Client
	prefix string
}

// NewLimiter creates a new rate limiter instance.
func NewLimiter(rdb *redis.Client, prefix string) *Limiter {
	if prefix == "" {
		prefix = "dawa24:ratelimit:"
	}
	return &Limiter{rdb: rdb, prefix: prefix}
}

// LimitByIP creates a middleware that restricts requests per client IP address.
func (l *Limiter) LimitByIP(limit int, window time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if l == nil || l.rdb == nil {
				next.ServeHTTP(w, r)
				return
			}

			ip := getClientIP(r)
			key := fmt.Sprintf("%sip:%s", l.prefix, ip)

			allowed, err := l.allow(r.Context(), key, limit, window)
			if err != nil || !allowed {
				Error(w, r, nil, apperr.New(apperr.KindRateLimited, "rate_limit_exceeded", "Too many requests. Please try again later."))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// LimitByOrg creates a middleware that restricts requests per tenant organization.
func (l *Limiter) LimitByOrg(limit int, window time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if l == nil || l.rdb == nil {
				next.ServeHTTP(w, r)
				return
			}

			orgID, ok := database.TenantFrom(r.Context())
			if !ok {
				orgID = 0
			}

			key := fmt.Sprintf("%sorg:%d", l.prefix, orgID)
			allowed, err := l.allow(r.Context(), key, limit, window)
			if err != nil || !allowed {
				Error(w, r, nil, apperr.New(apperr.KindRateLimited, "org_rate_limit_exceeded", "Organization rate limit exceeded. Please try again later."))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func (l *Limiter) allow(ctx context.Context, key string, limit int, window time.Duration) (bool, error) {
	count, err := l.rdb.Incr(ctx, key).Result()
	if err != nil {
		return true, err // Fail open on Redis error so legitimate traffic is not dropped
	}
	if count == 1 {
		l.rdb.Expire(ctx, key, window)
	}
	return count <= int64(limit), nil
}

func getClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		clientIP := strings.TrimSpace(parts[0])
		if clientIP != "" {
			return clientIP
		}
	}
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return strings.TrimSpace(xri)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}
