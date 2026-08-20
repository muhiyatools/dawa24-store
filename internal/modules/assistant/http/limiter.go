package http

import (
	"sync"
	"time"
)

// RateLimiter bounds user requests per minute.
type RateLimiter struct {
	mu      sync.Mutex
	limits  map[int64][]time.Time
	window  time.Duration
	maxReqs int
}

// NewRateLimiter creates a sliding window rate limiter.
func NewRateLimiter(maxReqs int, window time.Duration) *RateLimiter {
	return &RateLimiter{
		limits:  make(map[int64][]time.Time),
		window:  window,
		maxReqs: maxReqs,
	}
}

// Allow reports whether a request by userID is within rate limit.
func (rl *RateLimiter) Allow(userID int64) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-rl.window)

	reqs := rl.limits[userID]
	valid := make([]time.Time, 0, len(reqs))
	for _, t := range reqs {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}

	if len(valid) >= rl.maxReqs {
		rl.limits[userID] = valid
		return false
	}

	valid = append(valid, now)
	rl.limits[userID] = valid
	return true
}
