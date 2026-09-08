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

// AllowWithLimit reports whether key may proceed this minute under the given limit, incrementing its count.
func (rl *RateLimiter) AllowWithLimit(key string, limit int) bool {
	if rl == nil || limit <= 0 {
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
	if e.count >= limit {
		return false
	}
	e.count++
	return true
}

// Allow reports whether key may proceed this minute, incrementing its count.
func (rl *RateLimiter) Allow(key string) bool {
	if rl == nil {
		return true
	}
	return rl.AllowWithLimit(key, rl.limit)
}

// APIKeyRateLimit enforces per-key RPM on /v1/* routes.
// If the authenticated APIKey has an explicit RPM > 0, it takes precedence.
// Otherwise it falls back to limitFn(keyStr).
func APIKeyRateLimit(limitFn func(key string) int) func(http.Handler) http.Handler {
	rl := NewRateLimiter(0)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !strings.HasPrefix(r.URL.Path, "/v1/") {
				next.ServeHTTP(w, r)
				return
			}

			keyObj := auth.APIKeyFromContext(r.Context())
			var keyStr string
			var limit int

			if keyObj != nil {
				keyStr = keyObj.Key
				if keyObj.RPM > 0 {
					limit = keyObj.RPM
				}
			}
			if keyStr == "" {
				keyStr = auth.ExtractAPIKey(r.Header.Get("Authorization"), r.Header.Get("x-api-key"))
			}

			if limit <= 0 && limitFn != nil {
				limit = limitFn(keyStr)
			}

			if limit <= 0 || keyStr == "" {
				next.ServeHTTP(w, r)
				return
			}

			if !rl.AllowWithLimit(keyStr, limit) {
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
