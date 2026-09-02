// Package middleware — inbound per-key rate limiting for /v1/* endpoints.
//
// Design: fixed 1-minute window, per API key, setting-driven
// (settings.APIKeyRPM; 0 = disabled). The limiter runs AFTER APIKeyAuth so
// only authenticated, valid keys consume slots. Entries expire after the
// window and the map is opportunistically trimmed, bounding memory.
package middleware

import (
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/arisvia/cyrene-gateway/internal/auth"
)

type windowEntry struct {
	count    int
	windowAt time.Time // start of the current fixed window
}

// RateLimiter enforces a per-key fixed-window request rate.
type RateLimiter struct {
	mu      sync.Mutex
	entries map[string]*windowEntry
	limit   int
}

// NewRateLimiter creates a limiter allowing limit requests per key per minute.
func NewRateLimiter(limit int) *RateLimiter {
	return &RateLimiter{
		entries: make(map[string]*windowEntry),
		limit:   limit,
	}
}

// Allow reports whether key may proceed this minute, incrementing its count.
func (rl *RateLimiter) Allow(key string) bool {
	if rl == nil || rl.limit <= 0 {
		return true
	}
	now := time.Now()
	rl.mu.Lock()
	defer rl.mu.Unlock()

	// Opportunistic cleanup to keep the map bounded.
	if len(rl.entries) > 4096 {
		for k, e := range rl.entries {
			if now.Sub(e.windowAt) >= time.Minute {
				delete(rl.entries, k)
			}
		}
	}

	e, ok := rl.entries[key]
	if !ok || now.Sub(e.windowAt) >= time.Minute {
		rl.entries[key] = &windowEntry{count: 1, windowAt: now}
		return true
	}
	if e.count >= rl.limit {
		return false
	}
	e.count++
	return true
}

// APIKeyRateLimit enforces the per-key RPM on /v1/* routes. limitFn is
// evaluated per request so settings changes take effect without restart;
// limit 0 disables the limiter entirely.
func APIKeyRateLimit(limitFn func() int) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			limit := limitFn()
			if limit <= 0 || !strings.HasPrefix(r.URL.Path, "/v1/") {
				next.ServeHTTP(w, r)
				return
			}
			key := auth.ExtractAPIKey(r.Header.Get("Authorization"), r.Header.Get("x-api-key"))
			if key == "" {
				next.ServeHTTP(w, r)
				return
			}
			if !limiterFor(limit).Allow(key) {
				w.Header().Set("Retry-After", "60")
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				w.Write([]byte(`{"error":"rate limit exceeded for this API key","limit":` + strconv.Itoa(limit) + `}`))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

var (
	rlMu    sync.Mutex
	rlCache = map[int]*RateLimiter{}
)

// limiterFor reuses one limiter instance per distinct limit value so window
// state survives across settings reads that return the same value.
func limiterFor(limit int) *RateLimiter {
	rlMu.Lock()
	defer rlMu.Unlock()
	rl, ok := rlCache[limit]
	if !ok {
		rl = NewRateLimiter(limit)
		rlCache[limit] = rl
	}
	return rl
}
